package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPublicFooterHandler_NoDeps(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterPublicFooterRoute(r, nil, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/public/biz_setting/footer", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Success bool          `json:"success"`
		Data    FooterPayload `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	if !env.Success {
		t.Fatal("expected success=true")
	}
	if env.Data.TOSURL == "" || env.Data.PrivacyURL == "" {
		t.Fatalf("expected defaults for tos/privacy, got %+v", env.Data)
	}
	if env.Data.ConsentTextVersion == "" {
		t.Fatal("expected non-empty default consent_text_version")
	}
}
