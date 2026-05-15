// internal/outbox/aliyun_mns_consumer.go — Aliyun MNS consumer (SINK)（Fix-B' part 3 CRIT-B5）.
//
// 拓扑：
//
//	[MNS queue (inbound)]
//	         │ ReceiveMessage long-poll
//	         ▼
//	[MNSConsumer.RunLoop]
//	         │ dispatch by event_type → registered handler
//	         ▼
//	  ┌──── 成功 ────► DeleteMessage
//	  └──── 失败 ────► leave message → MNS visibility 超时后 redeliver
//	                  redeliver 计数 >= N → MoveToDLQ
//
// 与 publisher 用同一份 envelope（attrs + body）格式。
//
// MNSClient 抽象 ReceiveMessage / DeleteMessage / MoveToDLQ，便于注入 fake 跑 tests。
package outbox

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// MNSMessage MNS 收到的一条消息（已 decode envelope）.
type MNSMessage struct {
	MessageID     string
	ReceiptHandle string // DeleteMessage 凭证
	Body          []byte
	Attrs         map[string]string
	DequeueCount  int
}

// MNSClient SDK 抽象（test 用 fake）.
type MNSClient interface {
	// ReceiveMessage 长轮询；ctx 取消 / 超时无消息 → 返 nil, nil（业务无错）.
	ReceiveMessage(ctx context.Context, queueName string, waitSec int) (*MNSMessage, error)
	// DeleteMessage 成功 ack；receipt 失效返 error.
	DeleteMessage(ctx context.Context, queueName, receiptHandle string) error
	// MoveToDLQ 失败 N 次后兜底；实现可选——若 MNS 配 RedrivePolicy，consumer 只 leave +
	// dequeueCount 达阈值则强删（"poison message" 处理）。本接口提供二选一弹性。
	MoveToDLQ(ctx context.Context, queueName, dlqName string, msg *MNSMessage) error
}

// Handler 单条 MNS 消息处理函数；error == nil 视为成功（删消息）.
type Handler func(ctx context.Context, msg *MNSMessage) error

// MNSConsumer 长轮询 + dispatch.
type MNSConsumer struct {
	client       MNSClient
	queueName    string
	dlqName      string
	dataRegion   string
	waitSec      int
	dlqThreshold int

	mu       sync.RWMutex
	handlers map[string]Handler

	// noopOnUnknown true → 未注册 event_type 时 log warning + ack（不卡队列）.
	noopOnUnknown bool
}

// MNSConsumerOptions 构造参数.
type MNSConsumerOptions struct {
	QueueName     string // sink queue
	DLQName       string // dead-letter queue
	DataRegion    string // cn / sg; non-empty requires matching msg attr data_region
	WaitSeconds   int    // long-poll 等待秒；默认 20（MNS 最大 30）
	DLQThreshold  int    // dequeue_count >= 此值 → MoveToDLQ；默认 10
	NoopOnUnknown bool   // 未注册 event_type 时 log+ack（默认 true）
}

// NewMNSConsumer 构造.
func NewMNSConsumer(client MNSClient, opts MNSConsumerOptions) (*MNSConsumer, error) {
	if client == nil {
		return nil, errors.New("mns consumer: nil client")
	}
	if opts.QueueName == "" {
		return nil, errors.New("mns consumer: queue name required")
	}
	if opts.WaitSeconds <= 0 {
		opts.WaitSeconds = 20
	}
	if opts.DLQThreshold <= 0 {
		opts.DLQThreshold = 10
	}
	return &MNSConsumer{
		client:        client,
		queueName:     opts.QueueName,
		dlqName:       opts.DLQName,
		dataRegion:    opts.DataRegion,
		waitSec:       opts.WaitSeconds,
		dlqThreshold:  opts.DLQThreshold,
		handlers:      map[string]Handler{},
		noopOnUnknown: opts.NoopOnUnknown,
	}, nil
}

// Register 注册 event_type → handler；同 event_type 重复注册以最后一次为准.
func (c *MNSConsumer) Register(eventType string, h Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[eventType] = h
}

// Run 长轮询主循环；ctx.Done 后退出.
func (c *MNSConsumer) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := c.tick(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			log.Warn().Err(err).Str("queue", c.queueName).Msg("mns_consumer_tick_failed")
			// 短暂退避，避免 hot loop
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		}
	}
}

// tick 单次 receive → dispatch → delete.
func (c *MNSConsumer) tick(ctx context.Context) error {
	msg, err := c.client.ReceiveMessage(ctx, c.queueName, c.waitSec)
	if err != nil {
		return err
	}
	if msg == nil {
		return nil // long-poll timeout，无消息
	}
	return c.handleOne(ctx, msg)
}

