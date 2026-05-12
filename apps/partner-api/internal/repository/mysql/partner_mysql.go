// Package mysql MySQL GORM 实现（W1b）。
//
// 命名：每张表一个 row 类型 + table.go；row tag 用 GORM v2 风格；
// row → domain 转换在同一文件内（rowToX / xToRow）。
//
// 跨方言：partner-api 标定 MySQL ≥ 5.7.8 / PostgreSQL ≥ 9.6 / SQLite（测试用）。
// 故 raw SQL 选 ANSI 子集；不使用 MySQL `ON DUPLICATE KEY` / PG `RETURNING` 等方言扩展。
// 行锁通过 selectForUpdate(tx) 包装：在 MySQL/PG 上加 SELECT ... FOR UPDATE，
// 在 SQLite 上 no-op（SQLite 整库写锁，TX 已足够）。
//
// 软删除：partner / customer / kyc 走 `gorm.DeletedAt` 列；wallet / wallet_hold / wallet_log
// 不软删（append-mostly）。
package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/partner"
)

// selectForUpdate 在 MySQL/PG 上 SELECT ... FOR UPDATE；SQLite 跳过（不支持 / 不需要）。
func selectForUpdate(tx *gorm.DB) *gorm.DB {
	switch tx.Dialector.Name() {
	case "mysql", "postgres":
		return tx.Clauses(clause.Locking{Strength: "UPDATE"})
	default:
		return tx
	}
}

