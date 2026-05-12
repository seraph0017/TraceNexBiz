// JWT auth middleware（backend §7.2，ADR-007）。
//
// 设计要点：
//   - cookie-first（tnbiz_access），/api/sdk/* 路径 fallback Authorization: Bearer
//   - 公钥来自 cfg.JWT.VerifyKeyPEM（KMS Secret Manager 注入），不从 biz_setting 读
//   - revocation lookup fail-closed：Redis 不可达 → 503，不放行
//   - kyc_approved_at（D-9）由 service 层在 revoke 时把全部旧 jti 写入 revoked:jti:*，
//     此处仅需查 jti 即可命中
//   - 成功后 c.Set("jwt_claims", claims) + actor context；任何失败 401 abort
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CookieAccess access_token httpOnly cookie 名（v0.2 SEC CRIT-1）。
const CookieAccess = "tnbiz_access"

// CookieRefresh refresh_token httpOnly cookie 名。
const CookieRefresh = "tnbiz_refresh"

// CookieCSRF non-httpOnly double-submit cookie 名。
const CookieCSRF = "tnbiz_csrf"

// CtxKeyJWTClaims jwt 解析后的 claims 注入 ctx 的 key。
const CtxKeyJWTClaims = "jwt_claims"

// CtxKeyActorType / CtxKeyActorID 是 service / handler 层取 actor 的统一 key。
const (
	CtxKeyActorType = "actor_type"
	CtxKeyActorID   = "actor_id"
)

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
// clock 注入用于测试；prod 传 time.Now。
func JWT(v Verifier, rs RevocationStore, clock func() int64) gin.HandlerFunc {
	if clock == nil {
		clock = nowUnix
	}
	return func(c *gin.Context) {
		raw := extractToken(c)
		if raw == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		cl, err := v.Verify(raw)
		if err != nil || cl == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		// exp 校验（verifier 自身也会校验，此处兜底）
		now := clock()
		if cl.Exp > 0 && cl.Exp <= now {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		// fail-closed revocation lookup
		revoked, err := rs.IsRevoked(cl.Jti)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "auth_revocation_unavailable"})
			return
		}
		if revoked {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		// inject ctx
		c.Set(CtxKeyJWTClaims, cl)
		c.Set(CtxKeyActorType, cl.ActorType)
		c.Set(CtxKeyActorID, cl.ActorID)
		c.Next()
	}
}

// extractToken cookie-first / Bearer fallback。
//
// /api/sdk/* 路径优先用 Bearer（server-to-server），其它路径优先 cookie。
func extractToken(c *gin.Context) string {
	path := c.Request.URL.Path
	if strings.HasPrefix(path, "/api/sdk/") {
		if tok := bearerToken(c); tok != "" {
			return tok
		}
		if v, err := c.Cookie(CookieAccess); err == nil && v != "" {
			return v
		}
		return ""
	}
	if v, err := c.Cookie(CookieAccess); err == nil && v != "" {
		return v
	}
	return bearerToken(c)
}

func bearerToken(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if h == "" {
		return ""
	}
	const p = "Bearer "
	if len(h) > len(p) && strings.EqualFold(h[:len(p)], p) {
		return strings.TrimSpace(h[len(p):])
	}
	return ""
}

// ClaimsFrom 工具方法：从 ctx 取 *Claims（auth middleware 已注入）。
func ClaimsFrom(c *gin.Context) (*Claims, bool) {
	v, ok := c.Get(CtxKeyJWTClaims)
	if !ok {
		return nil, false
	}
	cl, ok := v.(*Claims)
	return cl, ok
}
