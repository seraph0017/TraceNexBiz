// Package leader 实现 Redis SETNX leader 选举（backend §6 / Phase 1；Phase 2 切 K8s Lease，ADR-008）.
//
// 用法（settlement-runner / audit-sealer / kek-rotator / kyc-purge / dispatcher-12377）:
//
//	rdb := ... // *redis.Client
//	owner := uuid.NewString()
//	lk := leader.NewRedisLock(rdb, "cron:audit-sealer", owner, 30*time.Second, 10*time.Second)
//	go lk.Run(ctx, func(ctx context.Context) error { /* leader work */ })
//
// 失败语义：
//   - SETNX 失败（key 已被持有）→ 当前 tick 跳过，下个 tick 再试
//   - Renew 失败（owner 已被抢占 / Redis 断连）→ leader 立即下线，cancel 内部 ctx，回到 SETNX 循环
//   - ctx 退出 → 主动 release（DEL key，仅当 owner 匹配）
//
// 与已有 pkg/leader.TryAcquire/RenewLoop 互补：那两个是底层原语，本结构是 polling-friendly 封装。
package leader

import (
	"context"
	"errors"
	"time"

	"github.com/go-redis/redis/v8"
)

// Lock 抽象一个互斥角色（leader）的获取 / 续约 / 释放。
type Lock interface {
	// Run 阻塞执行 body。Body 仅当本实例是 leader 时被调用，并在 leader 身份丢失时被 cancel。
	Run(ctx context.Context, body func(ctx context.Context) error) error
}

// AlwaysLeader 永远获胜，单机 dev / 测试使用。
type AlwaysLeader struct{}

// Run .
func (AlwaysLeader) Run(ctx context.Context, body func(ctx context.Context) error) error {
	return body(ctx)
}

// RedisLock 基于 Redis SET NX EX + Lua-atomic-renew 的 leader.
type RedisLock struct {
	rdb     *redis.Client
	key     string
	owner   string
	ttl     time.Duration
	refresh time.Duration
}

// NewRedisLock 构造；ttl ≥ 2×refresh（建议 30s/10s）。
func NewRedisLock(rdb *redis.Client, key, owner string, ttl, refresh time.Duration) *RedisLock {
	return &RedisLock{rdb: rdb, key: key, owner: owner, ttl: ttl, refresh: refresh}
}

// renewScript: 仅当当前 value == owner 才续期；返回 1 表示续期成功。
const renewScript = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("pexpire", KEYS[1], ARGV[2]) else return 0 end`

// releaseScript: 仅当 owner 匹配才删 key。
const releaseScript = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`

// TryAcquire 一次性 SETNX。
func (l *RedisLock) TryAcquire(ctx context.Context) (bool, error) {
	if l.rdb == nil {
		return false, errors.New("leader: nil redis client")
	}
	return l.rdb.SetNX(ctx, l.key, l.owner, l.ttl).Result()
}

// Renew 原子续约（仅 owner 匹配）。
func (l *RedisLock) Renew(ctx context.Context) (bool, error) {
	res, err := l.rdb.Eval(ctx, renewScript, []string{l.key}, l.owner, l.ttl.Milliseconds()).Result()
	if err != nil {
		return false, err
	}
	n, _ := res.(int64)
	return n == 1, nil
}

// Release 原子释放（仅 owner 匹配）。
func (l *RedisLock) Release(ctx context.Context) error {
	_, err := l.rdb.Eval(ctx, releaseScript, []string{l.key}, l.owner).Result()
	return err
}

// Run 阻塞循环：抢锁→续约→执行 body→断连退出 body→重新抢锁。
//
// 任何一次 Renew 失败 → 立即 cancel body 内部 ctx → body 退出 → 进入下一轮 SETNX。
// 外层 ctx 取消 → 主动 Release，函数返回 ctx.Err()。
func (l *RedisLock) Run(ctx context.Context, body func(ctx context.Context) error) error {
	if l.rdb == nil {
		// dev fallback：直接当 leader 跑（避免 dev 启动失败）。
		return body(ctx)
	}
	backoff := 1 * time.Second
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ok, err := l.TryAcquire(ctx)
		if err != nil || !ok {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			if backoff < l.refresh {
				backoff *= 2
			}
			continue
		}
		backoff = 1 * time.Second

		// 我们是 leader：起 body + renew loop。
		leaderCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- body(leaderCtx) }()

		renewTick := time.NewTicker(l.refresh)
	renewLoop:
		for {
			select {
			case <-leaderCtx.Done():
				break renewLoop
			case bErr := <-done:
				cancel()
				_ = l.Release(context.Background())
				renewTick.Stop()
				return bErr
			case <-renewTick.C:
				renewed, err := l.Renew(leaderCtx)
				if err != nil || !renewed {
					// 失去 leader：cancel body，回到 SETNX 循环。
					cancel()
					<-done
					break renewLoop
				}
			}
		}
		renewTick.Stop()
		_ = l.Release(context.Background())
		// loop again
	}
}
