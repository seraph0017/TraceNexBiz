# HANDOFF-W1f — partner-web-partner 渠道商后台

**日期**：2026-05-13
**承接**：W1g（customer + admin 前端）/ W2（多角色 review）
**前置**：HANDOFF-W0 / HANDOFF-W1a / HANDOFF-W1b / HANDOFF-W1c / HANDOFF-W1e（共享模式来源）
**目录**：`apps/partner-web-partner/`，端口 5175，TS strict + React 18 + Semi UI 2.66 + TanStack Query 5 + Zustand 4 + zod 3.

---

## 1. 已交付（src/ 共 ~3,850 LOC TS/TSX）

```
src/
├── api/                       本地 API 客户端（与 packages/api-client 同形）
│   ├── client.ts              axios + interceptors（X-Csrf / Idempotency-Key / X-Oneapi-Request-Id；
│   │                          401 silent refresh / 失败派发 tnbiz:auth:expired 事件）
│   ├── error-mapping.ts       25 条 BIZ_* 错误码 → toast i18n key（含 W1f 新增 11 条）
│   ├── partner.ts             ~30 endpoints typed wrapper
│   │                          （auth / me / dashboard / wallet / wallet/logs / wallet/holds /
│   │                           customers / customer/usage / allocate(saga) / saga/:id /
│   │                           invitation×3 / pricing / statements×2 / invoice/apply /
│   │                           disputes×3 / tickets×4 / settings×2 / kyc×2 / topup×2 / mfa×2）
│   └── types.ts               ApiEnvelope / ApiError / ApiException / PageMeta
├── components/
│   ├── ErrorBoundary.tsx      顶层 fallback
│   ├── Field.tsx              受控 form 字段 wrapper（不进 Form context；Semi Form.* value 由 form 管理，本项目用平面 state）
│   ├── KycBanner.tsx          rejected / frozen 横幅 + 重审入口
│   ├── Layout.tsx             NavBar + KycBanner + main + outlet
│   ├── MoneyDisplay.tsx       分→元，千分位，tabular-nums
│   ├── NavBar.tsx             11 个 NavLink + 用户菜单（current / 全设备登出）+ 语言切换
│   ├── Page.tsx               标题 / 操作区 / 内容容器
│   ├── RoleGuard.tsx          me 校验 + KYC pass→MFA 强制 redirect
│   ├── Sparkline.tsx          自绘 SVG 趋势图（不引 ECharts/Recharts，省 ~250 KB）
│   └── Stepper.tsx            saga 三阶段步骤指示器（aria-current）
├── hooks/
│   ├── useApiToast.ts         ApiException → Semi Toast（按 severity）
│   ├── useAuth.ts             me query + login/logout + tnbiz:auth:expired 监听
│   └── useThrottledSubmit.ts  cooldown + in-flight lock（与 storefront 一致）
├── i18n/
│   ├── index.ts               i18next + react-i18next，namespace=partner，默认 zh-CN
│   └── locales/{zh-CN,en-US}.json  ~150 条文案（含 errors / nav / 11 个业务模块 / validation）
├── lib/
│   ├── pii.ts                 maskPhone / maskEmail / maskIdCard / maskBankAccount / fenToYuan
│   └── zodResolver.ts         平面 react-hook-form resolver（沿用 storefront）
├── pages/                     13 业务页 + Login/Mfa/NotFound
│   ├── Login.tsx              httpOnly cookie；不存 token；otp 可选
│   ├── Mfa.tsx                WebAuthn / TOTP 双 tab；KYC pass 后 RoleGuard 强制
│   ├── Dashboard.tsx          余额 / 月 gross/cost/net / 客户 / KYC 待续审 / sparkline；30s 自刷
│   ├── Customers.tsx          列表 + 筛选 + 分页（v5 placeholderData） + CSV 客户端导出（脱敏）
│   ├── NewCustomer.tsx        生成邀请码 + 复制
│   ├── CustomerDetail.tsx     4 tab：基础 / 配额 / 用量 / 工单
│   ├── Allocate.tsx           saga 三阶段 UI；客户端状态机 idle→submitting→running→
│   │                          (succeeded | failed_user | failed_system | pending_unknown(60s) | escalated(5min))；
│   │                          按 [1,2,4,8,16,30]s 退避 polling /partner/saga/:id
│   ├── Invitations.tsx        列表 + 生成 (perm/one_time/limited) + QR + revoke
│   ├── Pricing.tsx            模型 markup（bps）per-partner override
│   ├── Wallet.tsx             余额三色卡 + 流水分页 + holds tab
│   ├── WalletTopup.tsx        跳持牌方 + processing→pending_unknown(60s)→escalated(5m)→funded（§7.5 v0.2.1 PM-HIGH-6）
│   ├── Statements.tsx         列表 + 详情 line items + applyInvoice
│   ├── Disputes.tsx           列表 + 提交 (fy_account / tn_account 场景 K) + 详情
│   ├── Tickets.tsx            列表 + 提交 + drilldown 消息流 + reply
│   ├── Settings.tsx           4 tab；银行卡仅 mask + KMS warning；staleTime/gcTime=0 不缓存 PII
│   ├── Kyc.tsx                状态卡 + 驳回/冻结 banner + 重审入口
│   └── NotFound.tsx
├── stores/
│   ├── allocateStore.ts       saga phase 机；不持久化（saga 由 server canonical 推进）
│   └── authStore.ts           me 持久化（仅 *_masked 字段；token 在 httpOnly cookie）
├── App.tsx                    13 业务 routes + login + mfa + 404；RoleGuard + Layout 嵌套
└── main.tsx                   bootstrap：initI18n → render（QueryClient staleTime=60s, retry=1, mutations retry=0）
```

