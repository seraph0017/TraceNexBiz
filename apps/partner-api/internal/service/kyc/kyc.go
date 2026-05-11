// Package kyc 实现申请创建 + 三方核验 stub + 加密落库 + 状态机。
//
// 引用：PRD §7.7 / §8.9 / Compliance NEW-2，backend §3.9 / §5.6 / §9。
//
// 状态机：draft → submitted → under_review → approved / rejected / expiring / expired / frozen_yearly_limit。
// 关键 invariant：
//   - PII 字段（legal_person_id / bank_account / alipay_open_id）信封加密；明文不入库
//   - blind_index = HMAC(salt, plain) 用于全局唯一约束（同一身份证不能多个申请）
//   - upload 走 OSS presigned PUT；mime / size / Content-Type / magic-byte 二次校验（W1a infra/oss）
//   - 30 天后 hot purge：清明文 + 移动到 OSS Archive；5 年后 cold purge（cron `kyc.purge.cold`）
//   - 年度驳回 ≥ 3 次 → frozen_yearly_limit
package kyc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// 状态枚举。
const (
	StatusDraft        = "draft"
	StatusSubmitted    = "submitted"
	StatusUnderReview  = "under_review"
	StatusApproved     = "approved"
	StatusRejected     = "rejected"
	StatusExpiring     = "expiring"
	StatusExpired      = "expired"
	StatusFrozenYrly   = "frozen_yearly_limit"
)

// Type 申请类型。
const (
	TypeEnterprise int8 = 1
	TypeIndividual int8 = 2
)

// Sentinel.
var (
	ErrKYCNotFound       = errors.New("kyc: application not found")
	ErrInvalidTransition = errors.New("kyc: invalid status transition")
	ErrYearlyLimit       = errors.New("kyc: yearly reject limit exceeded")
	ErrConsentRequired   = errors.New("kyc: consent required (sensitive_pi + biometric)")
	ErrFileNotVerified   = errors.New("kyc: uploaded file failed magic-byte / mime check")
)

// SubmitInput 客户提交 KYC 申请。
//
// PII 字段（legal_person_id / bank_account / alipay_open_id）以明文承载；
// service 在加密后立即清空内存（runtime.GC + zero []byte 由 W1a infra/kms 实现）。
type SubmitInput struct {
	FyUserID              int64
	Type                  int8
	BusinessLicenseURL    string
	BusinessLicenseOCR    string // OCR 结果文本（W1c 接 OCR 后回填）
	LegalPersonName       string
	LegalPersonID         string
	LegalPersonIDURL      string
	BankAccount           string // Phase 2A
	AlipayOpenID          string
	AlipayRealName        string
	BiometricLivenessURL  string
	ConsentSensitivePIID  int64
	ConsentBiometricID    int64
}

// ApprovalInput staff 审核入参。
type ApprovalInput struct {
	StaffID          int64
	Approve          bool
	RejectReasonCode string
	RejectReasonText string
}

// Repository KYC 持久化端口。
type Repository interface {
	Insert(ctx context.Context, app domain.KYCApplication) (int64, error)
	FindByFyUserID(ctx context.Context, fyUserID int64) (*domain.KYCApplication, error)
	FindByID(ctx context.Context, id int64) (*domain.KYCApplication, error)
	FindByLegalIDBlindIndex(ctx context.Context, blindIndex string) (*domain.KYCApplication, error)
	Update(ctx context.Context, id int64, updater func(domain.KYCApplication) domain.KYCApplication) (*domain.KYCApplication, error)
	ListPendingReview(ctx context.Context, limit int) ([]domain.KYCApplication, error)
	PurgeColdExpired(ctx context.Context, before time.Time, limit int) (int, error)
}

// CryptoPort KMS 信封加密 + HMAC blind index。
type CryptoPort interface {
	Encrypt(ctx context.Context, scope string, plain []byte) (cipher []byte, keyID string, err error)
	HMAC(ctx context.Context, scope string, plain []byte) ([]byte, error)
}

