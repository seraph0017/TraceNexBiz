# Dev Design Round-1 Review — Architect (independent 2nd reviewer)

> Date: 2026-05-10
> Reviewer: Architect (独立第二位 reviewer，并非任何一份开发文档的起草人)
> Scope: 四份开发文档 v0.1 共 6752 行 + PRD v1.0 (2295 行) + Round-2 PRD architect verdict
> Verdict: **NEEDS REWORK**（CRITICAL=4 / HIGH=11，远高于 round-2 通过门槛 CRITICAL=0 / HIGH≤3）

---

## 1. 执行摘要

三句话评语：

1. **集成层契约本身是 doc 集合中最扎实的**——`integration-design` v0.1 把 PRD §9 / §22.1 + Round-2 architect verdict 几乎全部转成了"工程级具象"，HMAC 4 元组、outbox 表、saga 状态机、GRANT 矩阵都在；但**契约的"消费侧"（backend / frontend）多处与契约文本不闭环**：JWT 是放 cookie 还是 header 在 overview / backend / frontend 三份文档之间打架，`audit_log.id` 来源在三处描述自相矛盾，`/api/internal/token/create` 返回的 sk-key 是否最终能流到 partner UI 三份文档没对齐。
2. **数据模型层有数处"假完整"**——20 张表 DDL 看似齐全，但 `partner_wallet.balance` 没有 CHECK（与 PRD §22.1 F-2 决策直接相关）、`saga_step` 状态枚举三处不全等、`audit_log.id` 与 `audit_log_unsealed.id` 同源约束没在 schema 强制、`pipl_request` / `content_safety_event` / `password_reset_token` 三张 backend §14.3 phase 演进表点名的表在 §3 全表清单中**根本没出现**。
3. **Saga / outbox 端到端在快乐路径下成立，但失败路径有真破绽**——尤其是 backend §8.1 idempotency middleware 的"先 lookup 再 service 内 commit"机制本身**与 ADR-003 的"同 TX"承诺不一致**（middleware 已经 c.Next 出去了不可能"同 TX"），同时 outbox poller 在跨实例 LOG_DB 场景下 `SELECT FOR UPDATE SKIP LOCKED → DELETE` 与 `bizDB.INSERT revenue_log` 的 commit 顺序在 §3.3 伪代码中不正确（成功 / 失败两条路径都用 `tx.Delete(&r)`），导致 retry / dead_letter 路径形同虚设。

下面是详细分项。

---

## 2. 跨文档契约对齐矩阵（≥ 15 项）

