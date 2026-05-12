package pii

import (
	"errors"
	"testing"
)

func TestBlindIndex_Stable(t *testing.T) {
	a, err := BlindIndex("62284801234567", []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := BlindIndex("62284801234567", []byte("key"))
	if a != b {
		t.Fatalf("nondeterministic: %s != %s", a, b)
	}
	if len(a) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(a))
	}
}

func TestBlindIndex_DifferentKey(t *testing.T) {
	a, _ := BlindIndex("acct", []byte("key1"))
	b, _ := BlindIndex("acct", []byte("key2"))
	if a == b {
		t.Fatal("expected different blind index with different key")
	}
}

func TestBlindIndex_MissingKey(t *testing.T) {
	if _, err := BlindIndex("x", nil); !errors.Is(err, ErrBlindIndexKeyMissing) {
		t.Fatalf("expected ErrBlindIndexKeyMissing got %v", err)
	}
}

func TestNormalizeBankAccount(t *testing.T) {
	if got := NormalizeBankAccount("6228 4801-23 45  67"); got != "62284801234567" {
		t.Fatalf("got %q", got)
	}
}
