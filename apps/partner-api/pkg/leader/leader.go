// Package leader 实现 Redis SETNX leader 选举（backend §6 / Phase 1；Phase 2 切 K8s Lease，ADR-008）.
//
// 用法（settlement-runner / audit-sealer / kek-rotator）:
//
//	owner := uuid.NewString()
//	if !leader.TryAcquire(ctx, rdb, "cron:settlement:monthly", owner, 60*time.Second) {
//	    return // not leader
//	}
//	go leader.RenewLoop(ctx, rdb, "cron:settlement:monthly", owner, 60*time.Second, 20*time.Second)
//
// W0 scaffold：W1a 落 redis-go SETNX + lua atomic renew + KeyExpired callback。
package leader

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

// TryAcquire 尝试获取 leader（SETNX EX TTL）.
func TryAcquire(ctx context.Context, rdb *redis.Client, key, owner string, ttl time.Duration) bool {
	ok, err := rdb.SetNX(ctx, key, owner, ttl).Result()
	if err != nil {
		return false
	}
	return ok
}

// RenewLoop 每隔 interval 续约（保持 leader 身份）；ctx Done 退出。
//
// W1a 实现：用 lua 脚本原子比较 owner 再续约（防止 owner 已被抢占）.
func RenewLoop(ctx context.Context, rdb *redis.Client, key, owner string, ttl, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// TODO(W1a): atomic compare-and-set via lua;
			//   if KEYS[1].value == ARGV[1] then redis.call('expire', KEYS[1], ARGV[2]) end.
			rdb.Expire(ctx, key, ttl)
		}
	}
}
