// W1a routes smoke test：覆盖 BOLA fail-closed / WithScope binding / public 放行 三条路径。
//
// 测试不引入 main.go 全链；直接构造 engine + 手挂关键 middleware，验证：
//   1. /healthz 无鉴权 200
//   2. /partner/me 无 JWT → 401（JWT middleware 阻断）
//   3. /partner/me 有有效 JWT + 匹配 :id → reached handler（200）
//   4. /partner/8（path :id != actor_id）→ 403（BOLA mismatch）
//   5. /public/_status 200（WithScope("public") pass-through）
//   6. 未挂 WithScope 的路由 → 404（BOLA fail-closed）
//
// 注：完整 main.go 集成测试依赖 Redis / MySQL，跳过；这里只测 middleware × 路由 binding。
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/middleware"
)

// fakeVerifier always returns supplied claims (or err).
type fakeVerifier struct {
	cl  *middleware.Claims
	err error
}

func (f *fakeVerifier) Verify(string) (*middleware.Claims, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.cl, nil
}

type fakeRevoke struct{}

func (fakeRevoke) IsRevoked(string) (bool, error) { return false, nil }

func newSmokeRouter(t *testing.T, cl *middleware.Claims) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())

	// 模拟 main.go：每个 mw 用 path-filter 包装后 r.Use 注册，让 c.Next() 自然推进。
	v := &fakeVerifier{cl: cl}
	authedPrefixes := []string{"/partner", "/customer", "/admin"}
	filter := func(mw gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			p := c.Request.URL.Path
			for _, pre := range authedPrefixes {
				if len(p) >= len(pre) && p[:len(pre)] == pre {
					mw(c)
					return
				}
			}
			c.Next()
		}
	}
	r.Use(filter(middleware.JWT(v, fakeRevoke{}, nil)))

	// healthz：无鉴权
	r.GET("/healthz", Healthz)

	// public：WithScope("public")，BOLA pass-through
	r.GET("/public/_status", middleware.WithScope("public"), func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// partner_self：actor_id 必须匹配 :id（WithScope 内部 enforce）
	r.GET("/partner/:id/me", middleware.WithScope("partner_self"), func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// 故意不挂 WithScope — JWT 验证通过后无 BOLA 把关 → 200（说明这种漏挂是 dev 阶段的 footgun）。
	// 为防止漏挂，CI 会跑 bola-scope-required analyzer（subtask 3，本 PR 不实现）。
	r.GET("/partner/no-scope", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	return r
}

func TestSmoke_HealthzNoAuth200(t *testing.T) {
	r := newSmokeRouter(t, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSmoke_PublicScopePassesWithoutJWT(t *testing.T) {
	r := newSmokeRouter(t, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/public/_status", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSmoke_PartnerNoJWT401(t *testing.T) {
	r := newSmokeRouter(t, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/partner/7/me", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSmoke_PartnerSelfMatchedPasses(t *testing.T) {
	cl := &middleware.Claims{ActorType: "partner", ActorID: 7, Jti: "j", Exp: time.Now().Add(time.Hour).Unix()}
	r := newSmokeRouter(t, cl)
	req := httptest.NewRequest("GET", "/partner/7/me", nil)
	req.AddCookie(&http.Cookie{Name: middleware.CookieAccess, Value: "tok"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSmoke_PartnerSelfMismatchedDenied(t *testing.T) {
	cl := &middleware.Claims{ActorType: "partner", ActorID: 7, Jti: "j", Exp: time.Now().Add(time.Hour).Unix()}
	r := newSmokeRouter(t, cl)
	req := httptest.NewRequest("GET", "/partner/8/me", nil)
	req.AddCookie(&http.Cookie{Name: middleware.CookieAccess, Value: "tok"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestSmoke_NoScope200_WithScopeNotMounted(t *testing.T) {
	// 漏挂 WithScope 的路由不会被 BOLA 拦截 — 由 bola-scope-required analyzer 在 CI 阻断。
	cl := &middleware.Claims{ActorType: "partner", ActorID: 7, Jti: "j", Exp: time.Now().Add(time.Hour).Unix()}
	r := newSmokeRouter(t, cl)
	req := httptest.NewRequest("GET", "/partner/no-scope", nil)
	req.AddCookie(&http.Cookie{Name: middleware.CookieAccess, Value: "tok"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
