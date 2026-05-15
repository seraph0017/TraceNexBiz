// Package main publishes local_outbox pending rows to Aliyun MNS.
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
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/outbox"
	pkgleader "github.com/seraph0017/tracenexbiz/apps/partner-api/pkg/leader"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339Nano})

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config.Load failed")
	}
	if cfg.MNS.Backend != "aliyun_mns" {
		log.Info().Str("backend", cfg.MNS.Backend).Msg("outbox-poller: skipped (non-MNS backend)")
		return
	}
	bizDB, _, err := db.Open(cfg)
	if err != nil || bizDB == nil {
		log.Fatal().Err(err).Msg("outbox-poller: bizDB required")
	}
	pub, err := outbox.NewMNSPublisher(outbox.MNSConfig{
		Endpoint:        cfg.MNS.Endpoint,
		AccessKeyID:     cfg.MNS.AccessKeyID,
		AccessKeySecret: cfg.MNS.AccessKeySecret,
		Timeout:         5 * time.Second,
		MaxRetries:      3,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("outbox-poller: NewMNSPublisher failed")
	}
	repo := outbox.NewGormLocalRepo(bizDB)

	rdb, _ := infraredis.Open(cfg)
	owner := "outbox-poller-" + uuid.NewString()
	var lock *pkgleader.RedisLock
	if rdb != nil {
		lock = pkgleader.NewRedisLock(rdb, "cron:outbox-poller", owner, 30*time.Second, 10*time.Second)
	} else if cfg.Env != config.EnvDev {
		log.Fatal().Msg("outbox-poller: Redis required for leader election in staging/prod")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	go func() { <-stop; cancel() }()

	body := func(ctx context.Context) error {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		runOnce(ctx, repo, pub, cfg)
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				runOnce(ctx, repo, pub, cfg)
			}
		}
	}
	if lock != nil {
		err = lock.Run(ctx, body)
	} else {
		err = body(ctx)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Error().Err(err).Msg("outbox-poller exited with error")
		os.Exit(1)
	}
}

func runOnce(ctx context.Context, repo outbox.LocalRepo, pub outbox.Publisher, cfg *config.Config) {
	sent, err := outbox.PublishPendingOnce(ctx, repo, pub, cfg.MNS.QueueOut, cfg.MNS.DataRegion, 100)
	if err != nil {
		log.Error().Err(err).Msg("outbox-poller: publish batch failed")
		return
	}
	if sent > 0 {
		log.Info().Int("sent", sent).Str("queue", cfg.MNS.QueueOut).Str("data_region", cfg.MNS.DataRegion).Msg("outbox-poller: sent")
	}
}
