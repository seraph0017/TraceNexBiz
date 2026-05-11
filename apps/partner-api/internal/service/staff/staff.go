// Package staff admin staff CRUD + 子角色（PRD §7.4 + §3.14）.
//
// 子角色（v1.0 ARCH 增强）：
//
//	super_admin     最终一级（唯一可建账号、可调权限矩阵）
//	risk_admin      风控（saga.force_resolve / KYC 终审等高敏 verb）
//	finance_admin   财务（settlement.run / invoice.review / wallet.adjust）
//	cs_admin        客服（ticket.assign / customer 信息只读）
//	kyc_reviewer    KYC 一审（kyc.approve / kyc.reject + 年额配额）
//
// 鉴权额外要求（W1c）：
//   - VPN / IP 白名单（middleware 占位 — 调用方传 sourceIP，service 只校验 allowlist）
//   - step-up MFA：elevated_until 由 service 维护；超过 15min 强制重做
package staff

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrStaffNotFound .
var ErrStaffNotFound = errors.New("staff: not found")

// ErrInvalidRole .
var ErrInvalidRole = errors.New("staff: invalid role")

// ErrIPNotAllowed VPN / IP 白名单失败.
var ErrIPNotAllowed = errors.New("staff: source IP not in allowlist")

// ErrSuperAdminOnly 仅 super_admin 可执行.
var ErrSuperAdminOnly = errors.New("staff: super_admin required")

// ErrStepUpRequired step-up MFA 过期.
var ErrStepUpRequired = errors.New("staff: step-up MFA required")

// Role enum.
type Role string

const (
	RoleSuperAdmin   Role = "super_admin"
	RoleRiskAdmin    Role = "risk_admin"
	RoleFinanceAdmin Role = "finance_admin"
	RoleCSAdmin      Role = "cs_admin"
	RoleKYCReviewer  Role = "kyc_reviewer"
)

// validRoles whitelist.
var validRoles = map[Role]struct{}{
	RoleSuperAdmin:   {},
	RoleRiskAdmin:    {},
	RoleFinanceAdmin: {},
	RoleCSAdmin:      {},
	RoleKYCReviewer:  {},
}

// ValidateRole 校验子角色 enum.
func ValidateRole(r Role) error {
	if _, ok := validRoles[r]; !ok {
		return ErrInvalidRole
	}
	return nil
}

// StepUpWindow 15 min step-up MFA 有效期（PRD §3.4 elevated）.
const StepUpWindow = 15 * time.Minute

// Staff 值对象.
type Staff struct {
	ID            int64
	Username      string
	PasswordHash  string
	Role          Role
	Email         string
	Status        string // active / suspended / disabled
	LastLogin     *time.Time
	ElevatedUntil *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// IPAllowlist 实现.
type IPAllowlist interface {
	Allow(ip string) bool
}

// CIDRAllowlist 简单 CIDR 列表实现.
type CIDRAllowlist struct {
	nets []*net.IPNet
}

// NewCIDRAllowlist 解析 CIDR 列表（"10.0.0.0/8" 等）；非法跳过.
func NewCIDRAllowlist(cidrs []string) *CIDRAllowlist {
	a := &CIDRAllowlist{}
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		_, n, err := net.ParseCIDR(c)
		if err == nil {
			a.nets = append(a.nets, n)
		}
	}
	return a
}

