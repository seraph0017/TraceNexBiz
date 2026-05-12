// Webhook 独立 idempotency middleware（integration §3 + backend §7.1 webhook chain）。
//
// 与 user-facing idempotency 是两套独立机制（namespace 不共享）：
//   - dedup TTL 7 days
//   - key 来自 Idempotency-Key header 或 X-Webhook-Event-Id（per provider 不同）
//   - namespace：webhook:{provider}:{event_id}
//   - 命中即直接 200 ack 不触发业务，body {"status":"already_processed"}
//   - W1a 简化：redis 不可达时 fail-closed（与 backend §7.1 footer 反向 fail-open
//     设计的差异，已在 Round-1 P0 修复中收敛为 fail-closed 对齐）。
package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// HeaderWebhookEventID 备选幂等键（部分渠道独立 event id）。
const HeaderWebhookEventID = "X-Webhook-Event-Id"

// CtxKeyWebhookProvider 路由 binding 写入 provider 名（wechat / alipay / fyapi）。
const CtxKeyWebhookProvider = "webhook_provider"

// WebhookIdempotency 单 provider dedup。
//
// provider 通过 c.Set("webhook_provider", "...") 在路由 binding 时注入；
// 缺失时使用 "default"。
func WebhookIdempotency(rds *redis.Client, ttl time.Duration) gin.HandlerFunc {
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	return func(c *gin.Context) {
		eventID := c.GetHeader(HeaderIdemKey)
		if eventID == "" {
			eventID = c.GetHeader(HeaderWebhookEventID)
		}
		if eventID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "webhook_event_id_required"})
			return
		}
		provider := c.GetString(CtxKeyWebhookProvider)
		if provider == "" {
			provider = "default"
		}
		if rds == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "webhook_idempotency_unavailable"})
			return
		}
		key := "webhook:" + provider + ":" + eventID
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		ok, err := rds.SetNX(ctx, key, "1", ttl).Result()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "webhook_idempotency_unavailable"})
			return
		}
		if !ok {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"status": "already_processed"})
			return
		}
		c.Next()
	}
}
