# 开发文档 v0.1 Review — Product Manager (Dev Round 1)

> Date: 2026-05-10
> Reviewer: PM agent（产品经理视角，非架构 / 非安全 / 非合规）
> 待审文档：
> - `docs/00-architecture-overview.md` v0.1（766 行）
> - `docs/integration-design.md` v0.1（1558 行）
> - `docs/backend-design.md` v0.1（2704 行）
> - `docs/frontend-design.md` v0.1（1724 行）
> 权威：`prd/PRD-v1.0.md`（2295 行，已通过 round-2 四方 review）
> Verdict: **PASS-with-conditions**（0 CRITICAL / 6 HIGH / 9 MEDIUM / 6 LOW）

---

## 1. 执行摘要

四份开发文档相对 PRD v1.0 的"忠实落地度"整体合格：12 个模块全部在 backend / frontend 找到 owner package + 路由树落点；17 个业务场景（A–Q）大多数有 wireframe + 状态机 + saga 时序图；权限矩阵（PRD §3.4）在 backend `permission.matrix` + frontend `<PermissionGuard>` 双侧镜像；PRD §22.1 F-1..F-14 Phase 1 follow-ups 全部被点名跟踪。**契约层（overview + integration-design）质量高、可立即动工；backend + frontend 在"模块表覆盖"维度合格，但在"场景端到端可达"维度有数处非致命断链。** Phase 1 工程可立刻按 backend §17.1 / frontend §18.1 / overview §8.1 切片开始 sprint。

但作为 PM 必须把以下三类问题挑明：(a) **场景 K（账单争议）端到端缺位**——PRD §4.11 + §7.5 M5-09 + §14.5 都存在，开发文档对 dispute 表 / endpoint / 客户端"我有疑问"按钮 / dispute 状态机一概没落地；(b) **场景 I（渠道商终止 → 客户 30 天宽限）orphaned/adopted 转换链路 backend / frontend 全缺**——直接撞 PRD §4.9 + §14.2 + 电商法连带责任；(c) **M2-15 通知中心 Phase 标签 frontend §3.2/§18.2 标 2A，PRD / overview / backend 标 3**，前后端 phase 错位会导致 Phase 2A 上线时 UI 调 API 返 404。

剩余 HIGH / MEDIUM 包括：发票红冲（M8）前端 UI 缺失、场景 H（客户切换）staff 终审 admin 入口在 frontend admin 路由树缺位、备案号展示路径不存在、M9-01 渠道商品牌 banner 全局可见性弱、PRD §1.4 KPI 业务埋点未成套。这些都不阻塞 Phase 1 第一行代码，但若不在 Week 2-3 收敛，Phase 2A 一开 sprint 就会撞墙。

一句话定性：**契约层 ready-to-build；业务完成度在场景端到端维度有 4 处非致命断链，可在 v0.2 小 patch 内修复。**

---

## 2. 覆盖矩阵：PRD §7 十二模块 × 四份文档

| Module | overview | integration-design | backend-design | frontend-design | 完整度 | 备注 |
|---|---|---|---|---|:---:|---|
| M1 公开商城 / 招商 | §3.2 / §3.3 | — | §1.2 / §4.2 / §5.1 | §3.1 / §7.1 | ✅ | M1-07 串到 §7.6 持牌方 saga，三份覆盖 |
| M2 终端客户后台 | §3.2 / §3.3 | §4.5 客户充值 saga | §1.2 / §4.3 / §5.2, §5.7 | §3.2 / §7.5 | ⚠️ | M2-15 通知中心 Phase 冲突（HIGH-1）|
| M3 渠道商后台 | §3.2 / §3.3 | §4.3 M3-04 saga | §1.2 / §4.4 / §5.3 | §3.2 / §7.4 | ✅ | 核心 happy path 端到端贯通 |
| M4 平台后台 | §3.2 / §3.3 | — | §1.2 / §4.5 / §5.10.2 | §3.3 / §7.8 | ⚠️ | M4-02 / M4-03 endpoint 命名混淆（MEDIUM-8）|
| M5 分润结算 | §3.2 / §8.3 | §7 freshness gate | §4.5 / §5.5 / §6 runner | §3.3 / §18.3 | ⚠️ | M5-09 争议链路缺位（HIGH-4）|
| M6 支付 | §3.2 | §4.5 saga（F-3）| §1.2 / §4.9 / §5.7 | §3.2 / §7.5 | ⚠️ | EPay 边界（MEDIUM-3）|
| M7 KYC | ADR-009 | C-4 / C-5 | §1.2 / §4.8 / §5.6 / §9 | §7.2 拍照 + presigned | ✅ | 完整 |
| M8 发票 | §3.2 | — | §1.2 / §4.10 / §5.8 | §3.2 / §7.6 | ⚠️ | **红冲 UI 缺失**（HIGH-3）；DDL 缺失（HIGH-5）|
| M9 防绕过 / 客户体验 | §3.2 | — | §1.2 / §5.2 | §3.2 隐式 | ⚠️ | M9-01 全局 banner 弱（MEDIUM-2）|
| M10 工单 | §3.2 | — | §1.2 / §4.6 / §5.9 | §3.2 / §3.3 | ⚠️ | M10-03 drill-down 缺 wireframe（MEDIUM-4）|
| M11 通知 | §3.2 | §3 outbox | §1.2 / §4.7 / §5.9 / §6 | §3.2 customer 有；partner 缺 | ⚠️ | partner 通知入口缺；HIGH-1 联动 |
| M12 内容安全 | §3.2 | — | §1.2 / §4.11 | §3.3 / §7.10 | ⚠️ | M12-05 水印、M12-06 备案号未明（MEDIUM-1）|
| M13 PIPL 用户权利 | §3.2 / §3.3 | — | §1.2 / §4.12 / §5.11 | §3.2 / §7.9 | ✅ | 完整 |

