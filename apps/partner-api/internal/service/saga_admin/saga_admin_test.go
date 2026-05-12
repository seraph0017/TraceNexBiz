package saga_admin

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTest() (*Service, *MemoryTokenStore, *MemoryCooldownStore, *StubResolver, *CapturingAudit) {
	tok := NewMemoryTokenStore()
	cd := NewMemoryCooldownStore()
	rv := &StubResolver{}
	au := &CapturingAudit{}
	return NewService(tok, cd, rv, au), tok, cd, rv, au
}

func TestForceResolve_HappyPath(t *testing.T) {
	t.Parallel()
	svc, _, _, rv, au := newTest()
	tk, err := svc.IssueApproverToken(context.Background(), "saga-1", 200, "10.2.2.7")
	if err != nil {
		t.Fatal(err)
	}
	err = svc.ForceResolve(context.Background(), ForceResolveInput{
		SagaID: "saga-1", InitiatorStaffID: 100,
		InitiatorIP: "10.1.1.5",
		ApproverToken: tk.Token, Outcome: "resolved", Reason: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rv.Calls) != 1 || rv.Calls[0].Outcome != "resolved" {
		t.Fatalf("calls = %+v", rv.Calls)
	}
	if len(au.Events) != 1 {
		t.Fatalf("audit not captured")
	}
}

func TestForceResolve_RejectsSamePerson(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTest()
	tk, _ := svc.IssueApproverToken(context.Background(), "s", 100, "10.2.1.1")
	err := svc.ForceResolve(context.Background(), ForceResolveInput{
		SagaID: "s", InitiatorStaffID: 100,
		InitiatorIP: "10.1.1.1",
		ApproverToken: tk.Token, Outcome: "resolved",
	})
	if !errors.Is(err, ErrApproverSamePerson) {
		t.Fatalf("got %v", err)
	}
}

func TestForceResolve_RejectsSameSubnet(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTest()
	tk, _ := svc.IssueApproverToken(context.Background(), "s", 200, "10.1.1.7")
	err := svc.ForceResolve(context.Background(), ForceResolveInput{
		SagaID: "s", InitiatorStaffID: 100,
		InitiatorIP: "10.1.1.5",
		ApproverToken: tk.Token, Outcome: "resolved",
	})
	if !errors.Is(err, ErrApproverSameSubnet) {
		t.Fatalf("got %v", err)
	}
}

func TestForceResolve_TokenSingleUse(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTest()
	tk, _ := svc.IssueApproverToken(context.Background(), "s", 200, "10.2.2.2")
	in := ForceResolveInput{
		SagaID: "s", InitiatorStaffID: 100,
		InitiatorIP: "10.1.1.1",
		ApproverToken: tk.Token, Outcome: "resolved",
	}
	if err := svc.ForceResolve(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	// second use should fail (consumed) — but cooldown will trip first; bump clock to clear cooldown
	svc.clock = func() time.Time { return time.Now().Add(2 * Cooldown) }
	err := svc.ForceResolve(context.Background(), in)
	if !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestForceResolve_TokenExpiry(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTest()
	tk, _ := svc.IssueApproverToken(context.Background(), "s", 200, "10.2.2.2")
	svc.clock = func() time.Time { return time.Now().Add(2 * TokenTTL) }
	err := svc.ForceResolve(context.Background(), ForceResolveInput{
		SagaID: "s", InitiatorStaffID: 100,
		InitiatorIP: "10.1.1.1",
		ApproverToken: tk.Token, Outcome: "resolved",
	})
	if !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestForceResolve_Cooldown(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _ := newTest()
	tk, _ := svc.IssueApproverToken(context.Background(), "s", 200, "10.2.2.2")
	in := ForceResolveInput{
		SagaID: "s", InitiatorStaffID: 100,
		InitiatorIP: "10.1.1.1",
		ApproverToken: tk.Token, Outcome: "resolved",
	}
	if err := svc.ForceResolve(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	// Issue new token; second resolve hits cooldown
	tk2, _ := svc.IssueApproverToken(context.Background(), "s", 300, "10.3.3.3")
	in.ApproverToken = tk2.Token
	err := svc.ForceResolve(context.Background(), in)
	if !errors.Is(err, ErrCooldown) {
		t.Fatalf("got %v", err)
	}
}
