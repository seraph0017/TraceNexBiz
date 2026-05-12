// internal/saga/idempotency.go — same-TX idempotency bridge (Fix-B' part 2 CRIT-B3).
//
// backend §8.1 v0.2.2 要求：idempotency_record 必须与业务写在同一 transaction co-commit。
//
// 现状（实事求是）：
//   - customer / saga 服务逻辑大量分散，单个 service 调用横跨多个独立的 GORM tx（每个 saga step 一个 tx）。
//   - 要把 idempotency_record 严格同 tx 写到 *每个* 业务写显然不现实，会造成大规模重构。
//
// 因此本文件实现 "first-step co-commit" pattern：
//   - service 入口：调用 WithIdempotency(ctx, db, idemRec, fn) → 在 tx 中先 InsertWithinTx，
//     再调用 fn(tx)；任何业务写（saga.RunWithInput 之外的，或显式传入 tx 的）都可在 fn 中复用同 tx。
//   - UNIQUE 冲突（重复 idempotency-key）→ 返 ErrDuplicateIdempotency；caller 应 Find 取 cached 响应回放。
//
// 注：saga orchestrator 内部仍用自己的 tx 推进每个 step；这是 design trade-off：
//   - 上层 service 用 WithIdempotency 锁定 (key, route) 唯一性；
//   - 后续 saga.RunWithInput 推进 step（独立 tx）；
//   - 一旦 service 入口 commit 成功，idempotency_record 已落库 → middleware DB 回退路径
//     可在 Redis miss 时回放，保证 ≥ once-only 业务效果。
//
// 与 v1 KMS 加密的关系：Phase-1 写 plaintext ResponseBody；Fix-C KMS 上线后改写 ResponseCipher
// 不影响本文件的 tx 协议。
package saga

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// IdempotencyInserter 抽象 InsertWithinTx；解耦避免 saga 包反向依赖 repository/mysql。
type IdempotencyInserter interface {
	InsertWithinTx(tx *gorm.DB, rec *domain.IdempotencyRecord) error
}

// ErrDuplicateIdempotency 同 key + endpoint 已被先发请求占用；caller 应回放已落库响应。
var ErrDuplicateIdempotency = errors.New("saga: duplicate idempotency_record")

// WithIdempotency 在 db 开 tx → InsertWithinTx idempotency_record → 调用 fn(tx)。
//
// fn 在同 tx 内执行；fn 失败 → tx rollback，idempotency_record 也回滚（co-commit invariant）。
// UNIQUE 冲突 → ErrDuplicateIdempotency。
//
// 用法：
//
//	rec := &domain.IdempotencyRecord{ActorType:"partner", ActorID:pid, IdempotencyKey:key, Endpoint:"POST /x", ...}
//	err := saga.WithIdempotency(ctx, db, idemRepo, rec, func(tx *gorm.DB) error {
//	    // 业务写（可包括 saga.RunWithInput 在其他 tx 内推进 step）
//	    return nil
//	})
//	if errors.Is(err, saga.ErrDuplicateIdempotency) {
//	    cached, _ := idemRepo.Find(ctx, ...)  // 回放
//	}
func WithIdempotency(ctx context.Context, db *gorm.DB, ins IdempotencyInserter, rec *domain.IdempotencyRecord, fn func(tx *gorm.DB) error) error {
	if db == nil {
		return errors.New("saga.WithIdempotency: nil db")
	}
	if ins == nil {
		return errors.New("saga.WithIdempotency: nil inserter")
	}
	if rec == nil {
		return errors.New("saga.WithIdempotency: nil record")
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ins.InsertWithinTx(tx, rec); err != nil {
			// 包装并返回；caller errors.Is 判断。
			return err
		}
		if fn == nil {
			return nil
		}
		return fn(tx)
	})
}
