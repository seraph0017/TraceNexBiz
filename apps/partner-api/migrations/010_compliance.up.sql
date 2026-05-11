-- 010_compliance.up.sql
-- 合规：内容安全事件 / 上报通道 / PIA 报告 / PIPL 投诉 / PIPL 用户权利请求
-- 引用：backend §3.23 / §3.24 / §3.25 / §3.26 / §3.27

CREATE TABLE IF NOT EXISTS `content_safety_event` (
    `id`                    BIGINT NOT NULL AUTO_INCREMENT,
    `fy_user_id`            BIGINT NOT NULL,
    `kind`                  VARCHAR(16) NOT NULL,
    `provider`              VARCHAR(32) NOT NULL,
    `prompt_hash`           CHAR(64) NOT NULL COMMENT 'SHA-256 防重；不存原文',
    `category`              VARCHAR(64) NOT NULL,
    `score`                 DECIMAL(5,4) NOT NULL,
    `disposition`           VARCHAR(32) NOT NULL,
    `reviewed_by`           BIGINT NULL,
    `reviewed_at`           TIMESTAMP(3) NULL,
    `reported_to_12377_at`  TIMESTAMP(3) NULL,
    `audit_log_id`          BIGINT NULL,
    `trace_id`              VARCHAR(64) NOT NULL DEFAULT '',
    `created_at`            TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    KEY `idx_csafety_user` (`fy_user_id`, `created_at`),
    KEY `idx_csafety_disposition` (`disposition`, `reported_to_12377_at`),
    CONSTRAINT `chk_csafety_kind` CHECK (`kind` IN ('input','output')),
    CONSTRAINT `chk_csafety_provider` CHECK (`provider` IN ('aliyun','tencent','mock'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='内容安全事件 (Phase 2A schema)';

CREATE TABLE IF NOT EXISTS `content_safety_report` (
    `id`                BIGINT NOT NULL AUTO_INCREMENT,
    `event_id`          BIGINT NOT NULL,
    `target_authority`  VARCHAR(32) NOT NULL,
    `payload`           JSON NOT NULL,
    `status`            VARCHAR(32) NOT NULL DEFAULT 'pending',
    `submitted_at`      TIMESTAMP(3) NULL,
    `sla_due_at`        TIMESTAMP(3) NOT NULL COMMENT 'created_at + 24h',
    `response_payload`  JSON,
    `retry_count`       INT NOT NULL DEFAULT 0,
    `last_error`        TEXT,
    `created_at`        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    KEY `idx_csreport_pending` (`status`, `sla_due_at`),
    CONSTRAINT `fk_csreport_event` FOREIGN KEY (`event_id`) REFERENCES `content_safety_event` (`id`),
    CONSTRAINT `chk_csreport_status` CHECK (`status` IN ('pending','submitted','acknowledged','failed','dead_letter')),
    CONSTRAINT `chk_csreport_authority` CHECK (`target_authority` IN ('12377','public_security','internal'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='12377/网信办上报 (Phase 2A; Compliance CRIT-2)';

CREATE TABLE IF NOT EXISTS `pia_report` (
    `id`                  BIGINT NOT NULL AUTO_INCREMENT,
    `title`               VARCHAR(255) NOT NULL,
    `scope`               VARCHAR(64) NOT NULL,
    `purpose_text`        TEXT NOT NULL,
    `scope_text`          TEXT NOT NULL,
    `necessity_text`      TEXT NOT NULL,
    `legal_basis_text`    TEXT NOT NULL,
    `impact_text`         TEXT NOT NULL,
    `risk_text`           TEXT NOT NULL,
    `measures_text`       TEXT NOT NULL,
    `residual_risk_text`  TEXT NOT NULL,
    `report_url`          VARCHAR(1024),
    `valid_from`          TIMESTAMP(3) NOT NULL,
    `valid_until`         TIMESTAMP(3) NOT NULL COMMENT '留档 ≥ 3 年',
    `signed_by_dpo`       BIGINT NOT NULL,
    `signed_at`           TIMESTAMP(3) NOT NULL,
    `created_at`          TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    KEY `idx_pia_scope_validity` (`scope`, `valid_until`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='PIA 报告 (Phase 2A; PIPL §55 + GB/T 39335-2020)';

CREATE TABLE IF NOT EXISTS `pipl_complaint` (
    `id`                    BIGINT NOT NULL AUTO_INCREMENT,
    `subject_fy_user_id`    BIGINT NULL,
    `contact_email`         VARCHAR(128) NOT NULL,
    `contact_phone_cipher`  VARBINARY(512),
    `category`              VARCHAR(32) NOT NULL,
    `description`           TEXT NOT NULL,
    `status`                VARCHAR(32) NOT NULL DEFAULT 'received',
    `assigned_to`           BIGINT NULL,
    `sla_due_at`            TIMESTAMP(3) NOT NULL COMMENT 'created_at + 15d (PIPL §50)',
    `resolution_text`       TEXT,
    `resolved_at`           TIMESTAMP(3) NULL,
    `audit_log_id`          BIGINT NULL,
    `trace_id`              VARCHAR(64) NOT NULL DEFAULT '',
    `created_at`            TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at`            TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`id`),
    KEY `idx_pipl_complaint_status` (`status`, `sla_due_at`),
    CONSTRAINT `chk_pipl_complaint_category` CHECK (`category` IN ('erase','access','rectify','consent_withdrawal','other')),
    CONSTRAINT `chk_pipl_complaint_status` CHECK (`status` IN ('received','reviewing','responded','closed','escalated'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='PIPL 投诉受理通道 (Phase 2A)';

CREATE TABLE IF NOT EXISTS `pipl_request` (
    `id`                  BIGINT NOT NULL AUTO_INCREMENT,
    `actor_type`          VARCHAR(16) NOT NULL,
    `actor_id`            BIGINT NOT NULL,
    `fy_user_id`          BIGINT NULL,
    `request_type`        VARCHAR(32) NOT NULL,
    `state`               VARCHAR(32) NOT NULL DEFAULT 'submitted',
    `deadline`            TIMESTAMP(3) NOT NULL COMMENT '+5d 核身',
    `completed_deadline`  TIMESTAMP(3) NOT NULL COMMENT '+30d (PIPL §50)',
    `reason`              TEXT,
    `rejection_reason`    TEXT,
    `export_oss_key`      VARCHAR(255),
    `audit_log_id`        BIGINT NULL,
    `trace_id`            VARCHAR(64) NOT NULL DEFAULT '',
    `submitted_at`        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `completed_at`        TIMESTAMP(3) NULL,
    PRIMARY KEY (`id`),
    KEY `idx_pipl_req_actor` (`actor_type`, `actor_id`, `state`),
    KEY `idx_pipl_req_deadline` (`state`, `deadline`),
    KEY `idx_pipl_req_completed_deadline` (`state`, `completed_deadline`),
    CONSTRAINT `chk_pipl_req_actor_type` CHECK (`actor_type` IN ('customer','partner')),
    CONSTRAINT `chk_pipl_req_type` CHECK (`request_type` IN ('access','rectify','erase','restrict','port')),
    CONSTRAINT `chk_pipl_req_state` CHECK (`state` IN ('submitted','id_check','approved','executing','completed','rejected','expired'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='PIPL §44-§47 用户权利请求 (Phase 2A; M13)';
