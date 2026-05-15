package outbox

import (
	"context"
	"testing"
	"time"
)

type fakeLocalRepo struct {
	rows   []LocalRow
	sent   []int64
	failed map[int64]string
}

func (f *fakeLocalRepo) ClaimPending(_ context.Context, n int) ([]LocalRow, error) {
	if len(f.rows) > n {
		return f.rows[:n], nil
	}
	return f.rows, nil
}

func (f *fakeLocalRepo) MarkSent(_ context.Context, id int64) error {
	f.sent = append(f.sent, id)
	return nil
}

func (f *fakeLocalRepo) MarkFailed(_ context.Context, id int64, errText string) error {
	if f.failed == nil {
		f.failed = map[int64]string{}
	}
	f.failed[id] = errText
	return nil
}

type fakePublisher struct {
	calls []map[string]string
}

func (f *fakePublisher) Publish(_ context.Context, _ string, _ []byte, attrs map[string]string) error {
	cp := map[string]string{}
	for k, v := range attrs {
		cp[k] = v
	}
	f.calls = append(f.calls, cp)
	return nil
}

func TestPublishPendingOnceStampsRegionAndMarksSent(t *testing.T) {
	repo := &fakeLocalRepo{rows: []LocalRow{
		{ID: 1, EventType: "a", Body: []byte("1"), TraceID: "t1", CreatedAt: time.Now()},
		{ID: 2, EventType: "b", Body: []byte("2"), TraceID: "t2", CreatedAt: time.Now()},
		{ID: 3, EventType: "c", Body: []byte("3"), TraceID: "t3", CreatedAt: time.Now()},
	}}
	pub := &fakePublisher{}
	sent, err := PublishPendingOnce(context.Background(), repo, pub, "q", "cn", 10)
	if err != nil {
		t.Fatalf("PublishPendingOnce: %v", err)
	}
	if sent != 3 || len(repo.sent) != 3 || len(pub.calls) != 3 {
		t.Fatalf("sent=%d repo.sent=%v calls=%d", sent, repo.sent, len(pub.calls))
	}
	for _, attrs := range pub.calls {
		if attrs["data_region"] != "cn" {
			t.Fatalf("data_region not stamped: %v", attrs)
		}
		if attrs["event_type"] == "" {
			t.Fatalf("event_type missing: %v", attrs)
		}
	}
}
