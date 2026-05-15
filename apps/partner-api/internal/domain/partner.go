// Package domain 是 partner-api 领域模型层（POJO；不含 GORM tag）。
// 与 repository / service 解耦：repository 层做 entity ↔ row 映射，
// service 层只见 domain 类型（与 user 全局规则 immutability 对齐）。
//
// 本文件覆盖 PRD §8.1 / §8.10 partner + invitation_code。
package domain

import (
	"errors"
	"time"
)

// ErrInvalidTaxStatus 非 ValidTaxStatuses 集合内的写入值（Fix-C item 5）.
var ErrInvalidTaxStatus = errors.New("domain: invalid tax_status (must be one of: individual / sole_proprietor / partnership / llc / corp)")

// PartnerStatus 见 PRD §14.1。
type PartnerStatus string

const (
	PartnerStatusApplied    PartnerStatus = "applied"
	PartnerStatusReviewing  PartnerStatus = "reviewing"
	PartnerStatusApproved   PartnerStatus = "approved"
	PartnerStatusRejected   PartnerStatus = "rejected"
	PartnerStatusFrozen     PartnerStatus = "frozen"
	PartnerStatusSuspended  PartnerStatus = "suspended"
	PartnerStatusTerminated PartnerStatus = "terminated"
)

// TaxStatus v0.2 Compliance HIGH-1（Fix-C item 5：5 枚举对齐 PRD §19）。
//
// 当前枚举（migration 015）：individual / sole_proprietor / partnership / llc / corp.
// 旧枚举常量（individual_business / company / unknown）保留 const 仅为兼容历史代码读取；
// 新写入路径必须经 ValidTaxStatus 校验，只接受 5 个有效值之一。
type TaxStatus string

const (
	TaxIndividual     TaxStatus = "individual"
	TaxSoleProprietor TaxStatus = "sole_proprietor"
	TaxPartnership    TaxStatus = "partnership"
	TaxLLC            TaxStatus = "llc"
	TaxCorp           TaxStatus = "corp"

	// Deprecated: kept for read-back of pre-migration rows; do NOT use for new writes.
	TaxIndividualBusiness TaxStatus = "individual_business"
	// Deprecated.
	TaxCompany TaxStatus = "company"
	// Deprecated.
	TaxUnknown TaxStatus = "unknown"
)

// ValidTaxStatuses 写入校验集合（migration 015 enum）.
var ValidTaxStatuses = map[TaxStatus]struct{}{
	TaxIndividual:     {},
	TaxSoleProprietor: {},
	TaxPartnership:    {},
	TaxLLC:            {},
	TaxCorp:           {},
}

// ValidateTaxStatus 写入路径调；返回 ErrInvalidTaxStatus 时调用方应拒绝.
func ValidateTaxStatus(t TaxStatus) error {
	if _, ok := ValidTaxStatuses[t]; !ok {
		return ErrInvalidTaxStatus
	}
	return nil
}

// Partner 渠道商主体（PRD §8.1）。
//
// 不可变约定：service / repository 返回的 *Partner 必须视为 read-only；
// 修改通过 PartnerRepository.Update(ctx, partnerID, updater func(p Partner) Partner) 模式。
type Partner struct {
	ID                  int64
	FyUserID            int64
	InvitationCode      string
	Status              PartnerStatus
	KYCType             int8 // 0=未认证 1=企业 2=个人
	KYCStatus           int8 // 0..4
	KYCExpiresAt        *time.Time
	DefaultRevenueShare float64 // 兼容旧字段
	Tier                int8    // 0..9
	AppliedAt           time.Time
	ApprovedAt          *time.Time
	ApprovedBy          *int64
	ContactName         string
	// PII：手机号 KMS 信封加密（backend §3.1 + §9）
	ContactPhonePlain  string // 仅 service 内瞬态；不出库不入日志
	ContactPhoneCipher []byte
	ContactPhoneKeyID  string
	ContactEmail       string
	ContactEmailHMAC   string // v0.2 ARCH M-8.1：HMAC(email) 索引
	TaxStatus          TaxStatus
	Notes              string
	SettlementProvider *int64
	ProviderSubAccount string
	FrozenAt           *time.Time
	FrozenReason       string
	TerminatedAt       *time.Time
	TerminatedReason   string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// InvitationCode PRD §8.10。
type InvitationCode struct {
	ID         int64
	PartnerID  int64
	Code       string
	Type       string // permanent / one_time / limited
	UsageLimit int32
	UsedCount  int32
	ExpiresAt  *time.Time
	Status     string // active / expired / revoked
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
