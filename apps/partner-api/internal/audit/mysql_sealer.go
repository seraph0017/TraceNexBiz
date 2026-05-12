// internal/audit/mysql_sealer.go — GORM-backed Store + BufferedSink → unsealed enqueue 桥（Fix-B' part 4 CRIT-B6）.
//
// 角色：
//
//   - GormStore 实现 audit.Store；audit-sealer cron 用它把 audit_log_unsealed 哈希链化进 audit_log.
//   - EnqueueSink 接 middleware.AuditSink 接口，把 AuditEntry 落 audit_log_unsealed（同样异步 buffered）.
//
// 哈希链公式：
//
//	hash = SHA-256( seq || prev_hash || canonical(payload) )
//
// 其中 canonical(payload) 由 sealer.go 的 computeHash 处理（含 actor/route/method/status/request_hash/payload_json 全部字段）。
// 第一条 sealed log 的 prev_hash = GENESIS.
//
// Flush policy（middleware → unsealed）:
//   - 缓冲 channel size = 1024（与原 BufferedSink 对齐）
//   - 内部 worker 每条立即 INSERT；INSERT 失败 retry 最多 3 次（exponential 100ms/200ms/400ms）
//   - 3 次失败 → drop + AuditDropsTotal++ + ERROR log（不阻塞主链）
//
// 与原 BufferedSink 的区别：原版只 log.Info()，本实现真正写库。
package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// auditLogUnsealedRow 对应 audit_log_unsealed 表（006 + 014 迁移）.
type auditLogUnsealedRow struct {
	ID               int64     `gorm:"primaryKey;column:id"`
	ActorType        string    `gorm:"column:actor_type;size:16"`
	ActorID          int64     `gorm:"column:actor_id"`
	Action           string    `gorm:"column:action;size:64"`
	TargetType       string    `gorm:"column:target_type;size:32"`
	TargetID         int64     `gorm:"column:target_id"`
	TargetKey        string    `gorm:"column:target_key;size:128"`
	DiffRedacted     *string   `gorm:"column:diff_redacted;type:text"`
	DiffPIIID        *int64    `gorm:"column:diff_pii_id"`
	IPAddress        string    `gorm:"column:ip_address;size:64"`
	UserAgent        string    `gorm:"column:user_agent;size:512"`
	TraceID          string    `gorm:"column:trace_id;size:64"`
	SecondApproverID *int64    `gorm:"column:second_approver_id"`
	OccurredAt       time.Time `gorm:"column:occurred_at"`
	Route            string    `gorm:"column:route;size:255"`
	Method           string    `gorm:"column:method;size:8"`
	Status           int       `gorm:"column:status"`
	RequestHash      string    `gorm:"column:request_hash;size:64"`
	PayloadJSON      *string   `gorm:"column:payload_json;type:text"`
}

func (auditLogUnsealedRow) TableName() string { return "audit_log_unsealed" }

// auditLogRow 对应 audit_log 表（006 + 014 迁移）.
type auditLogRow struct {
	ID               int64     `gorm:"primaryKey;column:id"`
	ActorType        string    `gorm:"column:actor_type;size:16"`
	ActorID          int64     `gorm:"column:actor_id"`
	Action           string    `gorm:"column:action;size:64"`
	TargetType       string    `gorm:"column:target_type;size:32"`
	TargetID         int64     `gorm:"column:target_id"`
	TargetKey        string    `gorm:"column:target_key;size:128"`
	DiffRedacted     *string   `gorm:"column:diff_redacted;type:text"`
	DiffPIIID        *int64    `gorm:"column:diff_pii_id"`
	IPAddress        string    `gorm:"column:ip_address;size:64"`
	UserAgent        string    `gorm:"column:user_agent;size:512"`
	TraceID          string    `gorm:"column:trace_id;size:64"`
	SecondApproverID *int64    `gorm:"column:second_approver_id"`
	OccurredAt       time.Time `gorm:"column:occurred_at"`
	PrevHash         string    `gorm:"column:prev_hash;size:64"`
	SelfHash         string    `gorm:"column:self_hash;size:64"`
	SealedAt         time.Time `gorm:"column:sealed_at"`
	Route            string    `gorm:"column:route;size:255"`
	Method           string    `gorm:"column:method;size:8"`
	Status           int       `gorm:"column:status"`
	RequestHash      string    `gorm:"column:request_hash;size:64"`
	PayloadJSON      *string   `gorm:"column:payload_json;type:text"`
}

func (auditLogRow) TableName() string { return "audit_log" }

// GormStore audit.Store 的 GORM 实现.
type GormStore struct {
	db *gorm.DB
}

// NewGormStore 构造.
func NewGormStore(db *gorm.DB) *GormStore { return &GormStore{db: db} }

