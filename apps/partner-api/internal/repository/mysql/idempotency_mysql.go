// idempotency_mysql.go — GORM 实现 repository.IdempotencyRepository (Fix-B' part 2 CRIT-B3).
//
// 同 TX co-commit：service 层 open tx → 在 fn 内调用 InsertWithinTx(tx, rec) → 再做业务写
// → tx.Commit()。UNIQUE (actor_type, actor_id, idempotency_key, endpoint) 冲突 = 重复请求，
// caller 回滚 + Find 取已落库结果回放。
//
// Phase-1 写明文 response_body；待 Fix-C KMS Encrypt 上线后切回 response_cipher / response_key_id。
package mysql

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository"
)

// idempotencyRecordRow 对应 idempotency_record 表（007 + 013 迁移）.
type idempotencyRecordRow struct {
	ID             int64     `gorm:"primaryKey;column:id"`
	ActorType      string    `gorm:"column:actor_type;size:16;uniqueIndex:uk_idem,priority:1"`
	ActorID        int64     `gorm:"column:actor_id;uniqueIndex:uk_idem,priority:2"`
	IdempotencyKey string    `gorm:"column:idempotency_key;size:64;uniqueIndex:uk_idem,priority:3"`
	Endpoint       string    `gorm:"column:endpoint;size:128;uniqueIndex:uk_idem,priority:4"`
	RequestHash    string    `gorm:"column:request_hash;size:64"`
	ResponseStatus int       `gorm:"column:response_status"`
	ResponseHash   string    `gorm:"column:response_hash;size:64"`
	ResponseBody   *string   `gorm:"column:response_body;type:mediumtext"`
	ResponseCipher []byte    `gorm:"column:response_cipher"`
	ResponseKeyID  *string   `gorm:"column:response_key_id;size:128"`
	TraceID        string    `gorm:"column:trace_id;size:64"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	ExpiresAt      time.Time `gorm:"column:expires_at"`
}

func (idempotencyRecordRow) TableName() string { return "idempotency_record" }

// IdempotencyRepository GORM 实现.
type IdempotencyRepository struct {
	db *gorm.DB
}

// NewIdempotencyRepository 构造.
func NewIdempotencyRepository(db *gorm.DB) *IdempotencyRepository {
	return &IdempotencyRepository{db: db}
}

// Find 按 (actor_type, actor_id, key, endpoint) 查；不存在返 gorm.ErrRecordNotFound（保持
// upstream API 透明，service 层用 errors.Is(err, gorm.ErrRecordNotFound) 判断 miss）。
func (r *IdempotencyRepository) Find(ctx context.Context, actorType string, actorID int64, key, endpoint string) (*domain.IdempotencyRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("idempotency mysql: nil db")
	}
	var row idempotencyRecordRow
	err := r.db.WithContext(ctx).
		Where("actor_type = ? AND actor_id = ? AND idempotency_key = ? AND endpoint = ?",
			actorType, actorID, key, endpoint).
		Take(&row).Error
	if err != nil {
		return nil, err
	}
	return rowToIdempotencyDomain(&row), nil
}

// InsertWithinTx 在调用方提供的 tx 中插入；UNIQUE 冲突 → repository.ErrDuplicateKey.
//
// 必须传入业务 tx（同 transaction），不能从 r.db 自行开 tx — 否则就不是 same-TX 语义。
func (r *IdempotencyRepository) InsertWithinTx(tx *gorm.DB, rec *domain.IdempotencyRecord) error {
	if tx == nil {
		return errors.New("idempotency mysql: nil tx (must use caller's business transaction)")
	}
	if rec == nil {
		return errors.New("idempotency mysql: nil record")
	}
	row := idempotencyDomainToRow(rec)
	if err := tx.Create(&row).Error; err != nil {
		if isDuplicateKeyErr(err) {
			return repository.ErrDuplicateKey
		}
		return err
	}
	rec.ID = row.ID
	return nil
}

// isDuplicateKeyErr 探测 GORM / MySQL / SQLite duplicate-key 错误。
//
// gorm.ErrDuplicatedKey (gorm v1.25+); MySQL: "Error 1062"; SQLite (test): "UNIQUE constraint failed".
func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "Error 1062") ||
		strings.Contains(msg, "Duplicate entry") ||
		strings.Contains(msg, "UNIQUE constraint failed")
}

func rowToIdempotencyDomain(r *idempotencyRecordRow) *domain.IdempotencyRecord {
	if r == nil {
		return nil
	}
	body := ""
	if r.ResponseBody != nil {
		body = *r.ResponseBody
	}
	keyID := ""
	if r.ResponseKeyID != nil {
		keyID = *r.ResponseKeyID
	}
	return &domain.IdempotencyRecord{
		ID:             r.ID,
		ActorType:      r.ActorType,
		ActorID:        r.ActorID,
		IdempotencyKey: r.IdempotencyKey,
		Endpoint:       r.Endpoint,
		RequestHash:    r.RequestHash,
		ResponseStatus: r.ResponseStatus,
		ResponseHash:   r.ResponseHash,
		ResponseBody:   body,
		ResponseCipher: r.ResponseCipher,
		ResponseKeyID:  keyID,
		TraceID:        r.TraceID,
		CreatedAt:      r.CreatedAt,
		ExpiresAt:      r.ExpiresAt,
	}
}

func idempotencyDomainToRow(d *domain.IdempotencyRecord) idempotencyRecordRow {
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	if d.ExpiresAt.IsZero() {
		d.ExpiresAt = now.Add(24 * time.Hour)
	}
	row := idempotencyRecordRow{
		ID:             d.ID,
		ActorType:      d.ActorType,
		ActorID:        d.ActorID,
		IdempotencyKey: d.IdempotencyKey,
		Endpoint:       d.Endpoint,
		RequestHash:    d.RequestHash,
		ResponseStatus: d.ResponseStatus,
		ResponseHash:   d.ResponseHash,
		ResponseCipher: d.ResponseCipher,
		TraceID:        d.TraceID,
		CreatedAt:      d.CreatedAt,
		ExpiresAt:      d.ExpiresAt,
	}
	if d.ResponseBody != "" {
		v := d.ResponseBody
		row.ResponseBody = &v
	}
	if d.ResponseKeyID != "" {
		v := d.ResponseKeyID
		row.ResponseKeyID = &v
	}
	return row
}
