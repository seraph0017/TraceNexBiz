package admin

import (
	"context"
	"testing"
)

// TestDualControl_InitiatorEqualsApprover verifies that initiator == approver is rejected.
func TestDualControl_InitiatorEqualsApprover(t *testing.T) {
	t.Parallel()
	r, deps := newTestRouter(t)
	// Staff 1 issues token for themselves
	tk, err := deps.SagaAdmin.IssueApproverToken(context.Background(), "saga-dual-1", 1, "10.2.2.2")
	if err != nil {
		t.Fatal(err)
	}
	// Staff 1 tries to force-resolve using their own token
	w := doJSON(r, "POST", "/api/admin/saga/saga-dual-1/force-resolve", map[string]any{
		"approver_token": tk.Token,
		"outcome":        "resolved",
		"reason":         "test",
	})
	if w.Code != 403 {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDualControl_MissingApproverToken verifies that missing token is rejected.
func TestDualControl_MissingApproverToken(t *testing.T) {
	t.Parallel()
	r, _ := newTestRouter(t)
	w := doJSON(r, "POST", "/api/admin/saga/saga-dual-2/force-resolve", map[string]any{
		"outcome": "resolved",
		"reason":  "test",
	})
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDualControl_HappyPath verifies that two distinct staff can force-resolve.
func TestDualControl_HappyPath(t *testing.T) {
	t.Parallel()
	r, deps := newTestRouter(t)
	// Staff 2 issues token (approver)
	tk, err := deps.SagaAdmin.IssueApproverToken(context.Background(), "saga-dual-4", 2, "10.4.4.4")
	if err != nil {
		t.Fatal(err)
	}
	// Staff 1 (initiator, from middleware stub) uses the token
	w := doJSON(r, "POST", "/api/admin/saga/saga-dual-4/force-resolve", map[string]any{
		"approver_token": tk.Token,
		"outcome":        "resolved",
		"reason":         "dual-control happy path",
	})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

