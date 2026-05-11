package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestValidateInvariants(t *testing.T) {
	c := &Config{
		Env:                    EnvDev,
		IdempotencyTTL:         24 * time.Hour,
		SagaWallClock:          1 * time.Hour,
		InternalIdempotencyTTL: 7 * 24 * time.Hour,
	}
	assert.NoError(t, c.validate())
}

func TestValidateSagaTTLBound(t *testing.T) {
	c := &Config{
		Env:                    EnvDev,
		IdempotencyTTL:         1 * time.Hour,
		SagaWallClock:          2 * time.Hour, // 违反
		InternalIdempotencyTTL: 7 * 24 * time.Hour,
	}
	assert.Error(t, c.validate())
}

func TestRegistryHasICP(t *testing.T) {
	found := false
	for _, s := range Registry {
		if s.Key == "compliance.icp_record_no" {
			found = true
			break
		}
	}
	assert.True(t, found, "compliance.icp_record_no must be in registry")
}
