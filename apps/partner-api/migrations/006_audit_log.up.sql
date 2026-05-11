-- 006_audit_log.up.sql
-- 审计哈希链：audit_log_unsealed (app INSERT 队列) + audit_log (sealer 哈希链最终表) + audit_log_pii (PIPL 删除)
-- 引用：backend §3.13 + ADR-006 + Security HIGH-r2-1

CREATE TABLE IF NOT EXISTS `audit_log_unsealed` (
    `id`                  BIGINT NOT NULL AUTO_INCREMENT,
    `actor_type`          VARCHAR(16) NOT NULL,
    `actor_id`            BIGINT NOT NULL,
    `action`              VARCHAR(64) NOT NULL,
    `target_type`         VARCHAR(32) NOT NULL,
    `target_id`           BIGINT NOT NULL DEFAULT 0,
    `target_key`          VARCHAR(128) NOT NULL DEFAULT '' COMMENT 'v0.2 SEC HIGH-12 string target',
    `diff_redacted`       TEXT,
    `diff_pii_id`         BIGINT NULL,
    `ip_address`          VARCHAR(64),
    `user_agent`          VARCHAR(512),
    `trace_id`            VARCHAR(64) NOT NULL DEFAULT '',
    `second_approver_id`  BIGINT NULL COMMENT 'v0.2 SEC CRIT-5 dual-control',
    `occurred_at`         TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='应用 INSERT 队列；sealer 消费 (Phase 1)';

-- audit_log.id 非 AUTO_INCREMENT；由 sealer 把 unsealed.id 原样拷贝过来
CREATE TABLE IF NOT EXISTS `audit_log` (
    `id`                  BIGINT NOT NULL,
    `actor_type`          VARCHAR(16) NOT NULL,
    `actor_id`            BIGINT NOT NULL,
    `action`              VARCHAR(64) NOT NULL,
    `target_type`         VARCHAR(32) NOT NULL,
    `target_id`           BIGINT NOT NULL DEFAULT 0,
    `target_key`          VARCHAR(128) NOT NULL DEFAULT '',
    `diff_redacted`       TEXT,
    `diff_pii_id`         BIGINT NULL,
    `ip_address`          VARCHAR(64),
    `user_agent`          VARCHAR(512),
    `trace_id`            VARCHAR(64) NOT NULL DEFAULT '',
    `second_approver_id`  BIGINT NULL,
    `occurred_at`         TIMESTAMP(3) NOT NULL,
    `prev_hash`           CHAR(64) NOT NULL,
    `self_hash`           CHAR(64) NOT NULL,
    `sealed_at`           TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    KEY `idx_audit_actor` (`actor_type`, `actor_id`, `occurred_at`),
    KEY `idx_audit_target` (`target_type`, `target_id`, `occurred_at`),
    KEY `idx_audit_target_key` (`target_type`, `target_key`, `occurred_at`),
    KEY `idx_audit_action` (`action`, `occurred_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='追加只写 + 哈希链 (Phase 1)';

CREATE TABLE IF NOT EXISTS `audit_log_pii` (
    `id`                BIGINT NOT NULL AUTO_INCREMENT,
    `diff_cipher`       VARBINARY(65535) NOT NULL COMMENT 'v0.2 Compliance M-19 64KB',
    `diff_oss_ref`      VARCHAR(1024) COMMENT '超长 PII OSS 引用',
    `encryption_key_id` VARCHAR(128) NOT NULL,
    `tombstoned_at`     TIMESTAMP(3) NULL COMMENT 'PIPL §47 删除后置位',
    `created_at`        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='PII 加密侧表，可 PIPL 删除而不破坏哈希链 (Phase 1)';
