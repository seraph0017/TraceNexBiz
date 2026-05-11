# Backend Design — TraceNex Partner（partner-api）后端开发设计

> 版本：**v1.2（Fy-api round-2 cosmetic 收口, 2026-05-11）**
> 维护人：Architect (主架构师)
> 最后更新：2026-05-11（v1.1 → v1.2：交叉文档对齐 Fy-api round-2 残余 4 项 cosmetic）
> 维护人：Backend Architect
> 上游契约（不可改）：`docs/00-architecture-overview.md` v1.2 + `docs/integration-design.md` v1.2 + `prd/PRD-v1.0.md`（2295 行）
> Round-1 review：`reviews/dev-round-1/{01-PM,02-Architect,03-Security,04-Compliance}-review.md`
> Round-2 review：`reviews/dev-round-2/{01-PM,02-Architect,03-Security,04-Compliance}-review.md`（4 方 PASS）
> Fy-api review：`reviews/fy-api-review/01-fy-api-side-review.md`（ACCEPT-WITH-CHANGES, 2026-05-12）+ `02-fy-api-round2-review.md`（ACCEPT, 2026-05-13）
> 关联输出：本文是 partner-api 仓 (`TraceNexBiz/`) 工程实施的指导文档；前端 `frontend-design.md` v1.0 同步。详细修订条目见本文末尾 §20 "v0.1 → v0.2 CHANGELOG" + §21/§22 ADDENDUM + §23 "v0.2.2 → v1.0 收口" + §24 "v1.0 → v1.1 CHANGELOG（合入 Fy-api review）" + §25 "v1.1 → v1.2 CHANGELOG（Fy-api round-2 cosmetic）"。

---

## 0. 阅读说明 + 目录

本文是 partner-api 单仓 Go 工程的实施级设计。**它依赖**两份硬契约：

- 任何与 Fy-api 的交互（HMAC、内部 API、outbox、Pub/Sub、saga、GRANT）走 `integration-design.md` 的字面规约；本文不复述只引用。
- 任何模块边界（12 模块 / Phase 切片 / 命名 / 错误码 / 测试钩子集）走 `00-architecture-overview.md`。

本文新增的内容是 partner-api 内部如何把这两份契约**落地为可运行 Go 代码 + MySQL DDL + OpenAPI**，以及 partner_db 的全部 DDL（含索引、外键、CHECK）。

> ⚠️ 本文每节写法：**先一句话指明引用章节，再展开"怎么实现"**；不复述 PRD/overview/integration-design 已经定的内容。

### 目录

1. 总览：12 模块 → Go package 树
2. 代码组织 / 分层（handler/service/repository/domain/infra）
3. partner_db 全部表 DDL（§8.1 ~ §8.20）
4. partner-api 对外 REST API（OpenAPI 3.1 风格片段）
5. 核心 service 流程（mermaid + Go 伪代码）
6. 后台计划任务 / async worker 清单
7. 鉴权与会话（JWT / MFA / CSRF / CORS / headers）
8. 幂等 / Saga 实现细节
9. 密钥管理 / 信封加密
10. 审计 + 哈希链 audit_log 实施
11. 错误码 / 错误模型
12. 观测（Prometheus + SLS + trace_id）
13. 配置 + 部署（env / ConfigMap / Secret / health / shutdown）
14. 数据库迁移策略
15. 测试策略 + invariants
16. 性能预算
17. Phase 切片
18. 风险 + ADR
19. 与 overview/integration-design 的"待 align"清单

---

## 1. 总览：12 模块 → Go package 树

> 引用：`overview` §3.1（package 矩阵已定）；本节落到具体文件级骨架，标注每个文件的 owner、Phase、≤400 行的拆分原则。

### 1.1 仓库根

```
TraceNexBiz/                     # repo root（已有 docs/ prd/ reviews/ 目录）
├── cmd/
│   ├── api/                     # gin 入口（HTTP server）
│   ├── outbox-poller/           # 独立进程：消费 fy_api_db.consume_log_outbox（§9.3）
│   ├── settlement-runner/       # Cron leader：月结 / 周结
│   ├── audit-sealer/            # 单实例 leader：哈希链 sealer（HIGH-r2-1）
│   ├── notification-dispatcher/ # 通知 outbox 消费者（邮件/SMS/webhook）
│   ├── kek-rotator/             # KEK 轮换 + DEK 重加密 batch 任务
│   └── migrate/                 # golang-migrate runner
├── internal/                    # 应用内私有 package
│   ├── app/                     # DI 容器、wire 装配
│   ├── config/                  # env + biz_setting 读取
│   ├── auth/                    # JWT 验证、MFA、scope、permission middleware
│   ├── permission/              # 权限 enum + 矩阵 + lint
│   ├── partner/                 # 渠道商 domain（M3, M4-01..02, 04..06）
│   ├── customer/                # 终端客户 domain（M2, M9）
│   ├── wallet/                  # 钱包 + wallet_hold + 应付台账
│   ├── pricing/                 # partner_pricing_rule + 解析器
│   ├── revenue/                 # revenue_log + outbox 消费侧 domain（不含 poller 进程）
│   ├── settlement/              # 月结 / 周结 / 个税
│   ├── kyc/                     # KYC 流程 + OCR + KMS
│   ├── invoice/                 # 全电发票
│   ├── payment/                 # 持牌方 SDK + ISV mchid
│   ├── ticket/                  # 工单
│   ├── notify/                  # 事件中心 + dispatcher
│   ├── content_safety/          # 输入/输出审核
│   ├── pipl_rights/             # 用户权利中心（M13）
│   ├── platform/                # 平台 staff 视图 + reporting
│   ├── audit/                   # audit_log + audit_log_pii + sealer 接口
│   ├── idempotency/             # idempotency_record + middleware
│   ├── saga/                    # saga_step + 编排器 + retry worker
│   ├── outbox/                  # outbox poller + DLQ（partner-api 侧）
│   ├── fyapi_client/            # Fy-api /api/internal/* SDK（HMAC + retry）
│   ├── kms_envelope/            # KMS 封装 + DEK cache
│   ├── pubsub/                  # Redis Pub/Sub 订阅
│   └── webhook/                 # 持牌方支付回调入口
├── pkg/                         # 跨进程公共
│   ├── errors/                  # 错误模型
│   ├── tracing/                 # trace_id 注入
│   ├── validator/               # 自定义 validator
│   ├── decimal/                 # shopspring/decimal 包装
│   ├── pii/                     # `pii:"true"` redactor
│   └── ginx/                    # 通用 gin 中间件
├── openapi/                     # OpenAPI 3.1 spec
│   ├── partner-api.yaml         # 对前端 REST API
│   └── payment-callback.yaml    # 持牌方 webhook
├── migrations/                  # golang-migrate SQL（partner_db DDL 主路径）
├── scripts/                     # ops / CI 脚本
├── deploy/                      # K8s manifests / Helm chart
└── test/                        # e2e / integration test 顶层
```

> 文件级 lint：每个 Go 文件 ≤ 400 行（通用），handler 文件 ≤ 200 行；超过即重构。golangci-lint 自定义检查 `funlen=80, lll=120`。

### 1.2 模块 → Phase 表

> 摘自 `overview` §3.2 + §8；本节只标"该模块的 partner-api 入口在 Phase 几进可运行集"。

| Module | Owner Pkg | Phase 1 | Phase 2A | Phase 2B | Phase 3 |
|---|---|:---:|:---:|:---:|:---:|
| M1 公开商城 | view-only via `pricing` + `customer` | M1-04/06/09 | M1-07/08 | — | M1-03 |
| M2 客户后台 | `customer`, `wallet (view)` | basic + M2-13 | M2-03/04 | — | M2-06/10/15 |
| M3 渠道商后台 | `partner`, `wallet`, `pricing`, `revenue` | M3-01..04, 09, 13 | M3-05, 08, 12, 14 | — | M3-06, 11 |
| M4 平台后台 | `platform` (cross-pkg view) | M4-01..07, 15 | M4-03, 08..10, 12, 16, 17 | M4-11, 13 | — |
| M5 分润结算 | `settlement` | — | — | M5-01, 04..11 | M5-09 完整 |
| M6 支付 | `payment`, `webhook` | — | M6-01..08 | M6-09 | — |
| M7 KYC | `kyc`, `kms_envelope`, `pipl_rights` | stub UI + consent_log | M7-01..10 | — | — |
| M8 发票 | `invoice` | — | — | M8-01..06 | — |
| M9 防绕过 | `customer` | M9-01/02 | — | — | M9-04 |
| M10 工单 | `ticket` | M10-01..04 basic | — | — | full |
| M11 通知 | `notify` | email + inapp | sms + webhook | — | — |
| M12 内容安全 | `content_safety` | M12-01/02 basic | M12-02..06 | — | — |
| M13 用户权利 | `pipl_rights` | — | M13-01..05 | — | — |

---

## 2. 代码组织 / 分层

> 引用：PRD pattern.md "Repository Pattern"；本节定义五层 + 命名 + DI + 错误传播。

### 2.1 分层

```
┌────────────────────────────────────────────────────┐
│ Layer 1: handler (gin)                             │
│   - 解析 DTO / 调用 service / 翻译错误 / 写响应     │
│   - 不含业务逻辑；不可直接调 repository            │
│   - 文件：internal/{domain}/handler.go             │
├────────────────────────────────────────────────────┤
│ Layer 2: service                                   │
│   - 业务规则 / saga 编排 / 调多个 repository       │
│   - 不依赖 gin、不依赖 GORM 类型                   │
│   - 接口在 service.go；实现在 service_impl.go      │
├────────────────────────────────────────────────────┤
│ Layer 3: domain (entity + value object + state)    │
│   - 纯 Go struct + 不可变方法 + state 机定义       │
│   - 文件：internal/{domain}/state.go +             │
│           internal/{domain}/model.go               │
├────────────────────────────────────────────────────┤
│ Layer 4: repository                                │
│   - GORM v2 → MySQL；隔离 SQL/scope 注入           │
│   - 接口在 repository.go；实现在 repository_gorm.go │
│   - 返回 domain entity（不返 GORM model）          │
├────────────────────────────────────────────────────┤
│ Layer 5: infra                                     │
│   - bizDB / fyDB / Redis / KMS / OSS / SLS / Fy-api │
│     client 等 driver-specific 实现                 │
│   - 文件：internal/infra/* + pkg/*                 │
└────────────────────────────────────────────────────┘
```

**严格规则**：

1. handler **永远不**直接调 repository；service 是唯一入口
2. repository 返回的 domain entity **immutable**，业务态变更走 `Update*` 系列方法（参考 §8.x，每个 entity 有 `WithStatus(status)` 等返回新拷贝的方法）
3. 跨 domain 调用走 service 接口，**不**跨 package 触发 repository（`overview` I-3.1）
4. `internal/fyapi_client` 是唯一允许调 Fy-api `/api/internal/*` 的 package（grep gate, `overview` I-3.3）

### 2.2 命名约定

| 元素 | 约定 | 例 |
|---|---|---|
| Go package | snake_case 单数 | `internal/wallet` |
| domain entity struct | PascalCase 单数 | `Partner` `WalletHold` |
| service interface | `XxxService` | `WalletService` |
| service impl | `xxxServiceImpl` | `walletServiceImpl` |
| repository interface | `XxxRepo` | `PartnerRepo` |
| repository impl | `xxxGormRepo` | `partnerGormRepo` |
| handler 方法 | `Handle{Verb}{Resource}` | `HandleAllocateQuota` |
| Mutation method | `With{Field}` 返回新拷贝 | `p.WithStatus(StatusActive)` |
| state 常量 | `{Domain}Status{Name}` | `PartnerStatusActive` |
| DTO struct | `XxxRequest` / `XxxResponse` | `AllocateQuotaRequest` |

### 2.3 DI 方式

`google/wire` 编译期 DI（不用 reflect-based 容器）。`internal/app/wire.go` 为 wire set 唯一入口；每个 domain 在 `internal/{domain}/wire.go` 暴露自己的 ProviderSet。

构造函数签名：

```go
func NewWalletService(
    repo wallet.Repo,
    partnerRepo partner.Repo,
    fyClient fyapi_client.Client,
    saga saga.Orchestrator,
    audit audit.Writer,
    clock clockwork.Clock,
) wallet.Service { ... }
```

> Clock 抽象用 `jonboulle/clockwork`，便于测试时控制时间。

### 2.4 错误传播规则

> 引用：`overview` §11.3 错误码区段；`integration-design` §8 错误映射。

```go
// pkg/errors/errors.go
type AppError struct {
    Code      string            // BIZ_*
    HTTPStatus int
    MessageZH string
    MessageEN string
    TraceID   string
    Details   map[string]any
    Cause     error              // 内部 wrap，不外露
}

func (e *AppError) Error() string { ... }
func (e *AppError) Unwrap() error { return e.Cause }
```

- repository 层只返 `gorm.ErrRecordNotFound` / SQL error / `pkg/errors` wrapping，不返业务码
- service 层把基础 error **翻译**为 `AppError`（业务码）；重要：404 / 越权统一返 `BIZ_RES_NOT_FOUND` 不区分（PRD §16.3）
- handler 层 `errors.As(&AppError)` 提取出 code，按 §11 错误响应 envelope 输出
- 不允许 swallow：每条 `if err != nil` 必须 return 或 wrap；CI 启用 `errcheck` + `errorlint`

---

## 3. partner_db 全部表 DDL

> 引用：PRD §8.1 ~ §8.20。本节给 MySQL 8 InnoDB 可执行 DDL，含索引、CHECK、外键、注释、字符集。每张表注明 **owner package**、**Phase**、**访问规则**。
>
> 字符集统一 `utf8mb4` + collation `utf8mb4_0900_ai_ci`；金额一律 `BIGINT`；时间 `TIMESTAMP(3)` 默认 `CURRENT_TIMESTAMP(3)`。`updated_at` 用 `ON UPDATE CURRENT_TIMESTAMP(3)`。
>
> 外键策略：仅在 partner_db 内部建外键；跨 fy_api_db 的 `fy_user_id` / `fy_api_log_id` **不**建 FK（跨库 FK 在 MySQL 不可用，且不愿 GORM 跨连接 join）。
>
> CHECK 约束：MySQL 8.0.16+ 支持，本文使用；migration 仅生成 MySQL，PG/SQLite 走 `bizDB` 连接 GORM AutoMigrate（dev 环境 fallback）。

> ⚠️ 全部表均有 `id BIGINT PRIMARY KEY AUTO_INCREMENT`、`created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)`、`updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)`。除 `audit_log` 类追加只写表外都有 `deleted_at TIMESTAMP(3) NULL`。下文若未重复声明则按此默认。

### 3.1 `partner` （PRD §8.1）

- Owner: `internal/partner` ｜ Phase 1 ｜ tnbiz_app RW

```sql
CREATE TABLE partner (
    id                    BIGINT NOT NULL AUTO_INCREMENT,
    fy_user_id            BIGINT NOT NULL COMMENT '关联 fy_api_db.users.id（不建 FK，跨库）',
    invitation_code       VARCHAR(64) NOT NULL COMMENT '渠道商专属邀请码（≥16 字符高熵）',
    status                VARCHAR(32) NOT NULL DEFAULT 'applied' COMMENT '见 PRD §14.1',
    kyc_type              TINYINT NOT NULL DEFAULT 0 COMMENT '0=未认证 1=企业 2=个人',
    kyc_status            TINYINT NOT NULL DEFAULT 0 COMMENT '0=未提交 1=待审核 2=通过 3=驳回 4=年审中',
    kyc_expires_at        TIMESTAMP(3) NULL COMMENT '年审到期',
    default_revenue_share DECIMAL(6,4) NOT NULL DEFAULT 0.0000 COMMENT '兼容旧字段（v1.0 实际用 partner_pricing_rule）',
    tier                  TINYINT NOT NULL DEFAULT 0 COMMENT 'markup tier 0-9（per-tier group 命名）',
    applied_at            TIMESTAMP(3) NOT NULL,
    approved_at           TIMESTAMP(3) NULL,
    approved_by           BIGINT NULL COMMENT '→ staff.id',
    contact_name          VARCHAR(64),
    contact_phone_cipher  VARBINARY(512) COMMENT '§19 信封加密（手机号）',
    contact_phone_key_id  VARCHAR(128) COMMENT 'KMS DEK 句柄（含 keyVersion）',
    contact_email         VARCHAR(128),
    contact_email_hmac    CHAR(64) COMMENT 'v0.2 ARCH M-8.1：HMAC(email) 搜索索引，admin 搜不再全表扫',
    tax_status            VARCHAR(32) NOT NULL DEFAULT 'unknown' COMMENT 'v0.2 Compliance HIGH-1：individual | sole_proprietor | individual_business | company | unknown',
    notes                 TEXT,
    settlement_provider_id BIGINT NULL,
    provider_sub_account_id VARCHAR(128) DEFAULT '',
    frozen_at             TIMESTAMP(3) NULL,
    frozen_reason         TEXT,
    terminated_at         TIMESTAMP(3) NULL,
    terminated_reason     TEXT,
    created_at            TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at            TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    deleted_at            TIMESTAMP(3) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_partner_fy_user (fy_user_id),
    UNIQUE KEY uk_partner_invitation_code (invitation_code),
    UNIQUE KEY uk_partner_email_hmac (contact_email_hmac), -- v0.2 ARCH M-8.1
    KEY idx_partner_status (status, deleted_at),
    KEY idx_partner_kyc (kyc_status, deleted_at),
    CONSTRAINT chk_partner_status CHECK (status IN ('applied','reviewing','approved','rejected','frozen','suspended','terminated')),
    CONSTRAINT chk_partner_tier CHECK (tier BETWEEN 0 AND 9),
    CONSTRAINT chk_partner_tax_status CHECK (tax_status IN ('individual','sole_proprietor','individual_business','company','unknown'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='渠道商主表';
```

### 3.2 `customer` （PRD §8.2）

- Owner: `internal/customer` ｜ Phase 1 ｜ tnbiz_app RW

```sql
CREATE TABLE customer (
    id                  BIGINT NOT NULL AUTO_INCREMENT,
    fy_user_id          BIGINT NOT NULL,
    partner_id          BIGINT NULL COMMENT 'NULL = 直营客户',
    joined_via          VARCHAR(32) NOT NULL DEFAULT 'invitation',
    invitation_code_used VARCHAR(64),
    status              VARCHAR(32) NOT NULL DEFAULT 'active' COMMENT '见 PRD §14.2',
    group_name_in_fy_api VARCHAR(128) NOT NULL DEFAULT 'default' COMMENT 'partner_X_tier_Y',
    quota_limit         BIGINT NOT NULL DEFAULT 0 COMMENT '0 = 不限',
    transferred_from    BIGINT NULL,
    transferred_at      TIMESTAMP(3) NULL,
    created_at          TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at          TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    deleted_at          TIMESTAMP(3) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_customer_fy_user (fy_user_id),
    KEY idx_customer_partner (partner_id, deleted_at),
    KEY idx_customer_status (status, partner_id),
    CONSTRAINT fk_customer_partner FOREIGN KEY (partner_id) REFERENCES partner(id),
    CONSTRAINT chk_customer_status CHECK (status IN ('active','disabled','transferred','orphaned','adopted','direct','deleted')),
    CONSTRAINT chk_customer_joined CHECK (joined_via IN ('invitation','manual_create','self_register_with_code','direct'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='终端客户主表';
```

### 3.3 `partner_wallet` （PRD §8.3）

- Owner: `internal/wallet` ｜ Phase 1 ｜ tnbiz_app RW

> **v0.2 决策（ARCH D-5 / M-4 verdict）**：**drop `held_amount`**。"可用余额" = `balance - SUM(wallet_hold.amount WHERE status='held' AND partner_id=?)`；`idx_hold_partner_held(partner_id, status, held_at)` 支持此查询 sub-ms（实测 10 万行 <2ms）。理由：单一 source of truth；避免 daily drift checker 噪声；wallet dashboard 一次 JOIN 即可。替代 invariant 由集成测试保障（见 §15.2 I-W-7）。

```sql
CREATE TABLE partner_wallet (
    id              BIGINT NOT NULL AUTO_INCREMENT,
    partner_id      BIGINT NOT NULL,
    balance         BIGINT NOT NULL DEFAULT 0 COMMENT '应付台账（quota 单位）',
    -- held_amount   DROPPED in v0.2（ARCH D-5 / M-4 verdict）
    paid_out_total  BIGINT NOT NULL DEFAULT 0,
    version         BIGINT NOT NULL DEFAULT 0 COMMENT '乐观锁',
    created_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_wallet_partner (partner_id),
    CONSTRAINT fk_wallet_partner FOREIGN KEY (partner_id) REFERENCES partner(id),
    CONSTRAINT chk_wallet_amounts CHECK (paid_out_total >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='渠道商应付台账；held 由 wallet_hold 计算';
```

### 3.4 `partner_wallet_log` （PRD §8.4，v0.2 UNIQUE 粒度决议）

- Owner: `internal/wallet` ｜ Phase 1 ｜ append-mostly（status 可 update）

> **v0.2 决策（ARCH HIGH-8 / #9 verdict）**：保留 `UNIQUE(idempotency_key, type)` 双键。理由：**同一 saga 在退款场景下会产生一组对偶 log**（如 `refund_clawback` + `adjustment` 复合操作共用同 idem_key 但 type 不同）；若退回单键 UNIQUE(idempotency_key)，saga 本身需拆出两个独立 idem_key，语义上破坏了"一个 saga 一个 idem_key"的单调约束。CHECK type 枚举严格限制可组合的 type 对，防止误写（见下）。新增 `platform_isv_commission_in` type（Compliance HIGH-5）。

```sql
CREATE TABLE partner_wallet_log (
    id               BIGINT NOT NULL AUTO_INCREMENT,
    partner_id       BIGINT NOT NULL,
    type             VARCHAR(32) NOT NULL,
    amount           BIGINT NOT NULL,
    balance_after    BIGINT NOT NULL,
    ref_id           VARCHAR(128) NOT NULL DEFAULT '',
    idempotency_key  VARCHAR(64) NOT NULL,
    status           VARCHAR(32) NOT NULL DEFAULT 'committed',
    note             TEXT,
    operator_type    VARCHAR(32) NOT NULL,
    operator_id      BIGINT NOT NULL DEFAULT 0,
    trace_id         VARCHAR(64) NOT NULL DEFAULT '',
    created_at       TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_wallet_log_idem (idempotency_key, type),
    KEY idx_wallet_log_partner_time (partner_id, created_at),
    KEY idx_wallet_log_ref (ref_id),
    CONSTRAINT fk_wallet_log_partner FOREIGN KEY (partner_id) REFERENCES partner(id),
    CONSTRAINT chk_wallet_log_type CHECK (type IN (
        'revenue_accrual','allocate_to_customer','settlement_payout','refund_clawback',
        'adjustment','saga_aborted_unknown','initial_topup',
        'platform_isv_commission_in'  -- v0.2 Compliance HIGH-5：ISV 佣金独立台账
    ))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='钱包流水（append-mostly）';
```

### 3.5 `wallet_hold` （PRD §8.5）

- Owner: `internal/wallet` ｜ Phase 1 ｜ tnbiz_app RW

```sql
CREATE TABLE wallet_hold (
    id           BIGINT NOT NULL AUTO_INCREMENT,
    wallet_id    BIGINT NOT NULL,
    partner_id   BIGINT NOT NULL,
    amount       BIGINT NOT NULL,
    saga_id      VARCHAR(64) NOT NULL COMMENT '= idempotency_key',
    status       VARCHAR(16) NOT NULL DEFAULT 'held',
    held_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    resolved_at  TIMESTAMP(3) NULL,
    created_at   TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at   TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_hold_saga (saga_id),
    KEY idx_hold_wallet_status (wallet_id, status),
    KEY idx_hold_partner_held (partner_id, status, held_at),
    CONSTRAINT fk_hold_wallet FOREIGN KEY (wallet_id) REFERENCES partner_wallet(id),
    CONSTRAINT fk_hold_partner FOREIGN KEY (partner_id) REFERENCES partner(id),
    CONSTRAINT chk_hold_amount CHECK (amount > 0),
    CONSTRAINT chk_hold_status CHECK (status IN ('held','committed','released'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='钱包 hold 两阶段表';
```

### 3.6 `partner_pricing_rule` （PRD §8.6）

- Owner: `internal/pricing` ｜ Phase 1 (单层) / Phase 2A (多层)

```sql
CREATE TABLE partner_pricing_rule (
    id           BIGINT NOT NULL AUTO_INCREMENT,
    partner_id   BIGINT NOT NULL,
    customer_id  BIGINT NULL COMMENT 'NULL = partner 默认',
    model_name   VARCHAR(128) NULL COMMENT 'NULL = 全模型',
    tier_name    VARCHAR(64) NULL,
    -- v0.2 ARCH 8.6：MySQL UNIQUE 在含 NULL 列上不去重；用 generated column 把 NULL 转成 '*' sentinel 解决
    customer_id_canon VARCHAR(32) AS (IFNULL(CAST(customer_id AS CHAR), '*')) STORED,
    model_name_canon  VARCHAR(128) AS (IFNULL(model_name, '*')) STORED,
    tier_name_canon   VARCHAR(64)  AS (IFNULL(tier_name, '*')) STORED,
    markup       DECIMAL(10,4) NOT NULL COMMENT 'shopspring/decimal',
    valid_from   TIMESTAMP(3) NOT NULL,
    valid_to     TIMESTAMP(3) NULL,
    status       VARCHAR(16) NOT NULL DEFAULT 'active',
    created_by   BIGINT NOT NULL,
    note         TEXT,
    created_at   TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at   TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    deleted_at   TIMESTAMP(3) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_pricing_rule_canon (partner_id, customer_id_canon, model_name_canon, tier_name_canon, valid_from),
    KEY idx_pricing_partner_active (partner_id, status, valid_from, valid_to),
    KEY idx_pricing_customer (customer_id, valid_from),
    CONSTRAINT fk_pricing_partner FOREIGN KEY (partner_id) REFERENCES partner(id),
    CONSTRAINT chk_pricing_markup CHECK (markup >= 1.0 AND markup <= 5.0), -- v0.2 SEC LOW-9：上界从 100 收紧到 5
    CONSTRAINT chk_pricing_status CHECK (status IN ('active','archived','draft'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='渠道商定价规则（多维度，v0.2 generated column 解 NULL 唯一约束失效）';
```

> service 层校验 `valid_from < valid_to` 与重叠（PM MEDIUM-2）；v0.2 ARCH 8.6 通过 generated column 解决 MySQL `NULL ≠ NULL` 语义导致 UNIQUE 失效问题。

### 3.7 `revenue_log` （PRD §8.7）

- Owner: `internal/revenue` ｜ Phase 1（无 settlement_id）/ Phase 2B（接 settlement）

```sql
CREATE TABLE revenue_log (
    id              BIGINT NOT NULL AUTO_INCREMENT,
    partner_id      BIGINT NOT NULL,
    customer_id     BIGINT NOT NULL,
    fy_api_log_id   BIGINT NOT NULL COMMENT '关联 fy_api_db.logs.id（不建 FK）',
    occurrence      TINYINT NOT NULL DEFAULT 1 COMMENT '1=正常 2+=显式调整',
    gross_amount    BIGINT NOT NULL,
    cost_amount     BIGINT NOT NULL,
    net_revenue     BIGINT NOT NULL,
    applied_rule_id BIGINT NOT NULL,
    occurred_at     TIMESTAMP(3) NOT NULL,
    settlement_id   BIGINT NULL,
    trace_id        VARCHAR(64) NOT NULL DEFAULT '',
    created_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_revenue_log (fy_api_log_id, occurrence),
    KEY idx_revenue_partner_time (partner_id, occurred_at),
    KEY idx_revenue_customer_time (customer_id, occurred_at),
    KEY idx_revenue_settlement (settlement_id),
    CONSTRAINT fk_revenue_partner FOREIGN KEY (partner_id) REFERENCES partner(id),
    CONSTRAINT fk_revenue_customer FOREIGN KEY (customer_id) REFERENCES customer(id),
    CONSTRAINT fk_revenue_rule FOREIGN KEY (applied_rule_id) REFERENCES partner_pricing_rule(id),
    CONSTRAINT chk_revenue_occurrence CHECK (occurrence BETWEEN 1 AND 127)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='收益记录（outbox 消费写入）';
```

### 3.8 `settlement` / `settlement_item` / `settlement_run` / `settlement_config_change_log` （PRD §8.8）

- Owner: `internal/settlement` ｜ Phase 2B