// OCRPort 三方 OCR + 实名核验（W1c 接 Aliyun OCR / 公安二要素）。
type OCRPort interface {
	ParseBusinessLicense(ctx context.Context, ossURL string) (companyName, legalPerson, regNo string, err error)
	VerifyIDName(ctx context.Context, name, idNo string) (matched bool, err error)
}

// OSSPort 上传校验（W1a infra/oss.Service.VerifyMagicBytes）。
type OSSPort interface {
	VerifyKYCObject(ctx context.Context, ossURL, expectedMime string) error
}

// ConsentPort 校验 sensitive_pi + biometric 同意。
type ConsentPort interface {
	HasConsent(ctx context.Context, consentID int64) (bool, error)
}

// PartnerLinker KYC 通过后回写 partner.kyc_status / kyc_type / kyc_expires_at。
type PartnerLinker interface {
	OnKYCApproved(ctx context.Context, fyUserID int64, kycType int8, expiresAt time.Time) error
}

// Service KYC 门面。
type Service struct {
	repo    Repository
	crypto  CryptoPort
	ocr     OCRPort
	oss     OSSPort
	consent ConsentPort
	link    PartnerLinker
	clock   func() time.Time
	hotTTL  time.Duration // 默认 30d
	coldTTL time.Duration // 默认 5y
}

// NewService .
func NewService(repo Repository, crypto CryptoPort, ocr OCRPort, oss OSSPort,
	consent ConsentPort, linker PartnerLinker) *Service {
	return &Service{
		repo: repo, crypto: crypto, ocr: ocr, oss: oss,
		consent: consent, link: linker, clock: time.Now,
		hotTTL: 30 * 24 * time.Hour, coldTTL: 5 * 365 * 24 * time.Hour,
	}
}

// WithClock 测试注入。
func (s *Service) WithClock(c func() time.Time) *Service { s.clock = c; return s }

// Submit 提交 KYC 申请；service 不持久化明文。
func (s *Service) Submit(ctx context.Context, in SubmitInput) (*domain.KYCApplication, error) {
	if err := s.validateInput(in); err != nil {
		return nil, err
	}
	if err := s.checkConsents(ctx, in); err != nil {
		return nil, err
	}
	if err := s.verifyUploads(ctx, in); err != nil {
		return nil, err
	}
	app, err := s.encryptInto(ctx, in)
	if err != nil {
		return nil, err
	}
	if existing, _ := s.repo.FindByLegalIDBlindIndex(ctx, app.LegalPersonIDBlindIndex); existing != nil && existing.FyUserID != in.FyUserID {
		return nil, fmt.Errorf("kyc: legal_person_id already used by other applicant")
	}
	now := s.clock()
	app.SubmittedAt = ptrTime(now)
	app.Status = StatusSubmitted
	app.CreatedAt = now
	app.UpdatedAt = now
	id, err := s.repo.Insert(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("kyc: insert: %w", err)
	}
	app.ID = id
	return &app, nil
}

// MarkUnderReview staff 一审接单 → 状态推 under_review。
func (s *Service) MarkUnderReview(ctx context.Context, id int64) (*domain.KYCApplication, error) {
	updated, err := s.repo.Update(ctx, id, func(a domain.KYCApplication) domain.KYCApplication {
		if a.Status != StatusSubmitted {
			return a
		}
		a.Status = StatusUnderReview
		return a
	})
	if err != nil {
		return nil, fmt.Errorf("kyc: mark review: %w", err)
	}
	if updated.Status != StatusUnderReview {
		return nil, ErrInvalidTransition
	}
	return updated, nil
}

