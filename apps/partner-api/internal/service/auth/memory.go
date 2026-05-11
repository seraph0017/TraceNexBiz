// 内存实现 + 简易 hasher / signer / revocation；用于单测和 dev 启动 fallback。
//
// 任何写入返回 entity 拷贝；调用方拿到的 Credentials / Session 视为 read-only。
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// MemoryRepo 单测 / dev fallback。
type MemoryRepo struct {
	mu       sync.RWMutex
	creds    map[string]Credentials // key = string(actor)+":"+handle
	byUserID map[string]Credentials
	sessions map[int64]Session
	resets   map[string]PasswordResetToken // key = token_hash
	jtis     map[string][]string
	nextID   int64
}

// NewMemoryRepo 构造空仓。
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		creds: map[string]Credentials{}, byUserID: map[string]Credentials{},
		sessions: map[int64]Session{}, resets: map[string]PasswordResetToken{},
		jtis: map[string][]string{},
	}
}

// SeedCredentials 注入测试账号（仅 test 用）。
func (r *MemoryRepo) SeedCredentials(c Credentials, handle string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.creds[string(c.ActorType)+":"+handle] = c
	r.byUserID[handle] = c
}

// FindCredentials .
func (r *MemoryRepo) FindCredentials(_ context.Context, actor ActorType, handle string) (Credentials, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.creds[string(actor)+":"+handle]
	if !ok {
		return Credentials{}, errors.New("auth: credentials not found")
	}
	return c, nil
}

// FindCredentialsAny .
func (r *MemoryRepo) FindCredentialsAny(_ context.Context, handle string) (Credentials, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.byUserID[handle]
	if !ok {
		return Credentials{}, errors.New("auth: credentials not found")
	}
	return c, nil
}

// IncFailedAttempts .
func (r *MemoryRepo) IncFailedAttempts(_ context.Context, actor ActorType, actorID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range r.creds {
		if v.ActorType == actor && v.ActorID == actorID {
			next := v
			next.FailedCount++
			if next.FailedCount >= 5 {
				next.Locked = true
			}
			r.creds[k] = next
		}
	}
	return nil
}

// ResetFailedAttempts .
func (r *MemoryRepo) ResetFailedAttempts(_ context.Context, actor ActorType, actorID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range r.creds {
		if v.ActorType == actor && v.ActorID == actorID {
			next := v
			next.FailedCount = 0
			r.creds[k] = next
		}
	}
	return nil
}

// RecordLastLogin .
func (r *MemoryRepo) RecordLastLogin(_ context.Context, actor ActorType, actorID int64, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range r.creds {
		if v.ActorType == actor && v.ActorID == actorID {
			next := v
			next.LastLogin = &at
			r.creds[k] = next
		}
	}
	return nil
}

// CreateSession .
func (r *MemoryRepo) CreateSession(_ context.Context, s Session) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	s.ID = r.nextID
	r.sessions[s.ID] = s
	key := string(s.ActorType) + ":" + fmt.Sprint(s.ActorID)
	r.jtis[key] = append(r.jtis[key], s.AccessJti, s.RefreshJti)
	return s.ID, nil
}

// ListActiveJTIs .
func (r *MemoryRepo) ListActiveJTIs(_ context.Context, actor ActorType, actorID int64) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := string(actor) + ":" + fmt.Sprint(actorID)
	out := make([]string, len(r.jtis[key]))
	copy(out, r.jtis[key])
	return out, nil
}

// CloseAllSessions .
func (r *MemoryRepo) CloseAllSessions(_ context.Context, actor ActorType, actorID int64, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, s := range r.sessions {
		if s.ActorType == actor && s.ActorID == actorID && s.ClosedAt == nil {
			next := s
			next.ClosedAt = &at
			r.sessions[id] = next
		}
	}
	key := string(actor) + ":" + fmt.Sprint(actorID)
	r.jtis[key] = nil
	return nil
}

// InsertResetToken .
func (r *MemoryRepo) InsertResetToken(_ context.Context, t PasswordResetToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.resets[t.TokenHash]; exists {
		return errors.New("auth: reset token hash collision")
	}
	r.nextID++
	t.ID = r.nextID
	t.CreatedAt = time.Now()
	r.resets[t.TokenHash] = t
	return nil
}

