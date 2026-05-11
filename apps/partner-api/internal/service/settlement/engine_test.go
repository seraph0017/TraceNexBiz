package settlement

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

func TestComputeItem_CorporateNoTax(t *testing.T) {
	got := ComputeItem(Aggregate{
		PartnerID: 1, PartnerKind: PartnerKindCorporate,
		RevenueTotal: 10000, CostTotal: 4000, Adjustments: 500,
	})
	if got.Revenue != 10500 || got.Cost != 4000 || got.WithheldTax != 0 || got.Payout != 6500 {
		t.Fatalf("%+v", got)
	}
}

func TestComputeItem_PersonalWithholding(t *testing.T) {
	got := ComputeItem(Aggregate{
		PartnerID: 1, PartnerKind: PartnerKindPersonal,
		RevenueTotal: 10000, CostTotal: 0,
	})
	// net = 10000；tax = 20%；payout = 8000
	if got.WithheldTax != 2000 || got.Payout != 8000 {
		t.Fatalf("%+v", got)
	}
}

func TestComputeItem_NegativeNetClampedToZero(t *testing.T) {
	got := ComputeItem(Aggregate{
		PartnerID: 1, PartnerKind: PartnerKindCorporate,
		RevenueTotal: 100, CostTotal: 500,
	})
	if got.Payout != 0 || got.Revenue != 100 {
		t.Fatalf("%+v", got)
	}
}

func TestStateMachine_Transitions(t *testing.T) {
	cases := []struct {
		from, to Status
		ok       bool
	}{
		{StatusDraft, StatusGenerated, true},
		{StatusGenerated, StatusLocked, true},
		{StatusLocked, StatusPaying, true},
		{StatusPaying, StatusPaid, true},
		{StatusPaid, StatusPartialDispute, true},
		{StatusPaid, StatusDraft, false},
		{StatusLocked, StatusDraft, false},
		{StatusGenerated, StatusGateFailed, true},
	}
	for _, c := range cases {
		if got := CanTransition(c.from, c.to); got != c.ok {
			t.Errorf("%s→%s: got %v want %v", c.from, c.to, got, c.ok)
		}
	}
}

func TestAssertPayable(t *testing.T) {
	if err := AssertPayable(domain.SettlementItem{Status: "pending"}, ""); !errors.Is(err, ErrEmptyPayoutEvidence) {
		t.Fatalf("evidence: %v", err)
	}
	if err := AssertPayable(domain.SettlementItem{Status: "paid"}, "ok"); err == nil {
		t.Fatal("expected non-pending error")
	}
	if err := AssertPayable(domain.SettlementItem{Status: "pending"}, "ok"); err != nil {
		t.Fatalf("happy: %v", err)
	}
}

func TestAssertLockable_GateFails(t *testing.T) {
	s := domain.Settlement{Status: string(StatusGenerated)}
	if err := AssertLockable(s, false); !errors.Is(err, ErrFreshnessGateFailed) {
		t.Fatal(err)
	}
}

func TestAssertLockable_AlreadyLocked(t *testing.T) {
	s := domain.Settlement{Status: string(StatusLocked)}
	if err := AssertLockable(s, true); !errors.Is(err, ErrAlreadyLocked) {
		t.Fatal(err)
	}
}

// in-mem runner test
type fakeLoader struct {
	ids   []int64
	calls int
}

func (l *fakeLoader) ListPartnerIDs(_ context.Context, _ Period, batch, offset int) ([]int64, error) {
	if offset >= len(l.ids) {
		return nil, nil
	}
	end := offset + batch
	if end > len(l.ids) {
		end = len(l.ids)
	}
	return l.ids[offset:end], nil
}

func (l *fakeLoader) Load(_ context.Context, pid int64, _ Period) (Aggregate, error) {
	l.calls++
	return Aggregate{PartnerID: pid, PartnerKind: PartnerKindCorporate, RevenueTotal: 1000, CostTotal: 400}, nil
}

type fakeWriter struct {
	mu    sync.Mutex
	items []domain.SettlementItem
	state domain.Settlement
}

func (w *fakeWriter) UpsertSettlement(_ context.Context, s domain.Settlement) (domain.Settlement, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.state = s
	return s, nil
}

func (w *fakeWriter) UpsertItem(_ context.Context, item domain.SettlementItem) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.items = append(w.items, item)
	return nil
}

type fakeGate struct {
	fresh bool
	err   error
}

func (g *fakeGate) FreshnessGate(_ context.Context) (time.Duration, bool, error) {
	return time.Second, g.fresh, g.err
}

func TestRunner_HappyPath_LocksAfterGenerate(t *testing.T) {
	loader := &fakeLoader{ids: []int64{1, 2, 3}}
	writer := &fakeWriter{}
	r := NewRunner(loader, writer, &fakeGate{fresh: true})
	period, _ := NewMonthlyPeriod(2026, time.May, "Asia/Shanghai")
	out, err := r.RunMonthly(context.Background(), period, domain.Settlement{ID: 1, Period: period.Label})
	if err != nil {
		t.Fatalf("RunMonthly: %v", err)
	}
	if Status(out.Status) != StatusLocked {
		t.Fatalf("status=%s", out.Status)
	}
	if len(writer.items) != 3 {
		t.Fatalf("items=%d", len(writer.items))
	}
}

func TestRunner_GateFails_TransitionsToGateFailed(t *testing.T) {
	loader := &fakeLoader{ids: []int64{1}}
	writer := &fakeWriter{}
	r := NewRunner(loader, writer, &fakeGate{fresh: false, err: errors.New("lag")})
	period, _ := NewMonthlyPeriod(2026, time.May, "Asia/Shanghai")
	out, err := r.RunMonthly(context.Background(), period, domain.Settlement{ID: 2, Period: period.Label})
	if err == nil {
		t.Fatal("expected gate err")
	}
	if Status(out.Status) != StatusGateFailed {
		t.Fatalf("status=%s", out.Status)
	}
}

func TestNewMonthlyPeriod_BadTimezone(t *testing.T) {
	if _, err := NewMonthlyPeriod(2026, time.May, "Mars/Olympus"); !errors.Is(err, ErrPeriodInvalid) {
		t.Fatal(err)
	}
}
