// CSRF middleware（backend §7.3 / §7.6）：double-submit cookie pattern。
//
// 强制原则：
//   - 仅在 state-changing 方法 (POST/PUT/PATCH/DELETE) 校验
//   - GET/HEAD/OPTIONS 直接放行
//   - /api/internal/* 路径跳过（HMAC 独立链路）
//   - /api/public/* / /public/* 路径跳过（无 session）
//   - /webhook/* 路径跳过（IP allowlist + HMAC）
//   - cookie 与 header 都必须存在，长度 ≥ 32，constant-time 比较
package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// HeaderCSRF mutation 请求必带的 CSRF token header（前端从 tnbiz_csrf cookie 读出回写）。
const HeaderCSRF = "X-Csrf-Token"

// csrfMinLen 最小长度（与 service/auth/auth.go 的 randomHex(32) 输出 64 hex 一致；
// 这里取 32 是 hex 半长底线，方便测试 + 反 brute force）。
const csrfMinLen = 32

// CSRF double-submit token 校验。
func CSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		// skip 路径
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/internal/") ||
			strings.HasPrefix(p, "/api/public/") ||
			strings.HasPrefix(p, "/public/") ||
			strings.HasPrefix(p, "/webhook/") {
			c.Next()
			return
		}

		cookie, _ := c.Cookie(CookieCSRF)
		header := c.GetHeader(HeaderCSRF)
		if cookie == "" || header == "" || len(cookie) < csrfMinLen || len(header) < csrfMinLen {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "csrf_mismatch"})
			return
		}
		if subtle.ConstantTimeCompare([]byte(cookie), []byte(header)) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "csrf_mismatch"})
			return
		}
		c.Next()
	}
}
