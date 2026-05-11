// PRD §8.2 / §8.20 customer + customer_partner_change_log。
package domain

import "time"

// CustomerStatus PRD §14.2。
type CustomerStatus string

const (
	CustomerStatusActive       CustomerStatus = "active"
	CustomerStatusDisabled     CustomerStatus = "disabled"
	CustomerStatusTransferred  CustomerStatus = "transferred"
	CustomerStatusOrphaned     CustomerStatus = "orphaned"
	CustomerStatusAdopted      CustomerStatus = "adopted"
	CustomerStatusDirect       CustomerStatus = "direct"
	CustomerStatusDeleted      CustomerStatus = "deleted"
)

// Customer PRD §8.2。
type Customer struct {
	ID                 int64
	FyUserID           int64
	PartnerID          *int64 // NULL = 直营
	JoinedVia          string // invitation / manual_create / self_register_with_code / direct
	InvitationCodeUsed string
	Status             CustomerStatus
	GroupNameInFyAPI   string // partner_X_tier_Y
	QuotaLimit         int64  // 0 = 不限
	TransferredFrom    *int64
	TransferredAt      *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// CustomerPartnerChangeLog PRD §8.20 / 场景 H + I。
type CustomerPartnerChangeLog struct {
	ID            int64
	CustomerID    int64
	FromPartnerID *int64
	ToPartnerID   *int64
	InitiatorType string // customer / staff / system_termination
	InitiatorID   int64
	Status        string // pending_a / pending_b / pending_staff / completed / rejected / cooldown
	Reason        string
	OccurredAt    time.Time
	OldGroup      string
	NewGroup      string
}
