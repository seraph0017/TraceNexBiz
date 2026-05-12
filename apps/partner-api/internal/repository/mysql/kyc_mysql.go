// kyc_mysql.go — GORM 实现 service/kyc.Repository。
package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository"
	kycsvc "github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/kyc"
)

// kycApplicationRow 对应 kyc_application 表（005_kyc_invoice_seat.up.sql）。
type kycApplicationRow struct {
	ID                        int64          `gorm:"primaryKey;column:id"`
	FyUserID                  int64          `gorm:"column:fy_user_id;uniqueIndex"`
	Type                      int8           `gorm:"column:type"`
	Status                    string         `gorm:"column:status;size:32;index:idx_kyc_status"`
	BusinessLicenseURL        string         `gorm:"column:business_license_url;size:1024"`
	BusinessLicenseOCRCipher  []byte         `gorm:"column:business_license_ocr_cipher"`
	BusinessLicenseOCRKeyID   string         `gorm:"column:business_license_ocr_key_id;size:128"`
	LegalPersonNameCipher     []byte         `gorm:"column:legal_person_name_cipher"`
	LegalPersonNameKeyID      string         `gorm:"column:legal_person_name_key_id;size:128"`
	LegalPersonNameBlindIndex string         `gorm:"column:legal_person_name_blind_index;size:64"`
	LegalPersonIDCipher       []byte         `gorm:"column:legal_person_id_cipher"`
	LegalPersonIDKeyID        string         `gorm:"column:legal_person_id_key_id;size:128"`
	LegalPersonIDBlindIndex   string         `gorm:"column:legal_person_id_blind_index;size:64;index:idx_kyc_legal_id_blind"`
	LegalPersonIDURL          string         `gorm:"column:legal_person_id_url;size:1024"`
	LegalPersonIDArchiveURL   string         `gorm:"column:legal_person_id_archive_url;size:1024"`
	AlipayOpenIDCipher        []byte         `gorm:"column:alipay_open_id_cipher"`
	AlipayOpenIDKeyID         string         `gorm:"column:alipay_open_id_key_id;size:128"`
	AlipayRealNameCipher      []byte         `gorm:"column:alipay_real_name_cipher"`
	AlipayRealNameKeyID       string         `gorm:"column:alipay_real_name_key_id;size:128"`
	BankAccountCipher         []byte         `gorm:"column:bank_account_cipher"`
	BankAccountKeyID          string         `gorm:"column:bank_account_key_id;size:128"`
	BankAccountBlindIndex     string         `gorm:"column:bank_account_blind_index;size:64;index:idx_kyc_bank_acct_blind"`
	BiometricLivenessURL      string         `gorm:"column:biometric_liveness_url;size:1024"`
	BiometricPurgedAt         *time.Time     `gorm:"column:biometric_purged_at"`
	YearlyRejectCount         int8           `gorm:"column:yearly_reject_count"`
	YearlyRejectResetAt       *time.Time     `gorm:"column:yearly_reject_reset_at"`
	SubmittedAt               *time.Time     `gorm:"column:submitted_at"`
	ReviewedAt                *time.Time     `gorm:"column:reviewed_at"`
	ReviewedBy                *int64         `gorm:"column:reviewed_by"`
	RejectReasonCode          string         `gorm:"column:reject_reason_code;size:64"`
	RejectReasonText          string         `gorm:"column:reject_reason_text;type:text"`
	PIIPurgedAt               *time.Time     `gorm:"column:pii_purged_at"`
	ColdArchiveExpiresAt      *time.Time     `gorm:"column:cold_archive_expires_at"`
	CreatedAt                 time.Time      `gorm:"column:created_at"`
	UpdatedAt                 time.Time      `gorm:"column:updated_at"`
	DeletedAt                 gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (kycApplicationRow) TableName() string { return "kyc_application" }

// KYCRepository GORM 实现 service/kyc.Repository。
type KYCRepository struct {
	db *gorm.DB
}

// NewKYCRepository .
func NewKYCRepository(db *gorm.DB) *KYCRepository { return &KYCRepository{db: db} }

var _ kycsvc.Repository = (*KYCRepository)(nil)

// Insert .
func (r *KYCRepository) Insert(ctx context.Context, a domain.KYCApplication) (int64, error) {
	row := kycToRow(a)
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
		row.UpdatedAt = row.CreatedAt
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

// FindByFyUserID .
func (r *KYCRepository) FindByFyUserID(ctx context.Context, fyUserID int64) (*domain.KYCApplication, error) {
	var row kycApplicationRow
	if err := r.db.WithContext(ctx).First(&row, "fy_user_id = ?", fyUserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rowToKYC(&row), nil
}

// FindByID .
func (r *KYCRepository) FindByID(ctx context.Context, id int64) (*domain.KYCApplication, error) {
	var row kycApplicationRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rowToKYC(&row), nil
}

// FindByLegalIDBlindIndex .
func (r *KYCRepository) FindByLegalIDBlindIndex(ctx context.Context, bi string) (*domain.KYCApplication, error) {
	if bi == "" {
		return nil, nil
	}
	var row kycApplicationRow
	if err := r.db.WithContext(ctx).First(&row, "legal_person_id_blind_index = ?", bi).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rowToKYC(&row), nil
}

// Update updater 模式。
func (r *KYCRepository) Update(ctx context.Context, id int64,
	updater func(domain.KYCApplication) domain.KYCApplication) (*domain.KYCApplication, error) {
	var result domain.KYCApplication
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row kycApplicationRow
		if err := selectForUpdate(tx).First(&row, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return repository.ErrNotFound
			}
			return err
		}
		current := rowToKYC(&row)
		next := updater(*current)
		nextRow := kycToRow(next)
		nextRow.ID = id
		nextRow.CreatedAt = row.CreatedAt
		nextRow.UpdatedAt = time.Now().UTC()
		if err := tx.Save(&nextRow).Error; err != nil {
			return err
		}
		result = next
		result.UpdatedAt = nextRow.UpdatedAt
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ListPendingReview .
func (r *KYCRepository) ListPendingReview(ctx context.Context, limit int) ([]domain.KYCApplication, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []kycApplicationRow
	if err := r.db.WithContext(ctx).
		Where("status IN ?", []string{kycsvc.StatusSubmitted, kycsvc.StatusUnderReview}).
		Order("submitted_at ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.KYCApplication, 0, len(rows))
	for i := range rows {
		out = append(out, *rowToKYC(&rows[i]))
	}
	return out, nil
}

// PurgeColdExpired cold_archive_expires_at 早于 before → 硬删；返删除条数。
func (r *KYCRepository) PurgeColdExpired(ctx context.Context, before time.Time, limit int) (int, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	// 先查 IDs，再批量删；保证 SQL 跨方言（PG / SQLite 没 LIMIT for DELETE）。
	var ids []int64
	if err := r.db.WithContext(ctx).
		Model(&kycApplicationRow{}).
		Where("cold_archive_expires_at IS NOT NULL AND cold_archive_expires_at < ?", before).
		Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Unscoped().Delete(&kycApplicationRow{}, "id IN ?", ids)
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}

func rowToKYC(r *kycApplicationRow) *domain.KYCApplication {
	return &domain.KYCApplication{
		ID:                        r.ID,
		FyUserID:                  r.FyUserID,
		Type:                      r.Type,
		Status:                    r.Status,
		BusinessLicenseURL:        r.BusinessLicenseURL,
		BusinessLicenseOCRKeyID:   r.BusinessLicenseOCRKeyID,
		LegalPersonNameKeyID:      r.LegalPersonNameKeyID,
		LegalPersonNameBlindIndex: r.LegalPersonNameBlindIndex,
		LegalPersonIDKeyID:        r.LegalPersonIDKeyID,
		LegalPersonIDBlindIndex:   r.LegalPersonIDBlindIndex,
		LegalPersonIDURL:          r.LegalPersonIDURL,
		LegalPersonIDArchiveURL:   r.LegalPersonIDArchiveURL,
		AlipayOpenIDKeyID:         r.AlipayOpenIDKeyID,
		AlipayRealNameKeyID:       r.AlipayRealNameKeyID,
		BankAccountKeyID:          r.BankAccountKeyID,
		BankAccountBlindIndex:     r.BankAccountBlindIndex,
		BiometricLivenessURL:      r.BiometricLivenessURL,
		BiometricPurgedAt:         r.BiometricPurgedAt,
		YearlyRejectCount:         r.YearlyRejectCount,
		YearlyRejectResetAt:       r.YearlyRejectResetAt,
		SubmittedAt:               r.SubmittedAt,
		ReviewedAt:                r.ReviewedAt,
		ReviewedBy:                r.ReviewedBy,
		RejectReasonCode:          r.RejectReasonCode,
		RejectReasonText:          r.RejectReasonText,
		PIIPurgedAt:               r.PIIPurgedAt,
		ColdArchiveExpiresAt:      r.ColdArchiveExpiresAt,
		CreatedAt:                 r.CreatedAt,
		UpdatedAt:                 r.UpdatedAt,
	}
}

func kycToRow(a domain.KYCApplication) kycApplicationRow {
	return kycApplicationRow{
		ID:                        a.ID,
		FyUserID:                  a.FyUserID,
		Type:                      a.Type,
		Status:                    a.Status,
		BusinessLicenseURL:        a.BusinessLicenseURL,
		BusinessLicenseOCRKeyID:   a.BusinessLicenseOCRKeyID,
		LegalPersonNameKeyID:      a.LegalPersonNameKeyID,
		LegalPersonNameBlindIndex: a.LegalPersonNameBlindIndex,
		LegalPersonIDKeyID:        a.LegalPersonIDKeyID,
		LegalPersonIDBlindIndex:   a.LegalPersonIDBlindIndex,
		LegalPersonIDURL:          a.LegalPersonIDURL,
		LegalPersonIDArchiveURL:   a.LegalPersonIDArchiveURL,
		AlipayOpenIDKeyID:         a.AlipayOpenIDKeyID,
		AlipayRealNameKeyID:       a.AlipayRealNameKeyID,
		BankAccountKeyID:          a.BankAccountKeyID,
		BankAccountBlindIndex:     a.BankAccountBlindIndex,
		BiometricLivenessURL:      a.BiometricLivenessURL,
		BiometricPurgedAt:         a.BiometricPurgedAt,
		YearlyRejectCount:         a.YearlyRejectCount,
		YearlyRejectResetAt:       a.YearlyRejectResetAt,
		SubmittedAt:               a.SubmittedAt,
		ReviewedAt:                a.ReviewedAt,
		ReviewedBy:                a.ReviewedBy,
		RejectReasonCode:          a.RejectReasonCode,
		RejectReasonText:          a.RejectReasonText,
		PIIPurgedAt:               a.PIIPurgedAt,
		ColdArchiveExpiresAt:      a.ColdArchiveExpiresAt,
		CreatedAt:                 a.CreatedAt,
		UpdatedAt:                 a.UpdatedAt,
	}
}
