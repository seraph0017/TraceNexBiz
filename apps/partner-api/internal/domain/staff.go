// PRD §8.14 + §8.15 staff + biz_setting + password_reset_token + idempotency_record + saga_step + audit.
package domain

import "time"

// StaffRole PRD §3.2。
type StaffRole string

const (
	RoleSuperAdmin StaffRole = "super_admin"
	RoleOperations StaffRole = "operations"
	RoleFinance    StaffRole = "finance"
	RoleSupport    StaffRole = "support"
)

// Staff PRD §8.14。
type Staff struct {
	ID              int64
	Username        string
	PasswordHash    string // argon2id
	Role            StaffRole
	Email           string
	Status          string
	LastLogin       *time.Time
	MFASecretPlain  string // 仅 service 内瞬态
	MFASecretKeyID  string
	WebAuthnCreds   string // JSON
	ElevatedUntil   *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// BizSetting PRD §8.15。
type BizSetting struct {
	Key         string
	Value       string
	ValueType   string // plain / secret_ref
	Description string
	UpdatedAt   time.Time
	UpdatedBy   int64
}

// PasswordResetToken PRD §17.5 + backend §3.28（双因子 + 单次使用 + 15 min TTL）。
type PasswordResetToken struct {
	ID                 int64
	ActorType          string // partner / customer / staff
	ActorID            int64
	TokenHash          string // SHA-256 of 32-byte random
	SecondFactorType   string // email / sms
	SecondFactorHash   string
	RequestedIP        string
	UserAgent          string
	ExpiresAt          time.Time
	ConsumedAt         *time.Time
	FailedAttempts     int
	InvalidatedAt      *time.Time
	AuditLogID         *int64
	TraceID            string
	CreatedAt          time.Time
}

// IdempotencyRecord PRD §8.16。
//
// Fix-B' part 2 (CRIT-B3): same-TX co-commit semantics.
// ResponseBody 与 ResponseCipher 互斥：phase-1 走 ResponseBody 明文，Fix-C KMS 真接入后切 ResponseCipher。
type IdempotencyRecord struct {
	ID             int64
	ActorType      string
	ActorID        int64
	IdempotencyKey string
	Endpoint       string
	RequestHash    string
	ResponseStatus int
	ResponseHash   string
	ResponseBody   string // Phase-1 plaintext; KMS-encrypted path uses ResponseCipher instead.
	ResponseCipher []byte
	ResponseKeyID  string
	TraceID        string
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

// SagaStep PRD §8.17。
type SagaStep struct {
	ID             int64
	SagaID         string // = idempotency_key
	StepName       string
	Status         string // pending / in_progress / committed / compensated / failed / escalated / released_pessimistic
	Attempts       int
	LastError      string
	Payload        string // JSON; PII scrubber must hit
	StartedAt      *time.Time
	UpdatedAt      time.Time
	EscalatedAt    *time.Time
	EscalateReason string
	TraceID        string
	CreatedAt      time.Time
}