### 1.1 路由（per frontend §3.2 partner，13 个）

| Path | Page | 备注 |
|---|---|---|
| `/dashboard` | Dashboard | 实时余额 + 月分润 + 趋势 |
| `/customers` | Customers | 列表 + 筛选 + 导出（脱敏 CSV）|
| `/customers/new` | NewCustomer | 生成邀请码 |
| `/customers/:id` | CustomerDetail | 4 tab |
| `/allocate` | Allocate | saga 三阶段 + 状态机 |
| `/invitations` | Invitations | CRUD + QR |
| `/pricing` | Pricing | markup bps |
| `/wallet` | Wallet | 余额 + logs + holds |
| `/wallet/topup` | WalletTopup | escalated UX |
| `/statements` | Statements | 列表 |
| `/statements/:id` | StatementDetail | line items + applyInvoice |
| `/disputes` | Disputes | 提交 + 列表 |
| `/disputes/:id` | DisputeDetail | 详情 |
| `/tickets` | Tickets | 提交 + 列表 |
| `/tickets/:id` | TicketDetail | drilldown |
| `/settings` | Settings | 基础/联系人/银行/通知 |
| `/kyc` | Kyc | 状态 + 重审 |
| 公共 | `/auth/login` `/auth/mfa` `*` | unguarded |

> 共 17 个 route 元素，覆盖 frontend §3.2 13 业务 + 4 公共。

### 1.2 测试

- **vitest**：5 个 spec / **12 个 test 全绿**（pii×6、error-mapping×2、zodResolver×2、allocateStore×1、Stepper×1）
- **Playwright**：3 个 e2e 文件（login / allocate / ticket），mock 路由覆盖 happy path；**未跑** —— `@playwright/test` 未装。W2 接 CI 时 `npm i -D @playwright/test && npx playwright install chromium`

### 1.3 验收

| 验收 | 状态 | 备注 |
|---|---|---|
| `tsc --noEmit` | ✅ 0 错 0 警 |  |
| `vite build` | ✅ | gz 总 ~340 KB（react 53 + semi 147 + index 35 + i18n 16 + form 13 + query 9 + css 63） |
| `vitest run` | ✅ 12/12 PASS | 0.6s |
| 13 路由全可达 | ✅ | + 4 公共 |
| e2e ≥ 3 条 | ✅ | login / allocate / ticket |
| 文件 ≤ 400 LOC | ✅ | 最长 partner.ts 386 |

`semi` chunk gz 147 KB > 250 KB 单 route 预算 —— 需要 W2 优化（动态 import 拆 Modal/Tabs/Table 到各路由 lazy chunk）。当前 initial = react+index+css+semi 主体 ≈ 300 KB gz，仍超预算 50 KB。**这是已知 risk，写在本文档 §4**。

