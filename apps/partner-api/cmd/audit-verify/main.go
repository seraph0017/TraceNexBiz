// Package main is the audit-verify CLI（backend §10.3）.
//
// 用法：
//
//	audit-verify --since=<id>
//
// 退出码：
//
//	0  哈希链完整
//	2  发现 ErrChainBroken（含详细 id / 原因）
//	1  其他错误（DB / 配置）
//
// W0/W1c 落 CLI 骨架；GORM-backed Store 实现待 W1a/W1b 接 partner_db 后接入.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/audit"
)

func main() {
	since := flag.Int64("since", 0, "verify rows with id > since")
	dryRun := flag.Bool("dry-run", true, "use in-memory store stub (W1c default until GORM store wired)")
	flag.Parse()

	ctx := context.Background()
	var store audit.Store
	if *dryRun {
		store = audit.NewMemoryStore()
	} else {
		fmt.Fprintln(os.Stderr, "audit-verify: GORM store wiring deferred to W1a/W1b; use --dry-run for now")
		os.Exit(1)
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
