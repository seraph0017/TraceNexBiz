// Package main is the kyc-purge cron entry（Fix-C CRIT-C3）.
//
// PIPL §47：PII 必须在「同意撤回 / 关系终止」后 5 年内删除。
// 本 cron 每天扫一次 kyc_application：
//
//   - status='archived' 且 cold_archive_expires_at < now-5y → 进入 purge
//   - 把 PII 字段（cipher 列、blind_index、URL）置空，pii_purged_at = now
//   - 调 kms.ScheduleKeyDeletion(legal_person_*_key_id, 30d) 与 bank_account_key_id 等
//     注：失败可重试（idempotent — 同 keyID 多次预约删除是 KMS no-op）
//   - 写 audit_log_unsealed event_type='kyc.purge'
//
// 单 leader（Redis SETNX）；缺 Redis 仅 dev 允许降级.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/audit"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/config"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/db"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/kms"
	infraredis "github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/redis"
	pkgleader "github.com/seraph0017/tracenexbiz/apps/partner-api/pkg/leader"
)

// PIPL5Years 5y retention window per PIPL §47.
const PIPL5Years = 5 * 365 * 24 * time.Hour

// KMSDeletionWindow 30d default Aliyun KMS PendingWindowInDays.
const KMSDeletionWindow = 30

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
			log.Fatal().Err(dbErr).Msg("kyc-purge: bizDB required in staging/prod")
		}
		log.Warn().Err(dbErr).Msg("kyc-purge: bizDB unavailable; cron will no-op (dev)")
	}

	kmsSvc := buildKMS(cfg)
	rdb, _ := infraredis.Open(cfg)

	owner := "kyc-purge-" + uuid.NewString()
	var lock *pkgleader.RedisLock
	if rdb != nil {
		lock = pkgleader.NewRedisLock(rdb, "cron:kyc-purge", owner, 30*time.Second, 10*time.Second)
	} else if cfg.Env != config.EnvDev {
		log.Fatal().Msg("kyc-purge: Redis required for leader election in staging/prod")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	go func() { <-stop; cancel() }()

	body := func(ctx context.Context) error {
		log.Info().Msg("kyc-purge: leader; tick=24h")
		// run once on start
		runOnce(ctx, bizDB, kmsSvc)
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				runOnce(ctx, bizDB, kmsSvc)
			}
		}
	}

	if lock != nil {
		err = lock.Run(ctx, body)
	} else {
		err = body(ctx)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Error().Err(err).Msg("kyc-purge exited with error")
		os.Exit(1)
	}
	log.Info().Msg("kyc-purge: stopped")
}

func buildKMS(cfg *config.Config) kms.Service {
	if cfg.Env == config.EnvDev {
		if cfg.KMS.Endpoint == "" {
			return kms.NewStub()
		}
	}
	return kms.NewAliyunKMS(cfg.KMS.Endpoint, cfg.KMS.KeyID, cfg.KMS.Region, cfg.KMS.AccessKey, cfg.KMS.AccessSecret)
}

// kycPurgeRow minimal GORM row mapper（避免 import cycle 到 mysql repo 私有 type）.
type kycPurgeRow struct {
	ID                        int64      `gorm:"primaryKey;column:id"`
	Status                    string     `gorm:"column:status"`
	BusinessLicenseOCRCipher  []byte     `gorm:"column:business_license_ocr_cipher"`
	BusinessLicenseOCRKeyID   string     `gorm:"column:business_license_ocr_key_id"`
	LegalPersonNameCipher     []byte     `gorm:"column:legal_person_name_cipher"`
	LegalPersonNameKeyID      string     `gorm:"column:legal_person_name_key_id"`
	LegalPersonNameBlindIndex string     `gorm:"column:legal_person_name_blind_index"`
	LegalPersonIDCipher       []byte     `gorm:"column:legal_person_id_cipher"`
	LegalPersonIDKeyID        string     `gorm:"column:legal_person_id_key_id"`
	LegalPersonIDBlindIndex   string     `gorm:"column:legal_person_id_blind_index"`
	BankAccountCipher         []byte     `gorm:"column:bank_account_cipher"`
	BankAccountKeyID          string     `gorm:"column:bank_account_key_id"`
	BankAccountBlindIndex     string     `gorm:"column:bank_account_blind_index"`
	AlipayOpenIDCipher        []byte     `gorm:"column:alipay_open_id_cipher"`
	AlipayOpenIDKeyID         string     `gorm:"column:alipay_open_id_key_id"`
	AlipayRealNameCipher      []byte     `gorm:"column:alipay_real_name_cipher"`
	AlipayRealNameKeyID       string     `gorm:"column:alipay_real_name_key_id"`
	PIIPurgedAt               *time.Time `gorm:"column:pii_purged_at"`
	ColdArchiveExpiresAt      *time.Time `gorm:"column:cold_archive_expires_at"`
}

