DELETE FROM `biz_setting` WHERE `key` IN (
    'compliance.icp_record_no','compliance.icp_license_no','compliance.public_security_filing_no',
    'compliance.gen_ai_filing_no','compliance.algorithm_filing_no','compliance.deep_synthesis_filing_no',
    'compliance.dpo_contact_email','compliance.dpo_contact_phone','compliance.report_phone_12377_link',
    'compliance.icp_license_active','compliance.gen_ai_filing_active','compliance.algorithm_filing_active',
    'compliance.deep_synthesis_filing_active','compliance.epd_2_filing_active','compliance.licensed_provider_active',
    'compliance.pia_report_latest_at','jwt_verify_key_pem','refund_window_days','saga_wall_clock_hours',
    'idempotency_ttl_hours','internal_idempotency_ttl_days','payment.platform_isv_mchid','cors_origins'
);