// handleOne 处理单条消息（test 入口）.
func (c *MNSConsumer) handleOne(ctx context.Context, msg *MNSMessage) error {
	// dequeue_count 阈值检测 → DLQ
	if msg.DequeueCount >= c.dlqThreshold && c.dlqName != "" {
		log.Warn().
			Str("queue", c.queueName).
			Str("msg_id", msg.MessageID).
			Int("dequeue_count", msg.DequeueCount).
			Msg("mns_move_to_dlq")
		if err := c.client.MoveToDLQ(ctx, c.queueName, c.dlqName, msg); err != nil {
			return fmt.Errorf("mns consumer: move to dlq: %w", err)
		}
		return c.client.DeleteMessage(ctx, c.queueName, msg.ReceiptHandle)
	}

	eventType := msg.Attrs["event_type"]
	if c.dataRegion != "" && msg.Attrs["data_region"] != c.dataRegion {
		log.Error().
			Str("queue", c.queueName).
			Str("event_type", eventType).
			Str("msg_id", msg.MessageID).
			Str("message_region", msg.Attrs["data_region"]).
			Str("expected_region", c.dataRegion).
			Msg("mns_data_region_mismatch_leave_for_dlq")
		return nil
	}
	c.mu.RLock()
	h, ok := c.handlers[eventType]
	c.mu.RUnlock()
	if !ok {
		if c.noopOnUnknown {
			log.Warn().
				Str("queue", c.queueName).
				Str("event_type", eventType).
				Str("msg_id", msg.MessageID).
				Msg("mns_unknown_event_type_noop_ack")
			return c.client.DeleteMessage(ctx, c.queueName, msg.ReceiptHandle)
		}
		log.Error().
			Str("queue", c.queueName).
			Str("event_type", eventType).
			Msg("mns_no_handler")
		return nil // leave in queue → redeliver
	}
	if err := h(ctx, msg); err != nil {
		log.Warn().
			Err(err).
			Str("queue", c.queueName).
			Str("event_type", eventType).
			Str("msg_id", msg.MessageID).
			Int("dequeue_count", msg.DequeueCount).
			Msg("mns_handler_error_leave_for_redeliver")
		return nil // 不 delete；MNS visibility 超时后自动 redeliver
	}
	return c.client.DeleteMessage(ctx, c.queueName, msg.ReceiptHandle)
}

// ---- HTTP MNSClient（生产实现）----

// HTTPMNSClient REST 实现.
type HTTPMNSClient struct {
	cfg    MNSConfig
	client *http.Client
}

// NewHTTPMNSClient 构造.
func NewHTTPMNSClient(cfg MNSConfig) (*HTTPMNSClient, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("mns client: endpoint required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 35 * time.Second // 大于 waitSec
	}
	return &HTTPMNSClient{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}, nil
}

// receiveResp MNS GET /queues/{q}/messages 返回 XML.
type receiveResp struct {
	XMLName       xml.Name `xml:"Message"`
	MessageID     string   `xml:"MessageId"`
	ReceiptHandle string   `xml:"ReceiptHandle"`
	MessageBody   string   `xml:"MessageBody"`
	DequeueCount  int      `xml:"DequeueCount"`
}

// ReceiveMessage long-poll 拉一条消息.
func (h *HTTPMNSClient) ReceiveMessage(ctx context.Context, queueName string, waitSec int) (*MNSMessage, error) {
	path := fmt.Sprintf("/queues/%s/messages?waitseconds=%d", url.PathEscape(queueName), waitSec)
	endpoint := strings.TrimRight(h.cfg.Endpoint, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+path, nil)
	if err != nil {
		return nil, err
	}
	signMNSRequest(req, nil, h.cfg.AccessKeyID, h.cfg.AccessKeySecret, "application/xml")
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		// MNS 返回 404 当 queue 为空（MessageNotExist）
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("mns receive: status=%d body=%s", resp.StatusCode, string(b))
	}
	var rr receiveResp
	body, _ := io.ReadAll(resp.Body)
	if err := xml.Unmarshal(body, &rr); err != nil {
		return nil, fmt.Errorf("mns receive: xml parse: %w", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(rr.MessageBody)
	if err != nil {
		decoded = []byte(rr.MessageBody) // 容错：未 base64 时直接当原始 body
	}
	attrs, body2 := parseMNSEnvelope(decoded)
	if attrs["dequeue_count"] == "" {
		attrs["dequeue_count"] = strconv.Itoa(rr.DequeueCount)
	}
	return &MNSMessage{
		MessageID:     rr.MessageID,
		ReceiptHandle: rr.ReceiptHandle,
		Body:          body2,
		Attrs:         attrs,
		DequeueCount:  rr.DequeueCount,
	}, nil
}

// DeleteMessage ack.
func (h *HTTPMNSClient) DeleteMessage(ctx context.Context, queueName, receiptHandle string) error {
	path := fmt.Sprintf("/queues/%s/messages?ReceiptHandle=%s",
		url.PathEscape(queueName), url.QueryEscape(receiptHandle))
	endpoint := strings.TrimRight(h.cfg.Endpoint, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint+path, nil)
	if err != nil {
		return err
	}
	signMNSRequest(req, nil, h.cfg.AccessKeyID, h.cfg.AccessKeySecret, "application/xml")
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("mns delete: status=%d body=%s", resp.StatusCode, string(b))
}

// MoveToDLQ：partner-api 实现为「republish 到 DLQ + 原队列 delete」（MNS REST 无原生 move）。
// 需要 publisher；若没注入，return error.
//
// 实际生产建议：在 MNS console 给主队列配 RedrivePolicy → DLQ，依赖云端搬移。
// 此实现是 fallback / unit test 用。
func (h *HTTPMNSClient) MoveToDLQ(ctx context.Context, queueName, dlqName string, msg *MNSMessage) error {
	pub, err := NewMNSPublisher(h.cfg)
	if err != nil {
		return err
	}
	return pub.Publish(ctx, dlqName, msg.Body, msg.Attrs)
}
