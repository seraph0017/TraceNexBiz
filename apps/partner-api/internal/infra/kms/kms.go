// Package kms 封装 Aliyun KMS 信封加密。
//
// 引用：backend §9 + ADR-009 + PRD §19。
//
// 关键约束：
//   - DEK 缓存 key 必须 (tenant_id, key_id) 分隔，防租户横穿（per ADR-009）
//   - 90d DEK rotation；KEK 1y rotation
//   - prod pprof 关闭或 auth-gated；mlock DEK 内存页（Linux only）
//
// W0：定义 Service 接口 + stub 实现；W1d 接实际 Aliyun SDK。
package kms

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
)

// Service 是 KMS 信封加密的统一入口。
//
// 调用方仅与本接口交互；底层可换成 Aliyun KMS / AWS KMS / 本地 fake（dev）。
type Service interface {
	// Encrypt 用 (scope, version) 对应的 DEK 加密 plaintext；返回密文 + key 句柄。
	//
	// 调用方语义：
	//   - scope = "kyc:legal_id" / "wallet:bank_account" 等业务维度；不要传 tenantID 字符串
	//   - 同一 scope 在 90d rotation 周期内复用同一 DEK
	Encrypt(ctx context.Context, scope string, plaintext []byte) (ciphertext []byte, keyID string, err error)

	// Decrypt 用 keyID 索引的 DEK 解密。
	//
	// 失败原因：keyID 已 rotation 销毁、KMS 拒绝、ciphertext 损坏。
	Decrypt(ctx context.Context, keyID string, ciphertext []byte) ([]byte, error)

	// HMAC 用专用密钥计算 HMAC-SHA256，用于 blind index（per backend §3.9）。
	HMAC(ctx context.Context, scope string, message []byte) ([]byte, error)

	// RotateDEK 触发某 scope 的 DEK 轮换（per backend §9.3）。
	RotateDEK(ctx context.Context, scope string) (newKeyID string, err error)
}

// ErrNotImplemented W0 stub 错误；W1d 实现后删除。
var ErrNotImplemented = errors.New("kms: stub not implemented; W1d agent to wire Aliyun KMS SDK")

// ErrLocalKMSInProd LocalKMS 在 prod 环境被拒绝。
var ErrLocalKMSInProd = errors.New("kms: LocalKMS cannot be used in production (KMS_LOCAL_DEV != true)")

// LocalKMS dev-only XOR-based KMS（不做真加密，仅用于本地测试）。
//
// 严禁在 staging / prod 启用：构造时会 panic if KMS_LOCAL_DEV != "true"。
type LocalKMS struct {
	devKey []byte
}

// NewLocalKMS 返回 dev-only KMS；如果 KMS_LOCAL_DEV != "true" 则 panic。
func NewLocalKMS() Service {
	if os.Getenv("KMS_LOCAL_DEV") != "true" {
		panic(ErrLocalKMSInProd)
	}
	return &LocalKMS{devKey: []byte("tnbiz-local-dev-key-32bytes!!")}
}

func (k *LocalKMS) Encrypt(_ context.Context, scope string, plaintext []byte) ([]byte, string, error) {
	out := make([]byte, len(plaintext))
	for i := range plaintext {
		out[i] = plaintext[i] ^ k.devKey[i%len(k.devKey)]
	}
	keyID := "local:" + scope + ":v0"
	return out, keyID, nil
}

func (k *LocalKMS) Decrypt(_ context.Context, _ string, ciphertext []byte) ([]byte, error) {
	out := make([]byte, len(ciphertext))
	for i := range ciphertext {
		out[i] = ciphertext[i] ^ k.devKey[i%len(k.devKey)]
	}
	return out, nil
}

func (k *LocalKMS) HMAC(_ context.Context, _ string, message []byte) ([]byte, error) {
	// XOR-based HMAC stub
	out := make([]byte, 32)
	for i := range message {
		out[i%32] ^= message[i]
	}
	return out, nil
}

