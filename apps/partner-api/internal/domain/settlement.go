// PRD §8.8 settlement / settlement_item / settlement_run / settlement_config_change_log.
package domain

import "time"

// Settlement PRD §8.8。
type Settlement struct {
	ID              int64
	Period          string
	PeriodStart     time.Time
	PeriodEnd       time.Time
	Timezone        string
	TotalRevenue    int64
	TotalCost       int64
	TotalPayout     int64
	Status          string // generating / generated / paying / paid / failed / partially_disputed / gate_failed
	ProgressOffset  int64
	GeneratedAt     *time.Time
	PaidAt          *time.Time
	PaidBy          *int64
	PaymentEvidence string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// SettlementItem PRD §8.8 — partner-level 分账明细。
type SettlementItem struct {
	ID              int64
	SettlementID    int64
	PartnerID       int64
	Revenue         int64
	Cost            int64
	PlatformFee     int64
	WithheldTax     int64
	Payout          int64
	TaxEvidenceURL  string
	Status          string // pending / paid / disputed / failed
	ProviderTradeNo string
	PayoutEvidence  string
	InvoiceID       *int64
	IsPartial       bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// SettlementRun 心跳与 leader take-over（backend §6 settlement-runner）。
type SettlementRun struct {
	ID             int64
	SettlementID   int64
	Hostname       string
	PID            int
	StartedAt      time.Time
	LastHeartbeat  time.Time
	LeaseExpiresAt time.Time
	EndedAt        *time.Time
	Status         string
}
