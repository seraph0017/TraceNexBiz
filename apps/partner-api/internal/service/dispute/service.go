// Package dispute 是账单争议服务（场景 K + PRD §M5-09）.
//
// 业务流：
//
//	1. submit         客户/渠道商提交 → status=submitted
//	2. review         运营审核 → status=reviewing
//	3. accepted/rejected
//	4. accepted → 联动 refund saga（W1b saga_refund）
//
// 终审 endpoint 暴露给 admin（W1c）.
//
// 状态机不可逆：rejected / refunded 为终态.
package dispute

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// 错误.
var (
	ErrInvalidTransition = errors.New("dispute: invalid status transition")
	ErrNotFound          = errors.New("dispute: not found")
	ErrEmptyReason       = errors.New("dispute: reason required")
)

// Status billing_dispute.status.
type Status string

const (
	StatusSubmitted Status = "submitted"
	StatusReviewing Status = "reviewing"
	StatusAccepted  Status = "accepted"
	StatusRejected  Status = "rejected"
	StatusRefunded  Status = "refunded"
)

// CanTransition 状态机.
func CanTransition(from, to Status) bool {
	switch from {
	case StatusSubmitted:
		return to == StatusReviewing
	case StatusReviewing:
		return to == StatusAccepted || to == StatusRejected
	case StatusAccepted:
		return to == StatusRefunded
	}
	return false
}

// Dispute domain entity（W0 表已存在 billing_dispute；W1b 提供领域结构）.
type Dispute struct {
	ID                int64
	OpenerType        string
	OpenerID          int64
	RevenueLogID      int64
	Amount            int64
	Reason            string
	Status            Status
	ReviewerID        *int64
	ReviewedAt        *time.Time
	RefundSagaID      string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// SubmitRequest 客户/渠道商提交参数.
type SubmitRequest struct {
	OpenerType   string // partner / customer
	OpenerID     int64
	RevenueLogID int64
	Amount       int64
	Reason       string
}

// Validate.
func (r SubmitRequest) Validate() error {
	if r.OpenerID == 0 || r.RevenueLogID == 0 || r.Amount <= 0 {
		return fmt.Errorf("dispute: opener/revenue_log/amount required")
	}
	if r.Reason == "" {
		return ErrEmptyReason
	}
	if r.OpenerType != "partner" && r.OpenerType != "customer" {
		return fmt.Errorf("dispute: invalid opener_type=%s", r.OpenerType)
	}
	return nil
}

// Repository 持久化.
type Repository interface {
	Create(ctx context.Context, d Dispute) (Dispute, error)
	FindByID(ctx context.Context, id int64) (*Dispute, error)
	UpdateStatus(ctx context.Context, id int64, from, to Status, reviewerID int64, refundSagaID string, now time.Time) error
}

// RefundLauncher 联动 refund saga（W1b saga_refund.Service.Run）.
type RefundLauncher interface {
	Launch(ctx context.Context, sagaID string, revenueLogID, amount int64, reason string, operatorID int64) error
}

// SagaIDFactory 用于生成 UUIDv7.
type SagaIDFactory interface {
	New() (string, error)
}

// Service 编排.
type Service struct {
	repo    Repository
	refund  RefundLauncher
	sagaIDs SagaIDFactory
}

// NewService 构造.
func NewService(r Repository, refund RefundLauncher, ids SagaIDFactory) *Service {
	return &Service{repo: r, refund: refund, sagaIDs: ids}
}

// Submit 客户/渠道商提交争议.
func (s *Service) Submit(ctx context.Context, req SubmitRequest) (Dispute, error) {
	if err := req.Validate(); err != nil {
		return Dispute{}, err
	}
	now := time.Now().UTC()
	d := Dispute{
		OpenerType:   req.OpenerType,
		OpenerID:     req.OpenerID,
		RevenueLogID: req.RevenueLogID,
		Amount:       req.Amount,
		Reason:       req.Reason,
		Status:       StatusSubmitted,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	return s.repo.Create(ctx, d)
}

// StartReview admin 操作（dual-control 不要求；终审才要）.
func (s *Service) StartReview(ctx context.Context, id int64, reviewerID int64) error {
	now := time.Now().UTC()
	if err := s.repo.UpdateStatus(ctx, id, StatusSubmitted, StatusReviewing, reviewerID, "", now); err != nil {
		return fmt.Errorf("dispute: start review: %w", err)
	}
	return nil
}

// FinalizeAccept admin 终审通过 → 联动退款 saga.
//
// 接口暴露给 W1c admin handler；调用方负责 audit_log + dual-control 校验.
func (s *Service) FinalizeAccept(ctx context.Context, id, reviewerID, operatorID int64) error {
	d, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if d == nil {
		return ErrNotFound
	}
	if !CanTransition(d.Status, StatusAccepted) {
		return fmt.Errorf("from=%s: %w", d.Status, ErrInvalidTransition)
	}
	now := time.Now().UTC()
	if err := s.repo.UpdateStatus(ctx, id, StatusReviewing, StatusAccepted, reviewerID, "", now); err != nil {
		return err
	}
	sagaID, err := s.sagaIDs.New()
	if err != nil {
		return fmt.Errorf("dispute: gen saga id: %w", err)
	}
	if err := s.refund.Launch(ctx, sagaID, d.RevenueLogID, d.Amount, d.Reason, operatorID); err != nil {
		return fmt.Errorf("dispute: launch refund: %w", err)
	}
	if err := s.repo.UpdateStatus(ctx, id, StatusAccepted, StatusRefunded, reviewerID, sagaID, time.Now().UTC()); err != nil {
		return err
	}
	return nil
}

// FinalizeReject admin 终审驳回.
func (s *Service) FinalizeReject(ctx context.Context, id, reviewerID int64) error {
	now := time.Now().UTC()
	if err := s.repo.UpdateStatus(ctx, id, StatusReviewing, StatusRejected, reviewerID, "", now); err != nil {
		return fmt.Errorf("dispute: reject: %w", err)
	}
	return nil
}
