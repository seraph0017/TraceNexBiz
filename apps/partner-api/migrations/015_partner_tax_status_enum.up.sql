-- 015_partner_tax_status_enum.up.sql
-- Fix-C item 5：partner.tax_status 5 枚举对齐
--   PRD 要求：(individual, sole_proprietor, partnership, llc, corp)
--   旧值：    (individual, sole_proprietor, individual_business, company, unknown)
-- 迁移策略：先 ALTER 放宽 CHECK，把旧值映射到新值后再加新 CHECK.
--
-- 兼容矩阵（旧值 → 新值）：
--   individual          → individual
--   sole_proprietor     → sole_proprietor
--   individual_business → sole_proprietor   -- 个体工商户 ≈ sole proprietor
--   company             → corp              -- 公司主体（最常见）
--   unknown             → individual        -- 兜底
--
-- SQLite 不支持 ALTER TABLE DROP CONSTRAINT；migrator 在 SQLite 路径直接 skip 此文件.

ALTER TABLE `partner` DROP CHECK `chk_partner_tax_status`;

UPDATE `partner` SET `tax_status` = 'sole_proprietor' WHERE `tax_status` = 'individual_business';
UPDATE `partner` SET `tax_status` = 'corp'            WHERE `tax_status` = 'company';
UPDATE `partner` SET `tax_status` = 'individual'      WHERE `tax_status` = 'unknown';

ALTER TABLE `partner`
    ADD CONSTRAINT `chk_partner_tax_status`
    CHECK (`tax_status` IN ('individual','sole_proprietor','partnership','llc','corp'));
