# HANDOFF-W0 — TraceNexBiz 脚手架交付

**日期**：2026-05-11  
**承接**：W1a / W1b / W1c / W1d / W1e / W1f / W1g 七个并行 agent  
**前置文档**：`docs/00-architecture-overview.md` v1.2 / `docs/integration-design.md` v1.2 / `docs/backend-design.md` v1.2 / `docs/frontend-design.md` v1.2 / `prd/PRD-v1.0.md`  
**本文档作用**：W1 各 agent 接手前必读 —— W0 已经搭好了什么 / 没搭什么 / 各 agent 该改哪些目录 / 哪些文件不能动。

---

## 1. W0 已交付内容（脚手架）

### 1.1 仓库根

```
TraceNexBiz/
├── .editorconfig                tab(go) / 2-space(others)
├── .gitignore                   web dist + go bin + sbom + .env 全屏蔽
├── .github/workflows/
│   ├── partner-api.yml          go vet + golangci + race-test + build + govulncheck
│   ├── partner-web.yml          pnpm typecheck + lint + test + build + spec-drift gate
│   └── security.yml             nancy / pnpm audit / syft SBOM / cosign 占位
├── Makefile                     根级编排：dev / build / test / lint / migrate-up / sbom / sign
├── docker-compose.dev.yml       MySQL 8 + Redis 7 + LocalStack(s3,sqs) + Mailhog
├── package.json                 pnpm root；devDeps 共享
├── pnpm-workspace.yaml          4 apps + 4 packages
├── tsconfig.base.json           strict + ESNext + Bundler resolve
├── turbo.json                   build / dev / lint / typecheck / test pipelines
└── README.md                    已含 W0 dev quickstart（之前 PR 已加）
```

### 1.2 partner-api（Go）

```
apps/partner-api/
├── Makefile                     build / test / lint / migrate-up / sbom / sign
├── Dockerfile                   多阶段 + distroless nonroot
├── go.mod                       go 1.22；gin / gorm / go-redis v8.11.5（钦定）/ jwt v5 / decimal / zerolog
├── .golangci.yml                lint 配置（含 bola-scope-required 占位 exclude rule）
├── .env.example                 dev DSN / Redis / KMS noop 等
├── cmd/server/main.go           HTTP server + RequestID + SecurityHeaders + /healthz + graceful shutdown
├── internal/
│   ├── config/                  env + biz_setting key registry + 启动 invariant 校验
│   │                            keys.go：22 条 biz_setting key 注册（含 secret_ref）
│   ├── domain/                  PRD §8 全部 entity（28 张表对应 11 个 .go 文件，POJO 无 GORM tag）
│   ├── repository/              repository 接口 + mysql/ GORM 占位（partner_mysql.go 含 Find* 样例）
│   ├── service/                 service.go / doc.go + wallet.go / auth.go (signature + TODO)
│   ├── handler/                 health.go + routes.go (5 router groups 占位)
│   ├── middleware/              request_id / headers / cors / auth / csrf / idempotency /
│   │                            webhook_idempotency / bola_scope / audit / pii_scrubber
│   ├── infra/db/                gorm v2 多 DB 连接（partner_db RW + log_db RO）
│   ├── infra/redis/             go-redis v8.11.5 client
│   ├── infra/kms/               KMS 接口 + NoopService dev 实现
│   ├── infra/oss/               presigned PUT/GET 接口 + magic-byte hook
│   ├── infra/sls/               SLS hook 占位
│   ├── infra/pubsub/            MNS 抽象 (Phase 2A)
│   ├── infra/fyapi/             /api/internal/* HMAC client（接口 + 5xx unknown 路径）
│   ├── infra/license_payment/   持牌方接口 + Stub 实现（Q12 决策前）
│   ├── saga/                    Saga / Orchestrator / TxFn 接口（W1a 实现 retry worker）
│   ├── outbox/                  Poller + PurgeConsumed cron 接口
│   ├── audit/                   Sealer + Verify CLI 接口
│   └── idempotency/             Repo 接口 + PurgeExpired
├── pkg/
│   ├── errors/                  AppError + 30+ Code enum + HTTP 映射 + 单测
│   ├── permission/              22 verbs enum + Matrix（dual-control / elevated 标志）
│   ├── tracing/                 trace_id ctx helpers
│   ├── pii/                     Encrypted GORM 包装（plain 不持久化）
│   ├── piiscrubber/             3 类正则脱敏（手机 / 身份证 / email）+ 单测
│   ├── leader/                  Redis SETNX leader 选举骨架
│   └── validator/               go-playground/validator init 占位
├── migrations/                  golang-migrate 文件
│   ├── 00_bootstrap.sql         dev container init（创建库 + GRANT 简化版）
│   ├── 0001_partner_phase1.up/down.sql       partner / customer / wallet / pricing / revenue / invitation_code / customer_partner_change_log
│   ├── 0002_settlement_kyc.up/down.sql       settlement (4 张) / kyc_application
│   ├── 0003_audit_staff_config.up/down.sql   audit_log + unsealed + pii / staff / biz_setting / idempotency_record / saga_step / password_reset_token
│   ├── 0004_ticket_invoice_payment.up/down.sql seat / invoice (3 张) / ticket / notification_outbox / consent_log / topup_intent / partner_debt
│   └── 0005_compliance.up/down.sql           content_safety_event/report / pia_report / pipl_complaint / pipl_request
└── tests/e2e/                   占位（W1a 增补 chaos / saga unknown 探活）
```

