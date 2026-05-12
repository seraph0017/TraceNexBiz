// customer_mysql_test.go — CRUD + BOLA scope + OrphanByPartner + change_log。
package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/customer"
)

func TestCustomerRepository_CRUD(t *testing.T) {
	db := NewTestDB(t)
	r := NewCustomerRepository(db)
	ctx := context.Background()
	pid1 := int64(11)
	pid2 := int64(22)

	t.Run("insert + find by partner scope", func(t *testing.T) {
		id, err := r.Insert(ctx, domain.Customer{
			FyUserID: 1001, PartnerID: &pid1, JoinedVia: "invitation",
			Status: domain.CustomerStatusActive, GroupNameInFyAPI: "partner_11_default",
		})
		require.NoError(t, err)

		got, err := r.FindByIDForPartner(ctx, pid1, id)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, int64(1001), got.FyUserID)

		// BOLA：错误 partner 拿不到
		wrong, err := r.FindByIDForPartner(ctx, pid2, id)
		require.NoError(t, err)
		assert.Nil(t, wrong, "BOLA: cross-partner lookup must return nil")
	})

	t.Run("find by fy_user_id", func(t *testing.T) {
		_, err := r.Insert(ctx, domain.Customer{
			FyUserID: 2002, PartnerID: &pid1, Status: domain.CustomerStatusActive,
		})
		require.NoError(t, err)
		got, err := r.FindByFyUserID(ctx, 2002)
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("update applies", func(t *testing.T) {
		id, err := r.Insert(ctx, domain.Customer{
			FyUserID: 3003, PartnerID: &pid1, Status: domain.CustomerStatusActive,
		})
		require.NoError(t, err)
		got, err := r.Update(ctx, id, func(c domain.Customer) domain.Customer {
			c.Status = domain.CustomerStatusDisabled
			return c
		})
		require.NoError(t, err)
		assert.Equal(t, domain.CustomerStatusDisabled, got.Status)
	})

	t.Run("update missing returns ErrNotFound", func(t *testing.T) {
		_, err := r.Update(ctx, 9_999_999, func(c domain.Customer) domain.Customer { return c })
		assert.ErrorIs(t, err, repository.ErrNotFound)
	})
}

func TestCustomerRepository_OrphanByPartner(t *testing.T) {
	db := NewTestDB(t)
	r := NewCustomerRepository(db)
	ctx := context.Background()
	pid := int64(77)

	for i := 0; i < 3; i++ {
		_, err := r.Insert(ctx, domain.Customer{
			FyUserID: int64(5000 + i), PartnerID: &pid, Status: domain.CustomerStatusActive,
		})
		require.NoError(t, err)
	}
	// 一个 already disabled，应不被 orphan
	_, err := r.Insert(ctx, domain.Customer{
		FyUserID: 6001, PartnerID: &pid, Status: domain.CustomerStatusDisabled,
	})
	require.NoError(t, err)

	n, err := r.OrphanByPartner(ctx, pid, time.Now().Add(30*24*time.Hour), time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	rows, err := r.ListByPartner(ctx, pid, customer.ListFilter{Status: domain.CustomerStatusOrphaned, Limit: 50})
	require.NoError(t, err)
	assert.Len(t, rows, 3)
}

func TestCustomerRepository_ChangeLog(t *testing.T) {
	db := NewTestDB(t)
	r := NewCustomerRepository(db)
	ctx := context.Background()

	from := int64(1)
	to := int64(2)
	id, err := r.InsertChangeLog(ctx, domain.CustomerPartnerChangeLog{
		CustomerID:    100,
		FromPartnerID: &from,
		ToPartnerID:   &to,
		InitiatorType: "customer",
		InitiatorID:   100,
		Status:        "pending_b",
		Reason:        "transfer requested",
	})
	require.NoError(t, err)

	updated, err := r.UpdateChangeLog(ctx, id, func(l domain.CustomerPartnerChangeLog) domain.CustomerPartnerChangeLog {
		l.Status = "completed"
		return l
	})
	require.NoError(t, err)
	assert.Equal(t, "completed", updated.Status)

	_, err = r.UpdateChangeLog(ctx, 9_999, func(l domain.CustomerPartnerChangeLog) domain.CustomerPartnerChangeLog { return l })
	assert.ErrorIs(t, err, repository.ErrNotFound)
}
