-- 016_bank_account_blind_index.up.sql
-- Fix-C item 6：在 KYC 表上为 bank_account_blind_index 加 UNIQUE 索引.
-- 当前已有 KEY idx_kyc_bank_acct_blind（migration 005）；UNIQUE 约束确保同一持牌
-- 账户不能在 KYC 表中出现两次（预防欺诈 KYC 主体重复申报）.
--
-- 注意：blind_index 可能为空串（旧行 / 历史数据），UNIQUE 索引允许 NULL 但不允许多个空串行；
-- 故先把空串 → NULL，再加 UNIQUE.
--
-- SQLite 不支持 ALTER TABLE DROP INDEX 与多空串 UNIQUE 语义略有差异；
-- migrator SQLite 路径 skip 此文件（recreate-required）.

UPDATE `kyc_application` SET `bank_account_blind_index` = NULL WHERE `bank_account_blind_index` = '';

ALTER TABLE `kyc_application` DROP INDEX `idx_kyc_bank_acct_blind`;
ALTER TABLE `kyc_application`
    ADD UNIQUE INDEX `uk_kyc_bank_acct_blind` (`bank_account_blind_index`);
