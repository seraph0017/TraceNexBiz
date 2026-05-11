// internal/saga/orchestrator.go — saga 工厂 + retry sweep。
//
// 设计：
//   - NewSaga 校验 idemKey 必须 UUIDv7（W1b 范围内强制）
//   - Resume 用于 retry worker 挂载已有 saga
//   - Sweep 周期性扫描 in_progress / failed step → 推进或 escalate
//
// 上线后多副本运行：partition by hash(saga_id) % N == replica_index（W1b 约定接口；W1a 在 sweep loop 加 partition 过滤）。
package saga

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// orchestrator 实现 Orchestrator。
type orchestrator struct {
	repo     Repository
	tx       Transactor
	scrubber PIIScrubber
}

// NewOrchestrator 构造（main.go wire）。
func NewOrchestrator(repo Repository, tx Transactor, scrubber PIIScrubber) Orchestrator {
	return &orchestrator{repo: repo, tx: tx, scrubber: scrubber}
}

// NewOrchestratorWithGorm 便捷构造：从 *gorm.DB 包出 Transactor.
func NewOrchestratorWithGorm(repo Repository, db *gorm.DB, scrubber PIIScrubber) Orchestrator {
	return &orchestrator{repo: repo, tx: &GormTransactor{DB: db}, scrubber: scrubber}
}

// NewSaga 创建 saga 实例。idemKey 必须 UUIDv7。
//
// 不预创建 step：step 在第一次 Run 时才落库，避免 saga_step 表稀疏空记录。
func (o *orchestrator) NewSaga(idemKey string, kind Kind) (Saga, error) {
	if !IsValidUUIDv7(idemKey) {
		return nil, fmt.Errorf("saga: idem_key must be UUIDv7, got %q", idemKey)
	}
	if kind == "" {
		return nil, fmt.Errorf("saga: kind required")
	}
	return newInstance(idemKey, kind, o.repo, o.tx, o.scrubber), nil
}

// Resume 挂载已有 saga（不查 DB，仅构造句柄；后续 Run/Compensate 自然走 GetStep 探测）。
func (o *orchestrator) Resume(sagaID string) (Saga, error) {
	if !IsValidUUIDv7(sagaID) {
		return nil, fmt.Errorf("saga: saga_id must be UUIDv7, got %q", sagaID)
	}
	// kind 未知（DB 不存）→ 用 sentinel；retry worker 通过 step name 路由 fn
	return newInstance(sagaID, "", o.repo, o.tx, o.scrubber), nil
}

// SweepResult 一次 sweep 的统计。
type SweepResult struct {
	Scanned   int
	Escalated int
	Skipped   int
}

// Sweep 单次扫描；retry worker 在外层 ticker 调用。
//
// 注意：Sweep 不实际重做业务 fn —— 仅判断 escalation。重做需调用方注册 saga handler（W1a 后续）。
// W1b 范围只保证 escalation 决策可独立运行，便于 chaos test。
func (o *orchestrator) Sweep(ctx context.Context, batch int) (SweepResult, error) {
	if batch <= 0 {
		batch = 100
	}
	rows, err := o.repo.LoadPendingRetries(ctx, batch)
	if err != nil {
		return SweepResult{}, fmt.Errorf("saga: load pending retries: %w", err)
	}
	res := SweepResult{Scanned: len(rows)}
	for _, step := range rows {
		if step == nil {
			continue
		}
		now := nowUTC()
		if escalate, reason := ShouldEscalate(step, now); escalate {
			if err := o.repo.MarkEscalated(ctx, step.SagaID, step.StepName, reason); err != nil {
				res.Skipped++
				continue
			}
			res.Escalated++
			continue
		}
		res.Skipped++
	}
	return res, nil
}
