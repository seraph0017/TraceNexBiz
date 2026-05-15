package outbox

import (
	"context"
	"testing"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/audit"
)

type fakeWalletOpener struct{ partnerID int64 }

func (f *fakeWalletOpener) OpenWallet(_ context.Context, partnerID int64) error {
	f.partnerID = partnerID
	return nil
}

type fakeTopupFunder struct {
	sagaID   string
	fyUserID int64
	amount   int64
	traceID  string
}

func (f *fakeTopupFunder) Fund(_ context.Context, sagaID string, fyUserID, amount int64, traceID string) error {
	f.sagaID, f.fyUserID, f.amount, f.traceID = sagaID, fyUserID, amount, traceID
	return nil
}

type fakeRefundRunner struct{ req RefundRequest }

func (f *fakeRefundRunner) Run(_ context.Context, req RefundRequest) error {
	f.req = req
	return nil
}

type fakePartnerSuspender struct {
	partnerID int64
	reason    string
}

func (f *fakePartnerSuspender) Suspend(_ context.Context, partnerID int64, reason string) error {
	f.partnerID, f.reason = partnerID, reason
	return nil
}

type fakeAuditWriter struct{ row audit.UnsealedRow }

func (f *fakeAuditWriter) EnqueueUnsealed(_ context.Context, row audit.UnsealedRow) error {
	f.row = row
	return nil
}

func TestRegisterCoreHandlersDispatchesToDeps(t *testing.T) {
	fc := &fakeMNSClient{}
	c, _ := NewMNSConsumer(fc, MNSConsumerOptions{QueueName: "q", NoopOnUnknown: false})
	wallet := &fakeWalletOpener{}
	topup := &fakeTopupFunder{}
	refund := &fakeRefundRunner{}
	suspend := &fakePartnerSuspender{}
	auditw := &fakeAuditWriter{}
	RegisterCoreHandlers(c, HandlerDeps{
		WalletOpener:     wallet,
		TopupFunder:      topup,
		RefundRunner:     refund,
		PartnerSuspender: suspend,
		AuditWriter:      auditw,
	})

	cases := []*MNSMessage{
		makeMsg("m1", map[string]string{"event_type": EventPartnerKYCApproved}, `{"partner_id":11}`, 1),
		makeMsg("m2", map[string]string{"event_type": EventCustomerPaymentSucceeded, "trace_id": "tr-pay"}, `{"saga_id":"s1","fy_user_id":42,"amount":100}`, 1),
		makeMsg("m3", map[string]string{"event_type": EventCustomerRefundRequested, "trace_id": "tr-ref"}, `{"saga_id":"s2","partner_id":11,"customer_id":22,"amount":33,"operator_id":44}`, 1),
		makeMsg("m4", map[string]string{"event_type": EventPartnerSuspended}, `{"partner_id":55,"reason":"risk"}`, 1),
		makeMsg("m5", map[string]string{"event_type": EventStaffAuditEvent, "trace_id": "tr-audit"}, `{"actor_type":"staff","actor_id":7,"action":"approve","target_type":"partner","target_id":8}`, 1),
	}
	for _, msg := range cases {
		if err := c.handleOne(context.Background(), msg); err != nil {
			t.Fatalf("handleOne %s: %v", msg.MessageID, err)
		}
	}
	if wallet.partnerID != 11 {
		t.Fatalf("wallet partnerID=%d", wallet.partnerID)
	}
	if topup.sagaID != "s1" || topup.fyUserID != 42 || topup.amount != 100 || topup.traceID != "tr-pay" {
		t.Fatalf("topup=%+v", topup)
	}
	if refund.req.SagaID != "s2" || refund.req.TraceID != "tr-ref" || refund.req.Amount != 33 {
		t.Fatalf("refund=%+v", refund.req)
	}
	if suspend.partnerID != 55 || suspend.reason != "risk" {
		t.Fatalf("suspend=%+v", suspend)
	}
	if auditw.row.ActorType != "staff" || auditw.row.ActorID != 7 || auditw.row.TraceID != "tr-audit" {
		t.Fatalf("audit=%+v", auditw.row)
	}
	if len(fc.deleted) != 5 {
		t.Fatalf("expected 5 acked messages, got %d", len(fc.deleted))
	}
}

func TestCoreHandlerMissingDepLeavesMessage(t *testing.T) {
	fc := &fakeMNSClient{}
	c, _ := NewMNSConsumer(fc, MNSConsumerOptions{QueueName: "q", NoopOnUnknown: false})
	RegisterCoreHandlers(c, HandlerDeps{})
	msg := makeMsg("m1", map[string]string{"event_type": EventPartnerKYCApproved}, `{"partner_id":11}`, 1)
	if err := c.handleOne(context.Background(), msg); err != nil {
		t.Fatalf("handleOne should leave message without surfacing error: %v", err)
	}
	if len(fc.deleted) != 0 {
		t.Fatalf("missing dep must not ack; deleted=%v", fc.deleted)
	}
}
