// idempotency_mysql_test.go — Fix-B' part 2 CRIT-B3 repository contract.
package mysql

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository"
)

func newIdemRec(key string) *domain.IdempotencyRecord {
	return &domain.IdempotencyRecord{
		ActorType:      "partner",
		ActorID:        7,
		IdempotencyKey: key,
		Endpoint:       "POST /partner/wallet/allocate",
		RequestHash:    "deadbeef",
		ResponseStatus: 200,
		ResponseHash:   "cafef00d",
		ResponseBody:   `{"ok":true}`,
		TraceID:        "trace-1",
		ExpiresAt:      time.Now().Add(24 * time.Hour),
	}
}

func TestIdempotency_InsertAndFind(t *testing.T) {
	db := NewTestDB(t)
	repo := NewIdempotencyRepository(db)
	ctx := context.Background()

	rec := newIdemRec("idem-1")
	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.InsertWithinTx(tx, rec)
	})
	if err != nil {
		t.Fatalf("InsertWithinTx: %v", err)
	}
	if rec.ID == 0 {
		t.Fatalf("expected ID populated after insert")
	}

	got, err := repo.Find(ctx, "partner", 7, "idem-1", "POST /partner/wallet/allocate")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.ResponseStatus != 200 || got.ResponseBody != `{"ok":true}` {
		t.Fatalf("Find mismatch: %+v", got)
	}
}

func TestIdempotency_FindMissingReturnsNotFound(t *testing.T) {
	db := NewTestDB(t)
	repo := NewIdempotencyRepository(db)
	_, err := repo.Find(context.Background(), "partner", 99, "nope", "POST /x")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected gorm.ErrRecordNotFound, got %v", err)
	}
}

func TestIdempotency_DuplicateInsertReturnsErrDuplicate(t *testing.T) {
	db := NewTestDB(t)
	repo := NewIdempotencyRepository(db)

	rec1 := newIdemRec("idem-dup")
	if err := db.Transaction(func(tx *gorm.DB) error {
		return repo.InsertWithinTx(tx, rec1)
	}); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	rec2 := newIdemRec("idem-dup")
	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.InsertWithinTx(tx, rec2)
	})
	if !errors.Is(err, repository.ErrDuplicateKey) {
		t.Fatalf("expected ErrDuplicateKey, got %v", err)
	}
}

// TestIdempotency_SameTX_RolledBackOnBusinessFailure 验证 "co-commit" 语义：
// 业务 fn 在 tx 内 InsertWithinTx idempotency_record 后报错，整个 tx 必须回滚，
// idempotency_record 不应留下 — 否则 retry 会被错误 replay 成 "已成功"。
func TestIdempotency_SameTX_RolledBackOnBusinessFailure(t *testing.T) {
	db := NewTestDB(t)
	repo := NewIdempotencyRepository(db)
	ctx := context.Background()

	rec := newIdemRec("idem-rollback")
	bizErr := errors.New("business write failed")
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := repo.InsertWithinTx(tx, rec); err != nil {
			return err
		}
		// 模拟业务 fn 失败：返回 err → tx rollback。
		return bizErr
	})
	if !errors.Is(err, bizErr) {
		t.Fatalf("expected business error, got %v", err)
	}

	// idempotency_record 不应存在（co-commit 回滚）。
	_, err = repo.Find(ctx, "partner", 7, "idem-rollback", "POST /partner/wallet/allocate")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected NotFound after rollback, got %v", err)
	}
}

// TestIdempotency_ConcurrentSameKey 验证两并发 caller 用同 idempotency-key：
// 只有一方成功 Insert；另一方拿到 ErrDuplicateKey 并应触发 replay 路径。
func TestIdempotency_ConcurrentSameKey(t *testing.T) {
	db := NewTestDB(t)
	repo := NewIdempotencyRepository(db)

	const n = 8
	var wg sync.WaitGroup
	wg.Add(n)
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			rec := newIdemRec("idem-concurrent")
			err := db.Transaction(func(tx *gorm.DB) error {
				return repo.InsertWithinTx(tx, rec)
			})
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	var success, dup int
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, repository.ErrDuplicateKey):
			dup++
		default:
			// SQLite in test 可能返回 "database is locked"；非 unique-violation，
			// 不计入两类预期。
			t.Logf("unexpected concurrent err (sqlite contention): %v", err)
		}
	}
	if success < 1 {
		t.Fatalf("expected at least 1 success, got %d (dup=%d)", success, dup)
	}
	if success > 1 {
		t.Fatalf("expected exactly 1 success, got %d", success)
	}
}
