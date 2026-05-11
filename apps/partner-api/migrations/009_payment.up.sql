-- 009_payment.up.sql
-- 客户充值意图（持牌方收单）
-- 引用：backend §3.21

CREATE TABLE IF NOT EXISTS `topup_intent` (
    `id`                 BIGINT NOT NULL AUTO_INCREMENT,
    `customer_id`        BIGINT NOT NULL,
    `amount`             BIGINT NOT NULL,
    `channel`            VARCHAR(32) NOT NULL,
    `out_trade_no`       VARCHAR(64) NOT NULL,
    `state`              VARCHAR(32) NOT NULL DEFAULT 'created',
    `paid_at`            TIMESTAMP(3) NULL,
    `funded_at`          TIMESTAMP(3) NULL,
    `saga_id`            VARCHAR(64) NOT NULL COMMENT 'UUIDv7 字符串；用作 Idempotency-Key',
    `provider_trade_no`  VARCHAR(128),
    `callback_payload`   TEXT,
    `created_at`         TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`         TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_topup_channel_trade` (`channel`, `out_trade_no`),
    UNIQUE KEY `uk_topup_saga_id` (`saga_id`),
    KEY `idx_topup_customer` (`customer_id`, `state`),
    CONSTRAINT `fk_topup_customer` FOREIGN KEY (`customer_id`) REFERENCES `customer` (`id`),
    CONSTRAINT `chk_topup_state` CHECK (`state` IN ('created','paid','funded','refunded','failed','canceled'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='客户充值 intent (Phase 2A)';
