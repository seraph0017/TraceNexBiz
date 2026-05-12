package middleware

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestIDInjects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/x", func(c *gin.Context) {
		assert.NotEmpty(t, TraceIDFrom(c))
		c.Status(204)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 204, w.Code)
	assert.NotEmpty(t, w.Header().Get(HeaderTraceID))
}

func TestSecurityHeadersSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityHeaders())
	r.GET("/", func(c *gin.Context) { c.Status(204) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.NotEmpty(t, w.Header().Get("Content-Security-Policy"))
}

// ---------- helpers ----------

// fakeVerifier 用 HS-style 伪签名（仅 token == "good"/"bad" 判断）。
type fakeVerifier struct {
	claims *Claims
	err    error
}

func (f *fakeVerifier) Verify(token string) (*Claims, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.claims, nil
}

type fakeRevoke struct {
	revoked bool
	err     error
}

func (f *fakeRevoke) IsRevoked(jti string) (bool, error) {
	return f.revoked, f.err
}

func mustRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	c := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	require.NoError(t, c.Ping(context.Background()).Err())
	t.Cleanup(func() { _ = c.Close(); mr.Close() })
	return c, mr
}

// ---------- JWT ----------

func TestJWT_NoTokenUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(JWT(&fakeVerifier{err: errors.New("nope")}, &fakeRevoke{}, nil))
	r.GET("/p", func(c *gin.Context) { c.Status(200) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/p", nil))
	assert.Equal(t, 401, w.Code)
}

func TestJWT_HappyCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cl := &Claims{Sub: 1, ActorType: "partner", ActorID: 42, Jti: "j1", Exp: time.Now().Add(time.Hour).Unix()}
	r := gin.New()
	r.Use(JWT(&fakeVerifier{claims: cl}, &fakeRevoke{}, nil))
	r.GET("/p", func(c *gin.Context) {
		got, _ := ClaimsFrom(c)
		assert.Equal(t, int64(42), got.ActorID)
		c.Status(204)
	})
	req := httptest.NewRequest("GET", "/p", nil)
	req.AddCookie(&http.Cookie{Name: CookieAccess, Value: "tok"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 204, w.Code)
}

func TestJWT_RevokedUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cl := &Claims{Jti: "x", Exp: time.Now().Add(time.Hour).Unix()}
	r := gin.New()
	r.Use(JWT(&fakeVerifier{claims: cl}, &fakeRevoke{revoked: true}, nil))
	r.GET("/p", func(c *gin.Context) { c.Status(200) })
	req := httptest.NewRequest("GET", "/p", nil)
	req.AddCookie(&http.Cookie{Name: CookieAccess, Value: "tok"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}

func TestJWT_RevokeStoreErrorFailsClosed503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cl := &Claims{Jti: "x", Exp: time.Now().Add(time.Hour).Unix()}
	r := gin.New()
	r.Use(JWT(&fakeVerifier{claims: cl}, &fakeRevoke{err: errors.New("redis down")}, nil))
	r.GET("/p", func(c *gin.Context) { c.Status(200) })
	req := httptest.NewRequest("GET", "/p", nil)
	req.AddCookie(&http.Cookie{Name: CookieAccess, Value: "tok"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 503, w.Code)
}

func TestJWT_SDKPathPrefersBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cl := &Claims{ActorType: "partner", ActorID: 1, Jti: "j", Exp: time.Now().Add(time.Hour).Unix()}
	v := &fakeVerifier{claims: cl}
	r := gin.New()
	r.Use(JWT(v, &fakeRevoke{}, nil))
	r.GET("/api/sdk/ping", func(c *gin.Context) { c.Status(200) })
	req := httptest.NewRequest("GET", "/api/sdk/ping", nil)
	req.Header.Set("Authorization", "Bearer abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}

func TestJWT_ExpiredUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cl := &Claims{Exp: time.Now().Add(-time.Hour).Unix()}
	r := gin.New()
	r.Use(JWT(&fakeVerifier{claims: cl}, &fakeRevoke{}, nil))
	r.GET("/p", func(c *gin.Context) { c.Status(200) })
	req := httptest.NewRequest("GET", "/p", nil)
	req.AddCookie(&http.Cookie{Name: CookieAccess, Value: "tok"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}

// ---------- RSAVerifier round-trip ----------

func TestRSAVerifier_RoundTrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})

	v, err := NewRSAVerifier(pemBytes)
	require.NoError(t, err)

	cl := Claims{Sub: 7, ActorType: "partner", ActorID: 7, Jti: "abc", Exp: time.Now().Add(time.Hour).Unix(), Iat: time.Now().Unix()}
	token := signRS256(t, priv, cl)
	got, err := v.Verify(token)
	require.NoError(t, err)
	assert.Equal(t, int64(7), got.ActorID)

	// 篡改 payload
	bad := token[:len(token)-2] + "xx"
	_, err = v.Verify(bad)
	assert.Error(t, err)

	// alg=none 拒绝
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	body := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":1}`))
	_, err = v.Verify(hdr + "." + body + ".")
	assert.Error(t, err)
}

func signRS256(t *testing.T, priv *rsa.PrivateKey, cl Claims) string {
	t.Helper()
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	pb, _ := json.Marshal(cl)
	body := base64.RawURLEncoding.EncodeToString(pb)
	signing := hdr + "." + body
	h := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, h[:])
	require.NoError(t, err)
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// ---------- CSRF ----------

func TestCSRF_GetPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CSRF())
	r.GET("/", func(c *gin.Context) { c.Status(204) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	assert.Equal(t, 204, w.Code)
}

func TestCSRF_PostMissingDenies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CSRF())
	r.POST("/x", func(c *gin.Context) { c.Status(204) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/x", nil))
	assert.Equal(t, 403, w.Code)
}

func TestCSRF_PostMatchingTokenPasses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CSRF())
	r.POST("/x", func(c *gin.Context) { c.Status(204) })
	tok := strings.Repeat("a", 32)
	req := httptest.NewRequest("POST", "/x", nil)
	req.AddCookie(&http.Cookie{Name: CookieCSRF, Value: tok})
	req.Header.Set(HeaderCSRF, tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 204, w.Code)
}