---

## 2. 给 W1g 复用的模式（W1g 必读）

W1g 接 customer + admin 时，`apps/partner-web-customer/` 与 `apps/partner-web-admin/` 应**直接复制** W1f 的下列文件作为起点（与 storefront 保持的 "契约一致 / 物理两份" 原则一致；W2 review 时再统一搬到 `packages/ui-kit`）：

### 2.1 直接复制（API 层）
- `src/api/client.ts` —— axios + interceptors + silent refresh
- `src/api/types.ts` —— envelope / ApiException / PageMeta
- `src/api/error-mapping.ts` —— **复制后只 append 新错误码，不改已有 key**（防 i18n 漂移）
- 仿写 `src/api/customer.ts` / `src/api/admin.ts`，按 W1c openapi/admin.yaml 类型化

### 2.2 直接复制（组件 / hooks / lib）
- `components/Field.tsx` —— 受控表单 wrapper（**重要**：Semi UI Form.* 用 form context，value/onChange 不能 props 直传；本项目所有表单走平面 state + 这个 Field）
- `components/{ErrorBoundary,MoneyDisplay,Page,Stepper,Sparkline}.tsx`
- `hooks/{useApiToast,useThrottledSubmit}.ts`
- `lib/{pii,zodResolver}.ts`
- `i18n/index.ts`（改 namespace 为 customer / admin；保留 zh-CN/en-US 双语骨架）
- `stores/authStore.ts`（namespace 改 `tnbiz.customer.auth` / `tnbiz.admin.auth`）

### 2.3 必改（按角色调整）
- `RoleGuard.tsx`：customer 不强制 MFA，admin 走 step-up MFA + WebAuthn 必须；admin 还要叠 `<PermissionGuard verb=...>`（v1.2 §3.4 22 verb × 6 角色矩阵）
- `NavBar.tsx`：admin 站点要顶部水印（Compliance §9.4 屏幕水印）
- `vite.config.ts`：customer 端口 5174 / admin 端口 5176

### 2.4 共享 packages 当前状态
仍未提到 `packages/ui-kit` —— 同 W1e 的判断：跨 workspace import 在 W0 lockfile + 三 app 并行的当下风险大于收益。W2 review 后建议搬运三件：`Field` / `Stepper` / `MoneyDisplay`。**当前 W1f 写法已是 ready 的**（无非 framework 依赖、无业务耦合）。

---

## 3. 与后端契约对齐（W1a/W1b/W1c → W1f 假定）

W1f 全部按 HANDOFF-W1a/b/c §4 字段编写 typed wrapper。下列 **W1c 还没产出**的 endpoints 走前端先行假定 + 空态兜底（query fail = 显示 Empty/Spin，不让首屏崩）：

| Endpoint | 后端状态 | 前端兜底 |
|---|---|---|
| GET `/api/partner/dashboard` | 待 W1c 聚合 | Skeleton 加载；query fail 为空 |
| GET `/api/partner/wallet/holds` | W1a §4.4 已 list | OK |
| GET `/api/partner/customers/:id/usage` | 待 W1c | Empty |
| GET `/api/partner/saga/:id` | W1b saga 接口 | OK，state 字段已对齐 §1.3 |
| POST `/api/partner/wallet/topup` + GET 同 id | W1b saga_topup | OK |
| GET `/api/partner/pricing` / PUT `/:model_id` | 待 W1c（§3.2 Phase 1 单层） | 编辑可见、save 走 mutation |
| GET `/api/partner/statements` `/:id` | 待 W1c | Empty 兜底 |
| POST `/api/partner/invoice/apply` | 待 W1c | mutation OK |
| POST `/api/partner/disputes` 等 | W1b/W1c dispute service 已建 | mutation OK |
| `/api/partner/tickets/*` | W1c ticket OK | OK |
| `/api/partner/settings` GET/PUT | 待 W1c | Empty + mask 字段假定 |
| `/api/partner/kyc/status` | W1a kyc OK | OK |
| `/api/partner/mfa/{enroll,verify}` | 待 W1c（v0.2 SEC CRIT-7） | UI 走通流程，verify=true 即 redirect |

