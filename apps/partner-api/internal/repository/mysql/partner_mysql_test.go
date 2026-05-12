// partner_mysql_test.go — GORM PartnerRepository CRUD + soft-delete round-trip。
package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/partner"
)

func newPartner() domain.Partner {
	now := time.Now().UTC()
	return domain.Partner{
		FyUserID:         101,
		InvitationCode:   "INV-PARTNER-A",
		Status:           domain.PartnerStatusApplied,
		ContactName:      "Alice",
		ContactEmail:     "alice@example.com",
		ContactEmailHMAC: "hmac:alice",
		Tier:             1,
		AppliedAt:        now,
		TaxStatus:        domain.TaxUnknown,
	}
}

func TestPartnerRepository_CRUD(t *testing.T) {
	db := NewTestDB(t)
	r := NewPartnerRepository(db)
	ctx := context.Background()

	t.Run("insert + find by id / fy / hmac", func(t *testing.T) {
		id, err := r.Insert(ctx, newPartner())
		require.NoError(t, err)
		assert.NotZero(t, id)

		byID, err := r.FindByID(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, byID)
		assert.Equal(t, "INV-PARTNER-A", byID.InvitationCode)

		byFy, err := r.FindByFyUserID(ctx, 101)
		require.NoError(t, err)
		require.NotNil(t, byFy)
		assert.Equal(t, id, byFy.ID)

		byHMAC, err := r.FindByEmailHMAC(ctx, "hmac:alice")
		require.NoError(t, err)
		require.NotNil(t, byHMAC)
		assert.Equal(t, id, byHMAC.ID)
	})

	t.Run("not found returns nil, nil", func(t *testing.T) {
		got, err := r.FindByID(ctx, 99999)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("update applies and persists", func(t *testing.T) {
		id, err := r.Insert(ctx, domain.Partner{
			FyUserID: 202, InvitationCode: "INV-B",
			Status: domain.PartnerStatusApplied,
			ContactEmailHMAC: "hmac:b",
			TaxStatus: domain.TaxUnknown,
			AppliedAt: time.Now().UTC(),
		})
		require.NoError(t, err)

		updated, err := r.Update(ctx, id, func(p domain.Partner) domain.Partner {
			p.Status = domain.PartnerStatusApproved
			now := time.Now().UTC()
			p.ApprovedAt = &now
			return p
		})
		require.NoError(t, err)
		assert.Equal(t, domain.PartnerStatusApproved, updated.Status)
		assert.NotNil(t, updated.ApprovedAt)

		again, err := r.FindByID(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, domain.PartnerStatusApproved, again.Status)
	})

	t.Run("update on missing returns ErrNotFound", func(t *testing.T) {
		_, err := r.Update(ctx, 999999, func(p domain.Partner) domain.Partner { return p })
		assert.ErrorIs(t, err, repository.ErrNotFound)
	})

	t.Run("list with status filter", func(t *testing.T) {
		_, err := r.Insert(ctx, domain.Partner{
			FyUserID: 303, InvitationCode: "INV-C",
			Status: domain.PartnerStatusReviewing,
			ContactEmailHMAC: "hmac:c",
			TaxStatus: domain.TaxUnknown,
			AppliedAt: time.Now().UTC(),
		})
		require.NoError(t, err)

		got, err := r.List(ctx, partner.ListFilter{Status: string(domain.PartnerStatusReviewing), Limit: 50})
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("soft-delete excludes from find", func(t *testing.T) {
		id, err := r.Insert(ctx, domain.Partner{
			FyUserID: 404, InvitationCode: "INV-D",
			Status: domain.PartnerStatusApplied,
			ContactEmailHMAC: "hmac:d",
			TaxStatus: domain.TaxUnknown,
			AppliedAt: time.Now().UTC(),
		})
		require.NoError(t, err)
		require.NoError(t, db.Delete(&partnerRow{}, id).Error)

		got, err := r.FindByID(ctx, id)
		require.NoError(t, err)
		assert.Nil(t, got, "soft-deleted row must not surface")
	})
}
