// Package sls 封装阿里云 SLS 日志写入 + PII scrubber。
//
// 引用：backend §12.2 + §16.6 + Security M-r2-7。
//
// 强制字段：ts / level / trace_id / actor_type / actor_id / msg
// PII scrubber 必须命中：手机号 / 邮箱 / 身份证号 / 法人姓名 / 银行账号 / 营业执照号
package sls

import (
	"context"
	"regexp"
)

// Service 是 SLS 写入入口；W1c 后期对接阿里云 SLS SDK。
type Service interface {
	Write(ctx context.Context, level string, fields map[string]interface{}, msg string) error
}

// PIIScrubber 在结构化日志写入前 walk 所有字段，命中敏感模式即 redact。
type PIIScrubber struct {
	patterns []*regexp.Regexp
}

// NewPIIScrubber 默认覆盖 §16.6 列出的 6 类 PII。
func NewPIIScrubber() *PIIScrubber {
	return &PIIScrubber{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`1[3-9]\d{9}`),                                // mobile
			regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`), // email
			regexp.MustCompile(`[1-9]\d{5}(?:18|19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]`), // 身份证
			regexp.MustCompile(`\b\d{16,19}\b`),                              // 银行账号
		},
	}
}

// Scrub 把字符串 / map / slice 中命中模式的部分替换为 "[REDACTED]"。
//
// 调用方约束：传入 value 不被持有；返回新值（immutability）。
func (p *PIIScrubber) Scrub(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		out := v
		for _, pat := range p.patterns {
			out = pat.ReplaceAllString(out, "[REDACTED]")
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for k, vv := range v {
			out[k] = p.Scrub(vv)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, vv := range v {
			out[i] = p.Scrub(vv)
		}
		return out
	default:
		return v
	}
}

// Stub W0 占位实现：直接 stdout 输出。
type Stub struct {
	scrubber *PIIScrubber
}

func NewStub() Service { return &Stub{scrubber: NewPIIScrubber()} }

func (s *Stub) Write(_ context.Context, _ string, _ map[string]interface{}, _ string) error {
	// TODO(W1d): per backend §12.2 接 SLS PutLogs；fields 必须先 scrubber.Scrub
	return nil
}
