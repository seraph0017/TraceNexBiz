// 内存 repo + stubs.
package kyc

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// MemoryRepo .
type MemoryRepo struct {
	mu     sync.RWMutex
	rows   map[int64]domain.KYCApplication
	nextID int64
}

// NewMemoryRepo .
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{rows: map[int64]domain.KYCApplication{}}
}

// Insert .
func (r *MemoryRepo) Insert(_ context.Context, a domain.KYCApplication) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.rows {
		if x.FyUserID == a.FyUserID {
			return 0, errors.New("kyc: fy_user_id already has application")
		}
	}
	r.nextID++
	a.ID = r.nextID
	r.rows[a.ID] = a
	return a.ID, nil
}

// FindByFyUserID .
func (r *MemoryRepo) FindByFyUserID(_ context.Context, fy int64) (*domain.KYCApplication, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, a := range r.rows {
		if a.FyUserID == fy {
			cp := a
			return &cp, nil
		}
	}
	return nil, nil
}

// FindByID .
func (r *MemoryRepo) FindByID(_ context.Context, id int64) (*domain.KYCApplication, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.rows[id]
	if !ok {
		return nil, nil
	}
	cp := a
	return &cp, nil
}

// FindByLegalIDBlindIndex .
func (r *MemoryRepo) FindByLegalIDBlindIndex(_ context.Context, bi string) (*domain.KYCApplication, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, a := range r.rows {
		if a.LegalPersonIDBlindIndex == bi {
			cp := a
			return &cp, nil
		}
	}
	return nil, nil
}

// Update .
func (r *MemoryRepo) Update(_ context.Context, id int64, updater func(domain.KYCApplication) domain.KYCApplication) (*domain.KYCApplication, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.rows[id]
	if !ok {
		return nil, errors.New("kyc: not found")
	}
	next := updater(a)
	next.UpdatedAt = time.Now()
	r.rows[id] = next
	cp := next
	return &cp, nil
}

// ListPendingReview .
func (r *MemoryRepo) ListPendingReview(_ context.Context, limit int) ([]domain.KYCApplication, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.KYCApplication{}
	for _, a := range r.rows {
		if a.Status == StatusSubmitted || a.Status == StatusUnderReview {
			out = append(out, a)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

// PurgeColdExpired .
func (r *MemoryRepo) PurgeColdExpired(_ context.Context, before time.Time, limit int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for id, a := range r.rows {
		if a.ColdArchiveExpiresAt != nil && a.ColdArchiveExpiresAt.Before(before) {
			delete(r.rows, id)
			n++
			if limit > 0 && n >= limit {
				break
			}
		}
	}
	return n, nil
}

// stubCrypto 简单 SHA256 HMAC + 透传 cipher 占位。
type stubCrypto struct{}

// NewStubCrypto .
func NewStubCrypto() CryptoPort { return stubCrypto{} }

// Encrypt .
func (stubCrypto) Encrypt(_ context.Context, scope string, plain []byte) ([]byte, string, error) {
	return plain, "stub-" + scope + "-v1", nil
}

// HMAC .
func (stubCrypto) HMAC(_ context.Context, scope string, plain []byte) ([]byte, error) {
	h := sha256.Sum256(append([]byte(scope+":"), plain...))
	return h[:], nil
}

// stubOCR pass-through。
type stubOCR struct{}

// NewStubOCR .
func NewStubOCR() OCRPort { return stubOCR{} }

// ParseBusinessLicense .
func (stubOCR) ParseBusinessLicense(_ context.Context, _ string) (string, string, string, error) {
	return "TestCo Ltd.", "Alice", "REG-001", nil
}

// VerifyIDName .
func (stubOCR) VerifyIDName(_ context.Context, _, _ string) (bool, error) {
	return true, nil
}

// stubOSS .
type stubOSS struct{}

// NewStubOSS .
func NewStubOSS() OSSPort { return stubOSS{} }

// VerifyKYCObject .
func (stubOSS) VerifyKYCObject(_ context.Context, _, _ string) error { return nil }

// stubConsent .
type stubConsent struct{}

// NewStubConsent .
func NewStubConsent() ConsentPort { return stubConsent{} }

// HasConsent .
func (stubConsent) HasConsent(_ context.Context, id int64) (bool, error) { return id > 0, nil }

// stubLinker .
type stubLinker struct {
	mu     sync.Mutex
	called map[int64]int8
}

// NewStubLinker .
func NewStubLinker() *stubLinker { return &stubLinker{called: map[int64]int8{}} }

// OnKYCApproved .
func (s *stubLinker) OnKYCApproved(_ context.Context, fyUserID int64, kycType int8, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.called[fyUserID] = kycType
	return nil
}

// Called .
func (s *stubLinker) Called(fy int64) (int8, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.called[fy]
	return v, ok
}
