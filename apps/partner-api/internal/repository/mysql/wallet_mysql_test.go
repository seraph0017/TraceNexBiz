// wallet_mysql_test.go — read views + atomic AdjustBalance（HIGH-B7）+ EnsureWallet。
package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	walletsvc "github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/wallet"
)

func seedWallet(t *testing.T, r *WalletRepository, partnerID, balance int64) {
	t.Helper()
	require.NoError(t, r.EnsureWallet(context.Background(), partnerID))
	if balance > 0 {
		rows, err := r.AdjustBalance(context.Background(), partnerID, balance)
		require.NoError(t, err)
		require.Equal(t, int64(1), rows)
	}
}

func seedHold(t *testing.T, r *WalletRepository, walletID, partnerID, amount int64, sagaID, status string) {
	t.Helper()
	require.NoError(t, r.db.Create(&walletHoldRow{
		WalletID: walletID, PartnerID: partnerID, Amount: amount,
		SagaID: sagaID, Status: status,
		HeldAt: time.Now().UTC(),
	}).Error)
}

func TestWalletRepository_Views(t *testing.T) {
	db := NewTestDB(t)
	r := NewWalletRepository(db)
	ctx := context.Background()

	t.Run("find wallet missing", func(t *testing.T) {
		w, err := r.FindWallet(ctx, 1)
		require.NoError(t, err)
		assert.Nil(t, w)
	})

	t.Run("ensure + find wallet", func(t *testing.T) {
		seedWallet(t, r, 1, 1000)
		w, err := r.FindWallet(ctx, 1)
		require.NoError(t, err)
		require.NotNil(t, w)
		assert.Equal(t, int64(1000), w.Balance)
	})

	t.Run("sum held + list holds", func(t *testing.T) {
		seedWallet(t, r, 2, 500)
		w, err := r.FindWallet(ctx, 2)
		require.NoError(t, err)
		seedHold(t, r, w.ID, 2, 100, "saga-2-A", "held")
		seedHold(t, r, w.ID, 2, 50, "saga-2-B", "held")
		seedHold(t, r, w.ID, 2, 999, "saga-2-C", "released") // 不计

		sum, count, err := r.SumHeldByPartner(ctx, 2)
		require.NoError(t, err)
		assert.Equal(t, int64(150), sum)
		assert.Equal(t, 2, count)

		holds, err := r.ListHolds(ctx, 2)
		require.NoError(t, err)
		assert.Len(t, holds, 2)
	})

	t.Run("list logs filter", func(t *testing.T) {
		seedWallet(t, r, 3, 0)
		require.NoError(t, db.Create(&partnerWalletLogRow{
			PartnerID: 3, Type: "initial_topup", Amount: 100, BalanceAfter: 100,
			IdempotencyKey: "idem-3-A", Status: "committed", OperatorType: "staff", OperatorID: 1,
		}).Error)
		require.NoError(t, db.Create(&partnerWalletLogRow{
			PartnerID: 3, Type: "allocate_to_customer", Amount: -50, BalanceAfter: 50,
			IdempotencyKey: "idem-3-B", Status: "committed", OperatorType: "partner", OperatorID: 3,
		}).Error)

		all, err := r.ListLogs(ctx, 3, walletsvc.LogFilter{Limit: 50})
		require.NoError(t, err)
		assert.Len(t, all, 2)

		filtered, err := r.ListLogs(ctx, 3, walletsvc.LogFilter{Type: "initial_topup", Limit: 50})
		require.NoError(t, err)
		assert.Len(t, filtered, 1)
	})
}

// TestWalletRepository_AdjustBalance_AtomicCheck 验证 HIGH-B7 应用层负余额防护：
// 即便 SQLite 没法 ALTER ADD CHECK，应用层 `balance + ? >= 0` 也能拦下扣超出余额的尝试。
func TestWalletRepository_AdjustBalance_AtomicCheck(t *testing.T) {
	db := NewTestDB(t)
	r := NewWalletRepository(db)
	ctx := context.Background()
	require.NoError(t, r.EnsureWallet(ctx, 99))
	// 初始 balance = 0；+ 200 应通过
	rows, err := r.AdjustBalance(ctx, 99, 200)
	require.NoError(t, err)
	assert.Equal(t, int64(1), rows)

	// -100 留 100；OK
	rows, err = r.AdjustBalance(ctx, 99, -100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), rows)

	// -200 会落到 -100；必须被原子检查拦下。
	rows, err = r.AdjustBalance(ctx, 99, -200)
	require.NoError(t, err)
	assert.Equal(t, int64(0), rows, "AdjustBalance must refuse to drive balance negative")

	// balance 仍为 100，未被破坏
	w, err := r.FindWallet(ctx, 99)
	require.NoError(t, err)
	assert.Equal(t, int64(100), w.Balance)
}

// TestWalletRepository_EnsureWallet_Idempotent 验证 FirstOrCreate 不会重复插。
func TestWalletRepository_EnsureWallet_Idempotent(t *testing.T) {
	db := NewTestDB(t)
	r := NewWalletRepository(db)
	ctx := context.Background()
	require.NoError(t, r.EnsureWallet(ctx, 7))
	require.NoError(t, r.EnsureWallet(ctx, 7))
	w, err := r.FindWallet(ctx, 7)
	require.NoError(t, err)
	require.NotNil(t, w)
	assert.Equal(t, int64(0), w.Balance)
}
