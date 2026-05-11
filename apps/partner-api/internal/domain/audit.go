// PRD §8.13 audit_log + audit_log_pii + audit_log_unsealed（hash chain；HIGH-r2-1 重写）.
package domain

import "time"

// AuditLogUnsealed 应用 INSERT 队列；sealer 消费 → audit_log。
type AuditLogUnsealed struct {
	ID               int64
	ActorType        string
	ActorID          int64
	Action           string
	TargetType       string
	TargetID         int64
	TargetKey        string
	DiffRedacted     string
	DiffPIIID        *int64
	IPAddress        string
	UserAgent        string
	TraceID          string
	SecondApproverID *int64 // dual-control SEC CRIT-5
	OccurredAt       time.Time
}

// AuditLog 哈希链最终表（id 与 unsealed.id 1:1；非 AUTO_INCREMENT）。
type AuditLog struct {
	ID               int64
	ActorType        string
	ActorID          int64
	Action           string
	TargetType       string
	TargetID         int64
	TargetKey        string
	DiffRedacted     string
	DiffPIIID        *int64
	IPAddress        string
	UserAgent        string
	TraceID          string
	SecondApproverID *int64
	OccurredAt       time.Time
	PrevHash         string
	SelfHash         string
	SealedAt         time.Time
}

// AuditLogPII 加密侧表（PIPL §47 删除时 tombstoned）。
type AuditLogPII struct {
	ID              int64
	DiffCipher      []byte
	DiffOSSRef      string
	EncryptionKeyID string
	TombstonedAt    *time.Time
	CreatedAt       time.Time
}
