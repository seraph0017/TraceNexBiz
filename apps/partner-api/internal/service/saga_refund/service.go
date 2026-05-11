// Package refund 实现客户退款 saga（场景 J / integration §4.6）.
//
// 流程：
//
//	1. revenue.reverse       INSERT revenue_log(occurrence=2+, gross<0)
//	2. wallet.rollback       partner_wallet 应付台账回滚
//	3. provider.refund       持牌方退款（HMAC stub / Q12 后真实 SDK）
//	4. customer.notify       inApp + email + sms
package refund

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/saga"
)

// 错误.
var (
	ErrAmountInvalid = errors.New("refund: amount must be positive")
)

// Request 退款入参.
type Request struct {
	SagaID            string // UUIDv7
	OriginalRevenueID int64
	PartnerID         int64
	CustomerID        int64
	Amount            int64
	Reason            string
	OperatorID        int64
	TraceID           string
}

// Validate 校验.
func (r Request) Validate() error {
	if !saga.IsValidUUIDv7(r.SagaID) {
		return fmt.Errorf("refund: saga_id must be UUIDv7")
	}
	if r.Amount <= 0 {
		return ErrAmountInvalid
	}
	if r.PartnerID == 0 || r.CustomerID == 0 || r.OperatorID == 0 {
		return fmt.Errorf("refund: partner/customer/operator required")
	}
	return nil
}

// RevenuePort 写反向 revenue_log.
type RevenuePort interface {
	WriteReverse(ctx context.Context, req Request) error
}

// WalletPort partner_wallet 回滚.
type WalletPort interface {
	Rollback(ctx context.Context, partnerID, amount int64, sagaID string) error
}

// ProviderPort 持牌方退款.
type ProviderPort interface {
	Refund(ctx context.Context, sagaID string, amount int64) error
}

// Notifier 通知客户.
type Notifier interface {
	Notify(ctx context.Context, eventCode, refID, recipient, payload string) error
}

// Service 编排.
type Service struct {
	orch     saga.Orchestrator
	revenue  RevenuePort
	wallet   WalletPort
	provider ProviderPort
	notify   Notifier
}

// NewService 构造.
func NewService(o saga.Orchestrator, r RevenuePort, w WalletPort, p ProviderPort, n Notifier) *Service {
	return &Service{orch: o, revenue: r, wallet: w, provider: p, notify: n}
}

// Run 推进退款 saga.
func (s *Service) Run(ctx context.Context, req Request) error {
	if err := req.Validate(); err != nil {
		return err
	}
	sg, err := s.orch.NewSaga(req.SagaID, saga.KindCustomerRefund)
	if err != nil {
		return err
	}
	if _, err := sg.Run(ctx, "refund.reverse", func(_ *gorm.DB) (any, error) {
		return nil, s.revenue.WriteReverse(ctx, req)
	}); err != nil {
		return err
	}
	if _, err := sg.Run(ctx, "refund.wallet", func(_ *gorm.DB) (any, error) {
		return nil, s.wallet.Rollback(ctx, req.PartnerID, req.Amount, req.SagaID)
	}); err != nil {
		return err
	}
	if _, err := sg.Run(ctx, "refund.provider", func(_ *gorm.DB) (any, error) {
		return nil, s.provider.Refund(ctx, req.SagaID, req.Amount)
	}); err != nil {
		// 持牌方失败 → 不补偿（错误向人工托管），escalate 由 saga retry worker 决策
		return err
	}
	if _, err := sg.Run(ctx, "refund.notify", func(_ *gorm.DB) (any, error) {
		return nil, s.notify.Notify(ctx, "refund.success", req.SagaID, "", "")
	}); err != nil {
		return err
	}
	return nil
}
