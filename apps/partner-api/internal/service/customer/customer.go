// Package customer 终端客户主数据 + 邀请码绑定 + 状态机 + 切换渠道商 + PIPL 右遗忘。
//
// 引用：PRD §8.2 / §8.20 / §7.9 防绕过 / 场景 H / 场景 I / 场景 N / 场景 Q。
//
// 关键 invariant：
//   - 注册 customer 时必走 invitation_code 路径（防绕过 §7.9）
//   - 同一 fy_user_id 不可既是 partner_X 又是 partner_Y 的 customer（D-18）
//   - 终止 partner 时所有客户 status='orphaned'，grace_until = now + 30d；过期未转挂 → cron 转 direct
//   - PIPL 右遗忘：customer.deleted_at + status='deleted' + Fy-api /user/erase；不删 audit_log / revenue_log
package customer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/saga"
)

// 状态枚举（与 domain.CustomerStatus 对齐）。
const (
	StatusActive      = domain.CustomerStatusActive
	StatusDisabled    = domain.CustomerStatusDisabled
	StatusTransferred = domain.CustomerStatusTransferred
	StatusOrphaned    = domain.CustomerStatusOrphaned
	StatusAdopted     = domain.CustomerStatusAdopted
	StatusDirect      = domain.CustomerStatusDirect
	StatusDeleted     = domain.CustomerStatusDeleted
)

// JoinedVia 加入方式。
const (
	JoinedInvitation       = "invitation"
	JoinedManualCreate     = "manual_create"
	JoinedSelfRegisterWith = "self_register_with_code"
	JoinedDirect           = "direct"
)

// Sentinel.
var (
	ErrCustomerNotFound       = errors.New("customer: not found")
	ErrAlreadyDirect          = errors.New("customer: already direct")
	ErrAlreadyAffiliated      = errors.New("customer: already affiliated to partner")
	ErrSelfTransferNotAllowed = errors.New("customer: cannot transfer to same partner")
	ErrInvitationRequired     = errors.New("customer: invitation_code required (防绕过)")
	ErrInvalidStatusForOp     = errors.New("customer: invalid status for operation")
)

// RegisterInput 客户注册（PRD §8.2 + 防绕过 §7.9）。
type RegisterInput struct {
	FyUserID       int64  // Fy-api `/user/create` 返回的 user_id
	InvitationCode string // 必填（防绕过强制）；admin 直营单独走 RegisterDirect
	JoinedVia      string // 留空时按 InvitationCode 是否为空判定
	// ConsentTextVersion Fix-C P1-7：客户端必须传当前条款版本；service 构造时注入的
	// ConsentVerifier 会断言 == biz_setting.compliance.consent_text_version。
	ConsentTextVersion string
}

// Validate 入参校验。
func (in RegisterInput) Validate() error {
	if in.FyUserID <= 0 {
		return errors.New("customer: fy_user_id required")
	}
	if strings.TrimSpace(in.InvitationCode) == "" {
		return ErrInvitationRequired
	}
	return nil
}

// Repository 客户持久化端口；BOLA 强制 partner_id scope。
type Repository interface {
	Insert(ctx context.Context, c domain.Customer) (int64, error)
	FindByIDForPartner(ctx context.Context, partnerID, customerID int64) (*domain.Customer, error)
	FindByID(ctx context.Context, customerID int64) (*domain.Customer, error) // staff 用
	FindByFyUserID(ctx context.Context, fyUserID int64) (*domain.Customer, error)
	Update(ctx context.Context, customerID int64, updater func(domain.Customer) domain.Customer) (*domain.Customer, error)
	OrphanByPartner(ctx context.Context, partnerID int64, graceUntil time.Time, at time.Time) (count int, err error)
	ListByPartner(ctx context.Context, partnerID int64, f ListFilter) ([]domain.Customer, error)
	InsertChangeLog(ctx context.Context, log domain.CustomerPartnerChangeLog) (int64, error)
	UpdateChangeLog(ctx context.Context, id int64, updater func(domain.CustomerPartnerChangeLog) domain.CustomerPartnerChangeLog) (*domain.CustomerPartnerChangeLog, error)
}

// ListFilter 过滤。
type ListFilter struct {
	Status domain.CustomerStatus
	Limit  int
	Offset int
}

// InvitationResolver 与 invitation pkg 解耦：customer 注册时调 Resolve / Consume。
type InvitationResolver interface {
	Resolve(ctx context.Context, code string) (*domain.InvitationCode, error)
	Consume(ctx context.Context, code string) (*domain.InvitationCode, error)
}

// FyAPIPort Fy-api 客户端最小子集（与 W1a infra/fyapi.Client 对齐）。
type FyAPIPort interface {
	UpdateUserGroup(ctx context.Context, fyUserID int64, group string, idemKey string) error
	EraseUser(ctx context.Context, fyUserID int64, idemKey string) error
}

// ConsentVerifier Fix-C P1-7：consent_text_version guard 抽象（pkg/consent.VersionGuard 实现）.
type ConsentVerifier interface {
	Verify(version string) error
}

// alwaysOKConsent 默认实现：不强求版本（向后兼容）.
type alwaysOKConsent struct{}

