// Package idempotency adapts idempotency_record storage for middleware DB replay.
package idempotency

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/middleware"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository"
)

// DBLookup adapts repository.IdempotencyRepository to middleware.IdempotencyDBLookup.
type DBLookup struct {
	repo repository.IdempotencyRepository
}

// NewDBLookup constructs a middleware DB lookup adapter.
func NewDBLookup(repo repository.IdempotencyRepository) *DBLookup {
	return &DBLookup{repo: repo}
}

// Find returns a replayable response, or nil on miss.
func (l *DBLookup) Find(ctx context.Context, actorType string, actorID int64, key, endpoint string) (*middleware.IdempotencyDBRecord, error) {
	if l == nil || l.repo == nil {
		return nil, nil
	}
	rec, err := l.repo.Find(ctx, actorType, actorID, key, endpoint)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &middleware.IdempotencyDBRecord{
		ResponseStatus: rec.ResponseStatus,
		ResponseBody:   []byte(rec.ResponseBody),
	}, nil
}
