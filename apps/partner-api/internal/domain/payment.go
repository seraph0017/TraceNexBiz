// 支付：topup_intent (Phase 2A) + seat (Phase 3).
package domain

import "time"

// TopupIntent backend §3.21（Phase 2A，PRD §22.1 F-3 saga）.
type TopupIntent struct {
	ID              int64
	CustomerID      int64
	Amount          int64
	Channel         string
	OutTradeNo      string
	State           string // created / paid / funded / refunded / failed / canceled
	PaidAt          *time.Time
	FundedAt        *time.Time
	SagaID          string // UUIDv7；用作 Idempotency-Key 透传 Fy-api
	ProviderTradeNo string
	CallbackPayload string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Seat PRD §8.11（Phase 3）.
type Seat struct {
	ID          int64
	OwnerType   string // partner / customer
	OwnerID     int64
	Name        string
	FyTokenID   int64
	PurchasedAt time.Time
	ExpiresAt   time.Time
	Status      string // active / expired / revoked
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
