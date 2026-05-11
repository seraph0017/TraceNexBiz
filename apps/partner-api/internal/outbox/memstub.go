// internal/outbox/memstub.go — 内存 stub Source / Sink，用于单测 + dev 启动 smoke.
package outbox

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemSource 内存实现：保存若干 Event；Pull 拉取后切到 in_flight；Ack 删除；Nack 写错误.
type MemSource struct {
	mu        sync.Mutex
	pending   []Event
	inFlight  map[int64]Event
	consumed  []int64
	dlq       []int64
	failures  map[int64]string
	region    string
}

// NewMemSource 构造.
func NewMemSource(region string) *MemSource {
	return &MemSource{
		inFlight: make(map[int64]Event),
		failures: make(map[int64]string),
		region:   region,
	}
}

// Seed 注入测试事件.
func (s *MemSource) Seed(events ...Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, events...)
	sort.Slice(s.pending, func(i, j int) bool { return s.pending[i].OutboxID < s.pending[j].OutboxID })
}

// Pull 拉前 N 条 pending 切到 in_flight.
func (s *MemSource) Pull(_ context.Context, region string, batch int) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Event, 0, batch)
	keep := make([]Event, 0, len(s.pending))
	for _, e := range s.pending {
		if e.DataRegion != region {
			keep = append(keep, e)
			continue
		}
		if len(out) >= batch {
			keep = append(keep, e)
			continue
		}
		s.inFlight[e.OutboxID] = e
		out = append(out, e)
	}
	s.pending = keep
	return out, nil
}

// Ack 标记 consumed.
func (s *MemSource) Ack(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inFlight, id)
	s.consumed = append(s.consumed, id)
	return nil
}

// Nack 失败处理.
func (s *MemSource) Nack(_ context.Context, id int64, lastError string, dlq bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev, ok := s.inFlight[id]
	if !ok {
		return nil
	}
	delete(s.inFlight, id)
	s.failures[id] = lastError
	if dlq {
		s.dlq = append(s.dlq, id)
		return nil
	}
	ev.RetryCount++
	s.pending = append(s.pending, ev)
	return nil
}

// Lag 返回最老 pending 的滞后；测试可注入.
func (s *MemSource) Lag(_ context.Context, region string) (time.Duration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	var oldest *time.Time
	for _, e := range s.pending {
		if e.DataRegion != region {
			continue
		}
		t := e.OccurredAt
		if oldest == nil || t.Before(*oldest) {
			oldest = &t
		}
	}
	if oldest == nil {
		return 0, nil
	}
	return now.Sub(*oldest), nil
}

// Consumed 测试 inspector.
func (s *MemSource) Consumed() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]int64(nil), s.consumed...)
	return out
}

// DLQ 测试 inspector.
func (s *MemSource) DLQ() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int64(nil), s.dlq...)
}

// MemSink 写入收集器；UNIQUE by (FyLogID, Occurrence)。
type MemSink struct {
	mu      sync.Mutex
	written map[string]Event
	failOn  map[int64]error
}

// NewMemSink 构造.
func NewMemSink() *MemSink {
	return &MemSink{written: make(map[string]Event), failOn: make(map[int64]error)}
}

// FailFor 注入错误.
func (s *MemSink) FailFor(outboxID int64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failOn[outboxID] = err
}

// WriteRevenue UPSERT；UNIQUE 冲突 → false, nil.
func (s *MemSink) WriteRevenue(_ context.Context, ev Event) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.failOn[ev.OutboxID]; ok {
		return false, e
	}
	key := keyFor(ev.FyLogID, ev.Occurrence)
	if _, dup := s.written[key]; dup {
		return false, nil
	}
	s.written[key] = ev
	return true, nil
}

// Written 测试 inspector.
func (s *MemSink) Written() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Event, 0, len(s.written))
	for _, v := range s.written {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FyLogID < out[j].FyLogID })
	return out
}

func keyFor(logID int64, occ int8) string {
	return string(rune(occ)) + ":" + itoa(logID)
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	buf := make([]byte, 0, 20)
	for v > 0 {
		buf = append([]byte{byte('0' + v%10)}, buf...)
		v /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