// Allow 校验 IP 是否在白名单内（空白名单 = 拒绝全部，fail-closed）.
func (a *CIDRAllowlist) Allow(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range a.nets {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

// Repo 持久化.
type Repo interface {
	Insert(ctx context.Context, s *Staff) (int64, error)
	Get(ctx context.Context, id int64) (*Staff, error)
	GetByUsername(ctx context.Context, username string) (*Staff, error)
	Update(ctx context.Context, id int64, updater func(Staff) Staff) (*Staff, error)
	List(ctx context.Context) ([]Staff, error)
}

// Service .
type Service struct {
	repo      Repo
	allowlist IPAllowlist
	clock     func() time.Time
}

// NewService 构造.
func NewService(r Repo, a IPAllowlist) *Service {
	return &Service{repo: r, allowlist: a, clock: time.Now}
}

// CreateInput 创建 staff 入参（仅 super_admin 可调）.
type CreateInput struct {
	ActorID      int64 // 调用方 staff_id
	ActorRole    Role
	ActorIP      string
	Username     string
	PasswordHash string
	Role         Role
	Email        string
}

// Create 仅 super_admin + IP 白名单内可创建.
func (s *Service) Create(ctx context.Context, in CreateInput) (*Staff, error) {
	if !s.allowlist.Allow(in.ActorIP) {
		return nil, ErrIPNotAllowed
	}
	if in.ActorRole != RoleSuperAdmin {
		return nil, ErrSuperAdminOnly
	}
	if err := ValidateRole(in.Role); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Username) == "" || in.PasswordHash == "" {
		return nil, errors.New("staff: username + password_hash required")
	}
	now := s.clock()
	st := &Staff{
		Username: in.Username, PasswordHash: in.PasswordHash, Role: in.Role,
		Email: in.Email, Status: "active", CreatedAt: now, UpdatedAt: now,
	}
	id, err := s.repo.Insert(ctx, st)
	if err != nil {
		return nil, fmt.Errorf("staff: insert: %w", err)
	}
	st.ID = id
	return st, nil
}

// SetRole 修改子角色（super_admin + step-up MFA + IP 白名单）.
func (s *Service) SetRole(ctx context.Context, actorID int64, actorRole Role, actorIP string, targetID int64, role Role) (*Staff, error) {
	if !s.allowlist.Allow(actorIP) {
		return nil, ErrIPNotAllowed
	}
	if actorRole != RoleSuperAdmin {
		return nil, ErrSuperAdminOnly
	}
	if err := ValidateRole(role); err != nil {
		return nil, err
	}
	if err := s.requireStepUp(ctx, actorID); err != nil {
		return nil, err
	}
	return s.repo.Update(ctx, targetID, func(c Staff) Staff {
		c.Role = role
		c.UpdatedAt = s.clock()
		return c
	})
}

// SetStatus active / suspended / disabled.
func (s *Service) SetStatus(ctx context.Context, actorID int64, actorRole Role, actorIP string, targetID int64, status string) (*Staff, error) {
	if !s.allowlist.Allow(actorIP) {
		return nil, ErrIPNotAllowed
	}
	if actorRole != RoleSuperAdmin {
		return nil, ErrSuperAdminOnly
	}
	switch status {
	case "active", "suspended", "disabled":
	default:
		return nil, errors.New("staff: invalid status")
	}
	return s.repo.Update(ctx, targetID, func(c Staff) Staff {
		c.Status = status
		c.UpdatedAt = s.clock()
		return c
	})
}

// MarkStepUpPassed step-up MFA 通过；elevated_until = now + 15min.
func (s *Service) MarkStepUpPassed(ctx context.Context, staffID int64) error {
	until := s.clock().Add(StepUpWindow)
	_, err := s.repo.Update(ctx, staffID, func(c Staff) Staff {
		c.ElevatedUntil = &until
		c.UpdatedAt = s.clock()
		return c
	})
	return err
}

// CheckStepUp 仅校验是否 elevated（middleware / saga force-resolve 调）.
func (s *Service) CheckStepUp(ctx context.Context, staffID int64) error {
	return s.requireStepUp(ctx, staffID)
}

func (s *Service) requireStepUp(ctx context.Context, staffID int64) error {
	st, err := s.repo.Get(ctx, staffID)
	if err != nil {
		return err
	}
	if st.ElevatedUntil == nil || s.clock().After(*st.ElevatedUntil) {
		return ErrStepUpRequired
	}
	return nil
}

// List staff 列表（super_admin only；IP 白名单）.
func (s *Service) List(ctx context.Context, actorRole Role, actorIP string) ([]Staff, error) {
	if !s.allowlist.Allow(actorIP) {
		return nil, ErrIPNotAllowed
	}
	if actorRole != RoleSuperAdmin {
		return nil, ErrSuperAdminOnly
	}
	list, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list, nil
}

// MemoryRepo .
type MemoryRepo struct {
	mu    sync.Mutex
	rows  map[int64]*Staff
	byU   map[string]int64
	next  int64
}

// NewMemoryRepo .
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{rows: make(map[int64]*Staff), byU: make(map[string]int64)}
}

// Insert .
func (r *MemoryRepo) Insert(ctx context.Context, s *Staff) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byU[s.Username]; ok {
		return 0, errors.New("staff: username taken")
	}
	r.next++
	s.ID = r.next
	cp := *s
	r.rows[s.ID] = &cp
	r.byU[s.Username] = s.ID
	return s.ID, nil
}

// Get .
func (r *MemoryRepo) Get(ctx context.Context, id int64) (*Staff, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.rows[id]
	if !ok {
		return nil, ErrStaffNotFound
	}
	cp := *v
	return &cp, nil
}

// GetByUsername .
func (r *MemoryRepo) GetByUsername(ctx context.Context, username string) (*Staff, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byU[username]
	if !ok {
		return nil, ErrStaffNotFound
	}
	cp := *r.rows[id]
	return &cp, nil
}

// Update .
func (r *MemoryRepo) Update(ctx context.Context, id int64, updater func(Staff) Staff) (*Staff, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.rows[id]
	if !ok {
		return nil, ErrStaffNotFound
	}
	next := updater(*v)
	r.rows[id] = &next
	cp := next
	return &cp, nil
}

// List .
func (r *MemoryRepo) List(ctx context.Context) ([]Staff, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Staff, 0, len(r.rows))
	for _, v := range r.rows {
		out = append(out, *v)
	}
	return out, nil
}
