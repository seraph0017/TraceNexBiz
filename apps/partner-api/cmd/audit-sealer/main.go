// Package main is the audit-sealer cron entry（backend §10.1 / Fix-B' part 4 CRIT-B6）.
//
// 单 leader（dev: AlwaysLeader；prod: Redis SETNX via pkg/leader.RedisLock）；
// 200ms tick 拉 audit_log_unsealed → 写 audit_log + 哈希链 → 删 unsealed.
//
// Fix-B' part 4: bizDB 就绪时用 GormStore；缺 DSN → fail-fast 退出 1.
// Fix-C item 8: Redis 就绪时切 RedisLeader（30s TTL / 10s refresh）；缺 Redis 仅 dev 允许降级.
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

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/audit"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/config"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/db"
	infraredis "github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/redis"
	pkgleader "github.com/seraph0017/tracenexbiz/apps/partner-api/pkg/leader"
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
	leader := buildLeader(cfg)
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

// buildLeader 返回 audit.LeaderLock。
//
// Fix-C item 8 语义：
//   - Redis 可用                → RedisLeader (key=cron:audit-sealer, TTL=30s, refresh=10s)
//   - Redis 不可用 + env=dev    → AlwaysLeader（单机 dev 起得来）
//   - Redis 不可用 + 非 dev     → fail-fast（避免多 pod 双跑）
func buildLeader(cfg *config.Config) audit.LeaderLock {
	rdb, err := infraredis.Open(cfg)
	if err != nil {
		if cfg.Env != config.EnvDev {
			log.Fatal().Err(err).Msg("audit-sealer: Redis required for leader election in staging/prod")
		}
		log.Warn().Err(err).Msg("audit-sealer: Redis unavailable, falling back to AlwaysLeader (dev only)")
		return audit.AlwaysLeader{}
	}
	owner := "audit-sealer-" + uuid.NewString()
	lock := pkgleader.NewRedisLock(rdb, "cron:audit-sealer", owner, 30*time.Second, 10*time.Second)
	log.Info().Str("owner", owner).Msg("audit-sealer: Redis leader election active")
	return audit.NewRedisLeader(lock)
}
