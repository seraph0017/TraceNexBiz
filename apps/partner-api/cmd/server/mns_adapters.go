package main

import (
	"context"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository/mysql"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/partner"
)

type walletOpenAdapter struct {
	repo *mysql.WalletRepository
}

func (a walletOpenAdapter) OpenWallet(ctx context.Context, partnerID int64) error {
	return a.repo.EnsureWallet(ctx, partnerID)
}

type partnerSuspendAdapter struct {
	svc *partner.Service
}

func (a partnerSuspendAdapter) Suspend(ctx context.Context, partnerID int64, reason string) error {
	_, err := a.svc.Suspend(ctx, partnerID, reason)
	return err
}
