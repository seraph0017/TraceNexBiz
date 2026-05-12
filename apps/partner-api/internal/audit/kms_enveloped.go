// Package audit — Fix-C item 10: KMS-encrypted PayloadJSON.
//
// 把 audit_log_unsealed.payload_json 在写入路径上经 kms.Service.Encrypt 加密，
// 读取路径（FetchUnsealedBatch / IterateSealed）反向 Decrypt。
//
// 存储格式：基于现有 TEXT 列，存 base64(ciphertext) + "|" + key_id 形式：
//
//	"<base64-ciphertext>|<key_id>"
//
// 这样 schema 不变（payload_json 仍是 TEXT），EnvelopedStore 透明加解密。
//
// scope = "audit:payload" — DEK 与 idempotency / KYC 分离（per ADR-009）。
//
// 失败语义：
//   - Encrypt error → 写入路径返回 error，调用者退降到原 plaintext 路径（fail-open；
//     Phase-1 不阻塞链路；CRIT-C4 closes plaintext TODO）；EnvelopedStore.AllowFallback=true 时
//     plaintext 直接落库（仍 log warning）。
//   - Decrypt error → 读取返回原 ciphertext payload（避免在 verify 路径 crash）。
package audit

import (
	"context"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/kms"
)

const (
	auditPayloadScope = "audit:payload"
	// encryptedMarker 区分加密 payload 与历史 plaintext。
	encryptedMarker = "kms!"
	separator       = "|"
)

// EnvelopedStore 包装 GormStore，对 PayloadJSON 做透明加解密。
type EnvelopedStore struct {
	*GormStore
	kms           kms.Service
	AllowFallback bool // true: KMS 失败 plaintext 落库（dev 友好）；prod 应 false
}

// NewEnvelopedStore 构造；kms == nil 时 EncryptPayload no-op（向后兼容）.
func NewEnvelopedStore(inner *GormStore, k kms.Service) *EnvelopedStore {
	return &EnvelopedStore{GormStore: inner, kms: k}
}

// EnqueueUnsealed override：加密 PayloadJSON → 复用 inner.
func (s *EnvelopedStore) EnqueueUnsealed(ctx context.Context, r UnsealedRow) error {
	if s.kms != nil && r.PayloadJSON != nil && *r.PayloadJSON != "" {
		enc, kid, err := s.kms.Encrypt(ctx, auditPayloadScope, []byte(*r.PayloadJSON))
		if err != nil {
			if !s.AllowFallback {
				return err
			}
			log.Warn().Err(err).Msg("audit: KMS encrypt failed, plaintext fallback")
		} else {
			wrapped := encryptedMarker + kms.Base64Encode(enc) + separator + kid
			r.PayloadJSON = &wrapped
		}
	}
	return s.GormStore.EnqueueUnsealed(ctx, r)
}

// FetchUnsealedBatch override：解密 PayloadJSON.
func (s *EnvelopedStore) FetchUnsealedBatch(ctx context.Context, limit int) ([]UnsealedRow, error) {
	rows, err := s.GormStore.FetchUnsealedBatch(ctx, limit)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		decryptInplace(ctx, s.kms, &rows[i])
	}
	return rows, nil
}

// IterateSealed override：透明解密.
func (s *EnvelopedStore) IterateSealed(ctx context.Context, since int64, fn func(SealedRow) error) error {
	return s.GormStore.IterateSealed(ctx, since, func(r SealedRow) error {
		decryptInplace(ctx, s.kms, &r.UnsealedRow)
		return fn(r)
	})
}

func decryptInplace(ctx context.Context, k kms.Service, r *UnsealedRow) {
	if k == nil || r.PayloadJSON == nil {
		return
	}
	val := *r.PayloadJSON
	if !strings.HasPrefix(val, encryptedMarker) {
		return
	}
	rest := val[len(encryptedMarker):]
	idx := strings.LastIndex(rest, separator)
	if idx <= 0 {
		return
	}
	b64, kid := rest[:idx], rest[idx+1:]
	ct, err := kms.Base64Decode(b64)
	if err != nil {
		log.Warn().Err(err).Msg("audit: payload_json base64 decode failed")
		return
	}
	pt, err := k.Decrypt(ctx, kid, ct)
	if err != nil {
		log.Warn().Err(err).Msg("audit: payload_json decrypt failed")
		return
	}
	s := string(pt)
	r.PayloadJSON = &s
}
