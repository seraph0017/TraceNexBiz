-- 00_bootstrap.sql
-- 给 docker-compose dev 用：初始化用户 + log_db + GRANT 矩阵骨架
-- prod 走 backend §3 + integration §6 完整 GRANT 矩阵（不在此文件）。

CREATE DATABASE IF NOT EXISTS partner_db
  DEFAULT CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;

CREATE DATABASE IF NOT EXISTS log_db
  DEFAULT CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;

CREATE DATABASE IF NOT EXISTS fy_api_db
  DEFAULT CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;

-- dev 单密码方案；prod 必须每账号独立 secret + KMS Secret Manager
GRANT ALL PRIVILEGES ON partner_db.* TO 'tnbiz_app'@'%';
GRANT ALL PRIVILEGES ON log_db.* TO 'tnbiz_app'@'%';
GRANT SELECT ON fy_api_db.* TO 'tnbiz_app'@'%';

FLUSH PRIVILEGES;
