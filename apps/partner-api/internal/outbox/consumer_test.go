package outbox

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newConsumer(t *testing.T, region string, gate time.Duration) (*Consumer, *MemSource, *MemSink) {
	t.Helper()
	src := NewMemSource(region)
	sink := NewMemSink()
	c, err := NewConsumer(src, sink, Options{
		Region:        region,
		BatchSize:     10,
		PollInterval:  10 * time.Millisecond,
		FreshnessGate: gate,
		DLQThreshold:  3,
	})
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	return c, src, sink
}

func sampleEvent(id int64, region string) Event {
	return Event{
		OutboxID:    id,
		DataRegion:  region,
		FyLogID:     1000 + id,
		Occurrence:  1,
		UserID:      777,
		GrossAmount: 5000,
		CostAmount:  3000,
		OccurredAt:  time.Now().UTC().Add(-1 * time.Minute),
		TraceID:     "trace-x",
	}
}

func TestConsumer_HappyPath(t *testing.T) {
	c, src, sink := newConsumer(t, "cn", 5*time.Minute)
	src.Seed(sampleEvent(1, "cn"), sampleEvent(2, "cn"))
	res, err := c.TickOnce(context.Background())
	if err != nil {
		t.Fatalf("TickOnce: %v", err)
	}
	if res.Pulled != 2 || res.Acked != 2 || res.Failed != 0 {
		t.Fatalf("res=%+v", res)
	}
	if len(sink.Written()) != 2 {
		t.Fatalf("expected 2 written, got %d", len(sink.Written()))
	}
}

func TestConsumer_RegionIsolation(t *testing.T) {
	c, src, sink := newConsumer(t, "cn", 5*time.Minute)
	src.Seed(sampleEvent(1, "sg")) // 错误 region 注入；MemSource.Pull 已按 region 过滤
	res, _ := c.TickOnce(context.Background())
	if res.Pulled != 0 || res.Acked != 0 || res.DLQ != 0 {
		t.Fatalf("unexpected: %+v", res)
	}
	if len(sink.Written()) != 0 {
		t.Fatal("must not write")
	}
}

func TestConsumer_DLQAfterMaxRetries(t *testing.T) {
	c, src, sink := newConsumer(t, "cn", 5*time.Minute)
	ev := sampleEvent(7, "cn")
	src.Seed(ev)
	sink.FailFor(7, errors.New("downstream 5xx"))
	// retry threshold = 3
	for i := 0; i < 3; i++ {
		_, _ = c.TickOnce(context.Background())
	}
	dlq := src.DLQ()
	if len(dlq) != 1 || dlq[0] != 7 {
		t.Fatalf("expected DLQ=[7], got %v", dlq)
	}
}

func TestConsumer_IdempotentByUniqueKey(t *testing.T) {
	c, src, sink := newConsumer(t, "cn", 5*time.Minute)
	ev := sampleEvent(1, "cn")
	src.Seed(ev)
	_, _ = c.TickOnce(context.Background())
	// 第二次注入同 fy_log_id+occurrence
	dup := sampleEvent(2, "cn")
	dup.FyLogID = ev.FyLogID
	dup.Occurrence = ev.Occurrence
	src.Seed(dup)
	_, _ = c.TickOnce(context.Background())
	if got := len(sink.Written()); got != 1 {
		t.Fatalf("expected dedupe, got %d rows", got)
	}
	// 重复事件应该 ack（幂等成功），不应 DLQ
	if len(src.DLQ()) != 0 {
		t.Fatal("dup must not DLQ")
	}
}

func TestFreshnessGate(t *testing.T) {
	c, src, _ := newConsumer(t, "cn", 100*time.Millisecond)
	old := sampleEvent(1, "cn")
	old.OccurredAt = time.Now().UTC().Add(-1 * time.Hour)
	src.Seed(old)
	lag, fresh, err := c.FreshnessGate(context.Background())
	if !errors.Is(err, ErrLagBeyondGate) {
		t.Fatalf("expected gate error, got %v", err)
	}
	if fresh {
		t.Fatal("expected not fresh")
	}
	if lag <= 100*time.Millisecond {
		t.Fatalf("lag=%v", lag)
	}
}

func TestNewConsumer_RejectsBadRegion(t *testing.T) {
	if _, err := NewConsumer(NewMemSource("us"), NewMemSink(), Options{Region: "us"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidEvent_Malformed(t *testing.T) {
	c, src, _ := newConsumer(t, "cn", 5*time.Minute)
	bad := Event{OutboxID: 1, DataRegion: "cn"} // missing FyLogID etc.
	src.Seed(bad)
	res, _ := c.TickOnce(context.Background())
	if res.Pulled != 1 || res.Failed == 0 && res.DLQ == 0 {
		t.Fatalf("expected failed/dlq, got %+v", res)
	}
}
