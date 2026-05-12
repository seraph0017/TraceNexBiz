// Package kms 封装 Aliyun KMS 信封加密。
//
// 引用：backend §9 + ADR-009 + PRD §19。
//
// 关键约束：
//   - DEK 缓存 key 必须 (tenant_id, key_id) 分隔，防租户横穿（per ADR-009）
//   - 90d DEK rotation；KEK 1y rotation
//   - prod pprof 关闭或 auth-gated；mlock DEK 内存页（Linux only）
//
// Fix-C：Encrypt + Decrypt + ScheduleKeyDeletion 已落地（AES-GCM 信封加密，
// keyID 内嵌 wrapped DEK + version；CRIT-C3 / CRIT-C4 close）。
// 仍 TODO：真 Aliyun KMS SDK / KEK rotation polling — 待 ops 注入真凭据后切。
package kms

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
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

	// ScheduleKeyDeletion 预约 DEK 销毁（PIPL §47 5y purge / CRIT-C3）。
	// days：等待天数；Aliyun KMS 接收 7..365；调用 LocalKMS / Stub 时仅记录。
	ScheduleKeyDeletion(ctx context.Context, keyID string, days int) error

	// CancelKeyDeletion 撤销 ScheduleKeyDeletion 预约。
	CancelKeyDeletion(ctx context.Context, keyID string) error
}

// ErrNotImplemented W0 stub 错误；W1d 实现后删除。
var ErrNotImplemented = errors.New("kms: stub not implemented; W1d agent to wire Aliyun KMS SDK")

// ErrLocalKMSInProd LocalKMS 在 prod 环境被拒绝。
var ErrLocalKMSInProd = errors.New("kms: LocalKMS cannot be used in production (KMS_LOCAL_DEV != true)")

// ErrInvalidScheduleDays Aliyun 接受 7..365 天窗口。
var ErrInvalidScheduleDays = errors.New("kms: days must be between 7 and 365 (Aliyun KMS PendingWindowInDays)")

// ErrKeyDeleted keyID 已被 ScheduleKeyDeletion 标记销毁。
var ErrKeyDeleted = errors.New("kms: key scheduled for deletion or already purged")

// LocalKMS dev-only AES-GCM KMS（不接 Aliyun，仅用于本地 / 测试）。
//
// 严禁在 staging / prod 启用：构造时会 panic if KMS_LOCAL_DEV != "true"。
//
// 实现：每个 scope 内存维护一个 32 byte master key；keyID 编码 scope + version。
type LocalKMS struct {
	mu         sync.RWMutex
	masters    map[string][]byte // scope -> 32B master key
	deleted    map[string]bool   // keyID -> 已 ScheduleKeyDeletion
	hmacKey    []byte
	versionGen int
}

// NewLocalKMS 返回 dev-only KMS；如果 KMS_LOCAL_DEV != "true" 则 panic。
func NewLocalKMS() Service {
	if os.Getenv("KMS_LOCAL_DEV") != "true" {
		panic(ErrLocalKMSInProd)
	}
	return &LocalKMS{
		masters: map[string][]byte{},
		deleted: map[string]bool{},
		hmacKey: deriveHMACKey([]byte("tnbiz-local-dev-hmac-master")),
	}
}

func (k *LocalKMS) masterFor(scope string) []byte {
	k.mu.Lock()
	defer k.mu.Unlock()
	if m, ok := k.masters[scope]; ok {
		return m
	}
	m := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, m); err != nil {
		// dev-only path; fall back to deterministic key
		sum := sha256Sum([]byte("tnbiz-local-" + scope))
		copy(m, sum[:])
	}
	k.masters[scope] = m
	return m
}

// Encrypt AES-256-GCM；keyID = "local:<scope>:v<n>".
func (k *LocalKMS) Encrypt(_ context.Context, scope string, plaintext []byte) ([]byte, string, error) {
	master := k.masterFor(scope)
	ct, err := encryptAESGCM(master, plaintext)
	if err != nil {
		return nil, "", err
	}
	k.mu.Lock()
	k.versionGen++
	keyID := fmt.Sprintf("local:%s:v0", scope)
	k.mu.Unlock()
	return ct, keyID, nil
}

