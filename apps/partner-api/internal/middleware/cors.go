// CORS（backend §7.7）：白名单只允许 partner.tracenex.cn / admin.tracenex.cn / dev localhost。
// W0 scaffold：W1a 接 cfg.AllowedOrigins。
package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS 返回 partner-api 默认 CORS 中间件。
func CORS(allowedOrigins []string) gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "X-Csrf-Token", "Idempotency-Key", HeaderTraceID, "X-Second-Approver-Token"},
		ExposeHeaders:    []string{HeaderTraceID},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}
