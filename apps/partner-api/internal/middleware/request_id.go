// Package middleware - request_id.go：trace_id 注入 / 跨进程透传。
//
// 引用：overview §4.4 + backend §12.4。
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// HeaderTraceID 全链路 trace 透传 header（与 Fy-api 既有约定一致）。
const HeaderTraceID = "X-Oneapi-Request-Id"

// CtxKeyTraceID gin.Context 中存放 trace_id 的 key。
const CtxKeyTraceID = "trace_id"

// RequestID 注入 trace_id：缺失时生成 UUID（per overview §4.4）。
//
// 跨进程透传由 fyapi.Client / repository / audit_log 等显式读 ctx 后写入下游。
// W1c：切 UUIDv7（time-ordered）以便追溯。
func RequestID() gin.HandlerFunc {
	return func(gc *gin.Context) {
		tid := gc.GetHeader(HeaderTraceID)
		if tid == "" {
			tid = uuid.NewString()
		}
		gc.Set(CtxKeyTraceID, tid)
		gc.Header(HeaderTraceID, tid)
		gc.Next()
	}
}

// TraceIDFrom 从 gin.Context 取 trace_id；调用方 service / repository 透传给 Fy-api。
func TraceIDFrom(gc *gin.Context) string {
	if v, ok := gc.Get(CtxKeyTraceID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
