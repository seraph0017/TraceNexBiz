// 内存 repo + 测试用 stub。
package partner

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// MemoryRepo 内存实现（dev / 单测）。
type MemoryRepo struct {
	mu     sync.RWMutex
	rows   map[int64]domain.Partner
	nextID int64
}

// NewMemoryRepo 构造空仓。
func NewMemoryRepo() *MemoryRepo { return &MemoryRepo{rows: map[int64]domain.Partner{}} }

// Insert .
func (r *MemoryRepo) Insert(_ context.Context, p domain.Partner) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	p.ID = r.nextID
	if p.AppliedAt.IsZero() {
		p.AppliedAt = time.Now()
	}
	p.CreatedAt = p.AppliedAt
	p.UpdatedAt = p.AppliedAt
	r.rows[p.ID] = p
	return p.ID, nil
}

// FindByID .
func (r *MemoryRepo) FindByID(_ context.Context, id int64) (*domain.Partner, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.rows[id]
	if !ok {
		return nil, nil
	}
	cp := p
	return &cp, nil
}

// FindByFyUserID .
func (r *MemoryRepo) FindByFyUserID(_ context.Context, fyUserID int64) (*domain.Partner, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.rows {
		if p.FyUserID == fyUserID {
			cp := p
			return &cp, nil
		}
	}
	return nil, nil
}

// FindByEmailHMAC .
func (r *MemoryRepo) FindByEmailHMAC(_ context.Context, h string) (*domain.Partner, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.rows {
		if p.ContactEmailHMAC == h {
			cp := p
			return &cp, nil
		}
	}
	return nil, nil
}

// Update 走 updater；返回更新后副本。
func (r *MemoryRepo) Update(_ context.Context, id int64, updater func(domain.Partner) domain.Partner) (*domain.Partner, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.rows[id]
	if !ok {
		return nil, errors.New("partner: not found")
	}
	next := updater(p)
	next.UpdatedAt = time.Now()
	r.rows[id] = next
	cp := next
	return &cp, nil
}

// List 简单过滤。
func (r *MemoryRepo) List(_ context.Context, f ListFilter) ([]domain.Partner, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.Partner{}
	for _, p := range r.rows {
		if f.Status != "" && string(p.Status) != f.Status {
			continue
		}
		if f.Search != "" && !strings.Contains(p.InvitationCode, f.Search) {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// stubCrypto 测试用 fake：不加密；HMAC 用简单 hash。
type stubCrypto struct{}

// NewStubCrypto 构造。
func NewStubCrypto() CryptoPort { return stubCrypto{} }

// EncryptPhone 直接 returns plain bytes + 固定 keyID。
func (stubCrypto) EncryptPhone(_ context.Context, plain string) ([]byte, string, error) {
	return []byte(plain), "stub-key", nil
}

// HMACEmail 简易 lowercase + reverse；测试用。
func (stubCrypto) HMACEmail(_ context.Context, email string) (string, error) {
	return strings.ToLower("hmac:" + email), nil
}

// alwaysFreshConsent 测试用 ConsentPort：任何 ID 都返 sensitive_pi consented。
type alwaysFreshConsent struct{ at time.Time }

// NewAlwaysFreshConsent 构造（at = 5min 内）。
func NewAlwaysFreshConsent(at time.Time) ConsentPort { return alwaysFreshConsent{at: at} }

// FindByID .
func (c alwaysFreshConsent) FindByID(_ context.Context, _ int64) (time.Time, string, bool, error) {
	return c.at, "sensitive_pi+privacy_policy", false, nil
}

// stubInvitation 不去 invitation pkg；只生成可读 code。
type stubInvitation struct {
	mu sync.Mutex
	n  int
}

// NewStubInvitation .
func NewStubInvitation() InvitationGenerator { return &stubInvitation{} }

// Generate .
func (s *stubInvitation) Generate(_ context.Context, partnerID int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n++
	return "INV-TEST-" + strings.Repeat("X", 8) + "-" + intToStr(partnerID), nil
}

// stubOrphaner .
type stubOrphaner struct {
	mu     sync.Mutex
	called map[int64]time.Time
}

// NewStubOrphaner .
func NewStubOrphaner() *stubOrphaner { return &stubOrphaner{called: map[int64]time.Time{}} }

// OrphanByPartner .
func (s *stubOrphaner) OrphanByPartner(_ context.Context, partnerID int64, until time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.called[partnerID] = until
	return nil
}

// Called 测试断言。
func (s *stubOrphaner) Called(partnerID int64) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.called[partnerID]
	return v, ok
}

func intToStr(n int64) string {
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	return out
}
