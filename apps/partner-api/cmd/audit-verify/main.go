// Package main is the audit-verify CLI（backend §10.3 / Fix-B' part 4 CRIT-B6）.
//
// 用法：
//
//	audit-verify --since=<id>                    # 默认 GORM via DB_BIZ_DSN
//	audit-verify --since=<id> --dry-run          # 用内存 store（pre-DB / 单元测试场景）
//
// 退出码：
//
//	0  哈希链完整
//	2  发现 ErrChainBroken（含详细 id / 原因）
//	1  其他错误（DB / 配置）
//
// Fix-B' part 4: 接 GormStore，连 partner_db.audit_log 全表 / since 增量校验。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/audit"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/config"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/db"
)

func main() {
	since := flag.Int64("since", 0, "verify rows with id > since")
	dryRun := flag.Bool("dry-run", false, "use in-memory store stub (no DB needed)")
	flag.Parse()

	ctx := context.Background()
	var store audit.Store
	if *dryRun {
		store = audit.NewMemoryStore()
	} else {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "audit-verify: config.Load: %v\n", err)
			os.Exit(1)
		}
		bizDB, _, err := db.Open(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "audit-verify: db.Open: %v\n", err)
			os.Exit(1)
		}
		if bizDB == nil {
			fmt.Fprintln(os.Stderr, "audit-verify: bizDB unavailable (set DB_BIZ_DSN, or use --dry-run)")
			os.Exit(1)
		}
		store = audit.NewGormStore(bizDB)
	}

	if err := audit.Verify(ctx, store, *since); err != nil {
		if errors.Is(err, audit.ErrChainBroken) {
			fmt.Fprintf(os.Stderr, "CHAIN BROKEN: %v\n", err)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "audit-verify error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK: hash chain verified")
}
