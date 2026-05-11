package audit

import (
	"context"
	"errors"
	"testing"
	"time"
)

func enqueueN(s *MemoryStore, n int) {
	for i := 0; i < n; i++ {
		s.EnqueueUnsealed(UnsealedRow{
			ActorType:    "staff",
			ActorID:      int64(100 + i),
			Action:       "wallet.adjust",
			TargetType:   "partner_wallet",
			TargetID:     int64(i + 1),
			DiffRedacted: "{}",
			OccurredAt:   time.Unix(1700000000+int64(i), 0).UTC(),
		})
	}
}

func TestSealOnce_AppendsAndDeletesUnsealed(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	enqueueN(store, 5)
	sealer := NewSealer(store, AlwaysLeader{})
	n, err := sealer.SealOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("sealed = %d", n)
	}
	if store.UnsealedCount() != 0 {
		t.Fatalf("unsealed left: %d", store.UnsealedCount())
	}
	if store.SealedCount() != 5 {
		t.Fatalf("sealed total: %d", store.SealedCount())
	}
}

func TestVerify_HappyPath(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	enqueueN(store, 10)
	sealer := NewSealer(store, AlwaysLeader{})
	_, err := sealer.SealOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(context.Background(), store, 0); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestVerify_DetectsTampering(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	enqueueN(store, 5)
	sealer := NewSealer(store, AlwaysLeader{})
	_, _ = sealer.SealOnce(context.Background())
	store.Tamper(2, func(r *SealedRow) { r.DiffRedacted = `{"hacked":true}` })
	err := Verify(context.Background(), store, 0)
	if !errors.Is(err, ErrChainBroken) {
		t.Fatalf("expected ErrChainBroken, got %v", err)
	}
}

func TestSealer_BatchedAcrossRuns(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	enqueueN(store, 250)
	sealer := NewSealer(store, AlwaysLeader{})
	sealer.SetBatch(100)
	total := 0
	for i := 0; i < 5; i++ {
		n, err := sealer.SealOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		total += n
	}
	if total != 250 {
		t.Fatalf("total = %d", total)
	}
	if err := Verify(context.Background(), store, 0); err != nil {
		t.Fatalf("verify after batched: %v", err)
	}
}

func TestSealer_RefusesNonLeader(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	sealer := NewSealer(store, refusedLeader{})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := sealer.Run(ctx); err == nil {
		t.Fatal("expected error refusing non-leader")
	}
}

type refusedLeader struct{}

func (refusedLeader) Acquire(ctx context.Context) (bool, error) { return false, nil }
func (refusedLeader) Renew(ctx context.Context) error           { return nil }
func (refusedLeader) Release(ctx context.Context) error         { return nil }