```sql
CREATE TABLE settlement (
    id               BIGINT NOT NULL AUTO_INCREMENT,
    period           VARCHAR(32) NOT NULL,
    period_start     TIMESTAMP(3) NOT NULL,
    period_end       TIMESTAMP(3) NOT NULL,
    timezone         VARCHAR(32) NOT NULL DEFAULT 'Asia/Shanghai',
    total_revenue    BIGINT NOT NULL DEFAULT 0,
    total_cost       BIGINT NOT NULL DEFAULT 0,
    total_payout     BIGINT NOT NULL DEFAULT 0,
    status           VARCHAR(32) NOT NULL DEFAULT 'generating',
    progress_offset  BIGINT NOT NULL DEFAULT 0,
    generated_at     TIMESTAMP(3) NULL,
    paid_at          TIMESTAMP(3) NULL,
    paid_by          BIGINT NULL,
    payment_evidence TEXT,
    created_at       TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at       TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_settlement_period (period),
    KEY idx_settlement_status (status),
    CONSTRAINT chk_settlement_status CHECK (status IN ('generating','generated','paying','paid','failed','partially_disputed','gate_failed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE settlement_item (
    id                BIGINT NOT NULL AUTO_INCREMENT,
    settlement_id     BIGINT NOT NULL,
    partner_id        BIGINT NOT NULL,
    revenue           BIGINT NOT NULL,
    cost              BIGINT NOT NULL,
    platform_fee      BIGINT NOT NULL,
    withheld_tax      BIGINT NOT NULL DEFAULT 0,
    payout            BIGINT NOT NULL,
    tax_evidence_url  VARCHAR(1024),
    status            VARCHAR(16) NOT NULL DEFAULT 'pending',
    provider_trade_no VARCHAR(128),
    payout_evidence   TEXT,
    invoice_id        BIGINT NULL,
    is_partial        TINYINT(1) NOT NULL DEFAULT 0,
    created_at        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_settlement_item (settlement_id, partner_id),
    KEY idx_settlement_item_partner (partner_id, status),
    CONSTRAINT fk_si_settlement FOREIGN KEY (settlement_id) REFERENCES settlement(id),
    CONSTRAINT fk_si_partner FOREIGN KEY (partner_id) REFERENCES partner(id),
    CONSTRAINT chk_si_status CHECK (status IN ('pending','paid','disputed','failed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE settlement_run (
    id              BIGINT NOT NULL AUTO_INCREMENT,
    settlement_id   BIGINT NOT NULL,
    hostname        VARCHAR(128) NOT NULL,
    pid             INT NOT NULL,
    started_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    last_heartbeat  TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    lease_expires_at TIMESTAMP(3) NOT NULL COMMENT 'Redis SETNX 续约对应',
    ended_at        TIMESTAMP(3) NULL,
    status          VARCHAR(16) NOT NULL DEFAULT 'running',
    PRIMARY KEY (id),
    KEY idx_settlement_run_settlement (settlement_id),
    CONSTRAINT fk_sr_settlement FOREIGN KEY (settlement_id) REFERENCES settlement(id),
    CONSTRAINT chk_sr_status CHECK (status IN ('running','completed','crashed','taken_over'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE settlement_config_change_log (
    id            BIGINT NOT NULL AUTO_INCREMENT,
    changed_by    BIGINT NOT NULL,
    old_period    VARCHAR(32),
    new_period    VARCHAR(32),
    effective_from TIMESTAMP(3) NOT NULL,
    reason        TEXT,
    created_at    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.9 `kyc_application` （PRD §8.9）

- Owner: `internal/kyc` ｜ Phase 1 stub / Phase 2A 完整

```sql
CREATE TABLE kyc_application (
    id                       BIGINT NOT NULL AUTO_INCREMENT,
    fy_user_id               BIGINT NOT NULL,
    type                     TINYINT NOT NULL,
    status                   VARCHAR(32) NOT NULL DEFAULT 'draft',
    business_license_url     VARCHAR(1024),
    business_license_ocr_cipher VARBINARY(16384) COMMENT 'v0.2 Security MED-13：OCR 结果包含法人/公司名须加密',
    business_license_ocr_key_id VARCHAR(128),
    legal_person_name_cipher VARBINARY(512),
    legal_person_name_key_id VARCHAR(128),
    legal_person_name_blind_index CHAR(64) COMMENT 'v0.2 Security HIGH-11：HMAC(SECRET, name)；可查重不可逆',
    legal_person_id_cipher   VARBINARY(512),
    legal_person_id_key_id   VARCHAR(128),
    legal_person_id_blind_index CHAR(64) COMMENT 'v0.2 Security HIGH-11',
    legal_person_id_url      VARCHAR(1024),
    legal_person_id_archive_url VARCHAR(1024),
    alipay_open_id_cipher    VARBINARY(512),
    alipay_open_id_key_id    VARCHAR(128),
    alipay_real_name_cipher  VARBINARY(512),
    alipay_real_name_key_id  VARCHAR(128),
    bank_account_cipher      VARBINARY(512) COMMENT 'Phase 2A 提现账号',
    bank_account_key_id      VARCHAR(128),
    bank_account_blind_index CHAR(64) COMMENT 'v0.2 Security HIGH-11',
    -- encryption_key_id     DROPPED in v0.2（ARCH M-9 / Security MED-10：字段级 key_id 已足够；表级冗余）
    biometric_liveness_url   VARCHAR(1024) COMMENT 'v0.2 Security HIGH-10：完成认证后立即清（cron biometric.purge）',
    biometric_purged_at      TIMESTAMP(3) NULL,
    yearly_reject_count      TINYINT NOT NULL DEFAULT 0 COMMENT 'v0.2 PM MEDIUM-7：场景 L 每自然年驳回计数；cron yearly 清零；service 检测 ≥ 3 触发冻结',
    yearly_reject_reset_at   TIMESTAMP(3) NULL COMMENT 'v0.2：年度重置基准',
    submitted_at             TIMESTAMP(3) NULL,
    reviewed_at              TIMESTAMP(3) NULL,
    reviewed_by              BIGINT NULL,
    reject_reason_code       VARCHAR(64),
    reject_reason_text       TEXT,
    pii_purged_at            TIMESTAMP(3) NULL COMMENT '热归档 30d 后清明文 PII',
    cold_archive_expires_at  TIMESTAMP(3) NULL COMMENT '冷归档 5y 后由 kyc.purge.cold cron 彻底销毁',
    created_at               TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at               TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    deleted_at               TIMESTAMP(3) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_kyc_fy_user (fy_user_id),
    KEY idx_kyc_status (status),
    KEY idx_kyc_purge_hot (pii_purged_at),
    KEY idx_kyc_purge_cold (cold_archive_expires_at), -- v0.2 Compliance CRIT-3
    KEY idx_kyc_biometric_purge (biometric_purged_at), -- v0.2 Security HIGH-10
    KEY idx_kyc_legal_id_blind (legal_person_id_blind_index),
    KEY idx_kyc_bank_acct_blind (bank_account_blind_index),
    CONSTRAINT chk_kyc_type CHECK (type IN (1,2)),
    CONSTRAINT chk_kyc_status CHECK (status IN ('draft','submitted','under_review','approved','rejected','expiring','expired','frozen_yearly_limit'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='KYC 申请（PII 信封加密 + blind index）';
```

### 3.10 `invitation_code` （PRD §8.10）

- Owner: `internal/partner` ｜ Phase 1

```sql
CREATE TABLE invitation_code (
    id          BIGINT NOT NULL AUTO_INCREMENT,
    partner_id  BIGINT NOT NULL,
    code        VARCHAR(64) NOT NULL,
    type        VARCHAR(16) NOT NULL DEFAULT 'permanent',
    usage_limit INT NOT NULL DEFAULT 0 COMMENT '0 = 不限',
    used_count  INT NOT NULL DEFAULT 0,
    expires_at  TIMESTAMP(3) NULL,
    status      VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at  TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at  TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_invitation_code (code),
    KEY idx_invitation_partner (partner_id, status),
    CONSTRAINT fk_invitation_partner FOREIGN KEY (partner_id) REFERENCES partner(id),
    CONSTRAINT chk_invitation_type CHECK (type IN ('permanent','one_time','limited')),
    CONSTRAINT chk_invitation_status CHECK (status IN ('active','expired','revoked'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.11 `seat` （PRD §8.11）

- Owner: `internal/customer`（v1.0）/ 后续 `internal/seat` ｜ Phase 3

```sql
CREATE TABLE seat (
    id           BIGINT NOT NULL AUTO_INCREMENT,
    owner_type   VARCHAR(16) NOT NULL,
    owner_id     BIGINT NOT NULL,
    name         VARCHAR(128),
    fy_token_id  BIGINT NOT NULL,
    purchased_at TIMESTAMP(3) NOT NULL,
    expires_at   TIMESTAMP(3) NOT NULL,
    status       VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at   TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at   TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_seat_owner (owner_type, owner_id, status),
    UNIQUE KEY uk_seat_token (fy_token_id),
    CONSTRAINT chk_seat_owner_type CHECK (owner_type IN ('partner','customer')),
    CONSTRAINT chk_seat_status CHECK (status IN ('active','expired','revoked'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.12 `invoice_application` + `invoice_title` （PRD §8.12）

- Owner: `internal/invoice` ｜ Phase 2B

```sql
CREATE TABLE invoice_title (
    id          BIGINT NOT NULL AUTO_INCREMENT,
    owner_type  VARCHAR(16) NOT NULL,
    owner_id    BIGINT NOT NULL,
    title_type  TINYINT NOT NULL COMMENT '1=个人 2=企业',
    title       VARCHAR(255) NOT NULL,
    tax_number  VARCHAR(64),
    bank_info   TEXT,
    is_default  TINYINT(1) NOT NULL DEFAULT 0,
    created_at  TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at  TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    deleted_at  TIMESTAMP(3) NULL,
    PRIMARY KEY (id),
    KEY idx_title_owner (owner_type, owner_id, is_default),
    CONSTRAINT chk_title_owner_type CHECK (owner_type IN ('partner','customer')),
    CONSTRAINT chk_title_type CHECK (title_type IN (1,2))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE invoice_application (
    id                  BIGINT NOT NULL AUTO_INCREMENT,
    applicant_type      VARCHAR(16) NOT NULL,
    applicant_id        BIGINT NOT NULL,
    title_id            BIGINT NOT NULL,
    seller_entity_id    BIGINT NOT NULL COMMENT 'v0.2 Compliance HIGH-6 / C-5：销售方主体（biz_setting 中注册）',
    seller_tax_no       VARCHAR(64) NOT NULL COMMENT 'v0.2 Compliance HIGH-6 / C-5：销售方统一社会信用代码',
    amount              BIGINT NOT NULL,
    period              VARCHAR(32),
    status              VARCHAR(32) NOT NULL DEFAULT 'applied',
    invoice_url         VARCHAR(1024),
    fapiao_serial       VARCHAR(64) COMMENT '全电发票流水号',
    mail_address        VARCHAR(512),
    red_flush_request_id BIGINT NULL COMMENT 'v0.2 Compliance MED-17：关联红字发票申请单',
    applied_at          TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    issued_at           TIMESTAMP(3) NULL,
    archive_expires_at  TIMESTAMP(3) NOT NULL COMMENT 'v0.2 Compliance HIGH-6：≥ 10y 电子会计档案留存',
    notes               TEXT,
    reject_reason_code  VARCHAR(64),
    reject_reason_text  TEXT,
    created_at          TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at          TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    deleted_at          TIMESTAMP(3) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_invoice_fapiao (fapiao_serial), -- v0.2 ARCH 8.12：全局唯一防红冲误匹配
    KEY idx_invoice_app_applicant (applicant_type, applicant_id, status),
    CONSTRAINT fk_invoice_title FOREIGN KEY (title_id) REFERENCES invoice_title(id),
    CONSTRAINT chk_invoice_status CHECK (status IN ('applied','reviewing','issuing','issued','rejected','red_flushing','red_flushed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- v0.2 Compliance MED-17：红字发票申请单（红冲必须经税局《信息确认单》流程）
CREATE TABLE red_flush_request (
    id                  BIGINT NOT NULL AUTO_INCREMENT,
    original_invoice_id BIGINT NOT NULL,
    red_fapiao_serial   VARCHAR(64) COMMENT '红字发票流水号（税局下发后填写）',
    reason_code         VARCHAR(64) NOT NULL,
    reason_text         TEXT,
    status              VARCHAR(32) NOT NULL DEFAULT 'applied',
    requested_by        BIGINT NOT NULL COMMENT 'staff.id',
    requested_at        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    confirmed_at        TIMESTAMP(3) NULL COMMENT '税局信息确认单下发时间',
    completed_at        TIMESTAMP(3) NULL,
    created_at          TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_red_flush_original (original_invoice_id),
    CONSTRAINT fk_red_flush_invoice FOREIGN KEY (original_invoice_id) REFERENCES invoice_application(id),
    CONSTRAINT chk_red_flush_status CHECK (status IN ('applied','awaiting_tax_confirm','confirmed','completed','rejected'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.13 `audit_log` + `audit_log_pii` + `audit_log_unsealed` （PRD §8.13，HIGH-r2-1 重写）

- Owner: `internal/audit` ｜ Phase 1
- 访问规则：
  - `tnbiz_app`：`INSERT (excluding prev_hash, self_hash) ON audit_log_unsealed`；`SELECT ON audit_log / audit_log_pii / audit_log_unsealed`
  - `tnbiz_audit_sealer`：`SELECT, DELETE ON audit_log_unsealed`；`INSERT ON audit_log`；`SELECT ON audit_log_pii`
  - 任何 user 都**无** `UPDATE / DELETE` on `audit_log`

```sql
-- 1) 应用 INSERT 进 unsealed 队列
CREATE TABLE audit_log_unsealed (
    id            BIGINT NOT NULL AUTO_INCREMENT,
    actor_type    VARCHAR(16) NOT NULL,
    actor_id      BIGINT NOT NULL,
    action        VARCHAR(64) NOT NULL,
    target_type   VARCHAR(32) NOT NULL,
    target_id     BIGINT NOT NULL DEFAULT 0 COMMENT 'v0.2 Security HIGH-12：数值型 target',
    target_key    VARCHAR(128) NOT NULL DEFAULT '' COMMENT 'v0.2 Security HIGH-12：string 型 target（如 biz_setting key）',
    diff_redacted TEXT,
    diff_pii_id   BIGINT NULL,
    ip_address    VARCHAR(64),
    user_agent    VARCHAR(512),
    trace_id      VARCHAR(64) NOT NULL DEFAULT '',
    second_approver_id BIGINT NULL COMMENT 'v0.2 Security CRIT-5：dual-control 第二审批人；actor_id != second_approver_id 校验',
    occurred_at   TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='应用 INSERT 队列；sealer 消费';

-- 2) sealer 写入哈希链最终表
-- v0.2 ARCH-HIGH-4 / D-2：audit_log.id 非 AUTO_INCREMENT，由 sealer 把 unsealed.id 原样拷贝过来，保证 1:1 对齐
CREATE TABLE audit_log (
    id            BIGINT NOT NULL,                -- 与 unsealed.id 1:1（非 AUTO_INCREMENT；sealer 赋值）
    actor_type    VARCHAR(16) NOT NULL,
    actor_id      BIGINT NOT NULL,
    action        VARCHAR(64) NOT NULL,
    target_type   VARCHAR(32) NOT NULL,
    target_id     BIGINT NOT NULL DEFAULT 0,
    target_key    VARCHAR(128) NOT NULL DEFAULT '', -- v0.2 Security HIGH-12
    diff_redacted TEXT,
    diff_pii_id   BIGINT NULL,
    ip_address    VARCHAR(64),
    user_agent    VARCHAR(512),
    trace_id      VARCHAR(64) NOT NULL DEFAULT '',
    second_approver_id BIGINT NULL,
    occurred_at   TIMESTAMP(3) NOT NULL,
    prev_hash     CHAR(64) NOT NULL,              -- sha256 hex
    self_hash     CHAR(64) NOT NULL,
    sealed_at     TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_audit_actor (actor_type, actor_id, occurred_at),
    KEY idx_audit_target (target_type, target_id, occurred_at),
    KEY idx_audit_target_key (target_type, target_key, occurred_at), -- v0.2
    KEY idx_audit_action (action, occurred_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='追加只写 + 哈希链；DML 仅 INSERT';

CREATE TABLE audit_log_pii (
    id              BIGINT NOT NULL AUTO_INCREMENT,
    diff_cipher     VARBINARY(65535) NOT NULL COMMENT 'v0.2 Compliance M-19：8KB → 64KB 容纳 OCR 结果；"重 PII" (≥64KB) 外推到 OSS 引用字段',
    diff_oss_ref    VARCHAR(1024) COMMENT 'v0.2 Compliance M-19：超长 PII 指向 OSS 加密对象 key',
    encryption_key_id VARCHAR(128) NOT NULL,
    tombstoned_at   TIMESTAMP(3) NULL COMMENT 'PIPL §47 删除后置位',
    created_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='PII 加密侧表，可 PIPL 删除而不破坏哈希链';
```

> sealer 算法见 §10。

### 3.14 `staff` （PRD §8.14）

- Owner: `internal/auth` ｜ Phase 1

```sql
CREATE TABLE staff (
    id              BIGINT NOT NULL AUTO_INCREMENT,
    username        VARCHAR(64) NOT NULL,
    password_hash   VARCHAR(255) NOT NULL COMMENT 'argon2id',
    role            VARCHAR(32) NOT NULL,
    email           VARCHAR(128) NOT NULL,
    status          VARCHAR(16) NOT NULL DEFAULT 'active',
    last_login      TIMESTAMP(3) NULL,
    mfa_secret_cipher VARBINARY(512),
    mfa_secret_key_id VARCHAR(128),
    webauthn_creds  JSON COMMENT 'multi credential',
    elevated_until  TIMESTAMP(3) NULL COMMENT 'step-up MFA 单次授权窗口（≤15min）',
    created_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    deleted_at      TIMESTAMP(3) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_staff_username (username),
    KEY idx_staff_role (role, status),
    CONSTRAINT chk_staff_role CHECK (role IN ('super_admin','operations','finance','support')),
    CONSTRAINT chk_staff_status CHECK (status IN ('active','disabled','locked'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.15 `biz_setting` （PRD §8.15；v0.2 ARCH/SEC CRIT-7 加 value_type）

- Owner: `internal/config` ｜ Phase 1

```sql
CREATE TABLE biz_setting (
    `key`        VARCHAR(128) NOT NULL,
    `value`      TEXT NOT NULL,
    value_type   VARCHAR(16) NOT NULL DEFAULT 'plain' COMMENT 'v0.2 SEC CRIT-7：plain | secret_ref（secret_ref 的 value 仅存 KMS Secret ARN 或 env key 名）',
    description  VARCHAR(512),
    updated_at   TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    updated_by   BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (`key`),
    CONSTRAINT chk_biz_setting_value_type CHECK (value_type IN ('plain','secret_ref'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

**关键 key 注册**（v0.2 新增 `internal/config/keys.go` machine-readable enum + startup validate，ARCH D-3 / §8.15）：

| key | value_type | Phase | 说明 |
|---|---|---|---|
| `compliance.icp_record_no` | plain | 2 | ICP 备案号（Compliance CRIT-1） |
| `compliance.icp_license_no` | plain | 2 | ICP 经营许可证号 |
| `compliance.public_security_filing_no` | plain | 2 | 公网安备号 |
| `compliance.gen_ai_filing_no` | plain | 2 | 生成式 AI 服务提供者备案号 |
| `compliance.algorithm_filing_no` | plain | 2 | 算法备案号 |
| `compliance.deep_synthesis_filing_no` | plain | 2A+ | 深度合成备案号（若上架深合成模型才需） |
| `compliance.dpo_contact_email` | plain | 1 | DPO 邮箱（PIPL §52） |
| `compliance.dpo_contact_phone` | plain | 1 | DPO 电话 |
| `compliance.report_phone_12377_link` | plain | 2A | 12377 + 公司专用举报通道（Compliance CRIT-2） |
| `compliance.icp_license_active` / `compliance.gen_ai_filing_active` / `compliance.algorithm_filing_active` / `compliance.deep_synthesis_filing_active` / `compliance.epd_2_filing_active` / `compliance.licensed_provider_active` | plain (bool) | gated | readiness probe gate（overview §8.5） |
| `compliance.pia_report_latest_at` | plain (date) | 2A+ | 最近一次 PIA 有效时间 |
| `payment.platform_isv_mchid` | plain | 2A | Compliance M-2：ISV 佣金 mchid；webhook 收款方校验 |
| `refund_window_days` | plain (int) | 1 | Q6 决策占位（默认 7） |
| `saga_wall_clock_hours` | plain (int) | 1 | saga 上限（默认 1）；启动 assert 必须 ≤ idempotency_ttl_hours |
| `idempotency_ttl_hours` | plain (int) | 1 | 默认 24；启动 assert ≤ internal_idempotency_ttl_days × 24 |
| `internal_idempotency_ttl_days` | plain (int) | 1 | 默认 7 |
| `jwt_verify_key_pem` | **secret_ref** | 1 | **v0.2 SEC CRIT-7**：secret_ref，value 指向 KMS Secret ARN；实际 JWT 公钥从 KMS Secret Manager 注入到 env `JWT_VERIFY_KEY_PEM`；super_admin **不能**通过普通 `PUT /admin/biz-settings/{key}` 改此类 key，必须走 `.security` 权限（见 §7.4 v0.2）|

**启动 validate**（ARCH D-3 / SEC CRIT-7）：
- `config.MustLoadAndValidate()`：缺 key 或类型不符则 panic
- `saga_wall_clock_hours ≤ idempotency_ttl_hours ≤ internal_idempotency_ttl_days × 24`
- 所有 `secret_ref` value 必须解析为合法 KMS Secret ARN
- `compliance.*_active` 在 prod 且 Phase ≥ 2 时必须为 true，否则 readiness probe 失败

### 3.16 `idempotency_record` （PRD §8.16，Security M-r2-5 修订）

- Owner: `internal/idempotency` ｜ Phase 1

```sql
CREATE TABLE idempotency_record (
    id              BIGINT NOT NULL AUTO_INCREMENT,
    actor_type      VARCHAR(16) NOT NULL,
    actor_id        BIGINT NOT NULL,
    idempotency_key VARCHAR(64) NOT NULL,
    endpoint        VARCHAR(128) NOT NULL,
    request_hash    CHAR(64) NOT NULL,
    response_status INT NOT NULL,
    response_hash   CHAR(64) NOT NULL,
    response_cipher VARBINARY(16384) NOT NULL COMMENT 'system DEK 加密的 response body（Security M-r2-5）',
    response_key_id VARCHAR(128) NOT NULL,
    trace_id        VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'v1.0 Architect Round-2 cosmetic #1 / M-4：与 audit_log / saga_step / revenue_log 等同步保留 trace_id 字段，便于跨链路追溯重放路径',
    created_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    expires_at      TIMESTAMP(3) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_idem (actor_type, actor_id, idempotency_key, endpoint),
    KEY idx_idem_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='TTL 24h；过期 cron purge；v1.0 加 trace_id 列闭环 Architect Round-2 矩阵 #10';
```

### 3.17 `saga_step` （PRD §8.17 + Security HIGH-r2-2 修订）

- Owner: `internal/saga` ｜ Phase 1

```sql
CREATE TABLE saga_step (
    id              BIGINT NOT NULL AUTO_INCREMENT,
    saga_id         VARCHAR(64) NOT NULL,
    step_name       VARCHAR(64) NOT NULL,
    status          VARCHAR(32) NOT NULL DEFAULT 'pending',
    attempts        INT NOT NULL DEFAULT 0,
    last_error      TEXT,
    payload         TEXT COMMENT 'JSON；§16.6 PII scrubber 必须命中',
    started_at      TIMESTAMP(3) NULL,
    updated_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    escalated_at    TIMESTAMP(3) NULL,
    escalate_reason TEXT,
    trace_id        VARCHAR(64) NOT NULL DEFAULT '',
    created_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_saga_id (saga_id),
    KEY idx_saga_status (status, updated_at),
    UNIQUE KEY uk_saga_step (saga_id, step_name),
    CONSTRAINT chk_saga_status CHECK (status IN ('pending','in_progress','committed','compensated','failed','escalated','released_pessimistic'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.18 `ticket` / `ticket_reply` / `notification_outbox` / `consent_log` （PRD §8.18）

- Owner: `internal/ticket` + `internal/notify` + `internal/kyc`(consent_log) ｜ Phase 1 (basic)

```sql
CREATE TABLE ticket (
    id            BIGINT NOT NULL AUTO_INCREMENT,
    opener_type   VARCHAR(16) NOT NULL,
    opener_id     BIGINT NOT NULL,
    subject       VARCHAR(255) NOT NULL,
    category      VARCHAR(32) NOT NULL,
    status        VARCHAR(32) NOT NULL DEFAULT 'open',
    assigned_to   BIGINT NULL,
    priority      TINYINT NOT NULL DEFAULT 3,
    last_reply_at TIMESTAMP(3) NULL,
    sla_due_at    TIMESTAMP(3) NULL,
    created_at    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    deleted_at    TIMESTAMP(3) NULL,
    PRIMARY KEY (id),
    KEY idx_ticket_assigned (assigned_to, status, sla_due_at),
    KEY idx_ticket_opener (opener_type, opener_id, status),
    CONSTRAINT chk_ticket_status CHECK (status IN ('open','assigned','responding','waiting_user','resolved','closed','reopened')),
    CONSTRAINT chk_ticket_category CHECK (category IN ('billing','kyc','api','content_report','other'))  -- v0.2 ARCH-MED-11：扩枚举
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE ticket_reply (
    id          BIGINT NOT NULL AUTO_INCREMENT,
    ticket_id   BIGINT NOT NULL,
    sender_type VARCHAR(16) NOT NULL,
    sender_id   BIGINT NOT NULL,
    body_md     TEXT NOT NULL,
    attachments JSON,
    created_at  TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_reply_ticket (ticket_id, created_at),
    CONSTRAINT fk_reply_ticket FOREIGN KEY (ticket_id) REFERENCES ticket(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE notification_outbox (
    id            BIGINT NOT NULL AUTO_INCREMENT,
    recipient     VARCHAR(255) NOT NULL,
    channel       VARCHAR(16) NOT NULL,
    event_code    VARCHAR(64) NOT NULL,
    ref_id        VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'v1.0 Architect Round-2 cosmetic #3 / M-2：业务侧关联键（如 saga_id / ticket_id / topup_intent.saga_id），与 (event_code, recipient) 共同构成防重 UNIQUE；为空时退化为旧行为（同事件多次发送）',
    payload       TEXT,
    status        VARCHAR(16) NOT NULL DEFAULT 'pending',
    retry_count   INT NOT NULL DEFAULT 0,
    last_error    TEXT,
    dispatched_at TIMESTAMP(3) NULL,
    trace_id      VARCHAR(64) NOT NULL DEFAULT '',
    created_at    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_notif_pending (status, id),
    UNIQUE KEY uk_notif_dedup (event_code, recipient, ref_id) COMMENT 'v1.0 cosmetic #3 / M-2：防重复推送；同一 (event_code, recipient, ref_id) 三元组只允许一行；ref_id 默认空字符串使无关联事件仍可多次发送（如系统级广播）',
    CONSTRAINT chk_notif_channel CHECK (channel IN ('email','inapp','sms','webhook')),
    CONSTRAINT chk_notif_status CHECK (status IN ('pending','sent','failed','dead_letter'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE consent_log (
    id                   BIGINT NOT NULL AUTO_INCREMENT,
    subject_fy_user_id   BIGINT NOT NULL,
    consent_type         VARCHAR(64) NOT NULL,
    consent_text_version VARCHAR(64) NOT NULL,
    consented_at         TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    ip                   VARCHAR(64),
    user_agent           VARCHAR(512),
    withdrawn            TINYINT(1) NOT NULL DEFAULT 0,
    withdrawn_at         TIMESTAMP(3) NULL,
    PRIMARY KEY (id),
    KEY idx_consent_subject (subject_fy_user_id, consent_type),
    CONSTRAINT chk_consent_type CHECK (consent_type IN (
        'privacy_policy','sensitive_pi','biometric','cross_border','device_fingerprint',
        'automated_decision',  -- v0.2 Compliance HIGH-2 / M-4：PIPL §24 自动化决策同意
        'third_party_share'    -- v0.2 Compliance HIGH-2 / M-4：PIPL §23 向阿里云 OCR / 内容安全等共享
    ))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.19 `consume_log_outbox` （PRD §8.19，**位于 fy_api_db / LOG_DB**）

> 不属于 partner_db。本表 DDL 由 Fy-api 覆盖层提供，详见 `integration-design` §1.5。partner-api `internal/outbox` 通过 LOG_DB 连接消费。本节仅引用，**partner-api 不在 partner_db 复制此表**。

### 3.20 `customer_partner_change_log` （PRD §8.20；v0.2 ARCH 8.20 增 status 列）

- Owner: `internal/customer` ｜ Phase 1（场景 H Phase 2A）

```sql
CREATE TABLE customer_partner_change_log (
    id              BIGINT NOT NULL AUTO_INCREMENT,
    customer_id     BIGINT NOT NULL,
    from_partner_id BIGINT NULL,
    to_partner_id   BIGINT NULL,
    initiator_type  VARCHAR(16) NOT NULL,
    initiator_id    BIGINT NOT NULL,
    status          VARCHAR(32) NOT NULL DEFAULT 'pending_b' COMMENT 'v0.2 ARCH 8.20：三方确认状态机落库',
    reason          TEXT,
    occurred_at     TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    old_group       VARCHAR(128),
    new_group       VARCHAR(128),
    created_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_change_customer (customer_id, occurred_at),
    KEY idx_change_partners (from_partner_id, to_partner_id),
    KEY idx_change_status (status),
    CONSTRAINT fk_change_customer FOREIGN KEY (customer_id) REFERENCES customer(id),
    CONSTRAINT chk_change_initiator CHECK (initiator_type IN ('customer','staff','system_termination')),
    CONSTRAINT chk_change_status CHECK (status IN ('pending_a','pending_b','pending_staff','completed','rejected','cooldown'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.21 `topup_intent` （Phase 2A，PRD §22.1 F-3 saga 占位）

- Owner: `internal/payment` ｜ Phase 2A

```sql
CREATE TABLE topup_intent (
    id            BIGINT NOT NULL AUTO_INCREMENT,
    customer_id   BIGINT NOT NULL,
    amount        BIGINT NOT NULL,
    channel       VARCHAR(32) NOT NULL,
    out_trade_no  VARCHAR(64) NOT NULL,
    state         VARCHAR(32) NOT NULL DEFAULT 'created',
    paid_at       TIMESTAMP(3) NULL,
    funded_at     TIMESTAMP(3) NULL,
    saga_id       VARCHAR(64) NOT NULL COMMENT 'v0.2.1 ARCH-HIGH-NEW-D：UUIDv7 字符串；用作 Idempotency-Key 透传 Fy-api（OpenAPI uuid 契约）；不再用 BIGINT topup_intent.id',
    provider_trade_no VARCHAR(128),
    callback_payload TEXT,
    created_at    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_topup_channel_trade (channel, out_trade_no),
    UNIQUE KEY uk_topup_saga_id (saga_id), -- v0.2.1 ARCH-HIGH-NEW-D：webhook 反查 saga_id 唯一
    KEY idx_topup_customer (customer_id, state),
    CONSTRAINT fk_topup_customer FOREIGN KEY (customer_id) REFERENCES customer(id),
    CONSTRAINT chk_topup_state CHECK (state IN ('created','paid','funded','refunded','failed','canceled'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.22 `partner_debt` （v0.2 ADR-010 verdict：方案 A 落地，**Phase 2A**，Compliance M-3 / HIGH-9 上调）

- Owner: `internal/wallet` ｜ Phase 2A（首次退款上线）

```sql
CREATE TABLE partner_debt (
    id            BIGINT NOT NULL AUTO_INCREMENT,
    partner_id    BIGINT NOT NULL,
    amount        BIGINT NOT NULL COMMENT '欠款（正数）',
    cause         VARCHAR(64) NOT NULL,
    ref_id        VARCHAR(128),
    status        VARCHAR(16) NOT NULL DEFAULT 'open',
    cleared_at    TIMESTAMP(3) NULL,
    created_at    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_debt_partner (partner_id, status),
    CONSTRAINT fk_debt_partner FOREIGN KEY (partner_id) REFERENCES partner(id),
    CONSTRAINT chk_debt_amount CHECK (amount > 0),
    CONSTRAINT chk_debt_status CHECK (status IN ('open','clearing','cleared','written_off'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

> 退款 service（§5.10.2）默认走 partner_debt 路径；负 balance 仅在 P0 紧急 fallback 且必须有阈值告警 + ops runbook（避免被监管解读为"未持牌经营借贷"）。

### 3.23 `content_safety_event` （v0.2 Compliance HIGH-5 / M-10 新增；Phase 2A schema，Phase 1 仍 mock 但表结构本轮 freeze）

```sql
CREATE TABLE content_safety_event (
    id              BIGINT NOT NULL AUTO_INCREMENT,
    fy_user_id      BIGINT NOT NULL,
    kind            VARCHAR(16) NOT NULL,           -- 'input' | 'output'
    provider        VARCHAR(32) NOT NULL,           -- 'aliyun' | 'tencent'
    prompt_hash     CHAR(64) NOT NULL,              -- SHA-256 防重；不存原文
    category        VARCHAR(64) NOT NULL,
    score           DECIMAL(5,4) NOT NULL,
    disposition     VARCHAR(32) NOT NULL,           -- 'block' | 'review' | 'pass' | 'warn'
    reviewed_by     BIGINT NULL,
    reviewed_at     TIMESTAMP(3) NULL,
    reported_to_12377_at TIMESTAMP(3) NULL,
    audit_log_id    BIGINT NULL,
    trace_id        VARCHAR(64) NOT NULL DEFAULT '',
    created_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_csafety_user (fy_user_id, created_at),
    KEY idx_csafety_disposition (disposition, reported_to_12377_at),
    CONSTRAINT chk_csafety_kind CHECK (kind IN ('input','output')),
    CONSTRAINT chk_csafety_provider CHECK (provider IN ('aliyun','tencent','mock'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.24 `content_safety_report` （v0.2 Compliance CRIT-2 / M-11 新增；24h SLA 上报通道）

```sql
CREATE TABLE content_safety_report (
    id                  BIGINT NOT NULL AUTO_INCREMENT,
    event_id            BIGINT NOT NULL,
    target_authority    VARCHAR(32) NOT NULL,       -- '12377' | 'public_security' | 'internal'
    payload             JSON NOT NULL,              -- PRD 附录 E.4 字段：user_id 脱敏 / prompt_hash / 命中类目 / 处置动作
    status              VARCHAR(32) NOT NULL DEFAULT 'pending',
    submitted_at        TIMESTAMP(3) NULL,
    sla_due_at          TIMESTAMP(3) NOT NULL,      -- created_at + 24h；cron content_safety.report.dispatcher 扫
    response_payload    JSON,
    retry_count         INT NOT NULL DEFAULT 0,
    last_error          TEXT,
    created_at          TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at          TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_csreport_pending (status, sla_due_at),
    CONSTRAINT fk_csreport_event FOREIGN KEY (event_id) REFERENCES content_safety_event(id),
    CONSTRAINT chk_csreport_status CHECK (status IN ('pending','submitted','acknowledged','failed','dead_letter')),
    CONSTRAINT chk_csreport_authority CHECK (target_authority IN ('12377','public_security','internal'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.25 `pia_report` （v0.2 Compliance HIGH-7 / C-6 新增；PIPL §55 + GB/T 39335-2020）

```sql
CREATE TABLE pia_report (
    id                  BIGINT NOT NULL AUTO_INCREMENT,
    title               VARCHAR(255) NOT NULL,
    scope               VARCHAR(64) NOT NULL,       -- 'kyc' | 'consent' | 'cross_border' | 'partner_global'
    purpose_text        TEXT NOT NULL,              -- PIA 8 大项 1：处理目的
    scope_text          TEXT NOT NULL,              --       2：处理范围
    necessity_text      TEXT NOT NULL,              --       3：必要性
    legal_basis_text    TEXT NOT NULL,              --       4：合法性基础
    impact_text         TEXT NOT NULL,              --       5：影响
    risk_text           TEXT NOT NULL,              --       6：风险
    measures_text       TEXT NOT NULL,              --       7：措施
    residual_risk_text  TEXT NOT NULL,              --       8：剩余风险
    report_url          VARCHAR(1024),              -- OSS 加密 PDF
    valid_from          TIMESTAMP(3) NOT NULL,
    valid_until         TIMESTAMP(3) NOT NULL,      -- 留档 ≥ 3 年
    signed_by_dpo       BIGINT NOT NULL,
    signed_at           TIMESTAMP(3) NOT NULL,
    created_at          TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_pia_scope_validity (scope, valid_until)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.26 `pipl_complaint` （v0.2 Compliance HIGH-8 / C-8 新增；用户投诉受理通道）

```sql
CREATE TABLE pipl_complaint (
    id                  BIGINT NOT NULL AUTO_INCREMENT,
    subject_fy_user_id  BIGINT NULL,                -- 实名用户填，匿名为 NULL
    contact_email       VARCHAR(128) NOT NULL,
    contact_phone_cipher VARBINARY(512),
    category            VARCHAR(32) NOT NULL,       -- 'erase'|'access'|'rectify'|'consent_withdrawal'|'other'
    description         TEXT NOT NULL,
    status              VARCHAR(32) NOT NULL DEFAULT 'received',
    assigned_to         BIGINT NULL,                -- staff.id (DPO 或代理)
    sla_due_at          TIMESTAMP(3) NOT NULL,      -- created_at + 15 day（PIPL §50）
    resolution_text     TEXT,
    resolved_at         TIMESTAMP(3) NULL,
    audit_log_id        BIGINT NULL,
    trace_id            VARCHAR(64) NOT NULL DEFAULT '',
    created_at          TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at          TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_pipl_complaint_status (status, sla_due_at),
    CONSTRAINT chk_pipl_complaint_category CHECK (category IN ('erase','access','rectify','consent_withdrawal','other')),
    CONSTRAINT chk_pipl_complaint_status CHECK (status IN ('received','reviewing','responded','closed','escalated'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 3.27 `pipl_request` （v0.2.1 ARCH-CRIT-NEW-C 补 DDL；PRD §15.5 + §22.1 F-13；Phase 2A）

> v0.2 §14.3 phase 演进表点名但漏 DDL；v0.2.1 补回。承载 PIPL §44-§47 的 5 类用户权利请求（access / rectify / erase / restrict / port），与 §3.26 `pipl_complaint`（投诉受理）正交：本表是用户**主动行使权利**的工单流；complaint 是对处理结果的投诉。

```sql
CREATE TABLE pipl_request (
    id              BIGINT NOT NULL AUTO_INCREMENT,
    actor_type      VARCHAR(16) NOT NULL,        -- 'customer' | 'partner'
    actor_id        BIGINT NOT NULL,
    fy_user_id      BIGINT NULL,                 -- customer 路径必填，partner 路径 NULL
    request_type    VARCHAR(32) NOT NULL,        -- 'access'|'rectify'|'erase'|'restrict'|'port'
    state           VARCHAR(32) NOT NULL DEFAULT 'submitted',
                                                  -- submitted → id_check → approved → executing → completed | rejected | expired
    deadline        TIMESTAMP(3) NOT NULL,        -- submitted_at + 5d 核身（PRD §15.5）
    completed_deadline TIMESTAMP(3) NOT NULL,     -- submitted_at + 30d（PIPL §50）
    reason          TEXT NULL,
    rejection_reason TEXT NULL,
    export_oss_key  VARCHAR(255) NULL,            -- access / port 导出文件
    audit_log_id    BIGINT NULL,
    trace_id        VARCHAR(64) NOT NULL DEFAULT '',
    submitted_at    TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    completed_at    TIMESTAMP(3) NULL,
    PRIMARY KEY (id),
    KEY idx_pipl_req_actor (actor_type, actor_id, state),
    KEY idx_pipl_req_deadline (state, deadline),
    KEY idx_pipl_req_completed_deadline (state, completed_deadline),
    CONSTRAINT chk_pipl_req_actor_type CHECK (actor_type IN ('customer','partner')),
    CONSTRAINT chk_pipl_req_type CHECK (request_type IN ('access','rectify','erase','restrict','port')),
    CONSTRAINT chk_pipl_req_state CHECK (state IN ('submitted','id_check','approved','executing','completed','rejected','expired'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

> 关键 invariant：`state='submitted' → 'id_check'` 自动跳转（5d 内核身否则 `expired`）；`state='approved' → 'executing'` 由 §5.11 erase saga 拉起；30d 总 SLA（PIPL §50）由 `pia.report.annual` cron 之外的 `pipl_request.deadline.escalate` 监控（每日 04:00 扫，临期 7d 工单告警）。`audit_log_id` 关联完整审计链。

### 3.28 `password_reset_token` （v0.2.1 ARCH-CRIT-NEW-C 补 DDL；PRD §17.5 + backend §7.9；Phase 1）

> v0.2 §7.9 描述了"双因子邮件+SMS 重置 + revoke 全部 jti"，但 §3 缺 token 表 DDL。v0.2.1 补。

```sql
CREATE TABLE password_reset_token (
    id              BIGINT NOT NULL AUTO_INCREMENT,
    actor_type      VARCHAR(16) NOT NULL,        -- 'partner' | 'customer' | 'staff'
    actor_id        BIGINT NOT NULL,
    token_hash      CHAR(64) NOT NULL,            -- SHA-256(随机 32 byte)；不存原 token
    second_factor_type VARCHAR(16) NOT NULL,      -- 'email' | 'sms'；PRD §17.5 双因子的"次因子"
    second_factor_hash CHAR(64) NOT NULL,         -- 次因子 OTP 的 SHA-256（一次性 6 位）
    requested_ip    VARCHAR(45) NOT NULL,
    user_agent      VARCHAR(512),
    expires_at      TIMESTAMP(3) NOT NULL,        -- 15 min TTL
    consumed_at     TIMESTAMP(3) NULL,            -- 单次使用，使用后置位
    failed_attempts INT NOT NULL DEFAULT 0,       -- 第二因子尝试次数；≥ 5 invalidate
    invalidated_at  TIMESTAMP(3) NULL,
    audit_log_id    BIGINT NULL,
    trace_id        VARCHAR(64) NOT NULL DEFAULT '',
    created_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_prt_token_hash (token_hash),    -- 链路签发时 token_hash 全局唯一
    KEY idx_prt_actor (actor_type, actor_id, consumed_at),
    KEY idx_prt_expiry (expires_at),
    CONSTRAINT chk_prt_actor_type CHECK (actor_type IN ('partner','customer','staff')),
    CONSTRAINT chk_prt_factor_type CHECK (second_factor_type IN ('email','sms'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

> 关键 invariant：(a) token 与 second_factor 双双校验通过 + 未过期 + 未消费 + `failed_attempts < 5` 才允许 reset；(b) 成功 reset 后**同步** revoke 该 actor 全部 jti（写 jti revocation list）；(c) 任何 reset 路径写 `audit_log` action='auth.password_reset'；(d) `password_reset.purge` cron 每日清理 `expires_at < NOW() - 7d` 的行（仅保留 audit_log）—— **v1.0 cosmetic #5/#7：cron 已正式登记在 §6 cron 表 `password_reset.purge` 行**（ops `@platform-ops`，每日 04:30），v0.2.2 "ops debt 暂列在注释" 已闭合。

---

## 4. partner-api 对外 REST API（OpenAPI 3.1）

> 引用：`overview` §4.1（API 契约归属）；本节给每个 endpoint 的 `method/path/auth/req/resp/错误码/幂等键/PII/Phase`。完整 spec 在 `openapi/partner-api.yaml`，CI 用 `swagger-cli validate` + `dredd`。命名 kebab-case，资源单数。

### 4.1 公共 component

```yaml
openapi: 3.1.0
info:
  title: TraceNex Partner API
  version: 0.1.0
servers:
  - url: https://api.partner.tracenex.cn
components:
  securitySchemes:
    BearerJWT: { type: http, scheme: bearer, bearerFormat: JWT, description: 'Fy-api JWT (PRD §17.1)' }
  parameters:
    IdempotencyKey: { in: header, name: Idempotency-Key, required: false, schema: { type: string, format: uuid } }
    TraceId:        { in: header, name: X-Oneapi-Request-Id, required: false, schema: { type: string } }
  schemas:
    Envelope:
      type: object
      required: [success]
      properties:
        success: { type: boolean }
        data:    {}
        error:
          type: object
          properties:
            code:       { type: string }
            message_zh: { type: string }
            message_en: { type: string }
            trace_id:   { type: string }
            details:    { type: object }
    Money:    { type: integer, format: int64, minimum: 0, maximum: 1000000000 }
    Decimal4: { type: string, pattern: '^\d+(\.\d{1,4})?$' }
security:
  - BearerJWT: []
```

### 4.2 公开商城 / 招商落地页（Phase 1）

| Method | Path | Auth | Idempotency | PII | Phase |
|---|---|---|---|---|---|
| GET | `/public/models` | none | — | — | 1 |
| GET | `/public/pricing` | none | — | — | 1 |
| POST | `/public/partner/apply` | optional JWT | required | medium (邮箱/姓名/手机号) | 1 |

```yaml
/public/partner/apply:
  post:
    summary: 渠道商招商申请（场景 B）
    parameters: [$ref: '#/components/parameters/IdempotencyKey', $ref: '#/components/parameters/TraceId']
    requestBody:
      required: true
      content:
        application/json:
          schema:
            type: object
            required: [type, contact_name, contact_phone, contact_email, consent_token]
            properties:
              type:          { type: string, enum: [enterprise, individual] }
              business_name: { type: string, maxLength: 255 }
              contact_name:  { type: string, maxLength: 64 }
              contact_phone: { type: string, pattern: '^\+?[0-9]{8,15}$' }   # PII
              contact_email: { type: string, format: email, maxLength: 128 }
              note:          { type: string, maxLength: 1024 }
              consent_token: { type: string, description: 'consent_log id from §15.5 dual-checkbox' }
    responses:
      201:
        description: 已提交，进入审核
        content: { application/json: { schema: { $ref: '#/components/schemas/Envelope' } } }
      409: { description: BIZ_IDEM_REUSED_DIFFERENT_BODY }
```

错误码：`BIZ_VALID_*` / `BIZ_IDEM_*` / `BIZ_RES_NOT_FOUND`（consent_token 不存在）。

### 4.3 终端客户后台

| Method | Path | Auth | Idem | PII | Phase |
|---|---|---|---|---|---|
| GET | `/customer/dashboard` | customer JWT | — | — | 1 |
| GET | `/customer/balance` | customer | — | — | 1 |
| POST | `/customer/topup-intent` | customer | required | low | 2A |
| POST | `/customer/offline-topup` | customer | required | medium (银行回单 URL) | 2A |
| GET | `/customer/api-keys` | customer | — | high (key 仅创建时返回) | 1 |
| POST | `/customer/api-keys` | customer | required | high | 1 |
| DELETE | `/customer/api-keys/{id}` | customer | — | — | 1 |
| GET | `/customer/seats` | customer | — | — | 3 |
| GET | `/customer/billing` | customer | — | — | 1 |
| GET | `/customer/billing/export` | customer | — | medium (账单含模型 / 用量) | 1 |
| POST | `/customer/invoice` | customer | required | medium (抬头 / 税号) | 2B |
| POST | `/customer/tickets` | customer | required | low | 1 |
| GET | `/customer/tickets/{id}` | customer | — | low | 1 |
| POST | `/customer/transfer-request` | customer | required | — | 2A |
| POST | `/customer/pipl/erase-request` | customer | required | — | 2A |
| GET | `/customer/notifications` | customer | — | — | 1 |

```yaml
/customer/api-keys:
  post:
    summary: 创建 API Key（透传 Fy-api /api/internal/token/create）
    parameters: [$ref: '#/components/parameters/IdempotencyKey']
    requestBody:
      required: true
      content:
        application/json:
          schema:
            type: object
            required: [name]
            properties:
              name:            { type: string, maxLength: 64 }
              expired_time:    { type: integer, format: int64, description: "unix seconds; -1=永久" }
              unlimited_quota: { type: boolean, default: false }
              model_limits:    { type: array, items: { type: string } }
              allow_ips:       { type: array, items: { type: string } }
    responses:
      201:
        content:
          application/json:
            schema:
              type: object
              properties:
                success: { type: boolean }
                data:
                  type: object
                  properties:
                    token_id: { type: integer, format: int64 }
                    key:      { type: string, description: '★ 只在创建时返回；partner 永远不可见' }
```

> **越权 pattern**：partner 视角调用 `/customer/api-keys` 一律返 404（即使 customer 隶属于自己）。**v1.0 cosmetic #1（H-7 文档清理）**：partner **不允许**为 customer 创建 API key —— customer 必须从 `/customer/api-keys` 自助开通；`customer.allocate_quota` verb 仅用于充值额度（`POST /partner/customers/{id}/allocate-quota`），**不返 API key**，与本节路径无关。v0.1 末段"partner 端有专门的'为客户开 token'接口走 `customer.allocate_quota` verb"是 v0.1 残留误注释，v1.0 删除。

### 4.4 渠道商后台

| Method | Path | Verb (`§3.4`) | Idem | Phase |
|---|---|---|---|---|
| GET | `/partner/dashboard` | — | — | 1 |
| POST | `/partner/invitations` | partner | required | 1 |
| DELETE | `/partner/invitations/{id}` | invitation.revoke | — | 1 |
| GET | `/partner/customers` | customer.list_own | — | 1 |
| GET | `/partner/customers/{id}` | customer.read | — | 1 |
| POST | `/partner/customers/{id}/allocate-quota` | customer.allocate_quota | **required** | 1 (M3-04) |
| POST | `/partner/customers/{id}/disable` | customer.disable | required | 1 |
| GET | `/partner/wallet` | — | — | 1 |
| GET | `/partner/wallet/holds` | wallet_hold.list | — | 1 |
| GET | `/partner/pricing-rules` | pricing.read_wholesale | — | 1 |
| POST | `/partner/pricing-rules` | pricing.set | required | 2A |
| POST | `/partner/pricing-rules/{id}/archive` | pricing.archive | required | 2A |
| GET | `/partner/statements` | — | — | 1 |
| POST | `/partner/customer-transfer-accept` | partner | required | 2A |

#### 4.4.1 `POST /partner/customers/{id}/allocate-quota`（核心）

```yaml
/partner/customers/{id}/allocate-quota:
  post:
    summary: 渠道商→客户分配额度（M3-04 saga 入口）
    parameters:
      - { in: path, name: id, required: true, schema: { type: integer, format: int64 } }
      - $ref: '#/components/parameters/IdempotencyKey'   # required for this endpoint
      - $ref: '#/components/parameters/TraceId'
    requestBody:
      required: true
      content:
        application/json:
          schema:
            type: object
            required: [amount]
            properties:
              amount: { $ref: '#/components/schemas/Money', minimum: 1 }
              note:   { type: string, maxLength: 256 }
    responses:
      200:
        description: saga committed
        content:
          application/json:
            schema:
              type: object
              properties:
                success: { type: boolean }
                data:
                  type: object
                  properties:
                    customer_balance:  { type: integer, format: int64 }
                    partner_available: { type: integer, format: int64 }
                    saga_id:           { type: string }
      202:
        description: 5xx/timeout 后 saga 进入 retry，前端轮询 /partner/sagas/{saga_id}
      400: { description: BIZ_VALID_AMOUNT_OUT_OF_RANGE }
      402: { description: BIZ_WALLET_INSUFFICIENT_AVAILABLE }
      404: { description: BIZ_RES_NOT_FOUND（customer 不属于该 partner 或不存在） }
      409: { description: BIZ_IDEM_REUSED_DIFFERENT_BODY }
```

PII：none（只有 user_id + amount）。

### 4.5 平台管理后台扩展（staff endpoints）

> 全部 `/admin/*` 路径；JWT 必须含 `staff` 角色 claim；越权一律 404。所有破坏性 verb 须 `Elevated=true`（step-up MFA）。

| Method | Path | Verb | Phase |
|---|---|---|---|
| GET | `/admin/partners` | — | 1 |
| GET | `/admin/partners/{id}` | — | 1 |
| POST | `/admin/partners/{id}/approve` | partner.approve（M4-02 资质审核）| 1 |
| POST | `/admin/kyc/{id}/review` | kyc.review（M4-03，Phase 2A）| 2A |
| POST | `/admin/partners/{id}/suspend` | partner.suspend | 1 |
| POST | `/admin/partners/{id}/terminate` | partner.terminate (super_admin) | 1 |
| POST | `/admin/partners/{id}/wallet/adjust` | wallet.adjust | 1 |
| GET | `/admin/customers` | customer.list_all (🅰) | 1 |
| GET | `/admin/customers/{id}` | customer.read (🅰) | 1 |
| GET | `/admin/customers/orphaned` | customer.list_orphaned (🅰) | 2A v0.2 PM HIGH-2 |
| GET | `/admin/customers/transfers` | customer.transfer_list (🅰) | 2A v0.2 PM MEDIUM-6 |
| POST | `/admin/customer-transfer-approve` | customer.transfer_approve (🅰) | 2A |
| POST | `/admin/refund` | wallet.refund | 2A |
| POST | `/admin/settlement/{id}/mark-paid` | settlement.mark_paid | 2B |
| GET | `/admin/audit-log` | audit_log.read | 1 |
| GET | `/admin/audit-log/verify-chain` | super_admin elevated | 1 |
| POST | `/admin/saga/{id}/force-resolve` | saga.force_resolve (dual-control + 30min cooldown) | 1 |
| GET | `/admin/biz-settings` | system.config_write.trivial 读权限 | 1 |
| PUT | `/admin/biz-settings/{key}` | system.config_write.trivial（白名单 key 列表）| 1 |
| PUT | `/admin/biz-settings/security/{key}` | system.config_write.security（dual-control；jwt_verify_key_pem 等）| 1 |
| GET | `/admin/dlq/outbox` | super_admin | 2A |
| POST | `/admin/dlq/outbox/{id}/replay` | super_admin elevated | 2A |
| POST | `/admin/staff` | staff.create | 1 |
| POST | `/admin/disputes/{id}/arbitrate` | dispute.arbitrate (Phase 3) | 3 |
| GET | `/admin/content-safety/reports` | content_safety.report.read | 2A |
| GET | `/admin/pia` | pia.read | 2A |
| POST | `/admin/pia` | pia.write (DPO) | 2A |
| POST | `/admin/invoices/{id}/red-flush` | invoice.red_flush（v0.2 PM HIGH-3 UI 路径见 frontend）| 2B |
| POST | `/api/me/switch-actor` | me.switch_actor | 1（v0.2 ARCH D-15）|

> v0.2 PM MEDIUM-8：endpoint 拆 `partner.approve`（M4-02，Phase 1）vs `kyc.review`（M4-03，Phase 2A），不再用同名 `approve-kyc`。

#### 4.5.1 `POST /admin/saga/{id}/force-resolve`（v0.2 SEC CRIT-5 重写 dual-control）

> **dual-control 严格约束**（SEC CRIT-5 / ADR-015 v0.2）：
> 1. 必须同时携带两个 staff 的 step-up MFA token；service 层校验两人都在 `elevated_until > now`
> 2. **`approver.staff_id != initiator.staff_id`**（不能同人）
> 3. **`approver.role != initiator.role`**（一人 super_admin、一人 finance；不能两个 super_admin）
> 4. **两审批人 IP 不得同 /24**（防同设备伪造）
> 5. **`second_approver_token` 是一次性的服务端 challenge-response**（不复用 `elevated_until` 窗口；防同 super_admin 连续两次 step-up 冒充两人）
> 6. **30 min 冷却**：同 super_admin 30 min 内已做过 1 次 → 第 2 次必须等冷却或换 super_admin 发起
> 7. 两次审批均独立写 `audit_log_unsealed`（actor_id / second_approver_id 同步）

```go
// internal/saga/handler.go (v0.2)
func (h *Handler) ForceResolve(c *gin.Context) {
    initiator := getActor(c)
    approver, err := parseSecondApproverToken(c.GetHeader("X-Second-Approver-Token"))
    if err != nil { abort403(c, errs.CodeDualControlInvalidToken); return }
    // 6 项检查
    if approver.StaffID == initiator.ActorID { abort403(c, errs.CodeDualControlSamePerson); return }
    if approver.Role == initiator.Role       { abort403(c, errs.CodeDualControlSameRole); return }
    if sameSubnet24(approver.IP, initiator.IP) { abort403(c, errs.CodeDualControlSameSubnet); return }
    if !approver.IsOneTimeChallengeValid()    { abort403(c, errs.CodeDualControlChallengeReused); return }
    if cooldownStillActive(initiator.ActorID, "saga.force_resolve") {
        abort403(c, errs.CodeDualControlCooldown); return
    }
    // ... 真正 resolve；两次 audit_log
}
```

```yaml
/admin/saga/{id}/force-resolve:
  post:
    summary: 解决 saga released_pessimistic 卡死（HIGH-r2-2 + v0.2 SEC CRIT-5）
    parameters:
      - { in: path, name: id, required: true, schema: { type: string } }
      - $ref: '#/components/parameters/IdempotencyKey'
    requestBody:
      content:
        application/json:
          schema:
            type: object
            required: [target_state, second_approver_token, reason]
            properties:
              target_state:           { type: string, enum: [committed, released] }
              second_approver_token:  { type: string, description: 'second staff TOTP/WebAuthn step-up token' }
              reason:                 { type: string, minLength: 16, maxLength: 1024 }
    responses:
      200: { description: ok }
      403: { description: BIZ_PERM_DUAL_CONTROL_REQUIRED }
```

### 4.6 客服 / 工单（Phase 1 basic）

| Method | Path | Verb | Phase |
|---|---|---|---|
| POST | `/tickets` | ticket.create | 1 |
| GET | `/tickets/{id}` | — | 1 |
| POST | `/tickets/{id}/replies` | ticket.reply | 1 |
| POST | `/admin/tickets/{id}/assign` | ticket.assign | 1 |
| POST | `/admin/tickets/{id}/close` | ticket.close | 1 |

### 4.7 通知（Phase 1 inapp + email）

| Method | Path | Phase |
|---|---|---|
| GET | `/notifications` | 1 |
| POST | `/notifications/{id}/read` | 1 |
| GET | `/notifications/preferences` | 2A |
| PUT | `/notifications/preferences` | 2A |

### 4.8 KYC / 实名（Phase 2A 完整；Phase 1 stub）

| Method | Path | Verb | PII | Phase |
|---|---|---|---|---|
| POST | `/kyc/applications` | kyc.submit | high (身份证 / 法人 / 营业执照 URL) | 1 stub / 2A 完整 |
| GET | `/kyc/applications/me` | — | medium | 1 |
| POST | `/kyc/upload-token` | kyc.submit | low (返回 OSS PUT presigned URL) | 2A |
| POST | `/admin/kyc/{id}/review` | kyc.review | — | 1 |
| POST | `/admin/kyc/{id}/export-pii` | kyc.export_pii (🅰 elevated) | high | 2A |

### 4.9 支付 / 充值 / 提现（Phase 2A）

| Method | Path | 备注 |
|---|---|---|
| POST | `/customer/topup-intent` | 创建持牌方支付 intent（4.3 已列） |
| POST | `/webhook/payment/{provider}` | 持牌方异步回调；mTLS + IP 白名单；不走 JWT；走 `openapi/payment-callback.yaml` |
| POST | `/admin/refund` | 4.5 已列 |
| POST | `/admin/payout/replay/{settlement_item_id}` | 持牌方下账失败重试 (Phase 2B) |

### 4.10 发票（Phase 2B）

| Method | Path | Verb |
|---|---|---|
| GET | `/invoice/titles` | — |
| POST | `/invoice/titles` | invoice.title.create |
| POST | `/customer/invoice` | invoice.apply |
| POST | `/admin/invoice/{id}/issue` | invoice.issue |
| POST | `/admin/invoice/{id}/red-flush` | invoice.red_flush (🅰) |

### 4.11 内容安全（Phase 1 basic / Phase 2A 完整）

> 大多在 service 内部调用阿里云内容安全；外露端点很少，主要给 admin 后台审核。

| Method | Path | Phase |
|---|---|---|
| GET | `/admin/content-safety/events` | 2A |
| POST | `/admin/content-safety/events/{id}/disposition` | 2A |
| GET | `/admin/content-safety/models` | 1 |
| PUT | `/admin/content-safety/models/{model}/whitelist` | 1 |

### 4.12 PIPL 用户权利中心（M13，Phase 2A）

| Method | Path | Phase |
|---|---|---|
| GET | `/customer/pipl/data-access-request` | 2A |
| POST | `/customer/pipl/correction-request` | 2A |
| POST | `/customer/pipl/erase-request` | 2A |
| POST | `/customer/pipl/withdraw-consent` | 2A |
| GET | `/customer/pipl/data-portability` | 2A |

---

## 5. 核心 service 流程（mermaid 时序图 + Go 伪代码骨架）

> 引用：`integration-design` §4 saga 详细规约 + PRD §4 业务场景。本节按 PRD 场景给 partner-api 内部如何串联 service / repository。

> 跨服务调用规则（每张图都遵守）：
>
> | 字段 | 值 |
> |---|---|
> | 幂等键 | `Idempotency-Key` UUID v7（客户端生成或 service 生成） |
> | 超时 | partner-api → Fy-api 默认 10s，by-idem-key 探活 3s |
> | 重试 | 仅 5xx / timeout 路径走 saga retry worker（指数退避 2s..300s，total cap 1h） |
> | 补偿 | wallet_hold release / partner_wallet_log 反向 / saga_step compensated |
> | PII 标签 | service 入口处显式标注，参考 §11.4 PII 矩阵 |

### 5.1 渠道商招商入驻（场景 A/B）

```mermaid
sequenceDiagram
    autonumber
    participant UI as partner-web
    participant H as partner.handler
    participant S as partner.Service
    participant FY as fyapi_client
    participant BDB as partner_db
    participant N as notify.Service

    UI->>H: POST /public/partner/apply (consent_token, contacts)
    H->>S: ApplyAsPartner(req)
    S->>BDB: SELECT consent_log WHERE id = consent_token
    alt 场景 B (自助)
        S->>BDB: BEGIN TX
        S->>BDB: UPSERT customer (or 已是 customer)
        S->>BDB: INSERT partner (status='applied', kyc_status=0)
        S->>BDB: INSERT invitation_code (高熵生成)
        S->>BDB: INSERT idempotency_record + audit_log_unsealed
        S->>BDB: COMMIT
        S->>N: enqueue email (event=partner_applied)
    else 场景 A (人工)
        Note over H,S: staff 在 admin 后台 POST /admin/partners 创建；流程同 B 但 status='approved' 直接落
    end
    H-->>UI: 201 + partner_id
```

PII：`contact_phone` / `contact_email`（service 入口标 `pii:"true"`，repository 写入前调 `kms_envelope.Encrypt`）。

```go
// internal/partner/service.go
type Service interface {
    ApplyAsPartner(ctx context.Context, req ApplyRequest) (*Partner, error)
    Approve(ctx context.Context, partnerID int64, by *Staff) error
    Suspend(ctx context.Context, partnerID int64, reason string, by *Staff) error
    Terminate(ctx context.Context, partnerID int64, reason string, by *Staff) error
}

type ApplyRequest struct {
    Type            string `validate:"required,oneof=enterprise individual"`
    BusinessName    string `validate:"max=255"`
    ContactName     string `validate:"max=64" pii:"true"`
    ContactPhone    string `validate:"e164" pii:"true"`
    ContactEmail    string `validate:"email" pii:"true"`
    ConsentToken    int64  `validate:"required"`
    Note            string `validate:"max=1024"`
}
```

**关键 invariant**：

- I-P-1：consent_token 必须在 5 分钟内、未 withdrawn、type 包含 `sensitive_pi`，否则 422
- I-P-2：邀请码必须 ≥ 16 字符且全局唯一（CSPRNG 32 字节 base32）
- I-P-3：partner 行 INSERT 与 invitation_code 在同一 TX

### 5.2 客户被邀请入驻（场景 C）+ 防绕过 §7.9

```mermaid
sequenceDiagram
    autonumber
    participant UI as customer-web
    participant H as customer.handler
    participant CS as customer.Service
    participant FY as fyapi_client
    participant BDB as partner_db
    participant FYDB as fy_api_db (read-only)

    UI->>H: POST /public/register?invitation_code=XYZ
    H->>CS: RegisterWithInvitation(code, profile)
    CS->>BDB: SELECT invitation_code WHERE code=? AND status='active' FOR UPDATE
    alt 已是直营客户（fy_api_db.users.email 命中）
        CS->>FYDB: SELECT users WHERE email=?
        CS-->>UI: 422 BIZ_CUSTOMER_ALREADY_DIRECT (场景 N)
    end
    CS->>FY: POST /api/internal/user/create (group=partner_X_tier_default) +Idempotency-Key
    FY-->>CS: 201 user_id
    CS->>BDB: BEGIN TX
    CS->>BDB: INSERT customer (partner_id=invitation.partner_id, joined_via='invitation')
    CS->>BDB: UPDATE invitation_code SET used_count=used_count+1
    CS->>BDB: INSERT audit_log_unsealed
    CS->>BDB: COMMIT
    H-->>UI: 201 + redirect to partner.tracenex.cn （防绕过强制 brand）
```

> 防绕过（M9-02）：在 `aitracenex.com/topup` 公开路径上加中间件，如果 caller 的 fy_user_id 已绑 partner，重定向到 `partner.tracenex.cn/customer/topup`。

### 5.3 渠道商→客户分配额度（M3-04 saga；v0.2 加 BOLA row-level guard，SEC CRIT-2）

> 详细规约见 `integration-design` §4.3。本节给 partner-api **内部** service 调用骨架。
>
> **v0.2 SEC CRIT-2 强约束**：所有 service 入口的 `in.CustomerID` / `in.PartnerID` 不可信（即使前端 ScopeMiddleware 通过）。**每段 service 代码必须显式 `repo.FindByID(ctx, scope, id)`** + 校验 `entity.PartnerID == scope.PartnerID`，否则 404（不 403，避免暴露存在性）。Repository 接口已强制第二参数为 `ActorContext` 类型（CI golangci 自定义 analyzer 拒绝裸 ID 调用）。

```go
// internal/wallet/service_impl.go
func (s *walletServiceImpl) AllocateToCustomer(ctx context.Context, scope ActorContext, in AllocateInput) (*AllocateResult, error) {
    // v0.2 SEC CRIT-2 row-level guard：先核 customer.partner_id == scope.PartnerID
    cust, err := s.customerRepo.FindByID(ctx, scope, in.CustomerID)
    if err != nil || cust.PartnerID == nil || *cust.PartnerID != scope.PartnerID {
        return nil, errs.NotFound // 404，不暴露 customer 存在
    }
    // 0. 幂等：service 入口 idempotency middleware 已处理；这里假定首次执行
    saga := s.saga.NewSaga(in.IdempotencyKey, "allocate_to_customer")

    // Step 1: wallet hold（同 TX：wallet_hold + saga_step）
    holdRes, err := saga.Run(ctx, "wallet.hold", func(tx *gorm.DB) (any, error) {
        wallet, err := s.walletRepo.LockForUpdate(tx, scope, scope.PartnerID) // 双重 scope
        if err != nil { return nil, err }
        if wallet.Available() < in.Amount {  // Available() = balance - SUM(holds.held)，v0.2 ADR-012
            return nil, errs.New(errs.CodeWalletInsufficient).HTTPStatus(402)
        }
        hold := wallet.NewHold(in.Amount, in.IdempotencyKey)        // 不可变构造
        return hold, s.walletRepo.InsertHold(tx, scope, hold)
    })
    if err != nil { return nil, err }

    // Step 2: call Fy-api topup（带 Idempotency-Key）
    fyRes, err := s.fyClient.Topup(ctx, fyapi_client.TopupReq{
        UserID:         cust.FyUserID,        // 取自服务端 lookup，不取 in
        Amount:         in.Amount,
        IdempotencyKey: in.IdempotencyKey,
        Reason:         "partner_allocate",
    })
    switch {
    case err == nil && fyRes.Status == "succeeded":
        // Step 3: commit hold + decrement balance + log（同 TX）
        return saga.Commit(ctx, "wallet.commit", func(tx *gorm.DB) (any, error) {
            if err := s.walletRepo.CommitHold(tx, scope, in.IdempotencyKey); err != nil { return nil, err }
            if err := s.walletRepo.DecrementBalance(tx, scope, in.Amount); err != nil { return nil, err }
            return s.walletRepo.AppendLog(tx, scope, &WalletLog{
                Type: "allocate_to_customer", Amount: -in.Amount,
                IdempotencyKey: in.IdempotencyKey, RefID: strconv.FormatInt(cust.ID, 10),
                OperatorType: "partner", OperatorID: scope.ActorID,
            })
        })
    case errs.IsClient4xx(err):
        // Step 3': release hold
        return saga.Compensate(ctx, "wallet.release", func(tx *gorm.DB) (any, error) {
            return nil, s.walletRepo.ReleaseHold(tx, scope, in.IdempotencyKey)
        })
    case errs.IsWalletVersionMismatch(err):
        // v0.2 ARCH HIGH A-2：BIZ_WALLET_VERSION_MISMATCH 不计 attempts，重读 wallet 重算
        s.saga.RetryImmediate(ctx, in.IdempotencyKey)
        return &AllocateResult{Status: "pending"}, nil
    default:
        // 5xx / timeout → 入 saga retry worker；返回 202
        s.saga.EnqueueRetry(ctx, in.IdempotencyKey)
        return &AllocateResult{Status: "pending"}, nil
    }
}
```

**v0.2 BOLA pattern reference**（适用所有 service 入口；§5.5 / §5.6 / §5.7 / §5.10 / §5.11 同样必须采用）：

```go
// 反模式（v0.1 残留，禁用）：
// orig := s.someRepo.FindByID(ctx, in.SomeID) // 缺 scope guard

// 正模式（v0.2 强制）：
ent, err := s.someRepo.FindByID(ctx, scope, in.SomeID)
if err != nil || ent.OwnerScopeID() != scope.ActorID {
    return nil, errs.NotFound
}
```

CI gate（§15.3 升级）：自定义 golangci-lint analyzer `bola-scope-required`：所有 `repo.Find*` / `Update*` / `Delete*` 调用第 2 参必须为 `ActorContext` / `*ActorContext` 类型，否则 build fail。

**Mutability 规则**：
- `wallet.NewHold(amount, sagaID)` 返回新 `WalletHold` 值；不修改 wallet 自身
- `walletRepo.LockForUpdate` 返回 entity 拷贝，业务变更必须走 `walletRepo.CommitHold` / `DecrementBalance` 这些显式方法

### 5.4 计费 outbox 消费 → revenue_log（消费 §9.3 outbox）

> 引用：`integration-design` §3.3。partner-api `cmd/outbox-poller` 独立进程；多副本通过 `SKIP LOCKED` 并行。

```mermaid
sequenceDiagram
    autonumber
    participant POL as outbox-poller
    participant LDB as fy_api_db.consume_log_outbox
    participant CACHE as partner_resolver LRU (5min)
    participant PR as pricing.Service
    participant BDB as partner_db.revenue_log

    loop every 1s
        POL->>LDB: BEGIN TX; SELECT * WHERE status='pending' ORDER BY id LIMIT 1000 FOR UPDATE SKIP LOCKED
        loop each row r
            POL->>CACHE: ByFyUserId(r.user_id) → partner_id, customer_id
            alt 直营客户
                POL->>LDB: DELETE outbox row
            else 有 partner
                POL->>PR: Resolve(partner_id, customer_id, model_name, occurred_at)
                POL->>BDB: BEGIN TX; INSERT revenue_log (UNIQUE fy_api_log_id, occurrence) ON DUPLICATE skip; COMMIT
                POL->>LDB: DELETE outbox row
            end
        end
        POL->>LDB: COMMIT TX
    end
```

伪代码骨架（关键 invariant）：

- 失败累积：连续 N=10 次 process error → `status='dead_letter'`、`last_error` 更新；ops PagerDuty
- consumed 行直接 DELETE（不 UPDATE，避免 idx_unconsumed churn，ADR-011）
- partner_resolver 缓存 LRU 5 分钟，缓存 miss 走 `bizDB` lookup `partner.fy_user_id` / `customer.fy_user_id`
- pricing resolver "at-occurrence"：传入 `occurred_at` 取当时生效的 rule（`valid_from <= t < valid_to`）

```go
// internal/outbox/poller.go
type Poller struct {
    logDB  *gorm.DB        // fy_api_db / LOG_DB（GRANT: SELECT, UPDATE(status,...), DELETE）
    bizDB  *gorm.DB
    pricing pricing.Service
    cache  partner.Resolver
    metric metrics.Registry
}

func (p *Poller) Run(ctx context.Context) error {
    t := time.NewTicker(1 * time.Second)
    defer t.Stop()
    for {
        select {
        case <-ctx.Done(): return ctx.Err()
        case <-t.C:
            if err := p.tickOnce(ctx); err != nil {
                p.metric.IncCounter("outbox_tick_error_total", 1)
            }
        }
    }
}
```

### 5.5 settlement 批次生成 + 锁定 + 分账下发 + 回执对账（§7.5 + §8.8）

> 引用：PRD §4.6、`integration-design` §7（freshness gate）、ADR-008。

```mermaid
sequenceDiagram
    autonumber
    participant CRON as settlement-runner (Redis SETNX leader)
    participant BDB as partner_db
    participant LDB as fy_api_db / consume_log_outbox
    participant LIC as 持牌分账方
    participant N as notify.Service

    CRON->>CRON: SET NX cron:settlement:monthly with EX=1800
    alt failed to acquire
        CRON->>CRON: skip; another instance leader
    end
    CRON->>BDB: INSERT settlement_run (lease_expires_at=now+30min)
    CRON->>LDB: freshness gate: any pending outbox <= period_end?  (integration §7)
    CRON->>LDB: outbox lag p95 < 60s past 1h?
    alt gate passed
        CRON->>BDB: INSERT settlement (status='generating')
        loop each partner with revenue_log in period
            CRON->>BDB: aggregate gross/cost; compute platform_fee, withheld_tax
            CRON->>BDB: INSERT settlement_item (status='pending')
            CRON->>BDB: UPDATE settlement.progress_offset
        end
        CRON->>BDB: UPDATE settlement.status='generated'
    else
        CRON->>BDB: UPDATE settlement.status='gate_failed'
        CRON->>N: page on-call
    end

    Note over CRON: 后续阶段（同进程或 staff approve 触发）
    CRON->>BDB: UPDATE settlement.status='paying'
    loop each settlement_item where status='pending'
        CRON->>LIC: Payout(provider_sub_account, amount, idem=settlement_item.id)
        LIC-->>CRON: provider_trade_no / 5xx
        alt success
            CRON->>BDB: UPDATE settlement_item (status='paid', payout_evidence)
            CRON->>BDB: INSERT partner_wallet_log (type='settlement_payout', amount=-payout)
            CRON->>BDB: UPDATE partner_wallet.paid_out_total += payout
        else 5xx
            CRON->>BDB: 留 status='pending'，重试由 retry job 处理
        end
    end
    CRON->>BDB: UPDATE settlement.status='paid' (when all items paid)
    CRON->>N: email partners with bills
```

**v0.2 settlement runner 关键补丁**（PM MEDIUM-5 / Compliance HIGH-1）：

- **场景 O 跨期入驻 `is_partial`**（settlement_item v0.1 schema 已含 `is_partial BOOLEAN`，但 v0.1 runner 伪代码缺判断）：runner 在 aggregate 时若 `partner.approved_at BETWEEN period_start AND period_end` → `settlement_item.is_partial = true`，且 `period_start_for_item = MAX(partner.approved_at, period_start)`；UI badge 在 partner 后台展示"本期为部分计费"
- **场景 F progress_offset 续跑期间新 revenue 归属**：runner 启动时 `CRON->>BDB: SELECT MAX(occurred_at) FROM revenue_log WHERE created_at > settlement_run.lease_expires_at_t0` 得到续跑期间到达的 revenue；明确这些 revenue **算下个 period**（runner 当前 period 已 freshness-gate 通过，新到达视为越过 period 边界），不回写当前 period
- **个税计算实现**（Compliance HIGH-1 / M-15）：`platform_fee = gross × biz_setting.platform_fee_rate`；`withheld_tax = ComputeWithheldTax(net_before_tax, partner.kyc_type, partner.tax_status)`，其中函数实现按 41 号公告分档：
  - `tax_status='individual'` → 劳务报酬累进（20%/30%/40%）
  - `tax_status='sole_proprietor'`/`'individual_business'` → 经营所得（5%-35%）
  - `tax_status='company'` → 不代扣（partner 自行报税）
  - `tax_status='unknown'` → 拒绝 payout 并 page finance
- **银行账户实名一致性校验**（Compliance HIGH 链路 4）：Payout 前 assert `bank_account_blind_index == HMAC(SECRET, kyc_application.legal_person_name + kyc_application.bank_account)`；不一致拒绝 payout（防"以他人账户走帐 / 虚开"）
- **lease 一致性**（ARCH 8.8）：`settlement_run.lease_expires_at` 与 Redis SETNX 双写；heartbeat goroutine 同时 touch 两侧；CI invariant：lease_expires_at > NOW() 时 Redis SETNX 值 == hostname

**关键决策**：
- Cron leader 用 Redis SETNX + 心跳 goroutine 续约（每 30s 续 60s）；Phase 2 评估切换 K8s Lease（ADR-008）
- 续约失败 → 立即放弃所有正在写入的 settlement_run，下个 tick 别的实例接手
- `progress_offset` 记录已处理的最后 partner_id，崩溃续跑

### 5.6 KYC 申请 → 三方核验 → 加密落库

> 引用：PRD §7.7 + §19.3 信封加密 + Compliance NEW-2。

```mermaid
sequenceDiagram
    autonumber
    participant UI as customer-web
    participant H as kyc.handler
    participant S as kyc.Service
    participant OSS as Aliyun OSS (private bucket)
    participant OCR as Aliyun OCR
    participant KMS as Aliyun KMS
    participant BDB as partner_db
    participant CL as consent_log

    UI->>H: POST /kyc/upload-token (file_kind=business_license)
    H->>OSS: presign PUT (TTL 300s, ?content-disposition=...)
    H-->>UI: { upload_url, ossKey }
    UI->>OSS: PUT file directly
    UI->>H: POST /kyc/applications (ossKeys, consent_token)
    H->>S: Submit(req)
    S->>CL: assert consent (sensitive_pi + biometric)
    S->>OCR: ParseBusinessLicense(ossKey)
    OCR-->>S: {company, legal_person, ...}
    S->>KMS: GenerateDataKey(scope=tenant_X)  → {plain_DEK, enc_DEK}
    S->>S: AES-GCM(plain_DEK, legal_person_id_no) → cipher
    S->>BDB: BEGIN TX
    S->>BDB: INSERT kyc_application (cipher, key_id=enc_DEK_handle, status='submitted')
    S->>BDB: INSERT audit_log_unsealed (action='kyc.submit', diff_pii_id=...)
    S->>BDB: COMMIT
    Note over S: plain_DEK 用后 zeroize 内存
    S->>UI: 202
```

**关键 invariant**：
- 上传走 presigned PUT；partner-api 不流过文件字节
- DEK 缓存 key = `(scope=tenant_id, key_id)`（HIGH-r2-2）；缓存 TTL ≤ 1h；prod pprof 关闭
- 30 天后定时 job：清热桶原图、`pii_purged_at = NOW()`、移到 OSS Archive

### 5.7 支付：客户充值 saga（M2-03，F-3 占位）

> 引用：`integration-design` §4.5。partner-api 不持有客户付款；持牌方 webhook 是 funding 触发点。

```mermaid
sequenceDiagram
    autonumber
    participant CU as customer-web
    participant H as payment.handler
    participant S as payment.Service
    participant LIC as 持牌分账方
    participant BDB as partner_db
    participant FY as fyapi_client
    participant W as webhook.handler

    CU->>H: POST /customer/topup-intent (amount, channel)
    H->>S: CreateIntent(req)
    S->>S: saga_id = uuidv7.New()  # v0.2.1 ARCH-HIGH-NEW-D：UUIDv7 而非 BIGINT
    S->>BDB: INSERT topup_intent (state='created', saga_id)
    S->>LIC: PrepareOrder(out_trade_no=intent.id, amount)
    LIC-->>S: pay_url / qr_code
    S-->>CU: pay_url

    CU->>LIC: pay
    LIC->>W: POST /webhook/payment/{provider} (signature)
    W->>W: WebhookIdempotency middleware (provider, signer, event_id) → Redis SETNX  # v0.2.1 ARCH-HIGH-NEW-E
    W->>S: HandleWebhook(payload)
    S->>S: verifySignature + IP allowlist + amount cross-check
    S->>BDB: SELECT topup_intent FOR UPDATE WHERE channel+out_trade_no
    alt amount mismatch / replay
        S-->>W: 400 + ops alert
    end
    S->>BDB: UPDATE topup_intent.state='paid'
    S->>FY: POST /api/internal/user/topup (Idempotency-Key=topup_intent.saga_id)  # UUIDv7 字符串，符合 OpenAPI uuid 契约
    FY-->>S: 200
    S->>BDB: UPDATE topup_intent.state='funded'
    W-->>LIC: 200 ack
```

**关键 invariant**：
- `(channel, out_trade_no) UNIQUE` 防重放
- amount cross-check：`webhook.amount == topup_intent.amount`
- **saga_id = UUIDv7 字符串**（v0.2.1 ARCH-HIGH-NEW-D）：service 层 `CreateIntent` 时生成；写入 `topup_intent.saga_id`（§3.21 UNIQUE）；调 Fy-api `/user/topup` 时 `Idempotency-Key=saga_id`，**符合 §4.1 OpenAPI `IdempotencyKey: format: uuid` 契约**；不再用 BIGINT `topup_intent.id` 当 idem-key
- 5xx → 入 saga retry；webhook 仍 ack（避免持牌方反复推送）
- **v0.2.1 ARCH-HIGH-NEW-E**：webhook 入口走独立 `WebhookIdempotency` 中间件（§7.1 v0.2.1 新增）；基于 (provider, signer, event_id) 的 Redis SETNX 短路重传；与 user-facing `/customer/*` `idempotency_record` 完全隔离
- **v0.2 SEC HIGH-14 / Compliance M-2**：
  - webhook handler 加 `BodyLimit(1 MB)` + JSON parser strict + handler 超时 3s（防 ReDoS / 反序列化攻击）
  - **partner-attributed customer 走 EPay 保护**（PM MEDIUM-3）：充值入口 `channel=epay` 时若 `customer.partner_id != NULL` → 返 `BIZ_PAYMENT_EPAY_NOT_ALLOWED_FOR_PARTNERED`（frontend toast + UI 屏蔽 EPay 选项）
  - **ISV mchid 校验**（Compliance M-2）：webhook 收款方 mchid 等于 `biz_setting.payment.platform_isv_mchid` 时拒收（应该走持牌方分账，不走平台直收，否则被解读为"无证清算"）
  - `topup_intent.callback_payload` 改 cipher（Security MED-7）：`callback_payload_cipher VARBINARY(8192)` + `callback_payload_key_id`

### 5.8 发票申请 → 全电发票 API → 红冲（§7.8）

```mermaid
sequenceDiagram
    autonumber
    participant CU as customer-web
    participant H as invoice.handler
    participant S as invoice.Service
    participant FAPIAO as 全电发票 SDK
    participant BDB as partner_db
    participant N as notify.Service

    CU->>H: POST /customer/invoice (title_id, amount, period)
    H->>S: Apply(req)
    S->>BDB: validate amount <= sum(billing in period not invoiced)
    S->>BDB: INSERT invoice_application (status='applied')
    S-->>CU: 202

    Note over S: staff 审核
    S->>BDB: UPDATE invoice_application.status='issuing'
    S->>FAPIAO: IssueInvoice(title, amount, ...)
    FAPIAO-->>S: pdf_url, fapiao_serial
    S->>BDB: UPDATE invoice_application (status='issued', invoice_url, fapiao_serial)
    S->>N: enqueue email (event=invoice_issued, attach pdf)

    Note over CU,S: 红冲流程（admin）
    CU->>H: POST /admin/invoice/{id}/red-flush (reason)
    H->>S: RedFlush
    S->>FAPIAO: RedFlush(fapiao_serial)
    S->>BDB: UPDATE invoice_application.status='red_flushed' + audit_log
```

**关键 invariant**：
- 红冲不删原 row，新增 status；audit_log 记录
- 个税凭证：`settlement_item.tax_evidence_url` 指向月结跑出的 PDF（M5-10）
- **v0.2 Compliance HIGH-6 / M-16**：发票必须明示 `seller_entity_id` + `seller_tax_no`（销售方主体），由 `biz_setting.invoice.default_seller_entity_id` 注入；红冲走 §3.12 `red_flush_request` 表（监管必经流程）；电子档案 `archive_expires_at = issued_at + 10y`，OSS lifecycle 同步
- **v0.2 PM HIGH-3**：admin frontend 增 `/invoices/:id/red-flush` UI 路径（见 frontend §3.3 v0.2）

### 5.13 账单争议（M5-09 骨架，v0.2 PM HIGH-4 新增；Phase 2B basic / Phase 3 full）

```go
// internal/dispute/service.go (Phase 2B basic)
type Service interface {
    OpenByCustomer(ctx, scope ActorContext, in OpenInput) (*Dispute, error)
    PartnerRespond(ctx, scope, id int64, resp string) error
    StaffArbitrate(ctx, scope, id int64, decision string) error
}
```

DDL（追加到 §3）：

```sql
CREATE TABLE billing_dispute (
    id              BIGINT NOT NULL AUTO_INCREMENT,
    customer_id     BIGINT NOT NULL,
    partner_id      BIGINT NOT NULL,
    revenue_log_id  BIGINT NULL,
    settlement_item_id BIGINT NULL,
    amount          BIGINT NOT NULL,
    reason_text     TEXT NOT NULL,
    state           VARCHAR(32) NOT NULL DEFAULT 'opened', -- opened/awaiting_partner/awaiting_staff/resolved_refund/resolved_reject/closed
    customer_evidence_url VARCHAR(1024),
    partner_response_text TEXT,
    staff_decision  VARCHAR(32),
    sla_due_at      TIMESTAMP(3) NOT NULL,
    created_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_dispute_customer (customer_id, state),
    KEY idx_dispute_partner (partner_id, state),
    CONSTRAINT chk_dispute_state CHECK (state IN ('opened','awaiting_partner','awaiting_staff','resolved_refund','resolved_reject','closed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

Endpoints（§4 v0.2 增）：
- `POST /customer/disputes`（Phase 2B）
- `POST /partner/disputes/{id}/respond`（Phase 2B）
- `POST /admin/disputes/{id}/arbitrate`（Phase 3 full）

### 5.14 渠道商终止 → 客户孤儿化（v0.2 PM HIGH-2 / 场景 I 新增）

cron job `orphan.cleanup`（已加入 §6 worker 表）：每日 02:30 扫 `customer.status='orphaned' AND grace_until < NOW()` → 转 `direct`。`customer` schema 已含 `orphaned/adopted/direct` 枚举（§3.2 v0.1 已就绪），新增字段：

```sql
ALTER TABLE customer ADD COLUMN grace_until TIMESTAMP(3) NULL COMMENT '场景 I：partner 终止后 30d 宽限期 deadline';
```

partner.Service 在 `Terminate(partnerID)` 时：
1. 把所有 `customer WHERE partner_id=P AND status='active'` 改 `orphaned` + `grace_until = NOW() + 30d`
2. enqueue notification（"渠道商已终止，请在 30 天内选新渠道商"）
3. 30d 内若 `customer.transferred_to != NULL` → `adopted`；过期未变则 cron `orphan.cleanup` 转 `direct`

### 5.9 工单（§7.10）+ 通知（§7.11）

```mermaid
sequenceDiagram
    autonumber
    participant U as customer/partner
    participant H as ticket.handler
    participant S as ticket.Service
    participant BDB as partner_db
    participant N as notify.Service
    participant DSP as notification-dispatcher

    U->>H: POST /tickets (subject, category)
    H->>S: Create(req)
    S->>BDB: INSERT ticket (status='open', sla_due_at=now+24h)
    S->>N: enqueue email (event=ticket_opened to support)
    H-->>U: 201

    Note over BDB,DSP: 异步派发
    DSP->>BDB: SELECT FROM notification_outbox WHERE status='pending' LIMIT 100 FOR UPDATE SKIP LOCKED
    DSP->>DSP: render template; send via SMTP / SMS / webhook
    DSP->>BDB: UPDATE status='sent' / 'failed' / 'dead_letter'
```

### 5.10 客户切换渠道商（场景 H）+ revenue 反向（场景 J）

#### 5.10.1 客户转挂

```mermaid
sequenceDiagram
    autonumber
    participant CU as customer-web
    participant H as customer.handler
    participant S as customer.Service
    participant BDB as partner_db
    participant FY as fyapi_client
    participant N as notify.Service

    CU->>H: POST /customer/transfer-request (to_partner_id, reason)
    H->>S: RequestTransfer(req)
    S->>BDB: INSERT customer_partner_change_log (initiator='customer', to=to_partner_id) status='pending_3way'
    S->>N: notify partner A + partner B (event=transfer_requested)
    Note over BDB: partner A POST /partner/customer-transfer-accept (release)
    Note over BDB: partner B POST /partner/customer-transfer-accept (adopt)
    Note over BDB: staff POST /admin/customer-transfer-approve
    S->>BDB: BEGIN TX
    S->>BDB: UPDATE customer SET partner_id=to_partner_id, transferred_from=from, group_name_in_fy_api=new_tier
    S->>FY: PUT /api/internal/user/group (new_tier, Idempotency-Key=change_log.id)
    S->>BDB: UPDATE customer_partner_change_log status='completed'
    S->>BDB: COMMIT
```

> revenue 归属：当月 `revenue_log` 已写的归 partner A 结算；切换时点之后写入的归 B（按 occurred_at 自然分割）。

#### 5.10.2 退款 + revenue 反向（场景 J，参 `integration-design` §4.4）

```go
// internal/wallet/service_impl.go::Refund
func (s *walletServiceImpl) Refund(ctx context.Context, in RefundInput) (*RefundResult, error) {
    orig, err := s.revenueRepo.FindByID(ctx, in.RevenueLogID)
    if err != nil { return nil, err }

    // occurrence 分配：(fy_api_log_id, idempotency_key) 双键 + FOR UPDATE 取 MAX+1（Architect L-1）
    return s.bizDB.Transaction(func(tx *gorm.DB) error {
        next, err := s.revenueRepo.NextOccurrence(tx, orig.FyApiLogID)
        if err != nil { return err }
        rev := orig.WithReversal(next, in.IdempotencyKey)        // 不可变 entity
        if err := s.revenueRepo.Insert(tx, rev); err != nil { return err }

        // Fy-api topup customer back
        if _, err := s.fyClient.Topup(ctx, fyapi_client.TopupReq{
            UserID: orig.CustomerFyUserID, Amount: orig.GrossAmount,
            IdempotencyKey: in.IdempotencyKey, Reason: "refund",
        }); err != nil { return err }

        // partner_wallet 调整：分支与 settlement state
        switch s.settlementStateOf(orig) {
        case "before_settlement":
            return s.walletRepo.AppendLog(tx, &WalletLog{
                Type: "refund_clawback", Amount: -orig.NetRevenue, ...,
            })
        case "settled_paying":
            // settlement_item 标 disputed
            if err := s.settlementRepo.MarkDisputed(tx, orig.SettlementID, orig.PartnerID); err != nil { return err }
            return s.walletRepo.AppendLog(tx, &WalletLog{Type: "refund_clawback", ...})
        case "settled_paid":
            // F-2 决策窗口；v0.1 走负 balance + partner_wallet_log
            return s.walletRepo.AppendLog(tx, &WalletLog{Type: "refund_clawback", ...})
        }
        return nil
    })
}
```

### 5.11 PIPL Article 47 右遗忘（场景 Q）

```mermaid
sequenceDiagram
    autonumber
    participant CU as customer-web
    participant H as pipl.handler
    participant S as pipl_rights.Service
    participant BDB as partner_db
    participant OSS as OSS
    participant FY as fyapi_client
    participant KMS as KMS

    CU->>H: POST /customer/pipl/erase-request
    H->>S: SubmitErase(customer_id)
    S->>BDB: UPDATE pipl_request SET state='id_check', deadline=now+5d
    Note over S: 平台核身（≤5d）；核身通过 staff approval
    S->>BDB: BEGIN TX
    S->>BDB: UPDATE customer.deleted_at = NOW(), status='deleted'
    S->>BDB: UPDATE kyc_application.pii_purged_at=NOW()
    S->>OSS: DeleteObject(business_license_url, legal_person_id_url) (热桶) ; 冷桶保留
    S->>KMS: ScheduleKeyDeletion(DEK handles for this tenant) — 5y 后真正销毁
    S->>BDB: UPDATE audit_log_pii.tombstoned_at=NOW() WHERE diff_pii_id IN (...)
    S->>FY: POST /api/internal/user/erase (user_id, confirm)
    S->>BDB: COMMIT
    H-->>CU: 200
```

> **不删** `audit_log` 主表（保留哈希链）和 `revenue_log`（脱敏 fy_user_id 替换为 -1 等占位）。

---

## 6. 后台计划任务 / async worker 清单

> 引用：`overview` §3.1（cmd 进程清单）+ PRD §10.6 + `integration-design` §3 / §7 / §9。

| Job 名 | 进程 | 触发 | 幂等保证 | 失败补偿 | SLO |
|---|---|---|---|---|---|
| `outbox.poller` | `cmd/outbox-poller` (replicas≥3) | 1s tick | `revenue_log UNIQUE(fy_api_log_id, occurrence)` | retry 10 次 → dead_letter；ops alert | lag p95 < 1s（≤2s） |
| `notification.dispatcher` | `cmd/notification-dispatcher` (replicas≥2) | 5s tick | `notification_outbox.id` natural key + status check | retry 5 → dead_letter；超 7d purge | dispatch p95 < 30s |
| `settlement.runner.monthly` | `cmd/settlement-runner` (replicas=2，1 leader) | Cron `30 2 1 * *` Asia/Shanghai | `settlement.period UNIQUE` + freshness gate | gate_failed 后 ops 手工启动 | < 30 min |
| `settlement.payout.retry` | 同上 | 1min tick | `settlement_item.status='pending'` 自然幂等；持牌方 idem | 5xx 重试，5d 还失败 → ops | item-level p95 < 5s/payout |
| `audit.sealer` | `cmd/audit-sealer` (replicas=1) | 200ms tick | sealer 单 leader（K8s Lease；**v1.0 cosmetic #10**：即便 replicas=1 仍持有 K8s Lease，是为覆盖 pod 重启间隙的 leader 唯一性，而非"多副本互斥"；ops 风险接受方式 = pod 滚动重启时短暂 fallback 5min 由 §10.1 verifier daily 兜底，不靠多副本热备）；`UPDATE WHERE prev_hash IS NULL` | leader 失联 fallback 5min；offline verifier daily | seal lag p95 < 1s |
| `kek.rotator` | `cmd/kek-rotator` (replicas=1) | manual / yearly | per-row idempotent re-encrypt；记录 `progress_offset` | crash → 续跑 | < 24h |
| `dek.rotator` | 同上 | quarterly (90d) per scope | `(scope_id, key_id)` 自然 key | 同上 | — |
| `kyc.purge.hot` | `cmd/api`（go cron 内嵌）| daily 03:00 | `pii_purged_at IS NULL AND submitted_at < now-30d` | retry next day | — |
| `kyc.purge.cold` | 同上 | daily 04:30 | `cold_archive_expires_at < NOW()` 物理销毁 cipher / OSS Archive 对象 / KMS DEK ScheduleKeyDeletion 完结后清账（v0.2 Compliance CRIT-3 / M-7） | retry next day；任何失败 page DPO | 0 漏删（5y 边界） |
| `biometric.purge` | 同上 | every 5min | `biometric_liveness_url IS NOT NULL AND biometric_purged_at IS NULL AND status='approved'`（v0.2 SEC HIGH-10） | retry | 一旦 approved 即清 |
| `model_whitelist.review` | 同上 | monthly 01-01 02:00 | 从 `biz_setting.gen_ai_model_list_url` 拉取最新清单，diff `model_whitelist`，不在的自动 disable + 通知 ops（v0.2 Compliance HIGH-4 / M-9） | manual override | — |
| `content_safety.report.dispatcher` | 同上 | every 1min | `content_safety_report.status='pending' AND sla_due_at < NOW()+10min` 提交 12377/公安网安（v0.2 Compliance CRIT-2 / M-11） | retry 5 → dead_letter + DPO page | < 24h SLA |
| `pia.report.annual` | 同上 | monthly 1st | 检测 `pia_report.valid_until < now+90d` 触发 ops 工单（v0.2 Compliance HIGH-7） | — | 留档 ≥ 3y |
| `tax.withholding.annual` | 同上 | yearly Jan 31 | 41 号公告年度报送（v0.2 Compliance HIGH-1 / M-15）：扫 settlement_item 按 partner.tax_status 计算 + 生成报送 XML | retry 3 → finance 工单 | 1 月底前完成 |
| `kek.rotation.alert` | 同上 | daily | `last_kek_rotated_at < NOW()-380d` → page（v0.2 SEC MED-4） | — | — |
| `idempotency.purge` | 同上 | every 6h | `expires_at < now()` DELETE | — | < 5min |
| `outbox.dlq.alert` | 同上 | 5min tick | gauge `outbox_dead_letter_total` > 0 → page | manual replay UI | — |
| `wallet.drift.checker` | 同上 | daily 04:00 | partner-level `balance == sum(log.amount)` 校验 | drift > 0 page | drift = 0 invariant |
| `saga.retry.worker` | 同上（goroutine pool） | event-driven + 30s sweep | `saga_step.status='in_progress' AND attempts<30 AND now-updated_at>backoff` | 1h cap → `released_pessimistic` + page；`BIZ_WALLET_VERSION_MISMATCH` 不计 attempts 重读重算（v0.2 ARCH A-2） | — |
| `consume_outbox.gc` | `cmd/outbox-poller` 副 goroutine | hourly | （v0.2.2 重述）`outbox.purge` 主路径之外的兜底：清理 `status='dead_letter' AND updated_at < NOW()-30d` 行（DLQ 30d 后归档清理）；与 `outbox.purge` 互不重叠 | — | — |
| **`outbox.purge`** （v0.2.2 ARCH-CRIT-NEW-B 闭环；Phase 1，ops `@platform-ops`） | `cmd/outbox-poller` leader（与 §3.3 ackOne 同进程；K8s Lease / Redis SETNX 与其他 leader 一致） | Cron `15 3 * * *` Asia/Shanghai（每日 03:15 off-peak，避 02:00 settlement / 03:00 kyc.purge.hot） | 触发条件 `SELECT id FROM consume_log_outbox WHERE status='consumed' AND consumed_at < NOW() - INTERVAL 30 DAY LIMIT 10000` 循环到 0 行；每批 `DELETE WHERE id IN (...)` 单事务 ≤ 10k 行；幂等由"行已删则下次扫不到"自然保证 + leader 防多副本并发，崩溃续跑跳过已删行 | 单批失败退避 30s 重试 ≤ 5 → DLQ alert + page **`@platform-ops`**（v1.0 cosmetic #5/#6/N-1：原 v0.2.2 写"DPO page"系 escalation 错位 —— DPO 关心 PIPL 合规与数据销毁审计，不是 outbox 物理 purge 的 oncall；外送给 ops on-call）；连续 3 日跑空（0 行删）告警（疑似 ackOne 未 soft-mark）；连续 3 日残留 `consumed_at < NOW()-31d` 告警 | 单次 < 30 min；残留行数 = 0（与 ADR-011 / integration §3.4 一致） |
| **`password_reset.purge`** （v1.0 cosmetic #5/#7/N-2 / PR-INV-8 闭环；Phase 1，ops `@platform-ops`） | `cmd/api`（go cron 内嵌，与 `kyc.purge.hot` 同 process / 不依赖 leader：纯 stateless purge，行消失即幂等） | Cron `30 4 * * *` Asia/Shanghai（每日 04:30，避开 settlement / kyc.purge.hot 窗口；与 `kyc.purge.cold` 同时段不冲突 —— 两表互不重叠） | `DELETE FROM password_reset_token WHERE expires_at < NOW() - INTERVAL 7 DAY LIMIT 10000` 循环到 0 行；audit_log action='auth.password_reset' 行**永不删**（保留 PIPL §47 / 等保 2.0 审计要求） | 单批失败退避 30s 重试 ≤ 5 → page `@platform-ops`；连续 3 日残留 `expires_at < NOW()-8d` 告警 | 单次 < 5 min；残留行数 = 0；Phase 1 量级 < 100/d/system 下基本 0 累积 |
| `orphan.cleanup` | 同上 | daily 02:30 | `customer.status='orphaned' AND grace_until < NOW()` → 转 direct（v0.2 PM HIGH-2 / 场景 I） | — | — |

**leader 选举**（settlement-runner / audit-sealer / kek-rotator）：

```go
// pkg/leader/redis_setnx.go
func TryAcquire(ctx, rdb, key, owner, ttl) bool { ... }
func RenewLoop(ctx, rdb, key, owner, ttl, interval) { ... }
```

> Phase 1: Redis SETNX；Phase 2 切 K8s Lease（ADR-008）。

---

## 7. 鉴权与会话

> 引用：PRD §17（v1.0 Architect 修订）+ Security C-5 + ADR-007。本节给出 partner-api 中间件实现。

### 7.1 中间件链

每个 router group 顺序：

```
RequestID → CORS → SecurityHeaders → BodyLimit → Tracing
  → JWT (extract claims, jti revocation lookup)
  → Session (CSRF Origin/Referer check on state-changing)
  → Scope (resolve actor: partner / customer / staff)
  → Permission (verb gate, §3.4 enum)
  → Idempotency (if header present and endpoint marks `idempotent: true`)
  → Handler
```

**v0.2.1 ARCH-HIGH-NEW-E：Webhook 路径独立中间件链**（webhook 不持 JWT，permission middleware 跳过；user-facing idempotency 也不适用，因 idem-key 由持牌方控制不可信）：

```
RequestID → SecurityHeaders → BodyLimit(1MB) → Tracing
  → IPAllowlist (持牌方 source IP CIDR 列表)
  → SignatureVerify (持牌方 RSA / HMAC，§5.7)
  → WebhookIdempotency (新增 v0.2.1：基于 (provider, signer, event_id) 三元组；命中直接 200 ack 不触发业务)
  → Handler (业务代码内仍 SELECT FOR UPDATE topup_intent 兜底，但 race 窗口大幅压缩)
```

`WebhookIdempotency` 实现要点：
- **不复用** `idempotency_record` 表（actor_type 模型与 webhook 不匹配）；走 Redis `SETNX webhook:{provider}:{signer}:{event_id} EX 86400`，命中即 ack；持久化路径走 §3.21 `topup_intent.uk_topup_channel_trade` 兜底
- `signer` = 持牌方 mchid 或 partner_no（§5.7 IP allowlist 已绑定身份）；`event_id` = 持牌方回调 ID（无则用 `(out_trade_no, status_change_at)` 派生）
- Redis 不可达 → **fail-open**（仍走 handler，由业务层 SELECT FOR UPDATE 兜底） + alert；与 user-facing idempotency 的 fail-closed 政策**显式不同**（webhook 比内部请求更怕重复处理但又不能拒收持牌方推送）。**v1.0 cosmetic #8 / N-3**：本决策已在 `overview` §10 风险表 A-18 字面登记（不再仅藏在内联注释），与 Architect Round-2 N-3 / Security HIGH-r2 webhook 链审计 §6.2 一致接受 ACCEPT-AS-DEBT；同时**显式声明**：(a) Redis 健康时为 fail-closed（命中即 ack 不再触发业务，与 user-facing 一致）；(b) Redis 故障期为 fail-open（仅依赖业务层 `topup_intent.uk_topup_channel_trade` UNIQUE + handler `SELECT FOR UPDATE` 兜底）；两态切换以 `redis.Ping` 健康检查为准。
- audit：每次命中写 `audit_log` action='webhook.idempotency_hit' + provider + signer + event_id

> **隔离原则**：webhook idempotency 与 §8.1 user-facing idempotency 是**两套独立机制**：(a) 表 / Redis namespace 不共享；(b) 失败策略相反；(c) middleware 链不交叉。文档 §8.1 末段不再为 webhook 服务。

### 7.2 JWT 验证（Fy-api 共享；v0.2 ADR-007 重写：cookie 鉴权 + fail-closed revocation）

```go
// internal/auth/jwt.go
type Claims struct {
    Sub      int64    `json:"sub"`           // fy_user_id
    ActorType string  `json:"actor_type"`    // 'partner' | 'customer' | 'staff'（actor switch 时变更）
    ActorID  int64    `json:"actor_id"`
    Roles    []string `json:"roles"`
    Jti      string   `json:"jti"`
    Iat      int64    `json:"iat"`
    Exp      int64    `json:"exp"`
    Elev     bool     `json:"elev,omitempty"`
    Site     string   `json:"site,omitempty"` // 'partner' | 'customer' | 'admin'（cookie domain 互斥）
}

// v0.2 SEC CRIT-1：从 httpOnly cookie tnbiz_access 读 JWT；不读 Authorization header
// v0.2 SEC CRIT-3：jti revocation lookup fail-closed；Redis 不可达 → 503 + page，不放行
func Middleware(verifier Verifier, redis *redis.Client) gin.HandlerFunc {
    return func(c *gin.Context) {
        tok := extractFromCookie(c, "tnbiz_access")
        if tok == "" {
            // 仅 /api/sdk/* 路径允许 fallback Bearer header（server-to-server，OAuth2 client creds）
            if strings.HasPrefix(c.Request.URL.Path, "/api/sdk/") {
                tok = extractBearer(c)
            }
        }
        if tok == "" { abort401(c); return }
        cl, err := verifier.Verify(tok) // 公钥来自 env JWT_VERIFY_KEY_PEM（KMS Secret Manager 注入；不来自 biz_setting）
        if err != nil { abort401(c); return }
        // jti revocation：fail-closed（v0.2 SEC CRIT-3）
        revoked, err := redis.Exists(c, "revoked:jti:"+cl.Jti).Result()
        if err != nil {
            metrics.RevocationCheckFailed.Inc()
            // 不放行；返 503 + alert
            abort503(c, errs.CodeRevocationStoreUnavailable)
            return
        }
        if revoked > 0 { abort401(c, errs.CodeJWTRevoked); return }
        // KYC pass 强制 logout：staff/partner.kyc_approved_at 后任何 jti iat < approved_at 视为 revoked（D-9）
        if cl.Iat < kycApprovedAt(cl.Sub) { abort401(c, errs.CodeJWTRevoked); return }
        c.Set("fy_user_id", cl.Sub)
        c.Set("jwt_claims", cl)
        c.Next()
    }
}
```

> **JWT verification key**（v0.2 SEC CRIT-7）：从 KMS Secret Manager 注入 env `JWT_VERIFY_KEY_PEM`，不再从 `biz_setting.jwt_public_key_pem` 读。biz_setting 中 `jwt_verify_key_pem` 仅作 secret_ref 占位，super_admin 不能通过普通 `PUT /admin/biz-settings/{key}` 改安全关键 key（拆 `system.config_write.security` verb；见 §7.4）。
>
> **degrade 模式**：Redis 长时间不可达 → ops 通过启动 env `JWT_REVOCATION_BYPASS_TOKEN=<one-time>` 临时降级（每次 bypass 写 audit_log；自动 1h 过期）。

### 7.3 Scope（actor 解析；v0.2 ARCH D-15 加 actor switch endpoint）

```go
// internal/auth/scope.go
type ActorContext struct {
    ActorType string
    ActorID   int64
    FyUserID  int64
    Elevated  bool
    TraceID   string
}

func RequirePartnerScope(repo partner.Repo) gin.HandlerFunc { ... }
func RequireCustomerScope(repo customer.Repo) gin.HandlerFunc { ... }
func RequireStaffScope(repo staff.Repo) gin.HandlerFunc { ... }

// v0.2 ARCH D-15 / SEC：浏览器内同账号在 partner / customer 间切换（前端 `/api/me/switch-actor` 调用）
// 实现：核身后签发新 JWT（actor_type/actor_id 变更）+ 旧 jti 加入 revoked:jti:* + 设新 cookie tnbiz_access
func SwitchActorHandler(...) gin.HandlerFunc { ... }
```

> 若 scope 不匹配 → 404（PRD §16.3 / §3.4 footer）。
> v0.2 ARCH D-18：禁止同 fy_user_id 同时是 partner_X 与 partner_Y 的 customer；注册 / 邀请码入驻时若已是 partner 或已是另一 partner 的 customer → 拒。

### 7.4 Permission middleware（v0.2 增 saga.force_resolve / config_write 拆分）

```go
// internal/permission/middleware.go
type Verb string

const (
    // ... PRD §3.4 已定义 verbs ...
    VerbSagaForceResolve   Verb = "saga.force_resolve"   // v0.2 ARCH CRIT-3 / SEC CRIT-5：dual-control + step-up
    VerbConfigWriteTrivial Verb = "system.config_write.trivial"  // v0.2 SEC CRIT-7：拆分
    VerbConfigWriteSecurity Verb = "system.config_write.security" // v0.2 SEC CRIT-7：dual-control；jwt_verify_key_pem 等
)

func Require(verb Verb) gin.HandlerFunc {
    return func(c *gin.Context) {
        ac := getActor(c)
        if !matrix[verb][ac.ActorType] { abort404(c); return }
        if matrix[verb].RequiresElevated(ac.ActorType) && !ac.Elevated {
            abort403(c, errs.CodeStepUpRequired); return
        }
        if matrix[verb].RequiresDualControl() {
            if !verifyDualControl(c) { abort403(c, errs.CodeDualControlRequired); return }
        }
        c.Next()
    }
}

// v0.2 SEC CRIT-5：dual-control 严格约束
func verifyDualControl(c *gin.Context) bool {
    initiator := getActor(c)
    approver := parseSecondApproverToken(c.GetHeader("X-Second-Approver-Token"))
    if approver == nil { return false }
    if approver.StaffID == initiator.ActorID { return false }                  // 不能同人
    if approver.Role == initiator.Role { return false }                        // 不能同角色
    if sameSubnet24(approver.IP, initiator.IP) { return false }                 // 不能同 /24
    if !approver.IsOneTimeChallengeValid() { return false }                    // challenge-response，一次性
    if cooldownStillActive(initiator.ActorID, verb) { return false }            // 30 min 冷却
    return true
}
```

CI gate（Security M-r2-2）：每个 router 注册时必须 `permission.Require(SomeVerb)`；`grep -r "router.Handle" | xargs check_permission_enum` fail-on-miss。`saga.force_resolve` / `system.config_write.security` 必须 `+ Require(MFAStepUp)` 链式调用。

### 7.5 MFA / Step-up（v0.2 SEC HIGH-4 + WebAuthn 强制）

> partner KYC 通过即强制 TOTP（PRD §17.2 + F-9）。step-up MFA 在 admin 高危 verb：每次单独验证一次，写 `staff.elevated_until = now+15min`；在该窗口内调用 `Require(verb).Elevated()` 通过；超过即拒。
>
> **v0.2 SEC HIGH-4 / M-r2-3**：partner WebAuthn **强制**于以下场景（之前是"可选"）：
> - `partner_wallet.balance > ¥1,000`（wallet 单笔分配 / 充值）
> - `monthly_payout > ¥10,000`（按 settlement_item 累计）
> - 任何 `staff` 高危 verb（`saga.force_resolve` / `system.config_write.security` / `kyc.export_pii` / `customer.read.elevated`）
>
> 强制策略：服务端在 `RequireMFAStepUp` 中间件中校验 staff/partner.webauthn_creds 至少注册 1 个；未注册 → 返 `BIZ_AUTH_WEBAUTHN_REQUIRED` 强制引导前端注册流。

### 7.6 CSRF（v0.2 ARCH-CRIT-6 + SEC double-submit 落地）

```go
// internal/auth/csrf.go
//
// v0.2 双层防御：
//   1) Origin / Referer allowlist（防钓鱼来源）
//   2) double-submit：X-Csrf-Token header == tnbiz_csrf cookie（防 CSRF post）
//
// 两层都必须通过；任一失败即 403 + audit_log。
//
// 与 frontend §6.2 / overview ADR-007 v0.2 严格一致。

func RequireCSRF(allowlist []string) gin.HandlerFunc {
    return func(c *gin.Context) {
        if !isStateChanging(c.Request.Method) { c.Next(); return }
        // Layer 1: Origin / Referer
        origin := c.GetHeader("Origin")
        if origin == "" { origin = c.GetHeader("Referer") }
        if !contains(allowlist, normalizeOrigin(origin)) {
            abort403(c, errs.CodeCSRFOrigin); return
        }
        // Layer 2: double-submit token（v0.2 ARCH-CRIT-6）
        cookieTok, _ := c.Cookie("tnbiz_csrf")
        headerTok := c.GetHeader("X-Csrf-Token")
        if cookieTok == "" || headerTok == "" || !subtle.ConstantTimeEqual([]byte(cookieTok), []byte(headerTok)) {
            abort403(c, errs.CodeCSRFToken); return
        }
        c.Next()
    }
}
```

`SameSite=Lax` cookie；`tnbiz_access` HttpOnly + Secure + Path=/；`tnbiz_csrf` non-HttpOnly（前端读后回放到 header）+ Secure + Path=/；Domain 各子域明示（不用 `.tracenex.cn` 通配）。

### 7.7 CORS

`pkg/ginx/cors.go` 包装 `gin-contrib/cors`，allowlist 来自 `biz_setting.cors_origins`。无通配。`Access-Control-Max-Age` 限 600s。`Access-Control-Allow-Credentials: true`（cookie 鉴权必需）。

### 7.8 安全 headers / Rate-limit / Reverse-bot（v0.2 SEC HIGH-2 全局 rate-limit）

```go
// pkg/ginx/headers.go
func SecurityHeaders() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
        c.Header("X-Frame-Options", "DENY")
        c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
        c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
        c.Header("Content-Security-Policy", buildCSP(c))
        c.Next()
    }
}
```

CSP 用每请求 nonce（注入到 HTML template 上下文）。

**v0.2 全局 rate-limit middleware**（SEC HIGH-2 / M-r2-2 + Compliance HIGH-13）：

| 路径 | 限制 | 维度 | 实现 |
|---|---|---|---|
| `POST /auth/login` | 5/min | (account, IP) | `pkg/ratelimit/redis_token_bucket.go` |
| `POST /auth/refresh` | 30/min | account | 同 |
| `POST /public/partner/apply` | 3/hour | IP | 同 |
| `POST /customer/api-keys` | 10/min | customer | 同 |
| `/api/internal/*` | 100K/24h | (kid, endpoint) | per-kid quota，integration §6.3 |
| 内容安全推理（输入审核 endpoint）| 100/min | tenant (partner_id) | Compliance HIGH-13；防费用放大 |
| 默认 anon | 60/min | IP | 兜底 |

启动期 fail-loud：rate-limit Redis key 必须可创建，否则 panic。

### 7.9 密码（staff）+ 重置 + refresh rotation（v0.2 SEC HIGH-3；v0.2.2 ARCH-CRIT-NEW-C 闭环：补时序图 / invariant / e2e）

- argon2id（time=3, memory=64MB, parallelism=2）
- HIBP k-anonymity offline 检查（启动期载入 SHA-1 prefix 数据库）
- 5 次失败 / 15 min lockout per (account, IP)（LOW-r2-2）
- 密码重置 → revoke 全部 jti
- **refresh token rotation**（v0.2 SEC HIGH-3）：每次 `POST /auth/refresh` 下发新 refresh token，**旧 refresh token jti 立即加入 `revoked:jti:*` Redis（fail-closed）**；如同一旧 refresh 被复用 → 视为攻击，全 session revoke + alert。Refresh 用 in-flight Redis lock 防 race。

#### 7.9.1 双因子密码重置流程（v0.2.2 落地，PRD §17.5 + §3.28 `password_reset_token`）

```mermaid
sequenceDiagram
    autonumber
    participant U as 用户
    participant FE as Web (frontend)
    participant API as partner-api
    participant DB as partner_db
    participant CH as Email/SMS 通道
    participant AUD as audit_log + audit_log_unsealed
    participant REV as Redis revoked:jti:*

    rect rgb(245,245,245)
    note over U,FE: 阶段 1：发起重置（/auth/forgot 页面）
    U->>FE: 输入 email/phone（标识 actor）+ 同意 PIPL §17.5 重置条款
    FE->>API: POST /auth/password/forgot {actor_handle, consent_version}
    API->>API: 速率限流 5/min/IP + 5/h/handle（防枚举），错误响应**恒等**（不区分用户存在性）
    API->>DB: SELECT actor by handle（partner/customer/staff）；记录 consent_log（PIPL §17.5 同意快照）
    API->>DB: INSERT password_reset_token (token_hash=SHA256(rand32), second_factor_hash=SHA256(otp6), second_factor_type, requested_ip, user_agent, expires_at=NOW()+15min, audit_log_id, trace_id)
    API->>AUD: append action='auth.password_reset.requested' + actor + ip + ua + trace_id（unsealed → sealer 链式哈希）
    API->>CH: 异步派发：邮件链接 `https://partner.tracenex.cn/auth/reset/<raw_token>` + SMS OTP `<otp6>`（双因子分两通道送达）
    API-->>FE: 200 {message: "If an account matches, instructions have been sent."}（不泄漏命中）
    end

    rect rgb(245,245,245)
    note over U,FE: 阶段 2：用户点击邮件链接 + 输入 SMS OTP
    U->>FE: 打开 /auth/reset/:token（解析 raw_token）+ 输入 OTP + new_password (+ confirm)
    FE->>FE: zod 校验：new_password 强度（≥12 / 大小写 + 数字 + 符号 / HIBP 前 5 字符 prefix 上传 + 本地比对）
    FE->>API: POST /auth/password/reset {raw_token, otp, new_password}
    API->>DB: SELECT password_reset_token WHERE token_hash=SHA256(raw_token) AND consumed_at IS NULL AND invalidated_at IS NULL AND expires_at > NOW() AND failed_attempts < 5 FOR UPDATE
    alt token 不存在 / 已消费 / 过期 / 失败次数耗尽
        API->>AUD: action='auth.password_reset.token_invalid' + raw reason（仅服务端日志，响应不区分）
        API-->>FE: 400 {code: 'reset_token_invalid'}
    else token 命中
        alt OTP / 第二因子不匹配
            API->>DB: UPDATE password_reset_token SET failed_attempts = failed_attempts+1, invalidated_at = CASE WHEN failed_attempts+1 >= 5 THEN NOW() ELSE NULL END
            API->>AUD: action='auth.password_reset.factor_failed' + remaining_attempts
            API-->>FE: 400 {code: 'second_factor_mismatch', remaining_attempts}
        else 双因子全通过
            API->>API: 校验 IP / UA 漂移：requested_ip ≠ now_ip 或 UA family 不同 → 标记 risk='elevated'，发邮件提示但不阻断（PRD §17.5：IP/UA 仅风险信号，非硬约束）
            API->>DB: BEGIN TX
            API->>DB: UPDATE staff/customer/partner SET password_hash=argon2id(new_password), password_rotated_at=NOW()
            API->>DB: UPDATE password_reset_token SET consumed_at=NOW()（一次性销毁）
            API->>DB: SELECT all jti for actor → bulk INSERT revoked_jti list（与 §7.2 fail-closed 契约一致）
            API->>DB: COMMIT TX
            API->>REV: SETEX revoked:jti:<jti> EX <token_ttl> （强制下线全设备：所有现存 access/refresh token 立即失效）
            API->>AUD: action='auth.password_reset.success' + actor + risk_signals + revoked_jti_count（unsealed）
            API-->>FE: 200 {message: 'reset_ok'} → FE 重定向 /auth/login 并清前端 actor cache
        end
    end
    end
```

#### 7.9.2 关键 invariant（v0.2.2 显式声明）

| ID | invariant | 实现锚点 |
|---|---|---|
| **PR-INV-1** | `password_reset_token.expires_at - created_at ≤ 15 min`；CHECK 由 service 层硬编码（DB 不放 generated column 以免引入时区漂移） | §3.28 / §7.9.1 step 4 |
| **PR-INV-2** | token 单次有效：`consumed_at IS NOT NULL` 后任何 reset 调用立即 400；`UNIQUE(token_hash)` + 同 TX `UPDATE consumed_at=NOW()` 与 password_hash 写入原子提交 | §7.9.1 阶段 2 commit 块 |
| **PR-INV-3** | 第二因子失败 ≥ 5 次 → token 永久 invalidate（`invalidated_at=NOW()`）+ 该 actor 24h 内所有 forgot 请求触发 captcha + alert | §7.9.1 阶段 2 mismatch 分支 |
| **PR-INV-4** | reset 成功后**同步**（同请求生命周期内）revoke 该 actor 全部 jti；无 eventual consistency 窗口；revoke 与 password 写在同 TX commit 之前完成 Redis SETEX | §7.9.1 阶段 2 success 分支 step 8-9 |
| **PR-INV-5** | IP / UA 绑定为**软约束**：requested_ip / user_agent 写库但不参与硬校验，漂移仅写 `risk='elevated'` audit + 用户邮件提示；硬约束只有 token + OTP 双因子 | §7.9.1 阶段 2 success 分支 step 6 |
| **PR-INV-6** | PIPL §17.5 同意快照：`consent_log` 必有 action='password_reset.consent' + version + timestamp，否则 forgot 拒绝（403 `consent_required`） | §7.9.1 阶段 1 step 3 |
| **PR-INV-7** | 错误响应**信息恒等**：用户不存在 / 速率超限 / 通道发送失败 → 阶段 1 一律返回相同 200 文案，禁用枚举攻击；阶段 2 同理（`reset_token_invalid` 不区分 token 不存在 vs 过期 vs 已消费） | §7.9.1 阶段 1 step 5 + 阶段 2 invalid 分支 |
| **PR-INV-8** | `password_reset.purge` cron 每日清理 `expires_at < NOW()-7d` 行（保留 audit_log 永不删）；与 §6 cron 表 `password_reset.purge` 行字面对齐（**v1.0 cosmetic #5/#7：已在 §6 cron 表正式登记，每日 04:30 / ops `@platform-ops`**；v0.2.2 "暂列在注释 / Phase 1 第 2 周补"已闭合） | §6 cron 表 + §3.28 末注释 |

#### 7.9.3 e2e 测试钩子（≥ 3 条 invariant，CI 强制）

> 命名规范见 §15.2；测试文件 `tests/e2e/auth_password_reset_test.go`。

```go
// E2E-PR-1（PR-INV-2 + PR-INV-4）：token 单次有效 + reset 成功强制下线全设备
func TestPasswordReset_TokenSingleUse_AndGlobalLogout(t *testing.T) {
    // 准备：staff 已登录两台设备 → 取得 jti_a + jti_b
    actor := SeedStaffWithTwoSessions(t)
    rawToken, otp := RequestForgot(t, actor.Email)        // 阶段 1
    AssertSMSOTPDelivered(t, actor.Phone, otp)
    AssertEmailLinkDelivered(t, actor.Email, rawToken)

    // 第一次 reset：成功
    resp := PostReset(t, rawToken, otp, "Pa$$w0rd-NewSecret-2026")
    require.Equal(t, 200, resp.Code)

    // PR-INV-2：同一 raw_token 二次 reset → 400 reset_token_invalid
    resp2 := PostReset(t, rawToken, otp, "Another-NewSecret-2026")
    require.Equal(t, 400, resp2.Code)
    require.Equal(t, "reset_token_invalid", resp2.Body.Code)

    // PR-INV-4：jti_a / jti_b 都已 revoke，旧 access token 调任意 endpoint 返 401 jti_revoked
    require.Equal(t, 401, CallWithJTI(t, actor.JTI_A).Code)
    require.Equal(t, 401, CallWithJTI(t, actor.JTI_B).Code)
}

// E2E-PR-2（PR-INV-3）：第二因子失败 5 次永久 invalidate
func TestPasswordReset_FactorFailure5_InvalidatesToken(t *testing.T) {
    actor := SeedCustomer(t)
    rawToken, _ := RequestForgot(t, actor.Email)
    for i := 0; i < 5; i++ {
        resp := PostReset(t, rawToken, "000000", "Pa$$w0rd-NewSecret-2026")
        require.Equal(t, 400, resp.Code)
        require.Equal(t, "second_factor_mismatch", resp.Body.Code)
    }
    // 第 6 次哪怕 OTP 正确也拒
    correctOTP := GetOTPFromTestSMS(t, actor.Phone)
    resp := PostReset(t, rawToken, correctOTP, "Pa$$w0rd-NewSecret-2026")
    require.Equal(t, 400, resp.Code)
    require.Equal(t, "reset_token_invalid", resp.Body.Code)
    // DB invariant
    AssertColumn(t, "password_reset_token", rawToken, "invalidated_at IS NOT NULL")
}

// E2E-PR-3（PR-INV-7）：信息恒等防枚举
func TestPasswordReset_EnumerationResistant(t *testing.T) {
    respKnown   := PostForgot(t, "real@example.com")
    respUnknown := PostForgot(t, "no-such-user@example.com")
    require.Equal(t, respKnown.Code, respUnknown.Code)        // 200 = 200
    require.Equal(t, respKnown.Body, respUnknown.Body)        // 文案完全一致
    require.LessOrEqual(t, respKnown.Latency-respUnknown.Latency, 50*time.Millisecond) // ±50ms 防 timing oracle
}
```

#### 7.9.4 与 frontend 路由对齐

- `/auth/forgot` → 阶段 1 表单（输入 handle + PIPL 同意，POST `/auth/password/forgot`）
- `/auth/reset/:token` →（v0.2.2 frontend §6 路由表 v0.2.2 ADDENDUM 同步登记）阶段 2 表单（输入 OTP + new_password，POST `/auth/password/reset`）
- 两条路由均**不**需要 JWT；CSP / CSRF 走 §7.6 cookieless 表单 token；前端 zod schema 见 frontend §7

---

## 8. 幂等 / Saga 实现

> 引用：PRD §18 + §8.16 + §8.17 + `integration-design` §5 / §4。

### 8.1 idempotency middleware（v0.2.2 重写：middleware 只读，service 层同 TX 写入）

> **v0.2.2 ARCH-CRIT-NEW-A 闭环**：v0.2 / v0.2.1 的中间件代码块原本在 `c.Next()` 之后调 `repo.Insert(c, &Record{...})`，与 ADR-003 "idempotency_record 与业务 TX 同事务" 承诺自相矛盾。v0.2.1 仅在末段加了"已废弃看正解"的注释否定，未真删代码块。**v0.2.2 起本节代码块整段重写为下面的最终形态**——middleware 只做 SELECT 命中检查 / 重放 / 透传，**不再**调 `repo.Insert`；写 `idempotency_record` 由 service 层在 `bizDB.Transaction` 闭包内与业务 INSERT/UPDATE 同 TX 完成。`responseRecorder` 由 middleware 注入到 `c.Writer`，service 在事务闭包提交前从 `c.Writer.(*responseRecorder)` 读取响应字节并加密后落库；middleware **不**接 KMS / scrubber。

```go
// internal/idempotency/middleware.go
//
// 职责（v0.2.2 收敛）：
//   1. 解析 Idempotency-Key + 请求体哈希
//   2. SELECT idempotency_record：
//        - 命中 completed → 解密 response_cipher 重放（c.Abort）
//        - 命中 pending   → 查 saga_step 返 202 / 或 500 orphaned
//        - 命中但 hash 不同 → 409 reused-different-body
//        - 未命中 → 注入 responseRecorder 到 c.Writer，c.Next 透传业务
//   3. **不**写 idempotency_record；service 层在业务 TX 内调用 repo.Insert(tx, ...)
//
// 与 ADR-003 / §8.1 v0.2.2 正解一致：record 与业务变更绝对同 TX。
func Middleware(repo Repo, kms kms_envelope.Service) gin.HandlerFunc {
    return func(c *gin.Context) {
        if !endpointMarkedIdempotent(c) { c.Next(); return }
        key := c.GetHeader("Idempotency-Key")
        if key == "" {
            abort400(c, errs.CodeIdemKeyRequired); return
        }
        ac := getActor(c)
        body, _ := io.ReadAll(c.Request.Body)
        c.Request.Body = io.NopCloser(bytes.NewReader(body))
        reqHash := sha256Hex(canonicalize(c.Request.Method, c.Request.URL.Path, c.Request.URL.RawQuery, body))

        existing, err := repo.Find(c, ac.ActorType, ac.ActorID, key, c.Request.URL.Path)
        switch {
        case err == nil && existing.Status == "completed" && existing.RequestHash == reqHash:
            // 命中且已完成：解密 response_cipher 重放
            resp, _ := kms.Decrypt(existing.ResponseCipher, existing.ResponseKeyID)
            c.JSON(existing.ResponseStatus, json.RawMessage(resp))
            c.Abort(); return
        case err == nil && existing.Status == "pending":
            // pending 态：查 saga_step 状态（v0.2 ARCH CRIT A-1：避免在 zero-value cipher 上 Decrypt）
            step := sagaRepo.FindLatestByID(existing.SagaID)
            if step != nil && step.Status == "in_progress" {
                c.JSON(202, gin.H{"saga_id": existing.SagaID, "status": "in_progress"})
                c.Abort(); return
            }
            // pending 但无 saga_step（崩溃中断）→ 返 500 + trace_id，由 ops 介入
            abort500(c, errs.CodeSagaOrphanedPending, gin.H{"trace_id": traceID(c), "saga_id": existing.SagaID})
            return
        case err == nil && existing.RequestHash != reqHash:
            abort409(c, errs.CodeIdemReusedDifferentBody); return
        }

        // 注入 responseRecorder：service 层从 c.Writer 取出响应字节加密后同 TX 落库
        rec := &responseRecorder{ResponseWriter: c.Writer}
        c.Writer = rec
        // 同时把 (key, reqHash, actor) 三元组挂到 ctx 供 service 取用
        c.Set(ctxKeyIdemKey, key)
        c.Set(ctxKeyIdemReqHash, reqHash)
        c.Set(ctxKeyIdemActor, ac)

        c.Next()
        // 注意：此处**不再**调 repo.Insert。idempotency_record 已由 service 层在
        // bizDB.Transaction 闭包内、业务 INSERT/UPDATE 之后、commit 之前完成持久化。
    }
}
```

**Service 层职责骨架**（与 ADR-003 字面对齐）：

```go
// internal/<module>/service.go —— 任意 idempotent endpoint 的 service handler
func (s *Service) DoSomething(c *gin.Context, in DoSomethingIn) (out DoSomethingOut, err error) {
    rec := c.Writer.(*responseRecorder) // middleware 注入
    key      := c.MustGet(ctxKeyIdemKey).(string)
    reqHash  := c.MustGet(ctxKeyIdemReqHash).(string)
    actor    := c.MustGet(ctxKeyIdemActor).(Actor)
    scrubber := s.scrubber

    return out, s.bizDB.Transaction(func(tx *gorm.DB) error {
        // 1. 业务 INSERT/UPDATE（含 saga_step、wallet_log 等所有写）
        if err := s.doBusinessWrites(tx, in, &out); err != nil { return err }

        // 2. 业务结果序列化 + scrub + KMS 加密
        respBytes, _ := json.Marshal(out)
        scrubbed     := scrubber.RedactJSON(respBytes)
        cipher, kid  := s.kms.Encrypt("system", scrubbed)

        // 3. idempotency_record INSERT —— **同 TX**，与业务原子提交（ADR-003 invariant）
        return s.idemRepo.Insert(tx, &idempotency.Record{
            ActorType:      actor.ActorType,
            ActorID:        actor.ActorID,
            IdempotencyKey: key,
            Endpoint:       c.Request.URL.Path,
            RequestHash:    reqHash,
            ResponseStatus: rec.Status,                  // service 已 c.JSON 写入 status
            ResponseHash:   sha256Hex(respBytes),
            ResponseCipher: cipher,
            ResponseKeyID:  kid,
            TraceID:        actor.TraceID,                 // v1.0 cosmetic #1：与 §3.16 DDL trace_id 列对齐
            ExpiresAt:      time.Now().Add(24 * time.Hour),
        })
    })
}
```

> **invariant 三连**（CI analyzer §15.3 字面校验，对应 e2e 钩子见 §13）：
> 1. middleware 文件 `internal/idempotency/middleware.go` 不得出现 `repo.Insert(` 字面（grep -F 反向断言）。
> 2. 任何 endpoint 标记 `idempotent: true` 的 service 函数体内必须出现 `bizDB.Transaction(` **且**该闭包内出现 `idemRepo.Insert(tx,`（AST analyzer 配套 lint）。
> 3. e2e：模拟 service 业务写成功后 **panic 前** `idemRepo.Insert` —— 期望整个 TX 回滚，重放同 idem-key 不返 cached response；只允许业务再次执行（与 ADR-003 一致）。
>
> **ADR-003 承诺不变**；v0.2 / v0.2.1 的代码块在 v0.2.2 已被本块字面替换，下游引用统一指向本节最终形态。

### 8.2 Saga 编排器

```go
// internal/saga/orchestrator.go
type Orchestrator interface {
    NewSaga(idemKey, kind string) Saga
    EnqueueRetry(ctx, sagaID string)
}

type Saga interface {
    Run(ctx, step string, fn TxFn) (any, error)        // in_progress → committed
    Commit(ctx, step string, fn TxFn) (any, error)
    Compensate(ctx, step string, fn TxFn) (any, error)
}

type TxFn func(tx *gorm.DB) (any, error)
```

retry worker（goroutine pool 内 `cmd/api` 进程，或独立 `cmd/saga-runner`）：

```go
func (w *retryWorker) Loop(ctx context.Context) {
    t := time.NewTicker(30 * time.Second)
    for {
        select {
        case <-ctx.Done(): return
        case <-t.C:
            rows, _ := w.repo.LoadPendingRetries()
            for _, r := range rows {
                if time.Since(r.UpdatedAt) < backoff(r.Attempts) { continue }
                if r.Attempts >= 30 || time.Since(r.StartedAt) >= time.Hour {
                    w.escalate(r); continue
                }
                w.retry(r)
            }
        }
    }
}
```

partition 策略（M scale-out）：retry worker 多副本时按 `hash(saga_id) % N == replica_index` 划分（避免 race）。

### 8.3 idempotency cleanup job

`cmd/api` 内嵌 cron：每 6h `DELETE FROM idempotency_record WHERE expires_at < NOW() LIMIT 10000`，循环直至无行。

---

## 9. 密钥管理 / 信封加密

> 引用：PRD §19 + Security HIGH-r2-2 + ADR-009。

### 9.1 KMS 客户端封装

```go
// internal/kms_envelope/service.go
type Service interface {
    Encrypt(scope string, plain []byte) (cipher []byte, keyID string, err error)
    Decrypt(cipher []byte, keyID string) (plain []byte, err error)
    GenerateDataKey(scope string) (DEK, error)
    RotateDEK(scope string) error
    ScheduleKeyDeletion(scope string, after time.Duration) error
}
```

DEK 缓存：`map[scopedKey]plainDEK`，key 必须 `(scope, key_version)`；scope = `tenant:{partner_id}` 或 `system`；TTL 1h。Linux `unix.Mlock` 锁内存页；prod pprof 禁用或路径 auth-gated（ADR-009 / Security HIGH-r2-2）。

```go
type scopedKey struct { scope, version string }
```

### 9.2 PII 字段 GORM 包装

```go
// pkg/pii/encrypted.go
type Encrypted struct {
    Plain  string `pii:"true" json:"-"`
    Scope  string
    cipher []byte
    keyID  string
}

func (e *Encrypted) Value() (driver.Value, error) {
    return e.cipher, nil    // 只写 cipher
}

func (e *Encrypted) Scan(src any) error {
    e.cipher = src.([]byte)
    return nil               // 不解密；用 Reveal(kms) 主动解
}

func (e *Encrypted) Reveal(kms kms_envelope.Service) (string, error) { ... }
```

> repository 层永远不调 `Reveal`；service 层在需要时显式调（如导出 PII / 给 OCR），并在使用后立即 `runtime.GC()` + zero `[]byte`。

### 9.3 KEK 轮换流程

```
1. 调 KMS CreateKey → newKEK
2. biz_setting.kek_active_id = newKEK.id
3. cmd/kek-rotator 起 batch：
   for each row WHERE encryption_key_id != newKEK:
     plain = decrypt(row.cipher, row.key_id)
     newCipher, newKeyID = encrypt(plain, newKEK)
     UPDATE row SET cipher=newCipher, encryption_key_id=newKeyID
4. 完成后 oldKEK.disable()
5. 90d 后 oldKEK.scheduleDeletion()
```

### 9.4 OSS presigned URL helper（v0.2 SEC CRIT-4：补 PresignPut + 二次校验）

```go
// internal/kms_envelope/oss.go
func (s *Service) PresignGet(bucket, key string, ttl time.Duration) (string, error) {
    if ttl > 5*time.Minute { return "", errs.New(errs.CodeOssTTL) }
    return s.client.Sign("GET", bucket, key, ttl, "response-content-disposition=attachment")
}

// v0.2 SEC CRIT-4：服务端强校验的 presigned PUT
// allowedMime 必须是字面量白名单（CI AST scan 校验，禁止用户输入）
// maxBytes ≤ 10MB；ttl ≤ 300s；签名带 Content-Type / Content-Length / x-oss-content-md5
func (s *Service) PresignPut(bucket, key string, allowedMime []string, maxBytes int64, ttl time.Duration) (*PresignPutToken, error) {
    if ttl > 5*time.Minute { return nil, errs.New(errs.CodeOssTTL) }
    if maxBytes <= 0 || maxBytes > 10*1024*1024 { return nil, errs.New(errs.CodeOssMaxBytes) }
    for _, m := range allowedMime {
        if !mimeAllowed[m] { return nil, errs.New(errs.CodeOssMime) } // image/jpeg|png|webp 字面量
    }
    // 签名时绑定：Content-Type / Content-Length / x-oss-content-md5；bucket policy 拒绝不匹配的 PUT
    return s.client.SignPut(bucket, key, allowedMime, maxBytes, ttl), nil
}

// 提交后服务端 HEAD + magic-byte 二次校验（不信 Content-Type）
func (s *Service) VerifyUploadedKYCObject(ctx context.Context, bucket, key string, expectedMime string) error {
    obj, err := s.client.Head(ctx, bucket, key)
    if err != nil { return err }
    if obj.Size > 10*1024*1024 { return errs.New(errs.CodeOssOversize) }
    head, err := s.client.GetRange(ctx, bucket, key, 0, 16) // 读前 16 字节
    if err != nil { return err }
    if !magicbyte.Match(head, expectedMime) { return errs.New(errs.CodeOssMimeMagicByteMismatch) }
    // 异步病毒扫
    s.scanQueue.Enqueue(bucket, key)
    return nil
}
```

CI 检查：
- 所有调 `PresignGet` / `PresignPut` 处 `ttl ≤ 300s`（go AST scan）
- 所有 `PresignPut` 调用 `allowedMime` 必须是字面量切片（go AST 校验，无字符串拼接 / 无变量赋值）

frontend §12.1 CSP `connect-src` 必须包含 `https://*.oss-cn-hangzhou.aliyuncs.com`（v0.2 SEC MED-10）以允许浏览器直传。

---

## 10. 审计 + 哈希链 audit_log（HIGH-r2-1 重写）

> 引用：PRD §8.13 + Security HIGH-r2-1 + ADR-006。

### 10.1 sealer 设计

应用 INSERT 写到 `audit_log_unsealed`（仅 INSERT 权限，不带 hash 字段）。`cmd/audit-sealer` 单 leader 进程：

```
loop every 200ms:
    rows := SELECT * FROM audit_log_unsealed ORDER BY id ASC LIMIT 1000
    BEGIN TX
    for each row in rows:
        prev = SELECT self_hash FROM audit_log ORDER BY id DESC LIMIT 1
        self = sha256(canonical(row, prev))
        INSERT INTO audit_log (id=row.id, ..., prev_hash=prev, self_hash=self)
        DELETE FROM audit_log_unsealed WHERE id = row.id
    COMMIT
```

> id 取自 `audit_log_unsealed.id`，与 `audit_log` PK 1:1 不冲突（unsealed.id 是单调递增）。
>
> `audit_log` 表 `id` 列**非 AUTO_INCREMENT**（应用不写）；`audit_log_unsealed.id` AUTO_INCREMENT 提供顺序。

### 10.2 Writer 接口

```go
// internal/audit/writer.go
type Writer interface {
    Record(ctx context.Context, ac ActorContext, entry Entry) error
    RecordWithPII(ctx context.Context, ac ActorContext, entry Entry, piiDiff []byte) error
}

type Entry struct {
    Action     string  // verb
    TargetType string
    TargetID   int64
    Diff       any     // JSON-serializable
}
```

`Record` 在 `bizDB.Transaction` 闭包内 INSERT 到 `audit_log_unsealed`；`RecordWithPII` 额外加密 piiDiff 写 `audit_log_pii` 同 TX。

### 10.3 verify CLI

```bash
go run ./cmd/audit-verify --from-id=0 --to-id=100000
# 校验每行 prev_hash == 上一行 self_hash；mismatch 输出 row id + alert
```

跑 daily cron；mismatch → page on-call。

### 10.4 PIPL §47 删除时

`audit_log_pii.tombstoned_at = NOW()`，cipher 留空但行不删（保留 `id` ↔ `audit_log.diff_pii_id` 关系）。哈希链不破坏（hash 输入不含 audit_log_pii.cipher，只含 `diff_pii_id`）。

---

## 11. 错误码 / 错误模型

> 引用：`overview` §11.3 + `integration-design` §8。本节落到 Go 实现。

### 11.1 错误段位（partner-api 自身）

| 段位 | 含义 | 例 |
|---|---|---|
| `BIZ_AUTH_*` | 鉴权 | `BIZ_AUTH_JWT_REVOKED` `BIZ_AUTH_STEP_UP_REQUIRED` |
| `BIZ_PERM_*` | 越权 | `BIZ_PERM_FORBIDDEN` `BIZ_PERM_DUAL_CONTROL_REQUIRED` |
| `BIZ_VALID_*` | DTO 校验 | `BIZ_VALID_AMOUNT_OUT_OF_RANGE` |
| `BIZ_RES_*` | 资源 | `BIZ_RES_NOT_FOUND`（统一不暴露存在性） |
| `BIZ_IDEM_*` | 幂等 | `BIZ_IDEM_KEY_REQUIRED` `BIZ_IDEM_REUSED_DIFFERENT_BODY` |
| `BIZ_WALLET_*` | 钱包 | `BIZ_WALLET_INSUFFICIENT_AVAILABLE` `BIZ_WALLET_VERSION_MISMATCH` |
| `BIZ_PRICING_*` | 定价 | `BIZ_PRICING_OVERLAP_WINDOW` `BIZ_PRICING_MARKUP_BOUND` |
| `BIZ_SAGA_*` | saga | `BIZ_SAGA_STUCK_UNKNOWN` `BIZ_SAGA_FORCE_RESOLVED` |
| `BIZ_FYAPI_*` | 上游 | `BIZ_FYAPI_5XX` `BIZ_FYAPI_TIMEOUT` `BIZ_FYAPI_USER_ERASED` |
| `BIZ_KYC_*` | KYC | `BIZ_KYC_REJECTED` `BIZ_KYC_AGE` |
| `BIZ_PAYMENT_*` | 支付 | `BIZ_PAYMENT_AMOUNT_MISMATCH` `BIZ_PAYMENT_REPLAY` |
| `BIZ_CONTENT_*` | 内容安全 | `BIZ_CONTENT_BLOCKED_INPUT` |
| `BIZ_OSS_*` | OSS | `BIZ_OSS_TTL_EXCEEDED` |

5xxxx 系统层：

| Code | HTTP | 含义 |
|---|---|---|
| `SYS_DB_DOWN` | 503 | DB 不可达 |
| `SYS_FYAPI_DOWN` | 502 | Fy-api 内部 API 不可达 |
| `SYS_KMS_DOWN` | 503 | KMS 调用失败 |
| `SYS_PANIC` | 500 | 兜底 |

### 11.2 Fy-api 错误 → partner-api 映射

直接采用 `integration-design` §8.1 表；service 层调用方包装为 AppError。

### 11.3 i18n

- `message_zh` / `message_en` 在 `pkg/errors/messages.go` 由 `errCode → struct{zh, en string}` map 维护
- 启动期检查每个 code 必有两语言（缺失则 panic）

---

## 12. 观测

> 引用：PRD §10.6 + `integration-design` §9。

### 12.1 Prometheus 指标命名

```
# HTTP
http_requests_total{method, path, status, actor_type}
http_request_duration_seconds{method, path}
http_inflight_requests

# 钱包
wallet_drift_total{partner_id}
wallet_hold_held_amount{partner_id}
wallet_allocate_total{result}      # success / hold_failed / fy_failed / unknown

# saga
saga_total{kind, terminal_state}
saga_unknown_age_seconds              # gauge
saga_force_resolve_total{actor}

# outbox
outbox_lag_seconds{instance}
outbox_unconsumed_total
outbox_dead_letter_total
revenue_log_inserts_total{result}

# settlement
settlement_run_duration_seconds
settlement_payout_total{status}

# 鉴权
internal_auth_failures_total{kid, reason}
internal_auth_nonce_replays_total
jwt_revocation_lookups_total{result}

# pubsub
pubsub_publish_total{channel}
pubsub_consume_lag_seconds{channel}

# audit
audit_seal_lag_seconds
audit_chain_verify_failures_total

# kms
kms_encrypt_total{scope, result}
kms_decrypt_total{scope, result}
dek_cache_hit_ratio
```

### 12.2 SLS 日志格式

JSON via `zerolog`，字段：

```json
{
  "ts": "2026-05-10T12:34:56.789+08:00",
  "level": "info",
  "trace_id": "01J9G5...",
  "actor_type": "partner",
  "actor_id": 42,
  "endpoint": "/partner/customers/123/allocate-quota",
  "msg": "saga committed",
  "saga_id": "01J9...",
  "step": "wallet.commit",
  "duration_ms": 45
}
```

PII scrubber：`pkg/pii/scrubber.go` 在 zerolog hook 中识别 `pii:"true"` 标签 + 模式（身份证 18 位 / 手机号 / email / RFC5322），自动 redact。

### 12.3 关键 SLO + alert

| 指标 | 名义 | alert 阈值 |
|---|---|---|
| wallet drift | 0 | > 0 (page) |
| saga unknown 持续 | < 0.1% | > 1% |
| outbox lag | < 1s | > 5s 5min |
| settlement 跑完 | < 30 min | > 60 min |
| audit chain verify | 100% | < 100% |
| HMAC 验证失败 | < 0.01% | > 0.1% |
| JWT revocation lookup error | — | error rate > 1% |

### 12.4 trace_id 注入

- `pkg/tracing/middleware.go`：缺失则生成 UUIDv7，写到 `c.Request.Context()` + response header
- 跨进程透传：fyapi_client 自动注入到 `X-Oneapi-Request-Id`
- DB 写入：每个 saga / revenue / audit / outbox 行带 `trace_id` 列

---

## 13. 配置 + 部署

### 13.1 Env 变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `APP_ENV` | dev | dev/staging/prod |
| `BIZ_DSN` | — | partner_db DSN（mysql://tnbiz_app:...@host/partner_db） |
| `FY_DSN` | — | fy_api_db RO DSN |
| `LOG_DSN` | — | LOG_DB DSN（outbox poller 走 tnbiz_outbox_consumer） |
| `REDIS_ADDR` | — | Redis 地址 |
| `REDIS_PASSWORD` | — | Redis AUTH |
| `KMS_ENDPOINT` | — | Aliyun KMS endpoint |
| `KMS_KEY_ID` | — | system KEK id |
| `OSS_ENDPOINT` | — | — |
| `OSS_BUCKET_PII` | — | KYC 私有桶 |
| `OSS_BUCKET_ARCHIVE` | — | 冷归档桶 |
| `SLS_PROJECT` | — | — |
| `FY_API_BASE` | — | https://api.aitracenex.com |
| `FY_API_KEY_ID` | — | partner-api → Fy-api 的 X-Auth-KeyId |
| `FY_API_KEY_SECRET` | — | HMAC secret（KMS Secret Manager 注入） |
| `JWT_VERIFY_KEY_PEM` | — | Fy-api JWT 公钥（也可走 biz_setting） |
| `LICENSED_PROVIDER` | — | 持牌方 SDK 选项 |
| `SETTLEMENT_LEADER_KEY` | `cron:settlement:monthly` | Redis SETNX key |

### 13.2 ConfigMap / Secret 切分

K8s：
- ConfigMap `tnbiz-config`：所有非敏感 env（`APP_ENV` / `FY_API_BASE` / `OSS_ENDPOINT`...）
- Secret `tnbiz-secret`：DSN、HMAC secret、KMS credential、Aliyun AK/SK、SMTP password
- 通过 `external-secrets-operator` 从 KMS Secret Manager 同步（不允许 sealed yaml 直接 commit）

### 13.3 健康检查

- `GET /healthz/live`：进程活；返 200 直到 SIGTERM 才退
- `GET /healthz/ready`：依赖检查全过才 200
  - DB ping
  - Redis ping
  - KMS connectivity (`kms.DescribeKey` 1 次/30s 缓存)
  - OSS list (1 次/30s 缓存)
  - Fy-api `/api/internal/health` (1 次/30s 缓存)
  - SLS endpoint reachable

### 13.4 graceful shutdown

```go
// cmd/api/main.go
srv := &http.Server{...}
go srv.ListenAndServe()

stop := make(chan os.Signal, 1)
signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
<-stop
ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
defer cancel()
_ = srv.Shutdown(ctx)
// drain saga retry worker, dispatcher etc; wait for errgroup
```

K8s `terminationGracePeriodSeconds=30`、preStop hook 让 pod 从 Service 摘除后 sleep 5s 再退。

---

## 14. 数据库迁移策略

> 引用：`overview` §11.5 + Architect H-3 + `integration-design` §1.3。partner_db 迁移与 Fy-api 覆盖层迁移彼此独立。

### 14.1 工具

- 主路径：[`golang-migrate/migrate`](https://github.com/golang-migrate/migrate)（纯 SQL 文件，version 单调递增）
- dev fallback：GORM AutoMigrate（仅 sqlite/dev）
- migrations/ 目录命名：`YYYYMMDDHHMMSS_description.up.sql` / `.down.sql`
- 不可逆 migration 禁止；若必须，加 `# DESTRUCTIVE: backup snapshot required` 头部，CI 阻止合入

### 14.2 GRANT 用户

- `tnbiz_migrator@%`：DDL 用户，由 ops 持有；CI/CD pipeline 仅 staging 自动跑，prod 需手工 approve
- `tnbiz_app@%`：应用用户；启动期对 DDL 敏感操作返 ERROR

### 14.3 Phase 演进路径

| Phase | 新增表 / 列 |
|---|---|
| 1 | partner / customer / partner_wallet / partner_wallet_log / wallet_hold / partner_pricing_rule (单层) / revenue_log (no settlement_id) / invitation_code / kyc_application (stub fields) / staff / biz_setting / idempotency_record / saga_step / consent_log / audit_log + audit_log_pii + audit_log_unsealed / ticket / ticket_reply / notification_outbox / customer_partner_change_log / **password_reset_token (v0.2.1 §3.28)** |
| 2A | topup_intent / partner_pricing_rule (multi-layer) / kyc_application (full PII columns) / content_safety_event (§3.23) / **pipl_request (v0.2.1 §3.27)** / pia_report (§3.25) / pipl_complaint (§3.26) / partner_debt (§3.22) |
| 2B | settlement / settlement_item / settlement_run / settlement_config_change_log / invoice_application / invoice_title / red_flush_request (§3.12) / revenue_log.settlement_id 字段 |
| 3 | seat / partner_member / customer_member |

### 14.4 Fy-api 覆盖层迁移协调

- partner-api 启动期检测 fy_api_db 有 `consume_log_outbox` 表（fail-loud 否则）
- partner_db `revenue_log.fy_api_log_id` 写入只在 `users.id` BIGINT 升级完成后启用（biz_setting feature flag `revenue_log_writes_enabled`）

---

## 15. 测试策略 + invariants

> 引用：PRD §10.9 + Security 验收 gates §22.2。

### 15.1 三档

| 档 | 工具 | 范围 | 覆盖率 |
|---|---|---|---|
| Unit | `testing` + `testify` + `gomock` (fyapi_client) | service / domain / pkg | ≥ 70% |
| Integration | `testcontainers-go` (MySQL 8 / Redis / minio) + `wiremock` (Fy-api / KMS / OCR) | repository + service + middleware | ≥ 60% |
| E2E | `k6` + Playwright（前端协作） | 关键 user journey | 关键流程必须 |

### 15.2 关键 invariants（每个 service 至少 3 条）

partner.Service：
- I-P-1 consent_token 必须有效（5 分钟内、未 withdrawn、含 `sensitive_pi`）
- I-P-2 invitation_code ≥ 16 字符且唯一
- I-P-3 partner / invitation 在同 TX

customer.Service：
- I-C-1 RegisterWithInvitation 拒绝已是直营客户（场景 N）
- I-C-2 防绕过中间件命中 → 重定向（M9-02）
- I-C-3 transfer 必须三方确认才 commit

wallet.Service：
- I-W-1 available = balance - SUM(hold WHERE status='held')，并发下 invariant
- I-W-2 同 saga_id 重复 allocate 不重复扣 balance（idempotency）
- I-W-3 5xx 后 release 不会重复释放（hold.status 终态）
- I-W-4 wallet drift（balance vs SUM(log.amount)）每日 = 0

pricing.Service：
- I-PR-1 markup ∈ [1.0, max_markup]
- I-PR-2 同 (partner, customer, model, tier) 的 active rule 时间窗口不重叠
- I-PR-3 历史 revenue 永远引用当时 rule（`AppliedRuleId` 不变）

revenue.Service / outbox.Poller：
- I-R-1 同 fy_api_log_id 同 occurrence 不重复 insert
- I-R-2 outbox row 消费成功后必删
- I-R-3 dead_letter 行不再被 poll

settlement.Service：
- I-S-1 freshness gate fail → settlement 不进 generating
- I-S-2 progress_offset 续跑后总 partner 数 = period 内有 revenue 的 partner 数
- I-S-3 settlement_item.payout = revenue - cost - platform_fee - withheld_tax，BIGINT 不溢出

kyc.Service：
- I-K-1 提交时必须存在 sensitive_pi consent
- I-K-2 加密字段写入时 `encryption_key_id` 非空
- I-K-3 30 天后 hot 桶 OSS 对象不可访问

saga.Orchestrator：
- I-SA-1 1h 后未结束 → escalated（不会无限滞留）
- I-SA-2 force_resolve 必须 dual-control + audit_log
- I-SA-3 retry partition：同一 saga_id 在多副本下只一个 worker 处理

audit.Writer + sealer：
- I-A-1 应用 user 无 UPDATE/DELETE on audit_log（GRANT gate）
- I-A-2 sealer 顺序写入；prev_hash 链不断
- I-A-3 PIPL tombstone 后 audit_log 主表 hash 仍可校验

idempotency：
- I-IM-1 100 次同 key + 同 body 重放只动一次 balance
- I-IM-2 同 key 不同 body 必返 409
- I-IM-3 response_cipher 解密内容不含 PII pattern（scrubber gate）

### 15.3 BOLA 矩阵测试

CI 自动从 `permission.matrix` 生成测试：每条 `❌` 单元生成一个 e2e；调用方 token 与 resource owner 不同时必返 404（PRD §16.3 / Security S-6）。

### 15.4 性能 baseline

`k6` 脚本 + GitHub Actions：
- `/partner/customers/{id}/allocate-quota` p95 ≤ 800ms / 200 RPS / 5 min sustained
- `/customer/dashboard` p95 ≤ 500ms
- `/admin/audit-log` filter 查询 p95 ≤ 1s（10M rows）

### 15.5 chaos 测试

- 随机 kill outbox poller pod；revenue_log 不丢失
- 随机 partition Redis；JWT revocation 走 fail-open（degrade，alert）
- mock Fy-api 5xx 30%；saga 必须收敛

---

## 16. 性能预算

> 引用：PRD §10.1。本节给每条 endpoint 的 P95 + RPS + DB QPS 估算 + 索引验证。

| Endpoint | P95 | RPS | DB QPS | 关键索引 | 备注 |
|---|---|---|---|---|---|
| `/customer/dashboard` | 500ms | 50 | 200 (cache) | customer.fy_user_id PK；revenue_log idx_customer_time | 读优先 Redis 30s |
| `/customer/balance` | 200ms | 100 | 100 | customer.fy_user_id | 单查 |
| `/customer/billing` | 800ms | 30 | 60 | revenue_log idx_customer_time | cursor pagination |
| `/customer/api-keys` POST | 500ms | 5 | 10 | — | 调 Fy-api topup-style endpoint |
| `/partner/dashboard` | 800ms | 30 | 90 | revenue_log idx_partner_time | 聚合带 30s cache |
| `/partner/customers` | 500ms | 30 | 60 | customer idx_customer_partner | cursor pagination |
| `/partner/customers/{id}/allocate-quota` | 800ms (含 Fy-api) | 50 | 200 | wallet_hold uk_hold_saga + partner_wallet uk_wallet_partner | 关键路径 |
| `/admin/customers` | 1s | 10 | 30 | customer 多 idx | filter |
| `/admin/audit-log` | 1s | 5 | 10 | audit_log idx_actor / idx_target | 时间范围 |
| `/webhook/payment/*` | 500ms | 50 (持牌方决定) | 100 | topup_intent uk_topup_channel_trade | 强幂等 |
| outbox.poller tick | — | — | 1000 rows/tick × 5 inst = 5k rows/s | consume_log_outbox idx_unconsumed | LOG_DB 独立池 |
| settlement.runner | 30 min | — | bulk SCAN | revenue_log idx_partner_time | offset 续跑 |

后台 ≥ 200 QPS 总目标：连接池 `bizDB.MaxOpenConns=200`、`MaxIdleConns=50`、`ConnMaxLifetime=30min`；fyDB 同；Redis 连接 100。

索引验证：每次 PR 中触动 hot path 必须附 `EXPLAIN` 结果 / pg-hero / `Slow Query Log` baseline。

---

## 17. Phase 切片

> 引用：`overview` §8 + PRD §12。本节落到 partner-api 各 service / table / endpoint 的 phase 标签（已在 §1.2 / §3 / §4 表中标注）。

### 17.1 Phase 1 必交付（Week 1-4）

- 进程：`cmd/api`、`cmd/outbox-poller`、`cmd/audit-sealer`、`cmd/notification-dispatcher`
- service：partner / customer / wallet / pricing (单层) / revenue / kyc (stub) / ticket / notify (email/inapp) / content_safety (M12-01/02 basic) / audit / saga / idempotency / fyapi_client / kms_envelope (无 rotation)
- 表：§3.1-3.7 + §3.10-3.18 + §3.20（不含 settlement / topup_intent / invoice / partner_debt）
- endpoint：§4.2 / §4.3 (basic) / §4.4 (M3-01..04, 09, 13) / §4.5 (M4-01..07, 15) / §4.6 / §4.7 (邮件+站内信) / §4.8 (stub) / §4.11 (M12-01/02)
- 验收：§22.2 S-1..S-8 全过

### 17.2 Phase 2A（Week 5-7）

- 进程：+ `cmd/kek-rotator`
- service：+ payment / pipl_rights / pricing (多层) / kyc (full)
- 表：+ topup_intent / pipl_request / content_safety_event / partner_pricing_rule 多列
- endpoint：+ M2-03/04 / M3-05/08/12/14 / M4-03/08-10/12/16/17 / M6-01..08 / M7-01..10 / M11 (sms+webhook) / M12-02..06 / M13

### 17.3 Phase 2B（Week 8-10）

- 进程：`cmd/settlement-runner`
- service：+ settlement / invoice
- 表：+ settlement* / invoice* / partner_debt (if F-2 方案 A) / revenue_log.settlement_id
- endpoint：+ M5-01/04..11 / M6-09 / M8-01..06 / M4-11/13

### 17.4 Phase 3（Week 10-13）

- 表：+ seat / partner_member / customer_member（schema only 1.x）
- endpoint：+ M2-06/10/15 / M3-06/11 / M5-09 完整 / M9-04

---

## 18. 风险 + ADR

> 完整风险表见 PRD §11；本节给 partner-api 工程级 ADR（≥ 6 条）。

### ADR-001：repository 返回 immutable domain entity（不返 GORM model）

- 决策：每个 domain `model.go` 定义 struct 仅含值字段；mutation 走 `WithStatus` 等返回新拷贝；repository `Find*` 返回 entity，`Update*` 接收 entity + version 做乐观锁
- 否决：直接返回 GORM struct 让 service 改 → 隐藏写入路径；测试困难
- 引用：PRD coding-style.md immutability + `overview` §3 模块边界

### ADR-002：handler/service/repository 严格分层 + DI（wire）

- 决策：handler 不调 repository；跨 domain 走 service interface；wire 编译期 DI
- 否决：fx 反射 DI / 直接 new → 编译期不可见、测试 mock 重
- 引用：本文 §2

### ADR-003：idempotency_record 与业务 TX 同库同事务

- 决策：service 层在 `bizDB.Transaction` 内同时 INSERT idempotency_record（middleware 仅 lookup + 409）；避免业务回滚但 idem 已写
- 否决：先写 idem 再 commit → 边缘可能重放成功的 idem-key 但业务未生效
- 引用：PRD §18 + Security C-2

### ADR-004：outbox poller 与 saga retry worker 物理分离

- 决策：`cmd/outbox-poller` 仅消费 `consume_log_outbox` → `revenue_log`，无 saga 编排；saga retry 在 `cmd/api` 进程内 goroutine pool（或 `cmd/saga-runner` Phase 2）
- 否决：合并 → 一个 GC pause / outbox lag 拖累 saga 推进
- 引用：`integration-design` §3 + `overview` §3.1

### ADR-005：partner_debt 模型 vs 负 balance（F-2 待落）

- 决策（v0.1 倾向）：方案 A 独立 `partner_debt` 表（§3.22）。分润流入 balance 永远 ≥ 0；refund 已支付造成欠款写 `partner_debt` 行；月结优先冲销 debt
- 否决方案 B：负 balance + 阈值 suspend → balance 语义混乱（既是台账又是欠款）；UI 难于区分
- 引用：PRD §22.1 F-2 + PM HIGH-2

### ADR-006：audit_log sealed-by-async-batcher

- 决策：见 §10。应用写 unsealed；sealer 单 leader 进程 200ms tick；sealer crash 由 K8s Lease fallback
- 否决：BEFORE INSERT trigger（concurrent 下哈希链断裂）；SERIALIZABLE（杀吞吐）
- 引用：Security HIGH-r2-1 + PRD §22.1 F-7

### ADR-007：每 service 入口处显式 PII 标签 + redactor

- 决策：service Request DTO 字段在 struct tag 标 `pii:"true"`；middleware 入口写 audit / 日志前必经 `pii.Scrubber` 反射 strip
- 否决：handler 层手工 redact → 容易遗漏
- 引用：PRD §16.5/§16.6 + Security M-r2-5

### ADR-008：DEK cache 按 (scope, key_version)；prod pprof 关闭

- 决策：见 §9.1 / `overview` ADR-009
- 否决：单 keyID 不 scope → 跨租户 DEK 共享，单堆 dump 损全部 PII
- 引用：Security HIGH-r2-2

### ADR-009：partner_wallet.held_amount 处理（M-4 待落）

- 决策（v0.1 倾向）：保留 + 每日 drift 检查（§6 表）；Phase 2A reassess 是否 drop
- 否决 drop：影响 dashboard 单查 partner.available 时多一个 join sub-query
- 引用：Architect M-4

### ADR-010：HMAC key rotation 走 KMS Secret Manager + N+1 active

- 决策：`fyapi_client` 启动期 LoadAll() 拿 N+1 keys（current+previous，7 天 overlap）；KMS Secret 列表名 `fy-api-hmac/v{n}`；rotator 进程每 30d 生成新 key 并切换 active
- 否决：DB 表存 secret → 写入热点 + 审计噪声
- 引用：Security M-R2-2 + PRD §21.3

### ADR-011：Redis SETNX leader（Phase 1）；Phase 2 切 K8s Lease

- 决策：`pkg/leader/redis_setnx.go` 实现 + 心跳；待 ops topology Q11+ 落地后评估切换
- 引用：`overview` ADR-008 + PRD §22.1 F-5

### ADR-012：ginx 中间件链顺序（中央化）

- 决策：所有 router group 引用 `internal/app/middleware.Chain()` 公共构造器；防止局部漏 CSRF / scope check
- 否决：每个 router 自由选 middleware → 越权矩阵漏洞
- 引用：本文 §7.1

---

## 19. 与 overview / integration-design 的"待 align"清单

> 本文严格遵守上游契约；以下为发现的边缘需架构师 round review 时确认。**不在本文修改**。

1. **outbox `consume_log_outbox.deleted_at` 缺失** — `integration-design` §1.5.2 DDL 没有 `deleted_at` 列；本文 §5.4 也按"DELETE 行"处理（ADR-011）。但 `integration-design` §3.3 poller 伪代码显示 `tx.Delete(&r)`（GORM 软删除会写 `deleted_at`）。建议明确：物理 DELETE，且 GORM struct 不含 `gorm.DeletedAt`。— **trivial**
2. **`audit_log.id` 与 `audit_log_unsealed.id` 同源** — 本文 §10.1 / §3.13 把 `audit_log.id` 设为非 AUTO_INCREMENT，由 sealer 用 unsealed.id 写入。`overview` ADR-006 没有明确 id 来源。需架构师确认。
3. **idempotency_record TTL `≥ saga wall-clock`** — 本文按 24h，`integration-design` §1.7 internal_idempotency 7d。`overview` ADR-W / Architect L-2 要求 invariant `partner-api 24h ≥ saga 1h`。在 `cmd/api` 启动期断言 `biz_setting.saga_wall_clock_hours <= idempotency_ttl_hours` 否则 panic。— **invariant 增加**，建议 `overview` 明列。
4. **`saga_step` UNIQUE(saga_id, step_name)** — `integration-design` §4.1 schema 没声明 UNIQUE，本文 §3.17 加了。两次同 saga 同 step 重试如何 reconcile？建议在 retry worker 用 `INSERT ... ON DUPLICATE KEY UPDATE attempts=attempts+1, last_error=...`。— **建议补 `integration-design` §4.6**。
5. **partner_wallet.held_amount denorm 决定** — Architect M-4 + ADR-009：本文倾向保留。需 round review 一锤定音；如最终 drop，则 §3.3 DDL 删字段、`wallet.Available()` 改纯计算。
6. **F-1 per-model markup** — 本文 §3.6 `partner_pricing_rule.model_name` 已留字段；`integration-design` §1.4.4 决策 Phase 1 不开发。Phase 2A 入场前需要在 `fy_api_db` 加 `user_model_ratio_override` 表（覆盖层）。本文不在 Phase 1 挂钩 service。— **依赖 F-1 决策**。
7. **partner-api 是否管理 fy_api_db.users.email 唯一性的判定** — §5.2 场景 N 用 email 命中检查"已是直营"。Fy-api `users.email` 当前唯一性策略需架构师确认（覆盖层是否需保护）。
8. **SLS 跨索引 trace_id 查询** — `overview` I-4.2 要求 partner-api 与 Fy-api 共享同 SLS project，但 Fy-api 有自己的日志策略；ops 层是否能保证两侧 logstore 在一个 project / index？需 ops Q11+ 确认。
9. **MFA 强制时机 vs partner KYC 流** — F-9 要求 KYC 通过即强制 TOTP；本文 §7.5 已落实；**v0.2 D-9 verdict**：partner KYC 通过后强制 logout + revoke 全部 jti 重新登录（任何 jti `iat < kyc_approved_at` 一律视为 revoked，§7.2 中间件已实现）。
10. **Pub/Sub `customer_update` 频道** — **v0.2 D-10 verdict**：Phase 1 不引入；partner 改 markup 影响是 revenue_log 计算路径（outbox 消费时解析 rule），customer 余额视图不受影响。frontend TanStack Query `['customer', id]` 30s stale 即够。Phase 2A 重评。

> v0.2 状态：上述 10 条中 D-1..D-9 已 FIXED 或显式 ACCEPTED-AS-DEBT；D-8（SLS 跨 project trace_id）和 D-10（customer_update 频道）保留为债务。

---

## 20. v0.1 → v0.2 CHANGELOG（Round-1 合入）

> 状态枚举：**FIXED** / **ACCEPTED-AS-DEBT** / **DEFERRED-TO-PHASE-2A/2B** / **PRD-PATCH**。本节是 Round-2 reviewer 的导航索引。

### 20.1 CRITICAL（13 条全表）

| ID | 来源 review | 处置 | 本文落点 |
|---|---|---|---|
| ARCH-CRIT-1 / SEC-CRIT-1 | JWT 载体冲突 | **FIXED** | §7.2 改 `extractFromCookie("tnbiz_access")`；§7.6 CSRF double-submit；ADR-007 v0.2 引用 |
| ARCH-CRIT-2 / SEC-M-r2 | response_json 字段语义 | **FIXED** | §3.16 已是 `response_cipher` + `response_key_id`（v0.1 已对齐 backend）；§8.1 idempotency middleware 解密重放 |
| ARCH-CRIT-3 / PM-HIGH-4 | saga.force_resolve verb 缺失 | **FIXED**（文档）+ **PRD-PATCH-1** | §7.4 加 verb 枚举；§4.5/§4.5.1 dual-control 重写；ADR-015 引用 |
| SEC-CRIT-2 | BOLA row-level guard 普遍缺失 | **FIXED** | §5.3 service 伪代码改为 `repo.FindByID(ctx, scope, id)` + entity scope 校验；§5 BOLA pattern 段；CI golangci-lint analyzer `bola-scope-required` 强制；§15.3 升级 |
| SEC-CRIT-3 | JWT revocation fail-open | **FIXED** | §7.2 中间件 `redis.Exists` 失败 → 503 + alert，不放行；degrade 模式 env bypass token |
| SEC-CRIT-4 | OSS presigned PUT 无服务端校验 | **FIXED** | §9.4 新增 `PresignPut` + `VerifyUploadedKYCObject`（HEAD + magic-byte + 异步病毒扫）+ CI AST scan |
| SEC-CRIT-5 | dual-control 时间窗叠加 | **FIXED** | §4.5.1 重写 6 项 verifyDualControl 检查；§3.13 audit_log_unsealed 加 `second_approver_id` |
| SEC-CRIT-6 | 供应链 / SBOM | **FIXED** | overview ADR-016 引用；本文 §13.5 ops runbook 等待补 |
| SEC-CRIT-7 | jwt_public_key_pem in biz_setting | **FIXED** | §3.15 加 `value_type ENUM('plain','secret_ref')`；`jwt_verify_key_pem` 改 secret_ref；§7.2 公钥从 env 注入；§7.4 拆 `system.config_write.security` verb |
| COMP-CRIT-1 / M-8 | 备案号公示 9 keys | **FIXED** | §3.15 biz_setting 9 个 compliance.* key 注册；frontend 主落 `<ComplianceFooter>` |
| COMP-CRIT-2 / M-11 | 12377 上报通道 | **FIXED** | §3.24 `content_safety_report` 表 + §6 `content_safety.report.dispatcher` 24h SLA cron + §4.5 `/admin/content-safety/reports` endpoint |
| COMP-CRIT-3 / M-7 | KYC 5y 冷归档销毁 cron | **FIXED** | §6 增 `kyc.purge.cold` daily 04:30；§3.9 `cold_archive_expires_at` 索引就绪 |
| COMP-CRIT-4 / M-6 | outbox 跨境隔离 | **FIXED**（integration 主落） | integration §1.5.2 / §6.4 v0.2；本文 §3.19 引用 |

### 20.2 HIGH（30 条）

| ID | 处置 | 本文落点 |
|---|---|---|
| ARCH-HIGH-4 (#5/D-4) saga_step UNIQUE | **FIXED** | §3.17 已含 `uk_saga_step (saga_id, step_name)` |
| ARCH-HIGH-5 (B-1) outbox poller 两阶段 | **FIXED**（integration 主落，本文 §5.4 时序图保留） | integration §3.3 v0.2 |
| ARCH-HIGH-6 (#2) CSRF 不一致 | **FIXED** | §7.6 重写为 double-submit + Origin |
| ARCH-HIGH-7 (D-5) drop held_amount | **FIXED** | §3.3 字段 DROP；ADR-012 v0.2 引用；§5.3 wallet.Available() 计算 |
| ARCH-HIGH-8 (#9) wallet_log UNIQUE 粒度 | **FIXED**（保留双键） | §3.4 注释决议；type 枚举增 `platform_isv_commission_in` |
| ARCH-HIGH-9 (B-4) DLQ 阈值 | **FIXED**（integration 主落） | §6 worker 表 retry < 30 注释引用 |
| ARCH-HIGH A-1 pending 态 middleware | **FIXED** | §8.1 中间件三态分支重写（completed/pending+saga 进度/orphaned 500）|
| ARCH-HIGH A-2 wallet version mismatch | **FIXED** | §5.3 case；§6 worker 表注释；retry worker 不计 attempts |
| ARCH-HIGH A-3 不回写 terminal | **FIXED** | §6 worker 注释；integration §4.1 schema 注释 |
| SEC-HIGH-2 全局 rate-limit | **FIXED** | §7.8 v0.2 全局 rate-limit middleware 表 |
| SEC-HIGH-3 refresh rotation | **FIXED** | §7.9 v0.2 refresh rotation + revoked jti |
| SEC-HIGH-4 partner WebAuthn 强制 | **FIXED** | §7.5 v0.2 强制阈值表 |
| SEC-HIGH-7 biz_setting 改动 schema/dual-control | **FIXED** | §3.15 keys 表 + `system.config_write.security` |
| SEC-HIGH-8 mTLS mesh 边界 | **FIXED**（integration 主落） | integration §6.4 v0.2 |
| SEC-HIGH-9 admin VPN / 零信任 | **ACCEPTED-AS-DEBT T-11** | §13 ops runbook 占位；overview §10 风险登记 |
| SEC-HIGH-10 biometric 生命周期 | **FIXED** | §3.9 字段 `biometric_liveness_url` + `biometric_purged_at` + §6 cron `biometric.purge` |
| SEC-HIGH-11 blind index | **FIXED** | §3.9 `legal_person_id_blind_index` + `bank_account_blind_index` + 索引 |
| SEC-HIGH-12 audit_log.target_id | **FIXED** | §3.13 加 `target_key VARCHAR(128)` + 索引 |
| SEC-HIGH-13 内容安全 rate-limit + 上报闭环 | **FIXED** | §7.8 100/min/tenant；§3.24 `content_safety_report` + §6 cron |
| SEC-HIGH-14 webhook body limit | **FIXED** | §5.7 关键 invariant v0.2 BodyLimit(1MB) + timeout 3s |
| COMP-HIGH-1 partner.tax_status + 41 号公告 | **FIXED** | §3.1 字段；§5.5 `ComputeWithheldTax` 实现说明；§6 `tax.withholding.annual` cron |
| COMP-HIGH-2 consent_type 增枚举 | **FIXED** | §3.18 `chk_consent_type` 加 `automated_decision` / `third_party_share` |
| COMP-HIGH-3 第三方 PII 共享审计 | **FIXED**（架构约束） | §5.6 KYC OCR 入口规约；CHANGELOG 末段说明 |
| COMP-HIGH-4 模型白名单月度对齐 | **FIXED** | §6 `model_whitelist.review` monthly cron |
| COMP-HIGH-5 content_safety_event DDL | **FIXED** | §3.23 新表 |
| COMP-HIGH-6 发票销售方 + 10y 留存 | **FIXED** | §3.12 `seller_entity_id`/`seller_tax_no`/`archive_expires_at`；§3.12 `red_flush_request` 表 |
| COMP-HIGH-7 PIA 报告生成器 | **FIXED**（schema 落地） | §3.25 `pia_report` 表；§6 `pia.report.annual` cron |
| COMP-HIGH-8 DPO 入口 / 投诉受理 | **FIXED** | §3.26 `pipl_complaint` 表；§4.5 `/admin/pia` endpoint；frontend 主落 footer |
| COMP-HIGH-9 partner_debt 上调 Phase 2A | **FIXED** | §3.22 / ADR-010 v0.2 |
| PM-HIGH-1 M2-15 Phase 冲突 | **FIXED**（frontend 主落） | 见 frontend CHANGELOG |
| PM-HIGH-2 场景 I 孤儿客户 UX | **FIXED** | §3.2 customer.status 已含 orphaned/adopted/direct；§5.14 service；§6 `orphan.cleanup` cron；§4.5 `/admin/customers/orphaned` |
| PM-HIGH-3 红冲 UI | **FIXED** | §4.5 `/admin/invoices/{id}/red-flush` endpoint；frontend 主落 UI |
| PM-HIGH-4 场景 K 争议 | **FIXED** | §5.13 dispute 骨架 + DDL；§4.5 endpoints |

### 20.3 MEDIUM（合计 ~34 条；本文相关）

- ARCH-MED 8.2 customer.group_name drift checker → **FIXED** §15.2 加 invariant I-C-N（drift 注释；具体 check 在 wallet.drift.checker 旁路扩展）
- ARCH-MED 8.6 pricing_rule UNIQUE NULL → **FIXED** §3.6 generated column
- ARCH-MED 8.9 KYC encryption_key_id 冗余 → **FIXED** §3.9 顶级字段 DROPPED
- ARCH-MED 8.12 invoice fapiao_serial UNIQUE → **FIXED** §3.12 `uk_invoice_fapiao`
- ARCH-MED 8.18 ticket.category 枚举 → **FIXED** 加 `content_report`
- ARCH-MED 8.20 customer_partner_change_log status → **FIXED** §3.20 加 status 列
- ARCH-MED A-2 retry worker version mismatch → **FIXED**（HIGH 同条）
- ARCH-MED C-2 dual-control 同人校验 → **FIXED**（CRIT-5 已覆盖）
- ARCH-MED #20 Prometheus /metrics 限制 → **FIXED** §13 ops runbook（auth-gated 内部访问）
- SEC-MED-1..14（绝大多数 backend 处） → **FIXED**（详见各 §3 / §6 / §9 注释）；MED-12（已支付退款 fallback）走 partner_debt（§3.22 上调）
- COMP-MED-13..20 → **FIXED**（§3.9 OCR 加密、§3.13 audit_log_pii diff_cipher 提到 65535、§3.20 三方确认 status、partner_wallet_log 加 ISV 佣金 type 等）
- PM-MEDIUM-1..9 → **FIXED**（§3.15 备案 keys、§5.6 KYC 3 次/年 invariant I-K-4、§4.5 `/admin/customers/transfers` endpoint、§5.7 EPay 拒绝分支、§5.5 settlement is_partial / progress_offset 注释、§4.5 endpoint 拆 approve / kyc.review、§12.1 待补 business KPI metrics（DEFERRED-TO-PHASE-1-WEEK-3））

### 20.4 LOW

- SEC-LOW 1..9 → **FIXED**（§3.15 value_type、§3.6 markup 上界 5.0、其他在 §13/§9）
- ARCH-LOW 21..27 → **FIXED**（cmd 进程清单 ADR-004 注释 saga-runner 占位、UUIDv7 canonical 统一）

### 20.5 起草人（Backend Architect）原 §19 "待 align" 处置

D-1..D-10 全部状态：D-1/2/3/4/5/6/7/9 **FIXED**；D-8 **ACCEPTED-AS-DEBT T-6**；D-10 **DEFERRED-TO-PHASE-2A**。

### 20.6 PRD Patch 需求

见 overview §14.5 PRD-PATCH-1（saga.force_resolve verb + system.config_write 拆分）+ PRD-PATCH-2（biz_setting key 列表 + compliance.* / payment.platform_isv_mchid 等）。在 PRD patch 落地前，本文用 hardcoded allowlist + 启动 feature flag + audit 三层兜底。

---

> 本文档为 v0.2。下一步：进入 Round-2 review；目标 0 CRITICAL / HIGH ≤ 4 + 所有 HIGH 在债务清单中显式接受。

---

## 21. v0.2 → v0.2.1 ADDENDUM（Round-1 stale 摘要修正）

> v0.2 的修订摘要基于 Round-1 stdout 的早期 stale 计数（2 CRIT / 6 HIGH for Architect）；最终落盘的 02-Architect-review.md 实际是 4 CRIT / 11 HIGH，PM 是 6 HIGH（不是 4）。本节是补丁，只列 v0.2 漏掉或仅部分覆盖的项；其余条目仍以 §20 CHANGELOG 为准。

### 21.1 ARCH-CRIT 增量（v0.2 未覆盖）

| ID | 来源 | 处置 | 本文落点 |
|---|---|---|---|
| **ARCH-CRIT-NEW-A** | idempotency_record 同 TX 矛盾（ADR-003 vs §8.1 middleware c.Next 后 Insert） | **FIXED** | §8.1 末段 `repo.Insert(c, ...)` 标注**整段删除**；正解块明示 service 在 `bizDB.Transaction` 闭包内同 TX 写入；ADR-003 承诺不变，实现路径修正 |
| **ARCH-CRIT-NEW-B** | outbox poller DELETE 三义（integration §3.3 无条件 Delete vs §3.4 markFailed vs §5.4 ConsumedAt） | **FIXED** | 主落 integration §3.3/§3.4/§5（ack 路径成功 → DELETE / 失败 → markFailed retry++），本文 §5.4 不变（v0.2 已与正解一致："ack 成功 DELETE / 失败 markFailed"）；本节抄录决议供 backend 阅读 |
| **ARCH-CRIT-NEW-C** | 缺失 DDL：`pipl_request` / `password_reset_token` | **FIXED** | §3.27 `pipl_request` 完整 DDL（Phase 2A，承载 PIPL §44-§47 5 类权利）；§3.28 `password_reset_token` 完整 DDL（Phase 1，§7.9 双因子重置）；§14.3 phase 演进表同步登记 |

> 注：`content_safety_event` 已在 v0.2 §3.23 落地，本轮不重复。

### 21.2 ARCH-HIGH 增量（v0.2 未覆盖或仅部分）

| ID | 来源 | 处置 | 本文落点 |
|---|---|---|---|
| **ARCH-HIGH-NEW-D** | F-3 saga `Idempotency-Key` 类型违约（BIGINT vs OpenAPI uuid 契约） | **FIXED** | §3.21 `topup_intent.saga_id` 改 `VARCHAR(64) NOT NULL` + `uk_topup_saga_id`；service 层生成 UUIDv7；§5.7 时序图 + invariant 全部改为 `Idempotency-Key=topup_intent.saga_id`，符合 §4.1 OpenAPI `format: uuid` |
| **ARCH-HIGH-NEW-E** | webhook 路径无独立 idempotency middleware | **FIXED** | §7.1 v0.2.1 新增 webhook 中间件链（IPAllowlist → SignatureVerify → WebhookIdempotency(Redis SETNX (provider,signer,event_id) 24h) → Handler）；与 user-facing idempotency 隔离（不同表 / 不同失败策略 / 不同 namespace）；§5.7 时序图标注 |

### 21.3 PM-HIGH 增量（v0.2 摘要只覆盖 4 条；实为 6 条）

| ID | 来源 | 处置 | 本文落点 |
|---|---|---|---|
| **PM-HIGH-5** | 发票 backend §3 invoice DDL 缺失 | **CONFIRMED-ALREADY-FIXED** | v0.2 已落 §3.12 `invoice_application` + `invoice_title` + `red_flush_request`；本轮抽查无遗漏；CHANGELOG §20.2 PM-HIGH-5 追登 |
| **PM-HIGH-6** | 客户充值 saga escalated UX | **FIXED**（backend 协同）| §5.7 invariant 显式："saga 进入 escalated 时写 `notification_outbox` event_code='payment.topup.escalated' → 触发 inapp + email 通知客户"；frontend §7.5 主落 escalated UI（见 frontend §21 ADDENDUM） |

### 21.4 Security / Compliance 抽查复核

抽查 5 条 v0.2 声称 FIXED 的 Security 项，逐项 grep 验证：

| 抽查项 | 验证结果 |
|---|---|
| **SEC-CRIT-2** BOLA repo 层 row-level guard | ✅ §5 BOLA pattern 段 + §15.3 CI analyzer 描述齐；§5.3 `customerRepo.FindByID(ctx, scope, in.CustomerID)` 字面落地；其余 §5.x service 段引用 BOLA pattern 段（按 v0.2 §20.1 决议不逐段重写，CI analyzer 兜底） |
| **SEC-CRIT-4** OSS PresignPut magic-byte 校验 | ✅ §9.4 `PresignPut(allowedMime, maxBytes, ttl ≤ 300)` + 提交后 `VerifyUploadedKYCObject` HEAD + magic-byte 二次校验 + 异步病毒扫，CI AST scan 确认 `allowedMime` 字面量 |
| **SEC-CRIT-5** dual-control 30min cooldown 中间件 | ✅ §7.4 `cooldownStillActive(initiator.ActorID, verb)` + §4.5 endpoint 表 `saga.force_resolve (dual-control + 30min cooldown)` 字面落地 |
| **SEC-CRIT-6** SBOM / 镜像签名工具选型 | ✅ overview ADR-016 已点名 `govulncheck` + `nancy` + `pnpm audit` + `cosign sign` + `syft` SBOM + distroless base；本文 §13 ops runbook 引用（实际 CI 配置 ACCEPTED-AS-DEBT，由 ops 在 Phase 1 第 1 周交付） |
| **SEC-CRIT-7** jwt_public_key_pem 移走后存储路径 | ✅ §3.15 `jwt_verify_key_pem` `value_type=secret_ref`（指向 KMS Secret ARN，非 PEM 明文）；§7.2 `verifier.Verify(tok)` 注释"公钥来自 env JWT_VERIFY_KEY_PEM（KMS Secret Manager 注入；不来自 biz_setting）"；§7.4 拆 `system.config_write.security` verb dual-control |

抽查 5 条 v0.2 声称 FIXED 的 Compliance 项：

| 抽查项 | 验证结果 |
|---|---|
| **COMP-CRIT-3** kyc.purge.cold cron + OSS lifecycle 5y | ✅ §6 `kyc.purge.cold` daily 04:30 字面登记；`cold_archive_expires_at` 索引 §3.9；ops OSS lifecycle 5y 在 §6 注释（runbook 由 ops 配置） |
| **COMP-CRIT-2** 12377 上报通道 24h SLA | ✅ §3.24 `content_safety_report` 表 + §6 `content_safety.report.dispatcher` cron every 1min 处理 `sla_due_at < NOW()+10min` |
| **COMP-HIGH-1** partner.tax_status + ComputeWithheldTax + 41 号公告 | ✅ §3.1 `tax_status` 字段 + CHECK；§5.5 `ComputeWithheldTax` 分档伪代码；§6 `tax.withholding.annual` cron yearly Jan 31 |
| **COMP-HIGH-6** 发票 seller_entity_id + 10y 留存 | ✅ §3.12 `seller_entity_id` / `seller_tax_no` / `archive_expires_at` 字段；§5.8 invariant 明示 OSS lifecycle 10y |
| **COMP-HIGH-7** PIA 报告 | ✅ §3.25 `pia_report` 表 + §6 `pia.report.annual` cron + frontend admin 路由 |

抽查未发现"声称 FIXED 但文档里其实没真改"的情况（v0.2 落地质量超过早期 stdout 摘要预期）。

### 21.5 v0.2.1 跨文档对齐摘要

| 跨文档影响 | 同步修订 |
|---|---|
| §8.1 idempotency middleware 正解 | overview ADR-003 注释保持不变；integration §4.6 saga 终态写入仍由 service 同 TX 完成（与 v0.2 一致） |
| §3.21 saga_id UUIDv7 | integration §4.5 时序图 step 10 同步（见 integration §11 ADDENDUM） |
| §7.1 webhook 中间件链 | integration §4.5 webhook 路径标注；overview §10 风险登记（无新增风险，仅细化） |
| §3.27 / §3.28 新表 | overview §10 风险登记不动；integration §6 GRANT 表添加 `password_reset_token` / `pipl_request` SELECT/INSERT/UPDATE for `tnbiz_app`（已在 v0.2 GRANT pattern 范围内，无需补行） |

---

## 22. v0.2.1 → v0.2.2 ADDENDUM（Round-2 预防性闭环）

> 本节闭环主架构师在 v0.2.1 落盘后自查到的 3 项 reviewer 仍可能打回点，全部主落 backend。

### 22.1 闭环 3 项

| ID | v0.2.1 残留问题 | v0.2.2 处置 | 本文落点 |
|---|---|---|---|
| **R2-Risk-1（ARCH-CRIT-NEW-A 终态）** | §8.1 上方 v0.2 中间件代码块仍含 `c.Next` 后的 `repo.Insert(c, &Record{...})`；v0.2.1 仅在末段加注释否定，未真删 | **代码块整段重写**：middleware 仅做 SELECT 命中检查 + 重放 / 透传，**不再**调 `repo.Insert`；新增 service 层 `bizDB.Transaction` 闭包内 `idemRepo.Insert(tx, ...)` 骨架；新增 invariant 三连（middleware grep 反断言、AST analyzer 强制 service 双写同 TX、e2e panic 回滚）；ADR-003 承诺不变 | §8.1 v0.2.2 重写 |
| **R2-Risk-2（ARCH-CRIT-NEW-B 终态）** | `outbox.purge` cron 在 v0.2.1 仅在 integration §3.4 / overview ADR-014 提及，未在 backend §6 cron 表登记 | §6 cron 表新增 `outbox.purge` 行：周期 `15 3 * * *` Asia/Shanghai（每日 03:15 off-peak）/ 触发条件 `consumed_at < NOW() - INTERVAL 30 DAY` / 幂等（leader-only + 自然幂等）/ 失败补偿（退避 + DPO page）/ SLO（< 30 min；残留行 = 0）/ ops `@platform-ops` / Phase 1；同步：integration §3.4 / overview ADR-014 注脚字面引用本行 | §6 v0.2.2 |
| **R2-Risk-3（ARCH-CRIT-NEW-C 终态）** | §3.28 `password_reset_token` 仅有 DDL，§7.9 缺二因子流程时序图 + e2e | §7.9 拆 4 子节：§7.9.1 mermaid 时序图（双通道送 token+OTP / 同步 jti revoke / IP/UA 软约束）+ §7.9.2 8 条 invariant（PR-INV-1～8）+ §7.9.3 ≥3 条 e2e（E2E-PR-1 单次有效+全设备下线、E2E-PR-2 5 次失败 invalidate、E2E-PR-3 信息恒等防枚举）+ §7.9.4 frontend 路由对齐 `/auth/forgot` / `/auth/reset/:token` | §7.9.1～7.9.4 v0.2.2 |

### 22.2 跨文档对齐摘要

| 跨文档影响 | 同步修订 |
|---|---|
| §8.1 middleware 重写 | overview ADR-003 注脚不变；本文成为唯一字面源；CI analyzer §15.3 在 Phase 1 第 1 周交付 grep + AST 双校验 |
| §6 `outbox.purge` 登记 | integration §3.4 v0.2.2 注脚改写为引用本表；overview ADR-014 v0.2.2 注脚同步 |
| §7.9 双因子重置 | frontend §6 路由表 v0.2.2 ADDENDUM 追加 `/auth/reset/:token`；frontend §7 zod schema 同步密码强度规则（已有，不变） |

### 22.3 本轮自查仍存在的 HIGH 候选（移交 Round-2 评估）

- **R2-Risk-4（HIGH 候选）**：§7.9 双因子由 email 链接 + SMS OTP 组成；用户邮箱被攻陷 + SIM swap 同时发生时仍可重置。Mitigation：staff 已强制 WebAuthn step-up（§7.5 已落）；customer / partner 接受残余风险；v0.2.3 评估"高风险账户额外 KYC 实人比对"。**v0.2.2 不处理**，写入债务清单。

---

## 23. CHANGELOG: v0.2.2 → v1.0 收口

> 4 reviewer Round-2 verdict（2026-05-12 落盘）：PM PASS / Architect PASS-CONDITIONAL / Security PASS-CONDITIONAL-ACCEPT / Compliance PASS_WITH_NOTES。本节列本文相关的 cosmetic 修复 + 落点。

### 23.1 本文相关 cosmetic 修复（10 条 ✅FIXED）

| # | Cosmetic | 本文落点 |
|:---:|---|---|
| 1 | partner→customer token footer 勘误（H-7 文档清理） | §4.4 footer 改为"partner 不允许为 customer 创建 API key；customer 必须自助；`customer.allocate_quota` verb 仅用于充值额度，不返 token" |
| 3 | `notification_outbox` 加 `ref_id` 列 + UNIQUE(event_code, recipient, ref_id) | §3.18 DDL |
| 5 | backend §6 cron 表加 `password_reset.purge` 行（每日 04:30 / @platform-ops） | §6 cron 表新行 + §3.28 末注释更新 + §7.9.2 PR-INV-8 文案更新 |
| 6 | `outbox.purge` escalation 修正：DPO page → @platform-ops | §6 cron 表 `outbox.purge` 行 |
| 7 | `password_reset.purge` cron 正式登记（同 #5） | 同 #5 |
| 8 | webhook fail-open Redis 健康/故障两态切换显式声明 | §7.1 v0.2.1 webhook 中间件链 fail-open 注释扩展 |
| 9 | `idempotency_record` 加 `trace_id` 列（DDL + struct 双改）| §3.16 DDL + §8.1 service `idempotency.Record{...}` 字面字段 |
| 10 | audit-sealer leader 注释微调（明确 single-replica 仍需 Lease + ops 风险接受方式）| §6 cron 表 `audit.sealer` 行 |
| 4（关联）| `internal-api.yaml` 物理路径明示（主落 integration） | 本文 §5 引用 integration §6.1 |
| 11/12（关联）| 架构债务 + 合规 follow-up 合入 overview 附录 A/B（主落 overview） | 本文不动，参照 overview |

### 23.2 4 reviewer Round-2 verdict 摘要

| Reviewer | Verdict |
|---|---|
| PM | ✅ **PASS**（0C/0H/4M/5L） |
| Architect | ✅ **PASS — CONDITIONAL**（0C/0H/6M/4L；本文 §3.16 / §3.18 / §6 / §4.4 / §7.1 / §7.9.2 全部闭环 cosmetic） |
| Security | ✅ **PASS — CONDITIONAL-ACCEPT**（0C/0H/4M/2L；T-11 + R2-Risk-4 ACCEPT-AS-DEBT）|
| Compliance | ✅ **PASS_WITH_NOTES**（0C/0H/4M/5L；可追溯性 65% → 93%）|

**v1.0 定稿条件全部达成**。架构债务与合规 follow-up 集中收编于 overview 附录 A/B。Phase 1 工程编码立即从本文 §17.1 启动。

---

## 24. v1.0 → v1.1 CHANGELOG（合入 Fy-api review）

> 来源：`reviews/fy-api-review/01-fy-api-side-review.md`（ACCEPT-WITH-CHANGES, 2026-05-12）。本文 v1.1 仅做交叉文档对齐 + 引用 OVERLAY 编号修正；详细工程级修订集中在 `integration-design.md` v1.1 §17。

### 24.1 本文涉及修订

| 来源 | v1.1 处置 | 本文落点 |
|---|---|---|
| Fy-api OVERLAY 编号：v1.0 散见"B-8..B-14" 引用 → 实际从 B-12 起 | **NOTE-FIXED**（本文不直接列 OVERLAY 编号；以 integration §1.8 v1.1 为准）| §3.4 / §5.4 outbox 描述引用以 integration §3.3 v1.1 为准；本文不冗余引用编号 |
| Redis client 版本：v1.0 部分伪代码 / 中间件示意提到 `go-redis/v9` → 应统一 `go-redis/v8` v8.11.5 | **NOTE-FIXED**（本文 partner-api 自身依赖以 `TraceNexBiz/go.mod` 为准；Fy-api 侧伪代码以 integration §1.1.3 v1.1 为准）| 本文 §7.1 webhook idempotency middleware Redis 调用以 `redis/v8` ctx-first 风格为参考实现 |
| 部署形态：v1.0 暗示 K8s + Istio mesh → 实际 Podman + Nginx | **NOTE-FIXED** | partner-api 与 Fy-api 部署描述以 overview §2 流量表 v1.1 + integration §6.4 v1.1 为准；本文 §6 cron / §7.1 webhook 中间件不直接依赖 mesh 假设，无需逐字修订 |
| outbox poller 跨库 GORM 多 DB 配置 + 连接池建议 | **NEW** | §5.4 outbox poller 实施时按 integration §6.2 v1.1 GORM 多 DB 配置写法；本文不重复 |
| Fy-api 实施 5-PR 拆分 / feature flag | **NOTE** | 本文不涉及 Fy-api 内部 PR 拆分；partner-api 侧的 outbox poller 启用条件 = `OVERLAY_OUTBOX=true`（详见 integration §14） |

### 24.2 不涉及

本文 §3 数据模型 / §4 saga / §5 outbox / §6 cron / §7 webhook / §8 idempotency middleware 的字段定义、状态机、事务边界、KMS envelope、leader 选举等**全部不变**。Fy-api review 全部影响范围在 Fy-api 覆盖层 + 部署形态 + OVERLAY 编号，partner-api 自身 service 层无需重写。

### 24.3 跨文档同步状态

- `00-architecture-overview.md` v1.1 §18 / 附录 C / T-23 / T-24 已更新
- `integration-design.md` v1.1 §17 / §1.8 / §6.4 / §14 / §15 / §16 已更新
- `frontend-design.md` 不涉及（保持 v1.0）

---

## 25. v1.1 → v1.2 CHANGELOG（Fy-api round-2 cosmetic）

> 来源：`reviews/fy-api-review/02-fy-api-round2-review.md`（ACCEPT, 2026-05-13）。Round-2 verdict = ACCEPT（0 CRITICAL / 0 HIGH），4 项残余文字级补强全部位于 Fy-api 覆盖层 + 部署形态 + CHANGELOG 历史叙事，**partner-api（本文档）侧无字段 / 状态机 / 事务边界 / KMS / cron / webhook / saga / outbox poller 改动**。

### 25.1 本文涉及修订

| 来源 | v1.2 处置 | 本文落点 |
|---|---|---|
| Fy-api round-2 残余 #1 flag 热加载语义对 PR-3 隐性依赖 | **NO-CHANGE**（partner-api 侧 outbox poller 启用条件 = `OVERLAY_OUTBOX=true`，是 Fy-api 侧 flag；partner-api 自身无热加载需求）| 引用 `integration-design.md` v1.2 §14.1.1 |
| Fy-api round-2 残余 #2 gh-ost RDS trigger 权限 fallback | **NO-CHANGE**（Fy-api 侧 LOG_DB migration；partner-api 自身 partner_db migration 用 GORM AutoMigrate / 维护窗口，不涉及 gh-ost）| 引用 `integration-design.md` v1.2 §1.3.4 |
| Fy-api round-2 残余 #3 v1.0 mesh CHANGELOG 撤销标注 | **NO-CHANGE**（本文 §24.1 已声明部署形态以 overview §2 + integration §6.4 为准；mesh 历史叙事不在本文 CHANGELOG）| 引用 overview v1.2 §18.4 撤销项汇总 |
| Fy-api round-2 残余 #4 部署形态 → mTLS 决策树 | **NO-CHANGE**（本文 §7.1 webhook 中间件 / §6 cron 不直接依赖 partner-api ↔ Fy-api 部署形态；mTLS 决策由 ops + Fy-api 团队执行）| 引用 `integration-design.md` v1.2 §16.1 决策树 |

### 25.2 不涉及

本文 §3 数据模型 / §4 saga / §5 outbox / §6 cron / §7 webhook / §8 idempotency middleware **全部不变**。

### 25.3 跨文档同步状态

- `00-architecture-overview.md` v1.2 §14.2 deprecated 标注 + §18.4 撤销项汇总 + §19 v1.2 CHANGELOG 已更新
- `integration-design.md` v1.2 §14.1.1 / §1.3.4 / §16.1 / §10.1 deprecated 标注 / §18 v1.2 CHANGELOG 已更新
- `frontend-design.md` 不涉及（保持 v1.0）

