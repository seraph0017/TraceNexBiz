-- 012_wallet_balance_check.down.sql
-- 回滚 HIGH-B7：drop CHECK 约束。
ALTER TABLE `partner_wallet`
    DROP CONSTRAINT `chk_partner_wallet_balance_nonneg`;
