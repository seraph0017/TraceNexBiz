package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/audit"
)

const (
	EventPartnerKYCApproved       = "partner.kyc.approved"
	EventCustomerPaymentSucceeded = "customer.payment.succeeded"
	EventCustomerRefundRequested  = "customer.refund.requested"
	EventPartnerSuspended         = "partner.suspended"
	EventStaffAuditEvent          = "staff.audit.event"
)

// WalletOpener handles partner wallet bootstrap after KYC approval.
type WalletOpener interface {
	OpenWallet(ctx context.Context, partnerID int64) error
}

// TopupFunder advances a paid topup saga to Fy-api funding.
type TopupFunder interface {
	Fund(ctx context.Context, sagaID string, fyUserID, amount int64, traceID string) error
}

// RefundRunner starts a customer refund saga.
type RefundRunner interface {
	Run(ctx context.Context, req RefundRequest) error
}

// PartnerSuspender isolates or suspends a partner tree.
type PartnerSuspender interface {
	Suspend(ctx context.Context, partnerID int64, reason string) error
}

// AuditWriter writes local audit events.
type AuditWriter interface {
	EnqueueUnsealed(ctx context.Context, row audit.UnsealedRow) error
}

// HandlerDeps are optional; missing deps make registered handlers fail-loud so the
// message redelivers and eventually DLQs instead of being ack-dropped.
type HandlerDeps struct {
	WalletOpener     WalletOpener
	TopupFunder      TopupFunder
	RefundRunner     RefundRunner
	PartnerSuspender PartnerSuspender
	AuditWriter      AuditWriter
}

// RefundRequest is the MNS-facing refund request shape.
type RefundRequest struct {
	SagaID            string `json:"saga_id"`
	OriginalRevenueID int64  `json:"original_revenue_id"`
	PartnerID         int64  `json:"partner_id"`
	CustomerID        int64  `json:"customer_id"`
	Amount            int64  `json:"amount"`
	Reason            string `json:"reason"`
	OperatorID        int64  `json:"operator_id"`
	TraceID           string `json:"trace_id"`
}

// RegisterCoreHandlers registers the five Round-3 core MNS handlers.
func RegisterCoreHandlers(consumer *MNSConsumer, deps HandlerDeps) {
	if consumer == nil {
		return
	}
	consumer.Register(EventPartnerKYCApproved, func(ctx context.Context, msg *MNSMessage) error {
		var p struct {
			PartnerID int64 `json:"partner_id"`
		}
		if err := decodeBody(msg, &p); err != nil {
			return err
		}
		if deps.WalletOpener == nil {
			return missingHandlerDep(EventPartnerKYCApproved)
		}
		return deps.WalletOpener.OpenWallet(ctx, p.PartnerID)
	})
	consumer.Register(EventCustomerPaymentSucceeded, func(ctx context.Context, msg *MNSMessage) error {
		var p struct {
			SagaID   string `json:"saga_id"`
			FyUserID int64  `json:"fy_user_id"`
			Amount   int64  `json:"amount"`
			TraceID  string `json:"trace_id"`
		}
		if err := decodeBody(msg, &p); err != nil {
			return err
		}
		if p.TraceID == "" {
			p.TraceID = msg.Attrs["trace_id"]
		}
		if deps.TopupFunder == nil {
			return missingHandlerDep(EventCustomerPaymentSucceeded)
		}
		return deps.TopupFunder.Fund(ctx, p.SagaID, p.FyUserID, p.Amount, p.TraceID)
	})
	consumer.Register(EventCustomerRefundRequested, func(ctx context.Context, msg *MNSMessage) error {
		var req RefundRequest
		if err := decodeBody(msg, &req); err != nil {
			return err
		}
		if req.TraceID == "" {
			req.TraceID = msg.Attrs["trace_id"]
		}
		if deps.RefundRunner == nil {
			return missingHandlerDep(EventCustomerRefundRequested)
		}
		return deps.RefundRunner.Run(ctx, req)
	})
	consumer.Register(EventPartnerSuspended, func(ctx context.Context, msg *MNSMessage) error {
		var p struct {
			PartnerID int64  `json:"partner_id"`
			Reason    string `json:"reason"`
		}
		if err := decodeBody(msg, &p); err != nil {
			return err
		}
		if deps.PartnerSuspender == nil {
			return missingHandlerDep(EventPartnerSuspended)
		}
		return deps.PartnerSuspender.Suspend(ctx, p.PartnerID, p.Reason)
	})
	consumer.Register(EventStaffAuditEvent, func(ctx context.Context, msg *MNSMessage) error {
		var p struct {
			ActorType  string `json:"actor_type"`
			ActorID    int64  `json:"actor_id"`
			Action     string `json:"action"`
			TargetType string `json:"target_type"`
			TargetID   int64  `json:"target_id"`
			TraceID    string `json:"trace_id"`
		}
		if err := decodeBody(msg, &p); err != nil {
			return err
		}
		if p.TraceID == "" {
			p.TraceID = msg.Attrs["trace_id"]
		}
		if deps.AuditWriter == nil {
			return missingHandlerDep(EventStaffAuditEvent)
		}
		return deps.AuditWriter.EnqueueUnsealed(ctx, audit.UnsealedRow{
			ActorType:  p.ActorType,
			ActorID:    p.ActorID,
			Action:     p.Action,
			TargetType: p.TargetType,
			TargetID:   p.TargetID,
			TraceID:    p.TraceID,
			OccurredAt: time.Now().UTC(),
		})
	})
}

func decodeBody(msg *MNSMessage, out any) error {
	if msg == nil {
		return errors.New("mns handler: nil message")
	}
	if len(msg.Body) == 0 {
		return errors.New("mns handler: empty body")
	}
	if err := json.Unmarshal(msg.Body, out); err != nil {
		return fmt.Errorf("mns handler: decode body: %w", err)
	}
	return nil
}

func missingHandlerDep(eventType string) error {
	err := fmt.Errorf("mns handler dependency missing for %s", eventType)
	log.Error().Err(err).Str("event_type", eventType).Msg("mns_handler_dependency_missing")
	return err
}
