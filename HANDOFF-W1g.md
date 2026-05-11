# HANDOFF-W1g — partner-web-customer + partner-web-admin

**日期**：2026-05-13
**承接**：W2 多角色 review
**前置**：W0 / W1a / W1b / W1c / W1e / W1f
**目录**：
- `apps/partner-web-customer/` 端口 5174（M2 / M9 / M13）
- `apps/partner-web-admin/` 端口 5176（M4 / M5 / M6 / M8 / M10）
- TS strict + React 18 + Semi UI 2.66 + TanStack Query 5 + Zustand 4 + zod 3
- 两站点合计 ~6,022 LOC TS/TSX

---

## 1. 已交付（src/）

### 1.1 partner-web-customer（17 路由 + 3 公共）

| Path | Page | 备注 |
|---|---|---|
| `/dashboard` | Dashboard | 余额 / 用量 / sparkline 30d；30s 刷新 |
| `/balance` | Balance | 当前余额 + 充值入口 |
| `/topup` | Topup | 充值发起 |
| `/topup/:id` | TopupStatusPage | processing→pending_unknown(60s)→escalated(5m)→funded（§7.5） |
| `/api-keys` | ApiKeys | 客户自己 sk-key；一次性显示 + 后续 mask（M3-03 invariant：**永远不展示 partner 维度 sk-key**） |
| `/usage` | Usage | 调用记录 / 模型 / 计费 + CSV 导出（脱敏） |
| `/kyc` | Kyc | 客户实名 + 驳回重审 |
| `/tickets` `/tickets/:id` | Tickets / TicketDetail | 与 platform / partner 工单 + drilldown |
| `/orphan-notice` | OrphanNotice | 场景 I：30 天宽限 + adopt / direct / switch |
| `/pipl-rights` | PiplRights | 场景 Q：导出 / 删除 / 更正 / 可携带性 + 下载 |
| `/switch-partner` | SwitchPartner | 场景 H：申请 + 二次确认 |
| `/invoice` `/invoice/:id` | Invoices / InvoiceDetail | §7.8 申请 / 列表 / 详情 + M8 红冲 |
| `/settings` | Settings | 基础 / 通知；PII staleTime/gcTime=0 |
| `/consent` | Consent | PIPL 同意 dashboard + 撤回 |
| `*` | NotFound | 404 |
| 公共 | `/auth/login` `/auth/forgot` `/auth/reset/:token` | login + §7.9 password reset 双因子链路 |

- RoleGuard：customer **不强制 MFA**（弱化版）；orphan 状态强制 redirect → `/orphan-notice`
- ComplianceFooter（§11.5）：备案号 / ICP / 算法备案
- OrphanBanner：终止后顶部黄底提示

### 1.2 partner-web-admin（22 路由 + 2 公共）

| Path | Page | 备注 |
|---|---|---|
| `/partners` `/partners/:id` `/partners/new` | Partners / PartnerDetail / PartnerNew | CRUD + **终止合作** → 自动开 30d 客户宽限（场景 I） |
| `/kyc` `/kyc/:id` | KycList / KycDetail | 审核 + 通过 / 驳回 + **三方核验调用** |
| `/wallet` `/wallet/topup` | Wallet / WalletTopup | partner 余额表 + 调账（reason 必填进 audit） |
| `/settlements` `/settlements/:id` | Settlements / SettlementDetail | 锁定 → 分账下发 → 回执对账 + 个税代扣 banner |
| `/refunds` | Refunds | 发起 + 审核 + saga 触发警示 |
| `/red-flush` | RedFlush | 红冲发票管理 + reason_code |
| `/audit-log` | AuditLog | 列表 + filter + **verify 哈希链**（ADR-006）|
| `/content-safety/reports` `/:id` | ContentSafetyReports / Detail | **12377**（COMP-CRIT-2）：列表 / 详情 / 重试 / 一键派发 |
| `/pia` | Pia | 报告生成 + 下载 |
| `/pipl-complaints` `/:id` | PiplComplaints / Detail | 投诉处理 + timeline + decision |
| `/system/security` | SecuritySettings | IP 白名单 / step-up TTL / 屏幕水印开关 / session 最长 |
| `/system/biz-settings` | BizSettings | 9 keys 编辑 / 模型白名单 / cron |
| `/saga/force-resolve` | SagaForceResolve | **dual-control UI**（PRD-PATCH-1）：approver_token / approver_ip / outcome / reason + 升级候选列表 + cooldown 字段 |
| `/staff` `/staff/:id` | StaffList / StaffDetail | CRUD + 5 角色（super/risk/finance/cs/kyc）+ disable + step-up MFA banner |
| `*` | NotFound | 404 |
| 公共 | `/auth/login` `/auth/mfa-enroll` | step-up MFA enroll |

