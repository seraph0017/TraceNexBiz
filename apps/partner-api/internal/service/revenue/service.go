// Package revenue 是 partner-api 收益记录服务（PRD §8.7 + integration §3）。
//
// 与 outbox.Consumer 的 Sink 实现配合：consumer 拉到 Event → service.RecordFromOutbox 写 revenue_log。
//
// 关键 invariant：
//   1. 写入只在 saga step 成功 / outbox event ack 之前执行
//   2. UNIQUE (fy_api_log_id, occurrence) 防重；冲突视为幂等成功
//   3. cost / net / partner_fee / platform_fee 字段语义在 PRD §13 Q10 之前用 placeholder（cost = grossCost；net = gross - cost；partnerFee/platformFee = 0）.
//      ADR 标注：[ADR-NEW] revenue_log fee 字段语义（待 Q10 决议）.
//   4. trace_id 透传：来自 Event.TraceID，便于审计串接。
package revenue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/outbox"
)

// 错误.
var (
	ErrPartnerNotFound  = errors.New("revenue: partner not found for fy_user")
	ErrCustomerNotFound = errors.New("revenue: customer not found for fy_user")
)

// Resolver 解析 fy_user_id → (partnerID, customerID, appliedRuleID)。
//
// W1a 提供具体实现（查 partner / customer / pricing rule resolver）。
type Resolver interface {
	ResolveByFyUserID(ctx context.Context, fyUserID int64, occurredAt time.Time) (partnerID, customerID, ruleID int64, err error)
}

// Repository revenue_log 写入接口（W1a GORM 落地）.
type Repository interface {
	// Insert 幂等写入；UNIQUE(fy_api_log_id, occurrence) 冲突 → return false, nil.
	Insert(ctx context.Context, row *domain.RevenueLog) (inserted bool, err error)
}

// Service 暴露给 outbox.Consumer.Sink 用 + handler.
type Service struct {
	repo     Repository
	resolver Resolver
}

// NewService 构造.
func NewService(repo Repository, resolver Resolver) *Service {
	return &Service{repo: repo, resolver: resolver}
}

// WriteRevenue 实现 outbox.Sink 接口.
func (s *Service) WriteRevenue(ctx context.Context, ev outbox.Event) (bool, error) {
	if s.repo == nil || s.resolver == nil {
		return false, fmt.Errorf("revenue: not initialized")
	}
	partnerID, customerID, ruleID, err := s.resolver.ResolveByFyUserID(ctx, ev.UserID, ev.OccurredAt)
	if err != nil {
		return false, fmt.Errorf("revenue: resolve fy_user=%d: %w", ev.UserID, err)
	}
	row := buildRow(ev, partnerID, customerID, ruleID)
	return s.repo.Insert(ctx, &row)
}

// buildRow 不变量：纯函数，便于单测覆盖；不做参数校验（caller 已 validate）.
//
// 字段语义：
//   - GrossAmount = ev.GrossAmount（fy 计费总额）
//   - CostAmount  = ev.CostAmount（上游成本）
//   - NetRevenue  = max(0, gross - cost)
//   - PartnerFee  / PlatformFee 留 ADR-Q10 决议（暂 0；不入此 row，由 settlement 阶段重算）
func buildRow(ev outbox.Event, partnerID, customerID, ruleID int64) domain.RevenueLog {
	gross := ev.GrossAmount
	cost := ev.CostAmount
	net := gross - cost
	if net < 0 {
		// 上游成本 > 计费总额（极端 cache miss / refund coupon），按 0 入账，
		// 由 settlement freshness gate + 对账 raise alert（不阻塞消费）.
		net = 0
	}
	return domain.RevenueLog{
		PartnerID:     partnerID,
		CustomerID:    customerID,
		FyAPILogID:    ev.FyLogID,
		Occurrence:    ev.Occurrence,
		GrossAmount:   gross,
		CostAmount:    cost,
		NetRevenue:    net,
		AppliedRuleID: ruleID,
		OccurredAt:    ev.OccurredAt,
		TraceID:       ev.TraceID,
		CreatedAt:     time.Now().UTC(),
	}
}
