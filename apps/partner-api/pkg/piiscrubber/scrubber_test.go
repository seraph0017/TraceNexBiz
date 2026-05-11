package piiscrubber

import (
	"strings"
	"testing"
)

func TestRedactPhone(t *testing.T) {
	got := Redact("contact 13812345678 today")
	if !strings.Contains(got, "***-PHONE-***") {
		t.Fatalf("phone not redacted: %q", got)
	}
}

func TestRedactEmail(t *testing.T) {
	got := Redact("alice@example.com signed up")
	if !strings.Contains(got, "***-EMAIL-***") {
		t.Fatalf("email not redacted: %q", got)
	}
}

func TestRedactIDCard(t *testing.T) {
	got := Redact("身份证 11010119900307123X")
	if !strings.Contains(got, "***-IDCARD-***") {
		t.Fatalf("idcard not redacted: %q", got)
	}
}
