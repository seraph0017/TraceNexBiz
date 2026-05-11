-- 008_ticket_notify_consent.up.sql
-- 工单 + 通知 outbox + consent_log
-- 引用：backend §3.18

CREATE TABLE IF NOT EXISTS `ticket` (
    `id`             BIGINT NOT NULL AUTO_INCREMENT,
    `opener_type`    VARCHAR(16) NOT NULL,
    `opener_id`      BIGINT NOT NULL,
    `subject`        VARCHAR(255) NOT NULL,
    `category`       VARCHAR(32) NOT NULL,
    `status`         VARCHAR(32) NOT NULL DEFAULT 'open',
    `assigned_to`    BIGINT NULL,
    `priority`       TINYINT NOT NULL DEFAULT 3,
    `last_reply_at`  TIMESTAMP(3) NULL,
    `sla_due_at`     TIMESTAMP(3) NULL,
    `created_at`     TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`     TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    `deleted_at`     TIMESTAMP(3) NULL,
    PRIMARY KEY (`id`),
    KEY `idx_ticket_assigned` (`assigned_to`, `status`, `sla_due_at`),
    KEY `idx_ticket_opener` (`opener_type`, `opener_id`, `status`),
    CONSTRAINT `chk_ticket_status` CHECK (`status` IN ('open','assigned','responding','waiting_user','resolved','closed','reopened')),
    CONSTRAINT `chk_ticket_category` CHECK (`category` IN ('billing','kyc','api','content_report','other'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='工单 (Phase 1 basic / 3 full)';

CREATE TABLE IF NOT EXISTS `ticket_reply` (
    `id`           BIGINT NOT NULL AUTO_INCREMENT,
    `ticket_id`    BIGINT NOT NULL,
    `sender_type`  VARCHAR(16) NOT NULL,
    `sender_id`    BIGINT NOT NULL,
    `body_md`      TEXT NOT NULL,
    `attachments`  JSON,
    `created_at`   TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    KEY `idx_reply_ticket` (`ticket_id`, `created_at`),
    CONSTRAINT `fk_reply_ticket` FOREIGN KEY (`ticket_id`) REFERENCES `ticket` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='工单回复 (Phase 1)';

CREATE TABLE IF NOT EXISTS `notification_outbox` (
    `id`             BIGINT NOT NULL AUTO_INCREMENT,
    `recipient`      VARCHAR(255) NOT NULL,
    `channel`        VARCHAR(16) NOT NULL,
    `event_code`     VARCHAR(64) NOT NULL,
    `ref_id`         VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'v1.0 cosmetic #3 / M-2 防重复推送',
    `payload`        TEXT,
    `status`         VARCHAR(16) NOT NULL DEFAULT 'pending',
    `retry_count`    INT NOT NULL DEFAULT 0,
    `last_error`     TEXT,
    `dispatched_at`  TIMESTAMP(3) NULL,
    `trace_id`       VARCHAR(64) NOT NULL DEFAULT '',
    `created_at`     TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`     TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    KEY `idx_notif_pending` (`status`, `id`),
    UNIQUE KEY `uk_notif_dedup` (`event_code`, `recipient`, `ref_id`),
    CONSTRAINT `chk_notif_channel` CHECK (`channel` IN ('email','inapp','sms','webhook')),
    CONSTRAINT `chk_notif_status` CHECK (`status` IN ('pending','sent','failed','dead_letter'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='通知 outbox (Phase 1 inapp+email / 2A sms+webhook)';

CREATE TABLE IF NOT EXISTS `consent_log` (
    `id`                    BIGINT NOT NULL AUTO_INCREMENT,
    `subject_fy_user_id`    BIGINT NOT NULL,
    `consent_type`          VARCHAR(64) NOT NULL,
    `consent_text_version`  VARCHAR(64) NOT NULL,
    `consented_at`          TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `ip`                    VARCHAR(64),
    `user_agent`            VARCHAR(512),
    `withdrawn`             TINYINT(1) NOT NULL DEFAULT 0,
    `withdrawn_at`          TIMESTAMP(3) NULL,
    PRIMARY KEY (`id`),
    KEY `idx_consent_subject` (`subject_fy_user_id`, `consent_type`),
    CONSTRAINT `chk_consent_type` CHECK (`consent_type` IN (
        'privacy_policy','sensitive_pi','biometric','cross_border','device_fingerprint',
        'automated_decision','third_party_share'
    ))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='consent_log (Phase 1)';
