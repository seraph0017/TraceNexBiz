// Package audit 哈希链 sealer + verify（backend §10 重写）.
//
// 角色：
//
//	Sealer  单 leader 异步批处理：拉 audit_log_unsealed → 计算哈希链 → 写 audit_log → 删 unsealed
//	Verify  CLI 入口（cmd/audit-verify）：扫 audit_log 全表重算 self_hash 校验链
//
// 不变量：
//   - audit_log.id == audit_log_unsealed.id（1:1，sealer 拷贝）
//   - audit_log[i].prev_hash == audit_log[i-1].self_hash（i=1 时 prev = "GENESIS"）
//   - audit_log[i].self_hash == sha256(canonicalize(row || prev_hash))
//
// fail-closed：unsealed 不可消费时，业务可继续 INSERT unsealed（不阻塞主链路）；
// 但启动时若 sealer 无法获得 leader lease，cmd/audit-sealer 主进程退出 1（K8s 拉起其他副本）.
package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ErrChainBroken verify CLI 跨 sealer 边界发现哈希链断裂时的兜底错误.
var ErrChainBroken = errors.New("audit: hash chain broken")

// GenesisPrevHash 第一条 sealed log 的 prev_hash.
const GenesisPrevHash = "GENESIS"

// UnsealedRow app INSERT 队列行（service 层投影）.
type UnsealedRow struct {
	ID               int64
	ActorType        string
	ActorID          int64
	Action           string
	TargetType       string
	TargetID         int64
	TargetKey        string
	DiffRedacted     string
	DiffPIIID        *int64
	IPAddress        string
	UserAgent        string
	TraceID          string
	SecondApproverID *int64
	OccurredAt       time.Time

	// Fix-B' part 4 CRIT-B6: middleware AuditEntry → unsealed 映射字段.
	Route        string
	Method       string
	Status       int
	RequestHash  string  // SHA-256 of canonical PII-scrubbed body
	PayloadJSON  *string // NULL for GET
}

// SealedRow audit_log 哈希链最终行.
type SealedRow struct {
	UnsealedRow
	PrevHash string
	SelfHash string
	SealedAt time.Time
}

// Store sealer / verifier 持久化抽象.
type Store interface {
	// FetchUnsealedBatch 拉 ORDER BY id ASC LIMIT n.
	FetchUnsealedBatch(ctx context.Context, limit int) ([]UnsealedRow, error)
	// LastSealedHash 取 audit_log 表最后一行的 self_hash（不存在返 GenesisPrevHash）.
	LastSealedHash(ctx context.Context) (string, error)
	// AppendSealed 将 batch 写入 audit_log（原子；service 层保证 leader 单写）.
	AppendSealed(ctx context.Context, rows []SealedRow) error
	// DeleteUnsealed 删 audit_log_unsealed 中 id <= maxID 的所有行.
	DeleteUnsealed(ctx context.Context, ids []int64) error
	// IterateSealed 全表扫（verify CLI 用），传入 callback；callback 返 error 时终止.
	IterateSealed(ctx context.Context, since int64, fn func(SealedRow) error) error
}

// LeaderLock K8s Lease / Redis SETNX 包装；W1c 仅交付接口 + AlwaysLeader stub（cron 单 replica）.
type LeaderLock interface {
	Acquire(ctx context.Context) (bool, error)
	Renew(ctx context.Context) error
	Release(ctx context.Context) error
}

// AlwaysLeader stub（dev / single-replica）.
type AlwaysLeader struct{}

// Acquire .
func (AlwaysLeader) Acquire(ctx context.Context) (bool, error) { return true, nil }

// Renew .
func (AlwaysLeader) Renew(ctx context.Context) error { return nil }

// Release .
func (AlwaysLeader) Release(ctx context.Context) error { return nil }

// Sealer 单 leader 哈希链消费者.
type Sealer struct {
	store     Store
	leader    LeaderLock
	batchSize int
	tick      time.Duration
	clock     func() time.Time
}

// NewSealer 构造.
func NewSealer(s Store, l LeaderLock) *Sealer {
	return &Sealer{store: s, leader: l, batchSize: 200, tick: 200 * time.Millisecond, clock: time.Now}
}

// SetBatch 测试用 — 调批大小.
func (s *Sealer) SetBatch(n int) { s.batchSize = n }

// SetTick 测试用 — 调 tick 间隔.
func (s *Sealer) SetTick(d time.Duration) { s.tick = d }

// Run 启动 200ms tick 循环；until ctx done.
func (s *Sealer) Run(ctx context.Context) error {
	if ok, err := s.leader.Acquire(ctx); err != nil {
		return fmt.Errorf("audit: leader acquire: %w", err)
	} else if !ok {
		return errors.New("audit: not leader, refusing to run")
	}
	defer func() { _ = s.leader.Release(context.Background()) }()
	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := s.SealOnce(ctx); err != nil {
				return err
			}
		}
	}
}

