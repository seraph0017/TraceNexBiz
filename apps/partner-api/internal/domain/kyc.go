// PRD §8.9 KYC + §8.18 consent_log.
package domain

import "time"

// KYCApplication PRD §8.9（PII 信封加密 + blind index）。
type KYCApplication struct {
	ID             int64
	FyUserID       int64
	Type           int8 // 1=企业 2=个人
	Status         string
	BusinessLicenseURL string

	// 加密字段：明文仅 service 内瞬态；持久化层只看 cipher / key_id / blind_index
	BusinessLicenseOCRPlain string
	BusinessLicenseOCRKeyID string

	LegalPersonNamePlain      string
	LegalPersonNameKeyID      string
	LegalPersonNameBlindIndex string

	LegalPersonIDPlain      string
	LegalPersonIDKeyID      string
	LegalPersonIDBlindIndex string
	LegalPersonIDURL        string
	LegalPersonIDArchiveURL string

	AlipayOpenIDPlain   string
	AlipayOpenIDKeyID   string
	AlipayRealNamePlain string
	AlipayRealNameKeyID string

	BankAccountPlain      string
	BankAccountKeyID      string
	BankAccountBlindIndex string

	BiometricLivenessURL string
	BiometricPurgedAt    *time.Time
	YearlyRejectCount    int8
	YearlyRejectResetAt  *time.Time

	SubmittedAt          *time.Time
	ReviewedAt           *time.Time
	ReviewedBy           *int64
	RejectReasonCode     string
	RejectReasonText     string
	PIIPurgedAt          *time.Time
	ColdArchiveExpiresAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ConsentType PRD §8.18 + Compliance HIGH-2。
type ConsentType string

const (
	ConsentPrivacyPolicy     ConsentType = "privacy_policy"
	ConsentSensitivePI       ConsentType = "sensitive_pi"
	ConsentBiometric         ConsentType = "biometric"
	ConsentCrossBorder       ConsentType = "cross_border"
	ConsentDeviceFingerprint ConsentType = "device_fingerprint"
	ConsentAutomatedDecision ConsentType = "automated_decision"
	ConsentThirdPartyShare   ConsentType = "third_party_share"
)

// ConsentLog PRD §8.18。
type ConsentLog struct {
	ID                 int64
	SubjectFyUserID    int64
	ConsentType        ConsentType
	ConsentTextVersion string
	ConsentedAt        time.Time
	IP                 string
	UserAgent          string
	Withdrawn          bool
	WithdrawnAt        *time.Time
}
