// 测试辅助：构建 in-memory SQLite *gorm.DB + AutoMigrate 业务表。
//
// 用 pure-Go SQLite（glebarez/sqlite，modernc.org/sqlite 后端，无 CGO），跨平台跑得起。
// 业务 DDL 的 MySQL 方言（VARBINARY / TIMESTAMP(3) / ON UPDATE）SQLite 不支持；
// 故测试 DB 用 GORM AutoMigrate 走 SQLite 方言重建一份 row 结构（schema 等价）。
//
// 生产链路仍走 migrations/ 下的 MySQL DDL；这里只是为了 repository 单测能跑。
package mysql

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// allRowTypes 所有 row struct；NewTestDB 用它批量 AutoMigrate。
//
// 新增 GORM repo 时把新 row 加进来。
func allRowTypes() []any {
	return []any{
		&partnerRow{},
		&customerRow{},
		&customerPartnerChangeLogRow{},
		&invitationCodeRow{},
		&kycApplicationRow{},
		&partnerWalletRow{},
		&walletHoldRow{},
		&partnerWalletLogRow{},
	}
}

// NewTestDB 起一份 fresh in-memory SQLite + AutoMigrate 全部业务表。
//
// 调用方拿到的是空库；如果 t.Cleanup 触发，gorm 内部的 sql.DB 自然释放。
func NewTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:?_foreign_keys=on"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := gdb.AutoMigrate(allRowTypes()...); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return gdb
}
