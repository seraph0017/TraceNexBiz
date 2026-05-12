-- 014_audit_log_request.up.sql
-- Fix-B' part 4 CRIT-B6: middleware Audit 中间件入队的 AuditEntry 需要落 unsealed 表，
-- 现有 schema 缺 route / method / status / request_hash / payload_json 列，本迁移补齐。
--
-- 引用：backend §10.1 sealer / Security HIGH-r2-1 / handoff CRIT-B6.
--
-- 设计选择（与 task spec 的对照）：
--   spec.seq           = audit_log.id（BIGINT PK；audit_log_unsealed 自增后被 sealer 原样拷贝）
--   spec.hash          = audit_log.self_hash
--   spec.prev_hash     = audit_log.prev_hash（已有）
--   spec.actor_type    = audit_log.actor_type（已有）
--   spec.actor_id      = audit_log.actor_id（已有）
--   spec.route         = audit_log.route（新增）
--   spec.method        = audit_log.method（新增）
--   spec.status        = audit_log.status（新增）
--   spec.request_hash  = audit_log.request_hash（新增；SHA-256 of canonical PII-scrubbed body）
--   spec.payload_json  = audit_log.payload_json（新增；PII-scrubbed body；GET 可 NULL）
--   spec.request_id    = audit_log.trace_id（已有；语义等价）
--   spec.client_ip     = audit_log.ip_address（已有）
--   spec.created_at    = audit_log.occurred_at + audit_log.sealed_at（已有）
--
-- UNIQUE on seq 由 PRIMARY KEY 提供；INDEX (actor_type, actor_id, created_at) 已存在；
-- INDEX (request_id) 通过 trace_id 索引补齐。

ALTER TABLE `audit_log_unsealed`
    ADD COLUMN `route`        VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'Fix-B'' part 4 middleware path',
    ADD COLUMN `method`       VARCHAR(8)   NOT NULL DEFAULT '' COMMENT 'HTTP method',
    ADD COLUMN `status`       SMALLINT     NOT NULL DEFAULT 0  COMMENT 'HTTP status',
    ADD COLUMN `request_hash` CHAR(64)     NOT NULL DEFAULT '' COMMENT 'SHA-256 of canonical PII-scrubbed body',
    ADD COLUMN `payload_json` MEDIUMTEXT   NULL                COMMENT 'PII-scrubbed body (NULL for GET)';

ALTER TABLE `audit_log`
    ADD COLUMN `route`        VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'Fix-B'' part 4 middleware path',
    ADD COLUMN `method`       VARCHAR(8)   NOT NULL DEFAULT '' COMMENT 'HTTP method',
    ADD COLUMN `status`       SMALLINT     NOT NULL DEFAULT 0  COMMENT 'HTTP status',
    ADD COLUMN `request_hash` CHAR(64)     NOT NULL DEFAULT '' COMMENT 'SHA-256 of canonical PII-scrubbed body',
    ADD COLUMN `payload_json` MEDIUMTEXT   NULL                COMMENT 'PII-scrubbed body (NULL for GET)';

CREATE INDEX `idx_audit_trace_id` ON `audit_log` (`trace_id`);
