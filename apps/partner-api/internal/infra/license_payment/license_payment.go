// Package license_payment 持牌分账方客户端 stub。
//
// 引用：PRD §7.6 + integration §V0.2.1 ARCH-HIGH-NEW-E。
//
// 候选持牌方：连连 / 易宝 / 汇付 / 合利宝（Q12 待业务方选定）。
//
// 关键约束：
//   - 客户付款绝不进入 partner-api 账户（per ADR-002 去二清）
//   - 平台 mchid 仅作为 ISV 佣金接收主体
//   - webhook 走独立 idempotency middleware（per backend §7.1 v0.2.1）
//   - RSA 验签 + IP 白名单
//
// W0：仅定义接口；W1d 在 Q12 决策后实现。
package license_payment

import (
	"context"
	"errors"
)

// Service 持牌分账方统一接口。
type Service interface {
	// CreateTopupIntent 客户充值意图：返回支付链接 / 唤起参数。
	// 不直接扣费；用户在持牌方页面完成支付后走 webhook 回调。
	CreateTopupIntent(ctx context.Context, req TopupIntentRequest) (*TopupIntentResponse, error)

	// VerifyWebhook 校验持牌方异步回调签名 + 解 payload。
	VerifyWebhook(ctx context.Context, signature string, body []byte) (*WebhookEvent, error)

	// CreatePayout 月结分账下账（Phase 2B）。
	CreatePayout(ctx context.Context, req PayoutRequest) (*PayoutResponse, error)

	// QueryPayout 查询下账状态（重试 / 对账）。
	QueryPayout(ctx context.Context, providerTradeNo string) (*PayoutStatus, error)
}

// TopupIntentRequest TODO(W1d): 字段以 Q12 选定持牌方文档为准
type TopupIntentRequest struct {
	OutTradeNo string
	Amount     int64 // 单位：分
	CustomerID int64
	Channel    string // wechat / alipay
	NotifyURL  string
}

// TopupIntentResponse 返回前端用于唤起支付。
type TopupIntentResponse struct {
	PayURL          string
	ProviderTradeNo string
}

// WebhookEvent 持牌方推送事件。
type WebhookEvent struct {
	EventID         string
	OutTradeNo      string
	ProviderTradeNo string
	State           string // paid / refunded / failed
	Amount          int64
	OccurredAt      int64
	Raw             []byte
}

// PayoutRequest 下账请求（settlement_item 触发）。
type PayoutRequest struct {
	OutBatchNo  string
	PartnerSubAccountID string
	Amount      int64
	Memo        string
}

type PayoutResponse struct {
	ProviderTradeNo string
	State           string
}

type PayoutStatus struct {
	State    string
	PaidAt   int64
	FailCode string
}

// ErrNotImplemented W0 stub 错误。
var ErrNotImplemented = errors.New("license_payment: stub not implemented; W1d agent to wire post-Q12")

// Stub W0 占位。
type Stub struct{}

func NewStub() Service { return &Stub{} }

func (s *Stub) CreateTopupIntent(_ context.Context, _ TopupIntentRequest) (*TopupIntentResponse, error) {
	return nil, ErrNotImplemented
}

func (s *Stub) VerifyWebhook(_ context.Context, _ string, _ []byte) (*WebhookEvent, error) {
	return nil, ErrNotImplemented
}

func (s *Stub) CreatePayout(_ context.Context, _ PayoutRequest) (*PayoutResponse, error) {
	return nil, ErrNotImplemented
}

func (s *Stub) QueryPayout(_ context.Context, _ string) (*PayoutStatus, error) {
	return nil, ErrNotImplemented
}
