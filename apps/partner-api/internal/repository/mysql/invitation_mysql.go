// invitation_mysql.go — GORM 实现 service/invitation.Repository。
package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/invitation"
)

// invitationCodeRow 对应 invitation_code 表（001 中创建）。
type invitationCodeRow struct {
	ID         int64      `gorm:"primaryKey;column:id"`
	PartnerID  int64      `gorm:"column:partner_id;index:idx_invitation_partner"`
	Code       string     `gorm:"column:code;uniqueIndex;size:64"`
	Type       string     `gorm:"column:type;size:16"`
	UsageLimit int32      `gorm:"column:usage_limit"`
	UsedCount  int32      `gorm:"column:used_count"`
	ExpiresAt  *time.Time `gorm:"column:expires_at"`
	Status     string     `gorm:"column:status;size:16"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at"`
}

func (invitationCodeRow) TableName() string { return "invitation_code" }

// InvitationRepository GORM 实现 service/invitation.Repository。
type InvitationRepository struct {
	db *gorm.DB
}

// NewInvitationRepository .
func NewInvitationRepository(db *gorm.DB) *InvitationRepository {
	return &InvitationRepository{db: db}
}

var _ invitation.Repository = (*InvitationRepository)(nil)

// Insert .
func (r *InvitationRepository) Insert(ctx context.Context, c domain.InvitationCode) (int64, error) {
	row := invitationToRow(c)
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
		row.UpdatedAt = row.CreatedAt
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

// FindByCode .
func (r *InvitationRepository) FindByCode(ctx context.Context, code string) (*domain.InvitationCode, error) {
	var row invitationCodeRow
	if err := r.db.WithContext(ctx).First(&row, "code = ?", code).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rowToInvitation(&row), nil
}

// IncUsedCount 原子自增；用 UPDATE … SET used_count = used_count + 1。
//
// 之后回读返当前行。注意：检查 usage_limit 由 caller (service.Resolve) 完成；
// 这里只做无条件 +1。但为防 over-consume，加 `used_count < usage_limit OR usage_limit = 0` 判定。
func (r *InvitationRepository) IncUsedCount(ctx context.Context, code string) (*domain.InvitationCode, error) {
	var result *domain.InvitationCode
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&invitationCodeRow{}).
			Where("code = ? AND (usage_limit = 0 OR used_count < usage_limit)", code).
			Updates(map[string]any{
				"used_count": gorm.Expr("used_count + 1"),
				"updated_at": time.Now().UTC(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// 区分 not-found vs limit-exhausted：再查一次。
			var row invitationCodeRow
			if err := tx.First(&row, "code = ?", code).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return repository.ErrNotFound
				}
				return err
			}
			return errors.New("invitation: usage_limit exhausted")
		}
		var row invitationCodeRow
		if err := tx.First(&row, "code = ?", code).Error; err != nil {
			return err
		}
		result = rowToInvitation(&row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Update updater 模式（按 code 查）。
func (r *InvitationRepository) Update(ctx context.Context, code string,
	updater func(domain.InvitationCode) domain.InvitationCode) (*domain.InvitationCode, error) {
	var result domain.InvitationCode
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row invitationCodeRow
		if err := selectForUpdate(tx).First(&row, "code = ?", code).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return repository.ErrNotFound
			}
			return err
		}
		current := rowToInvitation(&row)
		next := updater(*current)
		nextRow := invitationToRow(next)
		nextRow.ID = row.ID
		nextRow.Code = row.Code // code 是 PK-候选；禁止 updater 修改
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

// ListByPartner .
func (r *InvitationRepository) ListByPartner(ctx context.Context, partnerID int64) ([]domain.InvitationCode, error) {
	var rows []invitationCodeRow
	if err := r.db.WithContext(ctx).
		Where("partner_id = ?", partnerID).
		Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.InvitationCode, 0, len(rows))
	for i := range rows {
		out = append(out, *rowToInvitation(&rows[i]))
	}
	return out, nil
}

func rowToInvitation(r *invitationCodeRow) *domain.InvitationCode {
	return &domain.InvitationCode{
		ID:         r.ID,
		PartnerID:  r.PartnerID,
		Code:       r.Code,
		Type:       r.Type,
		UsageLimit: r.UsageLimit,
		UsedCount:  r.UsedCount,
		ExpiresAt:  r.ExpiresAt,
		Status:     r.Status,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}
}

func invitationToRow(c domain.InvitationCode) invitationCodeRow {
	return invitationCodeRow{
		ID:         c.ID,
		PartnerID:  c.PartnerID,
		Code:       c.Code,
		Type:       c.Type,
		UsageLimit: c.UsageLimit,
		UsedCount:  c.UsedCount,
		ExpiresAt:  c.ExpiresAt,
		Status:     c.Status,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
	}
}
