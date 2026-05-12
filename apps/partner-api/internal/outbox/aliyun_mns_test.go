// internal/outbox/aliyun_mns_test.go — MNS publisher/consumer 单测（fake driven）.
package outbox

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- fake MNSClient ----

type fakeMNSClient struct {
	mu       sync.Mutex
	queue    []*MNSMessage
	deleted  []string
	dlq      []*MNSMessage
	recvErr  error
	delErr   error
	dlqErr   error
	recvHits int
}

func (f *fakeMNSClient) push(msg *MNSMessage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue = append(f.queue, msg)
}

func (f *fakeMNSClient) ReceiveMessage(_ context.Context, _ string, _ int) (*MNSMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recvHits++
	if f.recvErr != nil {
		return nil, f.recvErr
	}
	if len(f.queue) == 0 {
		return nil, nil
	}
	m := f.queue[0]
	f.queue = f.queue[1:]
	return m, nil
}

func (f *fakeMNSClient) DeleteMessage(_ context.Context, _, receipt string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.delErr != nil {
		return f.delErr
	}
	f.deleted = append(f.deleted, receipt)
	return nil
}

func (f *fakeMNSClient) MoveToDLQ(_ context.Context, _, _ string, msg *MNSMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dlqErr != nil {
		return f.dlqErr
	}
	f.dlq = append(f.dlq, msg)
	return nil
}

func makeMsg(id string, attrs map[string]string, body string, dequeue int) *MNSMessage {
	cp := map[string]string{}
	for k, v := range attrs {
		cp[k] = v
	}
	return &MNSMessage{
		MessageID:     id,
		ReceiptHandle: "rh-" + id,
		Body:          []byte(body),
		Attrs:         cp,
		DequeueCount:  dequeue,
	}
}

// ---- consumer tests ----

func TestMNSConsumer_HappyPath(t *testing.T) {
	t.Parallel()
	fc := &fakeMNSClient{}
	c, err := NewMNSConsumer(fc, MNSConsumerOptions{QueueName: "q-in", DLQName: "q-dlq", WaitSeconds: 1, DLQThreshold: 5})
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan *MNSMessage, 1)
	c.Register("partner.created", func(_ context.Context, m *MNSMessage) error {
		got <- m
		return nil
	})
	fc.push(makeMsg("m1", map[string]string{"event_type": "partner.created"}, "hello", 1))
	if err := c.handleOne(context.Background(), <-pop(fc)); err != nil {
		t.Fatalf("handleOne: %v", err)
	}
	select {
	case m := <-got:
		if string(m.Body) != "hello" {
			t.Fatalf("body=%q", m.Body)
		}
	case <-time.After(time.Second):
		t.Fatal("handler not called")
	}
	if len(fc.deleted) != 1 || fc.deleted[0] != "rh-m1" {
		t.Fatalf("deleted=%v", fc.deleted)
	}
}

func TestMNSConsumer_HandlerFailLeavesMessage(t *testing.T) {
	t.Parallel()
	fc := &fakeMNSClient{}
	c, _ := NewMNSConsumer(fc, MNSConsumerOptions{QueueName: "q-in", DLQName: "q-dlq", WaitSeconds: 1, DLQThreshold: 5})
	c.Register("partner.created", func(_ context.Context, _ *MNSMessage) error {
		return errors.New("downstream busy")
	})
	msg := makeMsg("m1", map[string]string{"event_type": "partner.created"}, "x", 1)
	if err := c.handleOne(context.Background(), msg); err != nil {
		t.Fatalf("handleOne: %v", err)
	}
	if len(fc.deleted) != 0 {
		t.Fatalf("should NOT delete on handler failure; deleted=%v", fc.deleted)
	}
	if len(fc.dlq) != 0 {
		t.Fatalf("should NOT DLQ; dlq=%d", len(fc.dlq))
	}
}

func TestMNSConsumer_FailThenSuccessRedeliver(t *testing.T) {
	t.Parallel()
	fc := &fakeMNSClient{}
	c, _ := NewMNSConsumer(fc, MNSConsumerOptions{QueueName: "q-in", DLQName: "q-dlq", WaitSeconds: 1, DLQThreshold: 5})
	calls := 0
	c.Register("evt", func(_ context.Context, _ *MNSMessage) error {
		calls++
		if calls < 2 {
			return errors.New("transient")
		}
		return nil
	})
	msg := makeMsg("m1", map[string]string{"event_type": "evt"}, "x", 1)
	// first attempt: leave
	_ = c.handleOne(context.Background(), msg)
	// MNS redeliver (dequeueCount bumps)
	msg.DequeueCount = 2
	_ = c.handleOne(context.Background(), msg)
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
	if len(fc.deleted) != 1 {
		t.Fatalf("deleted=%v", fc.deleted)
	}
}

