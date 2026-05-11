// Package ticket 工单（PRD §7.10 + §8.18）.
//
// 视角：
//   - opener: partner / customer 创建工单
//   - replies: 三方均可回复（partner / customer / staff）
//   - assigned_to: staff（cs_admin / risk_admin / kyc_reviewer）
//
// 状态机：
//
//	open → assigned → responding → waiting_user → resolved → closed
//	          ↑                                       ↓
//	          └────────── reopened ←──────────────────┘
//
// category 枚举（v0.2 ARCH-MED-11）：
//   billing / kyc / api / content_report / other
package ticket

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrTicketNotFound .
var ErrTicketNotFound = errors.New("ticket: not found")

// ErrInvalidStateTransition .
var ErrInvalidStateTransition = errors.New("ticket: invalid state transition")

// ErrInvalidCategory .
var ErrInvalidCategory = errors.New("ticket: invalid category")

// validCategories whitelist.
var validCategories = map[string]struct{}{
	"billing": {}, "kyc": {}, "api": {}, "content_report": {}, "other": {},
}

// ValidateCategory 校验 category 枚举.
func ValidateCategory(c string) error {
	if _, ok := validCategories[c]; !ok {
		return ErrInvalidCategory
	}
	return nil
}

// validTransitions 状态机.
var validTransitions = map[string]map[string]struct{}{
	"open":          {"assigned": {}, "closed": {}},
	"assigned":      {"responding": {}, "open": {}, "closed": {}},
	"responding":    {"waiting_user": {}, "resolved": {}, "assigned": {}},
	"waiting_user":  {"responding": {}, "resolved": {}, "closed": {}},
	"resolved":      {"closed": {}, "reopened": {}},
	"reopened":      {"assigned": {}, "responding": {}},
	"closed":        {"reopened": {}},
}

func canTransition(from, to string) bool {
	if from == to {
		return true
	}
	if m, ok := validTransitions[from]; ok {
		_, ok := m[to]
		return ok
	}
	return false
}

// Ticket 值对象.
type Ticket struct {
	ID          int64
	OpenerType  string
	OpenerID    int64
	Subject     string
	Category    string
	Status      string
	AssignedTo  *int64
	Priority    int8
	LastReplyAt *time.Time
	SLADueAt    *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Reply 回复.
type Reply struct {
	ID          int64
	TicketID    int64
	SenderType  string
	SenderID    int64
	BodyMD      string
	Attachments string
	CreatedAt   time.Time
}

// Repo 持久化抽象.
type Repo interface {
	Insert(ctx context.Context, t *Ticket) (int64, error)
	Get(ctx context.Context, id int64) (*Ticket, error)
	Update(ctx context.Context, id int64, updater func(Ticket) Ticket) (*Ticket, error)
	InsertReply(ctx context.Context, r *Reply) (int64, error)
	ListReplies(ctx context.Context, ticketID int64) ([]Reply, error)
	List(ctx context.Context, q ListQuery) ([]Ticket, int, error)
}

// ListQuery filters for staff drill-down.
type ListQuery struct {
	OpenerType string // optional
	OpenerID   int64  // optional (BOLA scope when non-zero)
	Status     string
	Category   string
	AssignedTo int64
	Limit      int
	Offset     int
}

// Service 工单服务.
type Service struct {
	repo  Repo
	clock func() time.Time
}

// NewService 构造.
func NewService(r Repo) *Service { return &Service{repo: r, clock: time.Now} }

// CreateInput 三方均可创建（不同 opener_type）.
type CreateInput struct {
	OpenerType string
	OpenerID   int64
	Subject    string
	Category   string
	Priority   int8
	BodyMD     string
}

// Create 创建工单 + 首条 reply（自动）.
func (s *Service) Create(ctx context.Context, in CreateInput) (*Ticket, error) {
	if strings.TrimSpace(in.Subject) == "" {
		return nil, errors.New("ticket: subject required")
	}
	if err := ValidateCategory(in.Category); err != nil {
		return nil, err
	}
	if in.OpenerType != "partner" && in.OpenerType != "customer" && in.OpenerType != "staff" {
		return nil, errors.New("ticket: invalid opener_type")
	}
	now := s.clock()
	prio := in.Priority
	if prio == 0 {
		prio = 3
	}
	t := &Ticket{
		OpenerType: in.OpenerType, OpenerID: in.OpenerID,
		Subject: in.Subject, Category: in.Category, Status: "open",
		Priority: prio, CreatedAt: now, UpdatedAt: now,
	}
	id, err := s.repo.Insert(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("ticket: insert: %w", err)
	}
	t.ID = id
	if in.BodyMD != "" {
		_, _ = s.repo.InsertReply(ctx, &Reply{
			TicketID: id, SenderType: in.OpenerType, SenderID: in.OpenerID,
			BodyMD: in.BodyMD, CreatedAt: now,
		})
	}
	return t, nil
}

// Reply 三方任一回复，会驱动状态机推进.
//
//	staff 回复       → responding
//	customer/partner → waiting_user → responding（若已 waiting_user）
func (s *Service) Reply(ctx context.Context, ticketID int64, senderType string, senderID int64, body string) (*Ticket, *Reply, error) {
	t, err := s.repo.Get(ctx, ticketID)
	if err != nil {
		return nil, nil, err
	}
	now := s.clock()
	rid, err := s.repo.InsertReply(ctx, &Reply{
		TicketID: ticketID, SenderType: senderType, SenderID: senderID, BodyMD: body, CreatedAt: now,
	})
	if err != nil {
		return nil, nil, err
	}
	nextStatus := t.Status
	if senderType == "staff" {
		nextStatus = "responding"
	} else if t.Status == "waiting_user" {
		nextStatus = "responding"
	}
	updated, err := s.repo.Update(ctx, ticketID, func(c Ticket) Ticket {
		c.Status = nextStatus
		c.LastReplyAt = &now
		c.UpdatedAt = now
		return c
	})
	if err != nil {
		return nil, nil, err
	}
	return updated, &Reply{ID: rid, TicketID: ticketID, SenderType: senderType, SenderID: senderID, BodyMD: body, CreatedAt: now}, nil
}

// Assign staff 分派.
func (s *Service) Assign(ctx context.Context, id, staffID int64) (*Ticket, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canTransition(t.Status, "assigned") {
		return nil, ErrInvalidStateTransition
	}
	return s.repo.Update(ctx, id, func(c Ticket) Ticket {
		c.AssignedTo = &staffID
		c.Status = "assigned"
		c.UpdatedAt = s.clock()
		return c
	})
}

// Transition 通用状态迁移.
func (s *Service) Transition(ctx context.Context, id int64, to string) (*Ticket, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canTransition(t.Status, to) {
		return nil, ErrInvalidStateTransition
	}
	return s.repo.Update(ctx, id, func(c Ticket) Ticket {
		c.Status = to
		c.UpdatedAt = s.clock()
		return c
	})
}

// AdminList drill-down 接口（staff only；调用方校验权限）.
func (s *Service) AdminList(ctx context.Context, q ListQuery) ([]Ticket, int, error) {
	return s.repo.List(ctx, q)
}

// MyList opener 自己的工单（BOLA scope 强制）.
func (s *Service) MyList(ctx context.Context, openerType string, openerID int64, status string, limit, offset int) ([]Ticket, int, error) {
	return s.repo.List(ctx, ListQuery{OpenerType: openerType, OpenerID: openerID, Status: status, Limit: limit, Offset: offset})
}

// MemoryRepo 内存实现.
type MemoryRepo struct {
	mu        sync.Mutex
	tickets   map[int64]*Ticket
	replies   map[int64][]Reply
	nextT     int64
	nextR     int64
}

// NewMemoryRepo .
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{tickets: make(map[int64]*Ticket), replies: make(map[int64][]Reply)}
}

