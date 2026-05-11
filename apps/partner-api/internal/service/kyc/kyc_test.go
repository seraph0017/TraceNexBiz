package kyc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

func newSvc(t *testing.T) (*Service, *MemoryRepo, *stubLinker) {
	t.Helper()
	repo := NewMemoryRepo()
	link := NewStubLinker()
	now := time.Date(2026, 5, 11, 8, 0, 0, 0, time.UTC)
	svc := NewService(repo, NewStubCrypto(), NewStubOCR(), NewStubOSS(),
		NewStubConsent(), link).WithClock(func() time.Time { return now })
	return svc, repo, link
}

func goodInput(fyUserID int64, idNo string) SubmitInput {
	return SubmitInput{
		FyUserID: fyUserID, Type: TypeIndividual,
		LegalPersonName:      "Alice",
		LegalPersonID:        idNo,
		LegalPersonIDURL:     "https://oss.example.com/id-front.jpg",
		BiometricLivenessURL: "https://oss.example.com/liveness.mp4",
		ConsentSensitivePIID: 1,
		ConsentBiometricID:   2,
	}
}

func TestSubmit_Happy(t *testing.T) {
	svc, _, _ := newSvc(t)
	a, err := svc.Submit(context.Background(), goodInput(1, "11010119900101001X"))
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != StatusSubmitted || a.LegalPersonIDBlindIndex == "" {
		t.Fatalf("submit wrong: %+v", a)
	}
	if a.LegalPersonIDKeyID == "" {
		t.Fatal("expected encrypted id key")
	}
	if a.ColdArchiveExpiresAt == nil {
		t.Fatal("expected cold archive expiry set")
	}
}

func TestSubmit_DuplicateLegalIDBlocked(t *testing.T) {
	svc, _, _ := newSvc(t)
	if _, err := svc.Submit(context.Background(), goodInput(1, "ID-DUP")); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Submit(context.Background(), goodInput(2, "ID-DUP"))
	if err == nil {
		t.Fatal("expected duplicate id error")
	}
}

func TestSubmit_ConsentMissing(t *testing.T) {
	svc, _, _ := newSvc(t)
	in := goodInput(1, "ID")
	in.ConsentSensitivePIID = 0
	if _, err := svc.Submit(context.Background(), in); !errors.Is(err, ErrConsentRequired) {
		t.Fatalf("expected ErrConsentRequired got %v", err)
	}
}

func TestReview_ApproveLinksPartner(t *testing.T) {
	svc, _, link := newSvc(t)
	a, _ := svc.Submit(context.Background(), goodInput(7, "ID7"))
	if _, err := svc.MarkUnderReview(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	upd, err := svc.Review(context.Background(), a.ID, ApprovalInput{StaffID: 99, Approve: true})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Status != StatusApproved {
		t.Fatalf("expected approved got %s", upd.Status)
	}
	if got, ok := link.Called(7); !ok || got != TypeIndividual {
		t.Fatalf("linker not called or wrong type: %v %v", got, ok)
	}
}

func TestReview_YearlyLimitFreezes(t *testing.T) {
	svc, repo, _ := newSvc(t)
	a, _ := svc.Submit(context.Background(), goodInput(9, "ID9"))
	if _, err := svc.MarkUnderReview(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Update(context.Background(), a.ID, func(x domain.KYCApplication) domain.KYCApplication {
		x.YearlyRejectCount = 2
		return x
	}); err != nil {
		t.Fatal(err)
	}
	upd, err := svc.Review(context.Background(), a.ID, ApprovalInput{
		StaffID: 1, Approve: false, RejectReasonCode: "blurred", RejectReasonText: "again",
	})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Status != StatusFrozenYrly {
		t.Fatalf("expected frozen got %s", upd.Status)
	}
}

func TestPurgeCold(t *testing.T) {
	svc, repo, _ := newSvc(t)
	expired := svc.clock().Add(-1 * time.Hour)
	repo.rows[1] = domain.KYCApplication{
		ID: 1, FyUserID: 1, Status: StatusApproved,
		ColdArchiveExpiresAt: &expired,
	}
	repo.nextID = 1
	n, err := svc.PurgeCold(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 purged got %d", n)
	}
}
