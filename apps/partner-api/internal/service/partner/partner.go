// Package partner 渠道商主数据 service（PRD §8.1 / §7.3 模块三）。
//
// 覆盖：
//   - 招商落地页注册（场景 A 人工 / 场景 B 自助）
//   - 状态机 applied → reviewing → approved → frozen / suspended / terminated
//   - 场景 I：终止时把客户状态置 orphaned + 30d 宽限期
//   - 邀请码同 TX 创建（与 invitation pkg 共用 InvitationGenerator 端口）
//
// PII：contact_phone / contact_email 经 KMS 信封加密；email 走 HMAC blind index
// 用于全局唯一性约束（partner.uk_partner_email_hmac）。
//
// immutability：service 返回的 *Partner 不可被调用方 mutate；状态变更通过显式方法。
package partner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// 状态枚举（与 domain.PartnerStatus 对齐；handler 层禁止暴露）。
const (
	StatusApplied    = domain.PartnerStatusApplied
	StatusReviewing  = domain.PartnerStatusReviewing
	StatusApproved   = domain.PartnerStatusApproved
	StatusRejected   = domain.PartnerStatusRejected
	StatusFrozen     = domain.PartnerStatusFrozen
	StatusSuspended  = domain.PartnerStatusSuspended
	StatusTerminated = domain.PartnerStatusTerminated
)

// Sentinel.
var (
	ErrPartnerNotFound        = errors.New("partner: not found")
	ErrInvalidTransition      = errors.New("partner: invalid status transition")
	ErrConsentMissing         = errors.New("partner: consent token invalid or expired")
	ErrEmailAlreadyRegistered = errors.New("partner: email already registered")
)

// ApplyInput 渠道商注册入参（场景 A/B）。
//
// PII 字段（contact_phone / contact_email）service 入口仍以明文承载；
// repository 层在写入前调 KMS 加密。明文不入库 / 不入日志。
type ApplyInput struct {
	FyUserID     int64  // 已是平台账号；0 表示新建（W1c 实现 fyapi.UserCreate 后回填）
	Type         string // enterprise / individual
	BusinessName string
	ContactName  string
	ContactPhone string
	ContactEmail string
	ConsentID    int64 // consent_log.id（前端先 POST /consents 拿到）
	Note         string
}

// Validate 输入校验（不依赖 validator 库；handler 层另做 zod-style）。
func (in ApplyInput) Validate() error {
	if err := in.validateProfile(); err != nil {
		return err
	}
	if in.ConsentID <= 0 {
		return errors.New("partner: consent_id required")
	}
	return nil
}

func (in ApplyInput) validateProfile() error {
	if in.Type != "enterprise" && in.Type != "individual" {
		return errors.New("partner: type must be enterprise or individual")
	}
	if strings.TrimSpace(in.ContactName) == "" {
		return errors.New("partner: contact_name required")
	}
	if !strings.Contains(in.ContactEmail, "@") {
		return errors.New("partner: contact_email invalid")
	}
	if !strings.HasPrefix(in.ContactPhone, "+") || len(in.ContactPhone) < 8 {
		return errors.New("partner: contact_phone must be E.164")
	}
	return nil
}

// Repository 渠道商持久化端口。所有写都接收 ctx；BOLA scope 不在本接口（partner 主数据
// 是平台级，仅 staff / 自身可访问；handler 层强制 scope）。
type Repository interface {
	Insert(ctx context.Context, p domain.Partner) (int64, error)
	FindByID(ctx context.Context, id int64) (*domain.Partner, error)
	FindByFyUserID(ctx context.Context, fyUserID int64) (*domain.Partner, error)
	FindByEmailHMAC(ctx context.Context, hmac string) (*domain.Partner, error)
	Update(ctx context.Context, id int64, updater func(domain.Partner) domain.Partner) (*domain.Partner, error)
	List(ctx context.Context, filter ListFilter) ([]domain.Partner, error)
}

// ListFilter 列表查询。
type ListFilter struct {
	Status string
	Search string // 仅搜 invitation_code / fy_user_id 精确
	Limit  int
	Offset int
}

// CryptoPort KMS 加密 + HMAC 索引（与 W1d/W1a 的 infra/kms.Service 对齐）。
type CryptoPort interface {
	EncryptPhone(ctx context.Context, plain string) (cipher []byte, keyID string, err error)
	HMACEmail(ctx context.Context, email string) (string, error)
}

// ConsentPort 校验 consent_log（与 auth.ConsentRepo 解耦：consent 已经存在仅查 ID）。
type ConsentPort interface {
	FindByID(ctx context.Context, consentID int64) (consentedAt time.Time, kind string, withdrawn bool, err error)
}

// InvitationGenerator 与 invitation pkg 解耦（service 注入）。
type InvitationGenerator interface {
	Generate(ctx context.Context, partnerID int64) (code string, err error)
}

