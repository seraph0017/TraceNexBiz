// Package mysql — biz_setting read-only repo（Fix-C item 1）.
//
// Footer / readiness gate / compliance keys 全部走此 repo 的 GetMany；
// 写入路径仍由 admin biz_setting 服务（dual-control）独占。
package mysql

import (
	"context"

	"gorm.io/gorm"
)

type bizSettingRow struct {
	Key   string `gorm:"column:key;primaryKey;size:128"`
	Value string `gorm:"column:value;type:text"`
}

// TableName .
func (bizSettingRow) TableName() string { return "biz_setting" }

// BizSettingRepository 提供只读 GetMany；ID 不参与映射。
type BizSettingRepository struct {
	db *gorm.DB
}

// NewBizSettingRepository .
func NewBizSettingRepository(db *gorm.DB) *BizSettingRepository {
	return &BizSettingRepository{db: db}
}

// GetMany 批读；返回 map[key]value；缺失 key 不出现在返回值中。
func (r *BizSettingRepository) GetMany(ctx context.Context, keys []string) (map[string]string, error) {
	if r == nil || r.db == nil || len(keys) == 0 {
		return map[string]string{}, nil
	}
	var rows []bizSettingRow
	if err := r.db.WithContext(ctx).Where("`key` IN ?", keys).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.Key] = row.Value
	}
	return out, nil
}
