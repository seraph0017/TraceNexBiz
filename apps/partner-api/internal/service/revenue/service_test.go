package revenue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/outbox"
)

type fakeRepo struct {
	mu      sync.Mutex
	rows    []domain.RevenueLog
	failOn  string
	failErr error
}

func (r *fakeRepo) Insert(_ context.Context, row *domain.RevenueLog) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failOn != "" && row.TraceID == r.failOn {
		return false, r.failErr
	}
	for _, existing := range r.rows {
		if existing.FyAPILogID == row.FyAPILogID && existing.Occurrence == row.Occurrence {
			return false, nil
		}
	}
	cp := *row
	r.rows = append(r.rows, cp)
	return true, nil
}

type fakeResolver struct {
	partner, customer, rule int64
	err                     error
}

func (r *fakeResolver) ResolveByFyUserID(_ context.Context, _ int64, _ time.Time) (int64, int64, int64, error) {
	return r.partner, r.customer, r.rule, r.err
}

func sample(id int64) outbox.Event {
	return outbox.Event{
		OutboxID: id, FyLogID: id, Occurrence: 1, UserID: 9, GrossAmount: 5000, CostAmount: 3000,
		OccurredAt: time.Now().UTC(), TraceID: "tr",
	}
}

func TestWriteRevenue_Happy(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, &fakeResolver{partner: 1, customer: 2, rule: 3})
	inserted, err := svc.WriteRevenue(context.Background(), sample(1))
	if err != nil || !inserted {
		t.Fatalf("err=%v inserted=%v", err, inserted)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("rows=%d", len(repo.rows))
	}
	r := repo.rows[0]
	if r.PartnerID != 1 || r.CustomerID != 2 || r.AppliedRuleID != 3 || r.NetRevenue != 2000 {
		t.Fatalf("row=%+v", r)
	}
}

func TestWriteRevenue_DupReturnsFalse(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, &fakeResolver{partner: 1, customer: 2, rule: 3})
	_, _ = svc.WriteRevenue(context.Background(), sample(1))
	inserted, err := svc.WriteRevenue(context.Background(), sample(1))
	if err != nil || inserted {
		t.Fatalf("expected dedupe; err=%v inserted=%v", err, inserted)
	}
}

func TestWriteRevenue_NegativeNetClampedToZero(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, &fakeResolver{partner: 1, customer: 2, rule: 3})
	ev := sample(1)
	ev.GrossAmount = 100
	ev.CostAmount = 500
	if _, err := svc.WriteRevenue(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if repo.rows[0].NetRevenue != 0 {
		t.Fatalf("net=%d", repo.rows[0].NetRevenue)
	}
}

func TestWriteRevenue_ResolverError(t *testing.T) {
	svc := NewService(&fakeRepo{}, &fakeResolver{err: errors.New("not found")})
	if _, err := svc.WriteRevenue(context.Background(), sample(1)); err == nil {
		t.Fatal("expected error")
	}
}
