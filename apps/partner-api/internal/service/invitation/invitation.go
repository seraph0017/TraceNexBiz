// Package invitation 邀请码生成 / 校验 / 失效。
//
// 引用：PRD §8.10 + §7.9 防绕过 + backend §5.1 invariant I-P-2。
//
// 关键约束：
//   - 邀请码 ≥ 16 字符（base32 32 byte CSPRNG → 26 字符）；全局 UNIQUE
//   - permanent / one_time / limited 三种类型；usage_limit > 0 时强制递增 + UNIQUE 校验
//   - 失效路径：expired (过期) / revoked (人工撤销) / 用满 (used_count == usage_limit)
//   - 防绕过：customer 注册必须走 invitation_code，session 内 sticky（customer pkg 实现）
package invitation

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// 类型枚举。
const (
	TypePermanent = "permanent"
	TypeOneTime   = "one_time"
	TypeLimited   = "limited"
)

// 状态枚举。
const (
	StatusActive  = "active"
	StatusExpired = "expired"
	StatusRevoked = "revoked"
	StatusUsedUp  = "used_up"
)

// Sentinel.
var (
	ErrCodeNotFound        = errors.New("invitation: code not found")
	ErrCodeInactive        = errors.New("invitation: code inactive")
	ErrCodeExpired         = errors.New("invitation: code expired")
	ErrCodeUsedUp          = errors.New("invitation: code used up")
	ErrInvalidType         = errors.New("invitation: invalid type")
	ErrCollisionExhausted  = errors.New("invitation: code collision retries exhausted")
)

// Repository invitation_code 持久化端口。
type Repository interface {
	Insert(ctx context.Context, c domain.InvitationCode) (int64, error)
	FindByCode(ctx context.Context, code string) (*domain.InvitationCode, error)
	IncUsedCount(ctx context.Context, code string) (*domain.InvitationCode, error)
	Update(ctx context.Context, code string, updater func(domain.InvitationCode) domain.InvitationCode) (*domain.InvitationCode, error)
	ListByPartner(ctx context.Context, partnerID int64) ([]domain.InvitationCode, error)
}

// Service 邀请码门面。
type Service struct {
	repo  Repository
	clock func() time.Time
	rng   func([]byte) error
}

// NewService 构造（默认 crypto/rand）。
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo, clock: time.Now,
		rng: func(b []byte) error { _, err := rand.Read(b); return err },
	}
}

// WithRng 测试注入。
func (s *Service) WithRng(r func([]byte) error) *Service { s.rng = r; return s }

// WithClock 测试注入。
func (s *Service) WithClock(c func() time.Time) *Service { s.clock = c; return s }

// Generate 为 partner 生成新 invitation_code（默认 permanent）。
//
// 实现 partner.InvitationGenerator 接口（duck-typed）。
func (s *Service) Generate(ctx context.Context, partnerID int64) (string, error) {
	return s.GenerateWith(ctx, GenerateInput{
		PartnerID: partnerID, Type: TypePermanent, UsageLimit: 0,
	})
}

// GenerateInput 完整生成参数。
type GenerateInput struct {
	PartnerID  int64
	Type       string
	UsageLimit int32      // 0 = 不限；type=one_time 时强制 1
	ExpiresAt  *time.Time // nil 表示不过期
}

// GenerateWith 复杂生成：支持 one_time / limited / TTL。
func (s *Service) GenerateWith(ctx context.Context, in GenerateInput) (string, error) {
	if in.PartnerID <= 0 {
		return "", errors.New("invitation: partner_id required")
	}
	if in.Type != TypePermanent && in.Type != TypeOneTime && in.Type != TypeLimited {
		return "", ErrInvalidType
	}
	if in.Type == TypeOneTime {
		in.UsageLimit = 1
	}
	if in.Type == TypeLimited && in.UsageLimit <= 0 {
		return "", errors.New("invitation: limited requires usage_limit > 0")
	}
	now := s.clock()
	for retry := 0; retry < 5; retry++ {
		code, err := s.randomCode()
		if err != nil {
			return "", fmt.Errorf("invitation: rng: %w", err)
		}
		c := domain.InvitationCode{
			PartnerID: in.PartnerID, Code: code,
			Type: in.Type, UsageLimit: in.UsageLimit,
			Status: StatusActive, ExpiresAt: in.ExpiresAt,
			CreatedAt: now, UpdatedAt: now,
		}
		if _, err := s.repo.Insert(ctx, c); err == nil {
			return code, nil
		}
	}
	return "", ErrCollisionExhausted
}

