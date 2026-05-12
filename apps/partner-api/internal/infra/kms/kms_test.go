// Fix-C item 10 round-trip tests：Encrypt → Decrypt → ScheduleKeyDeletion → Decrypt fails。
package kms

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestStubEncryptDecryptRoundtrip(t *testing.T) {
	s := NewStub()
	ct, kid, err := s.Encrypt(context.Background(), "audit:payload", []byte("hello"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if kid == "" {
		t.Fatal("empty kid")
	}
	pt, err := s.Decrypt(context.Background(), kid, ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(pt) != "hello" {
		t.Fatalf("got %q want hello", pt)
	}
}

func TestStubScheduleDeletionAccepts(t *testing.T) {
	s := NewStub()
	if err := s.ScheduleKeyDeletion(context.Background(), "kid", 30); err != nil {
		t.Fatalf("ScheduleKeyDeletion: %v", err)
	}
	if err := s.ScheduleKeyDeletion(context.Background(), "kid", 5); !errors.Is(err, ErrInvalidScheduleDays) {
		t.Fatalf("expected ErrInvalidScheduleDays got %v", err)
	}
	if err := s.ScheduleKeyDeletion(context.Background(), "kid", 366); !errors.Is(err, ErrInvalidScheduleDays) {
		t.Fatalf("expected ErrInvalidScheduleDays got %v", err)
	}
}

func TestLocalKMSEncryptDecryptRoundtrip(t *testing.T) {
	if err := os.Setenv("KMS_LOCAL_DEV", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Unsetenv("KMS_LOCAL_DEV") })

	s := NewLocalKMS()
	ct, kid, err := s.Encrypt(context.Background(), "kyc:legal_id", []byte("张三 110101199001011234"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	pt, err := s.Decrypt(context.Background(), kid, ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(pt) != "张三 110101199001011234" {
		t.Fatalf("roundtrip mismatch: %q", pt)
	}
	// schedule deletion → next decrypt fails
	if err := s.ScheduleKeyDeletion(context.Background(), kid, 30); err != nil {
		t.Fatalf("ScheduleKeyDeletion: %v", err)
	}
	if _, err := s.Decrypt(context.Background(), kid, ct); !errors.Is(err, ErrKeyDeleted) {
		t.Fatalf("after schedule, expected ErrKeyDeleted, got %v", err)
	}
	// cancel restores
	if err := s.CancelKeyDeletion(context.Background(), kid); err != nil {
		t.Fatalf("CancelKeyDeletion: %v", err)
	}
	if _, err := s.Decrypt(context.Background(), kid, ct); err != nil {
		t.Fatalf("after cancel: Decrypt: %v", err)
	}
}

func TestAliyunKMSRoundtripWithoutCreds(t *testing.T) {
	// 无凭据时 Encrypt 应返回 error（fail-closed）。
	s := NewAliyunKMS("ep", "kek", "cn", "", "")
	if _, _, err := s.Encrypt(context.Background(), "audit:payload", []byte("x")); err == nil {
		t.Fatal("expected error without creds")
	}
}

func TestAliyunKMSWithFakeCredsRoundtrip(t *testing.T) {
	s := NewAliyunKMS("ep", "kek", "cn", "ak", "as")
	ct, kid, err := s.Encrypt(context.Background(), "idem:response", []byte("payload"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	pt, err := s.Decrypt(context.Background(), kid, ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(pt) != "payload" {
		t.Fatalf("got %q", pt)
	}
	if err := s.ScheduleKeyDeletion(context.Background(), kid, 30); err != nil {
		t.Fatalf("ScheduleKeyDeletion: %v", err)
	}
	if _, err := s.Decrypt(context.Background(), kid, ct); !errors.Is(err, ErrKeyDeleted) {
		t.Fatalf("expected ErrKeyDeleted, got %v", err)
	}
}
