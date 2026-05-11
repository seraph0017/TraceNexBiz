package allocate

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/saga"
)

type fakeWallet struct {
	mu    sync.Mutex
	calls []string
	fail  map[string]error
}

func (w *fakeWallet) record(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls = append(w.calls, name)
	if e, ok := w.fail[name]; ok {
		return e
	}
	return nil
}

func (w *fakeWallet) Deduct(_ context.Context, _, _ int64, _ string) error {
	return w.record("deduct")
}
func (w *fakeWallet) Hold(_ context.Context, _, _ int64, _ string) error {
	return w.record("hold")
}
func (w *fakeWallet) CommitHold(_ context.Context, _ string) error  { return w.record("commit") }
func (w *fakeWallet) ReleaseHold(_ context.Context, _ string) error { return w.record("release") }
func (w *fakeWallet) Refund(_ context.Context, _, _ int64, _ string) error {
	return w.record("refund")
}
func (w *fakeWallet) WriteLog(_ context.Context, _ Request) error { return w.record("log") }

type fakeFyAPI struct {
	mu     sync.Mutex
	calls  []string
	failed bool
}

func (f *fakeFyAPI) TopupCustomer(_ context.Context, _, _ int64, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "topup")
	if f.failed {
		return errors.New("fyapi 5xx")
	}
	return nil
}
func (f *fakeFyAPI) RefundCustomer(_ context.Context, _, _ int64, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "refund")
	return nil
}

type fakeLookup struct{ fyID int64 }

func (l fakeLookup) FyUserID(_ context.Context, _, _ int64) (int64, error) { return l.fyID, nil }

type fakeTx struct{}

func (fakeTx) WithTx(_ context.Context, fn saga.TxFn) (any, error) { return fn(nil) }

func newSvc(t *testing.T, w *fakeWallet, f *fakeFyAPI) (*Service, saga.Orchestrator) {
	t.Helper()
	repo := saga.NewMemRepo()
	o := saga.NewOrchestrator(repo, fakeTx{}, nil)
	return NewService(o, w, f, fakeLookup{fyID: 100}), o
}

func TestAllocate_HappyPath(t *testing.T) {
	w := &fakeWallet{}
	f := &fakeFyAPI{}
	svc, _ := newSvc(t, w, f)
	req := Request{
		SagaID: saga.MustNewSagaID(), PartnerID: 1, CustomerID: 2, Amount: 5000, OperatorID: 9,
	}
	if err := svc.Run(context.Background(), req); err != nil {
		t.Fatalf("Run: %v", err)
	}
	expected := []string{"deduct", "hold", "commit", "log"}
	for i, c := range expected {
		if i >= len(w.calls) || w.calls[i] != c {
			t.Fatalf("calls=%v expected=%v", w.calls, expected)
		}
	}
}

func TestAllocate_FyAPIFailureCompensates(t *testing.T) {
	w := &fakeWallet{}
	f := &fakeFyAPI{failed: true}
	svc, _ := newSvc(t, w, f)
	req := Request{SagaID: saga.MustNewSagaID(), PartnerID: 1, CustomerID: 2, Amount: 5000, OperatorID: 9}
	if err := svc.Run(context.Background(), req); err == nil {
		t.Fatal("expected error")
	}
	// 必须包含 release + refund 补偿
	gotRelease, gotRefund := false, false
	for _, c := range w.calls {
		if c == "release" {
			gotRelease = true
		}
		if c == "refund" {
			gotRefund = true
		}
	}
	if !gotRelease || !gotRefund {
		t.Fatalf("expected compensation, calls=%v", w.calls)
	}
}

func TestAllocate_ValidatesInput(t *testing.T) {
	svc, _ := newSvc(t, &fakeWallet{}, &fakeFyAPI{})
	if err := svc.Run(context.Background(), Request{SagaID: "x"}); err == nil {
		t.Fatal("expected validation error")
	}
	if err := svc.Run(context.Background(), Request{SagaID: saga.MustNewSagaID(), PartnerID: 1, CustomerID: 2, OperatorID: 1, Amount: -1}); !errors.Is(err, ErrAmountInvalid) {
		t.Fatal(err)
	}
}

func TestAllocate_OverdraftStopsBeforeHold(t *testing.T) {
	w := &fakeWallet{fail: map[string]error{"deduct": ErrInsufficientBalance}}
	svc, _ := newSvc(t, w, &fakeFyAPI{})
	req := Request{SagaID: saga.MustNewSagaID(), PartnerID: 1, CustomerID: 2, Amount: 5000, OperatorID: 9}
	if err := svc.Run(context.Background(), req); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("expected insufficient balance got %v", err)
	}
	if len(w.calls) != 1 || w.calls[0] != "deduct" {
		t.Fatalf("overdraft must stop at deduct, calls=%v", w.calls)
	}
}

func TestAllocate_CompensationFailureSurfaces(t *testing.T) {
	releaseErr := errors.New("release failed")
	w := &fakeWallet{fail: map[string]error{"release": releaseErr}}
	f := &fakeFyAPI{failed: true}
	svc, _ := newSvc(t, w, f)
	req := Request{SagaID: saga.MustNewSagaID(), PartnerID: 1, CustomerID: 2, Amount: 5000, OperatorID: 9}
	err := svc.Run(context.Background(), req)
	if err == nil || !errors.Is(err, releaseErr) {
		t.Fatalf("expected compensation failure got %v", err)
	}
}
