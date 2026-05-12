-- 016_bank_account_blind_index.down.sql
ALTER TABLE `kyc_application` DROP INDEX `uk_kyc_bank_acct_blind`;
ALTER TABLE `kyc_application`
    ADD INDEX `idx_kyc_bank_acct_blind` (`bank_account_blind_index`);