// FindResetTokenByHash .
func (r *MemoryRepo) FindResetTokenByHash(_ context.Context, hash string) (PasswordResetToken, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.resets[hash]
	if !ok {
		return PasswordResetToken{}, errors.New("auth: reset token not found")
	}
	return t, nil
}

// IncResetFailedAttempts PR-INV-3：5 次失败 → invalidate。
func (r *MemoryRepo) IncResetFailedAttempts(_ context.Context, id int64, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range r.resets {
		if v.ID == id {
			next := v
			next.FailedAttempts++
			if next.FailedAttempts >= 5 {
				next.InvalidatedAt = &at
			}
			r.resets[k] = next
			return nil
		}
	}
	return errors.New("auth: reset token not found")
}

// ApplyPasswordReset PR-INV-2 / PR-INV-4：consumed_at + 新 password_hash 同步。
func (r *MemoryRepo) ApplyPasswordReset(_ context.Context, t PasswordResetToken, newHash string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.resets[t.TokenHash]
	if !ok {
		return errors.New("auth: reset token vanished")
	}
	v.ConsumedAt = &at
	r.resets[t.TokenHash] = v
	for k, c := range r.creds {
		if c.ActorType == t.ActorType && c.ActorID == t.ActorID {
			next := c
			next.PasswordHash = newHash
			next.FailedCount = 0
			next.Locked = false
			r.creds[k] = next
		}
	}
	return nil
}

// MemoryRevocation Redis 不可达时的内存替身（仅 dev / test）。
type MemoryRevocation struct {
	mu      sync.RWMutex
	revoked map[string]time.Time
	clock   func() time.Time
}

// NewMemoryRevocation 构造。
func NewMemoryRevocation() *MemoryRevocation {
	return &MemoryRevocation{revoked: map[string]time.Time{}, clock: time.Now}
}

// IsRevoked .
func (m *MemoryRevocation) IsRevoked(_ context.Context, jti string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	exp, ok := m.revoked[jti]
	if !ok {
		return false, nil
	}
	return exp.After(m.clock()), nil
}

// Revoke .
func (m *MemoryRevocation) Revoke(_ context.Context, jti string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revoked[jti] = m.clock().Add(ttl)
	return nil
}

// SimpleHasher 简易 SHA256 hash（仅测试 / dev；生产替换为 argon2id）。
type SimpleHasher struct{ Salt string }

// Hash .
func (h SimpleHasher) Hash(plain string) (string, error) {
	sum := sha256.Sum256([]byte(h.Salt + plain))
	return hex.EncodeToString(sum[:]), nil
}

// Compare .
func (h SimpleHasher) Compare(hash, plain string) bool {
	got, _ := h.Hash(plain)
	return hmac.Equal([]byte(hash), []byte(got))
}

// HMACSigner HS256 签名器（测试 / dev）。生产用 RSASigner（W1a 后续接 jwt-go v5）。
type HMACSigner struct{ Secret []byte }

// Sign .
func (s HMACSigner) Sign(c Claims) (string, error) {
	body, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, s.Secret)
	mac.Write(body)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Verify .
func (s HMACSigner) Verify(token string) (Claims, error) {
	dot := -1
	for i := len(token) - 1; i >= 0; i-- {
		if token[i] == '.' {
			dot = i
			break
		}
	}
	if dot < 0 {
		return Claims{}, errors.New("auth: malformed token")
	}
	body, err := base64.RawURLEncoding.DecodeString(token[:dot])
	if err != nil {
		return Claims{}, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(token[dot+1:])
	if err != nil {
		return Claims{}, err
	}
	mac := hmac.New(sha256.New, s.Secret)
	mac.Write(body)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return Claims{}, errors.New("auth: bad signature")
	}
	var c Claims
	if err := json.Unmarshal(body, &c); err != nil {
		return Claims{}, err
	}
	return c, nil
}

// NoopNotifier 测试用占位。
type NoopNotifier struct{}

// SendResetLink .
func (NoopNotifier) SendResetLink(_ context.Context, _, _ string) error { return nil }

// SendResetOTP .
func (NoopNotifier) SendResetOTP(_ context.Context, _, _ string) error { return nil }

// AlwaysConsented 测试用 ConsentRepo 占位。
type AlwaysConsented struct{}

// HasConsent .
func (AlwaysConsented) HasConsent(_ context.Context, _ int64, _, _ string) (bool, error) {
	return true, nil
}