- RoleGuard：staff JWT + **IP 白名单 ban 横幅** + MFA enroll 强制 + `<PermissionGuard verb=...>`（22 verb / 6 角色矩阵入口已留）
- **Watermark 屏幕水印**（compliance §9.4）：username + IP + 5min refresh
- 独立深色顶栏（ADR-F1）：#0f172a 背景 + #fbbf24 active

### 1.3 共享层（每站点物理一份，与 W1f 决策一致 — W2 review 时再搬 packages/ui-kit）

| 文件 | 来源 | 改动 |
|---|---|---|
| `api/{client,types,error-mapping}.ts` | W1f 复制 | 0 改动；append-only 错误码 |
| `api/customer.ts` `api/admin.ts` | W1g 新增 | 按 W1c openapi/admin.yaml 类型化；customer 30 endpoints / admin 35 endpoints |
| `components/{Field,MoneyDisplay,Page,Stepper,Sparkline,ErrorBoundary}.tsx` | W1f 复制 | 0 改动 |
| `components/RoleGuard.tsx` | 重写 | customer：orphan redirect；admin：MFA enroll + IP guard + PermissionGuard |
| `components/NavBar.tsx` | 重写 | customer 浅色 / admin 深色独立栏 |
| `components/{Layout, Watermark, OrphanBanner, ComplianceFooter}.tsx` | W1g 新增 | admin Watermark / customer OrphanBanner+ComplianceFooter |
| `hooks/{useApiToast,useThrottledSubmit}.ts` | W1f 复制 | 0 改动 |
| `hooks/useAuth.ts` | 重写 | namespace customer/admin |
| `lib/{pii,zodResolver}.ts` | W1f 复制 | 0 改动 |
| `i18n/index.ts` + `locales/{zh-CN,en-US}.json` | W1f 复制改 namespace | customer ~150 / admin ~140 文案条目 |
| `stores/authStore.ts` | 重写 | namespace `tnbiz.customer.auth` / `tnbiz.admin.auth` |

---

## 2. 验收

| 验收 | customer | admin | 备注 |
|---|---|---|---|
| `tsc --noEmit` | ✅ 0 错 | ✅ 0 错 | strict + noUncheckedIndexedAccess |
| `vite build` | ✅ | ✅ | customer gz: react 53 + semi 201 + index 33 + i18n 16 + form 14 + query 9 + css 64 / admin: 类似 |
| `vitest run` | ✅ 10/10 | ✅ 10/10 | pii×6 + zodResolver×2 + error-mapping×2 |
| 17 / 22 路由可达 | ✅ | ✅ | 每个 Page 有 Loading / Empty / Error 兜底 |
| e2e ≥ 3 条 | ✅ topup / ticket / pipl | ✅ kyc / saga force-resolve / 12377 dispatch | spec 写好；`@playwright/test` 未装跟 W1f 一致，W2 接 CI |
| 文件 ≤ 400 LOC | ✅ 最长 customer.ts 329 | ✅ 最长 admin.ts 388 | |
| TS strict / 不写 emoji | ✅ | ✅ | 仅 ASCII；无 console.log |
| Semi UI + zod + immer 风格 | ✅ | ✅ | 即时 immutable spread / set；Switch / Select / Modal 走 Semi |