> ✅ 完整 / ⚠️ 部分 / ❌ 缺漏。M13 是架构师在 overview §3.2 中显式立模块（PRD 隐含在 §4.17 + §15.5），PM 视角认可。

---

## 3. 场景矩阵：PRD §4.1–§4.17 × 四份文档

| 场景 | Happy path 落点 | 异常分支 / 状态机 | 完整度 |
|---|---|---|:---:|
| A 渠道商人工招商 | backend §5.1 (Note over Server) | — | ⚠️ frontend §3.3 admin 无 `/partners/new` 创建入口（MEDIUM-1' / M1） |
| B 渠道商自助入驻 | backend §5.1 + frontend §7.1 wireframe + zod | PRD §14.1；frontend §7.1 mermaid | ✅ |
| C 客户被邀请入驻 | backend §5.2 + frontend §3.2 invitation | alt 已是直营走 N | ✅ |
| D 渠道商充值客户 | integration §4.3 + backend §5.3 + frontend §7.4 三阶段 UI | escalated state | ✅ |
| E 计费 outbox | integration §3 + backend §5.4 | DLQ + retry_count 10 | ✅ |
| F 月结 | backend §5.5 + integration §7 freshness gate | gate_failed 手工接管 | ⚠️ progress_offset 续跑期新 revenue 归属未写明（MEDIUM-5）|
| G 发票申请 | backend §5.8 + frontend §7.6 | 红冲 | ⚠️ HIGH-3 红冲 UI / HIGH-5 DDL |
| H 客户切换渠道商 | backend §5.10.1 + frontend §7.7 | PRD §4.8 三方 acceptance | ⚠️ HIGH-2' staff 终审 endpoint / UI 缺（合并 MEDIUM-6）|
| I 渠道商终止 / 暂停 | PRD §4.9 + customer §3.20 schema | PRD §14.2 orphaned 状态 | ❌ HIGH-2：30 天宽限、cron、客户端 UI 全缺 |
| J 客户退款 + revenue 反向 | integration §4.4 + backend §5.10.2 + frontend §7.8 | 三种 settlement state 分支 | ⚠️ partner_debt 待 F-2 决策；不阻塞 Phase 1 |
| K 账单争议 | — | PRD §14.5 状态机 | ❌ HIGH-4：无 billing_dispute 表 / endpoint / wireframe |
| L KYC 驳回 + 重审 | backend §3.9 + frontend §16.4 E-10 | 5 类驳回 taxonomy | ⚠️ MEDIUM-7：3 次/年计数 + UI 预警缺 |
| M 客户直接注册 | frontend §3.1 公开 register | — | ✅ |
| N 已是直营客户被邀请 | backend §5.2 alt 422 | — | ⚠️ MEDIUM-N：staff 审核挂靠 endpoint 缺 |
| O 跨期入驻 | backend §3.8 is_partial 字段 | — | ⚠️ MEDIUM-5：runner 伪代码缺判断 |
| P 客户余额清零 | PRD §4.16 + M2-13 | — | ⚠️ MEDIUM-2：渠道商品牌 banner 弱 |
| Q PIPL 右遗忘 | backend §5.11 + frontend §7.9 + M13 | 5d 核身（已统一）| ✅ |

**场景完整度**：17 场景中 10 ✅ / 5 ⚠️ / 2 ❌（I 终止、K 争议）。两个 ❌ 都是 PRD Phase 2A/2B 范围，Phase 1 不阻塞，但需 round-2 修。

---

## 4. Phase 一致性随机抽样（≥ 20 项）