### 1.3 4 个 React 站点

每站点（端口）：
- `apps/partner-web-storefront`（5173）：公开商城 / 招商落地（路由：`/` `/models` `/models/:id` `/pricing` `/apply-partner` `/apply-partner/form` `/legal/:doc`）
- `apps/partner-web-customer`（5174）：客户后台（路由按 frontend §3.2 customer 视图占位 17 条）
- `apps/partner-web-partner`（5175）：渠道商后台（路由按 frontend §3.2 partner 视图占位 13 条）
- `apps/partner-web-admin`（5176）：平台管理后台（路由按 frontend §3.3 占位 22 条；ADR-F1 独立站）

每站点已含：
- `package.json`：React 18 / Vite / Semi UI / TanStack Query / Zustand / react-hook-form / zod / react-i18next / react-router-v6
- `tsconfig.json`：继承 base + strict + Vite client types
- `vite.config.ts`：proxy `/api → :8080`
- `index.html` + `src/main.tsx` + `src/App.tsx` + `src/pages/*.tsx`（storefront） / `<Todo>` 占位（其它三站）
- 全部挂载 `<ComplianceFooter>`（Compliance CRIT-1）

### 1.4 共享 packages

- `packages/ui-kit`：`ComplianceFooter` / `PiiField`（含默认 mask + reveal）/ `MoneyDisplay`
- `packages/api-client`：axios + interceptors（`Idempotency-Key` UUIDv4 自动注入 / `X-Csrf-Token` 双提交 / `X-Oneapi-Request-Id` 透传）+ `mapApiError` toast i18n key 表
- `packages/i18n`：react-i18next init + zh-CN / en-US `common.json`（≥ 16 条占位文案）
- `packages/config`：`FeatureFlags` interface + `DEFAULT_FLAGS`

### 1.5 docker-compose.dev.yml

- `mysql:8.0` 端口 3306（utf8mb4 / strict mode；启动时跑 `00_bootstrap.sql`）
- `redis:7-alpine` 端口 6379（appendonly off，仅 dev）
- `localstack/localstack:3.5` 端口 4566（services=s3,sqs；OSS 模拟）
- `mailhog/mailhog:v1.0.1` 1025/8025

---

## 2. 验收状态（make targets）

| target | 状态 |
|---|---|
| `make build` | ✅ 通过（partner-api 二进制 13 MB） |
| `make test-api` (`go test -race ./...`) | ✅ 通过（config / middleware / repository/mysql / errors / piiscrubber 五处单测） |
| `make lint-api`（go vet / golangci） | ⏳ vet 通过；golangci-lint 待安装本地 binary |
| `make migrate-up` | ⏳ 需要本地 docker compose up 后执行；DDL 已 lint 过 |
| `pnpm install` + `pnpm -r build` | ⏳ 需要 W1e/f/g 接手时跑（W0 未提交 lockfile，避免锁死版本） |
| 4 个 React app `pnpm dev` | ⏳ 同上 |
| `partner-api` 启动后 `/healthz` 返 200 | ✅（内置 handler） |

---

## 3. W1 各 agent 入口与依赖

### W1a — 后端基础设施（middleware / saga / outbox / audit / idempotency）

**目录**：
- `apps/partner-api/internal/middleware/*`
- `apps/partner-api/internal/saga/`
- `apps/partner-api/internal/outbox/`
- `apps/partner-api/internal/audit/`
- `apps/partner-api/internal/idempotency/`
- `apps/partner-api/internal/infra/{kms,oss,sls,pubsub,fyapi,license_payment}`
- `apps/partner-api/pkg/{leader,piiscrubber}`

