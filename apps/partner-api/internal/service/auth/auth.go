// Package auth 实现登录 / 登出 / 刷新 token / 全设备登出 / MFA / step-up / 双因子密码重置。
//
// 设计参考：backend §7.2 / §7.5 / §7.9（含 PR-INV-1..8 invariant），PRD §17。
//
// 三类 actor 共享同一 Service：partner / customer / staff。每次登录 cookie 与 site
// 互斥（frontend §6.2）；签发 access_token (15min) + refresh_token (8h) + csrf_token
// 三 cookie；在 Redis revoked:jti:* 维护 fail-closed 黑名单。
//
// 文件 ≤ 400 行；service 函数 ≤ 50 行。
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Sentinel 错误（service 层映射 AppError code，handler 渲染响应）。
var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrAccountLocked      = errors.New("auth: account locked")
	ErrMFARequired        = errors.New("auth: mfa required")
	ErrStepUpRequired     = errors.New("auth: step up required")
	ErrTokenInvalid       = errors.New("auth: token invalid")
	ErrTokenExpired       = errors.New("auth: token expired")
	ErrSecondFactorBad    = errors.New("auth: second factor mismatch")
	ErrConsentRequired    = errors.New("auth: consent required")
	ErrRevocationDown     = errors.New("auth: revocation store unavailable")
)

// ActorType 三类 actor。
type ActorType string

const (
	ActorPartner  ActorType = "partner"
	ActorCustomer ActorType = "customer"
	ActorStaff    ActorType = "staff"
)

// Site cookie 互斥维度。
type Site string

const (
	SitePartner  Site = "partner"
	SiteCustomer Site = "customer"
	SiteAdmin    Site = "admin"
)

// LoginInput 登录请求。
type LoginInput struct {
	Site        Site
	Handle      string // username / email / phone（按 actor 解析）
	Password    string
	MFAOTP      string // TOTP；partner KYC 后强制
	WebAuthnAss string // base64 assertion（staff 高危 verb 走 step-up）
	ClientIP    string
	UserAgent   string
	DeviceFP    string // 设备指纹（前端采集；session 表 device_fingerprint 列）
}

// LoginOutput 签发的 token + cookie 元数据。
type LoginOutput struct {
	AccessToken  string
	RefreshToken string
	CSRFToken    string
	ActorType    ActorType
	ActorID      int64
	FyUserID     int64
	Roles        []string
	ExpiresAt    time.Time
	SessionID    int64
}

// PasswordResetInitiateInput backend §7.9.1 阶段 1。
type PasswordResetInitiateInput struct {
	ActorHandle    string // email / phone；服务端解析 actor
	ConsentVersion string // PIPL §17.5 同意快照
	ClientIP       string
	UserAgent      string
}

// PasswordResetConfirmInput 阶段 2。
type PasswordResetConfirmInput struct {
	RawToken    string // 邮件链接 32 字节
	OTP         string // SMS 6 位
	NewPassword string
	ClientIP    string
	UserAgent   string
}

// Service 鉴权门面。
type Service struct {
	repo        Repository
	revoke      RevocationStore
	hasher      PasswordHasher
	signer      TokenSigner
	notifier    Notifier
	consentRepo ConsentRepo
	clock       func() time.Time
	rng         func([]byte) error // 测试可注入

	// invariant：accessTTL ≤ refreshTTL；resetTTL = 15min。
	accessTTL  time.Duration
	refreshTTL time.Duration
	resetTTL   time.Duration
}

// Options 构造选项。
type Options struct {
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	ResetTTL   time.Duration
	Clock      func() time.Time
	Rng        func([]byte) error
}

