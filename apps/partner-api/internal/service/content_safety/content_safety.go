// Package content_safety 内容安全 + 12377 上报通道（PRD §7.12 / COMP-CRIT-2）.
//
// W1c 范围（partner-api 侧）：
//
//   1. 接收 W1d Fy-api 端模型调用拦截事件 → INSERT content_safety_event
//      （Fy-api 通过 /api/internal/content-safety/event 上报，本包只负责 service / repo / dispatcher）
//   2. admin 审核 endpoint：list / detail / 改 disposition / 重派 12377
//   3. 12377 / 公安网安上报通道：dispatcher 拉 pending → HTTP 提交 → mark submitted
//      24h SLA：sla_due_at = created_at + 24h；超期未 submitted → alert
//   4. 失败 retry queue：retry_count < 5 → 回 pending；>= 5 → dead_letter + alert
package content_safety

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ErrEventNotFound .
var ErrEventNotFound = errors.New("content_safety: event not found")

// ErrInvalidDisposition .
var ErrInvalidDisposition = errors.New("content_safety: invalid disposition")

// ErrReportNotFound .
var ErrReportNotFound = errors.New("content_safety: report not found")

// validDispositions enum.
var validDispositions = map[string]struct{}{
	"block": {}, "review": {}, "pass": {}, "warn": {},
}

// SLAReport24h 12377 上报 24h SLA.
const SLAReport24h = 24 * time.Hour

// MaxRetries 失败重试上限.
const MaxRetries = 5

// Event 内容安全事件（domain.ContentSafetyEvent 的 service 层投影）.
type Event struct {
	ID                int64
	FyUserID          int64
	Kind              string
	Provider          string
	PromptHash        string
	Category          string
	Score             float64
	Disposition       string
	ReviewedBy        *int64
	ReviewedAt        *time.Time
	ReportedTo12377At *time.Time
	AuditLogID        *int64
	TraceID           string
	CreatedAt         time.Time
}

