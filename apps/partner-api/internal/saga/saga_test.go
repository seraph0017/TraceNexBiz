package saga

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// fakeTx 测试用：fn 直接调用，不走 DB.
type fakeTx struct{}

func (fakeTx) WithTx(_ context.Context, fn TxFn) (any, error) {
	return fn(nil)
}

func newTestOrch(t *testing.T) (Orchestrator, *MemRepo) {
	t.Helper()
	repo := NewMemRepo()
	o := NewOrchestrator(repo, fakeTx{}, nil)
	return o, repo
}

func TestNewSagaID_IsUUIDv7(t *testing.T) {
	id, err := NewSagaID()
	if err != nil {
		t.Fatalf("NewSagaID: %v", err)
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.Version() != 7 {
		t.Fatalf("expected v7, got v%d", parsed.Version())
	}
}

func TestRun_HappyPath_Idempotent(t *testing.T) {
	o, repo := newTestOrch(t)
	id := MustNewSagaID()
	s, err := o.NewSaga(id, KindWalletAllocate)
	if err != nil {
		t.Fatalf("NewSaga: %v", err)
	}
	called := 0
	fn := TxFn(func(_ *gorm.DB) (any, error) {
		called++
		return "ok", nil
	})
	if _, err := s.Run(context.Background(), "wallet.hold", fn); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// 第二次同 step 调用必须不再执行 fn（committed 幂等）
	if _, err := s.Run(context.Background(), "wallet.hold", fn); err != nil {
		t.Fatalf("Run idempotent: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected fn called once, got %d", called)
	}
	step, err := repo.GetStep(context.Background(), id, "wallet.hold")
	if err != nil {
		t.Fatalf("GetStep: %v", err)
	}
	if Status(step.Status) != StatusCommitted {
		t.Fatalf("status=%s", step.Status)
	}
}

func TestRun_FailureSetsFailedAndAttempts(t *testing.T) {
	o, repo := newTestOrch(t)
	id := MustNewSagaID()
	s, _ := o.NewSaga(id, KindWalletAllocate)

	bizErr := errors.New("network down")
	fn := TxFn(func(_ *gorm.DB) (any, error) { return nil, bizErr })
	if _, err := s.Run(context.Background(), "fyapi.topup", fn); err == nil {
		t.Fatal("expected error")
	}
	step, _ := repo.GetStep(context.Background(), id, "fyapi.topup")
	if Status(step.Status) != StatusFailed {
		t.Fatalf("status=%s", step.Status)
	}
	if step.Attempts != 1 {
		t.Fatalf("attempts=%d", step.Attempts)
	}

	// retry：attempts 递增，仍失败
	if _, err := s.Run(context.Background(), "fyapi.topup", fn); err == nil {
		t.Fatal("expected error on retry")
	}
	step, _ = repo.GetStep(context.Background(), id, "fyapi.topup")
	if step.Attempts != 2 {
		t.Fatalf("attempts after retry=%d", step.Attempts)
	}
}

func TestCompensate_OnlyAfterCommit(t *testing.T) {
	o, repo := newTestOrch(t)
	id := MustNewSagaID()
	s, _ := o.NewSaga(id, KindCustomerRefund)

	fn := TxFn(func(_ *gorm.DB) (any, error) { return 1, nil })
	if _, err := s.Run(context.Background(), "wallet.commit", fn); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := s.Compensate(context.Background(), "wallet.commit", fn); err != nil {
		t.Fatalf("Compensate: %v", err)
	}
	step, _ := repo.GetStep(context.Background(), id, "wallet.commit")
	if Status(step.Status) != StatusCompensated {
		t.Fatalf("status=%s", step.Status)
	}

	// 对不存在 step 的 compensate
	if _, err := s.Compensate(context.Background(), "ghost", fn); !errors.Is(err, ErrStepNotFound) {
		t.Fatalf("expected ErrStepNotFound, got %v", err)
	}
}

func TestCompensate_Idempotent(t *testing.T) {
	o, _ := newTestOrch(t)
	id := MustNewSagaID()
	s, _ := o.NewSaga(id, KindCustomerRefund)
	fn := TxFn(func(_ *gorm.DB) (any, error) { return nil, nil })
	_, _ = s.Run(context.Background(), "x", fn)
	if _, err := s.Compensate(context.Background(), "x", fn); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Compensate(context.Background(), "x", fn); err != nil {
		t.Fatalf("second compensate must be idempotent: %v", err)
	}
}

func TestSweep_EscalatesAfterMaxAttempts(t *testing.T) {
	o, repo := newTestOrch(t)
	orch := o.(*orchestrator)
	id := MustNewSagaID()
	s, _ := o.NewSaga(id, KindWalletAllocate)
	bizErr := errors.New("boom")
	for k := 0; k < MaxAttempts; k++ {
		_, _ = s.Run(context.Background(), "step1", TxFn(func(_ *gorm.DB) (any, error) { return nil, bizErr }))
	}
	res, err := orch.Sweep(context.Background(), 100)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	// 在最后一次 Run 内已 escalate；Sweep 此时见到的 step 已是 escalated（不再 in_progress/failed）
	step, _ := repo.GetStep(context.Background(), id, "step1")
	if Status(step.Status) != StatusEscalated {
		// 兜底：Sweep 应能接续 escalate
		if res.Escalated == 0 {
			t.Fatalf("expected escalation, status=%s, sweep=%+v", step.Status, res)
		}
	}
}

func TestValidateForceResolve(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	id := MustNewSagaID()
	base := ForceResolveRequest{
		SagaID:        id,
		StepName:      "fyapi.topup",
		Target:        StatusCompensated,
		ActorID:       1,
		ActorIP:       "10.1.1.5",
		ApproverID:    2,
		ApproverIP:    "10.2.1.5",
		TokenIssuedAt: now.Add(-5 * time.Minute),
		Now:           now,
		Reason:        "ops decided to compensate after upstream confirms not paid",
	}
	if err := ValidateForceResolve(base); err != nil {
		t.Fatalf("happy: %v", err)
	}

	// same actor
	r := base
	r.ApproverID = r.ActorID
	if err := ValidateForceResolve(r); !errors.Is(err, ErrSameActor) {
		t.Fatalf("same actor: %v", err)
	}

	// same /24
	r = base
	r.ApproverIP = "10.1.1.99"
	if err := ValidateForceResolve(r); !errors.Is(err, ErrIPSameSubnet) {
		t.Fatalf("same subnet: %v", err)
	}

	// token expired
	r = base
	r.TokenIssuedAt = now.Add(-31 * time.Minute)
	if err := ValidateForceResolve(r); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expired: %v", err)
	}

	// cooldown
	r = base
	last := now.Add(-10 * time.Minute)
	r.LastResolvedAt = &last
	if err := ValidateForceResolve(r); !errors.Is(err, ErrCooldownViolated) {
		t.Fatalf("cooldown: %v", err)
	}

	// invalid target
	r = base
	r.Target = StatusInProgress
	if err := ValidateForceResolve(r); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("invalid target: %v", err)
	}

	// empty reason
	r = base
	r.Reason = "   "
	if err := ValidateForceResolve(r); !errors.Is(err, ErrEmptyReason) {
		t.Fatalf("empty reason: %v", err)
	}

	// invalid IP
	r = base
	r.ActorIP = "not-an-ip"
	if err := ValidateForceResolve(r); !errors.Is(err, ErrInvalidIP) {
		t.Fatalf("invalid ip: %v", err)
	}
}

