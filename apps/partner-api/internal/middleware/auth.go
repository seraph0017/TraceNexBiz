// JWT auth middleware（backend §7.2，ADR-007）。
// W0 scaffold：仅给签名 + ctx wiring；W1a 实现 fail-closed Redis revocation lookup。
package middleware

import (
	"github.com/gin-gonic/gin"
)

// CookieAccess access_token httpOnly cookie 名（v0.2 SEC CRIT-1）。
const CookieAccess = "tnbiz_access"

// CookieRefresh refresh_token httpOnly cookie 名。
const CookieRefresh = "tnbiz_refresh"

// CookieCSRF non-httpOnly double-submit cookie 名。
const CookieCSRF = "tnbiz_csrf"

// Verifier JWT 公钥校验抽象；prod 实现使用 RS256，env JWT_VERIFY_KEY_PEM。
type Verifier interface {
	Verify(token string) (*Claims, error)
}

// Claims partner-api 全 actor 共享的 JWT 载荷（backend §7.2）。
type Claims struct {
	Sub       int64    `json:"sub"`
	ActorType string   `json:"actor_type"`
	ActorID   int64    `json:"actor_id"`
	Roles     []string `json:"roles"`
	Jti       string   `json:"jti"`
	Iat       int64    `json:"iat"`
	Exp       int64    `json:"exp"`
	Elev      bool     `json:"elev,omitempty"`
	Site      string   `json:"site,omitempty"`
}

// RevocationStore 抽象 Redis revoked:jti:* 查询；不可达必 fail-closed。
type RevocationStore interface {
	IsRevoked(jti string) (bool, error)
}

// JWT 装配 cookie-first JWT 校验 + jti revocation lookup。
//
// W1a 实现要点（按 backend §7.2）：
//   - 从 cookie tnbiz_access 取 token；/api/sdk/* 路径 fallback Bearer
//   - 公钥从 cfg.JWTVerifyKeyPEM（KMS Secret Manager 注入），不从 biz_setting 读
//   - revocation lookup fail-closed：Redis 不可达 → 503 + page，不放行
//   - kyc_approved_at 后所有 jti.iat 早于 approved_at 视为 revoked（D-9）
func JWT(_ Verifier, _ RevocationStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO(W1a): per backend §7.2 — extract cookie/Bearer, verify, check revocation fail-closed,
		//            inject Claims into c.Set("jwt_claims", ...) + actor context.
		c.Next()
	}
}
