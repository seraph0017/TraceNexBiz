-- 011_seed_biz_setting.up.sql
-- 种子 biz_setting：合规公示 + readiness gate + secret_ref + 业务参数
-- 引用：backend §3.15 表注释 + overview §8.5 + ADR-007 v0.2 / SEC CRIT-7

INSERT INTO `biz_setting` (`key`, `value`, `value_type`, `description`) VALUES
    -- 9 个合规公示 key（ComplianceFooter 消费）
    ('compliance.icp_record_no', '', 'plain', 'ICP 备案号 (Compliance CRIT-1)'),
    ('compliance.icp_license_no', '', 'plain', 'ICP 经营许可证号'),
    ('compliance.public_security_filing_no', '', 'plain', '公网安备号'),
    ('compliance.gen_ai_filing_no', '', 'plain', '生成式 AI 服务提供者备案号'),
    ('compliance.algorithm_filing_no', '', 'plain', '算法备案号'),
    ('compliance.deep_synthesis_filing_no', '', 'plain', '深度合成备案号（条件触发）'),
    ('compliance.dpo_contact_email', '', 'plain', 'DPO 邮箱（PIPL §52）'),
    ('compliance.dpo_contact_phone', '', 'plain', 'DPO 电话'),
    ('compliance.report_phone_12377_link', '', 'plain', '12377 + 公司专用举报通道'),

    -- readiness probe gate
    ('compliance.icp_license_active', 'false', 'plain', 'ICP 经营许可证生效'),
    ('compliance.gen_ai_filing_active', 'false', 'plain', '生成式 AI 备案生效'),
    ('compliance.algorithm_filing_active', 'false', 'plain', '算法备案生效'),
    ('compliance.deep_synthesis_filing_active', 'false', 'plain', '深合成备案'),
    ('compliance.epd_2_filing_active', 'false', 'plain', '等保 2.0 二级'),
    ('compliance.licensed_provider_active', 'false', 'plain', '持牌分账方合同'),
    ('compliance.pia_report_latest_at', '', 'plain', 'PIA 最新有效日期'),

    -- security-critical：value 仅存 KMS Secret ARN，实际 value 从 env 注入
    ('jwt_verify_key_pem', 'arn:aliyun:kms:cn-hangzhou:secret:tnbiz/jwt_verify_key_pem', 'secret_ref', 'JWT 公钥 ARN'),

    -- 业务运行参数
    ('refund_window_days', '7', 'plain', '退款窗口（默认 7）'),
    ('saga_wall_clock_hours', '1', 'plain', 'saga 上限（默认 1）'),
    ('idempotency_ttl_hours', '24', 'plain', '本地幂等 TTL（默认 24）'),
    ('internal_idempotency_ttl_days', '7', 'plain', 'Fy-api 内部幂等 TTL 天（默认 7）'),
    ('payment.platform_isv_mchid', '', 'plain', 'ISV 佣金 mchid（Compliance M-2）'),
    ('cors_origins', 'http://localhost:5173,http://localhost:5174,http://localhost:5175,http://localhost:5176', 'plain', 'CORS 白名单')
ON DUPLICATE KEY UPDATE `description` = VALUES(`description`);
