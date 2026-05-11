package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
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
