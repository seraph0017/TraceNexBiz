// Package main is the audit-sealer cron entry（backend §10.1）.
//
// 单 leader（dev: AlwaysLeader；prod: K8s Lease via W1a leader pkg）；
// 200ms tick 拉 audit_log_unsealed → 写 audit_log + 哈希链 → 删 unsealed.
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
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339Nano})

	// W1c dev wiring：MemoryStore + AlwaysLeader.
	// W1a/W1b 接 GORM-backed Store (audit_log_unsealed / audit_log) + Redis SETNX leader.
	store := audit.NewMemoryStore()
	leader := audit.AlwaysLeader{}
	sealer := audit.NewSealer(store, leader)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 优雅退出
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
