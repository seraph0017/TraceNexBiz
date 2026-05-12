// internal/saga/saga_retry_sweep_test.go — Fix-B' part 2 CRIT-B2 regression.
//
// Verifies that the Sweep actually re-runs registered StepFunc, not just bumps status:
//   - flaky fn succeeds on attempt 3 → sweep eventually commits
//   - permanent error → immediate escalate, no further retry
//   - exhausted attempts → escalate, no further retry
package saga

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// newSweepableOrch 直接给出 *orchestrator（暴露 Sweep）；fakeTx 不走 DB.
func newSweepableOrch(t *testing.T) (*orchestrator, *MemRepo) {
	t.Helper()
	repo := NewMemRepo()
	return &orchestrator{repo: repo, tx: fakeTx{}}, repo
}

func TestSweep_FlakyStep_EventuallySucceeds(t *testing.T) {
	t.Cleanup(ResetRegistryForTest)
	ResetRegistryForTest()

	const stepName = "flaky.step"
	var calls int32
	fn := StepFunc(func(_ context.Context, _ *gorm.DB, _ []byte) (any, error) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return nil, errors.New("transient")
		}
		return "ok", nil
	})
	RegisterStep(KindWalletAllocate, stepName, fn)

	o, repo := newSweepableOrch(t)
	id := MustNewSagaID()
	s, err := o.NewSaga(id, KindWalletAllocate)
	if err != nil {
		t.Fatalf("NewSaga: %v", err)
	}

	// First call → transient failure, attempts=1, status=failed.
	_, err = s.RunWithInput(context.Background(), stepName, []byte("{}"), fn)
	if err == nil {
		t.Fatalf("expected transient failure")
	}
	step, _ := repo.GetStep(context.Background(), id, stepName)
	if Status(step.Status) != StatusFailed || step.Attempts != 1 {
		t.Fatalf("unexpected step state: status=%s attempts=%d", step.Status, step.Attempts)
	}

	// Force updated_at backwards so backoff lets sweep proceed.
	rewindStep(t, repo, id, stepName, -1*time.Hour)

	// Sweep #1 → second attempt, still transient. Retried=1.
	res, err := o.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Retried != 1 {
		t.Fatalf("expected Retried=1, got %+v", res)
	}
	step, _ = repo.GetStep(context.Background(), id, stepName)
	if Status(step.Status) != StatusFailed || step.Attempts != 2 {
		t.Fatalf("after sweep#1: status=%s attempts=%d", step.Status, step.Attempts)
	}

	rewindStep(t, repo, id, stepName, -1*time.Hour)

	// Sweep #2 → third attempt succeeds.
	res, err = o.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Retried != 1 {
		t.Fatalf("expected Retried=1, got %+v", res)
	}
	step, _ = repo.GetStep(context.Background(), id, stepName)
	if Status(step.Status) != StatusCommitted {
		t.Fatalf("expected committed, got %s", step.Status)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("fn invocations = %d, want 3", got)
	}
}

func TestSweep_PermanentError_EscalatesImmediately(t *testing.T) {
	t.Cleanup(ResetRegistryForTest)
	ResetRegistryForTest()

	const stepName = "perm.step"
	var calls int32
	fn := StepFunc(func(_ context.Context, _ *gorm.DB, _ []byte) (any, error) {
		atomic.AddInt32(&calls, 1)
		return nil, ErrPermanent
	})
	RegisterStep(KindCustomerTopup, stepName, fn)

	o, repo := newSweepableOrch(t)
	id := MustNewSagaID()
	s, _ := o.NewSaga(id, KindCustomerTopup)

	_, err := s.RunWithInput(context.Background(), stepName, []byte("x"), fn)
	if err == nil || !errors.Is(err, ErrPermanent) {
		t.Fatalf("expected ErrPermanent wrap, got %v", err)
	}
	step, _ := repo.GetStep(context.Background(), id, stepName)
	if Status(step.Status) != StatusEscalated {
		t.Fatalf("expected escalated, got %s", step.Status)
	}
	// Sweep must NOT re-run an escalated step.
	rewindStep(t, repo, id, stepName, -1*time.Hour)
	if _, err := o.Sweep(context.Background(), 100); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("fn calls = %d, want 1", got)
	}
}