// Review staff 审核（approve / reject）。
func (s *Service) Review(ctx context.Context, id int64, in ApprovalInput) (*domain.KYCApplication, error) {
	app, err := s.repo.FindByID(ctx, id)
	if err != nil || app == nil {
		return nil, ErrKYCNotFound
	}
	if app.YearlyRejectCount >= 3 && !in.Approve {
		return nil, ErrYearlyLimit
	}
	if !in.Approve && app.YearlyRejectCount+1 >= 3 {
		return s.rejectFreeze(ctx, app, in)
	}
	now := s.clock()
	updated, err := s.repo.Update(ctx, id, func(a domain.KYCApplication) domain.KYCApplication {
		if a.Status != StatusUnderReview && a.Status != StatusSubmitted {
			return a
		}
		a.ReviewedAt = ptrTime(now)
		a.ReviewedBy = &in.StaffID
		if in.Approve {
			a.Status = StatusApproved
		} else {
			a.Status = StatusRejected
			a.RejectReasonCode = in.RejectReasonCode
			a.RejectReasonText = in.RejectReasonText
			a.YearlyRejectCount++
			a.YearlyRejectResetAt = ptrTime(now.AddDate(1, 0, 0))
		}
		return a
	})
	if err != nil {
		return nil, fmt.Errorf("kyc: review: %w", err)
	}
	if updated.Status == StatusApproved {
		if err := s.link.OnKYCApproved(ctx, updated.FyUserID, updated.Type, now.AddDate(1, 0, 0)); err != nil {
			return updated, fmt.Errorf("kyc: link approved: %w", err)
		}
	}
	return updated, nil
}

func (s *Service) rejectFreeze(ctx context.Context, app *domain.KYCApplication, in ApprovalInput) (*domain.KYCApplication, error) {
	now := s.clock()
	updated, err := s.repo.Update(ctx, app.ID, func(a domain.KYCApplication) domain.KYCApplication {
		a.Status = StatusFrozenYrly
		a.ReviewedAt = ptrTime(now)
		a.ReviewedBy = &in.StaffID
		a.RejectReasonCode = in.RejectReasonCode
		a.RejectReasonText = in.RejectReasonText
		a.YearlyRejectCount++
		a.YearlyRejectResetAt = ptrTime(now.AddDate(1, 0, 0))
		return a
	})
	if err != nil {
		return nil, fmt.Errorf("kyc: freeze yearly: %w", err)
	}
	return updated, nil
}

// PurgeCold cron `kyc.purge.cold`：5 年到期 → 物理销毁。
func (s *Service) PurgeCold(ctx context.Context, batch int) (int, error) {
	if batch <= 0 {
		batch = 100
	}
	cutoff := s.clock()
	n, err := s.repo.PurgeColdExpired(ctx, cutoff, batch)
	if err != nil {
		return 0, fmt.Errorf("kyc: purge cold: %w", err)
	}
	return n, nil
}

// FindByFyUser 给 partner / customer 查自己的 KYC。
func (s *Service) FindByFyUser(ctx context.Context, fyUserID int64) (*domain.KYCApplication, error) {
	a, err := s.repo.FindByFyUserID(ctx, fyUserID)
	if err != nil {
		return nil, fmt.Errorf("kyc: find by fy_user_id: %w", err)
	}
	if a == nil {
		return nil, ErrKYCNotFound
	}
	return a, nil
}

// 私有 helper

func (s *Service) validateInput(in SubmitInput) error {
	if in.FyUserID <= 0 {
		return errors.New("kyc: fy_user_id required")
	}
	if in.Type != TypeEnterprise && in.Type != TypeIndividual {
		return errors.New("kyc: type must be 1 (enterprise) or 2 (individual)")
	}
	if strings.TrimSpace(in.LegalPersonName) == "" || strings.TrimSpace(in.LegalPersonID) == "" {
		return errors.New("kyc: legal_person_name and id required")
	}
	if strings.TrimSpace(in.LegalPersonIDURL) == "" {
		return errors.New("kyc: legal_person_id_url required")
	}
	if in.Type == TypeEnterprise && strings.TrimSpace(in.BusinessLicenseURL) == "" {
		return errors.New("kyc: enterprise must provide business_license_url")
	}
	return nil
}

func (s *Service) checkConsents(ctx context.Context, in SubmitInput) error {
	ok, err := s.consent.HasConsent(ctx, in.ConsentSensitivePIID)
	if err != nil || !ok {
		return ErrConsentRequired
	}
	if in.BiometricLivenessURL != "" {
		ok, err := s.consent.HasConsent(ctx, in.ConsentBiometricID)
		if err != nil || !ok {
			return ErrConsentRequired
		}
	}
	return nil
}

