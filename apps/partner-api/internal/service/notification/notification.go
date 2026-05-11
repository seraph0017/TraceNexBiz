// Package notification 通知系统（PRD §7.11 / §8.18）.
//
// 三个通道：email / sms / inapp（webhook 留给 Phase 2A）.
// outbox 模型：service 层只管 INSERT notification_outbox；dispatcher 由 cron 5s tick 拉取（cmd/notify-dispatcher 后续 W1c-2）.
//
// UNIQUE(event_code, recipient, ref_id) 由 008_*.up.sql 保证，service 层重复 Send 走幂等命中.
//
// 模板系统：
//   - 模板字符串存 biz_setting，key 形如 `notify.template.<event_code>.<channel>`
//   - 简单 {{.Var}} 占位（text/template）
//   - service 层渲染后写 outbox.payload；channel adapter 各自再读 outbox.payload + recipient 投递
package notification

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"text/template"
	"time"
)

// ErrInvalidChannel .
var ErrInvalidChannel = errors.New("notification: invalid channel")

// ErrTemplateNotFound .
var ErrTemplateNotFound = errors.New("notification: template not found")

// ErrAlreadyQueued 已存在相同 (event_code, recipient, ref_id) 的 outbox 行（幂等命中）.
var ErrAlreadyQueued = errors.New("notification: already queued")

var validChannels = map[string]struct{}{"email": {}, "sms": {}, "inapp": {}, "webhook": {}}

// Channel 通道 adapter 接口.
type Channel interface {
	Name() string // email / sms / inapp / webhook
	Send(ctx context.Context, recipient, payload string) error
}

// Outbox row.
type Outbox struct {
	ID           int64
	Recipient    string
	Channel      string
	EventCode    string
	RefID        string
	Payload      string
	Status       string // pending / sent / failed / dead_letter
	RetryCount   int
	LastError    string
	DispatchedAt *time.Time
	TraceID      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TemplateStore 抽象 biz_setting 模板查询.
type TemplateStore interface {
	GetTemplate(ctx context.Context, eventCode, channel string) (string, error)
}

// Repo outbox 持久化.
type Repo interface {
	// InsertIfAbsent UNIQUE(event_code, recipient, ref_id) 命中返 ErrAlreadyQueued.
	InsertIfAbsent(ctx context.Context, o *Outbox) (int64, error)
	ClaimPending(ctx context.Context, n int) ([]Outbox, error)
	MarkSent(ctx context.Context, id int64, t time.Time) error
	MarkFailed(ctx context.Context, id int64, msg string, deadLetter bool) error
}

// Service 通知服务.
type Service struct {
	repo      Repo
	templates TemplateStore
	channels  map[string]Channel
	clock     func() time.Time
}

// NewService 构造.
func NewService(repo Repo, ts TemplateStore, channels ...Channel) *Service {
	cm := make(map[string]Channel, len(channels))
	for _, c := range channels {
		cm[c.Name()] = c
	}
	return &Service{repo: repo, templates: ts, channels: cm, clock: time.Now}
}

// SendInput .
type SendInput struct {
	EventCode string
	Channel   string
	Recipient string
	RefID     string
	TraceID   string
	Vars      map[string]any
}

// Send 渲染模板 + 写 outbox（幂等：UNIQUE 命中视为成功）.
func (s *Service) Send(ctx context.Context, in SendInput) (int64, error) {
	if _, ok := validChannels[in.Channel]; !ok {
		return 0, ErrInvalidChannel
	}
	if in.Recipient == "" || in.EventCode == "" {
		return 0, errors.New("notification: recipient + event_code required")
	}
	tmplStr, err := s.templates.GetTemplate(ctx, in.EventCode, in.Channel)
	if err != nil {
		return 0, fmt.Errorf("notification: get template: %w", err)
	}
	tmpl, err := template.New(in.EventCode).Parse(tmplStr)
	if err != nil {
		return 0, fmt.Errorf("notification: parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, in.Vars); err != nil {
		return 0, fmt.Errorf("notification: execute template: %w", err)
	}
	now := s.clock()
	row := &Outbox{
		Recipient: in.Recipient, Channel: in.Channel, EventCode: in.EventCode, RefID: in.RefID,
		Payload: buf.String(), Status: "pending", TraceID: in.TraceID, CreatedAt: now, UpdatedAt: now,
	}
	id, err := s.repo.InsertIfAbsent(ctx, row)
	if errors.Is(err, ErrAlreadyQueued) {
		return 0, nil
	}
	return id, err
}

// Dispatch 拉一批 pending → 调用 channel adapter → 标记 sent/failed.
//
// 该函数被 cron 5s tick 调；W1c 仅交付 service 函数，cmd 入口留给 W1a 起 cron.
func (s *Service) Dispatch(ctx context.Context, batch int) (int, error) {
	rows, err := s.repo.ClaimPending(ctx, batch)
	if err != nil {
		return 0, err
	}
	delivered := 0
	for _, row := range rows {
		ch, ok := s.channels[row.Channel]
		if !ok {
			_ = s.repo.MarkFailed(ctx, row.ID, "no channel adapter", row.RetryCount >= 5)
			continue
		}
		if err := ch.Send(ctx, row.Recipient, row.Payload); err != nil {
			_ = s.repo.MarkFailed(ctx, row.ID, err.Error(), row.RetryCount >= 5)
			continue
		}
		_ = s.repo.MarkSent(ctx, row.ID, s.clock())
		delivered++
	}
	return delivered, nil
}

// MemoryRepo 内存实现.
type MemoryRepo struct {
	mu    sync.Mutex
	rows  map[int64]*Outbox
	dedup map[string]int64 // event|recipient|ref → id
	next  int64
}

// NewMemoryRepo .
func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{rows: make(map[int64]*Outbox), dedup: make(map[string]int64)}
}

func dedupKey(o *Outbox) string { return o.EventCode + "|" + o.Recipient + "|" + o.RefID }

// InsertIfAbsent .
func (r *MemoryRepo) InsertIfAbsent(ctx context.Context, o *Outbox) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := dedupKey(o)
	if _, ok := r.dedup[k]; ok {
		return 0, ErrAlreadyQueued
	}
	r.next++
	o.ID = r.next
	cp := *o
	r.rows[o.ID] = &cp
	r.dedup[k] = o.ID
	return o.ID, nil
}

