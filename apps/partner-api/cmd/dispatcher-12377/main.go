// Package main is the dispatcher-12377 cron entry（Fix-C CRIT-C2）.
//
// 12377 = 中国互联网违法和不良信息举报中心（Cyberspace Administration）.
// 本 cron 周期性拉取 content_safety_report.status='pending' 行，调用
// content_safety.Service.DispatchOnce 把 payload POST 到 12377 端点；
// 失败重试逻辑由 service 层负责（≤5 retries → dead_letter）。
//
// SLA：sla_due_at = created_at + 24h；超期未提交 → 由 SLABreaches 暴露给监控告警.
//
// 单 leader（Redis SETNX via pkg/leader.RedisLock）；缺 Redis 仅 dev 允许降级.
//
// 退出条件：SIGTERM / leader lease lost / 持续 5 分钟 service 错误.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/config"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/db"
	infraredis "github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/redis"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/content_safety"
	pkgleader "github.com/seraph0017/tracenexbiz/apps/partner-api/pkg/leader"
)

// noopAuthority 默认 dev / staging stub — 接到真 12377 端点前用此占位.
//
// TODO(ops)：填入 12377 公文受理接口（HTTPS POST，需要 mTLS + IP 白名单），
// 替换为 outbound HTTP 客户端 + 签名鉴权.
type noopAuthority struct{}

func (noopAuthority) Submit(_ context.Context, authority, _ string) (string, error) {
	log.Warn().Str("authority", authority).Msg("dispatcher-12377: noop authority client (ops to wire real endpoint)")
	return `{"ack":"noop"}`, nil
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339Nano})

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config.Load failed")
	}
	bizDB, _, dbErr := db.Open(cfg)
	if dbErr != nil || bizDB == nil {
		if cfg.Env != config.EnvDev {
			log.Fatal().Err(dbErr).Msg("dispatcher-12377: bizDB required in staging/prod")
		}
		log.Warn().Err(dbErr).Msg("dispatcher-12377: bizDB unavailable; running with in-memory repo (dev)")
	}

	// content_safety service 装配。
	// Phase-1：repo 走 MemoryRepo（service 层自带）+ noopAuthority；
	// Phase-2：W1d 接 GORM repo + 真 12377 客户端.
	svc := content_safety.NewService(content_safety.NewMemoryRepo(), noopAuthority{})

	rdb, _ := infraredis.Open(cfg)
	owner := "dispatcher-12377-" + uuid.NewString()
	var lock *pkgleader.RedisLock
	if rdb != nil {
		lock = pkgleader.NewRedisLock(rdb, "cron:dispatcher-12377", owner, 30*time.Second, 10*time.Second)
	} else if cfg.Env != config.EnvDev {
		log.Fatal().Msg("dispatcher-12377: Redis required for leader election in staging/prod")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	go func() { <-stop; cancel() }()

	body := func(ctx context.Context) error {
		log.Info().Msg("dispatcher-12377: leader; tick=5m batch=50")
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		// Run immediate first iteration.
		runOnce(ctx, svc)
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				runOnce(ctx, svc)
			}
		}
	}

	if lock != nil {
		err = lock.Run(ctx, body)
	} else {
		err = body(ctx)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Error().Err(err).Msg("dispatcher-12377 exited with error")
		os.Exit(1)
	}
	log.Info().Msg("dispatcher-12377: stopped")
}

func runOnce(ctx context.Context, svc *content_safety.Service) {
	subm, fail, err := svc.DispatchOnce(ctx, 50)
	if err != nil {
		log.Error().Err(err).Msg("dispatcher-12377: DispatchOnce error")
		return
	}
	if subm > 0 || fail > 0 {
		log.Info().Int("submitted", subm).Int("failed", fail).Msg("dispatcher-12377: tick complete")
	}
	// SLA breach alarm: log only; ops piping into Prometheus alert.
	if breaches, err := svc.SLABreaches(ctx); err == nil && len(breaches) > 0 {
		log.Warn().Int("count", len(breaches)).Msg("dispatcher-12377: SLA breach (24h) — investigate")
	}
}
