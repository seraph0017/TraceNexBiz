// Package tracing 提供 trace_id 生成 + ctx 透传（backend §12.4）.
package tracing

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey int

const traceCtxKey ctxKey = 0

// FromContext 取 trace_id；缺失返 ""。
func FromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceCtxKey).(string); ok {
		return v
	}
	return ""
}

// WithTraceID 在 ctx 注入 trace_id（不存在时新建）.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	if traceID == "" {
		traceID = uuid.NewString()
	}
	return context.WithValue(ctx, traceCtxKey, traceID)
}
