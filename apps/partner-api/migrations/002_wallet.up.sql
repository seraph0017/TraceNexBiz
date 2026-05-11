-- 002_wallet.up.sql
-- 钱包：partner_wallet (drop held_amount per ADR-012) + wallet_hold + partner_wallet_log + partner_debt
-- 引用：backend §3.3 / §3.4 / §3.5 / §3.22

CREATE TABLE IF NOT EXISTS `partner_wallet` (
    `id`              BIGINT NOT NULL AUTO_INCREMENT,
    `partner_id`      BIGINT NOT NULL,
    `balance`         BIGINT NOT NULL DEFAULT 0 COMMENT '应付台账（quota 单位）',
    `paid_out_total`  BIGINT NOT NULL DEFAULT 0,
    `version`         BIGINT NOT NULL DEFAULT 0 COMMENT '乐观锁',
    `created_at`      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_wallet_partner` (`partner_id`),
    CONSTRAINT `fk_wallet_partner` FOREIGN KEY (`partner_id`) REFERENCES `partner` (`id`),
    CONSTRAINT `chk_wallet_amounts` CHECK (`paid_out_total` >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='渠道商应付台账 (Phase 1)；held 由 wallet_hold 计算';

CREATE TABLE IF NOT EXISTS `wallet_hold` (
    `id`           BIGINT NOT NULL AUTO_INCREMENT,
    `wallet_id`    BIGINT NOT NULL,
    `partner_id`   BIGINT NOT NULL,
    `amount`       BIGINT NOT NULL,
    `saga_id`      VARCHAR(64) NOT NULL COMMENT '= idempotency_key',
    `status`       VARCHAR(16) NOT NULL DEFAULT 'held',
    `held_at`      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `resolved_at`  TIMESTAMP(3) NULL,
    `created_at`   TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`   TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_hold_saga` (`saga_id`),
    KEY `idx_hold_wallet_status` (`wallet_id`, `status`),
    KEY `idx_hold_partner_held` (`partner_id`, `status`, `held_at`),
    CONSTRAINT `fk_hold_wallet` FOREIGN KEY (`wallet_id`) REFERENCES `partner_wallet` (`id`),
    CONSTRAINT `fk_hold_partner` FOREIGN KEY (`partner_id`) REFERENCES `partner` (`id`),
    CONSTRAINT `chk_hold_amount` CHECK (`amount` > 0),
    CONSTRAINT `chk_hold_status` CHECK (`status` IN ('held','committed','released'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='wallet hold 两阶段表 (Phase 1)';

CREATE TABLE IF NOT EXISTS `partner_wallet_log` (
    `id`               BIGINT NOT NULL AUTO_INCREMENT,
    `partner_id`       BIGINT NOT NULL,
    `type`             VARCHAR(32) NOT NULL,
    `amount`           BIGINT NOT NULL,
    `balance_after`    BIGINT NOT NULL,
    `ref_id`           VARCHAR(128) NOT NULL DEFAULT '',
    `idempotency_key`  VARCHAR(64) NOT NULL,
    `status`           VARCHAR(32) NOT NULL DEFAULT 'committed',
    `note`             TEXT,
    `operator_type`    VARCHAR(32) NOT NULL,
    `operator_id`      BIGINT NOT NULL DEFAULT 0,
    `trace_id`         VARCHAR(64) NOT NULL DEFAULT '',
    `created_at`       TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_wallet_log_idem` (`idempotency_key`, `type`),
    KEY `idx_wallet_log_partner_time` (`partner_id`, `created_at`),
    KEY `idx_wallet_log_ref` (`ref_id`),
    CONSTRAINT `fk_wallet_log_partner` FOREIGN KEY (`partner_id`) REFERENCES `partner` (`id`),
    CONSTRAINT `chk_wallet_log_type` CHECK (`type` IN (
        'revenue_accrual','allocate_to_customer','settlement_payout','refund_clawback',
        'adjustment','saga_aborted_unknown','initial_topup','platform_isv_commission_in'
    ))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='钱包流水 (Phase 1, append-mostly)';

CREATE TABLE IF NOT EXISTS `partner_debt` (
    `id`           BIGINT NOT NULL AUTO_INCREMENT,
    `partner_id`   BIGINT NOT NULL,
    `amount`       BIGINT NOT NULL COMMENT '欠款（正数）',
    `cause`        VARCHAR(64) NOT NULL,
    `ref_id`       VARCHAR(128),
    `status`       VARCHAR(16) NOT NULL DEFAULT 'open',
    `cleared_at`   TIMESTAMP(3) NULL,
    `created_at`   TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`   TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    KEY `idx_debt_partner` (`partner_id`, `status`),
    CONSTRAINT `fk_debt_partner` FOREIGN KEY (`partner_id`) REFERENCES `partner` (`id`),
    CONSTRAINT `chk_debt_amount` CHECK (`amount` > 0),
    CONSTRAINT `chk_debt_status` CHECK (`status` IN ('open','clearing','cleared','written_off'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='partner_debt (Phase 2A, ADR-010 verdict 方案 A)';