func (alwaysOKConsent) Verify(string) error { return nil }

// Service 客户主数据门面。
type Service struct {
	repo       Repository
	inv        InvitationResolver
	fyapi      FyAPIPort
	clock      func() time.Time
	consent    ConsentVerifier
	idemDB     *gorm.DB
	idemInsert saga.IdempotencyInserter
}

// NewService .
func NewService(repo Repository, inv InvitationResolver, fyapi FyAPIPort) *Service {
	return &Service{repo: repo, inv: inv, fyapi: fyapi, clock: time.Now, consent: alwaysOKConsent{}}
}

// WithConsentVerifier Fix-C P1-7：注入版本守门员（main.go 启动期注入）。
func (s *Service) WithConsentVerifier(v ConsentVerifier) *Service { s.consent = v; return s }

// WithClock 测试注入。
func (s *Service) WithClock(c func() time.Time) *Service { s.clock = c; return s }

// WithIdempotencyStore enables DB-backed idempotency records for mutation entries.
func (s *Service) WithIdempotencyStore(db *gorm.DB, inserter saga.IdempotencyInserter) *Service {
	s.idemDB = db
	s.idemInsert = inserter
	return s
}

// Register 被邀请客户注册（场景 C）。
//
// 调用前提：handler 已校验 fy_user_id 在 fy_api_db 存在（W1c 实现 fyapi.Client）；
// service 不直接 SELECT fy_api_db。
func (s *Service) Register(ctx context.Context, in RegisterInput) (*domain.Customer, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if s.consent != nil {
		if err := s.consent.Verify(in.ConsentTextVersion); err != nil {
			return nil, err
		}
	}
	if existing, _ := s.repo.FindByFyUserID(ctx, in.FyUserID); existing != nil {
		if existing.PartnerID != nil {
			return nil, ErrAlreadyAffiliated
		}
		return nil, ErrAlreadyDirect
	}
	resolved, err := s.inv.Resolve(ctx, in.InvitationCode)
	if err != nil {
		return nil, fmt.Errorf("customer: resolve invitation: %w", err)
	}
	now := s.clock()
	partnerID := resolved.PartnerID
	c := domain.Customer{
		FyUserID:           in.FyUserID,
		PartnerID:          &partnerID,
		JoinedVia:          inferJoinedVia(in.JoinedVia, in.InvitationCode),
		InvitationCodeUsed: resolved.Code,
		Status:             StatusActive,
		GroupNameInFyAPI:   fmt.Sprintf("partner_%d_default", partnerID),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	id, err := s.repo.Insert(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("customer: insert: %w", err)
	}
	if _, err := s.inv.Consume(ctx, in.InvitationCode); err != nil {
		return nil, fmt.Errorf("customer: consume invitation: %w", err)
	}
	c.ID = id
	return &c, nil
}

// RegisterDirect 直营客户（admin 创建 / 自助但无 invitation_code）。
//
// 仅 admin / 内部 worker 可调；防绕过 §7.9 判断由 handler middleware 强制。
func (s *Service) RegisterDirect(ctx context.Context, fyUserID int64) (*domain.Customer, error) {
	if existing, _ := s.repo.FindByFyUserID(ctx, fyUserID); existing != nil {
		return nil, ErrAlreadyDirect
	}
	now := s.clock()
	c := domain.Customer{
		FyUserID: fyUserID, PartnerID: nil,
		JoinedVia: JoinedDirect, Status: StatusDirect,
		GroupNameInFyAPI: "default", CreatedAt: now, UpdatedAt: now,
	}
	id, err := s.repo.Insert(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("customer: insert direct: %w", err)
	}
	c.ID = id
	return &c, nil
}

// GetForPartner BOLA-scoped 查询：partner 视角拿自己的客户。
func (s *Service) GetForPartner(ctx context.Context, partnerID, customerID int64) (*domain.Customer, error) {
	c, err := s.repo.FindByIDForPartner(ctx, partnerID, customerID)
	if err != nil {
		return nil, fmt.Errorf("customer: scoped find: %w", err)
	}
	if c == nil {
		return nil, ErrCustomerNotFound
	}
	return c, nil
}

// ListByPartner partner 自己的客户列表。
func (s *Service) ListByPartner(ctx context.Context, partnerID int64, f ListFilter) ([]domain.Customer, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	rows, err := s.repo.ListByPartner(ctx, partnerID, f)
	if err != nil {
		return nil, fmt.Errorf("customer: list: %w", err)
	}
	return rows, nil
}

// OrphanByPartner 实现 partner.CustomerOrphaner（场景 I）。
func (s *Service) OrphanByPartner(ctx context.Context, partnerID int64, graceUntil time.Time) error {
	if _, err := s.repo.OrphanByPartner(ctx, partnerID, graceUntil, s.clock()); err != nil {
		return fmt.Errorf("customer: orphan: %w", err)
	}
	return nil
}

// inferJoinedVia 根据 invitation_code 是否带判定。
func inferJoinedVia(explicit, code string) string {
	if explicit != "" {
		return explicit
	}
	if code != "" {
		return JoinedInvitation
	}
	return JoinedDirect
}