// ClaimPending FIFO.
func (r *MemoryRepo) ClaimPending(ctx context.Context, n int) ([]Outbox, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Outbox
	for _, row := range r.rows {
		if row.Status == "pending" {
			out = append(out, *row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > n {
		out = out[:n]
	}
	return out, nil
}

// MarkSent .
func (r *MemoryRepo) MarkSent(ctx context.Context, id int64, t time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[id]
	if !ok {
		return errors.New("notification: row not found")
	}
	row.Status = "sent"
	row.DispatchedAt = &t
	row.UpdatedAt = t
	return nil
}

// MarkFailed .
func (r *MemoryRepo) MarkFailed(ctx context.Context, id int64, msg string, deadLetter bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[id]
	if !ok {
		return errors.New("notification: row not found")
	}
	row.RetryCount++
	row.LastError = msg
	if deadLetter {
		row.Status = "dead_letter"
	} else {
		row.Status = "pending"
	}
	row.UpdatedAt = time.Now()
	return nil
}

// MapTemplateStore 内存模板（biz_setting stub）.
type MapTemplateStore struct {
	mu sync.Mutex
	m  map[string]string
}

// NewMapTemplateStore .
func NewMapTemplateStore() *MapTemplateStore { return &MapTemplateStore{m: make(map[string]string)} }

// Put .
func (s *MapTemplateStore) Put(eventCode, channel, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[eventCode+"|"+channel] = body
}

// GetTemplate .
func (s *MapTemplateStore) GetTemplate(ctx context.Context, eventCode, channel string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[eventCode+"|"+channel]
	if !ok {
		return "", ErrTemplateNotFound
	}
	return v, nil
}

// CapturingChannel 通道 stub（测试用，记录所有发出的消息）.
type CapturingChannel struct {
	ChannelName string
	mu          sync.Mutex
	Sent        []struct {
		Recipient string
		Payload   string
	}
	FailNext bool
}

// Name .
func (c *CapturingChannel) Name() string {
	if c.ChannelName == "" {
		return "email"
	}
	return c.ChannelName
}

// Send .
func (c *CapturingChannel) Send(ctx context.Context, recipient, payload string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.FailNext {
		c.FailNext = false
		return errors.New("notification: simulated send failure")
	}
	c.Sent = append(c.Sent, struct {
		Recipient string
		Payload   string
	}{recipient, payload})
	return nil
}