// FetchUnsealedBatch ORDER BY id ASC LIMIT n.
func (s *GormStore) FetchUnsealedBatch(ctx context.Context, limit int) ([]UnsealedRow, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("audit gorm store: nil db")
	}
	if limit <= 0 {
		limit = 200
	}
	var rows []auditLogUnsealedRow
	if err := s.db.WithContext(ctx).Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]UnsealedRow, len(rows))
	for i, r := range rows {
		out[i] = unsealedRowFromGorm(r)
	}
	return out, nil
}

// LastSealedHash 取 audit_log 表最后一行的 self_hash；空表返回 GENESIS.
func (s *GormStore) LastSealedHash(ctx context.Context) (string, error) {
	if s == nil || s.db == nil {
		return "", errors.New("audit gorm store: nil db")
	}
	var row auditLogRow
	err := s.db.WithContext(ctx).Order("id DESC").Limit(1).Find(&row).Error
	if err != nil {
		return "", err
	}
	if row.SelfHash == "" {
		return GenesisPrevHash, nil
	}
	return row.SelfHash, nil
}

// AppendSealed batch INSERT；single tx；DeleteUnsealed 由调用方分开调（与现有 Sealer 行为一致）.
func (s *GormStore) AppendSealed(ctx context.Context, rows []SealedRow) error {
	if s == nil || s.db == nil {
		return errors.New("audit gorm store: nil db")
	}
	if len(rows) == 0 {
		return nil
	}
	gormRows := make([]auditLogRow, len(rows))
	for i, r := range rows {
		gormRows[i] = sealedRowToGorm(r)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.CreateInBatches(&gormRows, 100).Error
	})
}

// DeleteUnsealed 删 audit_log_unsealed 中 id ∈ ids 的行.
func (s *GormStore) DeleteUnsealed(ctx context.Context, ids []int64) error {
	if s == nil || s.db == nil {
		return errors.New("audit gorm store: nil db")
	}
	if len(ids) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Where("id IN ?", ids).Delete(&auditLogUnsealedRow{}).Error
}

// IterateSealed 全表 / since 增量扫；分页避免大表 OOM.
func (s *GormStore) IterateSealed(ctx context.Context, since int64, fn func(SealedRow) error) error {
	if s == nil || s.db == nil {
		return errors.New("audit gorm store: nil db")
	}
	const page = 500
	last := since
	for {
		var rows []auditLogRow
		if err := s.db.WithContext(ctx).
			Where("id > ?", last).
			Order("id ASC").
			Limit(page).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, r := range rows {
			if err := fn(sealedRowFromGorm(r)); err != nil {
				return err
			}
		}
		last = rows[len(rows)-1].ID
		if len(rows) < page {
			return nil
		}
	}
}

// EnqueueUnsealed app 侧调用入口（middleware.Audit sink 通过 EnqueueSink 调）.
func (s *GormStore) EnqueueUnsealed(ctx context.Context, r UnsealedRow) error {
	if s == nil || s.db == nil {
		return errors.New("audit gorm store: nil db")
	}
	row := unsealedRowToGorm(r)
	return s.db.WithContext(ctx).Create(&row).Error
}

// ---- conversions ----

func unsealedRowFromGorm(r auditLogUnsealedRow) UnsealedRow {
	return UnsealedRow{
		ID:               r.ID,
		ActorType:        r.ActorType,
		ActorID:          r.ActorID,
		Action:           r.Action,
		TargetType:       r.TargetType,
		TargetID:         r.TargetID,
		TargetKey:        r.TargetKey,
		DiffRedacted:     derefStr(r.DiffRedacted),
		DiffPIIID:        r.DiffPIIID,
		IPAddress:        r.IPAddress,
		UserAgent:        r.UserAgent,
		TraceID:          r.TraceID,
		SecondApproverID: r.SecondApproverID,
		OccurredAt:       r.OccurredAt,
		Route:            r.Route,
		Method:           r.Method,
		Status:           r.Status,
		RequestHash:      r.RequestHash,
		PayloadJSON:      r.PayloadJSON,
	}
}

func unsealedRowToGorm(r UnsealedRow) auditLogUnsealedRow {
	if r.OccurredAt.IsZero() {
		r.OccurredAt = time.Now().UTC()
	}
	row := auditLogUnsealedRow{
		ID:               r.ID,
		ActorType:        r.ActorType,
		ActorID:          r.ActorID,
		Action:           r.Action,
		TargetType:       r.TargetType,
		TargetID:         r.TargetID,
		TargetKey:        r.TargetKey,
		DiffPIIID:        r.DiffPIIID,
		IPAddress:        r.IPAddress,
		UserAgent:        r.UserAgent,
		TraceID:          r.TraceID,
		SecondApproverID: r.SecondApproverID,
		OccurredAt:       r.OccurredAt,
		Route:            r.Route,
		Method:           r.Method,
		Status:           r.Status,
		RequestHash:      r.RequestHash,
		PayloadJSON:      r.PayloadJSON,
	}
	if r.DiffRedacted != "" {
		v := r.DiffRedacted
		row.DiffRedacted = &v
	}
	return row
}

