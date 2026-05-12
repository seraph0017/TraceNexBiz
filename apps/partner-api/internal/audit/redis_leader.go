// Package audit — Redis-backed LeaderLock adapter (Fix-C item 8).
//
// 把 pkg/leader.RedisLock 适配成 audit.LeaderLock 接口。
// Acquire 等价 TryAcquire；Renew 等价 RedisLock.Renew；Release 等价 Release。
package audit

import (
	"context"
	"errors"

	pkgleader "github.com/seraph0017/tracenexbiz/apps/partner-api/pkg/leader"
)

// RedisLeader 适配 *pkgleader.RedisLock → audit.LeaderLock。
type RedisLeader struct {
	Lock *pkgleader.RedisLock
}

// NewRedisLeader 构造。
func NewRedisLeader(l *pkgleader.RedisLock) *RedisLeader { return &RedisLeader{Lock: l} }

// Acquire .
func (r *RedisLeader) Acquire(ctx context.Context) (bool, error) {
	if r == nil || r.Lock == nil {
		return false, errors.New("audit: RedisLeader.Lock is nil")
	}
	return r.Lock.TryAcquire(ctx)
}

// Renew .
func (r *RedisLeader) Renew(ctx context.Context) error {
	if r == nil || r.Lock == nil {
		return errors.New("audit: RedisLeader.Lock is nil")
	}
	ok, err := r.Lock.Renew(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("audit: leader lease lost (owner mismatch)")
	}
	return nil
}

// Release .
func (r *RedisLeader) Release(ctx context.Context) error {
	if r == nil || r.Lock == nil {
		return nil
	}
	return r.Lock.Release(ctx)
}
