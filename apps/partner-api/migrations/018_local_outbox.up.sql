-- 018_local_outbox.up.sql
-- Local SOURCE outbox for partner-api -> Aliyun MNS publishing (Round-3 NEW-H1).

CREATE TABLE IF NOT EXISTS `outbox` (
    `id`            BIGINT NOT NULL AUTO_INCREMENT,
    `event_type`    VARCHAR(128) NOT NULL,
    `payload`       JSON NOT NULL,
    `status`        VARCHAR(32) NOT NULL DEFAULT 'pending',
    `trace_id`      VARCHAR(64) NOT NULL DEFAULT '',
    `last_error`    TEXT,
    `published_at`  TIMESTAMP(3) NULL,
    `created_at`    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    KEY `idx_outbox_pending` (`status`, `id`),
    KEY `idx_outbox_event_type` (`event_type`, `created_at`),
    CONSTRAINT `chk_outbox_status` CHECK (`status` IN ('pending','sent','dead_letter'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='partner-api local outbox SOURCE for MNS';
