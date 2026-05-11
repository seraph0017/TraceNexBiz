// Package handler 是 partner-api HTTP 边界层（W0 scaffold）。
// W1a/W1b/W1c 在此目录下增补 partner / customer / admin / webhook 四类 router group。
//
// 当前仅落 healthz 与 TODO router 占位，确保 partner-api build & start 通过。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Healthz 简易存活（兼容 K8s liveness）。
func Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// HealthzLive backend §13.3 — 进程活；返 200 直到 SIGTERM。
func HealthzLive(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "live"})
}

// HealthzReady backend §13.3 — 依赖检查全过才 200。
//
// W1a 实现：DB ping / Redis ping / KMS / OSS / Fy-api /api/internal/health / SLS reachable。
// 30s 缓存避免 readiness 抖动。
func HealthzReady(c *gin.Context) {
	// TODO(W1a): per backend §13.3 — multi-dep readiness probe with 30s cache.
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}
