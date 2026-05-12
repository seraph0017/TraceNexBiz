// customer_mysql.go — GORM 实现 service/customer.Repository。
package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/customer"
)

// customerRow 对应 001_partner_core.up.sql 的 `customer` 表。
type customerRow struct {
	ID                 int64          `gorm:"primaryKey;column:id"`
	FyUserID           int64          `gorm:"column:fy_user_id;uniqueIndex"`
	PartnerID          *int64         `gorm:"column:partner_id;index:idx_customer_partner"`
	JoinedVia          string         `gorm:"column:joined_via;size:32"`
	InvitationCodeUsed string         `gorm:"column:invitation_code_used;size:64"`
	Status             string         `gorm:"column:status;size:32;index:idx_customer_status"`
	GroupNameInFyAPI   string         `gorm:"column:group_name_in_fy_api;size:128"`
	QuotaLimit         int64          `gorm:"column:quota_limit"`
	TransferredFrom    *int64         `gorm:"column:transferred_from"`
	TransferredAt      *time.Time     `gorm:"column:transferred_at"`
	CreatedAt          time.Time      `gorm:"column:created_at"`
	UpdatedAt          time.Time      `gorm:"column:updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (customerRow) TableName() string { return "customer" }

// customerPartnerChangeLogRow 对应 customer_partner_change_log。
type customerPartnerChangeLogRow struct {
	ID            int64     `gorm:"primaryKey;column:id"`
	CustomerID    int64     `gorm:"column:customer_id;index"`
	FromPartnerID *int64    `gorm:"column:from_partner_id"`
	ToPartnerID   *int64    `gorm:"column:to_partner_id"`
	InitiatorType string    `gorm:"column:initiator_type;size:16"`
	InitiatorID   int64     `gorm:"column:initiator_id"`
	Status        string    `gorm:"column:status;size:32"`
	Reason        string    `gorm:"column:reason;type:text"`
	OccurredAt    time.Time `gorm:"column:occurred_at"`
	OldGroup      string    `gorm:"column:old_group;size:128"`
	NewGroup      string    `gorm:"column:new_group;size:128"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (customerPartnerChangeLogRow) TableName() string { return "customer_partner_change_log" }

// CustomerRepository GORM 实现 service/customer.Repository。
type CustomerRepository struct {
	db *gorm.DB
}

// NewCustomerRepository .
func NewCustomerRepository(db *gorm.DB) *CustomerRepository { return &CustomerRepository{db: db} }

var _ customer.Repository = (*CustomerRepository)(nil)

// Insert .
func (r *CustomerRepository) Insert(ctx context.Context, c domain.Customer) (int64, error) {
	row := customerToRow(c)
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
		row.UpdatedAt = row.CreatedAt
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

// FindByIDForPartner BOLA：partner_id 不匹配 → nil。
func (r *CustomerRepository) FindByIDForPartner(ctx context.Context, partnerID, customerID int64) (*domain.Customer, error) {
	var row customerRow
	if err := r.db.WithContext(ctx).
		Where("id = ? AND partner_id = ?", customerID, partnerID).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rowToCustomer(&row), nil
}

// FindByID staff 视角，无 partner 限制。
func (r *CustomerRepository) FindByID(ctx context.Context, customerID int64) (*domain.Customer, error) {
	var row customerRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", customerID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rowToCustomer(&row), nil
}

// FindByFyUserID .
func (r *CustomerRepository) FindByFyUserID(ctx context.Context, fyUserID int64) (*domain.Customer, error) {
	var row customerRow
	if err := r.db.WithContext(ctx).First(&row, "fy_user_id = ?", fyUserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rowToCustomer(&row), nil
}

// Update updater 模式。
func (r *CustomerRepository) Update(ctx context.Context, id int64, updater func(domain.Customer) domain.Customer) (*domain.Customer, error) {
	var result domain.Customer
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row customerRow
		if err := selectForUpdate(tx).First(&row, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return repository.ErrNotFound
			}
			return err
		}
		current := rowToCustomer(&row)
		next := updater(*current)
		nextRow := customerToRow(next)
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

// OrphanByPartner partner 终止时把所有 active 客户置 orphaned；返更新条数。
//
// SQL：UPDATE customer SET status='orphaned', updated_at=? WHERE partner_id=? AND status='active'。
// graceUntil 暂未写入 customer 表（无 grace_until 列）；保留参数兼容 service 接口 — cron 用
// updated_at + 30d 判定到期。
func (r *CustomerRepository) OrphanByPartner(ctx context.Context, partnerID int64, _ time.Time, at time.Time) (int, error) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	res := r.db.WithContext(ctx).Model(&customerRow{}).
		Where("partner_id = ? AND status = ?", partnerID, string(customer.StatusActive)).
		Updates(map[string]any{
			"status":     string(customer.StatusOrphaned),
			"updated_at": at,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}

// ListByPartner .
func (r *CustomerRepository) ListByPartner(ctx context.Context, partnerID int64, f customer.ListFilter) ([]domain.Customer, error) {
	q := r.db.WithContext(ctx).Model(&customerRow{}).Where("partner_id = ?", partnerID)
	if f.Status != "" {
		q = q.Where("status = ?", string(f.Status))
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	q = q.Order("id DESC").Limit(f.Limit).Offset(f.Offset)
	var rows []customerRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Customer, 0, len(rows))
	for i := range rows {
		out = append(out, *rowToCustomer(&rows[i]))
	}
	return out, nil
}

// InsertChangeLog .
func (r *CustomerRepository) InsertChangeLog(ctx context.Context, l domain.CustomerPartnerChangeLog) (int64, error) {
	row := changeLogToRow(l)
	if row.OccurredAt.IsZero() {
		row.OccurredAt = time.Now().UTC()
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = row.OccurredAt
		row.UpdatedAt = row.OccurredAt
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

// UpdateChangeLog .
func (r *CustomerRepository) UpdateChangeLog(ctx context.Context, id int64,
	updater func(domain.CustomerPartnerChangeLog) domain.CustomerPartnerChangeLog) (*domain.CustomerPartnerChangeLog, error) {
	var result domain.CustomerPartnerChangeLog
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row customerPartnerChangeLogRow
		if err := selectForUpdate(tx).First(&row, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return repository.ErrNotFound
			}
			return err
		}
		current := rowToChangeLog(&row)
		next := updater(*current)
		nextRow := changeLogToRow(next)
		nextRow.ID = id
		nextRow.CreatedAt = row.CreatedAt
		nextRow.UpdatedAt = time.Now().UTC()
		if err := tx.Save(&nextRow).Error; err != nil {
			return err
		}
		result = next
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func rowToCustomer(r *customerRow) *domain.Customer {
	return &domain.Customer{
		ID:                 r.ID,
		FyUserID:           r.FyUserID,
		PartnerID:          r.PartnerID,
		JoinedVia:          r.JoinedVia,
		InvitationCodeUsed: r.InvitationCodeUsed,
		Status:             domain.CustomerStatus(r.Status),
		GroupNameInFyAPI:   r.GroupNameInFyAPI,
		QuotaLimit:         r.QuotaLimit,
		TransferredFrom:    r.TransferredFrom,
		TransferredAt:      r.TransferredAt,
		CreatedAt:          r.CreatedAt,
		UpdatedAt:          r.UpdatedAt,
	}
}

func customerToRow(c domain.Customer) customerRow {
	return customerRow{
		ID:                 c.ID,
		FyUserID:           c.FyUserID,
		PartnerID:          c.PartnerID,
		JoinedVia:          c.JoinedVia,
		InvitationCodeUsed: c.InvitationCodeUsed,
		Status:             string(c.Status),
		GroupNameInFyAPI:   c.GroupNameInFyAPI,
		QuotaLimit:         c.QuotaLimit,
		TransferredFrom:    c.TransferredFrom,
		TransferredAt:      c.TransferredAt,
		CreatedAt:          c.CreatedAt,
		UpdatedAt:          c.UpdatedAt,
	}
}

func rowToChangeLog(r *customerPartnerChangeLogRow) *domain.CustomerPartnerChangeLog {
	return &domain.CustomerPartnerChangeLog{
		ID:            r.ID,
		CustomerID:    r.CustomerID,
		FromPartnerID: r.FromPartnerID,
		ToPartnerID:   r.ToPartnerID,
		InitiatorType: r.InitiatorType,
		InitiatorID:   r.InitiatorID,
		Status:        r.Status,
		Reason:        r.Reason,
		OccurredAt:    r.OccurredAt,
		OldGroup:      r.OldGroup,
		NewGroup:      r.NewGroup,
	}
}

func changeLogToRow(l domain.CustomerPartnerChangeLog) customerPartnerChangeLogRow {
	return customerPartnerChangeLogRow{
		ID:            l.ID,
		CustomerID:    l.CustomerID,
		FromPartnerID: l.FromPartnerID,
		ToPartnerID:   l.ToPartnerID,
		InitiatorType: l.InitiatorType,
		InitiatorID:   l.InitiatorID,
		Status:        l.Status,
		Reason:        l.Reason,
		OccurredAt:    l.OccurredAt,
		OldGroup:      l.OldGroup,
		NewGroup:      l.NewGroup,
	}
}