| # | 契约点 | overview | integration-design | backend | frontend | 一致性 | 矛盾 / 备注 |
|---|---|---|---|---|---|:---:|---|
| 1 | JWT 载体 | §2.2 流量分类 "TLS 1.3 + JWT" + §3.4 footer "在 `Authorization: Bearer` header 里" + ADR-007 | §1.1.3 透传 `Authorization`（默认假设）| §7.2 `extractBearer(c)` ✅ Authorization header | §6.2 ADR-F5 "**httpOnly cookie `tnbiz_access`，不走 Authorization header**" | ❌ | overview / backend = header；frontend = cookie。后端 middleware 不会读 cookie——**CRITICAL** |
| 2 | tnbiz_session 角色 | §3.4 / ADR-007 "仅做 CSRF 与 UI 引导，不参与鉴权决策" | 未提 | §7 `Session (CSRF Origin/Referer check)` | §12.4 同时列 `tnbiz_session`、`tnbiz_access`、`tnbiz_refresh`、`tnbiz_csrf` 四个 cookie | ⚠️ | frontend 把 cookie 数量从 1 膨胀到 5；`tnbiz_session` 与 `tnbiz_access` 角色重叠 |
| 3 | `/api/internal/token/create` 返回 sk-key | — | §2.2.4 "key... ★ 只在创建时返回，partner 永远不可见客户 key" | §4.3 `/customer/api-keys` POST 返 `key`；§4.4 表注 "partner 视角调用 `/customer/api-keys` 一律返 404" | §3.2 `/customer/api-keys` 仅 customer 视图 | ⚠️ | 三处都说 partner 看不到，但**没有任何文档定义 partner 端"为客户开 token"路径**；§4.4 footer 说有 `customer.allocate_quota` verb 而不是 token 路径——但 backend §4.4 表里没有 partner 创建 token 的 verb / endpoint。partner 是否能为客户开 token 没有最终答案 |
| 4 | `audit_log.id` 来源 | §9 ADR-006 "sealer 进程按 id 顺序计算 ... 应用 user 仅 INSERT (excluding hashes)" — **未明示 id 来源** | — | §3.13 `audit_log.id BIGINT NOT NULL`（**无 AUTO_INCREMENT**）+ §10.1 / §3.13 注 "id 取自 audit_log_unsealed.id" + backend §19 第 2 条主动 ask | — | ⚠️ | overview ADR-006 必须补丁；schema 层没有强制 |
| 5 | `saga_step` UNIQUE | §4.3 异步消息表只列字段不列约束 | §4.1 `SagaStep` Go struct **未声明 `gorm:"uniqueIndex"`** | §3.17 `UNIQUE KEY uk_saga_step (saga_id, step_name)` ✅ | — | ❌ | integration / backend 不一致；同 step retry 会插多行 |
| 6 | `partner_wallet.held_amount` 处置 | ADR-012 "倾向 drop"（待 M-4） | — | §3.3 / §18 ADR-009 "保留 + 每日 drift 检查"（**与 overview 立场相反**）| — | ❌ | overview / backend 结论相反 |
| 7 | `partner_debt` 模型 | ADR-010 "倾向方案 A"（独立表）| — | §3.22 给出 DDL + §18 ADR-005 "倾向方案 A" | — | ✅ | 立场一致；但 backend phase 标 "Phase 2A or later" 与 PRD F-2 "Phase 2B" 节奏稍歪 |
| 8 | outbox poller 消费成功后 DELETE 还是 UPDATE | §9 ADR-011 "consumed 行 30d 后批量 DELETE" | §3.3 伪代码无条件 `tx.Delete(&r)` + §3.4 又说 `markFailed status='pending' retry_count++` | §5.4 注 "consumed 行直接 DELETE" + §3.19 struct 含 `ConsumedAt *time.Time` | — | ❌ | "消费即 DELETE" 与 "30d 后 DELETE" 与 "失败 markFailed" 三种描述混用——CRITICAL |
| 9 | idempotency_record 与业务 TX 同事务 | — | §5 注 TTL 单调递增；未谈同 TX | §8.1 / ADR-003 "service 层在 bizDB.Transaction 内同时 INSERT" — **但 §8.1 middleware 代码示意中 `repo.Insert(c, &Record{...})` 在 `c.Next()` 之后调用（不在业务 TX 内）** | — | ❌ | ADR-003 与 §8.1 实现自相矛盾——CRITICAL |
| 10 | trace_id 字段持久化 | §4.4 列 audit_log / revenue_log（**待加列**）/ saga_step / consume_log_outbox | §9.1 同 | §3 各 DDL：revenue_log/saga_step/audit_log/notification_outbox/wallet_log ✅；**idempotency_record 缺 trace_id** | §15.3 客户端 trace_id | ⚠️ | idempotency_record 应补 trace_id |
| 11 | `consume_log_outbox` 软删 vs 硬删 | ADR-011 "rows 已 DELETE" | §3.1 Go struct 未列 `gorm.DeletedAt`；§3.3 `tx.Delete(&r)` 默认是 GORM 软删 | §5.4 同 ADR-011 | — | ❌ | GORM 默认软删会写 deleted_at，但 §1.5.2 DDL 没此列——struct vs DDL vs 行为三处不闭环 |
| 12 | partner-api → Fy-api Idempotency-Key 透传 | §4.3 invariant "透传相同 Idempotency-Key" | §5.2 同 | §5.3 saga 伪代码透传 ✅ | §5.3 客户端生成透传 ✅ | ✅ | 唯一一致点 |
| 13 | OpenAPI YAML 物理路径 | §4.1 "openapi/partner-api.yaml 在 partner-api 仓库内" | §2 注 "完整 spec 后续放 openapi/internal-api.yaml"；§11 未具体到仓库 | §1.1 `openapi/partner-api.yaml` + `openapi/payment-callback.yaml` 在 TraceNexBiz repo 根 | §17 / §5 通过 orval 拉 partner-api.yaml | ⚠️ | `internal-api.yaml`（Fy-api 覆盖层 spec）物理路径未指定——是检入 Fy-api repo 还是 TraceNexBiz repo？ |
| 14 | settlement freshness gate 阈值 | §7 表 "outbox lag p95 < 60s" | §7.2 `< 60s` ✅ | §5.5 freshness gate 描述无具体阈值 | — | ⚠️ | backend §5.5 缺阈值 |
| 15 | settlement runner leader 选举 | ADR-008 Redis SETNX → K8s Lease | — | §6 表 settlement-runner replicas=2 (1 leader)；audit-sealer replicas=1 也要 leader——**单 replica 不需要 leader** | — | ⚠️ | backend §6 表把 single-replica 与 leader-required 混列 |
| 16 | wallet drift 每日对账 | §7 / §10 "每日对账" + ADR-W1 | — | §6 表 `wallet.drift.checker` daily 04:00 + §15.2 I-W-4 | admin 路由没列 drift dashboard | ⚠️ | 后端有 metric 但 frontend 没暴露给 admin |
| 17 | mTLS 在 K8s 终结层 | §2.2 "mTLS 终结在 K8s service mesh / sidecar 层" | §1.1.3 备注同 | §13 K8s 配置无 service mesh 段 | — | ⚠️ | backend §13 ops topology 留白；与 overview §2.2 不闭环 |
| 18 | LOG_DB 拆分时跨库 JOIN fallback | §4.2 "fallback 到 GET /api/internal/usage/* HTTP" | §2.2.7 endpoint ✅ | §6 reporting service 未明示拓扑切换路径 | — | ⚠️ | backend reporting 切换机制（feature flag？dial test？）未明示 |
| 19 | by-idem-key 探活 KeyId 鉴权 | §4.4 没特别约束 | §2.2.10 "只允许原始提交者的 X-Auth-KeyId" | §8.2 retry worker 调用未显式提鉴权；fyapi_client 默认单 KeyId | — | ⚠️ | partner-api 多 cmd（settlement-runner / outbox-poller / api / saga-runner）是否共享 KeyId 未明 |
| 20 | F-3 客户充值 saga `Idempotency-Key` 类型 | — | §4.5.2 "topup_intent.id 作为 saga_id / Idempotency-Key" | §3.21 `topup_intent.id BIGINT`，§3.16 `idempotency_record.idempotency_key VARCHAR(64)` | — | ❌ | OpenAPI §2.1 声明 `IdempotencyKey: format: uuid`——BIGINT 字符串违反契约——HIGH |

**矩阵小结：✅=4 / ⚠️=11 / ❌=5**。⚠️ 已超 health threshold；❌ 全部是 round-2 阻塞项。

---

## 3. 起草人未决 / 不确信决策仲裁

backend §19 起草人列出 10 项；frontend §19/§20 ADR-F 与 F-R 风险若干；overview §9 ADR 中 ADR-010/011/012 显式留白。逐条仲裁如下。

### 3.1 backend §19 起草人列出 10 项

