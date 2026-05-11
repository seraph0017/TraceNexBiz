package invoice

import (
	"context"
	"errors"
	"testing"
)

func setup(t *testing.T) (*Service, *MemoryRepo) {
	t.Helper()
	repo := NewMemoryRepo()
	repo.PutTitle(&Title{ID: 10, OwnerType: "customer", OwnerID: 7, TitleType: 2, Title: "ACME 上海", TaxNumber: "91310000MA1FL0XXXX"})
	repo.PutTitle(&Title{ID: 11, OwnerType: "customer", OwnerID: 7, TitleType: 1, Title: "Zhang San"})
	svc := NewService(repo, &StubFapiaoGateway{}, SellerProfile{EntityID: 1, TaxNo: "91310000MA0SELLER0", Name: "TraceNex"})
	return svc, repo
}

func TestValidateTaxNo(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"91310000MA1FL0XXXX": true,
		"310000123456789":    true,
		"123":                false,
		"":                   false,
		"INVALID@@#$":        false,
	}
	for in, want := range cases {
		got := ValidateTaxNo(in) == nil
		if got != want {
			t.Errorf("ValidateTaxNo(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestApply_RejectsInvalidAmount(t *testing.T) {
	t.Parallel()
	svc, _ := setup(t)
	_, err := svc.Apply(context.Background(), ApplyInput{Amount: 0, TitleID: 10, ApplicantType: "customer", ApplicantID: 7})
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("expected ErrInvalidAmount, got %v", err)
	}
}

func TestApply_RequiresKnownTitleScopedToOwner(t *testing.T) {
	t.Parallel()
	svc, _ := setup(t)
	// owner mismatch (BOLA) → not found
	_, err := svc.Apply(context.Background(), ApplyInput{Amount: 100, TitleID: 10, ApplicantType: "customer", ApplicantID: 999})
	if !errors.Is(err, ErrTitleNotFound) {
		t.Fatalf("expected ErrTitleNotFound, got %v", err)
	}
}

func TestFullFlow_ApplyReviewIssueRedFlush(t *testing.T) {
	t.Parallel()
	svc, _ := setup(t)
	app, err := svc.Apply(context.Background(), ApplyInput{
		ApplicantType: "customer", ApplicantID: 7, TitleID: 10, Amount: 12300, Period: "2026-04",
	})
	if err != nil {
		t.Fatal(err)
	}
	if app.Status != "applied" {
		t.Fatalf("status = %s", app.Status)
	}
	if app.SellerEntityID != 1 || app.SellerTaxNo == "" {
		t.Fatal("seller not injected")
	}
	if app.ArchiveExpiresAt.Year()-app.AppliedAt.Year() != 10 {
		t.Fatalf("archive expiry should be 10y; got %v vs %v", app.ArchiveExpiresAt, app.AppliedAt)
	}
	// review approve
	app, err = svc.Review(context.Background(), app.ID, true, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if app.Status != "issuing" {
		t.Fatalf("status = %s", app.Status)
	}
	// issue
	app, err = svc.Issue(context.Background(), app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if app.Status != "issued" || app.FapiaoSerial == "" {
		t.Fatalf("issued not set: %+v", app)
	}
	// red flush
	final, rf, err := svc.RedFlush(context.Background(), app.ID, "C001", "客户开具错误", 88)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != "red_flushed" {
		t.Fatalf("final status %s", final.Status)
	}
	if rf.RedFapiaoSerial == "" {
		t.Fatal("red serial missing")
	}
}

func TestRedFlush_RejectsNonIssued(t *testing.T) {
	t.Parallel()
	svc, _ := setup(t)
	app, _ := svc.Apply(context.Background(), ApplyInput{ApplicantType: "customer", ApplicantID: 7, TitleID: 10, Amount: 100})
	_, _, err := svc.RedFlush(context.Background(), app.ID, "C001", "x", 1)
	if !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("want invalid transition, got %v", err)
	}
}

func TestReview_RejectPath(t *testing.T) {
	t.Parallel()
	svc, _ := setup(t)
	app, _ := svc.Apply(context.Background(), ApplyInput{ApplicantType: "customer", ApplicantID: 7, TitleID: 10, Amount: 100})
	app, err := svc.Review(context.Background(), app.ID, false, "X1", "材料不足")
	if err != nil {
		t.Fatal(err)
	}
	if app.Status != "rejected" || app.RejectReasonCode != "X1" {
		t.Fatalf("got %+v", app)
	}
}
