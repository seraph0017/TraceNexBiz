package saga

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
)

// NewIdempotencyRecord builds a minimal replay record for service mutation guards.
func NewIdempotencyRecord(actorType string, actorID int64, key, endpoint, traceID, responseBody string, status int, now time.Time) *domain.IdempotencyRecord {
	if responseBody == "" {
		responseBody = `{"success":true}`
	}
	if status == 0 {
		status = 200
	}
	return &domain.IdempotencyRecord{
		ActorType:      actorType,
		ActorID:        actorID,
		IdempotencyKey: key,
		Endpoint:       endpoint,
		RequestHash:    hashText(endpoint + ":" + key),
		ResponseStatus: status,
		ResponseHash:   hashText(responseBody),
		ResponseBody:   responseBody,
		TraceID:        traceID,
		CreatedAt:      now,
		ExpiresAt:      now.Add(24 * time.Hour),
	}
}

func hashText(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
