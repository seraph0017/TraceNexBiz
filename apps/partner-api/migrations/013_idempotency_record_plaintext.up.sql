-- 013_idempotency_record_plaintext.up.sql
-- Fix-B' part 2 CRIT-B3：同事务 idempotency_record co-commit.
--
-- 关闭 KMS 必传约束（response_cipher / response_key_id 改 NULL-able），新增 response_body
-- 明文列；待 Fix-C KMS Encrypt 真接入后切回 cipher（届时 service 层写 cipher，body 留 NULL）。
--
-- 索引保持不变：UNIQUE (actor_type, actor_id, idempotency_key, endpoint) 已是 §8.16 同 tx
-- duplicate detection 的核心。

ALTER TABLE `idempotency_record`
    MODIFY COLUMN `response_cipher` VARBINARY(16384) NULL,
    MODIFY COLUMN `response_key_id` VARCHAR(128) NULL,
    ADD COLUMN `response_body` MEDIUMTEXT NULL COMMENT 'Phase-1 明文响应；KMS 启用后回退 cipher (Fix-B'' part 2)';
