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
	Retried   int // Fix-B' part 2: 实际 re-run fn 的 step 数
	Escalated int
	Skipped   int
}

// Sweep 单次扫描；retry worker 在外层 ticker 调用。
//
// Fix-B' part 2 CRIT-B2：本扫描真正 re-run 业务 fn（之前的版本只 bump 状态）。
// 流程：
//  1. SELECT in_progress / failed step（attempts < MaxAttempts）
//  2. 检查 backoff：now < updated_at + BackoffFor(attempts) → 跳过
//  3. 检查 escalate 条件：max attempts / max wallclock → MarkEscalated 跳过
//  4. 从 Payload envelope decode (kind, input)；kind 未知或 fn 未注册 → 跳过 + 计 Skipped
//  5. 通过 LookupStep(kind, step_name) 找到 fn → 在新 instance 上 RunWithInput（保留 attempts）
//
// Skipped 含义在新版扩为 "had-no-effect-this-tick"：包含 backoff/未注册/transient-failure-but-未达max。
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
		// Backoff 闸：上次更新 + BackoffFor(attempts) 还未到。
		if step.Attempts > 0 && now.Sub(step.UpdatedAt) < BackoffFor(step.Attempts) {
			res.Skipped++
			continue
		}
		// Decode payload envelope；未持久化 input 的 step（legacy Run 调用）只能 Skipped。
		kind, input, err := decodePayloadEnvelope(step.Payload)
		if err != nil || kind == "" {
			res.Skipped++
			continue
		}
		fn := LookupStep(kind, step.StepName)
		if fn == nil {
			res.Skipped++
			continue
		}
		// 构造临时 instance 重放；不走 Resume（已知 kind）。
		inst := newInstance(step.SagaID, kind, o.repo, o.tx, o.scrubber)
		if _, err := inst.RunWithInput(ctx, step.StepName, input, fn); err != nil {
			// RunWithInput 内部已写 attempts++ + failed；不计入 Skipped（视为 Retried）。
			res.Retried++
			continue
		}
		res.Retried++
	}
	return res, nil
}
