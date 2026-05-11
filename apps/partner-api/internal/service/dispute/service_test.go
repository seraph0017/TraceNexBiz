package dispute

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/saga"
)

type memRepo struct {
	mu    sync.Mutex
	rows  map[int64]Dispute
	seq   int64
}

func newRepo() *memRepo { return &memRepo{rows: make(map[int64]Dispute)} }

func (r *memRepo) Create(_ context.Context, d Dispute) (Dispute, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	d.ID = r.seq
	r.rows[d.ID] = d
	return d, nil
}

func (r *memRepo) FindByID(_ context.Context, id int64) (*Dispute, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.rows[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := d
	return &cp, nil
}

func (r *memRepo) UpdateStatus(_ context.Context, id int64, from, to Status, reviewer int64, refundID string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.rows[id]
	if !ok {
		return ErrNotFound
	}
	if d.Status != from {
		return ErrInvalidTransition
	}
	d.Status = to
	d.ReviewerID = &reviewer
	d.ReviewedAt = &now
	if refundID != "" {
		d.RefundSagaID = refundID
	}
	d.UpdatedAt = now
	r.rows[id] = d
	return nil
}

type memRefund struct{ launched int }

func (m *memRefund) Launch(_ context.Context, _ string, _, _ int64, _ string, _ int64) error {
	m.launched++
	return nil
}

type idsFactory struct{}

func (idsFactory) New() (string, error) { return saga.NewSagaID() }

func TestSubmit_ValidatesInput(t *testing.T) {
	svc := NewService(newRepo(), &memRefund{}, idsFactory{})
	if _, err := svc.Submit(context.Background(), SubmitRequest{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := svc.Submit(context.Background(), SubmitRequest{
		OpenerType: "partner", OpenerID: 1, RevenueLogID: 2, Amount: 10,
	}); !errors.Is(err, ErrEmptyReason) {
		t.Fatal(err)
	}
}

func TestStateMachine_Transitions(t *testing.T) {
	cases := []struct {
		from, to Status
		ok       bool
	}{
		{StatusSubmitted, StatusReviewing, true},
		{StatusReviewing, StatusAccepted, true},
		{StatusReviewing, StatusRejected, true},
		{StatusAccepted, StatusRefunded, true},
		{StatusRefunded, StatusReviewing, false},
		{StatusRejected, StatusAccepted, false},
		{StatusSubmitted, StatusRejected, false},
	}
	for _, c := range cases {
		if got := CanTransition(c.from, c.to); got != c.ok {
			t.Errorf("%s→%s: got %v want %v", c.from, c.to, got, c.ok)
		}
	}
}

func TestFinalizeAccept_LaunchesRefund(t *testing.T) {
	repo := newRepo()
	rf := &memRefund{}
	svc := NewService(repo, rf, idsFactory{})
	d, _ := svc.Submit(context.Background(), SubmitRequest{
		OpenerType: "customer", OpenerID: 1, RevenueLogID: 10, Amount: 100, Reason: "incorrect billing",
	})
	if err := svc.StartReview(context.Background(), d.ID, 99); err != nil {
		t.Fatal(err)
	}
	if err := svc.FinalizeAccept(context.Background(), d.ID, 99, 100); err != nil {
		t.Fatal(err)
	}
	if rf.launched != 1 {
		t.Fatalf("refund launched=%d", rf.launched)
	}
	final, _ := repo.FindByID(context.Background(), d.ID)
	if final.Status != StatusRefunded {
		t.Fatalf("status=%s", final.Status)
	}
	if final.RefundSagaID == "" {
		t.Fatal("expected refund saga id stored")
	}
}

func TestFinalizeReject(t *testing.T) {
	repo := newRepo()
	svc := NewService(repo, &memRefund{}, idsFactory{})
	d, _ := svc.Submit(context.Background(), SubmitRequest{
		OpenerType: "customer", OpenerID: 1, RevenueLogID: 10, Amount: 100, Reason: "issue",
	})
	_ = svc.StartReview(context.Background(), d.ID, 9)
	if err := svc.FinalizeReject(context.Background(), d.ID, 9); err != nil {
		t.Fatal(err)
	}
	final, _ := repo.FindByID(context.Background(), d.ID)
	if final.Status != StatusRejected {
		t.Fatalf("status=%s", final.Status)
	}
}
