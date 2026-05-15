// content_safety_mysql.go — GORM implementation for service/content_safety.Repo.
package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/content_safety"
)

type contentSafetyEventRow struct {
	ID                int64      `gorm:"primaryKey;column:id"`
	FyUserID          int64      `gorm:"column:fy_user_id;index:idx_csafety_user"`
	Kind              string     `gorm:"column:kind;size:16"`
	Provider          string     `gorm:"column:provider;size:32"`
	PromptHash        string     `gorm:"column:prompt_hash;size:64"`
	Category          string     `gorm:"column:category;size:64"`
	Score             float64    `gorm:"column:score"`
	Disposition       string     `gorm:"column:disposition;size:32;index:idx_csafety_disposition"`
	ReviewedBy        *int64     `gorm:"column:reviewed_by"`
	ReviewedAt        *time.Time `gorm:"column:reviewed_at"`
	ReportedTo12377At *time.Time `gorm:"column:reported_to_12377_at"`
	AuditLogID        *int64     `gorm:"column:audit_log_id"`
	TraceID           string     `gorm:"column:trace_id;size:64"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
}

func (contentSafetyEventRow) TableName() string { return "content_safety_event" }

type contentSafetyReportRow struct {
	ID              int64      `gorm:"primaryKey;column:id"`
	EventID         int64      `gorm:"column:event_id;index"`
	TargetAuthority string     `gorm:"column:target_authority;size:32"`
	Payload         string     `gorm:"column:payload;type:text"`
	Status          string     `gorm:"column:status;size:32;index:idx_csreport_pending"`
	SubmittedAt     *time.Time `gorm:"column:submitted_at"`
	SLADueAt        time.Time  `gorm:"column:sla_due_at;index:idx_csreport_pending"`
	ResponsePayload string     `gorm:"column:response_payload;type:text"`
	RetryCount      int        `gorm:"column:retry_count"`
	LastError       string     `gorm:"column:last_error;type:text"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (contentSafetyReportRow) TableName() string { return "content_safety_report" }

// ContentSafetyRepository persists content safety events and 12377 reports.
type ContentSafetyRepository struct {
	db *gorm.DB
}

// NewContentSafetyRepository constructs a content safety repository.
func NewContentSafetyRepository(db *gorm.DB) *ContentSafetyRepository {
	return &ContentSafetyRepository{db: db}
}

var _ content_safety.Repo = (*ContentSafetyRepository)(nil)

// InsertEvent inserts a content safety event.
func (r *ContentSafetyRepository) InsertEvent(ctx context.Context, e *content_safety.Event) (int64, error) {
	row := eventToRow(e)
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

// GetEvent loads a content safety event by id.
func (r *ContentSafetyRepository) GetEvent(ctx context.Context, id int64) (*content_safety.Event, error) {
	var row contentSafetyEventRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, content_safety.ErrEventNotFound
		}
		return nil, err
	}
	return rowToEvent(&row), nil
}

// UpdateEvent updates an event via updater.
func (r *ContentSafetyRepository) UpdateEvent(ctx context.Context, id int64, updater func(content_safety.Event) content_safety.Event) (*content_safety.Event, error) {
	var result content_safety.Event
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row contentSafetyEventRow
		if err := selectForUpdate(tx).First(&row, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return content_safety.ErrEventNotFound
			}
			return err
		}
		next := updater(*rowToEvent(&row))
		nextRow := eventToRow(&next)
		nextRow.ID = id
		nextRow.CreatedAt = row.CreatedAt
		if err := tx.Save(&nextRow).Error; err != nil {
			return err
		}
		result = *rowToEvent(&nextRow)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ListEvents lists events for admin review.
func (r *ContentSafetyRepository) ListEvents(ctx context.Context, q content_safety.ListQuery) ([]content_safety.Event, int, error) {
	dbq := r.db.WithContext(ctx).Model(&contentSafetyEventRow{})
	if q.Disposition != "" {
		dbq = dbq.Where("disposition = ?", q.Disposition)
	}
	if q.Category != "" {
		dbq = dbq.Where("category = ?", q.Category)
	}
	if q.FyUserID != 0 {
		dbq = dbq.Where("fy_user_id = ?", q.FyUserID)
	}
	return listEvents(dbq, q.Limit, q.Offset)
}

// InsertReport inserts a 12377 report.
func (r *ContentSafetyRepository) InsertReport(ctx context.Context, rep *content_safety.Report) (int64, error) {
	row := reportToRow(rep)
	now := time.Now().UTC()
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	if row.UpdatedAt.IsZero() {
		row.UpdatedAt = row.CreatedAt
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

// GetReport loads a report by id.
func (r *ContentSafetyRepository) GetReport(ctx context.Context, id int64) (*content_safety.Report, error) {
	var row contentSafetyReportRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, content_safety.ErrReportNotFound
		}
		return nil, err
	}
	return rowToReport(&row), nil
}

