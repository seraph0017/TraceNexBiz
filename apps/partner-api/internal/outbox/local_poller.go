// local_poller.go — local outbox SOURCE publisher to MNS.
package outbox

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// LocalRow is a pending local outbox event.
type LocalRow struct {
	ID        int64
	EventType string
	Body      []byte
	TraceID   string
	CreatedAt time.Time
}

type localOutboxRow struct {
	ID          int64      `gorm:"primaryKey;column:id"`
	EventType   string     `gorm:"column:event_type;size:128"`
	Payload     []byte     `gorm:"column:payload"`
	Status      string     `gorm:"column:status;size:32;index"`
	TraceID     string     `gorm:"column:trace_id;size:64"`
	LastError   string     `gorm:"column:last_error;type:text"`
	PublishedAt *time.Time `gorm:"column:published_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
}

func (localOutboxRow) TableName() string { return "outbox" }

// LocalRepo stores local outbox publish state.
type LocalRepo interface {
	ClaimPending(ctx context.Context, n int) ([]LocalRow, error)
	MarkSent(ctx context.Context, id int64) error
	MarkFailed(ctx context.Context, id int64, errText string) error
}

// GormLocalRepo implements LocalRepo.
type GormLocalRepo struct {
	db *gorm.DB
}

// NewGormLocalRepo constructs a local outbox repo.
func NewGormLocalRepo(db *gorm.DB) *GormLocalRepo { return &GormLocalRepo{db: db} }

// ClaimPending returns pending rows FIFO.
func (r *GormLocalRepo) ClaimPending(ctx context.Context, n int) ([]LocalRow, error) {
	if n <= 0 || n > 500 {
		n = 100
	}
	var rows []localOutboxRow
	if err := r.db.WithContext(ctx).
		Where("status = ?", "pending").
		Order("id ASC").
		Limit(n).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]LocalRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, LocalRow{
			ID:        row.ID,
			EventType: row.EventType,
			Body:      row.Payload,
			TraceID:   row.TraceID,
			CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

// MarkSent marks a row as sent.
func (r *GormLocalRepo) MarkSent(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Model(&localOutboxRow{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       "sent",
			"published_at": &now,
			"updated_at":   now,
			"last_error":   "",
		}).Error
}

// MarkFailed stores the last publish error while keeping the row pending.
func (r *GormLocalRepo) MarkFailed(ctx context.Context, id int64, errText string) error {
	return r.db.WithContext(ctx).Model(&localOutboxRow{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     "pending",
			"last_error": errText,
			"updated_at": time.Now().UTC(),
		}).Error
}

// PublishPendingOnce publishes up to batch pending rows and marks successful rows sent.
func PublishPendingOnce(ctx context.Context, repo LocalRepo, pub Publisher, queueName, dataRegion string, batch int) (int, error) {
	rows, err := repo.ClaimPending(ctx, batch)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, row := range rows {
		attrs := map[string]string{
			"event_type":  row.EventType,
			"data_region": dataRegion,
			"trace_id":    row.TraceID,
		}
		if err := pub.Publish(ctx, queueName, row.Body, attrs); err != nil {
			_ = repo.MarkFailed(ctx, row.ID, err.Error())
			continue
		}
		if err := repo.MarkSent(ctx, row.ID); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}
