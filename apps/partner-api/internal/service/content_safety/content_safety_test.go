package content_safety

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRecordEvent_BlockAutoQueuesReport(t *testing.T) {
	t.Parallel()
	repo := NewMemoryRepo()
	ac := &CapturingAuthorityClient{}
	svc := NewService(repo, ac)
	_, err := svc.RecordEvent(context.Background(), Event{
		FyUserID: 7, Kind: "input", Provider: "aliyun",
		PromptHash: "abc", Category: "violence", Score: 0.92, Disposition: "block",
	})
	if err != nil {
		t.Fatal(err)
	}
	reports, _, _ := svc.ListReports(context.Background(), ListQuery{})
	if len(reports) != 1 || reports[0].TargetAuthority != "12377" {
		t.Fatalf("expected 12377 report; got %+v", reports)
	}
}

func TestRecordEvent_PassDoesNotQueue(t *testing.T) {
	t.Parallel()
	repo := NewMemoryRepo()
	svc := NewService(repo, &CapturingAuthorityClient{})
	_, err := svc.RecordEvent(context.Background(), Event{
		FyUserID: 1, Disposition: "pass",
	})
	if err != nil {
		t.Fatal(err)
	}
	reports, _, _ := svc.ListReports(context.Background(), ListQuery{})
	if len(reports) != 0 {
		t.Fatalf("expected 0 reports; got %d", len(reports))
	}
}

func TestRecordEvent_InvalidDisposition(t *testing.T) {
	t.Parallel()
	svc := NewService(NewMemoryRepo(), &CapturingAuthorityClient{})
	_, err := svc.RecordEvent(context.Background(), Event{Disposition: "yeet"})
	if !errors.Is(err, ErrInvalidDisposition) {
		t.Fatalf("got %v", err)
	}
}

func TestDispatchOnce_HappyPath(t *testing.T) {
	t.Parallel()
	repo := NewMemoryRepo()
	ac := &CapturingAuthorityClient{}
	svc := NewService(repo, ac)
	_, _ = svc.RecordEvent(context.Background(), Event{FyUserID: 1, Disposition: "block"})
	sub, fail, err := svc.DispatchOnce(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if sub != 1 || fail != 0 {
		t.Fatalf("sub=%d fail=%d", sub, fail)
	}
	if len(ac.Calls) != 1 {
		t.Fatalf("authority calls = %d", len(ac.Calls))
	}
}

func TestDispatchOnce_RetryThenDeadLetter(t *testing.T) {
	t.Parallel()
	repo := NewMemoryRepo()
	ac := &CapturingAuthorityClient{FailNext: MaxRetries}
	svc := NewService(repo, ac)
	_, _ = svc.RecordEvent(context.Background(), Event{FyUserID: 1, Disposition: "block"})
	for i := 0; i < MaxRetries; i++ {
		_, _, _ = svc.DispatchOnce(context.Background(), 10)
	}
	reports, _, _ := svc.ListReports(context.Background(), ListQuery{})
	if reports[0].Status != "dead_letter" {
		t.Fatalf("expected dead_letter, got %s (retry=%d)", reports[0].Status, reports[0].RetryCount)
	}
}

func TestSLABreaches_DetectsOverdue(t *testing.T) {
	t.Parallel()
	repo := NewMemoryRepo()
	svc := NewService(repo, &CapturingAuthorityClient{})
	svc.clock = func() time.Time { return time.Now().Add(-30 * time.Hour) }
	_, _ = svc.RecordEvent(context.Background(), Event{FyUserID: 1, Disposition: "block"})
	svc.clock = time.Now
	breaches, err := svc.SLABreaches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(breaches) != 1 {
		t.Fatalf("expected 1 breach; got %d", len(breaches))
	}
}

func TestRetryReport_ResetsDeadLetter(t *testing.T) {
	t.Parallel()
	repo := NewMemoryRepo()
	svc := NewService(repo, &CapturingAuthorityClient{FailNext: MaxRetries})
	_, _ = svc.RecordEvent(context.Background(), Event{FyUserID: 1, Disposition: "block"})
	for i := 0; i < MaxRetries; i++ {
		_, _, _ = svc.DispatchOnce(context.Background(), 10)
	}
	reports, _, _ := svc.ListReports(context.Background(), ListQuery{})
	rep, err := svc.RetryReport(context.Background(), reports[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "pending" {
		t.Fatalf("status %s", rep.Status)
	}
}

func TestAdminReview_StampsReviewer(t *testing.T) {
	t.Parallel()
	repo := NewMemoryRepo()
	svc := NewService(repo, &CapturingAuthorityClient{})
	e, _ := svc.RecordEvent(context.Background(), Event{FyUserID: 7, Disposition: "review"})
	updated, err := svc.AdminReview(context.Background(), e.ID, "block", 99)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Disposition != "block" || updated.ReviewedBy == nil || *updated.ReviewedBy != 99 {
		t.Fatalf("%+v", updated)
	}
}
