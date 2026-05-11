// 安全 headers（backend §7.8）：CSP / HSTS / X-Frame-Options / Referrer-Policy / Permissions-Policy。
// W0 scaffold：默认 strict 头部；W1a 按 frontend §12.1 同步细化 connect-src OSS 列表。
package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders 全局响应头。
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		// HSTS（仅 prod 启用；W1a 按 cfg.Env 判断）
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		// XFO + 旧版兼容
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=()")
		// CSP（W1a 收紧 nonce / strict-dynamic / connect-src OSS allowlist）
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self'; frame-ancestors 'none'")
		c.Next()
	}
}
