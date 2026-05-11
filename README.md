# TraceNexBiz / TraceNex Partner

> 渠道分销 SaaS — 在 TraceNex 现有 AI 网关（Fy-api）之上提供二级分销代理能力的独立产品。

**当前状态**：v1.0 PRD 已定稿（2026-05-09），Phase 1 工程开工窗口已开。

**开发文档 v1.0 已定稿**（2026-05-11，4 方 review 两轮通过：PM PASS / Architect PASS-CONDITIONAL / Security PASS-CONDITIONAL-ACCEPT / Compliance PASS_WITH_NOTES）。

## 目录结构

```
TraceNexBiz/
├── README.md           本文件
├── prd/
│   ├── PRD-v0.1.md     初稿
│   ├── PRD-v0.2.md     基于 Round-1 review 重写
│   └── PRD-v1.0.md     ★ 当前定稿
├── reviews/
│   ├── round-1/        v0.1 的四方 review（PM / Architect / Security / Compliance）
│   └── round-2/        v0.2 的四方 review（最终通过）
└── docs/               ★ Phase 1 工程文档 v1.0 定稿（2026-05-11）
    ├── 00-architecture-overview.md   架构总览（C4 拓扑 + ADR + 风险登记 + 附录 A 22 条架构债务 + 附录 B 16 项合规 hard-gate）
    ├── integration-design.md         partner-api ↔ Fy-api 集成层（OpenAPI / outbox / saga / 跨库 GRANT）
    ├── backend-design.md             partner-api 后端蓝图（DDL / service / cron / 鉴权 / 幂等 / 加密 / 审计 / 测试）
    └── frontend-design.md            前端工程蓝图（三站点路由 / 状态管理 / 表单 / 鉴权 / PII / 性能 / 测试 / Phase 切片）
```

> 四份文档定稿行数：overview ~1140 / integration ~1770 / backend ~3690 / frontend ~1940。架构师角色一句话：overview 是宪法、integration 是契约、backend 是后端蓝图、frontend 是前端蓝图。

## 关键事实

| 项 | 值 |
|---|---|
| 产品代号 | **TraceNex Partner**（仓库名 TraceNexBiz） |
| 后端 | Go + Gin + GORM v2 |
| 前端 | React 18 + Vite + Semi UI |
| 与 Fy-api 关系 | 独立部署；通过 `/api/internal/*`（覆盖层）+ 同实例不同 DB 集成 |
| 资金清算 | 持牌分账方托管（去二清） |
| 时间线 | 10-12 周完整商业化（Phase 1 / 2A / 2B / 3）|

## Phase 1 启动前必须答的 BLOCK 问题

参见 `prd/PRD-v1.0.md` §13：
- Q11.1-4 ICP 经营许可证四项前置条件
- Q12 持牌分账方选哪家
- Q13 DPO 由谁担任
- Q14 算法备案文本（详见附录 E）
- Q16 渠道商合作协议模板

## Phase 2 hard-gate（合规）

参见 §22.3：ICP 证、生成式 AI 备案、持牌分账上线、个税代扣、全电发票、PIA 报告、等保 2.0 二级、DPO 公示、内容安全闭环——**任意一项不达标，Phase 2 商业化不能上线**。

---

## W0 Dev Quickstart（脚手架 ready，2026-05-11）

W0 已完成全仓库脚手架：partner-api 可 build/start，4 个 React 站点可启动，
20+ 张表 DDL 已落 migrations，CI / Makefile / docker-compose 就绪。
W1 各 agent 接手细节见 `HANDOFF-W0.md`。

### 启动本地依赖

```bash
docker compose -f docker-compose.dev.yml up -d   # MySQL 8 / Redis 7 / LocalStack / Mailhog
make migrate-up                                   # 应用 11 个 migration（partner_db 全部表）
```

### 运行 partner-api

```bash
make api-dev          # go run ./cmd/server，端口 8080
curl http://localhost:8080/healthz
```

### 运行 4 个前端

```bash
pnpm install
pnpm -r --parallel --filter './apps/partner-web-*' dev
# storefront  http://localhost:5173
# customer    http://localhost:5174
# partner     http://localhost:5175
# admin       http://localhost:5176
```

### 测试

```bash
make test             # go test ./... + pnpm test
make lint             # go vet + golangci-lint + pnpm typecheck
```

### W1 各 agent 入口

| Agent | 工作目录 | 任务 |
|---|---|---|
| W1a 后端基础 | `apps/partner-api/internal/{middleware,saga,outbox,audit,permission}` | JWT/CSRF fail-closed / saga 编排 / outbox 两阶段 / 审计哈希链 |
| W1b 后端业务 1 | `apps/partner-api/internal/{partner,customer,wallet,pricing,revenue}` 域 | 渠道商 / 客户 / 钱包 saga / 定价 / 收益 |
| W1c 后端业务 2 | `apps/partner-api/internal/{auth,kyc,invoice,payment,settlement,notify,ticket,content_safety,pipl_rights}` | 鉴权 / 合规 / 结算 |
| W1d Fy-api 覆盖 | `~/Projects/apiGateway/Fy-api/` (OVERLAY.md B-12..B-18) | C-1..C-7 覆盖层 |
| W1e Storefront | `apps/partner-web-storefront` | M1 公开商城 + ComplianceFooter |
| W1f Customer Web | `apps/partner-web-customer` | M2 / M9 / M13 |
| W1g Partner+Admin Web | `apps/partner-web-partner` + `apps/partner-web-admin` | M3 / M4 / M5 / M6 / M8 / M10 |

详见 `HANDOFF-W0.md`。