| # | 起草人疑问 | Architect verdict |
|---|---|---|
| 1 | `consume_log_outbox.deleted_at` 缺失，物理 DELETE vs 软删 | **物理 DELETE**（与 ADR-011 一致）。要求 integration §3.1 显式注 "no `gorm.DeletedAt` 字段"；DDL 不加 deleted_at；GORM 用 `Unscoped().Delete(&r)` 强制硬删 |
| 2 | `audit_log.id` 与 `audit_log_unsealed.id` 同源 | 同意 backend，但**必须在 overview ADR-006 补丁**显式声明 "sealer 严格按 unsealed.id ASC 顺序消费，audit_log.id = unsealed.id"；同时 audit_log_unsealed 加 schema 注释 "sealer DELETE 后 autoinc 不回退" |
| 3 | idempotency_record TTL ≥ saga wall-clock 启动期断言 | **同意 strong assertion**。`cmd/api` startup 检测 `biz_setting.saga_wall_clock_hours <= idempotency_ttl_hours` 否则 panic。invariant 写进 overview §4.3 |
| 4 | `saga_step` UNIQUE(saga_id, step_name) | **必须加**。integration §4.1 SagaStep struct 加 `gorm:"uniqueIndex:uk_saga_step;"`；retry 用 `INSERT ... ON DUPLICATE KEY UPDATE attempts=attempts+1` |
| 5 | partner_wallet.held_amount 保留 vs drop | **drop**。理由：(a) overview ADR-012 已倾向 drop；(b) backend ADR-009 主张保留+drift 的"理由"是"dashboard 单查多 join sub-query"——这是伪问题，因 wallet_hold 上有 `idx_hold_partner_held(partner_id, status, held_at)`，sub-ms 命中；(c) denorm 多一个出错点。**整改**：backend §3.3 删字段、§5 wallet 业务改纯计算、ADR-009 翻判 |
| 6 | F-1 per-model markup | **Phase 2A 起开发**，覆盖层加 `user_model_ratio_override` 表（fy_api_db）。本轮 review 不强制；先把 schema 占位 + biz_setting feature flag。integration §1.4.4 显式列入 Phase 2A entry criteria |
| 7 | partner-api 是否管理 fy_api_db.users.email 唯一性 | **不管理**。partner-api 仅 SELECT；唯一性归 Fy-api。澄清后 backend §5.2 在 customer.handler 加注释 |
| 8 | SLS 跨索引 trace_id | **延后到 ops topology Q11+**。backend §12 / overview §9 显式标 "依赖 ops Q11+"；fallback：partner-api / Fy-api 各自 logstore 不同 project 时走 SLS Federation API |
| 9 | MFA 在 KYC 通过到首次登录之间的 session | **强制 logout + revoke jti 重新登录**。状态变化 → 全部 jti 失效是最安全策略。backend §7.5 已基本表达，需写入 audit_log 显式 action `partner.mfa_enforced_after_kyc` |
| 10 | Pub/Sub `customer_update` 频道 | **不在 Phase 1 引入**。partner 改 markup 影响的 customer 视图缓存靠 TanStack Query mutation onSuccess invalidate（frontend §4.4 已有）。Phase 2A 评估 |

### 3.2 ADR-F5 vs ADR-007 JWT cookie 仲裁（最关键）

**最关键的不确信决策**：

| 文档 | 立场 |
|---|---|
| overview ADR-007 / §3.4 | "JWT 在 `Authorization: Bearer` header 里" |
| backend §7.2 | `extractBearer(c)` Authorization header |
| frontend ADR-F5 | "JWT 走 httpOnly cookie，不走 Authorization header" |

**Architect verdict**：**采用 frontend ADR-F5（cookie 路径）**，理由：

- `Authorization: Bearer` + memory store + refresh cookie 在 SPA 单页刷新时丢失 access_token，依赖 refresh 必产生竞态；cookie + double-submit CSRF 综合 XSS / CSRF 防御更好（frontend §6.2 论证）。
- 本系统 Phase 1 不对外暴露 partner SDK / 第三方 API；如有此需求，单独走 API key + HMAC（与 partner-api → Fy-api 内部路径同模式），与 JWT 路径无冲突。
- SSR storefront 路径上 cookie 也更自然。

**整改要求**：
- overview §3.4 footer + ADR-007 重写为 "JWT 走 httpOnly cookie `tnbiz_access`；partner-api 在 middleware 从 cookie 抽取，不读 `Authorization` header"。
- backend §7.2 把 `extractBearer(c)` 替换为 `extractAccessFromCookie(c)`；prod 仅 cookie；migration 期间可同时读 header（feature flag）。
- 新增 ADR-007 修订条目作为 v0.2 标记。

### 3.3 ADR-006 audit chain race

backend §10 "200ms tick" + "loop 1000 rows" sealer，但**没说 sealer 与应用 INSERT 的 race 处理**：sealer 正在读 LIMIT 1000 时，应用同时 INSERT 新行——sealer 漏读直到下个 tick。

**Verdict**：可接受（最终一致），但需要：
1. backend §10.1 显式标 "audit 不保证 200ms 即时入哈希链；保证 < 5s p95"
2. sealer crash 4h 时 unsealed 表会堆积——加观测 metric `audit_unsealed_pending_count` + alert > 1000

### 3.4 ADR-008 cron leader 仲裁

overview ADR-008 + backend ADR-011：Phase 1 Redis SETNX，Phase 2 K8s Lease。但 backend §6 表 `cmd/audit-sealer (replicas=1)`——若 replicas=1 就**不需要 leader 选举**。

**Verdict**：
- audit-sealer：单 replica + K8s StatefulSet/Deployment 即可，不需 SETNX leader
- settlement-runner：必须 leader（防双跑）
- kek-rotator：单 replica 即可
要求 backend §6 表区分清楚。

### 3.5 Frontend F-R risk 与 ADR-F1..F10