**字段假定**：dashboard 字段（balance/available/held_total/monthly_*/customers_*/trend_30d）需要 W1c 在 partner-api `internal/handler/partner_dashboard.go` 实现。建议 schema 直接复制 `src/api/partner.ts::DashboardSummary`。

---

## 4. 担心 review 的点

1. **bundle 超预算 50 KB**：semi-ui 全量 import 在 main chunk，gz 147 KB 已挤掉 250 KB / route 预算。修复：把 `Tabs/Table/Modal/Form/Avatar/Banner/Descriptions` 改成路由级 lazy import（`React.lazy(() => import('@/pages/Tickets'))` + `<Suspense>`）。一笔 PR 工作量。
2. **没用 `packages/api-client` / `packages/ui-kit`**：与 W1e 决策一致；W2 review 时统一搬。当前重复代码（client.ts/types.ts/error-mapping.ts）三份独立维护是已知 cost。
3. **Semi UI Form.* 不可控**：踩了一次坑（Form.Input 等 value/onChange 由 form context 管，props 直传 TS 报错）。本项目用平面 state + `<Field>` wrapper + 原始 `<Input>/<InputNumber>/<Select>/<TextArea>`；副作用是 zod 校验需要手动调用（已在 Allocate / Login 用 zod safeParse + Toast.warning 兜底）。后续如需大型嵌套表单（admin saga force-resolve）可考虑装 `@hookform/resolvers/zod` + 用 react-hook-form 标准接口。
4. **Sparkline 自绘**：30 天点用纯 SVG 折线 + 渐变填充；设计师要求重构成 Recharts 时改 100 行代码即可。理由是 ECharts 拉 ~400 KB / Recharts ~70 KB gz，对 dashboard 一张图不划算。
5. **e2e 没真跑**：未装 `@playwright/test`；spec 写好但 selector 没在 CI 验证（label 文案对了，可能 Semi UI 的 Modal 渲染要 portal 选择器）。W2 接 CI 时验。
6. **充值 escalated 状态**：当前 phase machine 的 `processing→pending_unknown→escalated` 时间线写死在前端（60s / 5min）；与 backend §5.7 invariant 时间一致。如果后端调，前端要同步改。
7. **Allocate 的 "失败回到 form"**：`failed_user` 时直接复用上次输入的 customerId/amount/note；没有自动清空 idempotency-key —— 但 client interceptor 每次 POST 都生成新 UUID，所以不会复用 key（**安全**）。已通过 axios 默认 header 设计验证。
8. **CSV 导出仅当前页**：客户端导出，全量导出走后端流式（M4-* 任务，W1c 还未实现）。当前导出已脱敏（只 export `email_masked`，不含明文）。
9. **复制邀请码 / 工单附件上传**：用 `navigator.clipboard.writeText` —— Safari 在非 https 下会失败，用户体验弱化但不阻断。附件上传 UI 占位（`tickets.attach` i18n 已建），实际 OSS direct upload 复用 storefront `KycUploader` 模式，W2 review 阶段补。
10. **认证 flow 与 backend §7.9 password reset**：partner-api 已有 `/public/auth/password/forgot|reset`（W1a §1.2）；前端目前只接了 login/logout/refresh —— forgot/reset 入口在登录页 todo 链接。W1g 接 customer 时一并实现页面（`/auth/forgot` `/auth/reset/:token`）。

---

## 5. 联调 cheatsheet

```bash
cd apps/partner-web-partner
./node_modules/.bin/vite --port 5175
# 浏览器：http://localhost:5175
# 默认 redirect /dashboard，未登录 → /auth/login
# proxy: /api/* → http://localhost:8080（partner-api dev）

# 单元测试 / 类型检查 / 构建
./node_modules/.bin/tsc --noEmit
./node_modules/.bin/vitest run
./node_modules/.bin/vite build
```

dev 模式 partner-api 走 `X-Dev-Actor-*` header bypass（W1a §6）；本前端**不发**这个 header（cookie 优先），所以纯前端 dev 时 login 必须先 mock 或后端 dev 模式接受空 cookie + 任意 handle。建议 W1c JWT middleware 接好后再做端到端联调。

---

— W1f agent，2026-05-13
