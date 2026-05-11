-- 003_pricing_revenue.up.sql
-- 定价规则 + 收益日志
-- 引用：backend §3.6 / §3.7

CREATE TABLE IF NOT EXISTS `partner_pricing_rule` (
    `id`                 BIGINT NOT NULL AUTO_INCREMENT,
    `partner_id`         BIGINT NOT NULL,
    `customer_id`        BIGINT NULL COMMENT 'NULL = partner 默认',
    `model_name`         VARCHAR(128) NULL COMMENT 'NULL = 全模型',
    `tier_name`          VARCHAR(64) NULL,
    `customer_id_canon`  VARCHAR(32)  AS (IFNULL(CAST(`customer_id` AS CHAR), '*')) STORED,
    `model_name_canon`   VARCHAR(128) AS (IFNULL(`model_name`, '*')) STORED,
    `tier_name_canon`    VARCHAR(64)  AS (IFNULL(`tier_name`, '*')) STORED,
    `markup`             DECIMAL(10,4) NOT NULL,
    `valid_from`         TIMESTAMP(3) NOT NULL,
    `valid_to`           TIMESTAMP(3) NULL,
    `status`             VARCHAR(16) NOT NULL DEFAULT 'active',
    `created_by`         BIGINT NOT NULL,
    `note`               TEXT,
    `created_at`         TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`         TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    `deleted_at`         TIMESTAMP(3) NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_pricing_rule_canon` (`partner_id`, `customer_id_canon`, `model_name_canon`, `tier_name_canon`, `valid_from`),
    KEY `idx_pricing_partner_active` (`partner_id`, `status`, `valid_from`, `valid_to`),
    KEY `idx_pricing_customer` (`customer_id`, `valid_from`),
    CONSTRAINT `fk_pricing_partner` FOREIGN KEY (`partner_id`) REFERENCES `partner` (`id`),
    CONSTRAINT `chk_pricing_markup` CHECK (`markup` >= 1.0 AND `markup` <= 5.0),
    CONSTRAINT `chk_pricing_status` CHECK (`status` IN ('active','archived','draft'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='定价规则 (Phase 1 单层 / Phase 2A 多层)';

CREATE TABLE IF NOT EXISTS `revenue_log` (
    `id`               BIGINT NOT NULL AUTO_INCREMENT,
    `partner_id`       BIGINT NOT NULL,
    `customer_id`      BIGINT NOT NULL,
    `fy_api_log_id`    BIGINT NOT NULL COMMENT '关联 fy_api_db.logs.id（不建 FK）',
    `occurrence`       TINYINT NOT NULL DEFAULT 1 COMMENT '1=正常 2+=显式调整',
    `gross_amount`     BIGINT NOT NULL,
    `cost_amount`      BIGINT NOT NULL,
    `net_revenue`      BIGINT NOT NULL,
    `applied_rule_id`  BIGINT NOT NULL,
    `occurred_at`      TIMESTAMP(3) NOT NULL,
    `settlement_id`    BIGINT NULL,
    `trace_id`         VARCHAR(64) NOT NULL DEFAULT '',
    `created_at`       TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_revenue_log` (`fy_api_log_id`, `occurrence`),
    KEY `idx_revenue_partner_time` (`partner_id`, `occurred_at`),
    KEY `idx_revenue_customer_time` (`customer_id`, `occurred_at`),
    KEY `idx_revenue_settlement` (`settlement_id`),
    CONSTRAINT `fk_revenue_partner` FOREIGN KEY (`partner_id`) REFERENCES `partner` (`id`),
    CONSTRAINT `fk_revenue_customer` FOREIGN KEY (`customer_id`) REFERENCES `customer` (`id`),
    CONSTRAINT `fk_revenue_rule` FOREIGN KEY (`applied_rule_id`) REFERENCES `partner_pricing_rule` (`id`),
    CONSTRAINT `chk_revenue_occurrence` CHECK (`occurrence` BETWEEN 1 AND 127)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='收益记录 (Phase 1 / Phase 2B 接 settlement)';
