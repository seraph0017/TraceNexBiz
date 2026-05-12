package mysql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 表名 sanity check（保留 W1a 的初版断言）。
func TestPartnerTableName(t *testing.T) {
	assert.Equal(t, "partner", partnerRow{}.TableName())
}

func TestRowToPartner(t *testing.T) {
	row := &partnerRow{ID: 42, FyUserID: 7, InvitationCode: "ABC123", Status: "approved"}
	p := rowToPartner(row)
	assert.Equal(t, int64(42), p.ID)
	assert.Equal(t, int64(7), p.FyUserID)
	assert.Equal(t, "ABC123", p.InvitationCode)
}
