// Package consent - Fix-C item 7：consent_text_version 校验.
//
// PIPL/合规：用户每次签署 ToS / 隐私政策 / KYC consent 时必须记录当时的 consent_text_version。
// 服务端在接受 signup / consent 写入时，必须断言客户端传入的 version 与
// `biz_setting.compliance.consent_text_version`（current 值）一致；否则拒绝（用户尚未看到
// 最新条款；前端必须刷新弹窗）.
//
// 用法：
//
//	v := consent.NewVersionGuard(cfg.BizSetting)
//	if err := v.Verify(submittedVersion); err != nil { reject }
//
// dev fallback：当 biz_setting 中未配置 current 值（空串）时，VersionGuard 接受任何非空
// version（包含 "v2.0"）；prod 必须配置.
package consent

import (
	"errors"
	"strings"
)

// ErrConsentVersionStale 客户端提交的 consent_text_version 与当前 biz_setting 不一致.
var ErrConsentVersionStale = errors.New("consent: client consent_text_version does not match current biz_setting (please re-accept ToS)")

// ErrConsentVersionEmpty 客户端未提交版本号.
var ErrConsentVersionEmpty = errors.New("consent: consent_text_version required")

// SettingProvider 抽象 biz_setting 访问；config.BizSettingConfig 自然满足.
type SettingProvider interface {
	Get(key string) string
}

// VersionGuard 持有 settings 引用 + lazy resolve 当前 version.
type VersionGuard struct {
	settings SettingProvider
	devMode  bool
}

// NewVersionGuard .
func NewVersionGuard(s SettingProvider) *VersionGuard {
	return &VersionGuard{settings: s}
}

// SetDevMode dev 模式：current 为空时不强求；prod 模式则严格.
func (g *VersionGuard) SetDevMode(b bool) { g.devMode = b }

// Current 返回当前 consent_text_version；空串说明未配置.
func (g *VersionGuard) Current() string {
	if g == nil || g.settings == nil {
		return ""
	}
	return strings.TrimSpace(g.settings.Get("compliance.consent_text_version"))
}

// Verify 校验客户端 submittedVersion ≡ current.
//
// 规则：
//   - submitted 空串 → ErrConsentVersionEmpty
//   - current 空串 + devMode → 接受 submitted（dev 兜底）
//   - current 空串 + !devMode → 拒绝（fail-closed；prod 必须配置）
//   - current ≠ submitted → ErrConsentVersionStale
func (g *VersionGuard) Verify(submittedVersion string) error {
	v := strings.TrimSpace(submittedVersion)
	if v == "" {
		return ErrConsentVersionEmpty
	}
	cur := g.Current()
	if cur == "" {
		if g.devMode {
			return nil
		}
		return ErrConsentVersionStale
	}
	if v != cur {
		return ErrConsentVersionStale
	}
	return nil
}
