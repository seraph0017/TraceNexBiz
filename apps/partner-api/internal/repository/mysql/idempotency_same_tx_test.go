// idempotency_same_tx_test.go — Fix-B' part 2 CRIT-B3：service-level same-TX co-commit verification.
//
// 模拟 service 层用 saga.WithIdempotency 包业务写：
//   - 两个并发 caller 用同 idempotency-key → 只有一个 fn 跑过；
//   - 业务 fn 失败 → idempotency_record 也回滚（同 tx invariant）。
package mysql

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/saga"
)

// Adapter 让 GORM IdempotencyRepository 满足 saga.IdempotencyInserter（不增加跨包耦合）。
type sagaInsAdapter struct{ inner *IdempotencyRepository }

func (a *sagaInsAdapter) InsertWithinTx(tx *gorm.DB, rec *domain.IdempotencyRecord) error {
	return a.inner.InsertWithinTx(tx, rec)
}

func TestSameTx_ConcurrentCallers_OnlyOneFnRuns(t *testing.T) {
	db := NewTestDB(t)
	repo := NewIdempotencyRepository(db)
	ins := &sagaInsAdapter{inner: repo}

	const n = 6
	var fnCalls int32
	mkRec := func() *domain.IdempotencyRecord {
		return &domain.IdempotencyRecord{
			ActorType: "partner", ActorID: 1,
			IdempotencyKey: "same-tx-key",
			Endpoint:       "POST /wallet/allocate",
			RequestHash:    "abc",
			ResponseStatus: 200,
			ResponseBody:   `{"ok":true}`,
			ExpiresAt:      time.Now().Add(24 * time.Hour),
		}
	}

	var wg sync.WaitGroup
	wg.Add(n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			err := saga.WithIdempotency(context.Background(), db, ins, mkRec(), func(tx *gorm.DB) error {
				atomic.AddInt32(&fnCalls, 1)
				return nil
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	var success, dup int
	for e := range errs {
		switch {
		case e == nil:
			success++
		case errors.Is(e, repository.ErrDuplicateKey):
			dup++
		default:
			t.Logf("other err (sqlite contention): %v", e)
		}
	}
	if success != 1 {
		t.Fatalf("expected exactly 1 success, got success=%d dup=%d", success, dup)
	}
	if got := atomic.LoadInt32(&fnCalls); got != 1 {
		t.Fatalf("fn must run exactly once for the winner, got %d", got)
	}

	// 第二阶段：所有失败方现在可以 Find 出已落库 idempotency_record（回放路径）。
	got, err := repo.Find(context.Background(), "partner", 1, "same-tx-key", "POST /wallet/allocate")
	if err != nil {
		t.Fatalf("Find after concurrent: %v", err)
	}
	if got.ResponseStatus != 200 {
		t.Fatalf("Find replay payload mismatch: %+v", got)
	}
}

func TestSameTx_BusinessFnError_RollsBackIdempotencyRecord(t *testing.T) {
	db := NewTestDB(t)
	repo := NewIdempotencyRepository(db)
	ins := &sagaInsAdapter{inner: repo}

	rec := &domain.IdempotencyRecord{
		ActorType: "partner", ActorID: 1,
		IdempotencyKey: "rollback-key",
		Endpoint:       "POST /x",
		ExpiresAt:      time.Now().Add(24 * time.Hour),
	}
	bizErr := errors.New("biz write boom")
	err := saga.WithIdempotency(context.Background(), db, ins, rec, func(tx *gorm.DB) error {
		return bizErr
	})
	if !errors.Is(err, bizErr) {
		t.Fatalf("expected biz err, got %v", err)
	}
	// 回放路径：idempotency_record 不应留下（co-commit 回滚 invariant）。
	_, err = repo.Find(context.Background(), "partner", 1, "rollback-key", "POST /x")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected NotFound, got %v", err)
	}
}
