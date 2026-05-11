// PII scrubber middleware + zerolog hook（backend §12.2）。
//
// 三层 scrubber（与 frontend §15.2 一致）：
//   1. logger：zerolog hook 识别 `pii:"true"` tag 与正则 (身份证 18 位 / 手机号 11 位 / RFC5322 email)
//   2. Sentry/SLS beforeSend：相同 hook
//   3. saga_step.payload / outbox.last_error 写入前 scrubPII()
//
// W0 scaffold：见 pkg/piiscrubber 里的实际实现入口；本文件仅装 middleware 透传。
package middleware

import "github.com/gin-gonic/gin"

// PIIScrubber 把 c.Set("pii_scrubber", scrubber) 注入下游 service 使用。
// W1a 实现：scrubber 单例由 main.go 注入 cfg-driven 模式列表。
func PIIScrubber() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO(W1a): wire pii scrubber singleton; per backend §12.2 / §16.6.
		c.Next()
	}
}
