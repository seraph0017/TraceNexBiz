// internal/outbox/aliyun_mns_publisher.go — Aliyun MNS publisher（Fix-B' part 3 CRIT-B5）.
//
// 设计选择：raw HTTP，不引入 aliyun-mns-go-sdk
//
//	理由：Aliyun MNS Go SDK 会拉入 30+ 间接依赖（aliyun-sdk-core / encrypt / utils），
//	对一个仅做 SendMessage/ReceiveMessage/DeleteMessage 的客户端而言体量过大；
//	MNS REST API 签名规则简单（HMAC-SHA1 over canonical headers + path），自实现易于审计。
//	若日后切回 SDK，只需替换 publisher 实现，Publisher interface 不变。
//
// 接口：
//
//	type Publisher interface {
//	    Publish(ctx, queueName, body []byte, attrs map[string]string) error
//	}
//
// 重试：transient 错误（5xx / network）3 次指数退避（1s / 2s / 4s）.
// 超时：单次 HTTP 请求 cfg.Timeout（默认 5s）.
// 日志：用 zerolog 输出 publish_attempt / publish_success / publish_failure 标签.
package outbox

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// Publisher 抽象 MNS 发布器（service 层注入 + 测试用 fake）.
type Publisher interface {
	Publish(ctx context.Context, queueName string, body []byte, attrs map[string]string) error
}

// MNSConfig 阿里云 MNS 配置.
type MNSConfig struct {
	Endpoint        string // e.g. https://1234567890.mns.cn-hangzhou.aliyuncs.com
	AccessKeyID     string
	AccessKeySecret string
	Timeout         time.Duration // 单次 HTTP 请求超时；默认 5s
	MaxRetries      int           // 最大重试；默认 3
}

// MNSPublisher 阿里云 MNS REST API publisher（raw HTTP）.
type MNSPublisher struct {
	cfg    MNSConfig
	client *http.Client
}

