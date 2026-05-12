// Audit middleware（backend §9 / §10.1 sealer + Security S-2）。
//
// 每个写操作（POST/PUT/DELETE/PATCH）的 2xx 响应入队 audit_log_unsealed。
// 队列由 buffered channel 实现，drop-on-overflow + 计数器；非阻塞。
//
// 设计要点：
//   - PII scrubbing：state-changing 路由的 body 摘要走 piiscrubber.Redact 再入队
//   - 不在响应链路上同步写 DB；service 层独立 sealer 进程消费
//   - dual-control 字段（second_approver_id）由 service 层在业务层显式写入；本 MW 不处理
package middleware

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/pkg/piiscrubber"
)

// AuditEntry 入队载荷（service 层 sealer 消费）。
type AuditEntry struct {
	ActorType    string
	ActorID      int64
	Method       string
	Path         string
	Status       int
	RequestID    string
	IP           string
	UserAgent    string
	BodyRedacted []byte
}

// AuditEnqueuer 注入 unsealed 队列（W1a 由 internal/audit.UnsealedRepo 提供）。
//
// 兼容旧签名（用于现有 sealer 测试），同时 *也* 接 AuditEntry-form 通过 EnqueueEntry 暴露。
type AuditEnqueuer interface {
	Enqueue(c *gin.Context, action string, targetType string, targetID int64, diffRedacted []byte) error
}

// AuditSink 是 W1a middleware 直接调用的非阻塞接收端。
//
// 用户可以提供 channel-backed 实现；middleware 已自带一个默认 buffered 实现。
type AuditSink interface {
	Send(e AuditEntry) // 非阻塞；超载时由实现决定 drop
}

// AuditDropsTotal 暴露 metric（main.go 注册 prometheus collector 时引用）。
var AuditDropsTotal atomic.Int64

// NewBufferedSink 创建一个 size 长度的 sink，启动 1 worker 调用 onFlush。
//
// drainDone 在 Close() 后 worker 真正退出时关闭，便于测试。
func NewBufferedSink(size int, onFlush func(AuditEntry)) *BufferedSink {
	if size <= 0 {
		size = 1024
	}
	s := &BufferedSink{ch: make(chan AuditEntry, size), done: make(chan struct{})}
	go func() {
		for e := range s.ch {
			onFlush(e)
		}
		close(s.done)
	}()
	return s
}

// BufferedSink 默认 channel-backed sink。
type BufferedSink struct {
	ch   chan AuditEntry
	done chan struct{}
}

// Send 非阻塞投递；满了 → drop + counter。
func (s *BufferedSink) Send(e AuditEntry) {
	select {
	case s.ch <- e:
	default:
		AuditDropsTotal.Add(1)
	}
}

// Close 关闭 channel；worker 处理完剩余条目后退出。
func (s *BufferedSink) Close() { close(s.ch) }

// Drained 测试用：等 worker 退出。
func (s *BufferedSink) Drained() <-chan struct{} { return s.done }

// Audit 装配请求成功后 enqueue 审计条目。
func Audit(sink AuditSink) gin.HandlerFunc {
	return func(c *gin.Context) {
		var bodyCopy []byte
		if isMutation(c.Request.Method) && c.Request.Body != nil && c.Request.ContentLength > 0 && c.Request.ContentLength < 64*1024 {
			b, err := io.ReadAll(c.Request.Body)
			if err == nil {
				bodyCopy = b
				c.Request.Body = io.NopCloser(bytes.NewReader(b))
			} else {
				_ = c.Error(err)
			}
		}
		c.Next()
		// 仅 2xx + mutation 写入；GET 也可以审计 staff_* 后台操作，由 service 层主动入队即可
		if !isMutation(c.Request.Method) {
			return
		}
		st := c.Writer.Status()
		if st < 200 || st >= 300 {
			return
		}
		actorType := c.GetString(CtxKeyActorType)
		aidNum, _ := c.Get(CtxKeyActorID)
		aid, _ := aidNum.(int64)
		redacted := bodyCopy
		if len(redacted) > 0 {
			redacted = []byte(piiscrubber.Redact(string(redacted)))
		}
		entry := AuditEntry{
			ActorType:    actorType,
			ActorID:      aid,
			Method:       c.Request.Method,
			Path:         c.Request.URL.Path,
			Status:       st,
			RequestID:    TraceIDFrom(c),
			IP:           c.ClientIP(),
			UserAgent:    c.Request.UserAgent(),
			BodyRedacted: redacted,
		}
		if sink != nil {
			sink.Send(entry)
		}
	}
}

// isMutation 仅 POST/PUT/PATCH/DELETE。
func isMutation(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// ErrAuditSinkNil 备用：调用方误传 nil 时 service 层应感知。
var ErrAuditSinkNil = errors.New("middleware: audit sink is nil")
