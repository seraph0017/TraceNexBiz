// Package main is the audit-sealer cron entry（backend §10.1 / Fix-B' part 4 CRIT-B6）.
//
// 单 leader（dev: AlwaysLeader；prod: K8s Lease via W1a leader pkg）；
// 200ms tick 拉 audit_log_unsealed → 写 audit_log + 哈希链 → 删 unsealed.
//
// Fix-B' part 4: bizDB 就绪时用 GormStore；缺 DSN → fail-fast 退出 1.
//
// 退出条件：SIGTERM / leader lease lost / store error.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/audit"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/config"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/db"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339Nano})

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config.Load failed")
	}

	var store audit.Store
	bizDB, _, dbErr := db.Open(cfg)
	if dbErr != nil || bizDB == nil {
		if cfg.Env != config.EnvDev {
			log.Fatal().Err(dbErr).Msg("audit-sealer: bizDB required in staging/prod")
		}
		log.Warn().Err(dbErr).Msg("audit-sealer: bizDB unavailable; falling back to MemoryStore (dev only)")
		store = audit.NewMemoryStore()
	} else {
		store = audit.NewGormStore(bizDB)
		log.Info().Msg("audit-sealer: GormStore backed by partner_db")
	}
	leader := audit.AlwaysLeader{} // W1c follow-up: Redis SETNX leader for multi-replica.
	sealer := audit.NewSealer(store, leader)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-stop
		log.Info().Msg("audit-sealer: shutdown initiated")
		cancel()
	}()

	log.Info().Msg("audit-sealer: starting")
	if err := sealer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error().Err(err).Msg("audit-sealer crashed")
		os.Exit(1)
	}
	log.Info().Msg("audit-sealer: stopped")
	_ = http.StatusOK // satisfy import in case of future probe wiring
}
