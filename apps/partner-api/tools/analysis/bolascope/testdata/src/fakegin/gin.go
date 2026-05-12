// Package fakegin is a minimal stand-in for github.com/gin-gonic/gin used by
// the bolascope analyzer test. The real analyzer matches *RouterGroup /
// *Engine by type-name suffix, so this works.
package fakegin

type Context struct{}

type HandlerFunc func(*Context)

type RouterGroup struct{}

func (*RouterGroup) GET(path string, h ...HandlerFunc)     {}
func (*RouterGroup) POST(path string, h ...HandlerFunc)    {}
func (*RouterGroup) PUT(path string, h ...HandlerFunc)     {}
func (*RouterGroup) PATCH(path string, h ...HandlerFunc)   {}
func (*RouterGroup) DELETE(path string, h ...HandlerFunc)  {}
func (*RouterGroup) OPTIONS(path string, h ...HandlerFunc) {}
func (*RouterGroup) HEAD(path string, h ...HandlerFunc)    {}
func (*RouterGroup) Any(path string, h ...HandlerFunc)     {}
func (*RouterGroup) Handle(method, path string, h ...HandlerFunc) {}

type Engine struct{ RouterGroup }

func WithScope(scope string) HandlerFunc { return func(*Context) {} }