// UpdateReport updates a report via updater.
func (r *ContentSafetyRepository) UpdateReport(ctx context.Context, id int64, updater func(content_safety.Report) content_safety.Report) (*content_safety.Report, error) {
	var result content_safety.Report
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row contentSafetyReportRow
		if err := selectForUpdate(tx).First(&row, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return content_safety.ErrReportNotFound
			}
			return err
		}
		next := updater(*rowToReport(&row))
		nextRow := reportToRow(&next)
		nextRow.ID = id
		nextRow.EventID = row.EventID
		nextRow.CreatedAt = row.CreatedAt
		if nextRow.UpdatedAt.IsZero() {
			nextRow.UpdatedAt = time.Now().UTC()
		}
		if err := tx.Save(&nextRow).Error; err != nil {
			return err
		}
		result = *rowToReport(&nextRow)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ListPendingReports returns pending reports FIFO.
func (r *ContentSafetyRepository) ListPendingReports(ctx context.Context, n int) ([]content_safety.Report, error) {
	if n <= 0 || n > 500 {
		n = 50
	}
	var rows []contentSafetyReportRow
	if err := r.db.WithContext(ctx).
		Where("status = ?", "pending").
		Order("id ASC").
		Limit(n).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rowsToReports(rows), nil
}

// ListReports lists reports for admin review.
func (r *ContentSafetyRepository) ListReports(ctx context.Context, q content_safety.ListQuery) ([]content_safety.Report, int, error) {
	dbq := r.db.WithContext(ctx).Model(&contentSafetyReportRow{})
	if q.Status != "" {
		dbq = dbq.Where("status = ?", q.Status)
	}
	return listReports(dbq, q.Limit, q.Offset)
}

// ListSLABreaches returns non-submitted reports past SLA.
func (r *ContentSafetyRepository) ListSLABreaches(ctx context.Context, now time.Time) ([]content_safety.Report, error) {
	var rows []contentSafetyReportRow
	if err := r.db.WithContext(ctx).
		Where("status <> ? AND sla_due_at < ?", "submitted", now).
		Order("sla_due_at ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rowsToReports(rows), nil
}

func listEvents(dbq *gorm.DB, limit, offset int) ([]content_safety.Event, int, error) {
	var total int64
	if err := dbq.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []contentSafetyEventRow
	if err := dbq.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]content_safety.Event, 0, len(rows))
	for i := range rows {
		out = append(out, *rowToEvent(&rows[i]))
	}
	return out, int(total), nil
}

func listReports(dbq *gorm.DB, limit, offset int) ([]content_safety.Report, int, error) {
	var total int64
	if err := dbq.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []contentSafetyReportRow
	if err := dbq.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rowsToReports(rows), int(total), nil
}

func rowsToReports(rows []contentSafetyReportRow) []content_safety.Report {
	out := make([]content_safety.Report, 0, len(rows))
	for i := range rows {
		out = append(out, *rowToReport(&rows[i]))
	}
	return out
}

func eventToRow(e *content_safety.Event) contentSafetyEventRow {
	if e == nil {
		return contentSafetyEventRow{}
	}
	return contentSafetyEventRow{
		ID:                e.ID,
		FyUserID:          e.FyUserID,
		Kind:              e.Kind,
		Provider:          e.Provider,
		PromptHash:        e.PromptHash,
		Category:          e.Category,
		Score:             e.Score,
		Disposition:       e.Disposition,
		ReviewedBy:        e.ReviewedBy,
		ReviewedAt:        e.ReviewedAt,
		ReportedTo12377At: e.ReportedTo12377At,
		AuditLogID:        e.AuditLogID,
		TraceID:           e.TraceID,
		CreatedAt:         e.CreatedAt,
	}
}

func rowToEvent(r *contentSafetyEventRow) *content_safety.Event {
	return &content_safety.Event{
		ID:                r.ID,
		FyUserID:          r.FyUserID,
		Kind:              r.Kind,
		Provider:          r.Provider,
		PromptHash:        r.PromptHash,
		Category:          r.Category,
		Score:             r.Score,
		Disposition:       r.Disposition,
		ReviewedBy:        r.ReviewedBy,
		ReviewedAt:        r.ReviewedAt,
		ReportedTo12377At: r.ReportedTo12377At,
		AuditLogID:        r.AuditLogID,
		TraceID:           r.TraceID,
		CreatedAt:         r.CreatedAt,
	}
}

func reportToRow(r *content_safety.Report) contentSafetyReportRow {
	if r == nil {
		return contentSafetyReportRow{}
	}
	return contentSafetyReportRow{
		ID:              r.ID,
		EventID:         r.EventID,
		TargetAuthority: r.TargetAuthority,
		Payload:         r.Payload,
		Status:          r.Status,
		SubmittedAt:     r.SubmittedAt,
		SLADueAt:        r.SLADueAt,
		ResponsePayload: r.ResponsePayload,
		RetryCount:      r.RetryCount,
		LastError:       r.LastError,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
	}
}

func rowToReport(r *contentSafetyReportRow) *content_safety.Report {
	return &content_safety.Report{
		ID:              r.ID,
		EventID:         r.EventID,
		TargetAuthority: r.TargetAuthority,
		Payload:         r.Payload,
		Status:          r.Status,
		SubmittedAt:     r.SubmittedAt,
		SLADueAt:        r.SLADueAt,
		ResponsePayload: r.ResponsePayload,
		RetryCount:      r.RetryCount,
		LastError:       r.LastError,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
	}
}
