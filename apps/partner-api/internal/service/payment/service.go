package payment

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// IntentRepo 持久化 topup_intent（内存实现用于单测；GORM 实现 W1a/W1b 落）.
type IntentRepo interface {
	Insert(ctx context.Context, in *Intent) error
	GetByOutTradeNo(ctx context.Context, out string) (*Intent, error)
	UpdateState(ctx context.Context, out, state string, paidAt *time.Time, providerTradeNo string) error
}

// Intent 内存表示.
type Intent struct {
	OutTradeNo      string
	ActorType       string
	ActorID         int64
	Amount          int64
	Channel         string
	State           string
	PaidAt          *time.Time
	ProviderTradeNo string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Service 协调 provider + intent repo + audit + outbox.
type Service struct {
	provider       LicensedProvider
	repo           IntentRepo
	clock          func() time.Time
	expectedMchID  string // Fix-C P1-4 ISV 模式反向断言；空串 = 跳过（dev / 非 ISV 模式）
}

// NewService 构造.
func NewService(p LicensedProvider, r IntentRepo) *Service {
	return &Service{provider: p, repo: r, clock: time.Now}
}

// SetExpectedMchID 启动期由 main.go 从 biz_setting `payment.platform_isv_mchid` 注入。
//
// 非空时 HandleCallback 将断言 payload.MchID == expectedMchID；不一致 → ErrMchIDMismatch。
// 这是防止其他 mchid 的 webhook 被重放到本 partner-api 实例的关键 SEC 控制（Compliance M-2）.
func (s *Service) SetExpectedMchID(mchID string) { s.expectedMchID = mchID }

// CreateTopup 入口：创建 intent + 调 provider + 返回跳转 URL.
//
// 幂等键：OutTradeNo（service 调用方传入 saga_id）.
func (s *Service) CreateTopup(ctx context.Context, req TopupRequest) (TopupResponse, error) {
	if req.OutTradeNo == "" {
		return TopupResponse{}, errors.New("payment: out_trade_no required")
	}
	now := s.clock()
	in := &Intent{
		OutTradeNo: req.OutTradeNo,
		ActorType:  req.ActorType,
		ActorID:    req.ActorID,
		Amount:     req.Amount,
		Channel:    req.Channel,
		State:      "created",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.repo.Insert(ctx, in); err != nil {
		return TopupResponse{}, fmt.Errorf("payment: insert intent: %w", err)
	}
	resp, err := s.provider.Topup(ctx, req)
	if err != nil {
		return TopupResponse{}, fmt.Errorf("payment: provider topup: %w", err)
	}
	return resp, nil
}

// HandleCallback 验签 + 状态机推进 (created → paid).
//
// 调用方负责再走 funded（Fy-api 充值落账）路径——本函数只把 paid 标记落库.
func (s *Service) HandleCallback(ctx context.Context, payload CallbackPayload) error {
	// Fix-C P1-4: ISV mch_id reverse assertion — payload 携带的 mch_id 必须等于
	// 本实例配置的 expectedMchID（biz_setting payment.platform_isv_mchid）.
	if s.expectedMchID != "" && payload.MchID != s.expectedMchID {
		return ErrMchIDMismatch
	}
	if err := s.provider.VerifyCallback(payload); err != nil {
		return err
	}
	intent, err := s.repo.GetByOutTradeNo(ctx, payload.OutTradeNo)
	if err != nil {
		return fmt.Errorf("payment: lookup intent: %w", err)
	}
	if intent.Amount != payload.Amount {
		return ErrAmountMismatch
	}
	if intent.State == "paid" || intent.State == "funded" {
		return nil // 重放幂等
	}
	if payload.Status != "success" {
		return s.repo.UpdateState(ctx, payload.OutTradeNo, "failed", nil, payload.ProviderTradeNo)
	}
	paidAt := payload.PaidAt
	return s.repo.UpdateState(ctx, payload.OutTradeNo, "paid", &paidAt, payload.ProviderTradeNo)
}

// ReconcileDay 拉持牌方对账文件 vs intent 表，返出不一致条目列表（金额对账，PRD §7.6）.
func (s *Service) ReconcileDay(ctx context.Context, day time.Time) ([]ReconcileEntry, error) {
	return s.provider.Reconcile(ctx, day)
}

// MemoryIntentRepo 内存实现（单测用）.
type MemoryIntentRepo struct {
	mu      sync.Mutex
	intents map[string]*Intent
}

// NewMemoryIntentRepo 构造.
func NewMemoryIntentRepo() *MemoryIntentRepo {
	return &MemoryIntentRepo{intents: make(map[string]*Intent)}
}

// Insert OutTradeNo 唯一；重复返错（service 层做幂等命中 → 重放）.
func (r *MemoryIntentRepo) Insert(ctx context.Context, in *Intent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.intents[in.OutTradeNo]; ok {
		return errors.New("payment: duplicate out_trade_no")
	}
	cp := *in
	r.intents[in.OutTradeNo] = &cp
	return nil
}

// GetByOutTradeNo 不存在返错.
func (r *MemoryIntentRepo) GetByOutTradeNo(ctx context.Context, out string) (*Intent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.intents[out]; ok {
		cp := *v
		return &cp, nil
	}
	return nil, errors.New("payment: intent not found")
}

// UpdateState 状态推进.
func (r *MemoryIntentRepo) UpdateState(ctx context.Context, out, state string, paidAt *time.Time, providerTradeNo string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.intents[out]
	if !ok {
		return errors.New("payment: intent not found")
	}
	v.State = state
	if paidAt != nil {
		v.PaidAt = paidAt
	}
	if providerTradeNo != "" {
		v.ProviderTradeNo = providerTradeNo
	}
	v.UpdatedAt = time.Now()
	return nil
}
