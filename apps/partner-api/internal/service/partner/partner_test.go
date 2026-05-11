package partner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

func newSvc(t *testing.T) (*Service, *MemoryRepo, *stubOrphaner) {
	t.Helper()
	repo := NewMemoryRepo()
	orph := NewStubOrphaner()
	now := time.Date(2026, 5, 11, 8, 0, 0, 0, time.UTC)
	svc := NewService(repo, NewStubCrypto(), NewAlwaysFreshConsent(now),
		NewStubInvitation(), orph).WithClock(func() time.Time { return now })
	return svc, repo, orph
}

func TestApply_Happy(t *testing.T) {
	svc, _, _ := newSvc(t)
	p, err := svc.Apply(context.Background(), ApplyInput{
		FyUserID: 100, Type: "individual",
		ContactName: "Alice", ContactPhone: "+8613000001111",
		ContactEmail: "alice@test.com", ConsentID: 1,
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if string(p.Status) != string(StatusApplied) || p.ContactEmailHMAC == "" {
		t.Fatalf("status / hmac wrong: %+v", p)
	}
}

func TestApply_DuplicateEmail(t *testing.T) {
	svc, _, _ := newSvc(t)
	in := ApplyInput{FyUserID: 1, Type: "individual", ContactName: "A",
		ContactPhone: "+8613000000001", ContactEmail: "a@x.com", ConsentID: 1}
	if _, err := svc.Apply(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	in.FyUserID = 2
	if _, err := svc.Apply(context.Background(), in); !errors.Is(err, ErrEmailAlreadyRegistered) {
		t.Fatalf("expected ErrEmailAlreadyRegistered got %v", err)
	}
}

func TestApply_BadInput(t *testing.T) {
	svc, _, _ := newSvc(t)
	_, err := svc.Apply(context.Background(), ApplyInput{Type: "wrong"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestApprove_FromAppliedToApproved(t *testing.T) {
	svc, _, _ := newSvc(t)
	p, _ := svc.Apply(context.Background(), ApplyInput{
		FyUserID: 1, Type: "individual", ContactName: "A",
		ContactPhone: "+8613000000010", ContactEmail: "a10@x.com", ConsentID: 1,
	})
	upd, err := svc.Approve(context.Background(), p.ID, 999)
	if err != nil {
		t.Fatal(err)
	}
	if upd.Status != StatusApproved || upd.ApprovedBy == nil || *upd.ApprovedBy != 999 {
		t.Fatalf("approve wrong: %+v", upd)
	}
}

func TestApprove_InvalidTransition(t *testing.T) {
	svc, repo, _ := newSvc(t)
	p, _ := svc.Apply(context.Background(), ApplyInput{
		FyUserID: 2, Type: "individual", ContactName: "B",
		ContactPhone: "+8613000000020", ContactEmail: "b@x.com", ConsentID: 1,
	})
	if _, err := repo.Update(context.Background(), p.ID, func(p2 domain.Partner) domain.Partner {
		p2.Status = StatusRejected
		return p2
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(context.Background(), p.ID, 1); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition got %v", err)
	}
}

func TestTerminate_OrphansCustomers(t *testing.T) {
	svc, _, orph := newSvc(t)
	p, _ := svc.Apply(context.Background(), ApplyInput{
		FyUserID: 3, Type: "individual", ContactName: "C",
		ContactPhone: "+8613000000030", ContactEmail: "c@x.com", ConsentID: 1,
	})
	if _, err := svc.Approve(context.Background(), p.ID, 1); err != nil {
		t.Fatal(err)
	}
	updated, err := svc.Terminate(context.Background(), p.ID, "fraud", 0)
	if err != nil {
		t.Fatalf("terminate: %v", err)
	}
	if updated.Status != StatusTerminated || updated.TerminatedAt == nil {
		t.Fatalf("status wrong: %+v", updated)
	}
	if _, ok := orph.Called(p.ID); !ok {
		t.Fatal("orphaner not invoked")
	}
}
