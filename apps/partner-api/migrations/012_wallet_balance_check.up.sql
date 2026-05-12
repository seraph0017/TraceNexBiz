-- 012_wallet_balance_check.up.sql
-- HIGH-B7: 给 partner_wallet.balance 加 CHECK (balance >= 0) 约束。
--
-- 创建时 002_wallet.up.sql 的 `chk_wallet_amounts` 只覆盖了 paid_out_total >= 0；
-- 本迁移补 balance >= 0 单独约束。
--
-- 跨方言说明：
--   - MySQL 8.0.16+ / PostgreSQL 9.6+ ：支持 ALTER TABLE ADD CONSTRAINT CHECK。本文件 SQL OK。
--   - SQLite ：不支持 ALTER TABLE ADD CHECK；迁移工具需走 12-step recreate；本项目的测试
--     DB 用 GORM AutoMigrate 走 SQLite 方言重建表，不跑此迁移；应用层在 wallet_mysql.go
--     的 AdjustBalance 用原子 SQL `balance + ? >= 0` 兜底（HIGH-B7 belt-and-braces）。

ALTER TABLE `partner_wallet`
    ADD CONSTRAINT `chk_partner_wallet_balance_nonneg` CHECK (`balance` >= 0);
