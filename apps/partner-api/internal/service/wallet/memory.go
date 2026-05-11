// 内存 repo for wallet。
package wallet

import (
	"context"
	"sync"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// MemoryRepo 内存仓。
type MemoryRepo struct {
	mu      sync.RWMutex
	wallets map[int64]domain.PartnerWallet // by partner_id
	holds   map[int64][]domain.WalletHold  // by partner_id
	logs    map[int64][]domain.PartnerWalletLog
}

// NewMemoryRepo .
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		wallets: map[int64]domain.PartnerWallet{},
		holds:   map[int64][]domain.WalletHold{},
		logs:    map[int64][]domain.PartnerWalletLog{},
	}
}

// SeedWallet 注入测试钱包。
func (r *MemoryRepo) SeedWallet(w domain.PartnerWallet) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w.CreatedAt.IsZero() {
		w.CreatedAt = time.Now()
	}
	w.UpdatedAt = w.CreatedAt
	r.wallets[w.PartnerID] = w
}

// SeedHold 注入测试 hold。
func (r *MemoryRepo) SeedHold(h domain.WalletHold) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.holds[h.PartnerID] = append(r.holds[h.PartnerID], h)
}

// SeedLog 注入测试 log。
func (r *MemoryRepo) SeedLog(l domain.PartnerWalletLog) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs[l.PartnerID] = append(r.logs[l.PartnerID], l)
}

// FindWallet .
func (r *MemoryRepo) FindWallet(_ context.Context, partnerID int64) (*domain.PartnerWallet, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.wallets[partnerID]
	if !ok {
		return nil, nil
	}
	cp := w
	return &cp, nil
}

// SumHeldByPartner .
func (r *MemoryRepo) SumHeldByPartner(_ context.Context, partnerID int64) (int64, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var sum int64
	var count int
	for _, h := range r.holds[partnerID] {
		if h.Status == "held" {
			sum += h.Amount
			count++
		}
	}
	return sum, count, nil
}

// ListLogs .
func (r *MemoryRepo) ListLogs(_ context.Context, partnerID int64, f LogFilter) ([]domain.PartnerWalletLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	src := r.logs[partnerID]
	out := make([]domain.PartnerWalletLog, 0, len(src))
	for _, l := range src {
		if f.Type != "" && l.Type != f.Type {
			continue
		}
		out = append(out, l)
	}
	if f.Offset > len(out) {
		return nil, nil
	}
	out = out[f.Offset:]
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

// ListHolds .
func (r *MemoryRepo) ListHolds(_ context.Context, partnerID int64) ([]domain.WalletHold, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	src := r.holds[partnerID]
	out := make([]domain.WalletHold, 0, len(src))
	for _, h := range src {
		if h.Status == "held" {
			out = append(out, h)
		}
	}
	return out, nil
}
