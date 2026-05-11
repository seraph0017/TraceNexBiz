// PRD §8.3 / §8.4 / §8.5 / §8.22 wallet + log + hold + debt。
package domain

import "time"

// PartnerWallet PRD §8.3（v0.2 drop held_amount，由 wallet_hold 计算）。
type PartnerWallet struct {
	ID            int64
	PartnerID     int64
	Balance       int64 // quota 单位
	PaidOutTotal  int64
	Version       int64 // 乐观锁
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// WalletLogType PRD §8.4 v0.2 ARCH HIGH-8 复合键。
type WalletLogType string

const (
	WalletLogRevenueAccrual         WalletLogType = "revenue_accrual"
	WalletLogAllocateToCustomer     WalletLogType = "allocate_to_customer"
	WalletLogSettlementPayout       WalletLogType = "settlement_payout"
	WalletLogRefundClawback         WalletLogType = "refund_clawback"
	WalletLogAdjustment             WalletLogType = "adjustment"
	WalletLogSagaAbortedUnknown     WalletLogType = "saga_aborted_unknown"
	WalletLogInitialTopup           WalletLogType = "initial_topup"
	WalletLogPlatformISVCommission  WalletLogType = "platform_isv_commission_in"
)

// PartnerWalletLog PRD §8.4。
type PartnerWalletLog struct {
	ID             int64
	PartnerID      int64
	Type           WalletLogType
	Amount         int64
	BalanceAfter   int64
	RefID          string
	IdempotencyKey string
	Status         string // committed / pending / etc.
	Note           string
	OperatorType   string
	OperatorID     int64
	TraceID        string
	CreatedAt      time.Time
}

// WalletHold PRD §8.5 — saga 两阶段 hold/commit/release。
type WalletHold struct {
	ID         int64
	WalletID   int64
	PartnerID  int64
	Amount     int64
	SagaID     string // = idempotency_key
	Status     string // held / committed / released
	HeldAt     time.Time
	ResolvedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// PartnerDebt PRD §8.22 / ADR-010 verdict (Phase 2A)。
type PartnerDebt struct {
	ID        int64
	PartnerID int64
	Amount    int64
	Cause     string
	RefID     string
	Status    string // open / clearing / cleared / written_off
	ClearedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}