func sealedRowToGorm(r SealedRow) auditLogRow {
	if r.SealedAt.IsZero() {
		r.SealedAt = time.Now().UTC()
	}
	row := auditLogRow{
		ID:               r.ID,
		ActorType:        r.ActorType,
		ActorID:          r.ActorID,
		Action:           r.Action,
		TargetType:       r.TargetType,
		TargetID:         r.TargetID,
		TargetKey:        r.TargetKey,
		DiffPIIID:        r.DiffPIIID,
		IPAddress:        r.IPAddress,
		UserAgent:        r.UserAgent,
		TraceID:          r.TraceID,
		SecondApproverID: r.SecondApproverID,
		OccurredAt:       r.OccurredAt,
		PrevHash:         r.PrevHash,
		SelfHash:         r.SelfHash,
		SealedAt:         r.SealedAt,
		Route:            r.Route,
		Method:           r.Method,
		Status:           r.Status,
		RequestHash:      r.RequestHash,
		PayloadJSON:      r.PayloadJSON,
	}
	if r.DiffRedacted != "" {
		v := r.DiffRedacted
		row.DiffRedacted = &v
	}
	return row
}

func sealedRowFromGorm(r auditLogRow) SealedRow {
	return SealedRow{
		UnsealedRow: UnsealedRow{
			ID:               r.ID,
			ActorType:        r.ActorType,
			ActorID:          r.ActorID,
			Action:           r.Action,
			TargetType:       r.TargetType,
			TargetID:         r.TargetID,
			TargetKey:        r.TargetKey,
			DiffRedacted:     derefStr(r.DiffRedacted),
			DiffPIIID:        r.DiffPIIID,
			IPAddress:        r.IPAddress,
			UserAgent:        r.UserAgent,
			TraceID:          r.TraceID,
			SecondApproverID: r.SecondApproverID,
			OccurredAt:       r.OccurredAt,
			Route:            r.Route,
			Method:           r.Method,
			Status:           r.Status,
			RequestHash:      r.RequestHash,
			PayloadJSON:      r.PayloadJSON,
		},
		PrevHash: r.PrevHash,
		SelfHash: r.SelfHash,
		SealedAt: r.SealedAt,
	}
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ---- EnqueueSink: middleware.AuditSink → audit_log_unsealed ----

// AuditDropsTotal exported metric — middleware sealer drop counter.
//
// Aliased to middleware.AuditDropsTotal in main.go wiring (avoids import cycle).
var AuditDropsTotal atomic.Int64

// EnqueueSink 接 middleware.AuditEntry → unsealed.
//
// 异步：内部 buffered channel + 1 worker；满了 → drop + counter.
// flushOne 失败 retry 3 次后 drop.
type EnqueueSink struct {
	store *GormStore
	ch    chan UnsealedRow
	done  chan struct{}
	once  sync.Once
}

// NewEnqueueSink 构造；buffer ≤ 0 默认 1024.
func NewEnqueueSink(store *GormStore, buffer int) *EnqueueSink {
	if buffer <= 0 {
		buffer = 1024
	}
	s := &EnqueueSink{
		store: store,
		ch:    make(chan UnsealedRow, buffer),
		done:  make(chan struct{}),
	}
	go s.run()
	return s
}

// Send 非阻塞投递；满 → drop + counter.
func (s *EnqueueSink) Send(r UnsealedRow) {
	if s == nil {
		return
	}
	select {
	case s.ch <- r:
	default:
		AuditDropsTotal.Add(1)
	}
}

// Close 关闭 channel；worker 处理完剩余条目后退出.
func (s *EnqueueSink) Close() {
	s.once.Do(func() { close(s.ch) })
}

// Drained 测试用：等 worker 退出.
func (s *EnqueueSink) Drained() <-chan struct{} { return s.done }

func (s *EnqueueSink) run() {
	defer close(s.done)
	ctx := context.Background()
	for r := range s.ch {
		s.flushOne(ctx, r)
	}
}

func (s *EnqueueSink) flushOne(ctx context.Context, r UnsealedRow) {
	backoff := 100 * time.Millisecond
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		err := s.store.EnqueueUnsealed(ctx, r)
		if err == nil {
			return
		}
		lastErr = err
		log.Warn().Err(err).Int("attempt", attempt).
			Str("route", r.Route).
			Str("trace_id", r.TraceID).
			Msg("audit enqueue retry")
		time.Sleep(backoff)
		backoff *= 2
	}
	AuditDropsTotal.Add(1)
	log.Error().Err(lastErr).
		Str("route", r.Route).
		Str("trace_id", r.TraceID).
		Msg("audit enqueue dropped after retries")
}

// HashRequestBody helper：computes SHA-256 of body; "" for nil/empty.
//
// 仅用于 middleware Audit 把 RequestHash 算好后传给 sink.Send.
func HashRequestBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	h := sha256.Sum256(body)
	return strings.ToLower(hex.EncodeToString(h[:]))
}
