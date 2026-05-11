// Package wallet 钱包查询 + 流水（PRD §8.3 / §8.4 / §8.5）。
//
// W1a 范围：read-only 查询 + service 接口（Allocate / Refund / Topup 留 stub 给 W1b 实现 saga）。
// 关键 invariant（backend §15.2 I-W-7）：available = balance - sum(wallet_hold.amount where status='held')。
//
// held_amount 字段已在 v0.2 ADR-012 drop；service 用 wallet_hold join 计算。
package wallet

import (
	"context"
	"errors"
	"fmt"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// Sentinel.
var (
	ErrWalletNotFound       = errors.New("wallet: not found")
	ErrInsufficientAvailable = errors.New("wallet: insufficient available")
	ErrNotImplemented       = errors.New("wallet: allocation saga not implemented; W1b")
)

// Snapshot 给 handler 渲染的视图：含 available 计算。
type Snapshot struct {
	Wallet         domain.PartnerWallet
	HeldTotal      int64
	Available      int64
	OpenHoldsCount int
}

// Repository 钱包 + hold + log 持久化端口。BOLA scope 强制：每个查询首参除 ctx 外必须带 partner_id。
type Repository interface {
	FindWallet(ctx context.Context, partnerID int64) (*domain.PartnerWallet, error)
	SumHeldByPartner(ctx context.Context, partnerID int64) (sum int64, count int, err error)
	ListLogs(ctx context.Context, partnerID int64, filter LogFilter) ([]domain.PartnerWalletLog, error)
	ListHolds(ctx context.Context, partnerID int64) ([]domain.WalletHold, error)
}

// LogFilter 流水过滤。
type LogFilter struct {
	Type   domain.WalletLogType
	Limit  int
	Offset int
}

// Service 钱包查询门面。
type Service struct {
	repo Repository
}

// NewService .
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// Get 查询当前快照（含 available 实时计算）。
func (s *Service) Get(ctx context.Context, partnerID int64) (*Snapshot, error) {
	if partnerID <= 0 {
		return nil, ErrWalletNotFound
	}
	w, err := s.repo.FindWallet(ctx, partnerID)
	if err != nil {
		return nil, fmt.Errorf("wallet: find: %w", err)
	}
	if w == nil {
		return nil, ErrWalletNotFound
	}
	heldSum, heldCount, err := s.repo.SumHeldByPartner(ctx, partnerID)
	if err != nil {
		return nil, fmt.Errorf("wallet: sum hold: %w", err)
	}
	return &Snapshot{
		Wallet: *w, HeldTotal: heldSum,
		Available: w.Balance - heldSum, OpenHoldsCount: heldCount,
	}, nil
}

// ListLogs partner 查自己的流水。
func (s *Service) ListLogs(ctx context.Context, partnerID int64, f LogFilter) ([]domain.PartnerWalletLog, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	rows, err := s.repo.ListLogs(ctx, partnerID, f)
	if err != nil {
		return nil, fmt.Errorf("wallet: list logs: %w", err)
	}
	return rows, nil
}

// ListHolds 列当前未结清的 hold（status='held'）。
func (s *Service) ListHolds(ctx context.Context, partnerID int64) ([]domain.WalletHold, error) {
	rows, err := s.repo.ListHolds(ctx, partnerID)
	if err != nil {
		return nil, fmt.Errorf("wallet: list holds: %w", err)
	}
	return rows, nil
}

// AllocateInput W1b 实现 saga 时使用；本签名锁定，给 W1b 落 saga 实现。
type AllocateInput struct {
	PartnerID      int64
	CustomerID     int64
	Amount         int64 // > 0
	IdempotencyKey string
	OperatorID     int64 // staff / partner self
	OperatorType   string
	TraceID        string
}

// AllocateOutput .
type AllocateOutput struct {
	SagaID     string
	NewBalance int64
	HeldAmount int64
	LogID      int64
}

// AllocateExecutor saga 执行器；由 W1b 实现注入。本接口固定保证 W1c admin / W1f partner UI
// 不需要在 W1b 完成前停工。
type AllocateExecutor interface {
	Allocate(ctx context.Context, in AllocateInput) (AllocateOutput, error)
	Refund(ctx context.Context, in RefundInput) (RefundOutput, error)
	Topup(ctx context.Context, in TopupInput) (TopupOutput, error)
}

// RefundInput 退款入参。
type RefundInput struct {
	PartnerID      int64
	CustomerID     int64
	RevenueLogID   int64
	Amount         int64
	IdempotencyKey string
	OperatorID     int64
	OperatorType   string
	TraceID        string
}

// RefundOutput .
type RefundOutput struct {
	SagaID  string
	LogID   int64
	DebtID  *int64 // partner_debt 写入时回填
}

// TopupInput 充值入参（partner.initial_topup / 客户充值由 payment pkg）。
type TopupInput struct {
	PartnerID      int64
	Amount         int64
	IdempotencyKey string
	OperatorID     int64
	OperatorType   string
	TraceID        string
}

// TopupOutput .
type TopupOutput struct {
	SagaID     string
	LogID      int64
	NewBalance int64
}

// StubAllocator W1a 给的占位实现：所有 saga 入口返回 ErrNotImplemented。
type StubAllocator struct{}

// Allocate .
func (StubAllocator) Allocate(_ context.Context, _ AllocateInput) (AllocateOutput, error) {
	return AllocateOutput{}, ErrNotImplemented
}

// Refund .
func (StubAllocator) Refund(_ context.Context, _ RefundInput) (RefundOutput, error) {
	return RefundOutput{}, ErrNotImplemented
}

// Topup .
func (StubAllocator) Topup(_ context.Context, _ TopupInput) (TopupOutput, error) {
	return TopupOutput{}, ErrNotImplemented
}
