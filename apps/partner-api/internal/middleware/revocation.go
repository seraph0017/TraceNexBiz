// Redis-backed jti revocation store (backend §7.2 / ADR-007 v0.2 fail-closed).
//
// Lookup key: "revoked:jti:<jti>". 任意 EXISTS == 1 视为 revoked。
// Redis 错误必须返回 error — 上层 JWT middleware 会 fail-closed 返 503。
//
// 写入侧（auth.Service.Logout / kyc_approved_at rotate）已由 service 层
// 在 auth/memory.go 的 RevocationStore 接口处理；本文件只暴露读侧
// 实现给 main.go 装配 middleware.RevocationStore。
package middleware

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

// RedisRevocationStore wraps a *redis.Client to satisfy middleware.RevocationStore.
//
// 读路径 fail-closed：Redis 不可达 / 超时 → 返回 err，上层返 503。
type RedisRevocationStore struct {
	rdb     *redis.Client
	timeout time.Duration
}

// NewRedisRevocationStore 构造。timeout<=0 时默认 1s。
func NewRedisRevocationStore(rdb *redis.Client, timeout time.Duration) *RedisRevocationStore {
	if timeout <= 0 {
		timeout = 1 * time.Second
	}
	return &RedisRevocationStore{rdb: rdb, timeout: timeout}
}

// IsRevoked 查询 revoked:jti:<jti>。EXISTS == 1 视为 revoked。
func (s *RedisRevocationStore) IsRevoked(jti string) (bool, error) {
	if s.rdb == nil {
		// nil client — fail-closed
		return true, redis.ErrClosed
	}
	if jti == "" {
		// no jti claim — fail-closed at JWT middleware via revocation lookup miss is fine,
		// but make absent jti an explicit revoke to avoid replay of unminted tokens
		return true, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	n, err := s.rdb.Exists(ctx, "revoked:jti:"+jti).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
