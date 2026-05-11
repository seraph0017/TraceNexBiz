// internal/service/settlement/runner.go — settlement_run 月度批次驱动入口（W1b 提供契约骨架）.
//
// 性能预算（per backend §17）：
//   - 月结一次性聚合 ≤ 50 万 partner 行 → 每批 1k partner、P95 ≤ 30s/batch、总 P95 ≤ 30min
//   - freshness gate 由 outbox.FreshnessGate 提供，阈值默认 5min（biz_setting 可调）
//
// W1c 实现 RunMonthly 主体（leader 选举 / 心跳 / partition）；本文件只确定接口 + 不变量.
package settlement

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// 错误.
var (
	ErrPeriodInvalid = errors.New("settlement: invalid period")
)

// FreshnessChecker 由 outbox 包提供（避免循环 import；service 层装配时注入）.
type FreshnessChecker interface {
	FreshnessGate(ctx context.Context) (lag time.Duration, fresh bool, err error)
}

// AggregateLoader 拉某 period 内某 partner 的 revenue 聚合.
type AggregateLoader interface {
	Load(ctx context.Context, partnerID int64, period Period) (Aggregate, error)
	ListPartnerIDs(ctx context.Context, period Period, limit, offset int) ([]int64, error)
}

// Writer 落 settlement / settlement_item.
type Writer interface {
	UpsertSettlement(ctx context.Context, s domain.Settlement) (domain.Settlement, error)
	UpsertItem(ctx context.Context, item domain.SettlementItem) error
}

// Period 结算周期：月度（PRD §7.5）；timezone 默认 Asia/Shanghai.
type Period struct {
	Label    string // YYYY-MM
	Start    time.Time
	End      time.Time
	Timezone string
}

// NewMonthlyPeriod 构造月度区间（含起，不含止）.
func NewMonthlyPeriod(year int, month time.Month, tzName string) (Period, error) {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return Period{}, fmt.Errorf("%w: tz=%s", ErrPeriodInvalid, tzName)
	}
	start := time.Date(year, month, 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, 0)
	return Period{
		Label:    fmt.Sprintf("%04d-%02d", year, int(month)),
		Start:    start,
		End:      end,
		Timezone: tzName,
	}, nil
}

// Runner 编排月结流程；不持有 *gorm.DB（依赖注入抽象接口便于测试）.
type Runner struct {
	loader  AggregateLoader
	writer  Writer
	gate    FreshnessChecker
	batch   int
}

// NewRunner 构造.
func NewRunner(loader AggregateLoader, writer Writer, gate FreshnessChecker) *Runner {
	return &Runner{loader: loader, writer: writer, gate: gate, batch: 1000}
}

// RunMonthly 单 period 主循环.
//
// 流程：
//  1. 创建/取得 settlement（status=draft）
//  2. partition iterate partner_id：聚合 → ComputeItem → UpsertItem
//  3. freshness gate
//  4. Transition → locked
//
// W1c 主导落 leader 选举 + heartbeat；本骨架保证状态机正确即可.
func (r *Runner) RunMonthly(ctx context.Context, period Period, draft domain.Settlement) (domain.Settlement, error) {
	now := time.Now().UTC()
	state := draft
	if state.Status == "" {
		state.Status = string(StatusDraft)
	}

	offset := 0
	for {
		ids, err := r.loader.ListPartnerIDs(ctx, period, r.batch, offset)
		if err != nil {
			return state, fmt.Errorf("settlement: list partners: %w", err)
		}
		if len(ids) == 0 {
			break
		}
		for _, pid := range ids {
			agg, err := r.loader.Load(ctx, pid, period)
			if err != nil {
				return state, fmt.Errorf("settlement: load partner %d: %w", pid, err)
			}
			draft := ComputeItem(agg)
			item := domain.SettlementItem{
				SettlementID: state.ID,
				PartnerID:    draft.PartnerID,
				Revenue:      draft.Revenue,
				Cost:         draft.Cost,
				PlatformFee:  draft.PlatformFee,
				WithheldTax:  draft.WithheldTax,
				Payout:       draft.Payout,
				Status:       "pending",
				CreatedAt:    now,
				UpdatedAt:    now,
			}
			if err := r.writer.UpsertItem(ctx, item); err != nil {
				return state, fmt.Errorf("settlement: upsert item p=%d: %w", pid, err)
			}
		}
		offset += len(ids)
	}

	// 进入 generated.
	gen, err := Transition(state, StatusGenerated, now)
	if err != nil {
		return state, err
	}
	state, err = r.writer.UpsertSettlement(ctx, gen)
	if err != nil {
		return state, err
	}

	// freshness gate.
	if r.gate != nil {
		_, fresh, gateErr := r.gate.FreshnessGate(ctx)
		if gateErr != nil || !fresh {
			fail, _ := Transition(state, StatusGateFailed, time.Now().UTC())
			state, _ = r.writer.UpsertSettlement(ctx, fail)
			return state, fmt.Errorf("settlement: gate: %w", gateErr)
		}
	}

	locked, err := Transition(state, StatusLocked, time.Now().UTC())
	if err != nil {
		return state, err
	}
	return r.writer.UpsertSettlement(ctx, locked)
}