func (s *Service) verifyUploads(ctx context.Context, in SubmitInput) error {
	if in.BusinessLicenseURL != "" {
		if err := s.oss.VerifyKYCObject(ctx, in.BusinessLicenseURL, "image/jpeg"); err != nil {
			return ErrFileNotVerified
		}
	}
	if err := s.oss.VerifyKYCObject(ctx, in.LegalPersonIDURL, "image/jpeg"); err != nil {
		return ErrFileNotVerified
	}
	if in.BiometricLivenessURL != "" {
		if err := s.oss.VerifyKYCObject(ctx, in.BiometricLivenessURL, "video/mp4"); err != nil {
			return ErrFileNotVerified
		}
	}
	return nil
}

func (s *Service) encryptInto(ctx context.Context, in SubmitInput) (domain.KYCApplication, error) {
	a := domain.KYCApplication{
		FyUserID: in.FyUserID, Type: in.Type, Status: StatusDraft,
		BusinessLicenseURL: in.BusinessLicenseURL,
		LegalPersonIDURL:   in.LegalPersonIDURL,
		BiometricLivenessURL: in.BiometricLivenessURL,
	}
	if in.LegalPersonName != "" {
		_, kid, err := s.crypto.Encrypt(ctx, "kyc:legal_person_name", []byte(in.LegalPersonName))
		if err != nil {
			return a, fmt.Errorf("kyc: encrypt name: %w", err)
		}
		a.LegalPersonNameKeyID = kid
		bi, err := s.crypto.HMAC(ctx, "kyc:legal_person_name", []byte(in.LegalPersonName))
		if err != nil {
			return a, fmt.Errorf("kyc: hmac name: %w", err)
		}
		a.LegalPersonNameBlindIndex = bytesToHex(bi)
	}
	_, kidID, err := s.crypto.Encrypt(ctx, "kyc:legal_person_id", []byte(in.LegalPersonID))
	if err != nil {
		return a, fmt.Errorf("kyc: encrypt id: %w", err)
	}
	a.LegalPersonIDKeyID = kidID
	bi, err := s.crypto.HMAC(ctx, "kyc:legal_person_id", []byte(in.LegalPersonID))
	if err != nil {
		return a, fmt.Errorf("kyc: hmac id: %w", err)
	}
	a.LegalPersonIDBlindIndex = bytesToHex(bi)
	if in.BankAccount != "" {
		_, kidB, err := s.crypto.Encrypt(ctx, "kyc:bank_account", []byte(in.BankAccount))
		if err != nil {
			return a, fmt.Errorf("kyc: encrypt bank: %w", err)
		}
		a.BankAccountKeyID = kidB
		bib, err := s.crypto.HMAC(ctx, "kyc:bank_account", []byte(in.BankAccount))
		if err != nil {
			return a, fmt.Errorf("kyc: hmac bank: %w", err)
		}
		a.BankAccountBlindIndex = bytesToHex(bib)
	}
	if in.AlipayOpenID != "" {
		_, kidA, err := s.crypto.Encrypt(ctx, "kyc:alipay_open_id", []byte(in.AlipayOpenID))
		if err != nil {
			return a, fmt.Errorf("kyc: encrypt alipay: %w", err)
		}
		a.AlipayOpenIDKeyID = kidA
	}
	if in.BusinessLicenseOCR != "" {
		_, kidO, err := s.crypto.Encrypt(ctx, "kyc:ocr", []byte(in.BusinessLicenseOCR))
		if err != nil {
			return a, fmt.Errorf("kyc: encrypt ocr: %w", err)
		}
		a.BusinessLicenseOCRKeyID = kidO
	}
	now := s.clock()
	cold := now.Add(s.coldTTL)
	a.ColdArchiveExpiresAt = &cold
	return a, nil
}

func ptrTime(t time.Time) *time.Time { return &t }

func bytesToHex(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hex[v>>4]
		out[i*2+1] = hex[v&0x0f]
	}
	return string(out)
}