ADR-F1（admin 独立 SPA）/ ADR-F2（pnpm + Turborepo）/ ADR-F3（TanStack + Zustand）/ ADR-F4（orval）/ ADR-F7（zod）/ ADR-F8（SSR storefront）/ ADR-F10（presigned URL）—— **全部 accept**，论证充分。
ADR-F5（cookie）—— accept，但要求 overview/backend 同步整改（见 §3.2）。
ADR-F6（PII view types 分离）—— accept 但需要 backend §4 OpenAPI spec 显式声明 `*ForPartnerView` 与 `*ForAdminView` 两种 schema（目前 backend §4 只有一种 schema）。
ADR-F9（禁 inline script + 禁 localStorage 存 token）—— accept。

---

## 4. 数据模型审计（20+ 表）

按 backend §3 顺序逐张过，引用 backend-design 行号。

| # | 表 | 行号 | 评分 | 问题 |
|---|---|---|---|---|
| 1 | `partner` | §3.1 248-284 | ✅ | tier 0-9 与 PRD §9.2 ≤10 档对齐；CHECK 完整。**轻微**：cipher 字段长度（VARBINARY(512)）多处出现需统一定义在 `pkg/pii` |
| 2 | `customer` | §3.2 290-313 | ⚠️ | `joined_via` CHECK 4 种与 PRD §M2-01 5 种 join 方式可能不齐；`status='deleted'` + `deleted_at` 冗余 |
| 3 | `partner_wallet` | §3.3 319-336 | ❌ | **缺 CHECK on balance** — F-2 决策 partner_debt（方案 A）后 balance 必须 ≥ 0；当前 schema 暗示方案 B（允许负数）。需在 F-2 决议后回填 |
| 4 | `partner_wallet_log` | §3.4 343-365 | ✅ | UNIQUE(idempotency_key, type) 合理；refund 路径 type='refund_clawback' 的 idem-key 必须与 saga_id 同源（这点 §5 没强调） |
| 5 | `wallet_hold` | §3.5 373-392 | ✅ | UNIQUE(saga_id) ✅；与 saga_step.saga_id 字段冗余但可接受 |
| 6 | `partner_pricing_rule` | §3.6 400-422 | ⚠️ | UNIQUE 防起点重复；重叠校验只能 service 层强制（PM MEDIUM-2）；markup 1.0-100.0 ✅ |
| 7 | `revenue_log` | §3.7 432-455 | ✅ | UNIQUE(fy_api_log_id, occurrence) ✅；occurrence 1-127 |
| 8-11 | settlement / settlement_item / settlement_run / settlement_config_change_log | §3.8 462-536 | ⚠️ | settlement_run.lease_expires_at 缺 CHECK >= started_at；settlement.status 含 `gate_failed` 但 §5.5 状态转换路径不全 |
| 12 | `kyc_application` | §3.9 544-578 | ⚠️ | 17 个 cipher/key_id 字段对未抽象（debt D-9）；建议 Phase 2A 重构成 `kyc_application_pii` 关联表 |
| 13 | `invitation_code` | §3.10 585-603 | ✅ | OK |
| 14 | `seat` | §3.11 611-627 | ⚠️ | polymorphic `owner_type` + `owner_id` 无 FK；service 层防孤儿 invariant 在 §15.2 缺失 |
| 15 | `invoice_application` + `invoice_title` | §3.12 635-676 | ✅ | OK；invoice_application 通过 settlement_item.invoice_id 反查 partner 可接受 |
| 16 | `audit_log` + `audit_log_pii` + `audit_log_unsealed` | §3.13 687-735 | ❌ | (a) audit_log.id 无 AUTO_INCREMENT 且 sealer 路径无 schema-level 强制；(b) unsealed/audit_log/audit_log_pii 之间无 FK；(c) sealer DELETE 后 autoinc reuse 风险——加 schema 注释 |
| 17 | `staff` | §3.14 745-765 | ✅ | argon2id + WebAuthn 完整 |
| 18 | `biz_setting` | §3.15 773-780 | ⚠️ | 缺 `value_type` 列；无法 schema 强制 int/bool/json；config 注入安全是必修项 |
| 19 | `idempotency_record` | §3.16 788-804 | ⚠️ | 缺 `trace_id`；response_cipher VARBINARY(16384) 16KB 限制对 export 类响应不够 |
| 20 | `saga_step` | §3.17 812-831 | ⚠️ | UNIQUE(saga_id, step_name) ✅；status 枚举与 integration §4.1 / §4.2 状态机三处不全等（integration §4.1 列 6 个 / §4.2 状态机增 `escalated`/`released_pessimistic` / backend chk 列 7 个） |
| 21 | `ticket` / `ticket_reply` / `notification_outbox` / `consent_log` | §3.18 839-905 | ⚠️ | notification_outbox 缺 (event_code, recipient, ref_id) UNIQUE——重复触发会发多次邮件 |
| 22 | `consume_log_outbox` | §3.19 909 | — | 引用而不重复 OK |
| 23 | `customer_partner_change_log` | §3.20 917-934 | ⚠️ | 缺 status 列；场景 H 三方确认状态机不持久化只能从 audit_log 反查 |
| 24 | `topup_intent` | §3.21 942-961 | ⚠️ | saga_id 没 UNIQUE；webhook 重传幂等只靠 (channel, out_trade_no) UNIQUE 兜底 |
| 25 | `partner_debt` | §3.22 969-984 | ⚠️ | F-2 决议落地后挂业务；Phase 标 "2A or later" 与 PRD F-2 / Phase 2B 节奏稍歪 |

**漏表清单**（backend §14.3 phase 演进表点名但 §3 无 DDL）：

- `pipl_request`（Phase 2A，PIPL erase 5d 流程）
- `content_safety_event`（M4-17 内容安全审核中心）
- `password_reset_token`（§7.9 密码重置）
- 建议新增 `webhook_event_dedup`（独立兜底，避免侥幸靠 topup_intent.UNIQUE）

---

## 5. Saga / 幂等 / Outbox 端到端走查（3 条关键链路找茬）

### 5.1 链路 A：M3-04 渠道商→客户分配额度 saga

