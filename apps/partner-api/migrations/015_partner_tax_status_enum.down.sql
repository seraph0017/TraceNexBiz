-- 015_partner_tax_status_enum.down.sql
-- 回滚到旧 5 枚举.
ALTER TABLE `partner` DROP CHECK `chk_partner_tax_status`;

UPDATE `partner` SET `tax_status` = 'individual_business' WHERE `tax_status` = 'sole_proprietor';
UPDATE `partner` SET `tax_status` = 'company'             WHERE `tax_status` IN ('llc','corp','partnership');

ALTER TABLE `partner`
    ADD CONSTRAINT `chk_partner_tax_status`
    CHECK (`tax_status` IN ('individual','sole_proprietor','individual_business','company','unknown'));
