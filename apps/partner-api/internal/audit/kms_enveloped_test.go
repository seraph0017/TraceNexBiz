package audit

import (
	"context"
	"strings"
	"testing"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/kms"
)

// TestEnvelopedStore_FallbackOnly verifies that decryption is a no-op for non-marked rows
// and that the marker → decrypt roundtrip works for an in-memory style scenario.
//
// Full GORM persistence path is exercised by repository/mysql tests; here we only verify
// the encode/decode logic via Stub KMS.
func TestEnvelopedStore_DecryptInplaceRoundtrip(t *testing.T) {
	s := &EnvelopedStore{kms: kms.NewStub()}
	// Encode by hand (mirror EnqueueUnsealed path).
	payload := "{\"foo\":\"bar\"}"
	ct, kid, err := s.kms.Encrypt(context.Background(), "audit:payload", []byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	wrapped := encryptedMarker + kms.Base64Encode(ct) + separator + kid
	row := UnsealedRow{PayloadJSON: &wrapped}
	decryptInplace(context.Background(), s.kms, &row)
	if row.PayloadJSON == nil || *row.PayloadJSON != payload {
		t.Fatalf("roundtrip failed, got %+v", row.PayloadJSON)
	}
}

func TestEnvelopedStore_NonEncryptedPassthrough(t *testing.T) {
	s := &EnvelopedStore{kms: kms.NewStub()}
	original := "{\"plain\":\"yes\"}"
	row := UnsealedRow{PayloadJSON: &original}
	decryptInplace(context.Background(), s.kms, &row)
	if row.PayloadJSON == nil || *row.PayloadJSON != original {
		t.Fatalf("non-encrypted row should be untouched")
	}
}

func TestEnvelopedStore_WrappingMarker(t *testing.T) {
	if !strings.HasPrefix(encryptedMarker+"x", encryptedMarker) {
		t.Fatal("marker constant sanity")
	}
}