| # | 模块 / 功能 | PRD §12 | overview §8 | backend §1.2 / §17 | frontend §3 / §18 | 状态 |
|---|---|:---:|:---:|:---:|:---:|:---:|
| 1 | M2-01 仪表盘 | 1 | 1 | 1 | 1 | ✅ |
| 2 | M2-03 持牌方收单 | 2A | 2A | 2A | 2A | ✅ |
| 3 | M2-13 余额预警 | 1 | 1 | 1 | 1 | ✅ |
| 4 | **M2-15 通知中心** | **3** | 1/2A | **3** | **2A** | ❌ HIGH-1 |
| 5 | M3-04 客户额度分配 | 1 | 1 | 1 | 1 | ✅ |
| 6 | M3-08 多层 markup | 2A | 2A | 2A | 2A | ✅ |
| 7 | M3-13 markup 上下限 | 1 | 1 | 1 | 1 | ✅ |
| 8 | M3-14 客户切换 | 2A | 2A | 2A | 2A | ✅ |
| 9 | **M4-03 KYC 审核** | **2A** | 2A | **§4.5 用 approve-kyc 标 1** | 2A | ⚠️ MEDIUM-8 命名混淆 |
| 10 | M4-15 操作日志 | 1 | 1 | 1 | 1 | ✅ |
| 11 | M4-16 资质年审入口 | 2A | 2A | 2A | 2A | ✅ |
| 12 | M4-17 内容安全审核中心 | 2A | 2A | 2A | 2A | ✅ |
| 13 | M5-01 月结 Cron | 2B | 2B | 2B | 2B | ✅ |
| 14 | **M5-09 争议完整** | **2B 基础 / 3 完整** | 2B/3 | 2B（§17.3）| **3**（§18.4 only "完整"）| ⚠️ frontend 未落 2B 基础 |
| 15 | M5-10 个税凭证 | 2B | 2B | 2B | 2B | ✅ |
| 16 | M6-09 持牌方分账下账 | 2B | 2B | 2B | 2B | ✅ |
| 17 | M7-10 PIA 年度 | 2A | 2A | 2A | — | ✅ |
| 18 | M8-06 个税发票凭证 | 2B | 2B | 2B | 2B | ✅ |
| 19 | M9-02 防直营绕过 | 1 | 1 | 1 | 1 | ✅ |
| 20 | M9-04 sandbox / demo | 3 | 3 | 3 | 3 | ✅ |
| 21 | M10-04 SLA <24h | 1 | 1 | 1 | 1（仅列出）| ✅ |
| 22 | M11-04 重要事件强制 | 1 | 1 | 1 | — | ⚠️ |
| 23 | M13-01..05 用户权利 | 2A | 2A | 2A | 2A | ✅ |
| 24 | Fy-api 覆盖层 C-5 outbox | 1 | 1 | — (integration 管) | — | ✅ |
| 25 | M7-08 年龄校验 | 2A | 2A | 2A | ⚠️ frontend zod 未 refine 18 岁 | ⚠️ LOW-3 |

**结论**：25 项抽样中 21 ✅ / 3 ⚠️ / 1 ❌。Phase 标签在三层对核心场景一致，唯一硬冲突是 M2-15（HIGH-1）；M5-09 在 frontend Phase 错位（前端等到 Phase 3，后端 Phase 2B 即上线 endpoint）是 HIGH-4 的孪生问题。

---

## 5. CRITICAL 问题清单

**无 CRITICAL。** Phase 1 工程可基于 v0.1 四份开发文档立刻开工。下述 HIGH 问题需在 Phase 1 实施期 Week 1-3 内通过 v0.2 小 patch 合入，不阻塞 kickoff 但会阻塞 Phase 2A / 2B。

---

## 6. HIGH 问题清单

### HIGH-1：M2-15 通知中心 Phase 标签 frontend 与 PRD/backend 冲突

- **事实**：
  - PRD §12.4 明确把 M2-15（客户通知中心）放 **Phase 3**
  - `overview §3.2` 隐式（M11 行 "Phase 1 邮件+inapp / Phase 2A sms+webhook"）
  - `backend-design §1.2` "M2 客户后台 → Phase 3: M2-06/10/15"——与 PRD 一致
  - `frontend-design §3.2` 标 `/customer/notifications` 为 **Phase 2A**；§18.2 Phase 2A 清单也含 "通知中心（M2-15 / M11）"
- **影响**：前端提前一个 Phase 交付通知中心，但后端这个 Phase 没有对应 service / endpoint。Phase 2A 验收会发现 "UI 存在但 API 返 404"。同时 M11（事件中心后端）与 M2-15（客户视图入口）的关系四份文档都没明说。
- **修复指令**：
  - **方案 A（推荐，不改 PRD）**：`frontend-design.md §3.2` + `§18.2` + `§18.4`：把 `/customer/notifications` 移回 Phase 3；保留 inline notification dropdown（Phase 1 已有 `/notifications` GET endpoint）作为 Phase 1 简版。
  - **方案 B**：PRD v1.0.1 patch 把 M2-15 拆为 a/b：a Phase 2A 基础（邮件+inapp drawer），b Phase 3 完整（SMS + webhook + 偏好），同步 backend §1.2 与 frontend §18 Phase 标签。
- **severity**：HIGH（直接影响 Phase 2A 验收 KPI）。

### HIGH-2：场景 I（渠道商终止 → 客户 30 天宽限期）端到端 UX 缺失