func TestSubnetEqual_IPv6(t *testing.T) {
	a := net.ParseIP("2001:db8:1::1")
	b := net.ParseIP("2001:db8:1::ffff")
	if !subnetEqual(a, b) {
		t.Fatal("ipv6 same /48 should be equal")
	}
	c := net.ParseIP("2001:db9:1::1")
	if subnetEqual(a, c) {
		t.Fatal("different /48 should differ")
	}
}

func TestNewSaga_RejectNonUUIDv7(t *testing.T) {
	o, _ := newTestOrch(t)
	if _, err := o.NewSaga("not-uuid", KindWalletAllocate); err == nil {
		t.Fatal("expected reject")
	}
	v4 := uuid.NewString()
	if _, err := o.NewSaga(v4, KindWalletAllocate); err == nil {
		t.Fatal("expected v4 reject")
	}
}

func TestBackoffFor(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{attempts: 0, want: 2 * time.Second},
		{attempts: 1, want: 2 * time.Second},
		{attempts: 2, want: 4 * time.Second},
		{attempts: 3, want: 8 * time.Second},
		{attempts: 8, want: 256 * time.Second},
		{attempts: 30, want: BackoffMax},
	}
	for _, tc := range cases {
		if got := BackoffFor(tc.attempts); got != tc.want {
			t.Fatalf("attempts=%d: got %v want %v", tc.attempts, got, tc.want)
		}
	}
}
