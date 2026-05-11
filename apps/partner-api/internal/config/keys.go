// Package config - keys.go：machine-readable biz_setting key 注册表。
//
// 引用：backend §3.15 + ADR D-3 / SEC CRIT-7。
//
// W1c：startup MustLoadAndValidate 必须 enumerate 此 Registry 并断言存在。
package config

// BizSettingSpec 是 machine-readable enum；任何 W1 agent 新增 key 必须先在此登记。
type BizSettingSpec struct {
	Key       string
	ValueType string // "plain" | "secret_ref"
	Phase     int    // 1 / 2 / 2A / 3
	Required  bool
	Desc      string
}

// 9 个合规公示 key（per overview §8.5）。
//
// frontend <ComplianceFooter> 消费此 9 个；任一为空则 storefront 拒绝构建（CI check）。
var ComplianceFooterKeys = []BizSettingSpec{
	{Key: "compliance.icp_record_no", ValueType: "plain", Phase: 2, Required: true, Desc: "ICP 备案号"},
	{Key: "compliance.icp_license_no", ValueType: "plain", Phase: 2, Required: true, Desc: "ICP 经营许可证号"},
	{Key: "compliance.public_security_filing_no", ValueType: "plain", Phase: 2, Required: true, Desc: "公网安备号"},
	{Key: "compliance.gen_ai_filing_no", ValueType: "plain", Phase: 2, Required: true, Desc: "生成式 AI 服务提供者备案号"},
	{Key: "compliance.algorithm_filing_no", ValueType: "plain", Phase: 2, Required: true, Desc: "算法备案号"},
	{Key: "compliance.deep_synthesis_filing_no", ValueType: "plain", Phase: 2, Required: false, Desc: "深度合成备案号（条件触发）"},
	{Key: "compliance.dpo_contact_email", ValueType: "plain", Phase: 1, Required: true, Desc: "DPO 邮箱（PIPL §52）"},
	{Key: "compliance.dpo_contact_phone", ValueType: "plain", Phase: 1, Required: true, Desc: "DPO 电话"},
	{Key: "compliance.report_phone_12377_link", ValueType: "plain", Phase: 2, Required: true, Desc: "12377 + 公司专用举报通道"},
}

// readiness probe gate（per overview §8.5 / readiness 表）。
var ComplianceGateKeys = []BizSettingSpec{
	{Key: "compliance.icp_license_active", ValueType: "plain", Phase: 2, Required: true, Desc: "ICP 经营许可证生效"},
	{Key: "compliance.gen_ai_filing_active", ValueType: "plain", Phase: 2, Required: true, Desc: "生成式 AI 备案生效"},
	{Key: "compliance.algorithm_filing_active", ValueType: "plain", Phase: 2, Required: true, Desc: "算法备案生效"},
	{Key: "compliance.deep_synthesis_filing_active", ValueType: "plain", Phase: 2, Required: false, Desc: "深合成备案"},
	{Key: "compliance.epd_2_filing_active", ValueType: "plain", Phase: 2, Required: true, Desc: "等保 2.0 二级"},
	{Key: "compliance.licensed_provider_active", ValueType: "plain", Phase: 2, Required: true, Desc: "持牌分账方合同"},
	{Key: "compliance.pia_report_latest_at", ValueType: "plain", Phase: 2, Required: false, Desc: "PIA 最新有效日期"},
}

// security-critical key（per ADR-007 v0.2 / SEC CRIT-7）。
//
// 写权限：system.config_write.security verb（dual-control + step-up MFA）。
// W1c：value_type=secret_ref；value 仅存 KMS Secret ARN，实际值从 env 注入。
var SecurityRefKeys = []BizSettingSpec{
	{Key: "jwt_verify_key_pem", ValueType: "secret_ref", Phase: 1, Required: true, Desc: "JWT 公钥 ARN（实际从 env JWT_VERIFY_KEY_PEM 注入）"},
}

// 业务运行参数。
var OperationalKeys = []BizSettingSpec{
	{Key: "refund_window_days", ValueType: "plain", Phase: 1, Required: true, Desc: "退款窗口（默认 7）"},
	{Key: "saga_wall_clock_hours", ValueType: "plain", Phase: 1, Required: true, Desc: "saga 上限（默认 1）"},
	{Key: "idempotency_ttl_hours", ValueType: "plain", Phase: 1, Required: true, Desc: "本地幂等 TTL（默认 24）"},
	{Key: "internal_idempotency_ttl_days", ValueType: "plain", Phase: 1, Required: true, Desc: "Fy-api 内部幂等 TTL 天（默认 7）"},
	{Key: "payment.platform_isv_mchid", ValueType: "plain", Phase: 2, Required: false, Desc: "ISV 佣金 mchid（Compliance M-2）"},
	{Key: "cors_origins", ValueType: "plain", Phase: 1, Required: true, Desc: "CORS 白名单（逗号分隔）"},
}

// Registry 汇总；W1c startup validator 遍历此切片。
var Registry = func() []BizSettingSpec {
	all := make([]BizSettingSpec, 0, len(ComplianceFooterKeys)+len(ComplianceGateKeys)+len(SecurityRefKeys)+len(OperationalKeys))
	all = append(all, ComplianceFooterKeys...)
	all = append(all, ComplianceGateKeys...)
	all = append(all, SecurityRefKeys...)
	all = append(all, OperationalKeys...)
	return all
}()