// CustomerOrphaner 终止 partner 时把客户置 orphaned + grace_until。
//
// 实现位于 customer pkg；本端口避免循环 import。
type CustomerOrphaner interface {
	OrphanByPartner(ctx context.Context, partnerID int64, graceUntil time.Time) error
}

// Service 渠道商主数据门面。
type Service struct {
	repo       Repository
	crypto     CryptoPort
	consent    ConsentPort
	invitation InvitationGenerator
	orphaner   CustomerOrphaner
	clock      func() time.Time
}

// NewService 构造。
func NewService(repo Repository, crypto CryptoPort, consent ConsentPort,
	inv InvitationGenerator, orphaner CustomerOrphaner) *Service {
	return &Service{
		repo: repo, crypto: crypto, consent: consent,
		invitation: inv, orphaner: orphaner, clock: time.Now,
	}
}

// WithClock 测试注入。
func (s *Service) WithClock(c func() time.Time) *Service { s.clock = c; return s }

// Apply 场景 B 自助申请；状态置 applied，签发邀请码。
func (s *Service) Apply(ctx context.Context, in ApplyInput) (*domain.Partner, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	at, kind, withdrawn, err := s.consent.FindByID(ctx, in.ConsentID)
	if err != nil {
		return nil, fmt.Errorf("partner: consent lookup: %w", err)
	}
	if withdrawn || !strings.Contains(kind, "sensitive_pi") {
		return nil, ErrConsentMissing
	}
	now := s.clock()
	if now.Sub(at) > 5*time.Minute {
		return nil, ErrConsentMissing
	}
	return s.createApplied(ctx, in)
}

// AdminCreate creates a partner from the staff console.
//
// The admin form only collects contact details. Until Fy-api user creation is
// wired into this service, use a negative synthetic fy_user_id so the NOT NULL
// + UNIQUE database invariant remains explicit and cannot collide with real
// Fy-api user IDs.
func (s *Service) AdminCreate(ctx context.Context, in ApplyInput) (*domain.Partner, error) {
	if strings.TrimSpace(in.Type) == "" {
		in.Type = "enterprise"
	}
	if in.FyUserID == 0 {
		in.FyUserID = pendingFyUserID(s.clock())
	}
	if err := in.validateProfile(); err != nil {
		return nil, err
	}
	return s.createApplied(ctx, in)
}

func (s *Service) createApplied(ctx context.Context, in ApplyInput) (*domain.Partner, error) {
	now := s.clock()
	emailHMAC, err := s.crypto.HMACEmail(ctx, strings.ToLower(strings.TrimSpace(in.ContactEmail)))
	if err != nil {
		return nil, fmt.Errorf("partner: hmac email: %w", err)
	}
	if existing, err := s.repo.FindByEmailHMAC(ctx, emailHMAC); err == nil && existing != nil {
		return nil, ErrEmailAlreadyRegistered
	}
	cipher, keyID, err := s.crypto.EncryptPhone(ctx, in.ContactPhone)
	if err != nil {
		return nil, fmt.Errorf("partner: encrypt phone: %w", err)
	}
	tier := tierForType(in.Type)
	p := domain.Partner{
		FyUserID:           in.FyUserID,
		InvitationCode:     provisionalInvitationCode(now, in.ContactEmail),
		Status:             StatusApplied,
		KYCStatus:          0,
		Tier:               tier,
		AppliedAt:          now,
		ContactName:        in.ContactName,
		ContactPhonePlain:  in.ContactPhone,
		ContactPhoneCipher: cipher,
		ContactPhoneKeyID:  keyID,
		ContactEmail:       in.ContactEmail,
		ContactEmailHMAC:   emailHMAC,
		TaxStatus:          domain.TaxIndividual,
		Notes:              in.Note,
	}
	id, err := s.repo.Insert(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("partner: insert: %w", err)
	}
	code, err := s.invitation.Generate(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("partner: generate invitation: %w", err)
	}
	p.ID = id
	p.InvitationCode = code
	if updated, err := s.repo.Update(ctx, id, func(current domain.Partner) domain.Partner {
		current.InvitationCode = code
		return current
	}); err == nil && updated != nil {
		p = *updated
		p.ContactPhonePlain = in.ContactPhone
	}
	return &p, nil
}

// Approve staff 审核通过 → status = approved + approved_at。
func (s *Service) Approve(ctx context.Context, partnerID int64, staffID int64) (*domain.Partner, error) {
	now := s.clock()
	updated, err := s.repo.Update(ctx, partnerID, func(p domain.Partner) domain.Partner {
		// invariant：approved 只能从 applied / reviewing 来。
		if p.Status != StatusApplied && p.Status != StatusReviewing {
			return p // 不变更，下层根据 dirty=false 决定是否短路
		}
		p.Status = StatusApproved
		p.ApprovedAt = &now
		p.ApprovedBy = &staffID
		return p
	})
	if err != nil {
		return nil, fmt.Errorf("partner: approve: %w", err)
	}
	if updated.Status != StatusApproved {
		return nil, ErrInvalidTransition
	}
	return updated, nil
}

