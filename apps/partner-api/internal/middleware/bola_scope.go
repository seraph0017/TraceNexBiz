// BOLA scope middleware（backend §7.4 / Security CI #1）。
//
// 强制原则：repository 层每个公共方法首参必须含 partner_id / customer_id / staff_id 之一作为
// 显式 scope，防止"别家 partner 看到 X 的客户"类越权（OWASP API1 BOLA）。
// 服务端越权统一返 BIZ_RES_NOT_FOUND（PRD §16.3，不暴露存在性）。
//
// CI gate（W1a 实现 golangci 自定义 analyzer）：
//   - lint 名 `bola-scope-required`：扫描 internal/repository/*.go，断言公共方法首参签名命中
//     `partnerID int64` / `customerID int64` / `staffID int64`，否则 fail-on-miss。
//   - 等价的 e2e：partner B 访问 partner A 的 /customers/:id → 期望 404。
package middleware

import "github.com/gin-gonic/gin"

// BOLAScope 在 router level 把 actor scope 注入 ctx，repository 层取用。
// W1a 实现：从 c.MustGet("jwt_claims") 拿 actor + 写 c.Set("scope_partner_id"|...)。
func BOLAScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO(W1a): per backend §7.4 — derive scope from actor type and inject into ctx.
		//            Repository layer must read scope_* keys and ALWAYS WHERE-clause-filter.
		c.Next()
	}
}
