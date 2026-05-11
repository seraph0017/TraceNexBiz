// Package pii 提供 PII 字段 GORM 包装（backend §9.2）.
//
// 用法：domain entity 中使用 *Encrypted 字段；service 在 KMS Encrypt 后填 cipher / keyID；
// repository 写入时 GORM Value() 只写 cipher（plain 不持久化）.
//
// W0 scaffold：W1a 接 KMS service + DEK cache。
package pii

import "database/sql/driver"

// Encrypted 信封加密字段封装；plain 仅 service 内瞬态。
type Encrypted struct {
	plain  string
	cipher []byte
	keyID  string
	scope  string
}

// New 给 service 用：构造一个含 plain 的 Encrypted（待 KMS Encrypt）.
func New(scope, plain string) *Encrypted {
	return &Encrypted{plain: plain, scope: scope}
}

// FromCipher repository 反序列化：从 cipher / keyID 构造（plain 留空）.
func FromCipher(cipher []byte, keyID string) *Encrypted {
	return &Encrypted{cipher: append([]byte(nil), cipher...), keyID: keyID}
}

// Plain 仅供 service 层临时读取；不允许 logging / Sentry / handler json marshal.
func (e *Encrypted) Plain() string { return e.plain }

// Cipher 持久化值。
func (e *Encrypted) Cipher() []byte { return e.cipher }

// KeyID DEK 句柄（含 keyVersion）.
func (e *Encrypted) KeyID() string { return e.keyID }

// SetCipher service 在 KMS Encrypt 完成后回写。
func (e *Encrypted) SetCipher(cipher []byte, keyID string) {
	e.cipher = append([]byte(nil), cipher...)
	e.keyID = keyID
	e.plain = "" // 立刻清掉 plain，防止落日志
}

// Value 实现 driver.Valuer：只写 cipher。
func (e *Encrypted) Value() (driver.Value, error) {
	return e.cipher, nil
}
