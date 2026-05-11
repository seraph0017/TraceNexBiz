package payment

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCreateTopup_Stub_HappyPath(t *testing.T) {
	t.Parallel()
	prov := NewHMACStubProvider("stub", []byte("secret"))
	svc := NewService(prov, NewMemoryIntentRepo())
	resp, err := svc.CreateTopup(context.Background(), TopupRequest{
		ActorType:  "customer",
		ActorID:    42,
		Amount:     10000,
		Channel:    "wechat_pay",
		OutTradeNo: "OUT-1",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if resp.PayURL == "" {
		t.Fatal("expected pay url")
	}
}

func TestCreateTopup_AmountInvalid(t *testing.T) {
	t.Parallel()
	prov := NewHMACStubProvider("stub", []byte("secret"))
	svc := NewService(prov, NewMemoryIntentRepo())
	_, err := svc.CreateTopup(context.Background(), TopupRequest{Amount: -1, OutTradeNo: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHandleCallback_VerifySignature(t *testing.T) {
	t.Parallel()
	prov := NewHMACStubProvider("stub", []byte("secret"))
	svc := NewService(prov, NewMemoryIntentRepo())
	_, _ = svc.CreateTopup(context.Background(), TopupRequest{Amount: 10000, OutTradeNo: "OUT-2", Channel: "wechat_pay"})

	good := CallbackPayload{
		OutTradeNo: "OUT-2",
		Status:     "success",
		Amount:     10000,
		PaidAt:     time.Now(),
	}
	good.Signature = prov.SignCallback(good.OutTradeNo, good.Status, good.Amount)

	if err := svc.HandleCallback(context.Background(), good); err != nil {
		t.Fatalf("verify good: %v", err)
	}
	// replay (idempotent)
	if err := svc.HandleCallback(context.Background(), good); err != nil {
		t.Fatalf("replay should be idempotent: %v", err)
	}

	bad := good
	bad.Signature = "deadbeef"
	if err := svc.HandleCallback(context.Background(), bad); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected invalid signature, got %v", err)
	}
}

func TestHandleCallback_AmountMismatch(t *testing.T) {
	t.Parallel()
	prov := NewHMACStubProvider("stub", []byte("k"))
	svc := NewService(prov, NewMemoryIntentRepo())
	_, _ = svc.CreateTopup(context.Background(), TopupRequest{Amount: 100, OutTradeNo: "M-1", Channel: "alipay"})

	cb := CallbackPayload{OutTradeNo: "M-1", Status: "success", Amount: 200}
	cb.Signature = prov.SignCallback(cb.OutTradeNo, cb.Status, cb.Amount)
	err := svc.HandleCallback(context.Background(), cb)
	if !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("expected amount mismatch, got %v", err)
	}
}

func TestReconcileDay_ListsRecordedIntents(t *testing.T) {
	t.Parallel()
	prov := NewHMACStubProvider("stub", []byte("k"))
	svc := NewService(prov, NewMemoryIntentRepo())
	for i := 0; i < 3; i++ {
		_, _ = svc.CreateTopup(context.Background(), TopupRequest{
			Amount: 100, OutTradeNo: "R-" + time.Now().Format("150405.000000000") + "-" + itoa(i),
			Channel: "alipay",
		})
	}
	entries, err := svc.ReconcileDay(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 3 {
		t.Fatalf("expected >=3 entries, got %d", len(entries))
	}
}

func itoa(i int) string { return string(rune('0' + i)) }
