-- 007_staff_biz_setting_idem_saga.up.sql
-- 平台 staff + biz_setting + idempotency_record + saga_step + password_reset_token
-- 引用：backend §3.14 / §3.15 / §3.16 / §3.17 / §3.28

CREATE TABLE IF NOT EXISTS `staff` (
    `id`                  BIGINT NOT NULL AUTO_INCREMENT,
    `username`            VARCHAR(64) NOT NULL,
    `password_hash`       VARCHAR(255) NOT NULL COMMENT 'argon2id',
    `role`                VARCHAR(32) NOT NULL,
    `email`               VARCHAR(128) NOT NULL,
    `status`              VARCHAR(16) NOT NULL DEFAULT 'active',
    `last_login`          TIMESTAMP(3) NULL,
    `mfa_secret_cipher`   VARBINARY(512),
    `mfa_secret_key_id`   VARCHAR(128),
    `webauthn_creds`      JSON,
    `elevated_until`      TIMESTAMP(3) NULL COMMENT 'step-up MFA 单次授权窗口（≤15min）',
    `created_at`          TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`          TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    `deleted_at`          TIMESTAMP(3) NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_staff_username` (`username`),
    KEY `idx_staff_role` (`role`, `status`),
    CONSTRAINT `chk_staff_role` CHECK (`role` IN ('super_admin','operations','finance','support')),
    CONSTRAINT `chk_staff_status` CHECK (`status` IN ('active','disabled','locked'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='平台 staff (Phase 1)';

CREATE TABLE IF NOT EXISTS `biz_setting` (
    `key`         VARCHAR(128) NOT NULL,
    `value`       TEXT NOT NULL,
    `value_type`  VARCHAR(16) NOT NULL DEFAULT 'plain' COMMENT 'v0.2 SEC CRIT-7',
    `description` VARCHAR(512),
    `updated_at`  TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    `updated_by`  BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (`key`),
    CONSTRAINT `chk_biz_setting_value_type` CHECK (`value_type` IN ('plain','secret_ref'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='业务运行参数 + 合规公示 + secret_ref (Phase 1)';

CREATE TABLE IF NOT EXISTS `idempotency_record` (
    `id`                BIGINT NOT NULL AUTO_INCREMENT,
    `actor_type`        VARCHAR(16) NOT NULL,
    `actor_id`          BIGINT NOT NULL,
    `idempotency_key`   VARCHAR(64) NOT NULL,
    `endpoint`          VARCHAR(128) NOT NULL,
    `request_hash`      CHAR(64) NOT NULL,
    `response_status`   INT NOT NULL,
    `response_hash`     CHAR(64) NOT NULL,
    `response_cipher`   VARBINARY(16384) NOT NULL COMMENT 'KMS 加密 response (SEC M-r2-5)',
    `response_key_id`   VARCHAR(128) NOT NULL,
    `trace_id`          VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'v1.0 cosmetic #1 / M-4',
    `created_at`        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `expires_at`        TIMESTAMP(3) NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_idem` (`actor_type`, `actor_id`, `idempotency_key`, `endpoint`),
    KEY `idx_idem_expires` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='idempotency_record TTL 24h (Phase 1)';

CREATE TABLE IF NOT EXISTS `saga_step` (
    `id`                BIGINT NOT NULL AUTO_INCREMENT,
    `saga_id`           VARCHAR(64) NOT NULL,
    `step_name`         VARCHAR(64) NOT NULL,
    `status`            VARCHAR(32) NOT NULL DEFAULT 'pending',
    `attempts`          INT NOT NULL DEFAULT 0,
    `last_error`        TEXT,
    `payload`           TEXT COMMENT 'JSON; PII scrubber 必须命中',
    `started_at`        TIMESTAMP(3) NULL,
    `updated_at`        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    `escalated_at`      TIMESTAMP(3) NULL,
    `escalate_reason`   TEXT,
    `trace_id`          VARCHAR(64) NOT NULL DEFAULT '',
    `created_at`        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    KEY `idx_saga_id` (`saga_id`),
    KEY `idx_saga_status` (`status`, `updated_at`),
    UNIQUE KEY `uk_saga_step` (`saga_id`, `step_name`),
    CONSTRAINT `chk_saga_status` CHECK (`status` IN ('pending','in_progress','committed','compensated','failed','escalated','released_pessimistic'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='saga_step (Phase 1, ADR-013 UNIQUE)';

CREATE TABLE IF NOT EXISTS `password_reset_token` (
    `id`                  BIGINT NOT NULL AUTO_INCREMENT,
    `actor_type`          VARCHAR(16) NOT NULL,
    `actor_id`            BIGINT NOT NULL,
    `token_hash`          CHAR(64) NOT NULL COMMENT 'SHA-256(随机 32 byte)',
    `second_factor_type`  VARCHAR(16) NOT NULL,
    `second_factor_hash`  CHAR(64) NOT NULL,
    `requested_ip`        VARCHAR(45) NOT NULL,
    `user_agent`          VARCHAR(512),
    `expires_at`          TIMESTAMP(3) NOT NULL COMMENT '15 min TTL',
    `consumed_at`         TIMESTAMP(3) NULL,
    `failed_attempts`     INT NOT NULL DEFAULT 0,
    `invalidated_at`      TIMESTAMP(3) NULL,
    `audit_log_id`        BIGINT NULL,
    `trace_id`            VARCHAR(64) NOT NULL DEFAULT '',
    `created_at`          TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_prt_token_hash` (`token_hash`),
    KEY `idx_prt_actor` (`actor_type`, `actor_id`, `consumed_at`),
    KEY `idx_prt_expiry` (`expires_at`),
    CONSTRAINT `chk_prt_actor_type` CHECK (`actor_type` IN ('partner','customer','staff')),
    CONSTRAINT `chk_prt_factor_type` CHECK (`second_factor_type` IN ('email','sms'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='密码重置 token (Phase 1, PRD §17.5 双因子)';
