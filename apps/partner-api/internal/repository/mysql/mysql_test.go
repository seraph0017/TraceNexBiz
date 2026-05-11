package mysql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 简易表名 / mapper 测试，作为 repository 层 sanity check。
// W1a 引入 sqlmock / dockertest 后扩为完整 round-trip 测试。
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