// NewMNSPublisher 构造；cfg.Timeout / cfg.MaxRetries 0 时取默认.
func NewMNSPublisher(cfg MNSConfig) (*MNSPublisher, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("mns publisher: endpoint required")
	}
	if cfg.AccessKeyID == "" || cfg.AccessKeySecret == "" {
		return nil, errors.New("mns publisher: access key required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	return &MNSPublisher{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}, nil
}

// sendMessageXML MNS SendMessage 请求 body（XML）.
type sendMessageXML struct {
	XMLName    xml.Name `xml:"Message"`
	XMLNS      string   `xml:"xmlns,attr"`
	Body       string   `xml:"MessageBody"`
	Priority   int      `xml:"Priority,omitempty"`
	DelaySec   int      `xml:"DelaySeconds,omitempty"`
	Attributes string   `xml:"MessageAttributes,omitempty"`
}

// errMNSTransient 5xx / network → 触发重试.
var errMNSTransient = errors.New("mns: transient")

// Publish 发到 queueName；attrs 透传为 user-properties（嵌进 body 头部行；MNS 不支持原生 attrs）.
//
// MNS REST: POST /queues/{queue}/messages
func (p *MNSPublisher) Publish(ctx context.Context, queueName string, body []byte, attrs map[string]string) error {
	if p == nil {
		return errors.New("mns publisher: nil")
	}
	if queueName == "" {
		return errors.New("mns publisher: queue name required")
	}
	envelope := buildMNSEnvelope(body, attrs)
	xmlPayload, err := xml.Marshal(&sendMessageXML{
		XMLNS: "http://mns.aliyuncs.com/doc/v1/",
		Body:  base64.StdEncoding.EncodeToString(envelope),
	})
	if err != nil {
		return fmt.Errorf("mns publisher: marshal: %w", err)
	}
	xmlPayload = append([]byte(xml.Header), xmlPayload...)

	path := fmt.Sprintf("/queues/%s/messages", url.PathEscape(queueName))
	backoff := time.Second
	var lastErr error
	for attempt := 1; attempt <= p.cfg.MaxRetries; attempt++ {
		log.Debug().
			Str("queue", queueName).
			Int("attempt", attempt).
			Int("body_len", len(body)).
			Msg("mns_publish_attempt")
		err := p.doSend(ctx, path, xmlPayload)
		if err == nil {
			log.Info().Str("queue", queueName).Int("attempt", attempt).Msg("mns_publish_success")
			return nil
		}
		lastErr = err
		if !errors.Is(err, errMNSTransient) {
			// 非 transient（4xx / 配置错误）直接返回，不重试
			log.Error().Err(err).Str("queue", queueName).Int("attempt", attempt).Msg("mns_publish_failure")
			return err
		}
		log.Warn().Err(err).Str("queue", queueName).Int("attempt", attempt).Msg("mns_publish_retry")
		// 退避；ctx 取消时立即退出
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return fmt.Errorf("mns publisher: exhausted retries: %w", lastErr)
}

func (p *MNSPublisher) doSend(ctx context.Context, path string, body []byte) error {
	endpoint := strings.TrimRight(p.cfg.Endpoint, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	signMNSRequest(req, body, p.cfg.AccessKeyID, p.cfg.AccessKeySecret, "application/xml")
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", errMNSTransient, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 500 {
		return fmt.Errorf("%w: status=%d body=%s", errMNSTransient, resp.StatusCode, string(respBody))
	}
	return fmt.Errorf("mns: status=%d body=%s", resp.StatusCode, string(respBody))
}

// signMNSRequest 设置 MNS 签名头.
//
// 文档：https://help.aliyun.com/document_detail/27487.html
// Authorization: MNS AccessKeyId:Signature
// StringToSign = HTTPMethod + "\n" + Content-MD5 + "\n" + Content-Type + "\n" + Date + "\n"
//              + CanonicalizedMNSHeaders + CanonicalizedResource
func signMNSRequest(req *http.Request, body []byte, ak, sk, contentType string) {
	date := time.Now().UTC().Format(http.TimeFormat)
	req.Header.Set("Date", date)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-mns-version", "2015-06-06")

	canonicalHeaders := canonicalizeMNSHeaders(req.Header)
	canonicalResource := req.URL.Path
	if req.URL.RawQuery != "" {
		canonicalResource += "?" + req.URL.RawQuery
	}
	stringToSign := req.Method + "\n" +
		req.Header.Get("Content-MD5") + "\n" +
		contentType + "\n" +
		date + "\n" +
		canonicalHeaders +
		canonicalResource
	mac := hmac.New(sha1.New, []byte(sk))
	_, _ = mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	req.Header.Set("Authorization", "MNS "+ak+":"+sig)
}

func canonicalizeMNSHeaders(h http.Header) string {
	keys := make([]string, 0, len(h))
	for k := range h {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-mns-") {
			keys = append(keys, lk)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString(":")
		b.WriteString(strings.TrimSpace(h.Get(k)))
		b.WriteString("\n")
	}
	return b.String()
}

// buildMNSEnvelope 把 attrs (event_type / trace_id / dequeue_count) prepend 进 body.
//
// 简化设计：行首 N 行 "k=v"，空行结束；然后是原始 body 字节。
// 消费端 parseMNSEnvelope 反解。
func buildMNSEnvelope(body []byte, attrs map[string]string) []byte {
	if len(attrs) == 0 {
		return body
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b bytes.Buffer
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(attrs[k])
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.Write(body)
	return b.Bytes()
}

// parseMNSEnvelope 拆 attrs + body.
func parseMNSEnvelope(env []byte) (map[string]string, []byte) {
	attrs := map[string]string{}
	// 找空行分隔
	for i := 0; i < len(env); i++ {
		if env[i] != '\n' {
			continue
		}
		if i+1 < len(env) && env[i+1] == '\n' {
			head := string(env[:i])
			for _, line := range strings.Split(head, "\n") {
				if eq := strings.IndexByte(line, '='); eq > 0 {
					attrs[line[:eq]] = line[eq+1:]
				}
			}
			return attrs, env[i+2:]
		}
		// 第一段就没 = 说明不是 envelope
		if !strings.Contains(string(env[:i]), "=") {
			return attrs, env
		}
	}
	return attrs, env
}