// Reject staff 驳回 → status = rejected。
func (s *Service) Reject(ctx context.Context, partnerID int64, _ string) (*domain.Partner, error) {
	updated, err := s.repo.Update(ctx, partnerID, func(p domain.Partner) domain.Partner {
		if p.Status != StatusApplied && p.Status != StatusReviewing {
			return p
		}
		p.Status = StatusRejected
		return p
	})
	if err != nil {
		return nil, fmt.Errorf("partner: reject: %w", err)
	}
	if updated.Status != StatusRejected {
		return nil, ErrInvalidTransition
	}
	return updated, nil
}

// Suspend / Freeze 通用入口（reason 写 audit_log 由 middleware 完成）。
func (s *Service) Suspend(ctx context.Context, partnerID int64, reason string) (*domain.Partner, error) {
	return s.transitionFrozen(ctx, partnerID, StatusSuspended, reason)
}

// Freeze 风控冻结。
func (s *Service) Freeze(ctx context.Context, partnerID int64, reason string) (*domain.Partner, error) {
	return s.transitionFrozen(ctx, partnerID, StatusFrozen, reason)
}

func (s *Service) transitionFrozen(ctx context.Context, id int64, target domain.PartnerStatus, reason string) (*domain.Partner, error) {
	now := s.clock()
	updated, err := s.repo.Update(ctx, id, func(p domain.Partner) domain.Partner {
		if p.Status != StatusApproved {
			return p
		}
		p.Status = target
		p.FrozenAt = &now
		p.FrozenReason = reason
		return p
	})
	if err != nil {
		return nil, fmt.Errorf("partner: %s: %w", target, err)
	}
	if updated.Status != target {
		return nil, ErrInvalidTransition
	}
	return updated, nil
}

// Terminate 终止 + 触发客户孤儿化（场景 I）。
func (s *Service) Terminate(ctx context.Context, partnerID int64, reason string, graceDays int) (*domain.Partner, error) {
	if graceDays <= 0 {
		graceDays = 30
	}
	now := s.clock()
	graceUntil := now.AddDate(0, 0, graceDays)
	updated, err := s.repo.Update(ctx, partnerID, func(p domain.Partner) domain.Partner {
		if p.Status == StatusTerminated {
			return p
		}
		p.Status = StatusTerminated
		p.TerminatedAt = &now
		p.TerminatedReason = reason
		return p
	})
	if err != nil {
		return nil, fmt.Errorf("partner: terminate: %w", err)
	}
	if updated.Status != StatusTerminated {
		return nil, ErrInvalidTransition
	}
	if err := s.orphaner.OrphanByPartner(ctx, partnerID, graceUntil); err != nil {
		// 不回滚 partner.Status —— 业务上允许 cron 兜底
		return updated, fmt.Errorf("partner: orphan customers: %w", err)
	}
	return updated, nil
}

// Get 单个查询（返回 read-only 拷贝）。
func (s *Service) Get(ctx context.Context, partnerID int64) (*domain.Partner, error) {
	p, err := s.repo.FindByID(ctx, partnerID)
	if err != nil {
		return nil, fmt.Errorf("partner: get: %w", err)
	}
	if p == nil {
		return nil, ErrPartnerNotFound
	}
	return p, nil
}

// FindByFyUser 用 fy_user_id 反查（防绕过 / outbox poller 用）。
func (s *Service) FindByFyUser(ctx context.Context, fyUserID int64) (*domain.Partner, error) {
	p, err := s.repo.FindByFyUserID(ctx, fyUserID)
	if err != nil {
		return nil, fmt.Errorf("partner: find by fy_user_id: %w", err)
	}
	if p == nil {
		return nil, ErrPartnerNotFound
	}
	return p, nil
}

// List staff 列表查询。
func (s *Service) List(ctx context.Context, f ListFilter) ([]domain.Partner, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	rows, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("partner: list: %w", err)
	}
	return rows, nil
}

func tierForType(t string) int8 {
	if t == "enterprise" {
		return 3
	}
	return 1
}

func provisionalInvitationCode(now time.Time, email string) string {
	clean := strings.NewReplacer("@", "-", ".", "-", "+", "-").Replace(strings.ToLower(strings.TrimSpace(email)))
	if len(clean) > 24 {
		clean = clean[:24]
	}
	return fmt.Sprintf("PENDING-%d-%s", now.UnixNano(), clean)
}

var pendingFyUserSeq atomic.Int64

func pendingFyUserID(now time.Time) int64 {
	base := now.UnixNano()
	if base <= 0 {
		base = time.Now().UnixNano()
	}
	seq := pendingFyUserSeq.Add(1) % 1000
	id := base + seq
	if id < 0 {
		return id
	}
	return -id
}