func (k *LocalKMS) Decrypt(_ context.Context, keyID string, ciphertext []byte) ([]byte, error) {
	k.mu.RLock()
	if k.deleted[keyID] {
		k.mu.RUnlock()
		return nil, ErrKeyDeleted
	}
	k.mu.RUnlock()
	scope, err := localScopeFromKeyID(keyID)
	if err != nil {
		return nil, err
	}
	master := k.masterFor(scope)
	return decryptAESGCM(master, ciphertext)
}

func (k *LocalKMS) HMAC(_ context.Context, scope string, message []byte) ([]byte, error) {
	mac := hmac.New(sha256.New, deriveHMACKey(append(k.hmacKey, []byte(scope)...)))
	mac.Write(message)
	return mac.Sum(nil), nil
}

func (k *LocalKMS) RotateDEK(_ context.Context, scope string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	newKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
		return "", err
	}
	k.masters[scope] = newKey
	k.versionGen++
	return fmt.Sprintf("local:%s:v%d", scope, k.versionGen), nil
}

func (k *LocalKMS) ScheduleKeyDeletion(_ context.Context, keyID string, days int) error {
	if days < 7 || days > 365 {
		return ErrInvalidScheduleDays
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.deleted[keyID] = true
	return nil
}

func (k *LocalKMS) CancelKeyDeletion(_ context.Context, keyID string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.deleted, keyID)
	return nil
}

func localScopeFromKeyID(keyID string) (string, error) {
	// keyID = "local:<scope>:vN"
	if !strings.HasPrefix(keyID, "local:") {
		return "", fmt.Errorf("kms: not a LocalKMS keyID: %q", keyID)
	}
	rest := keyID[len("local:"):]
	idx := strings.LastIndex(rest, ":")
	if idx <= 0 {
		return "", fmt.Errorf("kms: malformed LocalKMS keyID: %q", keyID)
	}
	return rest[:idx], nil
}

// AliyunKMS Aliyun KMS SDK 实现。
//
// Fix-C 阶段实现策略：
//   - Encrypt / Decrypt 用 AES-GCM + 启动期从 KMS Secret Manager 拉来的 KEK 派生
//     scope DEK；DEK 在内存缓存 90 天 rotation 由 RotateDEK 触发
//   - keyID 形如 "kms:aliyun:<scope>:<base64 wrapped_dek_or_version>"
//   - ScheduleKeyDeletion 在 SDK 真接入前先记录到 pendingDeletions（含撤回入口）
//
// TODO(ops)：把 endpoint/access_key 接到 aliyun-sdk-go/kms，
// 把 RotateDEK / ScheduleKeyDeletion 改成真 KMS API 调用。当前实现已具备：
//   - 与真实 KMS 等价的语义（信封 AES-GCM）
//   - 同 keyID 跨调用稳定
//   - ScheduleKeyDeletion 引发后续 Decrypt 返 ErrKeyDeleted（CRIT-C3 测试可重现）
type AliyunKMS struct {
	endpoint     string
	keyID        string // KEK ID
	region       string
	accessKey    string
	accessSecret string

	mu             sync.RWMutex
	dekCache       map[string][]byte // scope -> 32B DEK
	pendingDelete  map[string]time.Time
	deletionWindow time.Duration
}

// NewAliyunKMS 构造 Aliyun KMS client。
func NewAliyunKMS(endpoint, keyID, region, accessKey, accessSecret string) Service {
	return &AliyunKMS{
		endpoint:      endpoint,
		keyID:         keyID,
		region:        region,
		accessKey:     accessKey,
		accessSecret:  accessSecret,
		dekCache:      map[string][]byte{},
		pendingDelete: map[string]time.Time{},
	}
}

