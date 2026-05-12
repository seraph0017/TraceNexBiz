// BOLA scope middleware（backend §7.4 / Security CI #1 / Round-1 CRIT-C5）。
//
// 每个受保护路由必须通过 WithScope("partner_self" | "customer_self" | "staff_*") 显式声明
// scope；未声明 scope 的路由通过此 middleware 时一律返 404（fail-closed）。
//
// 服务端越权统一返 BIZ_RES_NOT_FOUND（PRD §16.3，不暴露存在性）。
package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// CtxKeyBOLAScope 路由 binding 设置的 scope 名（partner_self / customer_self / staff_*）。
const CtxKeyBOLAScope = "bola_scope"

// CtxKeyScopePartnerID 等：repository 层透传 scope 时使用。
const (
	CtxKeyScopePartnerID  = "scope_partner_id"
	CtxKeyScopeCustomerID = "scope_customer_id"
	CtxKeyScopeStaffID    = "scope_staff_id"
)

// BOLAAttemptLogger BOLA 违规时记录尝试（可选）。nil 时不记录。
type BOLAAttemptLogger interface {
	LogAttempt(actorType string, actorID int64, scope string, path string)
}

// BOLAScope 路由级 BOLA 强制。
func BOLAScope(logger BOLAAttemptLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope := c.GetString(CtxKeyBOLAScope)
		if scope == "" {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		cl, ok := ClaimsFrom(c)
		if !ok || cl == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		switch {
		case scope == "partner_self":
			if cl.ActorType != "partner" || !matchPathOrBodyID(c, "partner_id", cl.ActorID) {
				bolaDeny(c, logger, cl, scope)
				return
			}
			c.Set(CtxKeyScopePartnerID, cl.ActorID)
		case scope == "customer_self":
			if cl.ActorType != "customer" || !matchPathOrBodyID(c, "customer_id", cl.ActorID) {
				bolaDeny(c, logger, cl, scope)
				return
			}
			c.Set(CtxKeyScopeCustomerID, cl.ActorID)
		case strings.HasPrefix(scope, "staff_"):
			if cl.ActorType != "staff" {
				bolaDeny(c, logger, cl, scope)
				return
			}
			c.Set(CtxKeyScopeStaffID, cl.ActorID)
		default:
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		c.Next()
	}
}

// WithScope 路由 builder helper：在 handler 前注入 bola_scope。
//
// 用法：r.GET("/partner/wallet", middleware.WithScope("partner_self"), walletGetHandler(...))
func WithScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(CtxKeyBOLAScope, scope)
		c.Next()
	}
}

// matchPathOrBodyID 检查路径参数（`:partner_id` / `:customer_id` / `:id`）与 claims.ActorID
// 是否匹配；没有路径参数视为同 actor（actor-self 列表 / 详情）。
func matchPathOrBodyID(c *gin.Context, key string, actorID int64) bool {
	candidates := []string{key, strings.TrimSuffix(key, "_id"), "id"}
	for _, k := range candidates {
		v := c.Param(k)
		if v == "" {
			continue
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return false
		}
		if n != actorID {
			return false
		}
	}
	return true
}

func bolaDeny(c *gin.Context, logger BOLAAttemptLogger, cl *Claims, scope string) {
	if logger != nil {
		logger.LogAttempt(cl.ActorType, cl.ActorID, scope, c.Request.URL.Path)
	}
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
}