// partnerRow 是 partner_db.partner 表行映射（完整列；与 001_partner_core.up.sql 对齐）。
type partnerRow struct {
	ID                  int64          `gorm:"primaryKey;column:id"`
	FyUserID            int64          `gorm:"column:fy_user_id;uniqueIndex"`
	InvitationCode     string         `gorm:"column:invitation_code;uniqueIndex;size:64"`
	Status              string         `gorm:"column:status;size:32;index:idx_partner_status"`
	KYCType             int8           `gorm:"column:kyc_type"`
	KYCStatus           int8           `gorm:"column:kyc_status;index:idx_partner_kyc"`
	KYCExpiresAt        *time.Time     `gorm:"column:kyc_expires_at"`
	DefaultRevenueShare float64        `gorm:"column:default_revenue_share"`
	Tier                int8           `gorm:"column:tier"`
	AppliedAt           time.Time      `gorm:"column:applied_at"`
	ApprovedAt          *time.Time     `gorm:"column:approved_at"`
	ApprovedBy          *int64         `gorm:"column:approved_by"`
	ContactName         string         `gorm:"column:contact_name;size:64"`
	ContactPhoneCipher  []byte         `gorm:"column:contact_phone_cipher"`
	ContactPhoneKeyID   string         `gorm:"column:contact_phone_key_id;size:128"`
	ContactEmail        string         `gorm:"column:contact_email;size:128"`
	ContactEmailHMAC    string         `gorm:"column:contact_email_hmac;size:64;uniqueIndex"`
	TaxStatus           string         `gorm:"column:tax_status;size:32"`
	Notes               string         `gorm:"column:notes;type:text"`
	SettlementProvider  *int64         `gorm:"column:settlement_provider_id"`
	ProviderSubAccount  string         `gorm:"column:provider_sub_account_id;size:128"`
	FrozenAt            *time.Time     `gorm:"column:frozen_at"`
	FrozenReason        string         `gorm:"column:frozen_reason;type:text"`
	TerminatedAt        *time.Time     `gorm:"column:terminated_at"`
	TerminatedReason    string         `gorm:"column:terminated_reason;type:text"`
	CreatedAt           time.Time      `gorm:"column:created_at"`
	UpdatedAt           time.Time      `gorm:"column:updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (partnerRow) TableName() string { return "partner" }

// PartnerRepository GORM 实现 — 实现 service/partner.Repository。
type PartnerRepository struct {
	db *gorm.DB
}

// NewPartnerRepository .
func NewPartnerRepository(db *gorm.DB) *PartnerRepository { return &PartnerRepository{db: db} }

// 编译期保证：实现 service/partner.Repository。
var _ partner.Repository = (*PartnerRepository)(nil)

// Insert 写入新 partner；返 ID。
func (r *PartnerRepository) Insert(ctx context.Context, p domain.Partner) (int64, error) {
	row := partnerToRow(p)
	if row.AppliedAt.IsZero() {
		row.AppliedAt = time.Now().UTC()
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

// FindByID 主键查；soft-deleted 行不返回（gorm.DeletedAt 自动加 deleted_at IS NULL）。
func (r *PartnerRepository) FindByID(ctx context.Context, id int64) (*domain.Partner, error) {
	var row partnerRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // service.Get 把 nil 映射为 ErrPartnerNotFound
		}
		return nil, err
	}
	return rowToPartner(&row), nil
}

// FindByFyUserID fy_user_id 反查。
func (r *PartnerRepository) FindByFyUserID(ctx context.Context, fyUserID int64) (*domain.Partner, error) {
	var row partnerRow
	if err := r.db.WithContext(ctx).First(&row, "fy_user_id = ?", fyUserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rowToPartner(&row), nil
}

// FindByEmailHMAC contact_email_hmac 反查。
func (r *PartnerRepository) FindByEmailHMAC(ctx context.Context, hmac string) (*domain.Partner, error) {
	if hmac == "" {
		return nil, nil
	}
	var row partnerRow
	if err := r.db.WithContext(ctx).First(&row, "contact_email_hmac = ?", hmac).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rowToPartner(&row), nil
}

// Update updater 模式；SELECT FOR UPDATE → updater → UPDATE 同 TX。
func (r *PartnerRepository) Update(ctx context.Context, id int64, updater func(domain.Partner) domain.Partner) (*domain.Partner, error) {
	var result domain.Partner
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row partnerRow
		if err := selectForUpdate(tx).First(&row, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return repository.ErrNotFound
			}
			return err
		}
		current := rowToPartner(&row)
		next := updater(*current)
		nextRow := partnerToRow(next)
		nextRow.ID = id
		nextRow.CreatedAt = row.CreatedAt
		nextRow.UpdatedAt = time.Now().UTC()
		// Save 走 UPDATE（PK 已存在）。
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

// List per partner.ListFilter；status / search 简单 LIKE invitation_code 前缀。
func (r *PartnerRepository) List(ctx context.Context, f partner.ListFilter) ([]domain.Partner, error) {
	q := r.db.WithContext(ctx).Model(&partnerRow{})
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Search != "" {
		q = q.Where("invitation_code LIKE ?", f.Search+"%")
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	q = q.Order("id DESC").Limit(f.Limit).Offset(f.Offset)
	var rows []partnerRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Partner, 0, len(rows))
	for i := range rows {
		out = append(out, *rowToPartner(&rows[i]))
	}
	return out, nil
}

func rowToPartner(r *partnerRow) *domain.Partner {
	return &domain.Partner{
		ID:                  r.ID,
		FyUserID:            r.FyUserID,
		InvitationCode:      r.InvitationCode,
		Status:              domain.PartnerStatus(r.Status),
		KYCType:             r.KYCType,
		KYCStatus:           r.KYCStatus,
		KYCExpiresAt:        r.KYCExpiresAt,
		DefaultRevenueShare: r.DefaultRevenueShare,
		Tier:                r.Tier,
		AppliedAt:           r.AppliedAt,
		ApprovedAt:          r.ApprovedAt,
		ApprovedBy:          r.ApprovedBy,
		ContactName:         r.ContactName,
		ContactPhoneKeyID:   r.ContactPhoneKeyID,
		ContactEmail:        r.ContactEmail,
		ContactEmailHMAC:    r.ContactEmailHMAC,
		TaxStatus:           domain.TaxStatus(r.TaxStatus),
		Notes:               r.Notes,
		SettlementProvider:  r.SettlementProvider,
		ProviderSubAccount:  r.ProviderSubAccount,
		FrozenAt:            r.FrozenAt,
		FrozenReason:        r.FrozenReason,
		TerminatedAt:        r.TerminatedAt,
		TerminatedReason:    r.TerminatedReason,
		CreatedAt:           r.CreatedAt,
		UpdatedAt:           r.UpdatedAt,
	}
}

func partnerToRow(p domain.Partner) partnerRow {
	taxStatus := string(p.TaxStatus)
	if taxStatus == "" {
		taxStatus = string(domain.TaxUnknown)
	}
	return partnerRow{
		ID:                  p.ID,
		FyUserID:            p.FyUserID,
		InvitationCode:      p.InvitationCode,
		Status:              string(p.Status),
		KYCType:             p.KYCType,
		KYCStatus:           p.KYCStatus,
		KYCExpiresAt:        p.KYCExpiresAt,
		DefaultRevenueShare: p.DefaultRevenueShare,
		Tier:                p.Tier,
		AppliedAt:           p.AppliedAt,
		ApprovedAt:          p.ApprovedAt,
		ApprovedBy:          p.ApprovedBy,
		ContactName:         p.ContactName,
		ContactPhoneKeyID:   p.ContactPhoneKeyID,
		ContactEmail:        p.ContactEmail,
		ContactEmailHMAC:    p.ContactEmailHMAC,
		TaxStatus:           taxStatus,
		Notes:               p.Notes,
		SettlementProvider:  p.SettlementProvider,
		ProviderSubAccount:  p.ProviderSubAccount,
		FrozenAt:            p.FrozenAt,
		FrozenReason:        p.FrozenReason,
		TerminatedAt:        p.TerminatedAt,
		TerminatedReason:    p.TerminatedReason,
		CreatedAt:           p.CreatedAt,
		UpdatedAt:           p.UpdatedAt,
	}
}
