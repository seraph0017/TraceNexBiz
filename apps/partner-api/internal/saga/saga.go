// Package saga 是 partner-api saga 编排器（W1b 完整化）.
//
// 设计参考：backend §4.5 / §8.17，integration §4。
//
// 核心 invariant：
//  1. saga_id 使用 UUIDv7（不是 BIGINT）— v0.2.1 ARCH-HIGH-NEW-D 纠正
//  2. (saga_id, step_name) UNIQUE → 同 step 重入返回已有结果（幂等）
//  3. saga 步骤推进必须显式调 Run / Compensate，不暴露 in-place mutate
//  4. payload 写入前必经 PII scrubber（W1a 提供）
//  5. attempts ≥ 30 || wall-clock ≥ 1h → escalated；不再自动 retry
//
// W1b 范围：
//   - Saga / Steps / Compensate / IdempotencyKey 接口
//   - SagaScheduler 推进 / 补偿 / DLQ
//   - dual-control force-resolve（30min cooldown + 不同人 + IP /24 + 一次性 token）
package saga

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// Status saga step 状态枚举（与 chk_saga_status DDL 对齐）。
type Status string

const (
	StatusPending             Status = "pending"
	StatusInProgress          Status = "in_progress"
	StatusCommitted           Status = "committed"
	StatusCompensated         Status = "compensated"
	StatusFailed              Status = "failed"
	StatusEscalated           Status = "escalated"
	StatusReleasedPessimistic Status = "released_pessimistic"
)

// Kind saga 类型（用于 metrics / DLQ partition）。
type Kind string

const (
	KindWalletAllocate   Kind = "wallet.allocate"
	KindCustomerTopup    Kind = "customer.topup"
	KindCustomerRefund   Kind = "customer.refund"
	KindBillingDispute   Kind = "billing.dispute"
	KindSettlementPayout Kind = "settlement.payout"
	KindOutboxRevenue    Kind = "outbox.revenue"
)

// 重要常量（per backend §4.5 / Q-A retry policy）。
const (
	MaxAttempts          = 30
	MaxWallClock         = 1 * time.Hour
	RetrySweepInterval   = 30 * time.Second
	BackoffBase          = 2 * time.Second
	BackoffMax           = 5 * time.Minute
	ForceResolveCooldown = 30 * time.Minute
)

// 错误：稳定 sentinel，service 层可类型断言。
var (
	ErrStepAlreadyCommitted = errors.New("saga: step already committed")
	ErrStepNotFound         = errors.New("saga: step not found")
	ErrSagaUnknown          = errors.New("saga: status unknown, retry worker will probe")
	ErrSagaEscalated        = errors.New("saga: escalated, awaiting dual-control force-resolve")
	ErrInvalidTransition    = errors.New("saga: invalid status transition")
)

// TxFn 业务事务函数；返回 any 由 saga 上下文捕获（未持久化，仅给调用方）。
type TxFn func(tx *gorm.DB) (any, error)

// CompensateFn 补偿函数；与 TxFn 同签名但语义反向。
type CompensateFn = TxFn

// Saga 单次 saga 实例（不可被调用方 mutate；通过 Run / Commit / Compensate 状态变更）。
type Saga interface {
	// ID UUIDv7；同时是 idempotency_key。
	ID() string
	// Kind saga 类别。
	Kind() Kind
	// Run 推进一个业务步骤；committed 后再次调用直接返回已有结果（幂等）。
	Run(ctx context.Context, step string, fn TxFn) (any, error)
	// Compensate 触发某 step 的补偿；不可对未 committed 的 step 调用。
	Compensate(ctx context.Context, step string, fn CompensateFn) (any, error)
	// Steps 当前所有 step 快照（深拷贝；调用方 mutate 不影响内部）。
	Steps(ctx context.Context) ([]domain.SagaStep, error)
}

// Orchestrator saga 工厂 + retry sweep driver。
type Orchestrator interface {
	// NewSaga 创建新 saga；idemKey 由调用方提供（必须 UUIDv7）；如已存在 → 返回挂载实例。
	NewSaga(idemKey string, kind Kind) (Saga, error)
	// Resume 用已有 saga_id 挂载；用于 retry worker。
	Resume(sagaID string) (Saga, error)
}

// Repository saga_step 持久化抽象（与 internal/repository.SagaRepository 对齐）。
type Repository interface {
	GetByID(ctx context.Context, sagaID string) ([]*domain.SagaStep, error)
	GetStep(ctx context.Context, sagaID, stepName string) (*domain.SagaStep, error)
	// Save UPSERT by (saga_id, step_name)；committed step 不可再被改写。
	Save(ctx context.Context, step *domain.SagaStep) error
	// LoadPendingRetries 拉 in_progress / failed 且未 escalated 的 step（按 attempts ASC）。
	LoadPendingRetries(ctx context.Context, limit int) ([]*domain.SagaStep, error)
	// MarkEscalated attempts ≥ 30 || wall-clock ≥ 1h。
	MarkEscalated(ctx context.Context, sagaID, stepName, reason string) error
	// ForceResolve dual-control 落库（写 audit 由调用方）。
	ForceResolve(ctx context.Context, sagaID, stepName string, target Status) error
}

// PIIScrubber payload 写入前过滤（W1a 提供）；W1b 仅依赖接口。
type PIIScrubber interface {
	Scrub(s string) string
}

// NewSagaID 生成 UUIDv7 saga_id。
//
// 选择 v7：
//   - lexicographic-sortable 利于 saga_step 索引顺序写
//   - 单调时间戳便于 retry worker 按时间分区
//   - 跟 v0.2.1 ARCH-HIGH-NEW-D 决议一致
func NewSagaID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// MustNewSagaID 不可恢复 panic（仅启动期使用；service 层用 NewSagaID）。
func MustNewSagaID() string {
	id, err := NewSagaID()
	if err != nil {
		panic("saga: NewV7 failed: " + err.Error())
	}
	return id
}

// IsValidUUIDv7 校验输入字符串是否为合法 UUIDv7。
func IsValidUUIDv7(s string) bool {
	id, err := uuid.Parse(s)
	if err != nil {
		return false
	}
	return id.Version() == 7
}

// BackoffFor 根据 attempts 计算下次 retry 延迟（指数 + 上限 5min）。
//
// 表：1=2s, 2=4s, 3=8s, ... 上限 5min；attempts ≥ 9 后每次都 5min。
func BackoffFor(attempts int) time.Duration {
	if attempts <= 1 {
		return BackoffBase
	}
	d := BackoffBase
	for i := 1; i < attempts && d < BackoffMax; i++ {
		d *= 2
	}
	if d > BackoffMax {
		d = BackoffMax
	}
	return d
}

// ShouldEscalate attempts 或 wall-clock 越界 → escalated。
func ShouldEscalate(step *domain.SagaStep, now time.Time) (bool, string) {
	if step == nil {
		return false, ""
	}
	if step.Attempts >= MaxAttempts {
		return true, "max_attempts_exceeded"
	}
	if step.StartedAt != nil && now.Sub(*step.StartedAt) >= MaxWallClock {
		return true, "max_wallclock_exceeded"
	}
	return false, ""
}