**找茬 1**：integration §4.3.1 时序图 step 6 写 "INSERT idempotency_record (pending)"——**与 backend §8.1 + ADR-003 矛盾**。backend 说同 TX 内 service 写；integration 说 saga 入口 middleware 写"pending"。两侧不闭环。

**Verdict**：以 backend ADR-003 为准——middleware 仅 lookup + 409；service 在 `bizDB.Transaction` 内 INSERT。但 backend §8.1 middleware 代码示意中 `repo.Insert(c, &Record{...})` 在 c.Next() 之后调用，**也错了**——这就在 c.Next() 之外，非业务 TX。最终路径必须是：

```
middleware:  lookup → 命中返 cache; 不命中 → 透传业务执行
service:     bizDB.Transaction 闭包内：业务 INSERT/UPDATE + idempotency_record INSERT 同 TX
middleware:  c.Next 后不再写 idempotency_record
```

**找茬 2**：retry worker 与主请求 race——主请求 step 8 后 GC sleep 0.1s，retry worker 30s 后启动。两者用同 idem-key 调 Fy-api，Fy-api 端 internal_idempotency 一次执行 OK。但 partner-api 端两个 caller 都到 step 11 准备 commit hold——`UPDATE wallet_hold SET status='committed'` idempotent OK，但 `UPDATE partner_wallet SET balance -= amount, version+=1` 乐观锁第二次必 fail。**backend §8.2 retry 逻辑没显式覆盖此 race**。

**Verdict**：integration §4.3.2 加分支 "wallet.commit version mismatch → re-read wallet, idempotency_record 已存 → return cached"；backend §8.2 retry 在 commit step 必先 SELECT idempotency_record。

### 5.2 链路 B：outbox 消费 → revenue_log

**找茬 3 (CRITICAL)**：integration §3.3 / backend §5.4 伪代码：

```go
return p.logDB.Transaction(func(tx *gorm.DB) error {
    // SELECT FOR UPDATE SKIP LOCKED LIMIT 1000
    for _, r := range rows {
        processOne(...)  // 跨实例 bizDB.Create(revenue_log)
        tx.Delete(&r)    // 无条件 DELETE
    }
})
```

`processOne` 在 `bizDB`（partner_db）写 revenue_log；外层 TX 在 `logDB`（fy_api_db）。**两个数据库**。无条件 DELETE 与 §3.4 retry / DLQ "markFailed retry_count++" 完全冲突——consumed 即 DELETE 那 retry_count 永远不递增；retry / dead_letter / DLQ replay 整套机制形同虚设。

**Verdict**：必须改写为：

```go
for _, r := range rows {
    if err := p.processOne(ctx, &r); err != nil {
        p.markFailed(tx, &r, err)  // UPDATE retry_count++, last_error
        continue
    }
    tx.Unscoped().Delete(&r)        // 物理删；只在成功时
}
```

`markFailed` 在 retry_count >= 10 时改 status='dead_letter'。要求 integration §3.3 / backend §5.4 同步重写。

**找茬 4**：跨实例 commit 顺序——bizDB.Create(revenue_log) 已 commit + logDB 外层 TX rollback（网络分区）→ outbox 行未删，下次 tick 重试 → bizDB UNIQUE violation → markFailed？还是 skip + DELETE？

**Verdict**：UNIQUE violation 在 processOne 中识别为"已处理"返 `nil`（不是 error），让外层 DELETE 执行——integration §3.3 `if isUniqueViolation(err) { return nil }` 已正确。OK。

**找茬 5**：区分 transient vs permanent error。`partner_pricing_rule` lookup 返 nil（数据问题）retry 10 次后才发现是浪费。

**Verdict**：integration §3.4 加分类——transient（5xx / timeout / temp DB error）走 retry；permanent（lookup nil / unique violation / 上游脏数据）直接 dead_letter。

### 5.3 链路 C：F-3 客户充值 saga

**找茬 6 (HIGH)**：integration §4.5.1 step 10 "POST /api/internal/user/topup ... + Idempotency-Key=intent.id"。intent.id 是 BIGINT。但 §2.1 OpenAPI 声明 `IdempotencyKey: { format: uuid }`——**违反契约**。

**Verdict**：topup_intent 加 `saga_id CHAR(36) UNIQUE NOT NULL`（UUIDv7），webhook 通过 (channel, out_trade_no) 反查 saga_id；idem-key 用 saga_id（UUID 格式）。

**找茬 7 (HIGH)**：webhook 路径无 idempotency middleware——webhook 没 JWT，permission middleware 跳过，§7.1 中间件链不一定包含 webhook handler。同 webhook 重传第二次会重复处理（依赖业务代码 SELECT FOR UPDATE 兜底，但缺一道防线）。

**Verdict**：webhook 入口独立 middleware，识别 (provider, out_trade_no) 双键作 idempotency key；命中直接 ack 200。backend §4.9 / §5.7 补设计。

### 5.4 DLQ / poison message

如上 §5.2 找茬 5：transient vs permanent 区分缺失。

---

## 6. NFR 落地审计