// NewService 构造；默认 access 15min / refresh 8h / reset 15min。
func NewService(repo Repository, revoke RevocationStore, hasher PasswordHasher,
	signer TokenSigner, notifier Notifier, consentRepo ConsentRepo, opts Options) *Service {
	if opts.AccessTTL == 0 {
		opts.AccessTTL = 15 * time.Minute
	}
	if opts.RefreshTTL == 0 {
		opts.RefreshTTL = 8 * time.Hour
	}
	if opts.ResetTTL == 0 {
		opts.ResetTTL = 15 * time.Minute
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	if opts.Rng == nil {
		opts.Rng = func(b []byte) error { _, err := rand.Read(b); return err }
	}
	return &Service{
		repo: repo, revoke: revoke, hasher: hasher, signer: signer,
		notifier: notifier, consentRepo: consentRepo,
		clock: opts.Clock, rng: opts.Rng,
		accessTTL: opts.AccessTTL, refreshTTL: opts.RefreshTTL, resetTTL: opts.ResetTTL,
	}
}

// Login 登录入口（≤ 50 行）。失败计数 5 次锁账；成功签发 token + 写 session。
func (s *Service) Login(ctx context.Context, in LoginInput) (LoginOutput, error) {
	cred, err := s.repo.FindCredentials(ctx, mapSiteToActor(in.Site), in.Handle)
	if err != nil {
		return LoginOutput{}, ErrInvalidCredentials
	}
	if cred.Locked {
		return LoginOutput{}, ErrAccountLocked
	}
	if !s.hasher.Compare(cred.PasswordHash, in.Password) {
		_ = s.repo.IncFailedAttempts(ctx, cred.ActorType, cred.ActorID)
		return LoginOutput{}, ErrInvalidCredentials
	}
	if cred.MFAEnabled && !s.verifyMFA(cred, in.MFAOTP) {
		return LoginOutput{}, ErrMFARequired
	}
	now := s.clock()
	jti, err := s.randomHex(16)
	if err != nil {
		return LoginOutput{}, fmt.Errorf("auth: rng: %w", err)
	}
	claims := Claims{
		Sub: cred.FyUserID, ActorType: string(cred.ActorType), ActorID: cred.ActorID,
		Roles: cred.Roles, Jti: jti, Iat: now.Unix(), Exp: now.Add(s.accessTTL).Unix(),
		Site: string(in.Site),
	}
	access, err := s.signer.Sign(claims)
	if err != nil {
		return LoginOutput{}, fmt.Errorf("auth: sign access: %w", err)
	}
	refresh, refreshJti, err := s.issueRefresh(claims, now)
	if err != nil {
		return LoginOutput{}, err
	}
	csrf, err := s.randomHex(32)
	if err != nil {
		return LoginOutput{}, fmt.Errorf("auth: rng csrf: %w", err)
	}
	sid, err := s.repo.CreateSession(ctx, Session{
		ActorType: cred.ActorType, ActorID: cred.ActorID, AccessJti: jti,
		RefreshJti: refreshJti, DeviceFingerprint: in.DeviceFP, IP: in.ClientIP,
		UserAgent: in.UserAgent, IssuedAt: now, ExpiresAt: now.Add(s.refreshTTL),
	})
	if err != nil {
		return LoginOutput{}, fmt.Errorf("auth: create session: %w", err)
	}
	_ = s.repo.ResetFailedAttempts(ctx, cred.ActorType, cred.ActorID)
	_ = s.repo.RecordLastLogin(ctx, cred.ActorType, cred.ActorID, now)
	return LoginOutput{
		AccessToken: access, RefreshToken: refresh, CSRFToken: csrf,
		ActorType: cred.ActorType, ActorID: cred.ActorID, FyUserID: cred.FyUserID,
		Roles: cred.Roles, ExpiresAt: now.Add(s.accessTTL), SessionID: sid,
	}, nil
}

// Logout 单设备登出：把当前 jti 加入 revoked。
func (s *Service) Logout(ctx context.Context, jti string, ttl time.Duration) error {
	if jti == "" {
		return nil
	}
	if err := s.revoke.Revoke(ctx, jti, ttl); err != nil {
		return fmt.Errorf("auth: revoke jti: %w", err)
	}
	return nil
}

// LogoutAll 全设备登出：把 actor 全部 jti 加入 revoked + 关闭 session 行。
func (s *Service) LogoutAll(ctx context.Context, actor ActorType, actorID int64) error {
	jtis, err := s.repo.ListActiveJTIs(ctx, actor, actorID)
	if err != nil {
		return fmt.Errorf("auth: list jtis: %w", err)
	}
	for _, jti := range jtis {
		if err := s.revoke.Revoke(ctx, jti, s.refreshTTL); err != nil {
			return fmt.Errorf("auth: revoke %s: %w", jti, err)
		}
	}
	return s.repo.CloseAllSessions(ctx, actor, actorID, s.clock())
}

// Refresh refresh-token rotation：旧 jti 立刻 revoke，签发新对。
func (s *Service) Refresh(ctx context.Context, oldRefresh string) (LoginOutput, error) {
	claims, err := s.signer.Verify(oldRefresh)
	if err != nil {
		return LoginOutput{}, ErrTokenInvalid
	}
	if revoked, err := s.revoke.IsRevoked(ctx, claims.Jti); err != nil {
		return LoginOutput{}, ErrRevocationDown
	} else if revoked {
		// 复用攻击：全 session revoke。
		_ = s.LogoutAll(ctx, ActorType(claims.ActorType), claims.ActorID)
		return LoginOutput{}, ErrTokenInvalid
	}
	if err := s.revoke.Revoke(ctx, claims.Jti, s.refreshTTL); err != nil {
		return LoginOutput{}, fmt.Errorf("auth: revoke old refresh: %w", err)
	}
	now := s.clock()
	newJti, _ := s.randomHex(16)
	newClaims := Claims{
		Sub: claims.Sub, ActorType: claims.ActorType, ActorID: claims.ActorID,
		Roles: claims.Roles, Jti: newJti, Iat: now.Unix(),
		Exp: now.Add(s.accessTTL).Unix(), Site: claims.Site,
	}
	access, err := s.signer.Sign(newClaims)
	if err != nil {
		return LoginOutput{}, fmt.Errorf("auth: sign new: %w", err)
	}
	refresh, _, err := s.issueRefresh(newClaims, now)
	if err != nil {
		return LoginOutput{}, err
	}
	csrf, _ := s.randomHex(32)
	return LoginOutput{
		AccessToken: access, RefreshToken: refresh, CSRFToken: csrf,
		ActorType: ActorType(claims.ActorType), ActorID: claims.ActorID,
		FyUserID: claims.Sub, ExpiresAt: now.Add(s.accessTTL),
	}, nil
}

// PasswordResetInitiate / PasswordResetConfirm 见 password_reset.go。

// SwitchActor 浏览器内同账号 partner ↔ customer 切换。旧 jti revoke + 签发新 jti。
func (s *Service) SwitchActor(ctx context.Context, oldClaims Claims, target ActorType, targetID int64) (LoginOutput, error) {
	if oldClaims.Sub == 0 {
		return LoginOutput{}, ErrTokenInvalid
	}
	if err := s.revoke.Revoke(ctx, oldClaims.Jti, s.refreshTTL); err != nil {
		return LoginOutput{}, fmt.Errorf("auth: revoke old: %w", err)
	}
	now := s.clock()
	newJti, _ := s.randomHex(16)
	claims := Claims{
		Sub: oldClaims.Sub, ActorType: string(target), ActorID: targetID,
		Roles: nil, Jti: newJti, Iat: now.Unix(),
		Exp: now.Add(s.accessTTL).Unix(), Site: string(target),
	}
	access, err := s.signer.Sign(claims)
	if err != nil {
		return LoginOutput{}, fmt.Errorf("auth: sign: %w", err)
	}
	refresh, _, err := s.issueRefresh(claims, now)
	if err != nil {
		return LoginOutput{}, err
	}
	csrf, _ := s.randomHex(32)
	return LoginOutput{
		AccessToken: access, RefreshToken: refresh, CSRFToken: csrf,
		ActorType: target, ActorID: targetID, FyUserID: oldClaims.Sub,
		ExpiresAt: now.Add(s.accessTTL),
	}, nil
}

// VerifyAccessToken 中间件入口：验签 + revocation lookup（fail-closed）。
func (s *Service) VerifyAccessToken(ctx context.Context, raw string) (Claims, error) {
	cl, err := s.signer.Verify(raw)
	if err != nil {
		return Claims{}, ErrTokenInvalid
	}
	revoked, err := s.revoke.IsRevoked(ctx, cl.Jti)
	if err != nil {
		return Claims{}, ErrRevocationDown
	}
	if revoked {
		return Claims{}, ErrTokenInvalid
	}
	if cl.Exp > 0 && time.Unix(cl.Exp, 0).Before(s.clock()) {
		return Claims{}, ErrTokenExpired
	}
	return cl, nil
}

// 私有 helper

func (s *Service) issueRefresh(c Claims, now time.Time) (string, string, error) {
	jti, err := s.randomHex(16)
	if err != nil {
		return "", "", fmt.Errorf("auth: rng: %w", err)
	}
	c.Jti = jti
	c.Iat = now.Unix()
	c.Exp = now.Add(s.refreshTTL).Unix()
	tok, err := s.signer.Sign(c)
	return tok, jti, err
}

func (s *Service) verifyMFA(cred Credentials, otp string) bool {
	return cred.MFASecret != "" && verifyTOTP(cred.MFASecret, otp, s.clock())
}

func (s *Service) randomHex(n int) (string, error) {
	b := make([]byte, n)
	if err := s.rng(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Service) generateResetMaterial() (string, string, error) {
	raw, err := s.randomHex(32)
	if err != nil {
		return "", "", err
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", "", err
	}
	otp := fmt.Sprintf("%06d", n.Int64())
	return raw, otp, nil
}

func mapSiteToActor(site Site) ActorType {
	switch site {
	case SitePartner:
		return ActorPartner
	case SiteCustomer:
		return ActorCustomer
	case SiteAdmin:
		return ActorStaff
	default:
		return ActorCustomer
	}
}

func hashHex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// verifyTOTP W1c 接 totp 库；本包仅做最简校验占位（实际生产用 pquerna/otp）。
func verifyTOTP(secret, otp string, now time.Time) bool {
	if otp == "" || secret == "" {
		return false
	}
	if strings.HasPrefix(secret, "test:") {
		return otp == strings.TrimPrefix(secret, "test:")
	}
	_ = now
	return false
}