// Report 12377 / 公安网安 上报记录.
type Report struct {
	ID              int64
	EventID         int64
	TargetAuthority string
	Payload         string
	Status          string // pending / submitted / failed / dead_letter
	SubmittedAt     *time.Time
	SLADueAt        time.Time
	ResponsePayload string
	RetryCount      int
	LastError       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// AuthorityClient 12377 / 公安 API 客户端.
type AuthorityClient interface {
	Submit(ctx context.Context, authority, payload string) (responsePayload string, err error)
}

// Repo 持久化抽象.
type Repo interface {
	InsertEvent(ctx context.Context, e *Event) (int64, error)
	GetEvent(ctx context.Context, id int64) (*Event, error)
	UpdateEvent(ctx context.Context, id int64, updater func(Event) Event) (*Event, error)
	ListEvents(ctx context.Context, q ListQuery) ([]Event, int, error)

	InsertReport(ctx context.Context, r *Report) (int64, error)
	GetReport(ctx context.Context, id int64) (*Report, error)
	UpdateReport(ctx context.Context, id int64, updater func(Report) Report) (*Report, error)
	ListPendingReports(ctx context.Context, n int) ([]Report, error)
	ListReports(ctx context.Context, q ListQuery) ([]Report, int, error)
	ListSLABreaches(ctx context.Context, now time.Time) ([]Report, error)
}

// ListQuery filters.
type ListQuery struct {
	Disposition string
	Status      string
	Category    string
	FyUserID    int64
	Limit       int
	Offset      int
}

// Service 内容安全服务.
type Service struct {
	repo      Repo
	authority AuthorityClient
	clock     func() time.Time
}

// NewService 构造.
func NewService(r Repo, a AuthorityClient) *Service {
	return &Service{repo: r, authority: a, clock: time.Now}
}

// RecordEvent 记录单条 content_safety_event（Fy-api 侧调用 / 本地拦截器调用）.
//
// disposition='block' 自动派单 12377 上报（commercial 上线后 PRD §7.12）.
func (s *Service) RecordEvent(ctx context.Context, e Event) (*Event, error) {
	if _, ok := validDispositions[e.Disposition]; !ok {
		return nil, ErrInvalidDisposition
	}
	now := s.clock()
	e.CreatedAt = now
	id, err := s.repo.InsertEvent(ctx, &e)
	if err != nil {
		return nil, fmt.Errorf("content_safety: insert event: %w", err)
	}
	e.ID = id
	if e.Disposition == "block" {
		if _, err := s.QueueReport(ctx, id, "12377", buildPayload(&e)); err != nil {
			return nil, fmt.Errorf("content_safety: queue report: %w", err)
		}
	}
	return &e, nil
}

// QueueReport admin 一键上报 / 自动派单.
func (s *Service) QueueReport(ctx context.Context, eventID int64, authority, payload string) (*Report, error) {
	if _, err := s.repo.GetEvent(ctx, eventID); err != nil {
		return nil, err
	}
	now := s.clock()
	r := &Report{
		EventID: eventID, TargetAuthority: authority, Payload: payload,
		Status: "pending", SLADueAt: now.Add(SLAReport24h),
		CreatedAt: now, UpdatedAt: now,
	}
	id, err := s.repo.InsertReport(ctx, r)
	if err != nil {
		return nil, err
	}
	r.ID = id
	return r, nil
}

// AdminReview 平台审核（改 disposition + assign reviewer）.
func (s *Service) AdminReview(ctx context.Context, eventID int64, disposition string, reviewerID int64) (*Event, error) {
	if _, ok := validDispositions[disposition]; !ok {
		return nil, ErrInvalidDisposition
	}
	now := s.clock()
	return s.repo.UpdateEvent(ctx, eventID, func(e Event) Event {
		e.Disposition = disposition
		e.ReviewedBy = &reviewerID
		e.ReviewedAt = &now
		return e
	})
}

// ListEvents admin drill-down.
func (s *Service) ListEvents(ctx context.Context, q ListQuery) ([]Event, int, error) {
	return s.repo.ListEvents(ctx, q)
}

// ListReports admin drill-down.
func (s *Service) ListReports(ctx context.Context, q ListQuery) ([]Report, int, error) {
	return s.repo.ListReports(ctx, q)
}

// DispatchOnce 拉 pending → submit → 标记 submitted/failed.
//
// 该函数被 cron `12377.retry` 触发（W1c 这里只交付 service；cron 入口在 cmd/notify-dispatcher 后续）.
func (s *Service) DispatchOnce(ctx context.Context, batch int) (submitted, failed int, err error) {
	rows, err := s.repo.ListPendingReports(ctx, batch)
	if err != nil {
		return 0, 0, err
	}
	for _, row := range rows {
		resp, sErr := s.authority.Submit(ctx, row.TargetAuthority, row.Payload)
		if sErr != nil {
			deadLetter := row.RetryCount+1 >= MaxRetries
			_, _ = s.repo.UpdateReport(ctx, row.ID, func(r Report) Report {
				r.RetryCount++
				r.LastError = sErr.Error()
				r.UpdatedAt = s.clock()
				if deadLetter {
					r.Status = "dead_letter"
				} else {
					r.Status = "pending"
				}
				return r
			})
			failed++
			continue
		}
		now := s.clock()
		_, _ = s.repo.UpdateReport(ctx, row.ID, func(r Report) Report {
			r.Status = "submitted"
			r.SubmittedAt = &now
			r.ResponsePayload = resp
			r.UpdatedAt = now
			return r
		})
		// stamp event
		_, _ = s.repo.UpdateEvent(ctx, row.EventID, func(e Event) Event {
			if row.TargetAuthority == "12377" {
				e.ReportedTo12377At = &now
			}
			return e
		})
		submitted++
	}
	return submitted, failed, nil
}

// SLABreaches 24h SLA 内未 submitted 的 report 列表（cron alert 用）.
func (s *Service) SLABreaches(ctx context.Context) ([]Report, error) {
	return s.repo.ListSLABreaches(ctx, s.clock())
}

// RetryReport admin 手动重试.
func (s *Service) RetryReport(ctx context.Context, reportID int64) (*Report, error) {
	return s.repo.UpdateReport(ctx, reportID, func(r Report) Report {
		if r.Status == "dead_letter" || r.Status == "failed" {
			r.Status = "pending"
			r.LastError = ""
			r.UpdatedAt = s.clock()
		}
		return r
	})
}

func buildPayload(e *Event) string {
	return fmt.Sprintf(`{"fy_user_id":%d,"category":%q,"prompt_hash":%q,"score":%f}`,
		e.FyUserID, e.Category, e.PromptHash, e.Score)
}

// MemoryRepo 内存实现.
type MemoryRepo struct {
	mu      sync.Mutex
	events  map[int64]*Event
	reports map[int64]*Report
	nextE   int64
	nextR   int64
}

// NewMemoryRepo .
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{events: make(map[int64]*Event), reports: make(map[int64]*Report)}
}

