// PRD §8.6 / §8.7 pricing_rule + revenue_log.
package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

// PartnerPricingRule PRD §8.6 — markup 多维（partner / customer / model / tier / time-window）。
type PartnerPricingRule struct {
	ID         int64
	PartnerID  int64
	CustomerID *int64
	ModelName  *string
	TierName   *string
	Markup     decimal.Decimal // [1.0, 5.0]
	ValidFrom  time.Time
	ValidTo    *time.Time
	Status     string // active / archived / draft
	CreatedBy  int64
	Note       string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// RevenueLog PRD §8.7 — outbox 消费写入。
type RevenueLog struct {
	ID            int64
	PartnerID     int64
	CustomerID    int64
	FyAPILogID    int64 // 关联 fy_api_db.logs.id（不建 FK）
	Occurrence    int8  // 1=正常 2+=显式调整
	GrossAmount   int64
	CostAmount    int64
	NetRevenue    int64
	AppliedRuleID int64
	OccurredAt    time.Time
	SettlementID  *int64
	TraceID       string
	CreatedAt     time.Time
}