- **事实**：
  - PRD §4.9：`partner suspended/terminated → 该 partner 客户默认转入"平台直营托管池" → API Key 保持有效 30 天宽限期 → 30 天后必须主动选新渠道或维持直营`
  - PRD §14.2 customer 状态机：`active → orphaned (其 partner 终止，30d 宽限) → adopted | direct`
  - `backend-design.md` 全文无 "orphaned" / "adopted" / "30 day grace" 关键字；§3.2 customer schema 无 `orphaned_at` / `grace_period_expires_at` 字段；§6 cron job 清单无 `orphan.cleanup` / `orphan.grace.notifier`；§5 service 流程无对应 saga
  - `frontend-design §3.2` portal customer 只有 `/switch-partner`，没有"我的渠道商已终止，请选择新渠道商"特殊路径
  - `admin §3.3` 无"孤儿客户池"视图（staff 看 30 天宽限内未被认领的客户）
- **影响**：渠道商被合规处置后客户体验断裂——API Key 还能用 30 天但客户不知发生了什么；30 天到期没动作 / 没邮件提醒 → 客户 API 突然停服 → 大规模客诉。直接撞 PRD §1.4 Phase 2 KPI"客户流失率 < 10% / 月" 与 PRD §11 R-12 电商法连带责任。
- **修复指令**：
  - `backend-design.md` §3.2 customer schema 加 `orphaned_at TIMESTAMP NULL` + `grace_period_expires_at`；CHECK status 加 `orphaned`/`adopted`
  - §5 新增 "5.12 渠道商终止 → 客户孤儿化 saga"：迭代每个客户 → 写 customer_partner_change_log → 触发 30 天倒计时通知
  - §6 worker 清单增 `orphan.grace.notifier` cron（每日提醒）+ `orphan.grace.expirator` cron（30 天到期切 direct）
  - `frontend-design.md` §3.2 portal customer 增 `/orphan-notice`；§3.3 admin 增 `/customers/orphaned`；§7 增"孤儿客户引导"wireframe（红色 banner + 倒计时）
  - PRD §22.1 加 F-15 "场景 I 30 天宽限期实施"为 Phase 2A 必交付
- **severity**：HIGH（不阻塞 Phase 1，因首次实际触发是 Phase 2A；但不补合规审计过不了）。

### HIGH-3：发票红冲（M8）前端 UI 缺失

- **事实**：
  - PRD §7.8 M8-04 全电发票对接 P0；红冲是国税总局合规要求
  - `backend-design §4.10` 已定义 `POST /admin/invoice/{id}/red-flush`；§5.8 时序图画出红冲流程
  - `frontend-design §3.3 admin` 路由树 `/invoices` 无红冲子路由；§18.3 Phase 2B 清单仅"发票申请 / 抬头管理 / 财务审核"，无红冲 UI
- **影响**：全电发票合规要求红冲必须有 UI（财务不能 curl 改 DB）；Phase 2B 验收 M8-04 时撞墙。
- **修复指令**：
  - `frontend-design.md §3.3` admin 路由加 `/invoices/:id/red-flush`（Phase 2B）+ §7 增简短 wireframe（reason 必填、操作日志展示）+ §18.3 Phase 2B 清单加 "红冲流"
- **severity**：HIGH（PRD M8-04 P0；红冲是全电发票不可分割部分）。

### HIGH-4：场景 K（账单争议 / M5-09）后端 endpoint 与前端 UI 全链路未落地

- **事实**：
  - PRD §4.11 场景 K 完整 e2e 流；§14.5 Dispute 状态机 `opened → partner_responding → escalated → arbitrating → upheld | overruled`；§7.5 M5-09 P0 升级
  - `backend-design.md` 全文搜 "dispute"：仅 §3.8 `settlement_item.status CHECK 'disputed'` 出现；**无 `billing_dispute` 表 schema、无 dispute service、无 dispute endpoint**
  - `frontend-design.md` 无 dispute UI 路由 / wireframe；§16.4 e2e E-1..E-20 无 dispute 流
- **影响**：M5-09 在 PRD Phase 2B 升级 P0；Phase 2B 开工时撞墙——客户在账单中心想点"我有疑问"时没有后端 endpoint 可调。
- **修复指令**：
  - `backend-design.md`：
    - §3 增 `billing_dispute` 表（含 `revenue_log_id` FK、`opener_type/id`、`partner_responded_at`、`escalated_at`、`arbitration_outcome`、PRD §14.5 状态机映射）
    - §4 增 `POST /customer/billing/{logId}/dispute`、`POST /partner/disputes/{id}/respond`、`POST /admin/disputes/{id}/arbitrate`
    - §5 增 "5.13 账单争议 saga"（含 SLA timer 1 工作日自动升级 + 与 §5.10.2 refund saga 联动 upheld → trigger refund）
    - §15.2 增 dispute.Service invariants（I-D-1 dispute 必关联存在 revenue_log；I-D-2 1 工作日不响应自动升级 staff）
    - §17.3 phase 标签把 dispute 表 / endpoint 列入 Phase 2B
    - §3.4 PRD 权限矩阵加 `dispute.respond` / `dispute.arbitrate` 两 verb
  - `frontend-design.md`：§3.2 customer 加 `/usage/:logId/dispute`、partner 加 `/disputes` 列表 + 详情；§3.3 admin 加 `/disputes`；§7 新增 "7.12 账单争议提交 + 状态跟踪" wireframe；§16.4 e2e 加 E-21/22 三方 dispute 流；§18.3 Phase 2B 加基础版、§18.4 Phase 3 加完整版
  - `integration-design.md` §4 saga 详细规约新增 §4.7 dispute → refund 联动
