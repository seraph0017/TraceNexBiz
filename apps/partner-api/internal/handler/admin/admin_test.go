package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/middleware"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/content_safety"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/invoice"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/saga_admin"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/staff"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/ticket"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func newTestRouter(t *testing.T) (*gin.Engine, Deps) {
	t.Helper()
	r := gin.New()

	// invoice setup
	invRepo := invoice.NewMemoryRepo()
	invRepo.PutTitle(&invoice.Title{ID: 1, OwnerType: "customer", OwnerID: 1, TitleType: 2, Title: "ACME", TaxNumber: "91310000MA1FL0XXXX"})
	invSvc := invoice.NewService(invRepo, &invoice.StubFapiaoGateway{}, invoice.SellerProfile{EntityID: 1, TaxNo: "91310000MA0SELLER0", Name: "TraceNex"})

	// ticket setup
	tickSvc := ticket.NewService(ticket.NewMemoryRepo())

	// cs setup
	csSvc := content_safety.NewService(content_safety.NewMemoryRepo(), &content_safety.CapturingAuthorityClient{})

	// staff setup
	staffAllow := staff.NewCIDRAllowlist([]string{"127.0.0.1/32", "10.0.0.0/8"})
	staffSvc := staff.NewService(staff.NewMemoryRepo(), staffAllow)

	// saga admin setup
	sagaSvc := saga_admin.NewService(saga_admin.NewMemoryTokenStore(), saga_admin.NewMemoryCooldownStore(), &saga_admin.StubResolver{}, &saga_admin.CapturingAudit{})

	deps := Deps{
		Invoice: invSvc, Ticket: tickSvc, ContentSafety: csSvc, Staff: staffSvc, SagaAdmin: sagaSvc,
	}
	rg := r.Group("/api/admin")
	rg.Use(func(c *gin.Context) {
		c.Set("staff_id", int64(1))
		c.Set("staff_role", "super_admin")
		// 装载 JWT claims（WithScope 内 BOLA 需要 ClaimsFrom）
		c.Set(middleware.CtxKeyJWTClaims, &middleware.Claims{
			ActorType: "staff", ActorID: 1, Jti: "test-jti",
		})
		c.Next()
	})
	Register(rg, deps)
	return r, deps
}

func doJSON(r http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Real-IP", "10.1.1.1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAdmin_InvoiceFullCycle(t *testing.T) {
	t.Parallel()
	r, deps := newTestRouter(t)

	// seed application
	app, err := deps.Invoice.Apply(context.Background(), invoice.ApplyInput{
		ApplicantType: "customer", ApplicantID: 1, TitleID: 1, Amount: 100,
	})
	if err != nil {
		t.Fatal(err)
	}

	// review
	w := doJSON(r, "POST", "/api/admin/invoice/"+itoa(app.ID)+"/review", map[string]any{"approve": true})
	if w.Code != 200 {
		t.Fatalf("review: %d %s", w.Code, w.Body.String())
	}
	// issue
	w = doJSON(r, "POST", "/api/admin/invoice/"+itoa(app.ID)+"/issue", nil)
	if w.Code != 200 {
		t.Fatalf("issue: %d %s", w.Code, w.Body.String())
	}
	// red flush
	w = doJSON(r, "POST", "/api/admin/invoice/"+itoa(app.ID)+"/red-flush", map[string]any{"reason_code": "C001", "reason_text": "fix"})
	if w.Code != 200 {
		t.Fatalf("red_flush: %d %s", w.Code, w.Body.String())
	}
}

func TestAdmin_TicketListAndAssign(t *testing.T) {
	t.Parallel()
	r, deps := newTestRouter(t)
	tk, _ := deps.Ticket.Create(context.Background(), ticket.CreateInput{OpenerType: "customer", OpenerID: 1, Subject: "x", Category: "billing"})
	w := doJSON(r, "GET", "/api/admin/tickets?status=open&limit=10", nil)
	if w.Code != 200 {
		t.Fatalf("list: %d", w.Code)
	}
	w = doJSON(r, "POST", "/api/admin/tickets/"+itoa(tk.ID)+"/assign", map[string]any{"staff_id": 99})
	if w.Code != 200 {
		t.Fatalf("assign: %d %s", w.Code, w.Body.String())
	}
}

func TestAdmin_ContentSafetyEndpoints(t *testing.T) {
	t.Parallel()
	r, deps := newTestRouter(t)
	_, _ = deps.ContentSafety.RecordEvent(context.Background(), content_safety.Event{FyUserID: 1, Disposition: "block"})
	w := doJSON(r, "GET", "/api/admin/content-safety/reports", nil)
	if w.Code != 200 {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, "POST", "/api/admin/content-safety/reports/dispatch", map[string]any{"batch": 10})
	if w.Code != 200 {
		t.Fatalf("dispatch: %d %s", w.Code, w.Body.String())
	}
}

func TestAdmin_StaffCreate_RequiresAllowlist(t *testing.T) {
	t.Parallel()
	r, _ := newTestRouter(t)
	w := doJSON(r, "POST", "/api/admin/staff", map[string]any{
		"username": "u1", "password_hash": "h", "role": "cs_admin", "email": "a@b",
	})
	if w.Code != 200 {
		t.Fatalf("staff create: %d %s", w.Code, w.Body.String())
	}
}

func TestAdmin_SagaForceResolve(t *testing.T) {
	t.Parallel()
	r, deps := newTestRouter(t)
	tk, err := deps.SagaAdmin.IssueApproverToken(context.Background(), "saga-X", 222, "10.99.0.1")
	if err != nil {
		t.Fatal(err)
	}
	w := doJSON(r, "POST", "/api/admin/saga/saga-X/force-resolve", map[string]any{
		"approver_token": tk.Token,
		"outcome":        "resolved",
		"reason":         "manual",
	})
	if w.Code != 200 {
		t.Fatalf("force_resolve: %d %s", w.Code, w.Body.String())
	}
}

func TestAdmin_ParseInvalidID(t *testing.T) {
	t.Parallel()
	r, _ := newTestRouter(t)
	w := doJSON(r, "POST", "/api/admin/invoice/abc/review", map[string]any{"approve": true})
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func itoa(i int64) string {
	return strconv.FormatInt(i, 10)
}
