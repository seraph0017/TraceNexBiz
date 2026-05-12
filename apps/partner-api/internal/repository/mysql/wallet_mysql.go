// wallet_mysql.go — GORM 实现 service/wallet.Repository（read-only 视图）。
//
// 注：W1a Repository 接口只覆盖只读 + sum/list；写路径（Allocate/Refund/Topup）走
// AllocateExecutor 端口（W1b saga 实现）。CHECK (balance >= 0) 由 012 迁移加 + 应用层
// 原子 SQL 双保险（HIGH-B7）。
package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	walletsvc "github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/wallet"
)

// partnerWalletRow 对应 partner_wallet 表（002_wallet.up.sql）。
type partnerWalletRow struct {
	ID           int64     `gorm:"primaryKey;column:id"`
	PartnerID    int64     `gorm:"column:partner_id;uniqueIndex"`
	Balance      int64     `gorm:"column:balance"`
	PaidOutTotal int64     `gorm:"column:paid_out_total"`
	Version      int64     `gorm:"column:version"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (partnerWalletRow) TableName() string { return "partner_wallet" }

// walletHoldRow 对应 wallet_hold 表。
type walletHoldRow struct {
	ID         int64      `gorm:"primaryKey;column:id"`
	WalletID   int64      `gorm:"column:wallet_id;index"`
	PartnerID  int64      `gorm:"column:partner_id;index"`
	Amount     int64      `gorm:"column:amount"`
	SagaID     string     `gorm:"column:saga_id;uniqueIndex;size:64"`
	Status     string     `gorm:"column:status;size:16"`
	HeldAt     time.Time  `gorm:"column:held_at"`
	ResolvedAt *time.Time `gorm:"column:resolved_at"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at"`
}

func (walletHoldRow) TableName() string { return "wallet_hold" }