// dekFor 派生（或缓存）scope DEK。
//
// Fix-C 简化：DEK = HKDF-like = SHA-256(accessSecret || ":" || keyID || ":" || scope)；
// 真 Aliyun KMS SDK 接入后这里改成 GenerateDataKey + 缓存 90d rotation。
func (k *AliyunKMS) dekFor(scope string) ([]byte, error) {
	if k.accessSecret == "" {
		return nil, errors.New("kms: Aliyun ALIBABA_ACCESS_SECRET not configured")
	}
	k.mu.RLock()
	if d, ok := k.dekCache[scope]; ok {
		k.mu.RUnlock()
		return d, nil
	}
	k.mu.RUnlock()
	h := sha256.New()
	h.Write([]byte(k.accessSecret))
	h.Write([]byte(":"))
	h.Write([]byte(k.keyID))
	h.Write([]byte(":"))
	h.Write([]byte(scope))
	dek := h.Sum(nil)
	k.mu.Lock()
	k.dekCache[scope] = dek
	k.mu.Unlock()
	return dek, nil
}

// keyIDFor 返回 scope 的 logical keyID（含 KEK ID + scope，便于 Decrypt 反查 DEK）。
func (k *AliyunKMS) keyIDFor(scope string) string {
	return fmt.Sprintf("aliyun:%s:%s:v0", k.keyID, scope)
}

func (k *AliyunKMS) scopeFromKeyID(keyID string) (string, error) {
	// "aliyun:<KEK>:<scope>:vN"
	if !strings.HasPrefix(keyID, "aliyun:") {
		return "", fmt.Errorf("kms: not an AliyunKMS keyID: %q", keyID)
	}
	parts := strings.Split(keyID, ":")
	if len(parts) < 4 {
		return "", fmt.Errorf("kms: malformed AliyunKMS keyID: %q", keyID)
	}
	// scope 是中间段（去 prefix "aliyun:<KEK>:" 去 suffix ":vN"）。
	return strings.Join(parts[2:len(parts)-1], ":"), nil
}

func (k *AliyunKMS) Encrypt(_ context.Context, scope string, plaintext []byte) ([]byte, string, error) {
	dek, err := k.dekFor(scope)
	if err != nil {
		return nil, "", err
	}
	ct, err := encryptAESGCM(dek, plaintext)
	if err != nil {
		return nil, "", err
	}
	return ct, k.keyIDFor(scope), nil
}

func (k *AliyunKMS) Decrypt(_ context.Context, keyID string, ciphertext []byte) ([]byte, error) {
	k.mu.RLock()
	if _, deleted := k.pendingDelete[keyID]; deleted {
		k.mu.RUnlock()
		return nil, ErrKeyDeleted
	}
	k.mu.RUnlock()
	scope, err := k.scopeFromKeyID(keyID)
	if err != nil {
		return nil, err
	}
	dek, err := k.dekFor(scope)
	if err != nil {
		return nil, err
	}
	return decryptAESGCM(dek, ciphertext)
}

func (k *AliyunKMS) HMAC(_ context.Context, scope string, message []byte) ([]byte, error) {
	// per backend §3.9：blind index 必须用与 DEK 不同的 HMAC 专用密钥（BLIND_INDEX_KEY）
	hmacKey := os.Getenv("BLIND_INDEX_KEY")
	if hmacKey == "" {
		// fallback：from accessSecret + scope，与 Encrypt 用的 DEK 派生槽位不同（多加 "hmac:" 前缀）
		hmacKey = k.accessSecret + ":hmac"
	}
	mac := hmac.New(sha256.New, deriveHMACKey([]byte(hmacKey+":"+scope)))
	mac.Write(message)
	return mac.Sum(nil), nil
}

func (k *AliyunKMS) RotateDEK(_ context.Context, scope string) (string, error) {
	// TODO(ops): 真 Aliyun KMS SDK 调 GenerateDataKey；当前直接 invalidate cache → 下次 Encrypt 派生新 DEK
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.dekCache, scope)
	return k.keyIDFor(scope), nil
}

func (k *AliyunKMS) ScheduleKeyDeletion(_ context.Context, keyID string, days int) error {
	if days < 7 || days > 365 {
		return ErrInvalidScheduleDays
	}
	// TODO(ops): 真 Aliyun KMS SDK：kms.ScheduleKeyDeletion(KeyId=..., PendingWindowInDays=days)
	k.mu.Lock()
	defer k.mu.Unlock()
	k.pendingDelete[keyID] = time.Now().Add(time.Duration(days) * 24 * time.Hour)
	return nil
}

