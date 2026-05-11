package invitation

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGenerate_HappyPermanent(t *testing.T) {
	svc := NewService(NewMemoryRepo())
	code, err := svc.Generate(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(code) < 16 {
		t.Fatalf("code too short: %s", code)
	}
}

func TestGenerate_OneTime_ConsumeMarksUsedUp(t *testing.T) {
	repo := NewMemoryRepo()
	svc := NewService(repo)
	code, err := svc.GenerateWith(context.Background(), GenerateInput{
		PartnerID: 1, Type: TypeOneTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Consume(context.Background(), code); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	c, _ := repo.FindByCode(context.Background(), code)
	if c.Status != StatusUsedUp {
		t.Fatalf("expected used_up got %s", c.Status)
	}
	if _, err := svc.Consume(context.Background(), code); !errors.Is(err, ErrCodeInactive) {
		t.Fatalf("second consume: expected ErrCodeInactive got %v", err)
	}
}

func TestResolve_Expired(t *testing.T) {
	repo := NewMemoryRepo()
	now := time.Date(2026, 5, 11, 8, 0, 0, 0, time.UTC)
	svc := NewService(repo).WithClock(func() time.Time { return now })
	yesterday := now.Add(-24 * time.Hour)
	code, err := svc.GenerateWith(context.Background(), GenerateInput{
		PartnerID: 1, Type: TypePermanent, ExpiresAt: &yesterday,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resolve(context.Background(), code); !errors.Is(err, ErrCodeExpired) {
		t.Fatalf("expected expired got %v", err)
	}
}

func TestRevoke(t *testing.T) {
	repo := NewMemoryRepo()
	svc := NewService(repo)
	code, _ := svc.Generate(context.Background(), 1)
	if _, err := svc.Revoke(context.Background(), code); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resolve(context.Background(), code); !errors.Is(err, ErrCodeInactive) {
		t.Fatalf("expected inactive after revoke got %v", err)
	}
}

func TestGenerate_UniqueAfterCollision(t *testing.T) {
	repo := NewMemoryRepo()
	step := 0
	svc := NewService(repo).WithRng(func(b []byte) error {
		// 第一次和第二次返回相同字节，模拟碰撞
		v := byte(0)
		if step >= 2 {
			v = byte(step)
		}
		for i := range b {
			b[i] = v
		}
		step++
		return nil
	})
	if _, err := svc.Generate(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Generate(context.Background(), 1); err != nil {
		t.Fatalf("second gen should retry & succeed: %v", err)
	}
}
