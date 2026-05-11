// Package redis 封装 go-redis/v8 客户端。
//
// 注意：固定使用 v8.11.5（与 Fy-api 对齐，避免 v9 ctx 改动；per CLAUDE workspace rules）。
//
// 用途（per backend §7 / §10 + integration §3）：
//   - jti revocation（fail-closed，per ADR-007 v0.2）
//   - rate-limit token bucket
//   - settlement leader election（SETNX，per ADR-008）
//   - Pub/Sub option_update / user_update（per integration §3）
package redis

import (
	"context"
	"errors"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/config"
)

// Open 创建 redis client 并 PING；失败返回 error 由 caller 决定降级。
func Open(cfg *config.Config) (*redis.Client, error) {
	if cfg.Redis.Addr == "" {
		return nil, errors.New("REDIS_ADDR is empty")
	}
	c := redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.Addr,
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		PoolSize:     50,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}
