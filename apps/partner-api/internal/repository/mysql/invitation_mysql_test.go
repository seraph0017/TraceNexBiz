// invitation_mysql_test.go — Insert / Resolve / atomic IncUsedCount。
package mysql

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository"
)

func TestInvitationRepository_CRUD(t *testing.T) {
	db := NewTestDB(t)
	r := NewInvitationRepository(db)
	ctx := context.Background()

	t.Run("insert + find by code", func(t *testing.T) {
		_, err := r.Insert(ctx, domain.InvitationCode{
			PartnerID: 1, Code: "CODE-AAA", Type: "permanent",
			UsageLimit: 0, Status: "active",
		})
		require.NoError(t, err)
		got, err := r.FindByCode(ctx, "CODE-AAA")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, int64(1), got.PartnerID)
	})

	t.Run("not found returns nil", func(t *testing.T) {
		got, err := r.FindByCode(ctx, "NOPE")
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("inc used_count atomic, no limit", func(t *testing.T) {
		_, err := r.Insert(ctx, domain.InvitationCode{
			PartnerID: 1, Code: "CODE-PERM", Type: "permanent",
			UsageLimit: 0, Status: "active",
		})
		require.NoError(t, err)
		for i := 1; i <= 5; i++ {
			row, err := r.IncUsedCount(ctx, "CODE-PERM")
			require.NoError(t, err)
			assert.Equal(t, int32(i), row.UsedCount)
		}
	})

	t.Run("inc used_count blocks at limit", func(t *testing.T) {
		_, err := r.Insert(ctx, domain.InvitationCode{
			PartnerID: 1, Code: "CODE-ONE", Type: "one_time",
			UsageLimit: 1, Status: "active",
		})
		require.NoError(t, err)
		// 第一次成功
		row, err := r.IncUsedCount(ctx, "CODE-ONE")
		require.NoError(t, err)
		assert.Equal(t, int32(1), row.UsedCount)
		// 第二次应被限制（usage_limit exhausted）
		_, err = r.IncUsedCount(ctx, "CODE-ONE")
		assert.Error(t, err)
	})

	t.Run("update missing returns ErrNotFound", func(t *testing.T) {
		_, err := r.Update(ctx, "MISSING", func(c domain.InvitationCode) domain.InvitationCode { return c })
		assert.ErrorIs(t, err, repository.ErrNotFound)
	})

	t.Run("update applies, revoke", func(t *testing.T) {
		_, err := r.Insert(ctx, domain.InvitationCode{
			PartnerID: 1, Code: "CODE-REV", Type: "permanent", Status: "active",
		})
		require.NoError(t, err)
		got, err := r.Update(ctx, "CODE-REV", func(c domain.InvitationCode) domain.InvitationCode {
			c.Status = "revoked"
			return c
		})
		require.NoError(t, err)
		assert.Equal(t, "revoked", got.Status)
	})

	t.Run("list by partner", func(t *testing.T) {
		_, _ = r.Insert(ctx, domain.InvitationCode{PartnerID: 9, Code: "P9-A", Type: "permanent", Status: "active"})
		_, _ = r.Insert(ctx, domain.InvitationCode{PartnerID: 9, Code: "P9-B", Type: "permanent", Status: "active"})
		got, err := r.ListByPartner(ctx, 9)
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})
}
