// CSRF middleware（backend §7.6 / ARCH-CRIT-6）：double-submit token + Origin/Referer 校验。
// W0 scaffold：仅给 stub；W1a 实现 token 比较 / 安全 cookie 写入 / SameSite=Lax 配套。
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HeaderCSRF mutation 请求必带的 CSRF token header（前端从 tnbiz_csrf cookie 读出回写）。
const HeaderCSRF = "X-Csrf-Token"

// CSRF 强制 state-changing 请求 (POST/PUT/DELETE/PATCH) 必带 X-Csrf-Token == cookie tnbiz_csrf 值。
//
// W1a 实现：
//   1. 校验 Origin / Referer 必须在 cfg.AllowedOrigins
//   2. 比较 cookie 值与 header 值（constant-time compare）
//   3. /webhook/* 路径跳过（独立 IP allowlist + HMAC 链路）.
func CSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		// TODO(W1a): per backend §7.6 — Origin/Referer allowlist + constant-time double-submit compare.
		c.Next()
	}
}