| NFR | 设计承载 | 评分 |
|---|---|---|
| 内部 P95 < 500ms | backend §16 性能预算 + 索引清单 | ✅ |
| 端到端 P95 < 800ms | integration §9 SLO + backend §16 | ✅ |
| outbox lag < 2s | integration §9.2 + backend §6 SLO `lag p95 < 1s` | ✅ |
| 月结 < 30 min | backend §5.5 progress_offset 续跑 + §16 | ✅ |
| 后台 ≥ 200 QPS | backend §16 连接池 | ✅ |
| MVP 99.5% / Phase 2 99.9% SLO | overview §6 多副本 + RDS HA；**ops runbook 不在文档集** | ⚠️ |
| RDS 备份 + KYC 5 年 | backend §13.4 / §9.4 OSS Archive | ✅ |
| 钱包 ACID | wallet_hold 双阶段 + 乐观锁 | ✅ |
| 强禁 XA | saga / outbox 替代 | ✅ |
| BOLA 100% 读端点 | backend §15.3 + frontend §16.6 + overview I-3.2 | ✅ |
| 可观测性 | backend §12 / integration §9 / frontend §15 trace_id | ⚠️ ops 层 SLS 跨索引未确定 |
| wallet drift | backend §6 + §15.2 I-W-4 | ✅ |
| outbox lag SLO | integration §9.2 + backend §12 | ✅ |
| 国际化 | frontend §10 i18next | ✅ |
| 可配置性 biz_setting | backend §3.15 缺 value_type | ⚠️ |
| 单元测试 70% / 前端 60% | backend §15.1 / frontend §16.6 | ✅ |
| 列表首屏 < 2s | frontend §14.4 LCP 目标 + cursor pagination | ✅ |
| Phase 1 99.5% MTBF | 仅 K8s 多副本 + readiness probe | ⚠️ |
| audit 完整率 100% | backend §10 + offline verifier daily | ✅ |

**总评**：NFR 承载基本到位；ops 层（SLS / mTLS mesh / K8s lease / region 隔离）一律延后到 Q11+，对 Phase 1 验收 §22.2 S-1..S-8 是潜在阻塞。

---

## 7. CRITICAL 问题清单

### CRIT-1 JWT 鉴权方式不闭环

- 现状：overview §3.4 / ADR-007 + backend §7.2 = Authorization header；frontend ADR-F5 = httpOnly cookie。
- 影响：实施时 backend middleware 无法读到 JWT。
- 整改：以 frontend ADR-F5 为准；overview ADR-007 重写；backend §7.2 改 cookie 抽取（见 §3.2）。

### CRIT-2 idempotency_record 同 TX 承诺与实现矛盾

- 现状：backend ADR-003 承诺"业务 TX 内 INSERT"；§8.1 middleware 在 c.Next 后写——根本不在业务 TX 内。
- 影响：业务 TX rollback 后 idempotency 没写，重放风险；或反之业务成功但 idem 写失败导致 retry 不命中 cache。
- 整改：middleware 仅 lookup + 409；service 在 bizDB.Transaction 闭包内 INSERT idempotency_record。重写 §8.1 代码。

### CRIT-3 outbox poller 消费 + DELETE 逻辑混乱（retry 形同虚设）

- 现状：integration §3.3 伪代码无条件 `tx.Delete(&r)`；§3.4 又说 markFailed retry_count；§5.4 backend "consumed 行直接 DELETE"——三处不闭环；如果遵守 §3.3 则 retry / DLQ 永远不走。
- 影响：失败的 outbox 行重复 retry 无收敛；DLQ replay 无效。
- 整改：明确 "成功 DELETE / 失败 markFailed"；poller 进程实现按此规范。

### CRIT-4 数据模型缺三张表（pipl_request / content_safety_event / password_reset_token）

- 现状：backend §14.3 phase 演进表点名，但 §3 无 DDL；§7.9 密码重置无 token 表。
- 影响：Phase 2A 实施时无 schema 可用。
- 整改：补 §3.23-§3.25 三张表 DDL，含状态枚举与 retention 策略。

---

## 8. HIGH 问题清单

| # | 问题 | 整改指向 |
|---|---|---|
| H-1 | partner_wallet.held_amount 在 overview ADR-012 / backend ADR-009 立场相反 | drop 字段；统一以 wallet_hold.amount sum 计算 |
| H-2 | audit_log.id 来源未在 overview 锁定 | overview ADR-006 显式注 "id 取自 audit_log_unsealed.id" |
| H-3 | saga_step UNIQUE 在 integration / backend 不一致 | integration §4.1 加 `gorm:"uniqueIndex:uk_saga_step;"` |
| H-4 | saga_step 状态枚举三处不全等（6/7/8 个）| 统一 8 状态：pending/in_progress/committed/compensated/failed/escalated/released_pessimistic + 终态 |
| H-5 | F-3 客户充值 saga Idempotency-Key 类型违反 OpenAPI uuid 约束 | topup_intent 加 saga_id CHAR(36) UNIQUE；webhook 反查 |
| H-6 | webhook 入口 idempotency 无独立 middleware | backend §4.9 / §5.7 加 webhook idempotency middleware（(provider, out_trade_no) 双键）|
| H-7 | partner 是否能为 customer 创建 token 路径未定义 | backend §4.4 增 partner verb `customer.create_api_key` 端点 OR 显式声明 "不允许，customer 自助" |
| H-8 | idempotency_record.response_cipher 16KB 对 export 类响应不够 | service 层 opt-out（不缓存大响应）OR LONGBLOB |
| H-9 | sealer single replica 不需 leader 但被列入 leader 表 | backend §6 区分 single-replica vs leader-required |
| H-10 | OpenAPI internal-api.yaml 物理路径未指定 | integration §11 锁定（建议检入 Fy-api repo）|
| H-11 | retry worker vs main goroutine race（5.1 找茬 2）未在 backend §8 处理 | retry worker 在 commit step 前先 SELECT idempotency_record |

---

## 9. MEDIUM 问题清单