**关键任务**：
1. `JWT` middleware：cookie-first + fail-closed Redis revocation + KYC pass 强制 logout（backend §7.2）
2. `CSRF`：double-submit + Origin/Referer allowlist（§7.6）
3. `Idempotency`：middleware 只读 + service 层 TX 内 Insert（§8.1 v0.2.2 完整代码块）
4. `WebhookIdempotency`：(provider, signer, event_id) Redis SETNX，Redis 故障 fail-open（§7.1 v0.2.1）
5. `BOLA scope`：repository 层强制 + golangci 自定义 analyzer 落地（§7.4）
6. saga `Orchestrator` + retry worker（30s sweep / `attempts >= 30 || wall-clock >= 1h → escalated`）
7. outbox `Poller` 两阶段 claim/process/ack + `outbox.purge` cron（integration §3.3 v0.2）
8. `audit.Sealer` 200ms tick + verify CLI（§10）
9. KMS Aliyun SDK 接入 + DEK 1h 缓存 + Mlock（§9.1）
10. `fyapi.Client`：HMAC-SHA256 canonicalize + 5xx → ProbeByIdemKey（integration §1 / §5）

**依赖**：W1b/W1c 启动前完成 #1-3 + #6（service 层调用所需接口）

### W1b — 后端业务 1（渠道商 / 客户 / 钱包 / 定价 / 收益）

**目录**：
- `apps/partner-api/internal/service/{partner,customer,wallet,pricing,revenue}.go`
- `apps/partner-api/internal/repository/mysql/{partner,customer,wallet,pricing,revenue}_mysql.go`
- `apps/partner-api/internal/handler/{partner_routes,customer_routes}.go`

**关键任务**：
1. M3-04 Allocate saga（backend §5.3）：hold / commit / unknown 探活
2. M3-08 Pricing markup CRUD + overlap check（§5.4）
3. M3-02 邀请码生成 / 使用
4. M2-* 客户读模型（usage / api-keys / dashboard）
5. revenue_log outbox 消费（与 W1a outbox poller 协作）

**依赖**：W1a #1（JWT + Idempotency + Saga）

### W1c — 后端业务 2（鉴权 / KYC / 发票 / 支付 / 结算 / 通知 / 工单 / 内容安全 / PIPL）

**目录**：
- `apps/partner-api/internal/service/{auth,kyc,invoice,payment,settlement,notify,ticket,content_safety,pipl_rights,audit}.go`
- 对应 repository / handler

**关键任务**：
1. login / refresh / logout / MFA / step-up / 双因子密码重置（backend §7.2 / §7.5 / §7.9）
2. M4-03 KYC 审核（PII 信封加密 + blind index 落库）
3. M4-12 退款（含 partner_debt 路径）
4. M4-13 + M8 发票 + 红冲（Compliance HIGH-6 / MED-17）
5. settlement runner + freshness gate（§5.5）
6. content_safety 12377 上报 dispatcher（24h SLA）
7. PIPL §44-§47 用户权利工单流（30d SLA / 5d 核身）
8. notification dispatcher（5s tick / outbox-driven）

**依赖**：W1a #1-#3，W1b 完成 partner / customer / wallet 基础

### W1d — Fy-api 覆盖层（**单独仓库 ~/Projects/apiGateway/Fy-api/**）

**任务**（按 OVERLAY.md / integration §1）：
- C-1：`/api/internal/*` 7 个 endpoint（user/create / topup / deduct / token/create / user/group / group_ratio_override / usage/by-user）
- C-2：`consume_log_outbox` 表 DDL + Fy-api billing.go 写入
- C-3：HMAC-SHA256 鉴权 4 元组
- C-4：跨库 GRANT 发布到 fy_api_db
- C-5：`/api/internal/topup/by-idem-key` 探活端点

**禁止**：在 TraceNexBiz 仓库改 Fy-api 任何文件

### W1e — Storefront 前端

**目录**：`apps/partner-web-storefront/src/`

**任务**：
1. SSR 接 vite-plugin-ssr（frontend §1.3）
2. M1-05/06 渠道商申请表单（含单独同意 + zod schema + OSS direct upload）
3. 9 个 `/legal/*` 页面 SSR + 备案号渲染
4. orval codegen 接 `apps/partner-api/openapi/internal-api.yaml`（spec drift CI gate）
5. `<ComplianceFooter>` 接 `/api/me/flags` SSE

**依赖**：W1c 完成 OpenAPI spec 提交

### W1f — Customer Web

**目录**：`apps/partner-web-customer/src/`

**任务**：M2 17 个路由 + saga 三阶段 UI + KYC 表单 + topup 流（Phase 2A）

**依赖**：W1b/W1c 完成 customer / wallet / kyc / topup endpoint

### W1g — Partner + Admin Web

**目录**：`apps/partner-web-partner/src/` + `apps/partner-web-admin/src/`

**任务**：M3 + M4 + M5 + M8 + M10 全部页面，含 Allocate saga UI、KYC 审核、月结、发票红冲、审计哈希链 viewer、saga force-resolve dual-control UI

**依赖**：W1b 钱包 + W1c 鉴权 + 完整 RBAC 矩阵

