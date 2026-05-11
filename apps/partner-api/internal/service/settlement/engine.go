// Package settlement 是 partner-api 月度结算引擎（PRD §7.5 + §8.8 + backend §6 settlement-runner）.
//
// 状态机：
//
//	draft       初始（settlement_run claim leader 后建）
//	  │
//	locked      聚合完成 + freshness gate 通过 → 锁定，不再可改
//	  │
//	paying      下发持牌方分账（saga）
//	  │
//	paid        全部 item 分账成功
//	  │
//	reconciled  T+1 对账金额一致
//
// 关键 invariant：
//   1. lock 后 settlement / settlement_item 字段不可改（PRD §7.5 v0.2.2）
//   2. freshness gate：locked 之前必须 outbox lag ≤ 阈值（backend §9.3）
//   3. 个税代扣：personal 渠道商先扣 withheld_tax 再 payout（PRD §15.4）
//   4. payout_evidence 必须落 OSS + 哈希入库（finance audit）
//   5. settlement.id 仍为 BIGINT（不是 UUIDv7）—— PRD §3.8 既定字段类型，未在 ARCH-D 范围.
package settlement

import (
	"errors"
	"fmt"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// 错误.
var (
	ErrAlreadyLocked       = errors.New("settlement: already locked, immutable")
	ErrInvalidTransition   = errors.New("settlement: invalid status transition")
	ErrFreshnessGateFailed = errors.New("settlement: freshness gate failed; outbox lag exceeded")
	ErrEmptyPayoutEvidence = errors.New("settlement: payout_evidence required for paid")
)

// Status 状态枚举.
type Status string

const (
	StatusDraft        Status = "generating"
	StatusGenerated    Status = "generated"
	StatusLocked       Status = "locked"
	StatusPaying       Status = "paying"
	StatusPaid         Status = "paid"
	StatusFailed       Status = "failed"
	StatusGateFailed   Status = "gate_failed"
	StatusPartialDispute Status = "partially_disputed"
)

// PartnerKind 渠道商主体类型，影响个税代扣逻辑.
type PartnerKind string

const (
	PartnerKindCorporate  PartnerKind = "corporate"  // 企业；不代扣个税
	PartnerKindPersonal   PartnerKind = "personal"   // 个人；按规则代扣 20%
	PartnerKindIndividual PartnerKind = "individual" // 个体工商户；不代扣
)

// 个税：personal 渠道商按 (revenue - cost) 的 20% 代扣（PRD §15.4 placeholder；最终值由财务终审落地）.
const (
	PersonalWithholdRate = 20 // %
)

// Aggregate 聚合输入：一个 partner 在 period 内的 revenue_log 汇总.
type Aggregate struct {
	PartnerID     int64
	PartnerKind   PartnerKind
	RevenueTotal  int64
	CostTotal     int64
	Adjustments   int64 // 显式调整（occurrence > 1）
}

// ItemDraft 计算结果：尚未落库的 settlement_item 草稿.
type ItemDraft struct {
	PartnerID    int64
	Revenue      int64
	Cost         int64
	PlatformFee  int64
	WithheldTax  int64
	Payout       int64
}

// ComputeItem 纯函数：Aggregate → ItemDraft（不变量；调用方 lock 阶段调）.
//
// 公式（占位；ADR-Q10 决议前以此为准）：
//   gross   = RevenueTotal + Adjustments
//   net     = max(0, gross - CostTotal)
//   platformFee = 0      // 平台抽成留 Q10 决议
//   tax     = personal ? net * 20% : 0
//   payout  = net - platformFee - tax
func ComputeItem(in Aggregate) ItemDraft {
	gross := in.RevenueTotal + in.Adjustments
	net := gross - in.CostTotal
	if net < 0 {
		net = 0
	}
	platformFee := int64(0)
	tax := int64(0)
	if in.PartnerKind == PartnerKindPersonal {
		tax = (net - platformFee) * PersonalWithholdRate / 100
	}
	payout := net - platformFee - tax
	if payout < 0 {
		payout = 0
	}
	return ItemDraft{
		PartnerID:   in.PartnerID,
		Revenue:     gross,
		Cost:        in.CostTotal,
		PlatformFee: platformFee,
		WithheldTax: tax,
		Payout:      payout,
	}
}

// CanTransition 状态机校验.
func CanTransition(from, to Status) bool {
	switch from {
	case StatusDraft:
		return to == StatusGenerated || to == StatusGateFailed
	case StatusGenerated:
		return to == StatusLocked || to == StatusGateFailed
	case StatusLocked:
		return to == StatusPaying
	case StatusPaying:
		return to == StatusPaid || to == StatusFailed || to == StatusPartialDispute
	case StatusPaid:
		return to == StatusPartialDispute
	}
	return false
}

// Transition 不变 settlement：返回 settlement 副本，含新 status / 时间戳.
//
// 调用方负责 DB UPDATE WHERE status=from（乐观锁）.
func Transition(s domain.Settlement, to Status, now time.Time) (domain.Settlement, error) {
	if !CanTransition(Status(s.Status), to) {
		return s, fmt.Errorf("from=%s to=%s: %w", s.Status, to, ErrInvalidTransition)
	}
	out := s
	out.Status = string(to)
	out.UpdatedAt = now
	switch to {
	case StatusGenerated:
		t := now
		out.GeneratedAt = &t
	case StatusPaid:
		t := now
		out.PaidAt = &t
	}
	return out, nil
}

// AssertLockable freshness gate 通过 + 状态可锁.
func AssertLockable(s domain.Settlement, gateOK bool) error {
	if !gateOK {
		return ErrFreshnessGateFailed
	}
	if Status(s.Status) == StatusLocked || Status(s.Status) == StatusPaying || Status(s.Status) == StatusPaid {
		return ErrAlreadyLocked
	}
	return nil
}

// AssertPayable paid 前必须有 evidence.
func AssertPayable(item domain.SettlementItem, evidence string) error {
	if evidence == "" {
		return ErrEmptyPayoutEvidence
	}
	if item.Status != "pending" {
		return fmt.Errorf("settlement: item %d not pending: %w", item.ID, ErrInvalidTransition)
	}
	return nil
}