---

## 3. 关键 invariants 已落地

1. **PII 全 mask**：customer/admin 任何字段均显示 `*_masked`（来自 server）；前端不主动还原。`Settings` 走 `staleTime/gcTime=0` 防 PII 缓存。
2. **API key 不展示 partner 维度 sk-key（PRD §M3-03）**：customer `/api-keys` 页头部 banner 明示；只显示 customer 自己的 key（`sk-cu-*`），create 时一次性 raw_key + 后续 mask。
3. **充值 escalated UX（场景 D / §7.5）**：`/topup/:id` 的 phase 机时间线 60s / 5min 与 backend §5.7 invariant 对齐。
4. **场景 I 30 天宽限**：customer `OrphanBanner` + `OrphanNotice` 选择三选项；admin `Partners.terminate` 时强 banner 提示自动开宽限。
5. **场景 H 切换渠道商**：customer `SwitchPartner` 提交 → approve → 二次确认；接 admin `customer.transfer` verb（W1c 实现 server-side approval）。
6. **场景 Q PIPL §44-§47**：customer 4 类请求（export / delete / rectify / portability）+ download_url；admin `PiplComplaints` 处理 + decision 链条。
7. **M8 红冲**：customer `InvoiceDetail` apply red-flush（仅 issued + blue 时可用）；admin `RedFlush` 审核 + reason_code。
8. **dual-control force-resolve（PRD-PATCH-1）**：admin `SagaForceResolve` UI 字段全到位（approver_token / approver_ip / outcome / reason / cooldown）；server-side 检查 4 约束（不同人 / 不同 /24 / 一次性 token / 30min cooldown）。前端不重复校验，错误走 `errors.force_resolve.*` 文案。
9. **审计哈希链**：admin `AuditLog.verify` 调 `/api/admin/audit-log/verify`，结果 toast `audit.verify_ok` / `audit.verify_broken`；与 ADR-006 CLI 工具同源。
10. **12377 一键派发（COMP-CRIT-2）**：admin `ContentSafetyReports.dispatchAll` + 单条 retry。
11. **admin 屏幕水印（compliance §9.4）**：每屏对角矩阵渲染 `username · id · ts(5min refresh)`，opacity 0.06 不影响视觉。
12. **admin step-up MFA / IP 白名单**：RoleGuard 双层；`StaffList` 带 banner；server-side 每个敏感 verb 检查 step-up 时间戳。
13. **admin 独立 SPA + 深色顶栏（ADR-F1）**：完全不嵌入 Fy-api 既有 admin。
14. **Idempotency-Key + X-Csrf**：复用 W1f client.ts；每次写操作 client interceptor 自动生成 UUIDv4。
15. **silent refresh + tnbiz:auth:expired 事件**：customer/admin 同 partner 模式；refresh 失败自动跳 `/auth/login`。

---

## 4. 与后端契约对齐（前端先行假定 / 空态兜底）

下列 endpoint 在 W1c 还未全实现，前端 query fail = 显示 Empty/Spin 不让首屏崩：