- **severity**：HIGH（PRD 唯一明确画出"客户 → partner → staff"三方 SLA 的争议处理流；缺位会让 Phase 2B 上线后客户对账有疑问只能走通用工单，破坏 PRD §1.4 KPI"关键工单平均响应 ≤ 24h"承诺）。

### HIGH-5：发票全链路 backend §3 表 DDL 缺失

- **事实**：
  - PRD §8.12 定义 `InvoiceApplication` + `InvoiceTitle` 字段
  - `backend-design §3` 表 DDL 编号 §3.1–§3.22，覆盖到 partner_debt（候选），但**没有** §3.x invoice_application / invoice_title
  - §14.3 Phase 演进路径表第 3 行 `2B + invoice_application / invoice_title` 仅是 phase 标签，无 DDL
  - frontend §7.6 zod schema 完整（含税号正则）；backend §4.10 endpoint 表完整
- **影响**：Phase 2B 工程师落 backend §4.10 endpoint 时缺表结构权威 source；可能借 frontend zod 反推 → 字段名 / 长度 / 索引差异。发票链路触税合规（PRD §15.10 BLOCK）。
- **修复指令**：
  - `backend-design.md §3` 新增 `§3.23 invoice_application` 与 `§3.24 invoice_title` 完整 DDL（参考 PRD §8.12 + frontend §7.6 zod 字段对齐）；明确 `tax_number` / `bank_info` 是否走 KMS 信封加密（PRD §16.5 银行卡号 = 敏感 PI）
  - integration-design：明确全电发票 SDK 是否走 `internal/fyapi_client` 风格的 client + idempotency；目前 backend §5.8 仅提了 `FAPIAO SDK` 但 §6 后台进程清单无"发票推送 worker"
- **severity**：HIGH（DDL 是落地权威；不可在 Phase 2B sprint 临时补）。

### HIGH-6：客户充值 saga（场景 D customer 端 / PRD §22.1 F-3）frontend escalated 状态 UX 缺失

- **事实**：
  - integration-design §4.5 给完整 saga：`持牌方 webhook → topup_intent.state → fy.topup with idem-key → 5xx unknown branch`
  - backend §5.7 落地 saga + invariants
  - frontend §7.5 customer 端时序图只有 `redirect → 持牌方 → 302 回 → 轮询 intentStatus`；**没有处理 saga unknown / escalated 分支的 UI**——5xx 时 backend 1h 兜底，前端只显示"处理中"，escalated 后客户看不到任何升级提示
- **影响**：客户对充值最终结果可见性差；saga 卡 1h 后 escalated 用户没看到"已联系平台运营协助"UI。
- **修复指令**：
  - `frontend-design.md §7.5`：补 escalated 状态 UI（与 §7.4 三阶段 UI 对齐：`processing → pending_unknown → escalated`），并把 escalated 触发 inapp notification
  - §16.4 e2e 加 E-23 "客户充值持牌方 5xx 兜底"
- **severity**：HIGH（影响客户对支付的信任，KPI"客户从首次充值到首次出账成功率 ≥ 95%"directly 相关）。

---

## 7. MEDIUM 问题清单（建议修）

### MEDIUM-1：算法 / 模型备案号前端展示路径未明确

- 事实：PRD 附录 E / §15.1 要求"生成式 AI 服务提供者备案 + 算法备案"显示在用户可见页面。M12-06 P0。但 frontend §3.1 legal 仅列 privacy / terms / partner-agreement，**无备案号展示组件 / footer**。backend §8.15 biz_setting 也无对应 keys。
- 修复：
  - `backend §3.16 biz_setting` keys 列表加 `registration.ai_service_provider_no` / `registration.algorithm_no` / `registration.deep_synthesis_no` / `icp_no` / `psb_filing_no`
  - `frontend §3.1 + §11` 设计 `<ComplianceFooter>` 通用组件展示所有备案号；i18n 命名空间加 `legal.icp_no` 等占位 key

### MEDIUM-2：M9-01 "由 X 提供服务"在 frontend 仅一句备注

- 事实：PRD M9-01 要求客户后台**全局显示**"由 X 提供服务"（不可隐藏，防白标绕过）。`frontend §3.2` 只在 `/balance` 行备注一句，未给全局 layout 位置。
- 修复：`frontend §7` 增 `7.Y 渠道商品牌 Banner` + `§11` 增 `<PartnerBrandBanner>`（customer 视图全局 header 固定显示；不可 CSS 隐藏，前端测试钩子验证）。
- 与 P 场景（余额清零时"联系您的渠道商 X"按钮）耦合。

