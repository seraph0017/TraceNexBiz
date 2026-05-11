// Audit middleware（backend §10.1 sealer 设计 + §3.13 audit_log_unsealed）。
//
// 每个写操作（POST/PUT/DELETE/PATCH）成功后入队 audit_log_unsealed；
// 由独立 cmd/audit-sealer 进程（200ms tick）批量消费 → audit_log + 哈希链。
//
// W0 scaffold：仅给 hook；W1a 实现 actor / target_type / target_id / diff_redacted 写入 + PII 拆侧表。
package middleware

import "github.com/gin-gonic/gin"

// AuditEnqueuer 注入 unsealed 队列（W1a 由 internal/audit.UnsealedRepo 提供）。
type AuditEnqueuer interface {
	Enqueue(c *gin.Context, action string, targetType string, targetID int64, diffRedacted []byte) error
}

// Audit 装配请求成功后写 audit_log_unsealed。
//
// W1a 实现要点：
//   - 仅在 c.Writer.Status 为 2xx 写入
//   - diff_redacted 经 PII scrubber 处理
//   - 含 PII 的 diff 拆到 audit_log_pii 侧表（PIPL §47 删除时才不破坏哈希链）
//   - dual-control 必须 second_approver_id 字段非空（backend §7.4）.
func Audit(_ AuditEnqueuer) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		// TODO(W1a): per backend §10.1 — enqueue iff c.Writer.Status() in 2xx and method is mutation.
	}
}
