// 内存 repo 测试用。
package invitation

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// MemoryRepo 内存仓。
type MemoryRepo struct {
	mu     sync.RWMutex
	rows   map[string]domain.InvitationCode
	nextID int64
}

// NewMemoryRepo .
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{rows: map[string]domain.InvitationCode{}}
}

// Insert .
func (r *MemoryRepo) Insert(_ context.Context, c domain.InvitationCode) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.rows[c.Code]; exists {
		return 0, errors.New("invitation: code already exists")
	}
	r.nextID++
	c.ID = r.nextID
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	c.UpdatedAt = c.CreatedAt
	r.rows[c.Code] = c
	return c.ID, nil
}

// FindByCode .
func (r *MemoryRepo) FindByCode(_ context.Context, code string) (*domain.InvitationCode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.rows[code]
	if !ok {
		return nil, nil
	}
	cp := c
	return &cp, nil
}

// IncUsedCount .
func (r *MemoryRepo) IncUsedCount(_ context.Context, code string) (*domain.InvitationCode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.rows[code]
	if !ok {
		return nil, errors.New("invitation: code not found")
	}
	c.UsedCount++
	c.UpdatedAt = time.Now()
	r.rows[code] = c
	cp := c
	return &cp, nil
}

// Update .
func (r *MemoryRepo) Update(_ context.Context, code string, updater func(domain.InvitationCode) domain.InvitationCode) (*domain.InvitationCode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.rows[code]
	if !ok {
		return nil, errors.New("invitation: code not found")
	}
	next := updater(c)
	next.UpdatedAt = time.Now()
	r.rows[code] = next
	cp := next
	return &cp, nil
}

// ListByPartner .
func (r *MemoryRepo) ListByPartner(_ context.Context, partnerID int64) ([]domain.InvitationCode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.InvitationCode{}
	for _, c := range r.rows {
		if c.PartnerID == partnerID {
			out = append(out, c)
		}
	}
	return out, nil
}