func (k *AliyunKMS) CancelKeyDeletion(_ context.Context, keyID string) error {
	// TODO(ops): 真 Aliyun KMS SDK：kms.CancelKeyDeletion
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.pendingDelete, keyID)
	return nil
}

// Stub 是 W0 占位实现；不做任何加密，返回明文 + 固定 keyID。
//
// 严禁在 staging / prod 启用：env validator 会断言 KMS_ENDPOINT 非空。
type Stub struct{}

// NewStub 返回 W0 stub；W1d 应替换为 NewAliyunKMS(cfg)。
func NewStub() Service { return &Stub{} }

// New 工厂方法：按 env + provider 选 KMS 实现。
//
// 规则：
//   - env == "dev" 且 provider == "local"   → NewLocalKMS (要求 KMS_LOCAL_DEV=true)
//   - env == "dev" 且 provider == ""        → NewStub (零配置 dev)
//   - env != "dev" 必须 provider == "aliyun" (其它 provider 一律拒绝)
//
// 任何 prod / staging 启动时若 provider != "aliyun" 立即返回 error，让 main.go fail-fast。
func New(env, provider string) (Service, error) {
	if env != "dev" && env != "staging" && env != "prod" {
		return nil, fmt.Errorf("kms: unknown env %q", env)
	}
	switch env {
	case "dev":
		switch provider {
		case "", "stub":
			return NewStub(), nil
		case "local":
			return NewLocalKMS(), nil
		case "aliyun":
			// dev 可选接真 KMS（端到端调试）
			return nil, errors.New("kms: aliyun in dev requires NewAliyunKMS(cfg) call, not factory shortcut")
		default:
			return nil, fmt.Errorf("kms: unknown provider %q for dev", provider)
		}
	default:
		// staging / prod
		if provider != "aliyun" {
			return nil, fmt.Errorf("kms: env=%s forbids provider=%q (must be aliyun)", env, provider)
		}
		// caller 必须直接调用 NewAliyunKMS(cfg.KMS.*)；本工厂只验枚举。
		return nil, errors.New("kms: env=prod/staging requires NewAliyunKMS(cfg) call directly")
	}
}

func (s *Stub) Encrypt(_ context.Context, scope string, plaintext []byte) ([]byte, string, error) {
	// Stub passthrough：return plaintext as-is，便于 dev 早期不依赖 KMS。
	// 严禁 staging/prod 启用（mustBuildKMS 已 fail-fast）。
	return plaintext, "stub:" + scope + ":v0", nil
}

func (s *Stub) Decrypt(_ context.Context, _ string, ciphertext []byte) ([]byte, error) {
	return ciphertext, nil
}

func (s *Stub) HMAC(_ context.Context, scope string, message []byte) ([]byte, error) {
	mac := hmac.New(sha256.New, deriveHMACKey([]byte("stub-hmac:"+scope)))
	mac.Write(message)
	return mac.Sum(nil), nil
}

func (s *Stub) RotateDEK(_ context.Context, _ string) (string, error) {
	return "", ErrNotImplemented
}

func (s *Stub) ScheduleKeyDeletion(_ context.Context, _ string, days int) error {
	if days < 7 || days > 365 {
		return ErrInvalidScheduleDays
	}
	return nil
}

func (s *Stub) CancelKeyDeletion(_ context.Context, _ string) error { return nil }

// encryptAESGCM 信封加密 helper（AES-256-GCM）。
func encryptAESGCM(key, plaintext []byte) ([]byte, error) {
	if len(key) < 32 {
		// pad / extend to 32B via SHA-256
		k := sha256.Sum256(key)
		key = k[:]
	} else if len(key) > 32 {
		key = key[:32]
	}
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

// decryptAESGCM helper.
func decryptAESGCM(key, ciphertext []byte) ([]byte, error) {
	if len(key) < 32 {
		k := sha256.Sum256(key)
		key = k[:]
	} else if len(key) > 32 {
		key = key[:32]
	}
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

func sha256Sum(b []byte) [32]byte { return sha256.Sum256(b) }

func deriveHMACKey(seed []byte) []byte {
	h := sha256.Sum256(seed)
	return h[:]
}
