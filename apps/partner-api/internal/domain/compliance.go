// 合规相关：content_safety_event/report、pia_report、pipl_complaint、pipl_request、invoice/red_flush.
package domain

import "time"

// ContentSafetyEvent backend §3.23（Phase 2A schema，Phase 1 mock 但 freeze）.
type ContentSafetyEvent struct {
	ID                  int64
	FyUserID            int64
	Kind                string // input / output
	Provider            string // aliyun / tencent / mock
	PromptHash          string
	Category            string
	Score               float64
	Disposition         string // block / review / pass / warn
	ReviewedBy          *int64
	ReviewedAt          *time.Time
	ReportedTo12377At   *time.Time
	AuditLogID          *int64
	TraceID             string
	CreatedAt           time.Time
}

// ContentSafetyReport backend §3.24（24h SLA 上报通道）.
type ContentSafetyReport struct {
	ID              int64
	EventID         int64
	TargetAuthority string // 12377 / public_security / internal
	Payload         string // JSON
	Status          string
	SubmittedAt     *time.Time
	SLADueAt        time.Time
	ResponsePayload string
	RetryCount      int
	LastError       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// PIAReport backend §3.25（PIPL §55 + GB/T 39335-2020）.
type PIAReport struct {
	ID                int64
	Title             string
	Scope             string
	PurposeText       string
	ScopeText         string
	NecessityText     string
	LegalBasisText    string
	ImpactText        string
	RiskText          string
	MeasuresText      string
	ResidualRiskText  string
	ReportURL         string
	ValidFrom         time.Time
	ValidUntil        time.Time
	SignedByDPO       int64
	SignedAt          time.Time
	CreatedAt         time.Time
}

// PIPLComplaint backend §3.26（用户投诉受理通道；15d SLA）.
type PIPLComplaint struct {
	ID                 int64
	SubjectFyUserID    *int64
	ContactEmail       string
	ContactPhonePlain  string
	Category           string // erase / access / rectify / consent_withdrawal / other
	Description        string
	Status             string
	AssignedTo         *int64
	SLADueAt           time.Time
	ResolutionText     string
	ResolvedAt         *time.Time
	AuditLogID         *int64
	TraceID            string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// PIPLRequest backend §3.27（用户主动行使权利工单流；30d SLA）.
type PIPLRequest struct {
	ID                int64
	ActorType         string // customer / partner
	ActorID           int64
	FyUserID          *int64
	RequestType       string // access / rectify / erase / restrict / port
	State             string
	Deadline          time.Time
	CompletedDeadline time.Time
	Reason            string
	RejectionReason   string
	ExportOSSKey      string
	AuditLogID        *int64
	TraceID           string
	SubmittedAt       time.Time
	CompletedAt       *time.Time
}

// InvoiceTitle PRD §8.12。
type InvoiceTitle struct {
	ID         int64
	OwnerType  string // partner / customer
	OwnerID    int64
	TitleType  int8   // 1=个人 2=企业
	Title      string
	TaxNumber  string
	BankInfo   string
	IsDefault  bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// InvoiceApplication PRD §8.12 + Compliance HIGH-6。
type InvoiceApplication struct {
	ID                 int64
	ApplicantType      string
	ApplicantID        int64
	TitleID            int64
	SellerEntityID     int64
	SellerTaxNo        string
	Amount             int64
	Period             string
	Status             string // applied / reviewing / issuing / issued / rejected / red_flushing / red_flushed
	InvoiceURL         string
	FapiaoSerial       string
	MailAddress        string
	RedFlushRequestID  *int64
	AppliedAt          time.Time
	IssuedAt           *time.Time
	ArchiveExpiresAt   time.Time
	Notes              string
	RejectReasonCode   string
	RejectReasonText   string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// RedFlushRequest backend §3.12 末（Compliance MED-17）.
type RedFlushRequest struct {
	ID                int64
	OriginalInvoiceID int64
	RedFapiaoSerial   string
	ReasonCode        string
	ReasonText        string
	Status            string
	RequestedBy       int64
	RequestedAt       time.Time
	ConfirmedAt       *time.Time
	CompletedAt       *time.Time
	CreatedAt         time.Time
}
