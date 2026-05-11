// Package piiscrubber 实现 PII 脱敏（backend §12.2 / §16.6）.
//
// 命中规则：
//   - struct field 含 `pii:"true"` tag
//   - 模式：身份证 18 位 / 中国手机号 / 邮箱 / 银行卡（W1a 完善）
//
// 三个挂点：
//   1. zerolog hook：log.Logger.Hook(piiscrubber.Hook{})
//   2. saga_step.payload 写入前
//   3. consume_log_outbox.last_error 写入前（Security MED-6）
package piiscrubber

import "regexp"

// 简单 baseline regex；W1a 扩充 + 测试矩阵（与 frontend §15.2 一致）.
var (
	rePhone = regexp.MustCompile(`1[3-9]\d{9}`)
	reIDCard = regexp.MustCompile(`\b\d{17}[\dXx]\b`)
	reEmail = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`)
)

// Redact 简易实现：身份证 / 手机号 / email 走 *** 替换。
//
// 注意顺序：身份证（18 位）必须先于手机号（11 位）替换，否则手机号正则会吃掉身份证前 11 位。
func Redact(s string) string {
	s = reIDCard.ReplaceAllString(s, "***-IDCARD-***")
	s = rePhone.ReplaceAllString(s, "***-PHONE-***")
	s = reEmail.ReplaceAllString(s, "***-EMAIL-***")
	return s
}
