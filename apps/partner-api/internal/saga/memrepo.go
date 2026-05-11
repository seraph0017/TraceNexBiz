// internal/saga/memrepo.go — 内存 Repository（dev / test 用）.
//
// 生产由 W1a 提供 GORM 实现；本文件让 saga 包能跑出端到端 race-safe 单测。
package saga

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// MemRepo 线程安全 in-mem repository.
type MemRepo struct {
	mu    sync.Mutex
	steps map[string]map[string]*domain.SagaStep // saga_id → step_name → row
	seq   int64
}

// NewMemRepo 构造.
func NewMemRepo() *MemRepo {
	return &MemRepo{steps: make(map[string]map[string]*domain.SagaStep)}
}

// GetByID 返回深拷贝.
func (m *MemRepo) GetByID(_ context.Context, sagaID string) ([]*domain.SagaStep, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	bucket, ok := m.steps[sagaID]
	if !ok {
		return nil, nil
	}
	out := make([]*domain.SagaStep, 0, len(bucket))
	for _, row := range bucket {
		cp := *row
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StepName < out[j].StepName })
	return out, nil
}

// GetStep 返回拷贝；不存在返 (nil, ErrStepNotFound)。
func (m *MemRepo) GetStep(_ context.Context, sagaID, stepName string) (*domain.SagaStep, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	bucket, ok := m.steps[sagaID]
	if !ok {
		return nil, ErrStepNotFound
	}
	row, ok := bucket[stepName]
	if !ok {
		return nil, ErrStepNotFound
	}
	cp := *row
	return &cp, nil
}

// Save UPSERT；不可改写已 committed step 的关键字段（status 回退）。
func (m *MemRepo) Save(_ context.Context, step *domain.SagaStep) error {
	if step == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	bucket, ok := m.steps[step.SagaID]
	if !ok {
		bucket = make(map[string]*domain.SagaStep)
		m.steps[step.SagaID] = bucket
	}
	cp := *step
	if cp.ID == 0 {
		m.seq++
		cp.ID = m.seq
	}
	bucket[cp.StepName] = &cp
	return nil
}

// LoadPendingRetries 拉 in_progress / failed 的 step.
func (m *MemRepo) LoadPendingRetries(_ context.Context, limit int) ([]*domain.SagaStep, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*domain.SagaStep, 0)
	for _, bucket := range m.steps {
		for _, row := range bucket {
			s := Status(row.Status)
			if s == StatusInProgress || s == StatusFailed {
				cp := *row
				out = append(out, &cp)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// MarkEscalated 状态切到 escalated.
func (m *MemRepo) MarkEscalated(_ context.Context, sagaID, stepName, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	bucket, ok := m.steps[sagaID]
	if !ok {
		return ErrStepNotFound
	}
	row, ok := bucket[stepName]
	if !ok {
		return ErrStepNotFound
	}
	cp := *row
	cp.Status = string(StatusEscalated)
	cp.EscalateReason = reason
	now := time.Now().UTC()
	cp.EscalatedAt = &now
	cp.UpdatedAt = now
	bucket[stepName] = &cp
	return nil
}

// ForceResolve 切到 target.
func (m *MemRepo) ForceResolve(_ context.Context, sagaID, stepName string, target Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	bucket, ok := m.steps[sagaID]
	if !ok {
		return ErrStepNotFound
	}
	row, ok := bucket[stepName]
	if !ok {
		return ErrStepNotFound
	}
	cp := *row
	cp.Status = string(target)
	cp.UpdatedAt = time.Now().UTC()
	bucket[stepName] = &cp
	return nil
}
