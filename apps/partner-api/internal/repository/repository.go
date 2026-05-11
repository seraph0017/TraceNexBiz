// Package repository 是 partner_db 数据访问层（接口层；W0 scaffold）。
//
// 设计原则（与 Repository Pattern + 用户全局规则一致）：
//   1. 接口位于 internal/repository/<entity>.go；GORM 实现位于 internal/repository/mysql/<entity>_mysql.go（W1a 落地）
//   2. 每个公共方法首参 `ctx context.Context`；scope 类方法第二参为 partner_id / customer_id / staff_id（BOLA 强制）
//   3. 返回 domain.* 而非 GORM model；entity 不可被调用方 mutate（immutability）
//   4. 写操作走 Update(ctx, id, updater func(domain.X) domain.X) 模式；不暴露 Save(*X) 防误用
//   5. 事务通过 WithTx(tx *gorm.DB) Repository 包装；不在 repository 内开事务
//
// 各 repository 接口由 W1 各 agent 在自己的 entity 接口下增补具体方法签名（例：Find/List/Create/Update/Soft-delete）。
package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// ErrNotFound entity 不存在；上层映射为 BIZ_RES_NOT_FOUND（不暴露存在性，PRD §16.3）。
var ErrNotFound = errors.New("repository: not found")

// Tx 抽象供 service 把多个 repository 操作组合到同一 GORM 事务（backend §8.1 v0.2.2）.
type Tx interface {
	DB() *gorm.DB
}

// PartnerRepository PRD §8.1。W1a 增补具体方法。
type PartnerRepository interface {
	// FindByID 根据主键查；不存在返 ErrNotFound（service 层映射为 BIZ_RES_NOT_FOUND）。
	FindByID(ctx context.Context, id int64) (*domain.Partner, error)

	// FindByFyUserID 用 fy_user_id 反查 partner（outbox poller 用）。
	FindByFyUserID(ctx context.Context, fyUserID int64) (*domain.Partner, error)

	// TODO(W1a/W1b): per backend §3.1 — Create / Update(updater) / SoftDelete / List(filter) / Search(emailHMAC).
}

// CustomerRepository PRD §8.2 + §8.20 BOLA scope by partner_id。
type CustomerRepository interface {
	// FindByIDForPartner 强制 scope：partner_id 不匹配 → ErrNotFound（BOLA last line of defense）。
	FindByIDForPartner(ctx context.Context, partnerID, customerID int64) (*domain.Customer, error)

	FindByFyUserID(ctx context.Context, fyUserID int64) (*domain.Customer, error)

	// TODO(W1a/W1b): Create / Update / Transfer (saga-coordinated) / List(partner_id, filter)
}

// WalletRepository PRD §8.3 / §8.4 / §8.5（hold + log + balance）.
type WalletRepository interface {
	FindByPartner(ctx context.Context, partnerID int64) (*domain.PartnerWallet, error)

	// TODO(W1a/W1b): per backend §5.3 §8.1 v0.2.2 — Allocate（saga 三阶段，hold/commit/release）.
}

// PricingRepository PRD §8.6.
type PricingRepository interface {
	// TODO(W1a/W1b): Create / Resolve(at-occurrence) / OverlapCheck.
}

// RevenueRepository PRD §8.7.
type RevenueRepository interface {
	// TODO(W1a/W1b): per integration §3.3 — Insert (UNIQUE log_id+occurrence) / Aggregate / SettlementMark.
}

// SettlementRepository PRD §8.8.
type SettlementRepository interface {
	// TODO(W1c): per backend §5.5 — settlement runner reentrant; FreshnessGate.
}

// KYCRepository PRD §8.9.
type KYCRepository interface {
	// TODO(W1a/W1b): per backend §5.6 — submit/review/approve/reject/yearly_reject_count.
}

// AuditRepository backend §3.13.
type AuditRepository interface {
	// TODO(W1a): per backend §10 — EnqueueUnsealed / SealBatch (sealer leader-only) / Verify (CLI).
}

// IdempotencyRepository backend §3.16 / §8.1 v0.2.2.
type IdempotencyRepository interface {
	Find(ctx context.Context, actorType string, actorID int64, key, endpoint string) (*domain.IdempotencyRecord, error)
	// Insert 由 service 在业务 TX 内部调用；签名带 tx 参数。
	// TODO(W1a): Insert(tx Tx, rec *domain.IdempotencyRecord) error.
}

// SagaRepository backend §3.17.
type SagaRepository interface {
	// TODO(W1a): per backend §8.2 — Get(id) / Save(step) / LoadPendingRetries / Escalate / ForceResolve.
}

// StaffRepository PRD §8.14.
type StaffRepository interface {
	// TODO(W1c): FindByUsername / VerifyPassword / SetMFA / SetElevatedUntil.
}

// BizSettingRepository PRD §8.15 + ARCH D-3.
type BizSettingRepository interface {
	// TODO(W1a): Get(key) / List / Update（仅 trivial verb 走此 path；security verb 拆 /system/security/*).
}
