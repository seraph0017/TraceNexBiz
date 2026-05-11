// Webhook 独立 idempotency middleware（integration v0.2.1 ARCH-HIGH-NEW-E + backend §7.1 webhook chain）。
//
// 与 user-facing idempotency 是两套独立机制（backend §7.1 footer "隔离原则"）：
//   - 表 / Redis namespace 不共享
//   - Redis 健康时 fail-closed；Redis 故障时 fail-open（依赖业务层 topup_intent.uk_topup_channel_trade UNIQUE 兜底）
//   - middleware 链不交叉（webhook 不持 JWT、不走 permission）
//
// W0 scaffold：仅给 wiring；W1a 实现 (provider, signer, event_id) 三元组 SETNX。
package middleware

import "github.com/gin-gonic/gin"

// WebhookIdempotency 实现 SETNX webhook:{provider}:{signer}:{event_id} EX 86400。
//
// W1a 实现要点（backend §7.1）：
//   1. 解析 provider / signer / event_id（持牌方 mchid + 回调 ID 或派生键）
//   2. Redis SETNX，命中即直接 200 ack 不触发业务
//   3. Redis 不可达 → fail-open + alert（与 user-facing fail-closed 反向）
//   4. 写 audit_log action='webhook.idempotency_hit'
//   5. 持久化兜底：业务层 SELECT FOR UPDATE topup_intent + UNIQUE(channel, out_trade_no).
func WebhookIdempotency() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO(W1a): per backend §7.1 webhook chain + integration §V0.2.1 ARCH-HIGH-NEW-E.
		c.Next()
	}
}
