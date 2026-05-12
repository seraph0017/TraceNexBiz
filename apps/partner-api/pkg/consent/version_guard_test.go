package consent

import (
	"errors"
	"testing"
)

type stubSettings struct{ v string }

func (s stubSettings) Get(k string) string {
	if k == "compliance.consent_text_version" {
		return s.v
	}
	return ""
}

func TestVerify_Match(t *testing.T) {
	g := NewVersionGuard(stubSettings{v: "v2.0"})
	if err := g.Verify("v2.0"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestVerify_Stale(t *testing.T) {
	g := NewVersionGuard(stubSettings{v: "v2.0"})
	if err := g.Verify("v1.9"); !errors.Is(err, ErrConsentVersionStale) {
		t.Fatalf("expected ErrConsentVersionStale got %v", err)
	}
}

func TestVerify_Empty(t *testing.T) {
	g := NewVersionGuard(stubSettings{v: "v2.0"})
	if err := g.Verify(""); !errors.Is(err, ErrConsentVersionEmpty) {
		t.Fatalf("expected ErrConsentVersionEmpty got %v", err)
	}
}

func TestVerify_DevModeFallback(t *testing.T) {
	g := NewVersionGuard(stubSettings{v: ""})
	g.SetDevMode(true)
	if err := g.Verify("v2.0"); err != nil {
		t.Fatalf("dev fallback should accept non-empty submitted; got %v", err)
	}
}

func TestVerify_ProdRefusesEmptyBizSetting(t *testing.T) {
	g := NewVersionGuard(stubSettings{v: ""})
	if err := g.Verify("v2.0"); !errors.Is(err, ErrConsentVersionStale) {
		t.Fatalf("prod must refuse when biz_setting current is empty; got %v", err)
	}
}
