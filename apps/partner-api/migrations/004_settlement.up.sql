-- 004_settlement.up.sql
-- 月结：settlement / settlement_item / settlement_run / settlement_config_change_log
-- 引用：backend §3.8

CREATE TABLE IF NOT EXISTS `settlement` (
    `id`                BIGINT NOT NULL AUTO_INCREMENT,
    `period`            VARCHAR(32) NOT NULL,
    `period_start`      TIMESTAMP(3) NOT NULL,
    `period_end`        TIMESTAMP(3) NOT NULL,
    `timezone`          VARCHAR(32) NOT NULL DEFAULT 'Asia/Shanghai',
    `total_revenue`     BIGINT NOT NULL DEFAULT 0,
    `total_cost`        BIGINT NOT NULL DEFAULT 0,
    `total_payout`      BIGINT NOT NULL DEFAULT 0,
    `status`            VARCHAR(32) NOT NULL DEFAULT 'generating',
    `progress_offset`   BIGINT NOT NULL DEFAULT 0,
    `generated_at`      TIMESTAMP(3) NULL,
    `paid_at`           TIMESTAMP(3) NULL,
    `paid_by`           BIGINT NULL,
    `payment_evidence`  TEXT,
    `created_at`        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_settlement_period` (`period`),
    KEY `idx_settlement_status` (`status`),
    CONSTRAINT `chk_settlement_status` CHECK (`status` IN ('generating','generated','paying','paid','failed','partially_disputed','gate_failed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='月结主表 (Phase 2B)';

CREATE TABLE IF NOT EXISTS `settlement_item` (
    `id`                 BIGINT NOT NULL AUTO_INCREMENT,
    `settlement_id`      BIGINT NOT NULL,
    `partner_id`         BIGINT NOT NULL,
    `revenue`            BIGINT NOT NULL,
    `cost`               BIGINT NOT NULL,
    `platform_fee`       BIGINT NOT NULL,
    `withheld_tax`       BIGINT NOT NULL DEFAULT 0,
    `payout`             BIGINT NOT NULL,
    `tax_evidence_url`   VARCHAR(1024),
    `status`             VARCHAR(16) NOT NULL DEFAULT 'pending',
    `provider_trade_no`  VARCHAR(128),
    `payout_evidence`    TEXT,
    `invoice_id`         BIGINT NULL,
    `is_partial`         TINYINT(1) NOT NULL DEFAULT 0,
    `created_at`         TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`         TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_settlement_item` (`settlement_id`, `partner_id`),
    KEY `idx_settlement_item_partner` (`partner_id`, `status`),
    CONSTRAINT `fk_si_settlement` FOREIGN KEY (`settlement_id`) REFERENCES `settlement` (`id`),
    CONSTRAINT `fk_si_partner` FOREIGN KEY (`partner_id`) REFERENCES `partner` (`id`),
    CONSTRAINT `chk_si_status` CHECK (`status` IN ('pending','paid','disputed','failed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='月结明细 (Phase 2B)';

CREATE TABLE IF NOT EXISTS `settlement_run` (
    `id`                 BIGINT NOT NULL AUTO_INCREMENT,
    `settlement_id`      BIGINT NOT NULL,
    `hostname`           VARCHAR(128) NOT NULL,
    `pid`                INT NOT NULL,
    `started_at`         TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `last_heartbeat`     TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `lease_expires_at`   TIMESTAMP(3) NOT NULL COMMENT 'Redis SETNX 续约对应',
    `ended_at`           TIMESTAMP(3) NULL,
    `status`             VARCHAR(16) NOT NULL DEFAULT 'running',
    PRIMARY KEY (`id`),
    KEY `idx_settlement_run_settlement` (`settlement_id`),
    CONSTRAINT `fk_sr_settlement` FOREIGN KEY (`settlement_id`) REFERENCES `settlement` (`id`),
    CONSTRAINT `chk_sr_status` CHECK (`status` IN ('running','completed','crashed','taken_over'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='settlement runner leader 续约 (Phase 2B)';

CREATE TABLE IF NOT EXISTS `settlement_config_change_log` (
    `id`              BIGINT NOT NULL AUTO_INCREMENT,
    `changed_by`      BIGINT NOT NULL,
    `old_period`      VARCHAR(32),
    `new_period`      VARCHAR(32),
    `effective_from`  TIMESTAMP(3) NOT NULL,
    `reason`          TEXT,
    `created_at`      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='settlement 配置变更审计 (Phase 2B)';