func TestCSRF_PostMismatchDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CSRF())
	r.POST("/x", func(c *gin.Context) { c.Status(204) })
	req := httptest.NewRequest("POST", "/x", nil)
	req.AddCookie(&http.Cookie{Name: CookieCSRF, Value: strings.Repeat("a", 32)})
	req.Header.Set(HeaderCSRF, strings.Repeat("b", 32))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 403, w.Code)
}

func TestCSRF_InternalAndPublicSkipped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CSRF())
	r.POST("/api/internal/foo", func(c *gin.Context) { c.Status(204) })
	r.POST("/api/public/bar", func(c *gin.Context) { c.Status(204) })
	r.POST("/public/baz", func(c *gin.Context) { c.Status(204) })
	r.POST("/webhook/zap", func(c *gin.Context) { c.Status(204) })
	for _, p := range []string{"/api/internal/foo", "/api/public/bar", "/public/baz", "/webhook/zap"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", p, nil))
		assert.Equalf(t, 204, w.Code, "path %s", p)
	}
}

// ---------- BOLAScope ----------

func TestBOLA_NoScopeReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BOLAScope(nil))
	r.GET("/p", func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/p", nil))
	assert.Equal(t, 404, w.Code)
}

func TestBOLA_PartnerSelfPasses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cl := &Claims{ActorType: "partner", ActorID: 7}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(CtxKeyJWTClaims, cl); c.Next() })
	r.GET("/partner/:id", WithScope("partner_self"), BOLAScope(nil), func(c *gin.Context) { c.Status(204) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/partner/7", nil))
	assert.Equal(t, 204, w.Code)
}

func TestBOLA_PartnerSelfMismatchDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cl := &Claims{ActorType: "partner", ActorID: 7}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(CtxKeyJWTClaims, cl); c.Next() })
	r.GET("/partner/:id", WithScope("partner_self"), BOLAScope(nil), func(c *gin.Context) { c.Status(204) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/partner/8", nil))
	assert.Equal(t, 403, w.Code)
}

func TestBOLA_WrongActorTypeDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cl := &Claims{ActorType: "customer", ActorID: 7}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(CtxKeyJWTClaims, cl); c.Next() })
	r.GET("/partner/:id", WithScope("partner_self"), BOLAScope(nil), func(c *gin.Context) { c.Status(204) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/partner/7", nil))
	assert.Equal(t, 403, w.Code)
}

func TestBOLA_StaffScopePasses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cl := &Claims{ActorType: "staff", ActorID: 1}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(CtxKeyJWTClaims, cl); c.Next() })
	r.GET("/admin/op", WithScope("staff_finance"), BOLAScope(nil), func(c *gin.Context) { c.Status(204) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/admin/op", nil))
	assert.Equal(t, 204, w.Code)
}

// ---------- Idempotency ----------

func TestIdem_GETSkipped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	rds, _ := mustRedis(t)
	r.Use(Idempotency(rds, time.Hour))
	r.GET("/x", func(c *gin.Context) { c.Status(204) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	assert.Equal(t, 204, w.Code)
}

func TestIdem_MissingKey400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	rds, _ := mustRedis(t)
	r.Use(Idempotency(rds, time.Hour))
	r.POST("/x", func(c *gin.Context) { c.Status(204) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/x", nil))
	assert.Equal(t, 400, w.Code)
}

func TestIdem_ReplayReturnsSameBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rds, _ := mustRedis(t)
	count := 0
	r := gin.New()
	r.Use(Idempotency(rds, time.Hour))
	r.POST("/x", func(c *gin.Context) {
		count++
		c.JSON(200, gin.H{"ok": true, "n": count})
	})
	key := "11111111-1111-1111-1111-111111111111"
	mk := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/x", bytes.NewReader([]byte(`{}`)))
		req.Header.Set(HeaderIdemKey, key)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	w1 := mk()
	require.Equal(t, 200, w1.Code)
	w2 := mk()
	assert.Equal(t, 200, w2.Code)
	assert.Equal(t, w1.Body.String(), w2.Body.String())
	assert.Equal(t, "1", w2.Header().Get(HeaderIdemReplay))
	assert.Equal(t, 1, count, "handler should run exactly once")
}

func TestIdem_RedisDown503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mr, err := miniredis.Run()
	require.NoError(t, err)
	rds := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1, DialTimeout: 200 * time.Millisecond})
	mr.Close() // kill it
	r := gin.New()
	r.Use(Idempotency(rds, time.Hour))
	r.POST("/x", func(c *gin.Context) { c.Status(204) })
	req := httptest.NewRequest("POST", "/x", nil)
	req.Header.Set(HeaderIdemKey, "11111111-1111-1111-1111-111111111111")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 503, w.Code)
}

