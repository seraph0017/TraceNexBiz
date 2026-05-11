// Package idempotency 提供 idempotency_record CRUD（W0 scaffold；W1a 落 GORM 实现）.
//
// 关键 invariant（与 backend §8.1 v0.2.2 ADR-003 一致）：
//   1. middleware 仅 SELECT（命中 / 重放 / 透传），不写入
//   2. service 在 bizDB.Transaction 闭包内调 Insert(tx, record)
//   3. CI analyzer 校验 internal/idempotency/middleware.go 不出现 repo.Insert 字面（grep -F 反向断言）.
//   4. cleanup cron `0 */6 * * *` purge expired (backend §8.3) — DELETE FROM idempotency_record WHERE expires_at < NOW() LIMIT 10000.
package idempotency

import (
	"context"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// Repo idempotency_record 抽象（与 internal/middleware.IdemRepoReader 解耦，避免循环 import）。
type Repo interface {
	Find(ctx context.Context, actorType string, actorID int64, key, endpoint string) (*domain.IdempotencyRecord, error)
	// Insert 必须在业务 TX 内调（service layer responsibility，参 backend §8.1 v0.2.2）.
	Insert(tx *gorm.DB, rec *domain.IdempotencyRecord) error
}

// PurgeExpired cron `0 */6 * * *` 入口（backend §8.3）.
func PurgeExpired(ctx context.Context, db *gorm.DB) error {
	// TODO(W1a): DELETE FROM idempotency_record WHERE expires_at < NOW() LIMIT 10000 循环.
	return nil
}
