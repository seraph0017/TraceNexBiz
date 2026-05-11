// 鉴权领域类型与端口接口（DI 边界）。
package auth

import (
	"context"
	"time"
)

// Claims JWT payload；与 middleware.Claims 保持一致字段名。
type Claims struct {
	Sub       int64    `json:"sub"`
	ActorType string   `json:"actor_type"`
	ActorID   int64    `json:"actor_id"`
	Roles     []string `json:"roles"`
	Jti       string   `json:"jti"`
	Iat       int64    `json:"iat"`
	Exp       int64    `json:"exp"`
	Elev      bool     `json:"elev,omitempty"`
	Site      string   `json:"site,omitempty"`
}

// Credentials repository 返回的鉴权快照（不含 plain password）。
type Credentials struct {
	ActorType    ActorType
	ActorID      int64
	FyUserID     int64
	PasswordHash string
	MFAEnabled   bool
	MFASecret    string // KMS-decrypted 瞬态值；本结构体仅 service 内传递
	Email        string
	Phone        string
	Roles        []string
	Locked       bool
	FailedCount  int
	LastLogin    *time.Time
}

// Session 表（PRD §17 / backend §3 子表；W1a 落新 migration 0006_session.up.sql）。
type Session struct {
	ID                int64
	ActorType         ActorType
	ActorID           int64
	AccessJti         string
	RefreshJti        string
	DeviceFingerprint string
	IP                string
	UserAgent         string
	IssuedAt          time.Time
	ExpiresAt         time.Time
	ClosedAt          *time.Time
}

// PasswordResetToken backend §3.28。
type PasswordResetToken struct {
	ID               int64
	ActorType        ActorType
	ActorID          int64
	TokenHash        string
	SecondFactorType string
	SecondFactorHash string
	RequestedIP      string
	UserAgent        string
	ExpiresAt        time.Time
	ConsumedAt       *time.Time
	FailedAttempts   int
	InvalidatedAt    *time.Time
	CreatedAt        time.Time
}

// IsUsable PR-INV-1 / PR-INV-2 / PR-INV-3 综合判定。
func (t *PasswordResetToken) IsUsable(now time.Time) bool {
	if t == nil {
		return false
	}
	if t.ConsumedAt != nil || t.InvalidatedAt != nil {
		return false
	}
	if !t.ExpiresAt.After(now) {
		return false
	}
	if t.FailedAttempts >= 5 {
		return false
	}
	return true
}

// Repository auth 持久化端口。所有写入由 service 在 bizDB.Transaction 闭包内调用；
// 本接口仅为 mysql/auth_mysql.go 提供契约。
//
// 实现：W1a 增 internal/repository/mysql/auth_mysql.go。
type Repository interface {
	// FindCredentials 按 site/actor + handle 查；不存在或软删返 not-found error。
	FindCredentials(ctx context.Context, actor ActorType, handle string) (Credentials, error)
	// FindCredentialsAny 阶段 1 不知道 actor type 时使用（按 email/phone 反查；遍历 staff/partner/customer）。
	FindCredentialsAny(ctx context.Context, handle string) (Credentials, error)

	IncFailedAttempts(ctx context.Context, actor ActorType, actorID int64) error
	ResetFailedAttempts(ctx context.Context, actor ActorType, actorID int64) error
	RecordLastLogin(ctx context.Context, actor ActorType, actorID int64, at time.Time) error

	CreateSession(ctx context.Context, s Session) (int64, error)
	ListActiveJTIs(ctx context.Context, actor ActorType, actorID int64) ([]string, error)
	CloseAllSessions(ctx context.Context, actor ActorType, actorID int64, at time.Time) error

	InsertResetToken(ctx context.Context, t PasswordResetToken) error
	FindResetTokenByHash(ctx context.Context, hash string) (PasswordResetToken, error)
	IncResetFailedAttempts(ctx context.Context, id int64, at time.Time) error
	ApplyPasswordReset(ctx context.Context, t PasswordResetToken, newHash string, at time.Time) error
}

// RevocationStore Redis revoked:jti:* 抽象（fail-closed 政策由调用方决策）。
type RevocationStore interface {
	IsRevoked(ctx context.Context, jti string) (bool, error)
	Revoke(ctx context.Context, jti string, ttl time.Duration) error
}

// PasswordHasher argon2id 抽象（生产用 pkg/argon2idx）。
type PasswordHasher interface {
	Hash(plain string) (string, error)
	Compare(hash, plain string) bool
}

// TokenSigner JWT 签发与验证（生产 RS256；测试 HS256）。
type TokenSigner interface {
	Sign(c Claims) (string, error)
	Verify(token string) (Claims, error)
}

// Notifier 邮件 / SMS（W1c notify.Service 实现，注入到本包）。
type Notifier interface {
	SendResetLink(ctx context.Context, email, rawToken string) error
	SendResetOTP(ctx context.Context, phone, otp string) error
}

// ConsentRepo PIPL §17.5 同意检查（与 W1c content_safety 共用 consent_log 读端口）。
type ConsentRepo interface {
	HasConsent(ctx context.Context, fyUserID int64, kind, version string) (bool, error)
}