// SealOnce 单批处理（测试入口）.
func (s *Sealer) SealOnce(ctx context.Context) (int, error) {
	rows, err := s.store.FetchUnsealedBatch(ctx, s.batchSize)
	if err != nil {
		return 0, fmt.Errorf("audit: fetch unsealed: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	prevHash, err := s.store.LastSealedHash(ctx)
	if err != nil {
		return 0, fmt.Errorf("audit: last sealed hash: %w", err)
	}
	if prevHash == "" {
		prevHash = GenesisPrevHash
	}
	now := s.clock()
	sealed := make([]SealedRow, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		sr := SealedRow{UnsealedRow: r, PrevHash: prevHash, SealedAt: now}
		sr.SelfHash = computeHash(sr)
		sealed = append(sealed, sr)
		ids = append(ids, r.ID)
		prevHash = sr.SelfHash
	}
	if err := s.store.AppendSealed(ctx, sealed); err != nil {
		return 0, fmt.Errorf("audit: append sealed: %w", err)
	}
	if err := s.store.DeleteUnsealed(ctx, ids); err != nil {
		return 0, fmt.Errorf("audit: delete unsealed: %w", err)
	}
	return len(sealed), nil
}

// Verify 全表 / 增量校验哈希链.
func Verify(ctx context.Context, store Store, since int64) error {
	prev := GenesisPrevHash
	first := true
	return store.IterateSealed(ctx, since, func(row SealedRow) error {
		if first && since > 0 {
			// 增量模式：信任锚为本行 prev_hash；跳过链头校验
			prev = row.PrevHash
		}
		first = false
		if row.PrevHash != prev {
			return fmt.Errorf("%w: id=%d prev mismatch (have=%s want=%s)", ErrChainBroken, row.ID, row.PrevHash, prev)
		}
		expected := computeHash(row)
		if expected != row.SelfHash {
			return fmt.Errorf("%w: id=%d self mismatch", ErrChainBroken, row.ID)
		}
		prev = row.SelfHash
		return nil
	})
}

// computeHash canonicalize 字段并 sha256.
//
// Fix-B' part 4 CRIT-B6: 增加 Route/Method/Status/RequestHash/PayloadJSON 进 canonical form。
// 旧字段顺序保持不变 — 新字段附在末尾，零值时仍能产生确定性结果。
func computeHash(r SealedRow) string {
	h := sha256.New()
	approver := int64(0)
	if r.SecondApproverID != nil {
		approver = *r.SecondApproverID
	}
	piiID := int64(0)
	if r.DiffPIIID != nil {
		piiID = *r.DiffPIIID
	}
	payload := ""
	if r.PayloadJSON != nil {
		payload = *r.PayloadJSON
	}
	fmt.Fprintf(h, "%d|%s|%d|%s|%s|%d|%s|%s|%d|%s|%s|%s|%d|%d|%s|%s|%s|%d|%s|%s",
		r.ID, r.ActorType, r.ActorID, r.Action, r.TargetType, r.TargetID, r.TargetKey,
		r.DiffRedacted, piiID, r.IPAddress, r.UserAgent, r.TraceID, approver,
		r.OccurredAt.UnixNano(), r.PrevHash,
		r.Route, r.Method, r.Status, r.RequestHash, payload)
	return hex.EncodeToString(h.Sum(nil))
}

// MemoryStore 内存实现（测试 + cmd/audit-verify dry-run）.
type MemoryStore struct {
	mu       sync.Mutex
	unsealed []UnsealedRow
	sealed   []SealedRow
	nextID   int64
}

// NewMemoryStore .
func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

// EnqueueUnsealed app 侧调用（service 层在 saga / wallet / kyc 等关键 verb 处入队）.
func (s *MemoryStore) EnqueueUnsealed(r UnsealedRow) UnsealedRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	r.ID = s.nextID
	s.unsealed = append(s.unsealed, r)
	return r
}

// FetchUnsealedBatch .
func (s *MemoryStore) FetchUnsealedBatch(ctx context.Context, limit int) ([]UnsealedRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sort.Slice(s.unsealed, func(i, j int) bool { return s.unsealed[i].ID < s.unsealed[j].ID })
	if len(s.unsealed) <= limit {
		out := make([]UnsealedRow, len(s.unsealed))
		copy(out, s.unsealed)
		return out, nil
	}
	out := make([]UnsealedRow, limit)
	copy(out, s.unsealed[:limit])
	return out, nil
}

// LastSealedHash .
func (s *MemoryStore) LastSealedHash(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sealed) == 0 {
		return GenesisPrevHash, nil
	}
	return s.sealed[len(s.sealed)-1].SelfHash, nil
}

// AppendSealed .
func (s *MemoryStore) AppendSealed(ctx context.Context, rows []SealedRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sealed = append(s.sealed, rows...)
	return nil
}

// DeleteUnsealed .
func (s *MemoryStore) DeleteUnsealed(ctx context.Context, ids []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idset := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		idset[id] = struct{}{}
	}
	out := s.unsealed[:0]
	for _, r := range s.unsealed {
		if _, drop := idset[r.ID]; !drop {
			out = append(out, r)
		}
	}
	s.unsealed = out
	return nil
}

// IterateSealed .
func (s *MemoryStore) IterateSealed(ctx context.Context, since int64, fn func(SealedRow) error) error {
	s.mu.Lock()
	rows := make([]SealedRow, len(s.sealed))
	copy(rows, s.sealed)
	s.mu.Unlock()
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	for _, r := range rows {
		if r.ID <= since {
			continue
		}
		if err := fn(r); err != nil {
			return err
		}
	}
	return nil
}

// Tamper 测试用：篡改 sealed 行某字段（验证 verify 能抓出来）.
func (s *MemoryStore) Tamper(idx int, mutate func(*SealedRow)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mutate(&s.sealed[idx])
}

// SealedCount .
func (s *MemoryStore) SealedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sealed)
}

// UnsealedCount .
func (s *MemoryStore) UnsealedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.unsealed)
}
