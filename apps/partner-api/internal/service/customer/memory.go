// 内存 repo + stub。
package customer

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// MemoryRepo 内存仓。
type MemoryRepo struct {
	mu        sync.RWMutex
	rows      map[int64]domain.Customer
	logs      map[int64]domain.CustomerPartnerChangeLog
	nextID    int64
	nextLogID int64
}

// NewMemoryRepo .
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{rows: map[int64]domain.Customer{}, logs: map[int64]domain.CustomerPartnerChangeLog{}}
}

// Insert .
func (r *MemoryRepo) Insert(_ context.Context, c domain.Customer) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	c.ID = r.nextID
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	c.UpdatedAt = c.CreatedAt
	r.rows[c.ID] = c
	return c.ID, nil
}

// FindByIDForPartner BOLA 强制：partner_id 不匹配 → nil。
func (r *MemoryRepo) FindByIDForPartner(_ context.Context, partnerID, customerID int64) (*domain.Customer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.rows[customerID]
	if !ok {
		return nil, nil
	}
	if c.PartnerID == nil || *c.PartnerID != partnerID {
		return nil, nil
	}
	cp := c
	return &cp, nil
}

// FindByID .
func (r *MemoryRepo) FindByID(_ context.Context, customerID int64) (*domain.Customer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.rows[customerID]
	if !ok {
		return nil, nil
	}
	cp := c
	return &cp, nil
}

// FindByFyUserID .
func (r *MemoryRepo) FindByFyUserID(_ context.Context, fyUserID int64) (*domain.Customer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.rows {
		if c.FyUserID == fyUserID {
			cp := c
			return &cp, nil
		}
	}
	return nil, nil
}

// Update .
func (r *MemoryRepo) Update(_ context.Context, id int64, updater func(domain.Customer) domain.Customer) (*domain.Customer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.rows[id]
	if !ok {
		return nil, errors.New("customer: not found")
	}
	next := updater(c)
	next.UpdatedAt = time.Now()
	r.rows[id] = next
	cp := next
	return &cp, nil
}

// OrphanByPartner .
func (r *MemoryRepo) OrphanByPartner(_ context.Context, partnerID int64, _ time.Time, at time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for id, c := range r.rows {
		if c.PartnerID != nil && *c.PartnerID == partnerID && c.Status == StatusActive {
			c.Status = StatusOrphaned
			c.UpdatedAt = at
			r.rows[id] = c
			n++
		}
	}
	return n, nil
}

// ListByPartner .
func (r *MemoryRepo) ListByPartner(_ context.Context, partnerID int64, f ListFilter) ([]domain.Customer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.Customer{}
	for _, c := range r.rows {
		if c.PartnerID == nil || *c.PartnerID != partnerID {
			continue
		}
		if f.Status != "" && c.Status != f.Status {
			continue
		}
		out = append(out, c)
	}
	if f.Offset < len(out) {
		out = out[f.Offset:]
	} else if f.Offset >= len(out) {
		return nil, nil
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

// InsertChangeLog .
func (r *MemoryRepo) InsertChangeLog(_ context.Context, l domain.CustomerPartnerChangeLog) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextLogID++
	l.ID = r.nextLogID
	r.logs[l.ID] = l
	return l.ID, nil
}

// UpdateChangeLog .
func (r *MemoryRepo) UpdateChangeLog(_ context.Context, id int64, updater func(domain.CustomerPartnerChangeLog) domain.CustomerPartnerChangeLog) (*domain.CustomerPartnerChangeLog, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.logs[id]
	if !ok {
		return nil, errors.New("customer: change_log not found")
	}
	next := updater(l)
	r.logs[id] = next
	cp := next
	return &cp, nil
}

// stubInvitation 内嵌 invitation pkg 的最小 mock（避免 import 循环）。
type stubInvitation struct {
	mu       sync.Mutex
	codes    map[string]int64 // code → partner_id
	consumed map[string]bool
}

// NewStubInvitation .
func NewStubInvitation() *stubInvitation {
	return &stubInvitation{codes: map[string]int64{}, consumed: map[string]bool{}}
}

// Seed 注入 code → partner。
func (s *stubInvitation) Seed(code string, partnerID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codes[code] = partnerID
}

// Resolve .
func (s *stubInvitation) Resolve(_ context.Context, code string) (*domain.InvitationCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pid, ok := s.codes[code]
	if !ok {
		return nil, errors.New("invitation: code not found")
	}
	if s.consumed[code] {
		return nil, errors.New("invitation: code inactive")
	}
	return &domain.InvitationCode{Code: code, PartnerID: pid, Status: "active"}, nil
}

// Consume .
func (s *stubInvitation) Consume(_ context.Context, code string) (*domain.InvitationCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pid, ok := s.codes[code]
	if !ok {
		return nil, errors.New("invitation: code not found")
	}
	s.consumed[code] = true
	return &domain.InvitationCode{Code: code, PartnerID: pid, Status: "active", UsedCount: 1}, nil
}

// stubFyAPI 计数 group 更新 / erase 调用。
type stubFyAPI struct {
	mu      sync.Mutex
	groups  map[int64]string
	erased  map[int64]bool
}

// NewStubFyAPI .
func NewStubFyAPI() *stubFyAPI {
	return &stubFyAPI{groups: map[int64]string{}, erased: map[int64]bool{}}
}

// UpdateUserGroup .
func (s *stubFyAPI) UpdateUserGroup(_ context.Context, fyUserID int64, group, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groups[fyUserID] = group
	return nil
}

// EraseUser .
func (s *stubFyAPI) EraseUser(_ context.Context, fyUserID int64, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.erased[fyUserID] = true
	return nil
}

// Group .
func (s *stubFyAPI) Group(fyUserID int64) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.groups[fyUserID]
}

// Erased .
func (s *stubFyAPI) Erased(fyUserID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.erased[fyUserID]
}
