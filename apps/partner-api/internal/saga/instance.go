// internal/saga/instance.go — 单 saga 实例（线程安全 + immutable step 快照）.
//
// invariant：
//   - 任何 step.Status mutation 都新建结构体，不修改入参
//   - committed step 再次 Run 直接返回 nil（幂等）
//   - in_progress step 再次 Run 报 ErrInvalidTransition（caller 必须先 Compensate）
package saga

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// Transactor 抽象 bizDB.Transaction(...)；让 saga 包不强依赖 *gorm.DB（测试可注入）.
type Transactor interface {
	WithTx(ctx context.Context, fn TxFn) (any, error)
}

// GormTransactor 包装 *gorm.DB.
type GormTransactor struct{ DB *gorm.DB }

// WithTx 使用 gorm.DB.Transaction；fn 内 panic 由 gorm 自动 rollback.
func (g *GormTransactor) WithTx(ctx context.Context, fn TxFn) (any, error) {
	if g == nil || g.DB == nil {
		return nil, fmt.Errorf("saga: nil gorm db")
	}
	var result any
	err := g.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, e := fn(tx)
		if e != nil {
			return e
		}
		result = r
		return nil
	})
	return result, err
}

// instance 实现 Saga；不暴露字段，仅通过方法访问。
type instance struct {
	id       string
	kind     Kind
	repo     Repository
	tx       Transactor
	scrubber PIIScrubber
	mu       sync.Mutex
}

// newInstance 由 orchestrator 调用。
func newInstance(id string, kind Kind, repo Repository, tx Transactor, scrubber PIIScrubber) *instance {
	return &instance{id: id, kind: kind, repo: repo, tx: tx, scrubber: scrubber}
}

// ID 返回 UUIDv7 saga_id。
func (i *instance) ID() string { return i.id }

// Kind 返回 saga 类别。
func (i *instance) Kind() Kind { return i.kind }

// Steps 返回 step 列表深拷贝。
func (i *instance) Steps(ctx context.Context) ([]domain.SagaStep, error) {
	rows, err := i.repo.GetByID(ctx, i.id)
	if err != nil {
		return nil, fmt.Errorf("saga: load steps for %s: %w", i.id, err)
	}
	out := make([]domain.SagaStep, 0, len(rows))
	for _, r := range rows {
		if r == nil {
			continue
		}
		out = append(out, *r)
	}
	return out, nil
}

// Run 推进 step；committed step 再次调用返回已记录的结果占位（nil, nil）.
//
// 流程：
//  1. SELECT step
//  2. committed → return nil, nil（幂等）
//  3. else → status=in_progress + attempts++ + started_at（首次）
//  4. 在 bizDB.Transaction 中执行 fn
//  5. 成功 → status=committed；失败 → status=failed + last_error（PII scrub）
func (i *instance) Run(ctx context.Context, step string, fn TxFn) (any, error) {
	if step == "" || fn == nil {
		return nil, fmt.Errorf("saga: step name and fn required")
	}
	i.mu.Lock()
	defer i.mu.Unlock()

	existing, err := i.repo.GetStep(ctx, i.id, step)
	if err != nil && err != ErrStepNotFound {
		return nil, fmt.Errorf("saga: get step %s: %w", step, err)
	}

	if existing != nil {
		switch Status(existing.Status) {
		case StatusCommitted:
			return nil, nil
		case StatusEscalated:
			return nil, ErrSagaEscalated
		case StatusCompensated:
			return nil, fmt.Errorf("saga: step %s already compensated: %w", step, ErrInvalidTransition)
		}
	}

	now := time.Now().UTC()
	pending := i.snapshotStarting(existing, step, now)
	if err := i.repo.Save(ctx, &pending); err != nil {
		return nil, fmt.Errorf("saga: mark in_progress: %w", err)
	}

	var result any
	r, txErr := i.tx.WithTx(ctx, fn)
	if txErr == nil {
		result = r
	}
	if txErr != nil {
		failed := i.snapshotFailed(&pending, txErr.Error(), now)
		if saveErr := i.repo.Save(ctx, &failed); saveErr != nil {
			// 即使写库失败，业务错误优先返回；retry worker 会从 in_progress 推进。
			return nil, fmt.Errorf("saga: step %s failed (save also failed: %v): %w", step, saveErr, txErr)
		}
		if escalate, reason := ShouldEscalate(&failed, now); escalate {
			_ = i.repo.MarkEscalated(ctx, i.id, step, reason)
			return nil, fmt.Errorf("saga: step %s escalated (%s): %w", step, reason, ErrSagaEscalated)
		}
		return nil, fmt.Errorf("saga: step %s tx failed: %w", step, txErr)
	}

	committed := i.snapshotCommitted(&pending, now)
	if err := i.repo.Save(ctx, &committed); err != nil {
		// tx 已 commit，必须返回成功 + 非阻断错误（retry worker 会修复 status）
		return result, fmt.Errorf("saga: step %s committed but save status failed: %w", step, err)
	}
	return result, nil
}