func (k *LocalKMS) RotateDEK(_ context.Context, _ string) (string, error) {
	return "", ErrNotImplemented
}

// AliyunKMS Aliyun KMS SDK 实现（Phase-1: Decrypt only; Encrypt deferred）。
type AliyunKMS struct {
	endpoint     string
	keyID        string
	region       string
	accessKey    string
	accessSecret string
}

// NewAliyunKMS 构造 Aliyun KMS client。
func NewAliyunKMS(endpoint, keyID, region, accessKey, accessSecret string) Service {
	return &AliyunKMS{
		endpoint:     endpoint,
		keyID:        keyID,
		region:       region,
		accessKey:    accessKey,
		accessSecret: accessSecret,
	}
}

func (k *AliyunKMS) Encrypt(_ context.Context, scope string, plaintext []byte) ([]byte, string, error) {
	// TODO(Fix-C): Phase-1 partner-api doesn't write secrets; Encrypt deferred to Fix-C
	return nil, "", errors.New("kms: Encrypt not yet supported in Phase 1")
}

func (k *AliyunKMS) Decrypt(ctx context.Context, keyID string, ciphertext []byte) ([]byte, error) {
	// TODO(Fix-C): Wire actual Aliyun KMS SDK call
	// For now, use AES-GCM with a derived key (placeholder until SDK wired)
	// This is NOT production-ready; real impl must call kms.Decrypt API
	if k.accessKey == "" || k.accessSecret == "" {
		return nil, errors.New("kms: Aliyun credentials not configured")
	}

	// Placeholder: derive a key from accessSecret (NOT secure, just for build)
	key := make([]byte, 32)
	copy(key, []byte(k.accessSecret))

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("kms: cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("kms: gcm: %w", err)
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("kms: ciphertext too short")
	}

	nonce, ct := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("kms: decrypt: %w", err)
	}

	return plaintext, nil
}

func (k *AliyunKMS) HMAC(_ context.Context, _ string, message []byte) ([]byte, error) {
	// TODO(Fix-C): per backend §3.9 接 KMS HMAC API；blind index 必须用专用密钥
	return message, nil
}

func (k *AliyunKMS) RotateDEK(_ context.Context, _ string) (string, error) {
	// TODO(Fix-C): per backend §9.3 KEK 轮换流程
	return "", ErrNotImplemented
}

// Stub 是 W0 占位实现；不做任何加密，返回明文 + 固定 keyID。
//
// 严禁在 staging / prod 启用：env validator 会断言 KMS_ENDPOINT 非空。
type Stub struct{}

// NewStub 返回 W0 stub；W1d 应替换为 NewAliyunKMS(cfg)。
func NewStub() Service { return &Stub{} }

func (s *Stub) Encrypt(_ context.Context, scope string, plaintext []byte) ([]byte, string, error) {
	// TODO(W1d): per backend §9.1 接 Aliyun KMS Encrypt + 缓存 DEK by (scope, version)
	return plaintext, "stub:" + scope + ":v0", nil
}

func (s *Stub) Decrypt(_ context.Context, _ string, ciphertext []byte) ([]byte, error) {
	// TODO(W1d): per backend §9.1
	return ciphertext, nil
}

func (s *Stub) HMAC(_ context.Context, _ string, message []byte) ([]byte, error) {
	// TODO(W1d): per backend §3.9 接 KMS HMAC API；blind index 必须用专用密钥
	return message, nil
}

func (s *Stub) RotateDEK(_ context.Context, _ string) (string, error) {
	// TODO(W1d): per backend §9.3 KEK 轮换流程
	return "", ErrNotImplemented
}

// encryptAESGCM helper for AliyunKMS placeholder (NOT production-ready).
func encryptAESGCM(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// decryptAESGCM helper for AliyunKMS placeholder (NOT production-ready).
func decryptAESGCM(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

// Base64Encode helper for storing ciphertext in DB.
func Base64Encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// Base64Decode helper for reading ciphertext from DB.
func Base64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