func TestSweep_ExhaustedAttempts_NoFurtherRetry(t *testing.T) {
	t.Cleanup(ResetRegistryForTest)
	ResetRegistryForTest()

	const stepName = "doomed"
	fn := StepFunc(func(_ context.Context, _ *gorm.DB, _ []byte) (any, error) {
		return nil, errors.New("nope")
	})
	RegisterStep(KindWalletAllocate, stepName, fn)

	o, repo := newSweepableOrch(t)
	id := MustNewSagaID()
	s, _ := o.NewSaga(id, KindWalletAllocate)
	for k := 0; k < MaxAttempts; k++ {
		_, _ = s.RunWithInput(context.Background(), stepName, []byte("{}"), fn)
		rewindStep(t, repo, id, stepName, -1*time.Hour)
	}
	step, _ := repo.GetStep(context.Background(), id, stepName)
	if Status(step.Status) != StatusEscalated {
		// Sweep should escalate on the next tick if not already.
		res, err := o.Sweep(context.Background(), 100)
		if err != nil {
			t.Fatalf("Sweep: %v", err)
		}
		step, _ = repo.GetStep(context.Background(), id, stepName)
		if Status(step.Status) != StatusEscalated && res.Escalated == 0 {
			t.Fatalf("expected escalation; status=%s sweep=%+v", step.Status, res)
		}
	}
	// One more sweep must NOT touch escalated row.
	rewindStep(t, repo, id, stepName, -1*time.Hour)
	preCallAttempts := step.Attempts
	if _, err := o.Sweep(context.Background(), 100); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	step, _ = repo.GetStep(context.Background(), id, stepName)
	if step.Attempts != preCallAttempts {
		t.Fatalf("escalated step attempts changed: %d -> %d", preCallAttempts, step.Attempts)
	}
}

func TestSweep_NoPayload_Skipped(t *testing.T) {
	t.Cleanup(ResetRegistryForTest)
	ResetRegistryForTest()

	// Legacy Run（不带 Payload）→ Sweep 必然 Skipped；不能 panic / mis-retry。
	o, repo := newSweepableOrch(t)
	id := MustNewSagaID()
	s, _ := o.NewSaga(id, KindWalletAllocate)
	fn := TxFn(func(_ *gorm.DB) (any, error) { return nil, errors.New("transient") })
	_, _ = s.Run(context.Background(), "legacy.step", fn)
	rewindStep(t, repo, id, "legacy.step", -1*time.Hour)
	res, err := o.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Retried != 0 || res.Skipped == 0 {
		t.Fatalf("expected Retried=0 Skipped>=1, got %+v", res)
	}
}

func TestRegistry_DuplicateDifferentFnPanics(t *testing.T) {
	t.Cleanup(ResetRegistryForTest)
	ResetRegistryForTest()

	fn1 := StepFunc(func(_ context.Context, _ *gorm.DB, _ []byte) (any, error) { return nil, nil })
	fn2 := StepFunc(func(_ context.Context, _ *gorm.DB, _ []byte) (any, error) { return nil, nil })
	RegisterStep(KindWalletAllocate, "dup", fn1)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on conflicting RegisterStep")
		}
	}()
	RegisterStep(KindWalletAllocate, "dup", fn2)
}

func TestRegistry_DuplicateSameFnIdempotent(t *testing.T) {
	t.Cleanup(ResetRegistryForTest)
	ResetRegistryForTest()

	fn := StepFunc(func(_ context.Context, _ *gorm.DB, _ []byte) (any, error) { return nil, nil })
	RegisterStep(KindWalletAllocate, "same", fn)
	RegisterStep(KindWalletAllocate, "same", fn) // must NOT panic
}

// rewindStep 把 step.UpdatedAt 回拨 delta 秒，绕过 backoff，便于测试 Sweep 推进。
// StartedAt 故意不动 —— 否则会触发 MaxWallClock 误 escalate。
func rewindStep(t *testing.T, repo *MemRepo, sagaID, stepName string, delta time.Duration) {
	t.Helper()
	repo.mu.Lock()
	defer repo.mu.Unlock()
	bucket, ok := repo.steps[sagaID]
	if !ok {
		return
	}
	row, ok := bucket[stepName]
	if !ok {
		return
	}
	cp := *row
	cp.UpdatedAt = time.Now().UTC().Add(delta)
	bucket[stepName] = &cp
}

// guard unused symbols if domain wiring shifts.
var _ = domain.SagaStep{}
