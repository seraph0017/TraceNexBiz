// Package payment 持牌方分账接入（PRD §7.6 去二清）.
//
// Q12（持牌方未定）期间：本包暴露 LicensedProvider 接口 + Stub 实现；
// 真实持牌方（连连 / 通联 / 银盛 / 富友 …）SDK 接入 W1d-2 完成。
//
// 三个核心动作：
//
//	Topup     渠道商 / 客户充值入口 → 返回前端跳转 URL + out_trade_no
//	Withdraw  分账下发到持牌方虚拟户 → 异步回调
//	Reconcile 当日 T+1 对账（金额哈希一致性）
//
// 所有动作幂等：以 (provider, out_trade_no) UNIQUE 落地，回调侧走
// middleware.WebhookIdempotency（backend §7.1）。
package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ErrInvalidSignature 回调验签失败.
var ErrInvalidSignature = errors.New("payment: invalid callback signature")

// ErrAmountMismatch 对账金额不一致.
var ErrAmountMismatch = errors.New("payment: amount mismatch")

// ErrUnknown 上游 5xx / 超时；调用方进入 saga unknown 分支.
var ErrUnknown = errors.New("payment: provider unknown state")

// TopupRequest 创建充值意图.
type TopupRequest struct {
	ActorType  string // partner / customer
	ActorID    int64
	Amount     int64  // 分；> 0
	Channel    string // wechat_pay / alipay / bank
	OutTradeNo string // saga_id；UNIQUE
	NotifyURL  string
	ReturnURL  string
}

// TopupResponse 持牌方返回的跳转地址.
type TopupResponse struct {
	PayURL          string
	ProviderTradeNo string
	ExpiresAt       time.Time
}

// CallbackPayload 持牌方回调透传给 service.
type CallbackPayload struct {
	OutTradeNo      string
	ProviderTradeNo string
	Status          string // success / failed / pending
	Amount          int64
	PaidAt          time.Time
	Raw             []byte
	Signature       string
}

// WithdrawRequest 提现指令（结算后 T+1 落账）.
type WithdrawRequest struct {
	OutBatchNo string
	PartnerID  int64
	Amount     int64
	BankAcct   string
	BankName   string
}

// WithdrawResponse 受理结果（异步回调最终态）.
type WithdrawResponse struct {
	BatchNo  string
	Status   string // accepted / rejected
	Reason   string
}

// ReconcileEntry 单笔对账行.
type ReconcileEntry struct {
	OutTradeNo string
	Amount     int64
	Status     string
}

// LicensedProvider 持牌方接口（Q12 决策前以 Stub 实现）.
type LicensedProvider interface {
	Topup(ctx context.Context, req TopupRequest) (TopupResponse, error)
	VerifyCallback(payload CallbackPayload) error
	Withdraw(ctx context.Context, req WithdrawRequest) (WithdrawResponse, error)
	Reconcile(ctx context.Context, day time.Time) ([]ReconcileEntry, error)
	Name() string
}

// HMACStubProvider 本地 stub，签名采用 HMAC-SHA256(secret, out_trade_no|amount|status).
//
// 不要在生产用；只用于 dev / 单测。Q12 决策后用真实持牌方替换。
type HMACStubProvider struct {
	ProviderName string
	SecretKey    []byte

	mu      sync.Mutex
	intents map[string]TopupRequest // out_trade_no → req
}

// NewHMACStubProvider 构造.
func NewHMACStubProvider(name string, secret []byte) *HMACStubProvider {
	return &HMACStubProvider{
		ProviderName: name,
		SecretKey:    secret,
		intents:      make(map[string]TopupRequest),
	}
}

// Name 返回 provider 名（用于落 topup_intent.channel）.
func (p *HMACStubProvider) Name() string { return p.ProviderName }

// Topup 记录 intent 并返回 stub 跳转 URL.
func (p *HMACStubProvider) Topup(ctx context.Context, req TopupRequest) (TopupResponse, error) {
	if req.Amount <= 0 {
		return TopupResponse{}, fmt.Errorf("payment: amount must be positive")
	}
	if strings.TrimSpace(req.OutTradeNo) == "" {
		return TopupResponse{}, fmt.Errorf("payment: out_trade_no required")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.intents[req.OutTradeNo]; ok {
		if existing.Amount != req.Amount {
			return TopupResponse{}, fmt.Errorf("payment: out_trade_no replay with different amount")
		}
	}
	p.intents[req.OutTradeNo] = req
	return TopupResponse{
		PayURL:          "stub://" + p.ProviderName + "/" + req.OutTradeNo,
		ProviderTradeNo: "STUB-" + req.OutTradeNo,
		ExpiresAt:       time.Now().Add(15 * time.Minute),
	}, nil
}

// SignCallback 帮 dev / test 计算签名（生产 provider 会自带）.
func (p *HMACStubProvider) SignCallback(out, status string, amount int64) string {
	mac := hmac.New(sha256.New, p.SecretKey)
	fmt.Fprintf(mac, "%s|%d|%s", out, amount, status)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyCallback 校验回调签名 + 金额一致性.
func (p *HMACStubProvider) VerifyCallback(payload CallbackPayload) error {
	expect := p.SignCallback(payload.OutTradeNo, payload.Status, payload.Amount)
	if !hmac.Equal([]byte(expect), []byte(payload.Signature)) {
		return ErrInvalidSignature
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if intent, ok := p.intents[payload.OutTradeNo]; ok {
		if intent.Amount != payload.Amount {
			return ErrAmountMismatch
		}
	}
	return nil
}

// Withdraw stub 立即受理.
func (p *HMACStubProvider) Withdraw(ctx context.Context, req WithdrawRequest) (WithdrawResponse, error) {
	if req.Amount <= 0 {
		return WithdrawResponse{}, fmt.Errorf("payment: amount must be positive")
	}
	return WithdrawResponse{
		BatchNo: "STUB-" + req.OutBatchNo,
		Status:  "accepted",
	}, nil
}

// Reconcile stub 返回空（生产对接：拉对账文件并 diff）.
func (p *HMACStubProvider) Reconcile(ctx context.Context, day time.Time) ([]ReconcileEntry, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ReconcileEntry, 0, len(p.intents))
	for _, in := range p.intents {
		out = append(out, ReconcileEntry{OutTradeNo: in.OutTradeNo, Amount: in.Amount, Status: "success"})
	}
	return out, nil
}
