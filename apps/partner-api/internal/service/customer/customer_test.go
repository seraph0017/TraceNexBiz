package customer

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newSvc(t *testing.T) (*Service, *MemoryRepo, *stubInvitation, *stubFyAPI) {
	t.Helper()
	repo := NewMemoryRepo()
	inv := NewStubInvitation()
	fy := NewStubFyAPI()
	now := time.Date(2026, 5, 11, 8, 0, 0, 0, time.UTC)
	svc := NewService(repo, inv, fy).WithClock(func() time.Time { return now })
	return svc, repo, inv, fy
}

func TestRegister_HappyWithInvitation(t *testing.T) {
	svc, _, inv, _ := newSvc(t)
	inv.Seed("CODEABC", 7)
	c, err := svc.Register(context.Background(), RegisterInput{
		FyUserID: 100, InvitationCode: "CODEABC",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.PartnerID == nil || *c.PartnerID != 7 {
		t.Fatalf("partner_id wrong: %+v", c)
	}
	if c.Status != StatusActive || c.JoinedVia != JoinedInvitation {
		t.Fatalf("status/joined wrong: %+v", c)
	}
}

func TestRegister_BypassBlocked(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	_, err := svc.Register(context.Background(), RegisterInput{FyUserID: 100})
	if !errors.Is(err, ErrInvitationRequired) {
		t.Fatalf("expected ErrInvitationRequired got %v", err)
	}
}

func TestRegister_DuplicateBlocked(t *testing.T) {
	svc, _, inv, _ := newSvc(t)
	inv.Seed("X1", 1)
	if _, err := svc.Register(context.Background(), RegisterInput{FyUserID: 200, InvitationCode: "X1"}); err != nil {
		t.Fatal(err)
	}
	inv.Seed("X2", 2)
	_, err := svc.Register(context.Background(), RegisterInput{FyUserID: 200, InvitationCode: "X2"})
	if !errors.Is(err, ErrAlreadyAffiliated) {
		t.Fatalf("expected ErrAlreadyAffiliated got %v", err)
	}
}

func TestGetForPartner_BOLAScope(t *testing.T) {
	svc, _, inv, _ := newSvc(t)
	inv.Seed("X1", 1)
	c, _ := svc.Register(context.Background(), RegisterInput{FyUserID: 200, InvitationCode: "X1"})
	if got, _ := svc.GetForPartner(context.Background(), 1, c.ID); got == nil {
		t.Fatal("expected found")
	}
	if _, err := svc.GetForPartner(context.Background(), 999, c.ID); !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("expected not found for cross-partner; got %v", err)
	}
}

func TestOrphanByPartner_BulkUpdate(t *testing.T) {
	svc, repo, inv, _ := newSvc(t)
	inv.Seed("A", 1)
	inv.Seed("B", 1)
	if _, err := svc.Register(context.Background(), RegisterInput{FyUserID: 1, InvitationCode: "A"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Register(context.Background(), RegisterInput{FyUserID: 2, InvitationCode: "B"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.OrphanByPartner(context.Background(), 1, time.Now().Add(30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.ListByPartner(context.Background(), 1, ListFilter{Status: StatusOrphaned, Limit: 10})
	if len(got) != 2 {
		t.Fatalf("expected 2 orphaned got %d", len(got))
	}
}

func TestRequestTransfer_AndAdvance(t *testing.T) {
	svc, _, inv, fy := newSvc(t)
	inv.Seed("Z", 1)
	c, _ := svc.Register(context.Background(), RegisterInput{FyUserID: 99, InvitationCode: "Z"})
	log, err := svc.RequestTransfer(context.Background(), TransferRequestInput{
		CustomerID: c.ID, FromPartnerID: 1, ToPartnerID: 2,
		InitiatorType: "customer", InitiatorID: 99, Reason: "better",
	})
	if err != nil {
		t.Fatal(err)
	}
	if log.Status != "pending_a" {
		t.Fatalf("expected pending_a got %s", log.Status)
	}
	if _, err := svc.AdvanceTransferStage(context.Background(), log.ID, "completed"); err != nil {
		t.Fatal(err)
	}
	if got := fy.Group(99); got != "partner_2_default" {
		t.Fatalf("group not updated; got %q", got)
	}
}

func TestSubmitErase(t *testing.T) {
	svc, _, inv, fy := newSvc(t)
	inv.Seed("E", 1)
	c, _ := svc.Register(context.Background(), RegisterInput{FyUserID: 5, InvitationCode: "E"})
	if err := svc.SubmitErase(context.Background(), EraseInput{CustomerID: c.ID, PartnerID: 1, Reason: "pipl"}); err != nil {
		t.Fatal(err)
	}
	if !fy.Erased(5) {
		t.Fatal("fyapi erase not invoked")
	}
}

func TestTransfer_BOLAScope(t *testing.T) {
	svc, _, inv, _ := newSvc(t)
	inv.Seed("T", 1)
	c, _ := svc.Register(context.Background(), RegisterInput{FyUserID: 6, InvitationCode: "T"})
	_, err := svc.RequestTransfer(context.Background(), TransferRequestInput{
		CustomerID: c.ID, FromPartnerID: 2, ToPartnerID: 3,
		InitiatorType: "staff", InitiatorID: 9,
	})
	if !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("expected scoped not found for transfer got %v", err)
	}
	if err := svc.SubmitErase(context.Background(), EraseInput{CustomerID: c.ID, PartnerID: 2}); !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("expected scoped not found for erase got %v", err)
	}
}
