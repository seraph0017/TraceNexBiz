// MySQL GORM 实现（W0 scaffold）。
// W1a 在本目录下增补每张表的 row 类型 + 转换函数 + 具体方法。
package mysql

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository"
)

// partnerRow 是 partner_db.partner 表行映射（仅未加密字段；加密字段 W1a 增 cipher/key_id 列）。
type partnerRow struct {
	ID             int64  `gorm:"primaryKey"`
	FyUserID       int64  `gorm:"column:fy_user_id;uniqueIndex"`
	InvitationCode string `gorm:"column:invitation_code;uniqueIndex"`
	Status         string `gorm:"column:status"`
	// TODO(W1a): per backend §3.1 — 列出全部列 (status / kyc_type / kyc_status / kyc_expires_at /
	//            tier / contact_phone_cipher / contact_phone_key_id / contact_email / contact_email_hmac /
	//            tax_status / approved_at / frozen_at / terminated_at / created_at / updated_at / deleted_at ...).
}

func (partnerRow) TableName() string { return "partner" }

// PartnerRepository GORM 实现（W0 scaffold；仅 FindByID / FindByFyUserID）.
type PartnerRepository struct {
	db *gorm.DB
}

// NewPartnerRepository 构造函数（W1a 用 wire / 手写 DI 注入）.
func NewPartnerRepository(db *gorm.DB) *PartnerRepository { return &PartnerRepository{db: db} }

// FindByID per repository.PartnerRepository contract。
func (r *PartnerRepository) FindByID(ctx context.Context, id int64) (*domain.Partner, error) {
	var row partnerRow
	if err := r.db.WithContext(ctx).First(&row, "id = ? AND deleted_at IS NULL", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}
	return rowToPartner(&row), nil
}

// FindByFyUserID per repository.PartnerRepository contract。
func (r *PartnerRepository) FindByFyUserID(ctx context.Context, fyUserID int64) (*domain.Partner, error) {
	var row partnerRow
	if err := r.db.WithContext(ctx).First(&row, "fy_user_id = ? AND deleted_at IS NULL", fyUserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}
	return rowToPartner(&row), nil
}

func rowToPartner(r *partnerRow) *domain.Partner {
	// 返回新对象；调用方收到的 *domain.Partner 视为 read-only.
	return &domain.Partner{
		ID:             r.ID,
		FyUserID:       r.FyUserID,
		InvitationCode: r.InvitationCode,
		Status:         domain.PartnerStatus(r.Status),
		// TODO(W1a): full mapping per backend §3.1.
	}
}