// Compensate 触发已 committed step 的补偿。
func (i *instance) Compensate(ctx context.Context, step string, fn CompensateFn) (any, error) {
	if step == "" || fn == nil {
		return nil, fmt.Errorf("saga: step name and fn required")
	}
	i.mu.Lock()
	defer i.mu.Unlock()

	existing, err := i.repo.GetStep(ctx, i.id, step)
	if err != nil {
		return nil, fmt.Errorf("saga: get step %s for compensate: %w", step, err)
	}
	if existing == nil {
		return nil, ErrStepNotFound
	}
	if Status(existing.Status) == StatusCompensated {
		return nil, nil
	}
	if Status(existing.Status) != StatusCommitted {
		return nil, fmt.Errorf("saga: cannot compensate non-committed step %s (status=%s): %w",
			step, existing.Status, ErrInvalidTransition)
	}

	var result any
	r, txErr := i.tx.WithTx(ctx, fn)
	if txErr == nil {
		result = r
	}
	now := time.Now().UTC()
	if txErr != nil {
		failed := i.snapshotFailed(existing, "compensate: "+txErr.Error(), now)
		_ = i.repo.Save(ctx, &failed)
		return nil, fmt.Errorf("saga: compensate %s failed: %w", step, txErr)
	}
	compensated := i.snapshotCompensated(existing, now)
	if err := i.repo.Save(ctx, &compensated); err != nil {
		return result, fmt.Errorf("saga: compensate %s saved-status failed: %w", step, err)
	}
	return result, nil
}

// snapshotStarting 不变量：返回新结构体；不 mutate 入参。
func (i *instance) snapshotStarting(prev *domain.SagaStep, step string, now time.Time) domain.SagaStep {
	out := domain.SagaStep{
		SagaID:    i.id,
		StepName:  step,
		Status:    string(StatusInProgress),
		Attempts:  1,
		UpdatedAt: now,
		CreatedAt: now,
		StartedAt: ptrTime(now),
	}
	if prev != nil {
		out.ID = prev.ID
		out.Attempts = prev.Attempts + 1
		out.CreatedAt = prev.CreatedAt
		if prev.StartedAt != nil {
			out.StartedAt = prev.StartedAt
		}
		out.TraceID = prev.TraceID
	}
	return out
}

func (i *instance) snapshotFailed(prev *domain.SagaStep, errMsg string, now time.Time) domain.SagaStep {
	if i.scrubber != nil {
		errMsg = i.scrubber.Scrub(errMsg)
	}
	out := *prev
	out.Status = string(StatusFailed)
	out.LastError = truncate(errMsg, 4000)
	out.UpdatedAt = now
	return out
}

func (i *instance) snapshotCommitted(prev *domain.SagaStep, now time.Time) domain.SagaStep {
	out := *prev
	out.Status = string(StatusCommitted)
	out.LastError = ""
	out.UpdatedAt = now
	return out
}

func (i *instance) snapshotCompensated(prev *domain.SagaStep, now time.Time) domain.SagaStep {
	out := *prev
	out.Status = string(StatusCompensated)
	out.UpdatedAt = now
	return out
}

func ptrTime(t time.Time) *time.Time { return &t }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