// Resolve 校验 invitation_code 是否可用 + 返回 partner_id（防绕过用）。
//
// 不增 used_count；增计数走 Consume（必须在业务 TX 内调用）。
func (s *Service) Resolve(ctx context.Context, code string) (*domain.InvitationCode, error) {
	if code == "" {
		return nil, ErrCodeNotFound
	}
	c, err := s.repo.FindByCode(ctx, strings.TrimSpace(code))
	if err != nil {
		return nil, fmt.Errorf("invitation: lookup: %w", err)
	}
	if c == nil {
		return nil, ErrCodeNotFound
	}
	if c.Status != StatusActive {
		return nil, ErrCodeInactive
	}
	now := s.clock()
	if c.ExpiresAt != nil && !c.ExpiresAt.After(now) {
		return nil, ErrCodeExpired
	}
	if c.UsageLimit > 0 && c.UsedCount >= c.UsageLimit {
		return nil, ErrCodeUsedUp
	}
	return c, nil
}

// Consume 递增 used_count；调用方应在 customer 注册同 TX 内调（W1a 提供 RepositoryTx 变体由 W1c/W1b 增补）。
//
// 当 type=one_time 或 used_count == usage_limit 时把状态改成 used_up。
func (s *Service) Consume(ctx context.Context, code string) (*domain.InvitationCode, error) {
	c, err := s.Resolve(ctx, code)
	if err != nil {
		return nil, err
	}
	updated, err := s.repo.IncUsedCount(ctx, c.Code)
	if err != nil {
		return nil, fmt.Errorf("invitation: inc used count: %w", err)
	}
	if updated.UsageLimit > 0 && updated.UsedCount >= updated.UsageLimit {
		updated, err = s.repo.Update(ctx, c.Code, func(x domain.InvitationCode) domain.InvitationCode {
			x.Status = StatusUsedUp
			return x
		})
		if err != nil {
			return nil, fmt.Errorf("invitation: mark used_up: %w", err)
		}
	}
	return updated, nil
}

// Revoke 人工撤销（admin / partner 自己）。
func (s *Service) Revoke(ctx context.Context, code string) (*domain.InvitationCode, error) {
	updated, err := s.repo.Update(ctx, code, func(c domain.InvitationCode) domain.InvitationCode {
		if c.Status == StatusRevoked {
			return c
		}
		c.Status = StatusRevoked
		return c
	})
	if err != nil {
		return nil, fmt.Errorf("invitation: revoke: %w", err)
	}
	return updated, nil
}

// ListByPartner 列出某 partner 的所有 invitation_code。
func (s *Service) ListByPartner(ctx context.Context, partnerID int64) ([]domain.InvitationCode, error) {
	rows, err := s.repo.ListByPartner(ctx, partnerID)
	if err != nil {
		return nil, fmt.Errorf("invitation: list: %w", err)
	}
	return rows, nil
}

// randomCode 生成 26 字符 base32 编码的高熵 code（PRD I-P-2 要求 ≥ 16 字符）。
func (s *Service) randomCode() (string, error) {
	b := make([]byte, 16)
	if err := s.rng(b); err != nil {
		return "", err
	}
	encoded := strings.ToUpper(strings.TrimRight(base32.StdEncoding.EncodeToString(b), "="))
	return encoded, nil
}
