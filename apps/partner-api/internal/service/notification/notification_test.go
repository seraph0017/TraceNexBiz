package notification

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSend_RendersTemplateAndWritesOutbox(t *testing.T) {
	t.Parallel()
	repo := NewMemoryRepo()
	ts := NewMapTemplateStore()
	ts.Put("invoice.issued", "email", "Hello {{.Name}}, invoice {{.Serial}} ready.")
	ch := &CapturingChannel{ChannelName: "email"}
	svc := NewService(repo, ts, ch)

	id, err := svc.Send(context.Background(), SendInput{
		EventCode: "invoice.issued", Channel: "email", Recipient: "user@example.com", RefID: "FP1",
		Vars: map[string]any{"Name": "Alice", "Serial": "FP1"},
	})
	if err != nil || id == 0 {
		t.Fatalf("Send: %v / %d", err, id)
	}
	delivered, err := svc.Dispatch(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if delivered != 1 || len(ch.Sent) != 1 {
		t.Fatalf("delivered=%d sent=%d", delivered, len(ch.Sent))
	}
	if !strings.Contains(ch.Sent[0].Payload, "Alice") {
		t.Fatalf("payload missing var: %q", ch.Sent[0].Payload)
	}
}

func TestSend_InvalidChannel(t *testing.T) {
	t.Parallel()
	svc := NewService(NewMemoryRepo(), NewMapTemplateStore())
	_, err := svc.Send(context.Background(), SendInput{EventCode: "x", Channel: "carrier_pigeon", Recipient: "y"})
	if !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("got %v", err)
	}
}

func TestSend_DedupOnUnique(t *testing.T) {
	t.Parallel()
	repo := NewMemoryRepo()
	ts := NewMapTemplateStore()
	ts.Put("topup.success", "sms", "ok {{.Amount}}")
	svc := NewService(repo, ts, &CapturingChannel{ChannelName: "sms"})

	id1, err := svc.Send(context.Background(), SendInput{EventCode: "topup.success", Channel: "sms", Recipient: "13800000000", RefID: "T1", Vars: map[string]any{"Amount": 100}})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := svc.Send(context.Background(), SendInput{EventCode: "topup.success", Channel: "sms", Recipient: "13800000000", RefID: "T1", Vars: map[string]any{"Amount": 100}})
	if err != nil {
		t.Fatal(err)
	}
	if id1 == 0 || id2 != 0 {
		t.Fatalf("expected first non-zero second 0; got %d / %d", id1, id2)
	}
}

func TestDispatch_RetryAndDeadLetter(t *testing.T) {
	t.Parallel()
	repo := NewMemoryRepo()
	ts := NewMapTemplateStore()
	ts.Put("e", "email", "x")
	ch := &CapturingChannel{ChannelName: "email", FailNext: true}
	svc := NewService(repo, ts, ch)
	_, _ = svc.Send(context.Background(), SendInput{EventCode: "e", Channel: "email", Recipient: "a@b", RefID: "1"})
	delivered, err := svc.Dispatch(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if delivered != 0 {
		t.Fatalf("expected 0 delivered, got %d", delivered)
	}
	// retry should now succeed
	delivered, err = svc.Dispatch(context.Background(), 10)
	if err != nil || delivered != 1 {
		t.Fatalf("retry: delivered=%d err=%v", delivered, err)
	}
}
