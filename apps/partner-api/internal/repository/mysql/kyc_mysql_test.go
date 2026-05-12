// kyc_mysql_test.go — Insert / Find / Update / ListPending / PurgeCold。
package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository"
	kycsvc "github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/kyc"
)

func TestKYCRepository_CRUD(t *testing.T) {
	db := NewTestDB(t)
	r := NewKYCRepository(db)
	ctx := context.Background()

	t.Run("insert + find by fy / id / blind_index", func(t *testing.T) {
		id, err := r.Insert(ctx, domain.KYCApplication{
			FyUserID: 1, Type: kycsvc.TypeIndividual, Status: kycsvc.StatusSubmitted,
			LegalPersonIDBlindIndex: "blind:abc",
			SubmittedAt:             ptrTimeUTC(time.Now()),
		})
		require.NoError(t, err)
		assert.NotZero(t, id)

		byID, err := r.FindByID(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, byID)
		assert.Equal(t, int64(1), byID.FyUserID)

		byFy, err := r.FindByFyUserID(ctx, 1)
		require.NoError(t, err)
		require.NotNil(t, byFy)

		byBI, err := r.FindByLegalIDBlindIndex(ctx, "blind:abc")
		require.NoError(t, err)
		require.NotNil(t, byBI)
	})

	t.Run("not found returns nil", func(t *testing.T) {
		got, err := r.FindByID(ctx, 99999)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("update transitions status", func(t *testing.T) {
		id, err := r.Insert(ctx, domain.KYCApplication{
			FyUserID: 2, Type: kycsvc.TypeEnterprise, Status: kycsvc.StatusSubmitted,
			LegalPersonIDBlindIndex: "blind:002",
		})
		require.NoError(t, err)
		got, err := r.Update(ctx, id, func(a domain.KYCApplication) domain.KYCApplication {
			a.Status = kycsvc.StatusUnderReview
			return a
		})
		require.NoError(t, err)
		assert.Equal(t, kycsvc.StatusUnderReview, got.Status)
	})

	t.Run("update missing returns ErrNotFound", func(t *testing.T) {
		_, err := r.Update(ctx, 999_999, func(a domain.KYCApplication) domain.KYCApplication { return a })
		assert.ErrorIs(t, err, repository.ErrNotFound)
	})

	t.Run("list pending review", func(t *testing.T) {
		_, _ = r.Insert(ctx, domain.KYCApplication{FyUserID: 10, Type: 1, Status: kycsvc.StatusSubmitted, LegalPersonIDBlindIndex: "bi10"})
		_, _ = r.Insert(ctx, domain.KYCApplication{FyUserID: 11, Type: 1, Status: kycsvc.StatusUnderReview, LegalPersonIDBlindIndex: "bi11"})
		_, _ = r.Insert(ctx, domain.KYCApplication{FyUserID: 12, Type: 1, Status: kycsvc.StatusApproved, LegalPersonIDBlindIndex: "bi12"})
		got, err := r.ListPendingReview(ctx, 50)
		require.NoError(t, err)
		// 至少有上面两条 submitted/under_review；之前 test 也可能塞了 submitted
		assert.GreaterOrEqual(t, len(got), 2)
		for _, a := range got {
			assert.Contains(t, []string{kycsvc.StatusSubmitted, kycsvc.StatusUnderReview}, a.Status)
		}
	})

	t.Run("purge cold expired", func(t *testing.T) {
		past := time.Now().AddDate(0, 0, -1)
		_, err := r.Insert(ctx, domain.KYCApplication{
			FyUserID: 999, Type: 1, Status: kycsvc.StatusApproved,
			LegalPersonIDBlindIndex: "bi999",
			ColdArchiveExpiresAt:    &past,
		})
		require.NoError(t, err)
		n, err := r.PurgeColdExpired(ctx, time.Now(), 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, n, 1)
	})
}

func ptrTimeUTC(t time.Time) *time.Time {
	t = t.UTC()
	return &t
}
