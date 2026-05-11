// Package topup 实现客户充值 saga（场景 D / integration §4.5）.
//
// 流程：
//
//	1. payment.intent     创建/复用 topup_intent（state=created）
//	2. provider.topup     调持牌方分账（HMAC stub / Q12 后真实 SDK）→ provider_trade_no
//	3. provider.callback  持牌方回调（异步；callback handler 写 saga step "callback"）
//	4. fyapi.topup        Idempotency-Key=saga_id；customer fy_user_id 提额
//	5. notify             写 notification_outbox（in-app + email）
//
// 关键 invariant：
//   1. saga_id 是 UUIDv7（不是 topup_intent.id BIGINT）— v0.2.1 ARCH-HIGH-NEW-D
//   2. topup_intent 字段不可在 funded 之后被改（lock by state machine）
//   3. escalated 状态走 notification_outbox.event_code=topup.escalated（PRD §22.1 F-3）
//
// W1c 实现 callback handler；本文件提供 service skeleton + 状态机校验.
package topup

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/saga"
)

// 错误.
var (
	ErrInvalidState   = errors.New("topup: invalid state transition")
	ErrAmountInvalid  = errors.New("topup: amount must be positive")
	ErrCallbackReplay = errors.New("topup: callback replay rejected")
)

// State topup_intent.state 枚举.
type State string

const (
	StateCreated  State = "created"
	StatePaid     State = "paid"
	StateFunded   State = "funded"
	StateRefunded State = "refunded"
	StateFailed   State = "failed"
	StateCanceled State = "canceled"
)

// CanAdvance 状态机.
func CanAdvance(from, to State) bool {
	switch from {
	case StateCreated:
		return to == StatePaid || to == StateFailed || to == StateCanceled
	case StatePaid:
		return to == StateFunded || to == StateRefunded || to == StateFailed
	case StateFunded:
		return to == StateRefunded
	}
	return false
}

// Request 客户充值入参.
type Request struct {
	SagaID     string // UUIDv7
	CustomerID int64
	FyUserID   int64
	Amount     int64
	Channel    string // wechat_pay / alipay
	TraceID    string
}

// Validate 入参.
func (r Request) Validate() error {
	if !saga.IsValidUUIDv7(r.SagaID) {
		return fmt.Errorf("topup: saga_id must be UUIDv7")
	}
	if r.Amount <= 0 {
		return ErrAmountInvalid
	}
	if r.CustomerID == 0 || r.FyUserID == 0 {
		return fmt.Errorf("topup: customer/fy_user required")
	}
	if r.Channel == "" {
		return fmt.Errorf("topup: channel required")
	}
	return nil
}

// IntentPort topup_intent CRUD.
type IntentPort interface {
	UpsertCreated(ctx context.Context, req Request) error
	MarkPaid(ctx context.Context, sagaID, providerTradeNo string) error
	MarkFunded(ctx context.Context, sagaID string) error
	MarkRefunded(ctx context.Context, sagaID string) error
}

// ProviderPort 持牌方接口（避免循环 import）.
type ProviderPort interface {
	CreatePayment(ctx context.Context, req Request) (payURL, providerTradeNo string, err error)
	VerifyCallback(payload []byte, signature string) (sagaID string, paid bool, err error)
}

// FyAPIPort fyapi 充值.
type FyAPIPort interface {
	TopupCustomer(ctx context.Context, fyUserID int64, amount int64, idemKey, traceID string) error
}

// Notifier 写 notification_outbox.
type Notifier interface {
	Notify(ctx context.Context, eventCode, refID, recipient, payload string) error
}

// Service 编排.
type Service struct {
	orch     saga.Orchestrator
	intent   IntentPort
	provider ProviderPort
	fyapi    FyAPIPort
	notify   Notifier
}

// NewService 构造.
func NewService(o saga.Orchestrator, i IntentPort, p ProviderPort, f FyAPIPort, n Notifier) *Service {
	return &Service{orch: o, intent: i, provider: p, fyapi: f, notify: n}
}

const (
	StepIntent   = "topup.intent"
	StepProvider = "topup.provider"
	StepFundFy   = "topup.fy"
	StepNotify   = "topup.notify"
)

// Initiate 创建 intent + 跳转持牌方.
//
// 返回 (payURL)；客户支付完成后由 webhook 回调驱动 OnCallback / OnPaid.
func (s *Service) Initiate(ctx context.Context, req Request) (string, error) {
	if err := req.Validate(); err != nil {
		return "", err
	}
	sg, err := s.orch.NewSaga(req.SagaID, saga.KindCustomerTopup)
	if err != nil {
		return "", err
	}
	if _, err := sg.Run(ctx, StepIntent, func(_ *gorm.DB) (any, error) {
		return nil, s.intent.UpsertCreated(ctx, req)
	}); err != nil {
		return "", err
	}
	var payURL string
	if _, err := sg.Run(ctx, StepProvider, func(_ *gorm.DB) (any, error) {
		url, _, e := s.provider.CreatePayment(ctx, req)
		payURL = url
		return nil, e
	}); err != nil {
		return "", err
	}
	return payURL, nil
}

// OnCallback 持牌方回调驱动；payload+signature 由 caller webhook handler 透传.
func (s *Service) OnCallback(ctx context.Context, payload []byte, signature string) error {
	sagaID, paid, err := s.provider.VerifyCallback(payload, signature)
	if err != nil {
		return err
	}
	if !paid {
		return ErrCallbackReplay
	}
	sg, err := s.orch.Resume(sagaID)
	if err != nil {
		return err
	}
	if _, err := sg.Run(ctx, "topup.callback", func(_ *gorm.DB) (any, error) {
		return nil, s.intent.MarkPaid(ctx, sagaID, "")
	}); err != nil {
		return err
	}
	// fund 走异步：service 调用方根据 callback 后立刻进 fund 步骤.
	return nil
}

// Fund Fy-api 提额；通常在 OnCallback 之后.
func (s *Service) Fund(ctx context.Context, sagaID string, fyUserID, amount int64, traceID string) error {
	if !saga.IsValidUUIDv7(sagaID) {
		return fmt.Errorf("topup: saga_id must be UUIDv7")
	}
	sg, err := s.orch.Resume(sagaID)
	if err != nil {
		return err
	}
	if _, err := sg.Run(ctx, StepFundFy, func(_ *gorm.DB) (any, error) {
		return nil, s.fyapi.TopupCustomer(ctx, fyUserID, amount, sagaID, traceID)
	}); err != nil {
		return err
	}
	if _, err := sg.Run(ctx, "topup.intent.funded", func(_ *gorm.DB) (any, error) {
		return nil, s.intent.MarkFunded(ctx, sagaID)
	}); err != nil {
		return err
	}
	if _, err := sg.Run(ctx, StepNotify, func(_ *gorm.DB) (any, error) {
		return nil, s.notify.Notify(ctx, "topup.success", sagaID, "", "")
	}); err != nil {
		return err
	}
	return nil
}

// NotifyEscalated escalated 状态 UX 通知（场景 D 兜底）.
func (s *Service) NotifyEscalated(ctx context.Context, sagaID, recipient string) error {
	return s.notify.Notify(ctx, "topup.escalated", sagaID, recipient, "")
}