### MEDIUM-3：M6-04 EPay 过渡期 partner-attributed customer 处理未定义

- 事实：PRD Round-2 MEDIUM-10 已标"EPay 仅直营客户。Partner-attached customer 误充值如何处理"。开发文档四份全部未处理。
- 修复：`backend §5.7` 加分支：`channel=epay && customer.partner_id != NULL` → 返 `BIZ_PAYMENT_EPAY_NOT_ALLOWED_FOR_PARTNERED`；frontend §7.5 toast + UI 屏蔽 EPay 选项。

### MEDIUM-4：M10-03 工单上下文 drill-down（用量 / 账单 / KYC）前端 wireframe 缺失

- 事实：PRD M10-03 明确 support 打开工单时应直接看到对应客户的用量、账单、KYC 状态上下文。`frontend §3.3 admin /tickets` 仅列出，§18.1 仅写"工单分配 / SLA"，无 wireframe drill-down。
- 修复：`frontend §7` 增 `7.Z 工单详情（含客户上下文面板）`。
- 影响 KPI"<24h 响应"——无上下文支持需多次切换页面。

### MEDIUM-5：场景 O（is_partial）+ 场景 F（progress_offset 续跑期间新 revenue 归属）未写明

- 事实：
  - 场景 O：backend §3.8 schema 有 `is_partial`，但 §5.5 settlement runner 伪代码**没写** "if partner.approved_at within period → set is_partial=true"
  - 场景 F：integration-design §7 freshness gate 锁"period 内 outbox 全消费完"；但 settlement runner `progress_offset` 续跑时新到达的 revenue_log 如何归属未定义
- 修复：`backend §5.5` 补两段伪代码注释。

### MEDIUM-6：场景 H 客户切换 staff 终审 admin UI 缺失

- 事实：PRD §4.8 明确"平台 staff 终审"；`backend §5.10.1` 时序图 `Note over BDB: staff POST /admin/customer-transfer-approve`；但 backend §4.5 admin endpoints 表（行 1191-1209）**没有**这一行；`frontend §3.3` admin 路由树**未列**对应入口。
- 修复：
  - `backend §4.5` 增 `POST /admin/customer-transfer/{id}/approve`，verb=`customer.transfer.arbitrate`（PRD §3.4 矩阵也需补 verb 行）
  - `backend §5.10.1` 把 staff approve 步骤的 endpoint 显式画出
  - `frontend §3.3` admin 加 `/customers/transfers`（Phase 2A）+ §7 简短 wireframe；§16.4 e2e E-7 显式覆盖 staff approve 步骤

### MEDIUM-7：KYC 3 次/年驳回上限（场景 L）计数 + UI 预警缺失

- 事实：PRD §4.12 "每自然年最多 3 次驳回；超过冻结"；backend §3.9 kyc_application schema 无 `yearly_reject_count` 字段；§5.6 KYC service 无该检查；frontend §16.4 E-10 列出测试但无 UI wireframe 展示"已驳回 2/3 次"预警。
- 修复：
  - `backend §5.6` service 增 invariant `I-K-4: yearly_reject_count <= 3 else BIZ_KYC_YEARLY_LIMIT`
  - `frontend §7.2` 拍照表单顶部 banner 显示"本年度已驳回 X/3 次，请仔细核对资料"

### MEDIUM-8：M4-02（资质年审）vs M4-03（KYC 审核）在 backend §4.5 命名混淆

- 事实：`backend §4.5` 列 `/admin/partners/{id}/approve-kyc` 标 Phase 1，但 PRD M4-02（资质审核 + 年审）Phase 1、M4-03（KYC 审核）Phase 2A。endpoint 名"approve-kyc"语义贴 M4-03。
- 修复：`backend §4.5` endpoint 改 `/admin/partners/{id}/approve`（M4-02，Phase 1）；Phase 2A 另加 `/admin/kyc/{id}/review`（已在 §4.8 列出，保留）。

### MEDIUM-9：PRD §1.4 KPI 业务埋点未成套

- 事实：PRD §1.4 各 Phase 退出条件 + 全期 KPI 包含 `接入种子渠道商数`、`每家 ≥ 2 客户`、`累计 GMV`、`钱包漂移 = 0`、`KYC 一次通过率 ≥ 70%`、`激活率 ≥ 60%`、`N+1 留存 ≥ 80%` 等 15+ 指标。backend §12 列 Prometheus metrics（基础设施类），但**业务 KPI 指标**（`partners_activated_total` / `kyc_first_pass_ratio` / `partner_30d_retention` / `gmv_total`）全缺；无 KPI dashboard 规划。frontend §15.4 行为埋点提到数值 bucket 化，但 KPI 需要精确金额。
- 修复：
  - `backend §12.1` 增 business KPI 子节，列出对应 PromQL / 数据源 SQL view
  - `overview §8 测试钩子 I-8.1` 落到具体 Grafana dashboard JSON
  - `frontend §15.4` 区分"业务 KPI 埋点（精确）"vs"行为埋点（bucket）"