func TestMNSConsumer_DLQAfterThreshold(t *testing.T) {
	t.Parallel()
	fc := &fakeMNSClient{}
	c, _ := NewMNSConsumer(fc, MNSConsumerOptions{QueueName: "q-in", DLQName: "q-dlq", WaitSeconds: 1, DLQThreshold: 3})
	c.Register("evt", func(_ context.Context, _ *MNSMessage) error { return errors.New("nope") })
	msg := makeMsg("m1", map[string]string{"event_type": "evt"}, "x", 3) // == threshold
	if err := c.handleOne(context.Background(), msg); err != nil {
		t.Fatalf("handleOne: %v", err)
	}
	if len(fc.dlq) != 1 {
		t.Fatalf("dlq=%d (expected 1)", len(fc.dlq))
	}
	if len(fc.deleted) != 1 {
		t.Fatalf("should delete after DLQ move; deleted=%v", fc.deleted)
	}
}

func TestMNSConsumer_EmptyQueueNoop(t *testing.T) {
	t.Parallel()
	fc := &fakeMNSClient{}
	c, _ := NewMNSConsumer(fc, MNSConsumerOptions{QueueName: "q-in", DLQName: "q-dlq", WaitSeconds: 1})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_ = c.Run(ctx)
	if len(fc.deleted) != 0 || len(fc.dlq) != 0 {
		t.Fatalf("expected no-op; deleted=%d dlq=%d", len(fc.deleted), len(fc.dlq))
	}
	if fc.recvHits < 1 {
		t.Fatalf("should call receive at least once; hits=%d", fc.recvHits)
	}
}

func TestMNSConsumer_UnknownEventTypeNoopAck(t *testing.T) {
	t.Parallel()
	fc := &fakeMNSClient{}
	c, _ := NewMNSConsumer(fc, MNSConsumerOptions{QueueName: "q-in", DLQName: "q-dlq", WaitSeconds: 1, NoopOnUnknown: true})
	msg := makeMsg("m1", map[string]string{"event_type": "totally-unknown"}, "x", 1)
	if err := c.handleOne(context.Background(), msg); err != nil {
		t.Fatalf("handleOne: %v", err)
	}
	if len(fc.deleted) != 1 {
		t.Fatalf("expected noop ack on unknown; deleted=%v", fc.deleted)
	}
}

// pop adapts fc.queue head into a one-shot channel for cleaner test syntax.
func pop(fc *fakeMNSClient) <-chan *MNSMessage {
	ch := make(chan *MNSMessage, 1)
	m, _ := fc.ReceiveMessage(context.Background(), "", 0)
	ch <- m
	close(ch)
	return ch
}

// ---- publisher tests ----

func TestMNSPublisher_HappyPath(t *testing.T) {
	t.Parallel()
	var got struct {
		auth   string
		path   string
		body   []byte
		method string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.auth = r.Header.Get("Authorization")
		got.path = r.URL.Path
		got.method = r.Method
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		got.body = b
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	pub, err := NewMNSPublisher(MNSConfig{
		Endpoint:        srv.URL,
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
		Timeout:         time.Second,
		MaxRetries:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := pub.Publish(context.Background(), "myqueue", []byte("payload-x"), map[string]string{"event_type": "partner.created"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got.method != http.MethodPost {
		t.Fatalf("method=%s", got.method)
	}
	if !strings.Contains(got.path, "myqueue") {
		t.Fatalf("path=%s", got.path)
	}
	if !strings.HasPrefix(got.auth, "MNS ak:") {
		t.Fatalf("auth=%s", got.auth)
	}
}

func TestMNSPublisher_RetryOnTransient(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	pub, _ := NewMNSPublisher(MNSConfig{
		Endpoint: srv.URL, AccessKeyID: "ak", AccessKeySecret: "sk",
		Timeout: time.Second, MaxRetries: 5,
	})
	// shrink backoff for test: hack — set MaxRetries=5 with default backoff means up to 1+2+4+8s; reduce by ctx
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := pub.Publish(ctx, "q", []byte("x"), nil); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls=%d (expected 3)", calls)
	}
}

func TestMNSPublisher_NonTransient4xxNoRetry(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	pub, _ := NewMNSPublisher(MNSConfig{
		Endpoint: srv.URL, AccessKeyID: "ak", AccessKeySecret: "sk",
		Timeout: time.Second, MaxRetries: 5,
	})
	if err := pub.Publish(context.Background(), "q", []byte("x"), nil); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("calls=%d (expected 1; 4xx must not retry)", calls)
	}
}

func TestMNSEnvelopeRoundTrip(t *testing.T) {
	t.Parallel()
	body := []byte("payload-bytes-with\nnewlines")
	attrs := map[string]string{"event_type": "p.created", "trace_id": "tr-1"}
	env := buildMNSEnvelope(body, attrs)
	gotAttrs, gotBody := parseMNSEnvelope(env)
	if string(gotBody) != string(body) {
		t.Fatalf("body mismatch: %q", gotBody)
	}
	if gotAttrs["event_type"] != "p.created" || gotAttrs["trace_id"] != "tr-1" {
		t.Fatalf("attrs=%v", gotAttrs)
	}
}