---

## 4. 依赖关系总表

```
W1a (基础) ──┬─→ W1b (partner/customer/wallet) ──┐
            └─→ W1c (auth/kyc/invoice/...)  ────┼──→ W1f (customer web)
                                                 ├──→ W1g (partner+admin web)
                                                 └──→ W1e (storefront)

W1d (Fy-api 覆盖层) ── 独立并行；与 W1a #10 fyapi.Client 通过 OpenAPI spec 同步
```

---

## 5. 不变量（W1 任何 agent 都必须遵守）

> 这些是 review 二轮通过的前提条件，违反任何一条都会被 code review block。

1. **immutability**：service / repository 返回的 entity 不能被调用方 mutate；写操作走 `Update(updater func(X) X)` 模式
2. **idempotency invariant**：middleware 只读；service 层在 `bizDB.Transaction` 闭包内 `idemRepo.Insert(tx, ...)`（backend §8.1 v0.2.2 ADR-003）
3. **wallet drift = 0**：`balance == sum(partner_wallet_log.amount where partner_id=? and status='committed')`（backend §15.2 I-W-7）
4. **BOLA scope**：repository 公共方法首参带 `partnerID` / `customerID` / `staffID`；越权返 `BIZ_RES_NOT_FOUND`（不暴露存在性）
5. **PII**：明文不出库 / 不入日志 / 不入 Sentry / 不入 saga_step.payload；`pkg/piiscrubber` 三处挂点
6. **JWT cookie-first**：access_token 走 httpOnly cookie；不读 Authorization header（除 `/api/sdk/*`）
7. **fail-closed**：Redis revocation lookup 错误 → 503；user-facing idempotency 同；webhook idempotency 反向 fail-open
8. **错误 envelope**：`{success, data, error{code, message_zh, message_en, trace_id, details}}`；handler 不暴露 cause
9. **trace_id 注入**：每个 saga / revenue / audit / outbox 行带 trace_id；跨进程透传 `X-Oneapi-Request-Id`
10. **不要写完整业务**：service / handler 函数体若超过 50 行先拆；`pkg/errors.Wrap(err, code)` 显式包装

---

## 6. W0 已知遗漏（W1 接手时补）

1. `golangci-lint` 自定义 analyzer `bola-scope-required`：当前是 exclude rule，待 W1a 落地真正的 AST analyzer
2. CSP `connect-src` OSS allowlist：当前 `'self'`，W1a 接 cfg.AllowedOrigins
3. KMS `Aliyun SDK`：当前是 `NoopService`；W1a 引 `aliyun-kms-sdk-go`
4. JWT `Verifier` 实现：当前接口空，W1a 落 RSA + jti revocation
5. saga `retry worker`：接口已定义，业务实现 W1a
6. outbox `Poller.Run` 死循环 + claim/process/ack：骨架已写，主体 W1a
7. audit `Sealer.Run` 200ms tick + 哈希链：骨架已写，主体 W1a
8. OpenAPI spec：当前未生成 `apps/partner-api/openapi/internal-api.yaml`；W1c 实现 service 时由 W1c/W1e 协同导出（spec drift CI gate 依赖此）
9. orval codegen pipeline：脚本占位 `pnpm openapi:gen` 仅 echo，W1e 实现
10. e2e 测试目录 `apps/partner-api/tests/e2e/.gitkeep` 占位；W1a/b/c 各自补 chaos / BOLA matrix / saga unknown 路径

---

## 7. 不允许动的内容

- `prd/` `docs/` `reviews/` 三个目录任何文件（W0 ~ W1 全程冻结）
- `~/Projects/apiGateway/Fy-api/` 仓库（仅 W1d 可改）
- `~/Projects/apiGateway/new-api/` `~/Projects/apiGateway/old_code/` 任何文件（read-only reference）
- `apps/partner-api/migrations/0001..0005_*.up.sql` 已落地的 DDL —— W1 增量改动必须新建 `0006_*.up.sql`，不要 squash 老文件（`golang-migrate` 版本号一致性）

---

## 8. 找不到答案时的查询路径

- 行为决策：`docs/00-architecture-overview.md` §9 ADR
- API 契约：`docs/integration-design.md` §2 endpoint inventory + §8 错误码
- DDL 字段：`docs/backend-design.md` §3.x（28 张表）
- saga 时序：`docs/integration-design.md` §4
- 路由 / 表单 / 状态机：`docs/frontend-design.md` §3 / §7
- PRD §X.Y 业务规则：`prd/PRD-v1.0.md`
- 历史决策原因：`reviews/round-1` `reviews/round-2`

---

最后更新：2026-05-11 W0 交付  
任何 W1 agent 接手前请通读本文件 + 自己负责章节对应的 docs / prd 章节。