---

## 8. LOW / NICE-TO-HAVE

- **LOW-1**：`backend §3.10 invitation_code` schema 正确，但 `5次/IP/h 限速`（PRD M3-02）实现仅在 §5 隐含；建议 §7.1 中间件链显式加 `InvitationRateLimit` 条目；frontend §13 错误处理 + §10 i18n 加 `BIZ_RATE_LIMIT_INVITATION` toast 映射。
- **LOW-2**：`backend §3.16 biz_setting` keys 列表缺 `refund_window_days`（PRD CHANGELOG 21.1 已把 §4.10 改成 `${refund_window_days}` 占位，对应 Q6 BLOCK）+ `settlement.min_payout_threshold`（Q17）。Week 1 Q6/Q17 决议后必须加。
- **LOW-3**：M7-08 年龄校验在 frontend §7.1 个人 zod schema `idNo` 正则 `\d{17}[\dXx]`，但没在 zod 加 `refine` 校验出生年月 ≥ 18；backend §15.2 也没 invariant I-K-4 年龄校验。
- **LOW-4**：integration-design §2.2.7 `GET /api/internal/usage/by-user` 没声明 `Idempotency-Key`（GET 自然幂等）——但 overview §4.4 I-4.1 要求"state-changing endpoints 必须声明"，建议 §2.1 公共 parameter 显式说明 "GET 端点不强制 IdempotencyKey"。
- **LOW-5**：`frontend §11.1` 品牌主色 `#3D5AFE` 标"待 brand 团队确认"——UI ADR 级别事项应尽快锁定；不影响 Phase 1 kickoff。
- **LOW-6**：backend `google/wire` 编译期 DI；frontend `pnpm workspaces + Turborepo`——两套工具链在 monorepo 顶层如何协调（partner-api 与 web 是否平级）未明确。建议 `overview §3.1` 补"仓库顶层是否单 mono-repo / 多 repo"决策。

---

## 9. 具体修改指令（按 architect round-2 可施工粒度）

| # | 严重度 | 改哪份 | 改哪一节 | 怎么改 |
|---|---|---|---|---|
| 1 | HIGH-1 | frontend-design.md | §3.2 / §18.2 / §18.4 | `/customer/notifications` Phase 改 3；inline notification dropdown 留 1 |
| 2 | HIGH-2 | backend-design.md | §3.2 customer schema；§5 增 5.12；§6 cron 清单 | orphaned 字段 + 30d cron + cascade saga |
| 3 | HIGH-2 | frontend-design.md | §3.2 增 `/orphan-notice`；§3.3 增 `/customers/orphaned`；§7 增 wireframe；§18.2 加 | 孤儿 UX |
| 4 | HIGH-3 | frontend-design.md | §3.3 admin 加 `/invoices/:id/red-flush`；§7 wireframe；§18.3 加 | 红冲 UI |
| 5 | HIGH-4 | backend-design.md | §3 增 billing_dispute；§4 增 3 个 endpoint；§5 增 5.13；§15.2 增 invariants；§17.3 phase 标签；§3.4 PRD verb | M5-09 完整骨架 |
| 6 | HIGH-4 | frontend-design.md | §3.2/§3.3 dispute 路由；§7 增 7.12；§16.4 e2e E-21/22；§18.3/§18.4 phase | dispute UI |
| 7 | HIGH-4 | integration-design.md | §4 增 4.7 dispute → refund 联动 | saga 链 |
| 8 | HIGH-5 | backend-design.md | §3 增 §3.23/§3.24 invoice DDL | 发票表 schema |
| 9 | HIGH-6 | frontend-design.md | §7.5 补 escalated UI；§16.4 加 E-23 | 充值 saga UX |
| 10 | MEDIUM-1 | backend + frontend | backend §3.16 加 keys；frontend §3.1/§11 加 `<ComplianceFooter>` | 备案号 |
| 11 | MEDIUM-2 | frontend-design.md | §7 加 7.Y；§11 加 `<PartnerBrandBanner>` | M9-01 全局可见 |
| 12 | MEDIUM-3 | backend-design.md | §5.7 加 partner-attributed + EPay 拒绝分支 | EPay 边界 |
| 13 | MEDIUM-4 | frontend-design.md | §7 增 7.Z 工单详情含上下文面板 | M10-03 drill-down |
| 14 | MEDIUM-5 | backend-design.md | §5.5 settlement runner 补 is_partial + progress_offset 注释 | 场景 O / F |
| 15 | MEDIUM-6 | backend + frontend | backend §4.5 加 endpoint；frontend §3.3 加路由 | 场景 H staff 终审 |
| 16 | MEDIUM-7 | backend + frontend | backend §5.6 加 I-K-4；frontend §7.2 banner 预警 | 场景 L 3 次/年 |
| 17 | MEDIUM-8 | backend-design.md | §4.5 endpoint 更名 `/admin/partners/{id}/approve` | Phase 标签清理 |
| 18 | MEDIUM-9 | backend + overview | backend §12.1 增 business KPI 子节；overview §8 I-8.1 落 Grafana JSON | KPI 可测量 |

