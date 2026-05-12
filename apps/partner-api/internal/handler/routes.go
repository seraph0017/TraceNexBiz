// 路由注册占位（W0 scaffold）。
// 各业务 group 由 W1a/W1b/W1c 实现。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/middleware"
)

// RegisterTODORoutes 把 partner / customer / admin / webhook / sdk 五大 group 占位起来。
// W1 各 agent 在自己的 group 下增补 endpoint。
//
// 每条占位路由都挂 middleware.WithScope — BOLA fail-closed 政策要求所有路由必须声明 scope。
func RegisterTODORoutes(r *gin.Engine) {
	// W1b: Partner 渠道商后台（backend §4.4）
	partnerGroup := r.Group("/partner")
	partnerGroup.GET("/_status", middleware.WithScope("partner_self"), todoHandler("partner"))

	// W1b: Customer 终端客户后台（backend §4.3）
	customerGroup := r.Group("/customer")
	customerGroup.GET("/_status", middleware.WithScope("customer_self"), todoHandler("customer"))

	// W1c: Admin 平台管理后台（backend §4.5）
	adminGroup := r.Group("/admin")
	adminGroup.GET("/_status", middleware.WithScope("staff_admin"), todoHandler("admin"))

	// W1a: Webhook（backend §7.1 v0.2.1 webhook chain）
	webhookGroup := r.Group("/webhook")
	webhookGroup.POST("/_health", middleware.WithScope("public"), todoHandler("webhook"))

	// W1a: SDK / server-to-server（Bearer fallback，backend §7.2）
	sdkGroup := r.Group("/api/sdk")
	sdkGroup.GET("/_status", middleware.WithScope("sdk"), todoHandler("sdk"))

	// 公开商城 / 招商落地（不需鉴权）
	publicGroup := r.Group("/public")
	publicGroup.GET("/_status", middleware.WithScope("public"), todoHandler("public"))
}

func todoHandler(group string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{
			"success": false,
			"error": gin.H{
				"code":       "BIZ_TODO_NOT_IMPLEMENTED",
				"message_en": group + " endpoints pending W1 agent implementation",
				"message_zh": group + " 端点待 W1 实现",
			},
		})
	}
}
