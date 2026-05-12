-- 013_idempotency_record_plaintext.down.sql
ALTER TABLE `idempotency_record`
    DROP COLUMN `response_body`,
    MODIFY COLUMN `response_cipher` VARBINARY(16384) NOT NULL,
    MODIFY COLUMN `response_key_id` VARCHAR(128) NOT NULL;