// ---------- WebhookIdempotency ----------

func TestWebhookIdem_FirstPasses_SecondAlreadyProcessed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rds, _ := mustRedis(t)
	count := 0
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(CtxKeyWebhookProvider, "wechat"); c.Next() })
	r.Use(WebhookIdempotency(rds, time.Hour))
	r.POST("/webhook/wechat", func(c *gin.Context) {
		count++
		c.JSON(200, gin.H{"processed": true})
	})
	mk := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/webhook/wechat", nil)
		req.Header.Set(HeaderIdemKey, "evt-1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	w1 := mk()
	assert.Equal(t, 200, w1.Code)
	w2 := mk()
	assert.Equal(t, 200, w2.Code)
	assert.Contains(t, w2.Body.String(), "already_processed")
	assert.Equal(t, 1, count)
}

func TestWebhookIdem_MissingEventID400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rds, _ := mustRedis(t)
	r := gin.New()
	r.Use(WebhookIdempotency(rds, time.Hour))
	r.POST("/webhook/wechat", func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/webhook/wechat", nil))
	assert.Equal(t, 400, w.Code)
}

// ---------- Audit ----------

type capturingSink struct {
	mu      sync.Mutex
	entries []AuditEntry
}

func (s *capturingSink) Send(e AuditEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
}

