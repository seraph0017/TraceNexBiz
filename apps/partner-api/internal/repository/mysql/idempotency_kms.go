// idempotency_kms.go — Fix-C item 10：KMS Encrypt wrapper for IdempotencyRepository.
//
// Phase-1 写明文 ResponseBody；Fix-C 把 service.Persist → IdempotencyRepository.InsertWithinTx
// 的中间路径切到 EncryptingIdempotency：
//
//   - 写：ResponseBody plain → kms.Encrypt(scope="idem:response") → ResponseCipher + ResponseKeyID
//          清空 ResponseBody（明文不入库）
//   - 读：Find 返回 *domain.IdempotencyRecord 后，service 层调 DecryptResponseBody helper
//         恢复 ResponseBody
//
// scope = "idem:response" — 与 audit / KYC DEK 分离。
//
// 失败语义：AllowFallback=true 时 KMS 错误下写 plaintext（仍 log warn）；false 时 error。
package mysql

import (
	"context"
	"errors"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/kms"
)

// IdempotencyScope per ADR-009.
const IdempotencyScope = "idem:response"

// EncryptingIdempotency 包装 IdempotencyRepository，在 InsertWithinTx 入口加密 ResponseBody。
type EncryptingIdempotency struct {
	*IdempotencyRepository
	KMS           kms.Service
	AllowFallback bool
}

// NewEncryptingIdempotency 构造。
func NewEncryptingIdempotency(inner *IdempotencyRepository, k kms.Service) *EncryptingIdempotency {
	return &EncryptingIdempotency{IdempotencyRepository: inner, KMS: k}
}

// InsertWithinTx override.
func (e *EncryptingIdempotency) InsertWithinTx(tx *gorm.DB, rec *domain.IdempotencyRecord) error {
	if rec == nil {
		return errors.New("idempotency: nil record")
	}
	if e.KMS != nil && rec.ResponseBody != "" {
		ct, kid, err := e.KMS.Encrypt(context.Background(), IdempotencyScope, []byte(rec.ResponseBody))
		if err != nil {
			if !e.AllowFallback {
				return err
			}
			log.Warn().Err(err).Msg("idempotency: KMS encrypt failed, plaintext fallback")
		} else {
			rec.ResponseCipher = ct
			rec.ResponseKeyID = kid
			rec.ResponseBody = ""
		}
	}
	return e.IdempotencyRepository.InsertWithinTx(tx, rec)
}

// Find override：透明解密 ResponseCipher → ResponseBody.
func (e *EncryptingIdempotency) Find(ctx context.Context, actorType string, actorID int64, key, endpoint string) (*domain.IdempotencyRecord, error) {
	rec, err := e.IdempotencyRepository.Find(ctx, actorType, actorID, key, endpoint)
	if err != nil {
		return nil, err
	}
	if e.KMS != nil && len(rec.ResponseCipher) > 0 && rec.ResponseKeyID != "" {
		pt, derr := e.KMS.Decrypt(ctx, rec.ResponseKeyID, rec.ResponseCipher)
		if derr != nil {
			log.Warn().Err(derr).Msg("idempotency: KMS decrypt failed; returning cipher as-is")
			return rec, nil
		}
		rec.ResponseBody = string(pt)
	}
	return rec, nil
}
