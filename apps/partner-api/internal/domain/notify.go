// PRD §8.18 ticket / ticket_reply / notification_outbox.
package domain

import "time"

// Ticket PRD §8.18（v0.2 ARCH-MED-11：扩 content_report 类目）。
type Ticket struct {
	ID          int64
	OpenerType  string // partner / customer / staff
	OpenerID    int64
	Subject     string
	Category    string // billing / kyc / api / content_report / other
	Status      string // open / assigned / responding / waiting_user / resolved / closed / reopened
	AssignedTo  *int64
	Priority    int8
	LastReplyAt *time.Time
	SLADueAt    *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TicketReply PRD §8.18。
type TicketReply struct {
	ID          int64
	TicketID    int64
	SenderType  string
	SenderID    int64
	BodyMD      string
	Attachments string // JSON
	CreatedAt   time.Time
}

// NotificationOutbox PRD §8.18 + v1.0 cosmetic #3 (ref_id 防重)。
type NotificationOutbox struct {
	ID          int64
	Recipient   string
	Channel     string // email / inapp / sms / webhook
	EventCode   string
	RefID       string // 业务侧关联键 (saga_id / ticket_id / topup_intent.saga_id ...)
	Payload     string
	Status      string // pending / sent / failed / dead_letter
	RetryCount  int
	LastError   string
	DispatchedAt *time.Time
	TraceID     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