// InsertEvent .
func (r *MemoryRepo) InsertEvent(ctx context.Context, e *Event) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextE++
	e.ID = r.nextE
	cp := *e
	r.events[e.ID] = &cp
	return e.ID, nil
}

// GetEvent .
func (r *MemoryRepo) GetEvent(ctx context.Context, id int64) (*Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.events[id]
	if !ok {
		return nil, ErrEventNotFound
	}
	cp := *v
	return &cp, nil
}

// UpdateEvent .
func (r *MemoryRepo) UpdateEvent(ctx context.Context, id int64, updater func(Event) Event) (*Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.events[id]
	if !ok {
		return nil, ErrEventNotFound
	}
	next := updater(*v)
	r.events[id] = &next
	cp := next
	return &cp, nil
}

// ListEvents .
func (r *MemoryRepo) ListEvents(ctx context.Context, q ListQuery) ([]Event, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Event
	for _, e := range r.events {
		if q.Disposition != "" && e.Disposition != q.Disposition {
			continue
		}
		if q.Category != "" && e.Category != q.Category {
			continue
		}
		if q.FyUserID != 0 && e.FyUserID != q.FyUserID {
			continue
		}
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return paginateEvents(out, q.Limit, q.Offset)
}

// InsertReport .
func (r *MemoryRepo) InsertReport(ctx context.Context, rep *Report) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextR++
	rep.ID = r.nextR
	cp := *rep
	r.reports[rep.ID] = &cp
	return rep.ID, nil
}

// GetReport .
func (r *MemoryRepo) GetReport(ctx context.Context, id int64) (*Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.reports[id]
	if !ok {
		return nil, ErrReportNotFound
	}
	cp := *v
	return &cp, nil
}

// UpdateReport .
func (r *MemoryRepo) UpdateReport(ctx context.Context, id int64, updater func(Report) Report) (*Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.reports[id]
	if !ok {
		return nil, ErrReportNotFound
	}
	next := updater(*v)
	r.reports[id] = &next
	cp := next
	return &cp, nil
}

// ListPendingReports FIFO.
func (r *MemoryRepo) ListPendingReports(ctx context.Context, n int) ([]Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Report
	for _, v := range r.reports {
		if v.Status == "pending" {
			out = append(out, *v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > n {
		out = out[:n]
	}
	return out, nil
}

// ListReports .
func (r *MemoryRepo) ListReports(ctx context.Context, q ListQuery) ([]Report, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Report
	for _, v := range r.reports {
		if q.Status != "" && v.Status != q.Status {
			continue
		}
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return paginateReports(out, q.Limit, q.Offset)
}

// ListSLABreaches .
func (r *MemoryRepo) ListSLABreaches(ctx context.Context, now time.Time) ([]Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Report
	for _, v := range r.reports {
		if v.Status == "submitted" {
			continue
		}
		if now.After(v.SLADueAt) {
			out = append(out, *v)
		}
	}
	return out, nil
}

func paginateEvents(in []Event, limit, offset int) ([]Event, int, error) {
	total := len(in)
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if limit == 0 || end > total {
		end = total
	}
	return in[offset:end], total, nil
}

func paginateReports(in []Report, limit, offset int) ([]Report, int, error) {
	total := len(in)
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if limit == 0 || end > total {
		end = total
	}
	return in[offset:end], total, nil
}

// CapturingAuthorityClient 测试用 stub.
type CapturingAuthorityClient struct {
	mu       sync.Mutex
	Calls    []struct{ Authority, Payload string }
	FailNext int
}

// Submit .
func (c *CapturingAuthorityClient) Submit(ctx context.Context, authority, payload string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.FailNext > 0 {
		c.FailNext--
		return "", errors.New("content_safety: simulated authority 5xx")
	}
	c.Calls = append(c.Calls, struct{ Authority, Payload string }{authority, payload})
	return `{"ack":true}`, nil
}
