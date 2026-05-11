// internal/outbox/consumer.go — consume_log_outbox 消费者（W1b 完整化）.
//
// 设计参考：integration §3 / backend §5.4 / Fy-api OVERLAY-TNBIZ-HANDOFF.md §6 region 隔离。
//
// 拓扑：
//
//	[Fy-api LOG_DB.consume_log_outbox]  (region=cn|sg)
//	          │ pull (FOR UPDATE SKIP LOCKED, cap=batch)
//	          ▼
//	[partner-api consumer]  ── claim → process → ack
//	          │ writes
//	          ▼
//	[partner_db.revenue_log]  (UNIQUE fy_api_log_id+occurrence)
//
// 关键 invariant：
//
//  1. region 隔离：consumer 启动时绑定 dataRegion（cn / sg）；
//     pull 子句强制 `data_region = ?`，CN 不能消费 SG 事件（反之亦然）.
//  2. 幂等 by (fy_api_log_id, occurrence) UNIQUE — 重复消费返回成功（无新插入即视为已写过）.
//  3. trace_id 端到端：outbox.trace_id → revenue_log.trace_id 透传，便于审计.
//  4. retry exponential backoff：retry_count 由 Fy-api 侧维护；本端只负责 ack（消费成功）/ nack（失败 + 写 last_error）.
//  5. retry_count >= DLQThreshold → status=dead_letter；ops 介入.
//  6. PII scrub：last_error 写回前必经 scrubber（Security MED-6）.
package outbox

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// 错误.
var (
	ErrRegionMismatch    = errors.New("outbox: region mismatch")
	ErrLagBeyondGate     = errors.New("outbox: freshness gate exceeded")
	ErrNoSource          = errors.New("outbox: source not configured")
)

// 重要常量.
const (
	DefaultBatchSize     = 50
	DefaultPollInterval  = 1 * time.Second
	DefaultLeaseTTL      = 30 * time.Second
	DLQThreshold         = 10
	FreshnessGateDefault = 5 * time.Minute
	MaxLastErrorLen      = 4000
)

// Event 单条 outbox 事件（去掉 Fy-api 侧字段名差异，这里用业务语义命名）.
type Event struct {
	OutboxID    int64     // fy_api_db.consume_log_outbox.id
	DataRegion  string    // cn / sg
	FyLogID     int64     // 关联 fy_api_db.logs.id
	Occurrence  int8      // 1=正常 2+=显式调整
	UserID      int64     // fy_api_db.user.id（→ tnbiz partner/customer）
	GrossAmount int64     // 计费总额（quota）
	CostAmount  int64     // 上游成本
	OccurredAt  time.Time // 业务发生时间
	TraceID     string    // 端到端 trace
	RetryCount  int
	RawPayload  []byte // 原始 JSON（debug 备份；不入 revenue_log）
}

// Source 抽象 LOG_DB outbox 拉取。
//
// 实现位于 internal/outbox/source_log_db.go（W1a 落 GORM）；测试用 stub。
type Source interface {
	// Pull 两阶段第 1 步：claim N 条 status=pending 行 → in_flight。
	// 必须在 LOG_DB 短 TX 内 SELECT ... FOR UPDATE SKIP LOCKED。
	Pull(ctx context.Context, region string, batch int) ([]Event, error)
	// Ack 写回 status=consumed + consumed_at=NOW()。
	Ack(ctx context.Context, outboxID int64) error
	// Nack 写回 status=pending + retry_count++ + last_error；超阈值切 dead_letter。
	Nack(ctx context.Context, outboxID int64, lastError string, dlq bool) error
	// Lag 当前 region 最老 pending 事件的 NOW()-occurred_at；用于 freshness gate。
	Lag(ctx context.Context, region string) (time.Duration, error)
}

// Sink 写入 partner_db.revenue_log（幂等）.
type Sink interface {
	// WriteRevenue 写 revenue_log；UNIQUE 冲突视为幂等成功（return false, nil）.
	WriteRevenue(ctx context.Context, ev Event) (inserted bool, err error)
}

// Scrubber PII 脱敏接口（W1a 提供）.
type Scrubber interface {
	Scrub(s string) string
}

// Metrics 指标埋点接口（W1a 注入 prometheus 实现）.
type Metrics interface {
	IncConsumed(region string)
	IncDLQ(region string)
	IncFailed(region string)
	ObserveLag(region string, lag time.Duration)
}

// noopMetrics 默认实现.
type noopMetrics struct{}

func (noopMetrics) IncConsumed(string)              {}
func (noopMetrics) IncDLQ(string)                   {}
func (noopMetrics) IncFailed(string)                {}
func (noopMetrics) ObserveLag(string, time.Duration) {}

// Consumer 主 loop.
type Consumer struct {
	source     Source
	sink       Sink
	scrubber   Scrubber
	metrics    Metrics
	region     string
	batch      int
	interval   time.Duration
	gate       time.Duration
	dlqThresh  int
}

