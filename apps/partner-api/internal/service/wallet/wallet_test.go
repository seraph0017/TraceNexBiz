package wallet

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

func TestGet_AvailableMinusHeld(t *testing.T) {
	repo := NewMemoryRepo()
	repo.SeedWallet(domain.PartnerWallet{PartnerID: 1, Balance: 10000})
	repo.SeedHold(domain.WalletHold{PartnerID: 1, Amount: 3000, Status: "held"})
	repo.SeedHold(domain.WalletHold{PartnerID: 1, Amount: 200, Status: "released"})
	svc := NewService(repo)
	snap, err := svc.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if snap.HeldTotal != 3000 || snap.Available != 7000 || snap.OpenHoldsCount != 1 {
		t.Fatalf("snapshot wrong: %+v", snap)
	}
}

func TestGet_NotFound(t *testing.T) {
	svc := NewService(NewMemoryRepo())
	if _, err := svc.Get(context.Background(), 99); !errors.Is(err, ErrWalletNotFound) {
		t.Fatalf("expected not found got %v", err)
	}
}

func TestListLogs_FilterAndLimit(t *testing.T) {
	repo := NewMemoryRepo()
	for i := int64(0); i < 5; i++ {
		repo.SeedLog(domain.PartnerWalletLog{
			PartnerID: 1, Type: domain.WalletLogRevenueAccrual, Amount: 100, CreatedAt: time.Now(),
		})
		repo.SeedLog(domain.PartnerWalletLog{
			PartnerID: 1, Type: domain.WalletLogAllocateToCustomer, Amount: -100, CreatedAt: time.Now(),
		})
	}
	svc := NewService(repo)
	got, err := svc.ListLogs(context.Background(), 1, LogFilter{
		Type: domain.WalletLogRevenueAccrual, Limit: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 got %d", len(got))
	}
}

func TestStubAllocator_NotImplemented(t *testing.T) {
	if _, err := (StubAllocator{}).Allocate(context.Background(), AllocateInput{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented got %v", err)
	}
}