// Insert .
func (r *MemoryRepo) Insert(ctx context.Context, t *Ticket) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextT++
	t.ID = r.nextT
	cp := *t
	r.tickets[t.ID] = &cp
	return t.ID, nil
}

// Get .
func (r *MemoryRepo) Get(ctx context.Context, id int64) (*Ticket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tickets[id]
	if !ok {
		return nil, ErrTicketNotFound
	}
	cp := *t
	return &cp, nil
}

// Update immutable updater.
func (r *MemoryRepo) Update(ctx context.Context, id int64, updater func(Ticket) Ticket) (*Ticket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tickets[id]
	if !ok {
		return nil, ErrTicketNotFound
	}
	next := updater(*t)
	r.tickets[id] = &next
	cp := next
	return &cp, nil
}

// InsertReply .
func (r *MemoryRepo) InsertReply(ctx context.Context, rep *Reply) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextR++
	rep.ID = r.nextR
	r.replies[rep.TicketID] = append(r.replies[rep.TicketID], *rep)
	return rep.ID, nil
}

// ListReplies .
func (r *MemoryRepo) ListReplies(ctx context.Context, ticketID int64) ([]Reply, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Reply, len(r.replies[ticketID]))
	copy(out, r.replies[ticketID])
	return out, nil
}

// List with optional filters.
func (r *MemoryRepo) List(ctx context.Context, q ListQuery) ([]Ticket, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Ticket
	for _, t := range r.tickets {
		if q.OpenerType != "" && t.OpenerType != q.OpenerType {
			continue
		}
		if q.OpenerID != 0 && t.OpenerID != q.OpenerID {
			continue
		}
		if q.Status != "" && t.Status != q.Status {
			continue
		}
		if q.Category != "" && t.Category != q.Category {
			continue
		}
		if q.AssignedTo != 0 && (t.AssignedTo == nil || *t.AssignedTo != q.AssignedTo) {
			continue
		}
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	total := len(out)
	if q.Offset >= len(out) {
		return nil, total, nil
	}
	end := q.Offset + q.Limit
	if q.Limit == 0 || end > len(out) {
		end = len(out)
	}
	return out[q.Offset:end], total, nil
}