// Options consumer 构造参数（functional options 风格暴露给 main.go）.
type Options struct {
	Region          string
	BatchSize       int
	PollInterval    time.Duration
	FreshnessGate   time.Duration
	DLQThreshold    int
	Scrubber        Scrubber
	Metrics         Metrics
}

// NewConsumer 构造；region 必须 cn / sg.
func NewConsumer(source Source, sink Sink, opts Options) (*Consumer, error) {
	if source == nil || sink == nil {
		return nil, ErrNoSource
	}
	if opts.Region != "cn" && opts.Region != "sg" {
		return nil, fmt.Errorf("outbox: invalid region %q (cn / sg)", opts.Region)
	}
	c := &Consumer{
		source:    source,
		sink:      sink,
		scrubber:  opts.Scrubber,
		metrics:   opts.Metrics,
		region:    opts.Region,
		batch:     opts.BatchSize,
		interval:  opts.PollInterval,
		gate:      opts.FreshnessGate,
		dlqThresh: opts.DLQThreshold,
	}
	if c.batch <= 0 {
		c.batch = DefaultBatchSize
	}
	if c.interval <= 0 {
		c.interval = DefaultPollInterval
	}
	if c.gate <= 0 {
		c.gate = FreshnessGateDefault
	}
	if c.dlqThresh <= 0 {
		c.dlqThresh = DLQThreshold
	}
	if c.metrics == nil {
		c.metrics = noopMetrics{}
	}
	return c, nil
}

// Run 启动 ticker；ctx.Done 后 graceful drain.
func (c *Consumer) Run(ctx context.Context) error {
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if _, err := c.TickOnce(ctx); err != nil {
				// log 由调用方 wrap zerolog；这里不直接 print
				continue
			}
		}
	}
}

// TickResult 单次 tick 的结果（test-friendly）.
type TickResult struct {
	Pulled    int
	Acked     int
	Failed    int
	DLQ       int
}

// TickOnce 一次 pull → process → ack 循环.
func (c *Consumer) TickOnce(ctx context.Context) (TickResult, error) {
	var res TickResult
	events, err := c.source.Pull(ctx, c.region, c.batch)
	if err != nil {
		return res, fmt.Errorf("outbox: pull: %w", err)
	}
	res.Pulled = len(events)
	for _, ev := range events {
		if ev.DataRegion != c.region {
			// region 隔离 invariant —— 永远不应发生；触发 → 立即 nack to DLQ
			c.metrics.IncDLQ(c.region)
			_ = c.source.Nack(ctx, ev.OutboxID, c.scrub("region mismatch"), true)
			res.DLQ++
			continue
		}
		if err := c.processOne(ctx, ev); err != nil {
			c.metrics.IncFailed(c.region)
			dlq := ev.RetryCount+1 >= c.dlqThresh
			if dlq {
				res.DLQ++
				c.metrics.IncDLQ(c.region)
			} else {
				res.Failed++
			}
			_ = c.source.Nack(ctx, ev.OutboxID, c.scrub(err.Error()), dlq)
			continue
		}
		if err := c.source.Ack(ctx, ev.OutboxID); err != nil {
			res.Failed++
			c.metrics.IncFailed(c.region)
			continue
		}
		c.metrics.IncConsumed(c.region)
		res.Acked++
	}
	return res, nil
}

// processOne 单事件处理：写 revenue_log（幂等）.
func (c *Consumer) processOne(ctx context.Context, ev Event) error {
	if !validEvent(ev) {
		return fmt.Errorf("outbox: malformed event id=%d", ev.OutboxID)
	}
	_, err := c.sink.WriteRevenue(ctx, ev)
	if err != nil {
		return fmt.Errorf("outbox: write revenue: %w", err)
	}
	return nil
}

// FreshnessGate 用于结算前预检：T+1 settlement 启动时调，超阈值 → 阻塞.
//
// 返回 (lag, isFresh, err)；调用方 isFresh==false → 不要进入结算 lock 阶段（参 backend §9.3）.
func (c *Consumer) FreshnessGate(ctx context.Context) (time.Duration, bool, error) {
	lag, err := c.source.Lag(ctx, c.region)
	if err != nil {
		return 0, false, fmt.Errorf("outbox: lag query: %w", err)
	}
	c.metrics.ObserveLag(c.region, lag)
	if lag > c.gate {
		return lag, false, ErrLagBeyondGate
	}
	return lag, true, nil
}

func validEvent(ev Event) bool {
	if ev.FyLogID <= 0 || ev.UserID <= 0 || ev.Occurrence <= 0 || ev.OccurredAt.IsZero() {
		return false
	}
	if ev.DataRegion != "cn" && ev.DataRegion != "sg" {
		return false
	}
	return true
}

func (c *Consumer) scrub(s string) string {
	if c.scrubber == nil {
		return truncate(s, MaxLastErrorLen)
	}
	return truncate(c.scrubber.Scrub(s), MaxLastErrorLen)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
