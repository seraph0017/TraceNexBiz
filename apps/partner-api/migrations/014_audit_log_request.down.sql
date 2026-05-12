-- 014_audit_log_request.down.sql — revert Fix-B' part 4 audit_log schema additions.

DROP INDEX `idx_audit_trace_id` ON `audit_log`;

ALTER TABLE `audit_log`
    DROP COLUMN `route`,
    DROP COLUMN `method`,
    DROP COLUMN `status`,
    DROP COLUMN `request_hash`,
    DROP COLUMN `payload_json`;

ALTER TABLE `audit_log_unsealed`
    DROP COLUMN `route`,
    DROP COLUMN `method`,
    DROP COLUMN `status`,
    DROP COLUMN `request_hash`,
    DROP COLUMN `payload_json`;
