# TraceNexBiz / TraceNex Partner

> 渠道分销 SaaS — 在 TraceNex 现有 AI 网关（Fy-api）之上提供二级分销代理能力的独立产品。

**当前状态（2026-05-12）**：W3 (Fix-A/B'/C/D) 全部完成 + W4 Round-2 五方 review **0 CRITICAL** 通过，进入 Round-3 收口阶段。
- 上游 git：`git@github.com:seraph0017/TraceNexBiz.git` (`main` 分支，最新 commit `ec367d2`)
- Round-3 任务清单：`~/Projects/apiGateway/docs/TraceNexBiz-Round3-接手清单-20260512.md`
- Round-2 五方 review 全文：`reviews/code-round-2/01..05*.md`

## 目录结构

```
TraceNexBiz/
├── README.md             本文件
├── apps/
│   ├── partner-api/      Go + Gin + GORM v2 后端（W1a-c + W3 Fix-A/B'/C 已落地）
│   ├── partner-web-storefront/   React 18 公开商城 + ApplyPartner（W3 Fix-D 拆 8 文件）
│   ├── partner-web-customer/     React 18 客户 SaaS（W3 已完全迁移到 @tnbiz/api-client）
│   ├── partner-web-partner/      React 18 渠道商门户（W3 加 TODO header 待增量迁移）
│   └── partner-web-admin/        React 18 内部管理台（含 dual-control force-resolve UI）
├── packages/
│   ├── api-client/       ★ W3 新建：统一 API client + envelope + 静默刷新
│   ├── ui-kit/ i18n/ config/
├── migrations/           017 张 DDL（W3 期间从 011 增至 017）
├── prd/
│   └── PRD-v1.0.md       ★ 定稿（2026-05-09）
├── docs/                 ★ 4 份工程文档 v1.0 定稿（2026-05-11）
│   ├── 00-architecture-overview.md   架构总览 + ADR + 风险登记 + 22 条架构债务 + 16 项合规 hard-gate
│   ├── integration-design.md         partner-api ↔ Fy-api 集成层（OpenAPI / outbox / saga / 跨库 GRANT）
│   ├── backend-design.md             partner-api 后端蓝图（DDL / service / cron / 鉴权 / 幂等 / 加密 / 审计 / 测试）
│   └── frontend-design.md            前端工程蓝图（三站点路由 / 状态管理 / 表单 / 鉴权 / PII / 性能 / 测试 / Phase 切片）
└── reviews/
    ├── round-{1,2}/      PRD 4 方 review（PM / Architect / Security / Compliance）
    ├── dev-round-{1,2}/  开发文档 4 方 review
    ├── fy-api-review/    Fy-api 团队 3 轮 review
    ├── code-round-1/     代码 5 方 review（W2，15 CRITICAL / 37 HIGH）
    └── code-round-2/     代码 5 方 review（W4，**0 CRITICAL / 6 new HIGH**，全部 4 reviewer 升级）
```

## 关键事实

| 项 | 值 |
|---|---|
| 产品代号 | **TraceNex Partner**（仓库名 TraceNexBiz） |
| 后端 | Go 1.25+ / Gin / GORM v2 |
| 前端 | React 18 + Vite + Semi UI + react-hook-form + zod |
| 与 Fy-api 关系 | 独立部署；通过 `/api/internal/*`（HMAC-SHA256 + Idempotency-Key）+ outbox 异步事件集成 |
| HMAC parity | partner-api `fyapi/client.go::sign` 与 Fy-api `middleware/internal_auth.go::BuildCanonical` 字节级一致（`TestSign_FyApiParity` 6 case）|
| 资金清算 | 持牌分账方托管（去二清） |
| 时间线 | 10-12 周完整商业化（Phase 1 / 2A / 2B / 3）|

## W3 工程进展（2026-05-12）

| Fix Bundle | 关闭的 Round-1 CRIT | 关键产出 |
|---|---|---|
| **Fix-A** | CRIT-A1/A2/A3/A4 + SEC-C3/C5 | HMAC parity；5 个 fyapi.Client 方法；7 middleware 真逻辑（auth/csrf/bola/idempotency/webhook_idem/audit/pii_scrubber）；dual-control approver-token；KMS 接口 + LocalKMS(AES-GCM) + AliyunKMS 骨架；BOLA 分析器 + `make lint-bolascope` |
| **Fix-B'** | CRIT-B2/B3/B4/B5/B6 + HIGH-B7 | 5 个 GORM repo + `partner_wallet.balance CHECK >= 0`；Saga step registry + idempotency_record 同事务；Aliyun MNS raw-HTTP publisher/consumer + DLQ；audit_log MySQL hash-chain sealer + KMS 信封加密 |
| **Fix-C** | CRIT-C1/C2/C3 + 6 P1 | `/api/public/biz_setting/footer` endpoint；`cmd/dispatcher-12377/` cron；`cmd/kyc-purge/` cron（PIPL §47）；KMS Encrypt/Decrypt/ScheduleKeyDeletion 真接入；ISV mch_id 反向断言；tax_status 5 枚举；bank_account blind_index（HMAC-SHA256）；consent_text_version 守卫；Redis SETNX 单 leader |
| **Fix-D** | P2 item 17 | `packages/api-client/` 统一；customer 完全迁移（3 app 仍待迁移）；Force-resolve UI 两步（Approve + Submit，不再带 approver_ip）；ApplyPartner 拆 8 文件（每个 ≤200 LOC）；PRD §15.4/15.5 字段对齐 |

W4 Round-2 五方 review 结果（vs Round-1）：

| Reviewer | Round-1 | Round-2 |
|---|---|---|
| Code Quality | PASS-WITH-CONDITIONS | PASS-WITH-CONDITIONS |
| Security | **NEEDS REWORK** | **PASS-WITH-CONDITIONS** ✅ |
| Compliance | **NEEDS_REVISION** | **PASS_WITH_NOTES** ✅ |
| Architect | **FAIL** | **PASS-WITH-CONDITIONS** ✅ |
| Fy-api team | ACCEPT-WITH-CHANGES | ACCEPT-WITH-CHANGES |

Round-1 15 CRITICAL → Round-2 0 CRITICAL（全部关闭）。Round-2 留下 6 个新 HIGH，详见 Round-3 接手清单。

## Round-3 入场指引

接手第一步：

```bash
cd ~/Projects/apiGateway/TraceNexBiz
git pull --ff-only origin main
cat ~/Projects/apiGateway/docs/TraceNexBiz-Round3-接手清单-20260512.md
```

Round-3 必修（按优先级）：

1. **P0 scaffolding-vs-adoption 缺口**（4 项）：把 `saga.RegisterStep` 真接到 saga_allocate/saga_topup；把 `saga.WithIdempotency` 真接到 4 个 service 入口；写 `cmd/outbox-poller/main.go`；填 MNS SINK handler registry
2. **P0 Round-2 显式新 HIGH**（5 项）：MNSConsumer `noopOnUnknown ||= true` 逻辑 bug / pkg/leader cancel 函数泄漏 / dispatcher-12377 硬编码 MemoryRepo / saga.Compensate 失败不 escalate / MNS data_region 标签
3. **P1 Fy-api team 2 项**：RefundCustomer 字段语义错位 / GetUserQuota 丢字段
4. **P1 Round-1 余债 3 项**：另外 3 app api-client 迁移 / admin.bad() 双语 / envelope 三套统一

Round-3 通过后五方再 review → 0 CRITICAL / 0 HIGH → 进 staging。

## Phase 1 启动前必须答的 BLOCK 问题

参见 `prd/PRD-v1.0.md` §13（这些不影响工程进度，但影响上线决策）：
- Q11.1-4 ICP 经营许可证四项前置条件
- Q12 持牌分账方选哪家
- Q13 DPO 由谁担任
- Q14 算法备案文本
- Q16 渠道商合作协议模板

## Phase 2 hard-gate（合规）

参见 `docs/00-architecture-overview.md` §22.3：ICP 证、生成式 AI 备案、持牌分账上线、个税代扣、全电发票、PIA 报告、等保 2.0 二级、DPO 公示、内容安全闭环——任意一项不达标，Phase 2 商业化不能上线。

## 本地 Dev Quickstart

```bash
# 启动依赖
docker compose -f docker-compose.dev.yml up -d   # MySQL 8 / Redis 7 / LocalStack / Mailhog
make migrate-up                                   # 应用 17 个 migration

# 运行 partner-api
make api-dev                                      # go run ./cmd/server，端口 8080
curl http://localhost:8080/healthz

# 运行 4 个前端
pnpm install
pnpm -r --parallel --filter './apps/partner-web-*' dev
# storefront http://localhost:5173 / customer 5174 / partner 5175 / admin 5176

# 测试 + lint
make test                                         # go test ./... + pnpm test
make lint                                         # go vet + golangci-lint + pnpm typecheck
make lint-bolascope                               # BOLA scope 分析器（每条路由必须 WithScope）
```

## 与 Fy-api 的契约

- 调用方：partner-api `apps/partner-api/internal/infra/fyapi/client.go`
- 服务方：Fy-api `middleware/internal_auth.go` + `controller/tnbiz_internal/*.go`
- HMAC headers: `X-Auth-KeyId` / `X-Auth-Timestamp` / `X-Auth-Nonce` / `X-Signature`
- canonical：`METHOD\nPATH\nQUERY\nTS\nNONCE\nSHA256_HEX(body)`（METHOD uppercase，QUERY 按 key 字典序 RFC3986 编码）
- 签名输出：base64
- nonce TTL：5min Redis SETNX；时间窗 ±5min
- 幂等头：`Idempotency-Key`（写接口必传）
- 响应回放头：`X-Tnb-Idempotent-Replay: 1` + `X-Tnb-Idempotent-Source: db`（命中 DB 回放时）

任何 partner-api 端的契约改动必须先确认 Fy-api 端 `middleware/internal_auth.go` 和 `controller/tnbiz_internal/` 同步——`TestSign_FyApiParity` 是这条不变性的活契约。

## 索引

| 想找什么 | 看哪里 |
|---|---|
| 上一阶段（W3 启动前）状态 | `~/Projects/apiGateway/docs/TraceNexBiz-接手状态-20260512.md` |
| **当前活跃** Round-3 任务清单 | `~/Projects/apiGateway/docs/TraceNexBiz-Round3-接手清单-20260512.md` |
| 后端架构总览 | `docs/00-architecture-overview.md` |
| `/api/internal/*` 集成契约 | `docs/integration-design.md` v1.2 |
| 后端蓝图 | `docs/backend-design.md` |
| 前端蓝图 | `docs/frontend-design.md` |
| Round-2 五方 review 全文 | `reviews/code-round-2/01..05*.md` |
| W0 脚手架交付 | `HANDOFF-W0.md` |
| W1g 前端 admin/partner 交付 | `HANDOFF-W1g.md` |
| Fy-api 侧 overlay 清单 | `~/Projects/apiGateway/Fy-api/OVERLAY.md` 的 B-12..B-18 条目 |
| Fy-api 侧 overlay 详细交付 | `~/Projects/apiGateway/Fy-api/OVERLAY-TNBIZ-HANDOFF.md` |
