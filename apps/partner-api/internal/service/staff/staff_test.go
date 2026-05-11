package staff

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTestService() *Service {
	allow := NewCIDRAllowlist([]string{"10.0.0.0/8", "127.0.0.1/32"})
	return NewService(NewMemoryRepo(), allow)
}

func TestCreate_RequiresSuperAdmin(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	_, err := svc.Create(context.Background(), CreateInput{
		ActorID: 1, ActorRole: RoleFinanceAdmin, ActorIP: "10.0.0.1",
		Username: "u1", PasswordHash: "h", Role: RoleCSAdmin,
	})
	if !errors.Is(err, ErrSuperAdminOnly) {
		t.Fatalf("got %v", err)
	}
}

func TestCreate_RequiresIPAllowlist(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	_, err := svc.Create(context.Background(), CreateInput{
		ActorRole: RoleSuperAdmin, ActorIP: "8.8.8.8",
		Username: "u", PasswordHash: "h", Role: RoleCSAdmin,
	})
	if !errors.Is(err, ErrIPNotAllowed) {
		t.Fatalf("got %v", err)
	}
}

func TestCreate_HappyPath(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	st, err := svc.Create(context.Background(), CreateInput{
		ActorRole: RoleSuperAdmin, ActorIP: "10.1.2.3",
		Username: "alice", PasswordHash: "h", Role: RoleCSAdmin, Email: "a@b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "active" || st.Role != RoleCSAdmin {
		t.Fatalf("%+v", st)
	}
}

func TestSetRole_RequiresStepUp(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	st, _ := svc.Create(context.Background(), CreateInput{
		ActorRole: RoleSuperAdmin, ActorIP: "10.1.1.1", Username: "u", PasswordHash: "h", Role: RoleCSAdmin,
	})
	// no step-up → error
	_, err := svc.SetRole(context.Background(), st.ID, RoleSuperAdmin, "10.1.1.1", st.ID, RoleRiskAdmin)
	if !errors.Is(err, ErrStepUpRequired) {
		t.Fatalf("got %v", err)
	}
	// after step-up
	if err := svc.MarkStepUpPassed(context.Background(), st.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := svc.SetRole(context.Background(), st.ID, RoleSuperAdmin, "10.1.1.1", st.ID, RoleRiskAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Role != RoleRiskAdmin {
		t.Fatalf("role = %s", updated.Role)
	}
}

func TestStepUp_ExpiresAfterWindow(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	st, _ := svc.Create(context.Background(), CreateInput{ActorRole: RoleSuperAdmin, ActorIP: "10.0.0.1", Username: "u", PasswordHash: "h", Role: RoleCSAdmin})
	_ = svc.MarkStepUpPassed(context.Background(), st.ID)
	if err := svc.CheckStepUp(context.Background(), st.ID); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	// fast-forward
	svc.clock = func() time.Time { return time.Now().Add(StepUpWindow + time.Minute) }
	if err := svc.CheckStepUp(context.Background(), st.ID); !errors.Is(err, ErrStepUpRequired) {
		t.Fatalf("got %v", err)
	}
}

func TestList_Authz(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	if _, err := svc.List(context.Background(), RoleCSAdmin, "10.0.0.1"); !errors.Is(err, ErrSuperAdminOnly) {
		t.Fatalf("got %v", err)
	}
	if _, err := svc.List(context.Background(), RoleSuperAdmin, "8.8.8.8"); !errors.Is(err, ErrIPNotAllowed) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRole(t *testing.T) {
	t.Parallel()
	if err := ValidateRole(RoleKYCReviewer); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRole("hacker"); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("got %v", err)
	}
}