func TestAudit_MutationCaptured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := &capturingSink{}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(CtxKeyActorType, "partner")
		c.Set(CtxKeyActorID, int64(42))
		c.Next()
	})
	r.Use(Audit(sink))
	r.POST("/x", func(c *gin.Context) { c.Status(200) })
	req := httptest.NewRequest("POST", "/x", bytes.NewReader([]byte(`{"phone":"13800000000"}`)))
	req.ContentLength = int64(len(`{"phone":"13800000000"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	require.Len(t, sink.entries, 1)
	e := sink.entries[0]
	assert.Equal(t, "partner", e.ActorType)
	assert.Equal(t, int64(42), e.ActorID)
	assert.Equal(t, "POST", e.Method)
	assert.NotContains(t, string(e.BodyRedacted), "13800000000")
}

func TestAudit_GETNotCaptured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := &capturingSink{}
	r := gin.New()
	r.Use(Audit(sink))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	assert.Equal(t, 200, w.Code)
	assert.Empty(t, sink.entries)
}

func TestAudit_BufferedSinkDrops(t *testing.T) {
	called := 0
	s := NewBufferedSink(1, func(AuditEntry) { called++ })
	before := AuditDropsTotal.Load()
	s.Send(AuditEntry{})
	s.Send(AuditEntry{}) // overflow → drop
	s.Send(AuditEntry{})
	s.Close()
	<-s.Drained()
	assert.GreaterOrEqual(t, AuditDropsTotal.Load()-before, int64(1))
	assert.GreaterOrEqual(t, called, 1)
}

// ---------- PIIScrubber ----------

func TestPII_StaffScrubbed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cl := &Claims{ActorType: "staff", Roles: []string{"staff_support"}}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(CtxKeyJWTClaims, cl); c.Next() })
	r.Use(PIIScrubber())
	r.GET("/x", func(c *gin.Context) {
		c.JSON(200, gin.H{"phone": "13800000000", "email": "a@b.com"})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	assert.NotContains(t, w.Body.String(), "13800000000")
	assert.NotContains(t, w.Body.String(), "a@b.com")
}

func TestPII_PartnerNotScrubbed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cl := &Claims{ActorType: "partner"}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(CtxKeyJWTClaims, cl); c.Next() })
	r.Use(PIIScrubber())
	r.GET("/x", func(c *gin.Context) {
		c.JSON(200, gin.H{"phone": "13800000000"})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	assert.Contains(t, w.Body.String(), "13800000000")
}

func TestPII_StaffWithViewFullBypass(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cl := &Claims{ActorType: "staff", Roles: []string{VerbPIIViewFull}}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(CtxKeyJWTClaims, cl); c.Next() })
	r.Use(PIIScrubber())
	r.GET("/x", func(c *gin.Context) { c.JSON(200, gin.H{"phone": "13800000000"}) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	assert.Contains(t, w.Body.String(), "13800000000")
}

// extra smoke: ensure JWT->BOLA chain works end-to-end
func TestChain_JWTThenBOLA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cl := &Claims{ActorType: "partner", ActorID: 7, Jti: "j", Exp: time.Now().Add(time.Hour).Unix()}
	r := gin.New()
	r.Use(JWT(&fakeVerifier{claims: cl}, &fakeRevoke{}, nil))
	r.GET("/partner/:id/wallet", WithScope("partner_self"), BOLAScope(nil), func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})
	req := httptest.NewRequest("GET", "/partner/7/wallet", nil)
	req.AddCookie(&http.Cookie{Name: CookieAccess, Value: "tok"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	// path mismatch → 403
	req2 := httptest.NewRequest("GET", "/partner/8/wallet", nil)
	req2.AddCookie(&http.Cookie{Name: CookieAccess, Value: "tok"})
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, 403, w2.Code)
}

// guard against accidental import shrink
var _ = fmt.Sprintf