func (kycPurgeRow) TableName() string { return "kyc_application" }

// runOnce idempotent purge sweep.
func runOnce(ctx context.Context, gdb *gorm.DB, kmsSvc kms.Service) {
	if gdb == nil {
		log.Debug().Msg("kyc-purge: no DB, skip tick")
		return
	}
	cutoff := time.Now().Add(-PIPL5Years)
	var rows []kycPurgeRow
	q := gdb.WithContext(ctx).
		Where("status = ?", "archived").
		Where("(pii_purged_at IS NULL)").
		Where("(cold_archive_expires_at IS NOT NULL AND cold_archive_expires_at < ?)", cutoff).
		Limit(500)
	if err := q.Find(&rows).Error; err != nil {
		log.Error().Err(err).Msg("kyc-purge: select failed")
		return
	}
	if len(rows) == 0 {
		log.Debug().Msg("kyc-purge: nothing to do this tick")
		return
	}
	log.Info().Int("count", len(rows)).Msg("kyc-purge: rows past 5y retention")

	auditStore := audit.NewGormStore(gdb)
	for _, r := range rows {
		if err := purgeOne(ctx, gdb, kmsSvc, auditStore, r); err != nil {
			log.Error().Err(err).Int64("kyc_id", r.ID).Msg("kyc-purge: failed; will retry next tick")
			continue
		}
	}
}

func purgeOne(ctx context.Context, gdb *gorm.DB, kmsSvc kms.Service, auditStore *audit.GormStore, r kycPurgeRow) error {
	keyIDs := []string{
		r.BusinessLicenseOCRKeyID, r.LegalPersonNameKeyID, r.LegalPersonIDKeyID,
		r.BankAccountKeyID, r.AlipayOpenIDKeyID, r.AlipayRealNameKeyID,
	}
	for _, kid := range keyIDs {
		if kid == "" {
			continue
		}
		if err := kmsSvc.ScheduleKeyDeletion(ctx, kid, KMSDeletionWindow); err != nil {
			// transient error → bail; idempotent retry next tick.
			return fmt.Errorf("kms ScheduleKeyDeletion(%s): %w", kid, err)
		}
	}
	now := time.Now().UTC()
	updates := map[string]any{
		"business_license_ocr_cipher":  nil,
		"legal_person_name_cipher":     nil,
		"legal_person_name_blind_index": "",
		"legal_person_id_cipher":       nil,
		"legal_person_id_blind_index":  "",
		"bank_account_cipher":          nil,
		"bank_account_blind_index":     "",
		"alipay_open_id_cipher":        nil,
		"alipay_real_name_cipher":      nil,
		"pii_purged_at":                &now,
	}
	if err := gdb.WithContext(ctx).Model(&kycPurgeRow{}).
		Where("id = ?", r.ID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("update kyc row %d: %w", r.ID, err)
	}
	// audit row（best-effort；失败仅 log）.
	payload := fmt.Sprintf(`{"kyc_id":%d,"keyids":%d,"window_days":%d}`, r.ID, len(keyIDs), KMSDeletionWindow)
	row := audit.UnsealedRow{
		ActorType:   "system",
		ActorID:     0,
		Action:      "kyc.purge",
		TargetType:  "kyc_application",
		Route:       "/cron/kyc-purge",
		Method:      "CRON",
		Status:      200,
		RequestHash: audit.HashRequestBody([]byte(payload)),
		PayloadJSON: &payload,
		OccurredAt:  now,
	}
	if err := auditStore.EnqueueUnsealed(ctx, row); err != nil {
		log.Warn().Err(err).Msg("kyc-purge: audit append failed (non-blocking)")
	}
	log.Info().Int64("kyc_id", r.ID).Msg("kyc-purge: PII deleted + KMS keys scheduled for 30d deletion")
	return nil
}