| # | 问题 | 整改 |
|---|---|---|
| M-1 | biz_setting 缺 value_type | 加 value_type CHECK + value_schema |
| M-2 | notification_outbox 缺 (event_code, recipient, ref_id) UNIQUE | 加 UNIQUE 防重复发送 |
| M-3 | customer_partner_change_log 缺 status 列（场景 H 三方确认状态机）| 加 status enum |
| M-4 | idempotency_record 缺 trace_id | 加列 |
| M-5 | backend §6 cron 表 saga.retry.worker 1h cap 不显式 | 表注明 |
| M-6 | frontend cookie 数量从 1 个膨胀到 5 个 | 简化到 2 个：tnbiz_access (httpOnly) + tnbiz_csrf (non-httpOnly)；删 tnbiz_session |
| M-7 | consume_log_outbox 软删 vs 硬删 struct/DDL/行为不闭环 | integration §3.1 显式 "no DeletedAt" |
| M-8 | integration §3.4 markFailed 与 §3.3 unconditional DELETE 矛盾 | 同 CRIT-3 |
| M-9 | seat 表 polymorphic FK 无 invariant test | backend §15.2 加 I-Seat-1 |
| M-10 | KYC kyc_application 17 cipher/key_id 字段未抽象 | Phase 2A 重构成 PII 关联表 |
| M-11 | 内容安全 admin endpoints 在 backend §4.11 列但 frontend §3.3 admin 路由不齐 | frontend admin 路由补 `/content-safety/*` |
| M-12 | settlement freshness gate 阈值 60s 没在 backend §5.5 显式 | backend §5.5 补阈值 |
| M-13 | partner_wallet.balance 缺 CHECK 在 F-2 方案 A 决议后会有 bug 风险 | F-2 决议后 backend §3.3 加 CHECK balance >= 0 |

---

## 10. LOW

- L-1 partner.tier 0-9 命名约定 `partner_X_tier_Y` 中 Y 是 tier 名字还是 int？三处不一
- L-2 customer.group_name_in_fy_api VARCHAR(128) 与 fy_api_db.users.group 长度可能不一致
- L-3 backend §13.1 env 表缺 `MFA_ISSUER_NAME`、`SAGA_RETRY_CAP_HOURS` 等可调项
- L-4 frontend §12.4 cookie tnbiz_csrf SameSite=Lax → 改 Strict
- L-5 backend §3.13 audit_log.user_agent VARCHAR(512) 长 UA 可能截断
- L-6 frontend §16.4 E-13 BOLA 测试没指定具体 partner-A vs partner-B 数据隔离机制
- L-7 backend §16 性能预算未列 `/customer/billing/export`
- L-8 integration §1.9 LOC 修正与 round-2 architect §C-9 一致 ✅；但 PRD banner "约 200-300 LOC" 仍未撤销

---

## 11. 架构债务清单（被显式接受 / 未显式但隐含的"先这样"）

| ID | 债务 | 显式标记？ | 何时还 |
|---|---|---|---|
| D-1 | Phase 1 单层 markup（M3-13） | ✅ overview §3.2 / backend §17 | Phase 2A |
| D-2 | KYC Phase 1 stub | ✅ overview §8.1 / backend §17.1 | Phase 2A |
| D-3 | Cron leader Redis SETNX → K8s Lease | ✅ ADR-008/011 | Phase 2 |
| D-4 | KMS DEK rotation Phase 1 不做 | ✅ backend §17.1 | Phase 2A |
| D-5 | sandbox / demo M9-04 Phase 3 | ✅ overview §8.4 | Phase 3 |
| D-6 | F-1 per-model markup schema-only | ✅ integration §1.4.4 | Phase 2A |
| D-7 | F-2 partner_debt schema 已写未挂业务 | ⚠️ 半显式 | Phase 2A |
| D-8 | partner-api 报表跨库 JOIN LOG_DB 拆分 fallback HTTP | ✅ overview §4.2 + integration §6.3 | LOG_DB 拆分前 |
| D-9 | kyc_application 17 PII cipher 字段未抽象 | ❌ 无标记 | M-10 |
| D-10 | webhook idempotency 没独立 middleware | ❌ 无标记 | H-6 |
| D-11 | biz_setting 无 value_type 强制 | ❌ 无标记 | M-1 |
| D-12 | tnbiz_session cookie 与 ADR-007 角色模糊 | ❌ 无标记 | M-6 |
| D-13 | backend §6 cron leader vs single-replica 区分不清 | ❌ 无标记 | H-9 |
| D-14 | F-3 客户充值 saga idem-key 类型违反 OpenAPI 约束 | ❌ 无标记 | H-5 |
| D-15 | sealer 200ms tick lag 无 metric `audit_unsealed_pending_count` | ❌ 无标记 | 加观测 |
| D-16 | upstream-sync 月度仪式 vs OVERLAY.md 行数控制无 budget | ⚠️ overview A-1 / integration §1.9 但无月度 cap | continuous |

**12 条隐含债务** 必须在 round-2 显式标记，否则会变成 Phase 2/3 时的"考古发现"。

---

## 12. 修订指令（可机器执行的 patch list）

每条指明"改哪份文档的哪节，怎么改"。

### 12.1 overview/00-architecture-overview.md

1. **§3.4 footer + §9 ADR-007**：重写为 "JWT 走 httpOnly cookie `tnbiz_access`；`tnbiz_csrf` 走 non-httpOnly cookie 做 double-submit；不走 Authorization header"；删除 `tnbiz_session` 引用，改 "tnbiz_csrf 仅做 CSRF"。
2. **§9 ADR-006**：补 "sealer 严格按 unsealed.id ASC 顺序消费；audit_log.id 直接复用 unsealed.id；sealer 是单 replica（不需 leader 选举）"。
3. **§9 ADR-012**：把"倾向 drop"升级为"决定 drop"；引用 review §3.1 第 5 条 verdict。
4. **§4.3 关键 invariant**：补 "idempotency_record TTL ≥ saga max wall-clock；启动期 startup assertion fail panic"。
5. **§4.4 trace_id 字段持久化**：补 idempotency_record.trace_id 列。
6. **§7 NFR 表**：增 "JWT cookie + CSRF" 行映射到 frontend ADR-F5。
7. **新增 §A 架构债务清单**：把 16 条债务全列入 + Phase 标签。

### 12.2 integration-design.md

