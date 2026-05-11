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
	"errors"
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
