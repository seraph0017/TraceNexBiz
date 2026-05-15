// 客户切换渠道商（场景 H）+ PIPL 右遗忘（场景 Q）。
//
// 拆出独立文件保持单文件 ≤ 400 行。
package customer

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/saga"
)

// TransferRequestInput 客户发起转挂请求（场景 H）。
type TransferRequestInput struct {
	CustomerID     int64
	FromPartnerID  int64
	ToPartnerID    int64
	InitiatorType  string // customer / staff
	InitiatorID    int64
	Reason         string
	IdempotencyKey string
	TraceID        string
}

// RequestTransfer 客户发起转挂（场景 H 起点）。状态机：pending_a → pending_b → pending_staff → completed。
//
// 本方法仅写 customer_partner_change_log；customer.partner_id 不变。
// 后续 partner A accept / partner B adopt / staff approve 由 W1c admin / W1g UI 调本服务的 推进方法。
func (s *Service) RequestTransfer(ctx context.Context, in TransferRequestInput) (*domain.CustomerPartnerChangeLog, error) {
	var out *domain.CustomerPartnerChangeLog
	if err := s.withMutationIdempotency(ctx, "customer", in.InitiatorID, in.IdempotencyKey, "POST /customer/transfer", in.TraceID, func() error {
		var err error
		out, err = s.requestTransfer(ctx, in)
		return err
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) requestTransfer(ctx context.Context, in TransferRequestInput) (*domain.CustomerPartnerChangeLog, error) {
	c, err := s.repo.FindByIDForPartner(ctx, in.FromPartnerID, in.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("customer: scoped find for transfer: %w", err)
	}
	if c == nil {
		return nil, ErrCustomerNotFound
	}
	if c.Status == StatusDeleted {
		return nil, ErrInvalidStatusForOp
	}
	from := c.PartnerID
	if from != nil && *from == in.ToPartnerID {
		return nil, ErrSelfTransferNotAllowed
	}
	now := s.clock()
	log := domain.CustomerPartnerChangeLog{
		CustomerID: c.ID, FromPartnerID: from, ToPartnerID: &in.ToPartnerID,
		InitiatorType: in.InitiatorType, InitiatorID: in.InitiatorID,
		Status: "pending_a", Reason: in.Reason, OccurredAt: now,
		OldGroup: c.GroupNameInFyAPI,
		NewGroup: fmt.Sprintf("partner_%d_default", in.ToPartnerID),
	}
	id, err := s.repo.InsertChangeLog(ctx, log)
	if err != nil {
		return nil, fmt.Errorf("customer: insert change_log: %w", err)
	}
	log.ID = id
	return &log, nil
}

// AdvanceTransferStage 推进转挂阶段（A accept / B adopt / staff approve）。
//
// 当 toStage='completed' 时同时更新 customer.partner_id 并调 fyapi.UpdateUserGroup。
func (s *Service) AdvanceTransferStage(ctx context.Context, logID int64, toStage string) (*domain.CustomerPartnerChangeLog, error) {
	if toStage == "" {
		return nil, errors.New("customer: stage required")
	}
	updated, err := s.repo.UpdateChangeLog(ctx, logID, func(l domain.CustomerPartnerChangeLog) domain.CustomerPartnerChangeLog {
		if l.Status == "completed" || l.Status == "rejected" {
			return l
		}
		l.Status = toStage
		return l
	})
	if err != nil {
		return nil, fmt.Errorf("customer: update change_log: %w", err)
	}
	if updated.Status != "completed" {
		return updated, nil
	}
	if updated.ToPartnerID == nil {
		return updated, errors.New("customer: change_log missing to_partner_id")
	}
	now := s.clock()
	to := *updated.ToPartnerID
	if _, err := s.repo.Update(ctx, updated.CustomerID, func(c domain.Customer) domain.Customer {
		c.PartnerID = &to
		c.GroupNameInFyAPI = updated.NewGroup
		c.Status = StatusActive
		c.TransferredAt = &now
		c.TransferredFrom = updated.FromPartnerID
		return c
	}); err != nil {
		return updated, fmt.Errorf("customer: apply transfer: %w", err)
	}
	c, err := s.repo.FindByIDForPartner(ctx, to, updated.CustomerID)
	if err != nil || c == nil {
		return updated, fmt.Errorf("customer: scoped reload after transfer: %w", err)
	}
	idem := fmt.Sprintf("customer-transfer-%d-%d", updated.ID, now.Unix())
	if err := s.fyapi.UpdateUserGroup(ctx, c.FyUserID, c.GroupNameInFyAPI, idem); err != nil {
		// 不回滚 customer：业务允许 saga retry；W1c notify ops。
		return updated, fmt.Errorf("customer: fyapi update group: %w", err)
	}
	return updated, nil
}

// EraseInput PIPL §47 右遗忘（场景 Q）。
type EraseInput struct {
	CustomerID     int64
	PartnerID      int64
	Reason         string
	OperatorID     int64
	IdempotencyKey string
	TraceID        string
}

// SubmitErase 删除客户：customer.deleted_at + status='deleted' + Fy-api `/user/erase`。
//
// 不删 audit_log（哈希链）；不删 revenue_log（仅替换 fy_user_id 为 -1，由 W1c PIPL 服务实现）。
func (s *Service) SubmitErase(ctx context.Context, in EraseInput) error {
	return s.withMutationIdempotency(ctx, "customer", in.CustomerID, in.IdempotencyKey, "POST /customer/erase", in.TraceID, func() error {
		return s.submitErase(ctx, in)
	})
}

func (s *Service) submitErase(ctx context.Context, in EraseInput) error {
	c, err := s.repo.FindByIDForPartner(ctx, in.PartnerID, in.CustomerID)
	if err != nil {
		return fmt.Errorf("customer: scoped find for erase: %w", err)
	}
	if c == nil {
		return ErrCustomerNotFound
	}
	if c.Status == StatusDeleted {
		return nil // 幂等
	}
	now := s.clock()
	if _, err := s.repo.Update(ctx, in.CustomerID, func(x domain.Customer) domain.Customer {
		x.Status = StatusDeleted
		return x
	}); err != nil {
		return fmt.Errorf("customer: mark deleted: %w", err)
	}
	idem := fmt.Sprintf("customer-erase-%d-%d", in.CustomerID, now.Unix())
	if err := s.fyapi.EraseUser(ctx, c.FyUserID, idem); err != nil {
		return fmt.Errorf("customer: fyapi erase: %w", err)
	}
	return nil
}

func (s *Service) withMutationIdempotency(ctx context.Context, actorType string, actorID int64, key, endpoint, traceID string, fn func() error) error {
	if key == "" || s.idemDB == nil || s.idemInsert == nil {
		if fn == nil {
			return nil
		}
		return fn()
	}
	now := s.clock()
	rec := saga.NewIdempotencyRecord(actorType, actorID, key, endpoint, traceID, `{"success":true}`, 200, now)
	err := saga.WithIdempotency(ctx, s.idemDB, s.idemInsert, rec, func(_ *gorm.DB) error {
		if fn == nil {
			return nil
		}
		return fn()
	})
	if errors.Is(err, repository.ErrDuplicateKey) {
		return saga.ErrDuplicateIdempotency
	}
	return err
}
