// PII scrubber middleware（backend §12.2 / Security S-3）。
//
// 出向响应 PII 脱敏：
//   - 仅当 actor_type 以 "staff" 开头 且未持有 verb pii.view_full 时脱敏
//   - partner / customer 自身不脱敏（自己看自己 PII 正常）
//   - 非 JSON 响应原样穿透（基于 Content-Type）
//   - 通过 ResponseWriter wrapper 在 Write 路径上 in-place scrub
//
// 调用顺序假设：必须排在 JWT 之后（依赖 ClaimsFrom）。
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/pkg/piiscrubber"
)

// VerbPIIViewFull 持有此 verb 的 staff 可以看到原始 PII。
const VerbPIIViewFull = "pii.view_full"

// PIIScrubber 出向响应 PII 脱敏 middleware。
func PIIScrubber() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !shouldScrub(c) {
			c.Next()
			return
		}
		rec := &piiResponseRecorder{ResponseWriter: c.Writer}
		c.Writer = rec
		c.Next()
	}
}

// shouldScrub 根据 claims.ActorType / Roles 决定是否脱敏。
func shouldScrub(c *gin.Context) bool {
	cl, ok := ClaimsFrom(c)
	if !ok || cl == nil {
		return false
	}
	if !strings.HasPrefix(cl.ActorType, "staff") {
		return false
	}
	for _, r := range cl.Roles {
		if r == VerbPIIViewFull {
			return false
		}
	}
	return true
}

// piiResponseRecorder Write 路径上 in-place scrub JSON body。
type piiResponseRecorder struct {
	gin.ResponseWriter
}

func (r *piiResponseRecorder) Write(b []byte) (int, error) {
	if strings.Contains(r.Header().Get("Content-Type"), "application/json") {
		b = []byte(piiscrubber.Redact(string(b)))
	}
	return r.ResponseWriter.Write(b)
}

func (r *piiResponseRecorder) WriteString(s string) (int, error) {
	return r.Write([]byte(s))
}