| Endpoint | 站点 | 后端状态 | 前端兜底 |
|---|---|---|---|
| `/api/customer/dashboard` | customer | 待 W1c 聚合 | Skeleton / Empty |
| `/api/customer/balance` `/topup*` | customer | W1b saga_topup 已建 | OK |
| `/api/customer/api-keys*` | customer | 待 W1c | mutation OK，UI 走通 |
| `/api/customer/usage` | customer | 待 W1c | Empty 表 |
| `/api/customer/kyc/*` | customer | W1a kyc OK | OK |
| `/api/customer/tickets/*` | customer | W1c ticket OK | OK |
| `/api/customer/orphan*` `/switch-partner*` | customer | 待 W1c（场景 H/I） | mutation OK |
| `/api/customer/invoice/*` `/red-flush-apply` | customer | 待 W1c（M8） | OK |
| `/api/customer/settings` `/consent*` `/pipl*` | customer | 待 W1c（M13） | OK |
| `/api/customer/me` | customer | 待 W1c (CustomerMe schema) | RoleGuard 不阻 demo |
| `/api/admin/*` | admin | W1c admin.yaml 写好 5 个；剩余按 schema 假定 | Empty/Spin |
| `/api/admin/saga/escalated` | admin | 待 W1c | Empty 列表 |
| `/api/admin/audit-log/verify` | admin | ADR-006 CLI 已有，HTTP 待 W1c | toast 假定 OK |
| `/api/admin/security` `/biz-settings` | admin | 待 W1c | OK |
| `/api/admin/staff/*` | admin | 待 W1c | OK |

字段假定都写在 `src/api/customer.ts` / `src/api/admin.ts` 的 export interface 里；后端 schema 直接复制即可 wire-compatible。

---

## 5. 担心 review 的点

1. **bundle 偏大**：customer 主 chunk gz：semi 201 KB + react 53 + index 33 + i18n 16 + form 14 + query 9 ≈ 326 KB / admin 同量级。**与 W1f 同样的 risk**：semi-ui 全量 import；W2 改路由级 lazy chunk（`React.lazy(() => import('@/pages/...'))`）一笔 PR。
2. **没有 PermissionGuard 级 wrap 到具体按钮**：当前 RoleGuard 只挡整页，22 verb × 6 角色矩阵需要 W2 在每个写按钮上 `<PermissionGuard verb='...'>`。已建组件，未全量 wire；server-side 仍是真守护。
3. **MFA enroll 是占位**：admin `/auth/mfa-enroll` 直接 redirect；WebAuthn challenge 流程待 backend MFA endpoint 接 W1c 后实现。
4. **dual-control "前端校验"**：`SagaForceResolve` 没在前端预校验"不同人 / 不同 /24"；故意——server 才是 source of truth，前端只 surface error 文案。
5. **Watermark CSS-only**：`opacity 0.06` 黑色文字；如设计师要"反检测水印"（混入图像 noise）需重写。当前是合规可见性方案，符合 §9.4 demo 阶段需求。
6. **Settings 表单**：customer/admin 都用 `useEffect` 同步 server 数据 → 本地 state（immutable spread）。不上 react-hook-form 嵌套表单，与 W1f 决策一致。
7. **CSV 导出仅当前页**：customer `/usage` 当前页客户端导出（脱敏）；全量流式走 backend M4-* 任务（W1c 未实现）。
8. **i18n en-US 为 placeholder**：复制 zh-CN 一份占位；W2 翻译。
9. **e2e 没真跑**：`@playwright/test` 未装，spec 写好但未在 CI 验。W2 装 + `npx playwright install chromium` 即可跑。
10. **没用 packages/api-client / ui-kit**：与 W1e/f 同决策。三 app 重复维护 client.ts / types.ts 是已知 cost；W2 review 后搬 `Field` `Stepper` `MoneyDisplay` `Sparkline` `client.ts` 5 件到 ui-kit/api-client。
11. **admin Login 不查 IP 白名单**：login 之前 IP 不阻；登录后 RoleGuard 看 `me.ip_allowed`。这与 server-side IP allowlist middleware 互补，**不是替代**。

---

## 6. 联调 cheatsheet

```bash
# customer
cd apps/partner-web-customer
./node_modules/.bin/vite --port 5174    # http://localhost:5174
./node_modules/.bin/tsc --noEmit
./node_modules/.bin/vitest run
./node_modules/.bin/vite build

# admin
cd apps/partner-web-admin
./node_modules/.bin/vite --port 5176    # http://localhost:5176
# 同上
```

dev proxy `/api/*` → `http://localhost:8080`（partner-api dev）；与 W1a §6 `X-Dev-Actor-*` header bypass 配合。

— W1g agent，2026-05-13