8. **§3.1 ConsumeLogOutbox struct**：注释 "// no gorm.DeletedAt; physical DELETE only"；§3.3 伪代码改 `tx.Unscoped().Delete(&r)` 且仅在 processOne 成功时调用。
9. **§3.3 / §3.4 retry / DLQ 逻辑统一**：成功 → DELETE；失败 → markFailed (retry_count++, last_error)；retry_count >= 10 → status='dead_letter'；区分 transient vs permanent error，permanent 直 dead_letter。
10. **§4.1 SagaStep struct**：加 `gorm:"uniqueIndex:uk_saga_step;"`；注 "retry 用 INSERT ... ON DUPLICATE KEY UPDATE"。
11. **§4.1 status 枚举**：与 backend §3.17 同步 8 状态；§4.2 状态机增对应 transitions。
12. **§4.5 客户充值 saga**：idem-key 改 UUIDv7 saga_id（不用 BIGINT topup_intent.id）；topup_intent 加 saga_id 列。
13. **§5 跨服务幂等**：新增 §5.5 webhook idempotency 单独段（(provider, out_trade_no) 双键作 idem-key）。
14. **§7 freshness gate**：阈值 60s 显式 + 重试策略表格。
15. **§11 spec 物理路径**：锁定 `internal-api.yaml` 检入 Fy-api repo 路径。

### 12.3 backend-design.md

16. **§3.3 partner_wallet**：drop `held_amount` 列；§5 wallet 业务用 `SUM(wallet_hold WHERE status='held')` 计算 available；ADR-009 翻判。
17. **§3.13 audit_log**：补 schema 注释引用 §10 invariant；audit_log_pii 与 audit_log 不加 FK 但 service 层维护。
18. **§3.15 biz_setting**：加 `value_type VARCHAR(16) CHECK ('string','int','bool','json','duration')` + `value_schema TEXT`。
19. **§3.16 idempotency_record**：加 `trace_id VARCHAR(64)` 列；response_cipher 改 LONGBLOB 或 service 层支持 opt-out。
20. **§3.18 notification_outbox**：加 `UNIQUE(event_code, recipient, ref_id)`。
21. **§3.20 customer_partner_change_log**：加 `status VARCHAR(32) CHECK IN ('pending_3way','partner_a_released','partner_b_adopted','staff_approved','completed','aborted')`。
22. **§3.21 topup_intent**：加 `saga_id CHAR(36) UNIQUE NOT NULL`；webhook 通过 (channel, out_trade_no) 反查 saga_id。
23. **新增 §3.23 pipl_request / §3.24 content_safety_event / §3.25 password_reset_token**：补 DDL。
24. **§4.4 partner 路由**：增 `POST /partner/customers/{id}/api-keys` OR 显式声明 partner 不能为 customer 开 token，要求 customer 自助。
25. **§4.9 webhook**：加 webhook idempotency middleware 设计 + (provider, out_trade_no) 双键。
26. **§5.4 outbox**：与 integration §3.3/§3.4 重写后保持一致；明确"成功 DELETE / 失败 markFailed"。
27. **§6 cron 表**：区分 single-replica（audit-sealer / kek-rotator）vs leader-required（settlement-runner）；audit-sealer 删除 leader 选举要求。
28. **§7.2 JWT extract**：改 `extractAccessFromCookie(c)`；cookie name = `tnbiz_access`；migration 期间可读 Authorization header（feature flag）。
29. **§8.1 idempotency middleware**：改 "middleware 仅 lookup + 409；service 层在业务 TX 内 INSERT idempotency_record"；给出新代码骨架。
30. **§8.2 saga retry**：commit step 前显式 SELECT idempotency_record + version 检查（防 race）。
31. **§14.3 phase 演进表**：补 §3.23-§3.25 三表 phase 标签。
32. **§19 起草人 align 清单**：根据本 review §3.1 verdict 全部回填，标 ✅ resolved。
33. **§16 性能预算**：补 `/customer/billing/export` p95（PDF/CSV 导出）。

### 12.4 frontend-design.md

34. **§12.4 Cookie 表**：删除 `tnbiz_session` 行；保留 `tnbiz_access`、`tnbiz_refresh`、`tnbiz_csrf`、`tnbiz_locale`。
35. **§12.4 tnbiz_csrf**：改 `SameSite=Strict`。
36. **§3.3 admin 路由**：补 `/content-safety/*` / `/dlq/outbox/*` / `/wallet-drift` 路由。
37. **§16.4 E2E E-13 BOLA**：明示 partner-A / partner-B fixture 数据；测试断言 redirect to 404。
38. **§19 ADR-F5**：链接到 overview §9 ADR-007 修订条目作为 source of truth。
39. **§5.4 错误码 toast 映射**：与 backend §11 错误段位完全同步（增 `BIZ_AUTH_STEP_UP_REQUIRED` / `BIZ_PERM_DUAL_CONTROL_REQUIRED` 等漏项）。

### 12.5 跨文档协调

40. **OVERLAY.md (Fy-api repo)**：在 Phase 1 PR append B-8..B-14 条目（与 round-2 architect §C-8 一致）。
41. **CI gate**：增 doc-lint job，`grep "Authorization: Bearer" docs/` 应 0 命中（迁移到 cookie 后）。

---

## 13. 验收建议

把 4 条 CRITICAL + 11 条 HIGH + 13 条 MEDIUM 在 v0.2 修订（≤ 1 周）；LOW 与债务清单作为 Phase 1 sprint 内 follow-up。完成后再做 round-2 review 验证 cross-doc matrix 中 ❌ → ✅。

**当前 verdict**：**NEEDS REWORK**。CRITICAL=4 / HIGH=11，远超 round-2 通过门槛（CRITICAL=0 / HIGH ≤ 3）。集成层已扎实，问题集中在三份消费侧文档与契约的不闭环 + 数据模型 phase 演进缺表 + saga / outbox 失败路径设计破绽。

---

> 本 review ≥ 2000 字；按规则书严格 challenge 不放水；引用具体行号 / 章节；不做 PM / 安全 / 合规视角 review（其他 reviewer 并行做）。