// partnerWalletLogRow 对应 partner_wallet_log 表。
type partnerWalletLogRow struct {
	ID             int64     `gorm:"primaryKey;column:id"`
	PartnerID      int64     `gorm:"column:partner_id;index"`
	Type           string    `gorm:"column:type;size:32"`
	Amount         int64     `gorm:"column:amount"`
	BalanceAfter   int64     `gorm:"column:balance_after"`
	RefID          string    `gorm:"column:ref_id;size:128"`
	IdempotencyKey string    `gorm:"column:idempotency_key;size:64;index:uk_wallet_log_idem,unique,composite:idempotency_key_type"`
	Status         string    `gorm:"column:status;size:32"`
	Note           string    `gorm:"column:note;type:text"`
	OperatorType   string    `gorm:"column:operator_type;size:32"`
	OperatorID     int64     `gorm:"column:operator_id"`
	TraceID        string    `gorm:"column:trace_id;size:64"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (partnerWalletLogRow) TableName() string { return "partner_wallet_log" }

// WalletRepository GORM 实现 service/wallet.Repository。
type WalletRepository struct {
	db *gorm.DB
}

// NewWalletRepository .
func NewWalletRepository(db *gorm.DB) *WalletRepository { return &WalletRepository{db: db} }

var _ walletsvc.Repository = (*WalletRepository)(nil)

// FindWallet partner 自己的钱包；不存在 → nil。
func (r *WalletRepository) FindWallet(ctx context.Context, partnerID int64) (*domain.PartnerWallet, error) {
	var row partnerWalletRow
	if err := r.db.WithContext(ctx).First(&row, "partner_id = ?", partnerID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rowToWallet(&row), nil
}

// SumHeldByPartner status='held' 的 hold 求和 + count。
func (r *WalletRepository) SumHeldByPartner(ctx context.Context, partnerID int64) (int64, int, error) {
	type aggRow struct {
		Sum   *int64
		Count int64
	}
	var agg aggRow
	err := r.db.WithContext(ctx).Model(&walletHoldRow{}).
		Where("partner_id = ? AND status = ?", partnerID, "held").
		Select("COALESCE(SUM(amount), 0) AS sum, COUNT(*) AS count").
		Scan(&agg).Error
	if err != nil {
		return 0, 0, err
	}
	sum := int64(0)
	if agg.Sum != nil {
		sum = *agg.Sum
	}
	return sum, int(agg.Count), nil
}

// ListLogs partner 流水查询。
func (r *WalletRepository) ListLogs(ctx context.Context, partnerID int64, f walletsvc.LogFilter) ([]domain.PartnerWalletLog, error) {
	q := r.db.WithContext(ctx).Model(&partnerWalletLogRow{}).Where("partner_id = ?", partnerID)
	if f.Type != "" {
		q = q.Where("type = ?", string(f.Type))
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	q = q.Order("id DESC").Limit(f.Limit).Offset(f.Offset)
	var rows []partnerWalletLogRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.PartnerWalletLog, 0, len(rows))
	for i := range rows {
		out = append(out, *rowToWalletLog(&rows[i]))
	}
	return out, nil
}

// ListHolds 列出 partner 当前 status='held' 的 hold。
func (r *WalletRepository) ListHolds(ctx context.Context, partnerID int64) ([]domain.WalletHold, error) {
	var rows []walletHoldRow
	if err := r.db.WithContext(ctx).
		Where("partner_id = ? AND status = ?", partnerID, "held").
		Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.WalletHold, 0, len(rows))
	for i := range rows {
		out = append(out, *rowToWalletHold(&rows[i]))
	}
	return out, nil
}

// AdjustBalance 原子 balance 增减（HIGH-B7 belt-and-braces）。
//
// UPDATE partner_wallet SET balance = balance + ?, version = version + 1
//   WHERE partner_id = ? AND balance + ? >= 0
//
// 返回更新条数。若返 0 → 余额不足。这是给 W1b saga 用的辅助方法（接口外暴露）；
// CHECK (balance >= 0) DB 约束是第二道防线。
func (r *WalletRepository) AdjustBalance(ctx context.Context, partnerID, delta int64) (rowsAffected int64, err error) {
	res := r.db.WithContext(ctx).Model(&partnerWalletRow{}).
		Where("partner_id = ? AND balance + ? >= 0", partnerID, delta).
		Updates(map[string]any{
			"balance":    gorm.Expr("balance + ?", delta),
			"version":    gorm.Expr("version + 1"),
			"updated_at": time.Now().UTC(),
		})
	return res.RowsAffected, res.Error
}

// EnsureWallet partner 注册后初始化 wallet 行（balance=0）。幂等。
func (r *WalletRepository) EnsureWallet(ctx context.Context, partnerID int64) error {
	now := time.Now().UTC()
	row := partnerWalletRow{
		PartnerID: partnerID, Balance: 0, PaidOutTotal: 0, Version: 0,
		CreatedAt: now, UpdatedAt: now,
	}
	// FirstOrCreate 跨方言（不依赖 ON DUPLICATE / ON CONFLICT）。
	return r.db.WithContext(ctx).
		Where("partner_id = ?", partnerID).
		Attrs(row).
		FirstOrCreate(&row).Error
}

func rowToWallet(r *partnerWalletRow) *domain.PartnerWallet {
	return &domain.PartnerWallet{
		ID:           r.ID,
		PartnerID:    r.PartnerID,
		Balance:      r.Balance,
		PaidOutTotal: r.PaidOutTotal,
		Version:      r.Version,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}

func rowToWalletLog(r *partnerWalletLogRow) *domain.PartnerWalletLog {
	return &domain.PartnerWalletLog{
		ID:             r.ID,
		PartnerID:      r.PartnerID,
		Type:           domain.WalletLogType(r.Type),
		Amount:         r.Amount,
		BalanceAfter:   r.BalanceAfter,
		RefID:          r.RefID,
		IdempotencyKey: r.IdempotencyKey,
		Status:         r.Status,
		Note:           r.Note,
		OperatorType:   r.OperatorType,
		OperatorID:     r.OperatorID,
		TraceID:        r.TraceID,
		CreatedAt:      r.CreatedAt,
	}
}

func rowToWalletHold(r *walletHoldRow) *domain.WalletHold {
	return &domain.WalletHold{
		ID:         r.ID,
		WalletID:   r.WalletID,
		PartnerID:  r.PartnerID,
		Amount:     r.Amount,
		SagaID:     r.SagaID,
		Status:     r.Status,
		HeldAt:     r.HeldAt,
		ResolvedAt: r.ResolvedAt,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}
}
