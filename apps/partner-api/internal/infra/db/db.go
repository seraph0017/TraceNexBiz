// Package db 封装 GORM 多 DB 连接管理。
//
// 三类连接（per backend §14.2 + ADR-005）：
//   - bizDB：tnbiz_app → partner_db（RW 业务主库）
//   - fyDB ：tnbiz_app → fy_api_db（RO，由 GRANT 强制只读）
//   - logDB：tnbiz_outbox_consumer → fy_api_db（仅 outbox poller 用，独立 cmd 进程）
//
// W0：仅返 bizDB + fyDB；logDB 在 cmd/outbox-poller 内独立 Open。
package db

import (
	"errors"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/config"
)

// Open 打开 bizDB / fyDB；缺 DSN 时返回 nil + error，由 caller 决定降级。
func Open(cfg *config.Config) (bizDB *gorm.DB, fyDB *gorm.DB, err error) {
	if cfg.DB.BizDSN == "" {
		return nil, nil, errors.New("DB_BIZ_DSN is empty")
	}

	bizDB, err = openOne(cfg.DB.BizDSN, cfg)
	if err != nil {
		return nil, nil, err
	}

	if cfg.DB.FyReadOnlyDSN == "" {
		// W0 dev：fyDB 可缺；W1c 必须存在
		return bizDB, nil, nil
	}
	fyDB, err = openOne(cfg.DB.FyReadOnlyDSN, cfg)
	if err != nil {
		return bizDB, nil, err
	}
	return bizDB, fyDB, nil
}

func openOne(dsn string, cfg *config.Config) (*gorm.DB, error) {
	gdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DB.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.DB.ConnMaxLifetime)
	return gdb, nil
}
