-- 本地 MySQL 初始化（仅 dev）
-- 复刻 backend §3 + integration §6 GRANT 矩阵的简化版本（dev 用单密码 + 单 host）

CREATE DATABASE IF NOT EXISTS partner_db
  DEFAULT CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;

CREATE DATABASE IF NOT EXISTS fy_api_db
  DEFAULT CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;

CREATE USER IF NOT EXISTS 'tnbiz_app'@'%' IDENTIFIED BY 'tnbiz_app';
CREATE USER IF NOT EXISTS 'tnbiz_migrator'@'%' IDENTIFIED BY 'tnbiz_migrator';
CREATE USER IF NOT EXISTS 'tnbiz_outbox_consumer'@'%' IDENTIFIED BY 'tnbiz_outbox_consumer';
CREATE USER IF NOT EXISTS 'tnbiz_audit_sealer'@'%' IDENTIFIED BY 'tnbiz_audit_sealer';

-- partner_db
GRANT ALL PRIVILEGES ON partner_db.* TO 'tnbiz_migrator'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON partner_db.* TO 'tnbiz_app'@'%';
-- audit_log 仅 sealer 可写最终表；app 写 unsealed 队列
REVOKE UPDATE, DELETE ON partner_db.audit_log FROM 'tnbiz_app'@'%';
GRANT SELECT, INSERT ON partner_db.audit_log TO 'tnbiz_audit_sealer'@'%';
GRANT SELECT, DELETE ON partner_db.audit_log_unsealed TO 'tnbiz_audit_sealer'@'%';
GRANT SELECT ON partner_db.audit_log_pii TO 'tnbiz_audit_sealer'@'%';

-- fy_api_db: app 仅 SELECT
GRANT SELECT ON fy_api_db.* TO 'tnbiz_app'@'%';
-- outbox consumer 仅在 consume_log_outbox 上 SELECT/UPDATE/DELETE
GRANT SELECT, UPDATE, DELETE ON fy_api_db.consume_log_outbox TO 'tnbiz_outbox_consumer'@'%';

FLUSH PRIVILEGES;
