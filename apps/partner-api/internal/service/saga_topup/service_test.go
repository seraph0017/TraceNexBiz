package topup

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/saga"
	"gorm.io/gorm"
)

type fakeTx struct{}

func (fakeTx) WithTx(_ context.Context, fn saga.TxFn) (any, error) { return fn(&gorm.DB{}) }

type fakeIntent struct{}

func (fakeIntent) UpsertCreated(context.Context, Request) error   { return nil }
func (fakeIntent) MarkPaid(context.Context, string, string) error { return nil }
func (fakeIntent) MarkFunded(context.Context, string) error       { return nil }
func (fakeIntent) MarkRefunded(context.Context, string) error     { return nil }

type fakeProvider struct{}

func (fakeProvider) CreatePayment(context.Context, Request) (string, string, error) {
	return "https://pay", "trade", nil
}
func (fakeProvider) VerifyCallback([]byte, string) (string, bool, error) { return "", false, nil }

type flakyTopupFyAPI struct {
	failRemaining int32
	calls         int32
}

func (f *flakyTopupFyAPI) TopupCustomer(context.Context, int64, int64, string, string) error {
	atomic.AddInt32(&f.calls, 1)
	if atomic.AddInt32(&f.failRemaining, -1) >= 0 {
		return errors.New("fyapi transient")
	}
	return nil
}

type fakeNotifier struct{}

func (fakeNotifier) Notify(context.Context, string, string, string, string) error { return nil }

func TestTopup_SweepRetriesRegisteredFundStep(t *testing.T) {
	repo := saga.NewMemRepo()
	o := saga.NewOrchestrator(repo, fakeTx{}, nil)
	fy := &flakyTopupFyAPI{failRemaining: 2}
	svc := NewService(o, fakeIntent{}, fakeProvider{}, fy, fakeNotifier{})
	sagaID := saga.MustNewSagaID()

	if err := svc.Fund(context.Background(), sagaID, 42, 100, "tr"); err == nil {
		t.Fatal("expected first fyapi failure")
	}
	step, err := repo.GetStep(context.Background(), sagaID, StepFundFy)
	if err != nil {
		t.Fatalf("GetStep: %v", err)
	}
	if saga.Status(step.Status) != saga.StatusFailed || step.Attempts != 1 {
		t.Fatalf("initial step=%+v", step)
	}
	rewindMemStep(t, repo, sagaID, StepFundFy)
	res, err := o.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("Sweep #1: %v", err)
	}
	if res.Retried != 1 {
		t.Fatalf("Sweep #1 result=%+v", res)
	}
	rewindMemStep(t, repo, sagaID, StepFundFy)
	res, err = o.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("Sweep #2: %v", err)
	}
	if res.Retried != 1 {
		t.Fatalf("Sweep #2 result=%+v", res)
	}
	step, _ = repo.GetStep(context.Background(), sagaID, StepFundFy)
	if saga.Status(step.Status) != saga.StatusCommitted || step.Attempts != 3 {
		t.Fatalf("after sweep step=%+v", step)
	}
	if got := atomic.LoadInt32(&fy.calls); got != 3 {
		t.Fatalf("fy calls=%d want 3", got)
	}
}

func rewindMemStep(t *testing.T, repo *saga.MemRepo, sagaID, stepName string) {
	t.Helper()
	steps, err := repo.GetByID(context.Background(), sagaID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	for _, step := range steps {
		if step.StepName == stepName {
			cp := *step
			cp.UpdatedAt = time.Now().UTC().Add(-time.Hour)
			if err := repo.Save(context.Background(), &cp); err != nil {
				t.Fatalf("Save rewind: %v", err)
			}
			return
		}
	}
	t.Fatalf("step %s not found in %+v", stepName, steps)
}
