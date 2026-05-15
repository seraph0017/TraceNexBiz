package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/content_safety"
)

func TestContentSafetyRepository_DispatchFlow(t *testing.T) {
	db := NewTestDB(t)
	repo := NewContentSafetyRepository(db)
	ctx := context.Background()

	eventID, err := repo.InsertEvent(ctx, &content_safety.Event{
		FyUserID:    42,
		Kind:        "input",
		Provider:    "mock",
		PromptHash:  "hash",
		Category:    "illegal",
		Score:       0.9,
		Disposition: "block",
		TraceID:     "tr",
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
	reportID, err := repo.InsertReport(ctx, &content_safety.Report{
		EventID:         eventID,
		TargetAuthority: "12377",
		Payload:         `{"x":1}`,
		Status:          "pending",
		SLADueAt:        time.Now().UTC().Add(time.Hour),
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("InsertReport: %v", err)
	}
	pending, err := repo.ListPendingReports(ctx, 10)
	if err != nil {
		t.Fatalf("ListPendingReports: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != reportID {
		t.Fatalf("pending=%+v reportID=%d", pending, reportID)
	}
	now := time.Now().UTC()
	if _, err := repo.UpdateReport(ctx, reportID, func(r content_safety.Report) content_safety.Report {
		r.Status = "submitted"
		r.SubmittedAt = &now
		r.ResponsePayload = `{"ack":true}`
		return r
	}); err != nil {
		t.Fatalf("UpdateReport: %v", err)
	}
	pending, err = repo.ListPendingReports(ctx, 10)
	if err != nil {
		t.Fatalf("ListPendingReports after submit: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending after submit: %+v", pending)
	}
}