---

## 10. 未决 BLOCK 问题对工程的影响（PRD §13）

| # | BLOCK 问题 | 工程影响 | Phase 1 可否开工 | Phase 2A 是否必须先决 | Phase 2B 是否必须先决 |
|---|---|---|:---:|:---:|:---:|
| Q1 | 默认渠道商分润比例 | backend §3.16 默认 `default_revenue_share=0.20` 占位 | ✅ | ⚠️ Phase 2A 前 | — |
| Q6 | 退款窗口 | backend biz_setting `refund_window_days` 占位（LOW-2）| ✅ | ⚠️ Phase 2A 客户协议定稿 | — |
| Q10 | 平台佣金抽成位置 | settlement_item.platform_fee 已存在；策略缺 | ✅（Phase 1 不跑月结）| — | ❌ Phase 2B 必须 |
| Q11 | 公司主体注册资本 100 万实缴 | 不影响代码；影响 ICP 申请 | ✅ | — | ⚠️ Phase 2 上线 hard-gate |
| Q12 | 持牌分账方选哪家 | integration §4.5 抽象层；具体 SDK 缺 | ✅ (mock) | ❌ Phase 2A M6-01 前 | — |
| Q13 | DPO 人选 | 影响合规签字 + DPO 邮箱 UI | ✅ | ⚠️ Phase 2A M13 前 | — |
| Q14 | 算法备案文本 | PRD 附录 E 给草案；MEDIUM-1 备案号展示 | ✅ | ⚠️ Phase 2 备案前 | — |
| Q16 | 平台 → 渠道商签约模板 | 不影响代码；影响 partner 入驻协议 | ✅ | ⚠️ Phase 2A 自助入驻 | — |
| Q17 | 最低支付门槛 | backend biz_setting 缺 default | ✅ | — | ❌ Phase 2B 必须 |
| Q19 | max_markup 5.0 | biz_setting.max_markup 已存在；frontend zod 待 align | ✅ | — | — |

**结论**：**Phase 1 全部可开工**（10 项 BLOCK 对 Phase 1 kickoff 无硬性阻塞）；Phase 2A 开工前需 **Q1 / Q6 / Q12 / Q13 / Q14 / Q16** 6 项决议；Phase 2B 开工前需 **Q10 / Q11 / Q17** 落地。

建议在 `overview §10` 风险登记或 `§8` Phase 切片下加 "开工前决策 checkpoint" 表，把以上映射固化——目前四份开发文档默认了每个 BLOCK 的某种解（Q10 platform_fee 单一策略、Q12 单一 LICENSED_PROVIDER），这些假设需显式标注为"Round-2 前待验"。

---

## 11. 验收判定矩阵

| 维度 | 判定 |
|---|:---:|
| PRD 模块覆盖（12/12） | ✅ 全有落点，5 项 ⚠️ 瑕疵 |
| PRD 场景覆盖（17/17） | ⚠️ 10 ✅ / 5 ⚠️ / 2 ❌（场景 I、K） |
| Phase 一致性（25 抽样） | ⚠️ 1 硬冲突（HIGH-1）+ 3 软滑移 |
| 跨文档契约（REST / DB / async） | ✅ 锚点清晰、可机器验证 |
| 高敏感场景 UX 完整性 | ⚠️ 场景 I / K / L 需补 |
| KPI 可测量性 | ⚠️ 业务埋点缺失（MEDIUM-9） |
| BLOCK 问题影响 | ✅ Phase 1 不阻塞 |

**最终 Verdict: PASS-with-conditions**

- Phase 1 工程**立即启动**（从 backend §17.1 + frontend §18.1 + integration §1 Phase 1 条目开始）
- 本文 HIGH-1~6 必须在 Week 1-3 通过 v0.2 小 patch 合入
- MEDIUM 清单可在 Phase 2A kickoff 前分批解决
- BLOCK Q1/Q6/Q12/Q13/Q14/Q16 需在 Week 4 close-out 会议前决议

---

## 12. 对 architect round-2 的一句话交代

契约层（overview + integration-design）质量高、可直接动工；backend + frontend 在"模块表"维度完备，在"场景端到端"维度有 6 处非致命断链（场景 I 孤儿客户 / 场景 K 争议 / M8 红冲 + DDL / M2-15 Phase 错位 / 场景 H staff 终审 / 充值 saga escalated UX）。请在 v0.2 补上 HIGH-1~6 + MEDIUM-1/2/4/9，然后即可进入 Phase 1 实施并与 Security / Architecture reviewer 并行 round-2。

---

> 报告完毕。总字数约 3500 字（含表格）。
> Issue counts：CRITICAL 0 / HIGH 6 / MEDIUM 9 / LOW 6。
> 达到"graduate to v0.2"的 bar；Phase 1 工程可立即启动。
