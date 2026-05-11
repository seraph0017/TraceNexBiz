# Code Round-1 Review — Architect (independent reviewer)

> Date: 2026-05-10
> Reviewer: Architect（独立第三方架构 reviewer，与 W0..W1g 各 agent / code-reviewer / Security / Compliance / Backend Architect 并行；也是 dev-round-1 / dev-round-2 同一人）
> Scope: `apps/partner-api/` 全部 117 个 Go 文件 + 11 张 up.sql migration（共 865 行 DDL） + `apps/partner-web-{storefront,customer,partner,admin}/src/` 共 162 个 TS/TSX + `packages/{api-client,ui-kit,i18n,config}` + `Fy-api/` 七条 OVERLAY (B-12..B-18)
> 输入文档：`reviews/dev-round-2/02-Architect-review.md`（PASS-CONDITIONAL）+ HANDOFF-W0/W1a/W1b/W1c/W1g + dev doc v1.2
> Pass gate（沿用 dev round-2）：**0 CRITICAL / 0 HIGH** —— 比 dev round 更紧（代码层比文档层有更具体的真相）

---

## 1. 执行摘要 + Verdict

三句话：

1. **DDL 层已基本"踩在"dev doc v1.2 的字面落点上**：11 张 up.sql / 35+ 表的 DDL 全部建出，包含 `partner` / `customer` / `partner_wallet` / `wallet_hold` / `partner_wallet_log` / `partner_debt` / `partner_pricing_rule` / `revenue_log` / `settlement` 4 张 / `kyc_application` 17 cipher 字段 / `seat` / `invoice_*` / `audit_log_unsealed/audit_log/audit_log_pii` / `staff` / `biz_setting` / `idempotency_record` / `saga_step` / `password_reset_token` / `notification_outbox` / `consent_log` / `topup_intent` / `content_safety_*` / `pia_report` / `pipl_complaint` / `pipl_request`。dev round-2 留下的 5 项 cosmetic（M-2/M-4/M-6/M-13/N-2）落地度 = 4/5：M-2 ✅（`uk_notif_dedup`, 008:55）/ M-4 ✅（`idempotency_record.trace_id`, 007:49）/ M-6 ✅（前端 4 个 cookie，无 `tnbiz_session` 字面）/ N-2 ⚠️（`password_reset.purge` cron 仍未在 backend §6 表登记）/ M-13 ❌（`partner_wallet.balance` 仍**无** `CHECK >= 0`，002:5-17 仅 `chk_wallet_amounts` 检查 `paid_out_total >= 0`）。换句话说，dev round-2 cosmetic 基本兑现，但 M-13 在 W1b DDL 落地时被遗漏，与 dev round-2 §10 cosmetic 表第 5 行 + §10 附加项第 2 行的指令不符。
2. **Saga 框架（W1b）符合 §9.4 / integration §4 的核心 invariant，但实际工作量不到一半**：UUIDv7 ✅（`internal/saga/saga.go:126-132`） / `UNIQUE(saga_id, step_name)` ✅（007:74） / `MaxAttempts=30` + `MaxWallClock=1h` + `BackoffMax=5min` ✅（saga.go:55-62） / 7 状态枚举与 chk_saga_status DDL 完全对齐 ✅ / immutable snapshot ✅（instance.go:186-233 `snapshotStarting/Failed/Committed/Compensated` 全部返回新结构体不 mutate 入参） / payload PII scrubber 钩子 ✅（instance.go:209-217）。但是：(a) 补偿对称性**有缺陷**——saga_allocate/service.go:128-159 的失败补偿用 `_, _ = sg.Compensate(...)`忽略错误而**不进 retry / DLQ**，三段补偿任意一段失败，资金一致性即被破坏，CRITICAL； (b) DLQ + poison message 处理在 outbox/consumer.go:208-219 已有，但 saga 端 retry worker (`Sweep`) 实际**只判 escalation 不重做 fn**（orchestrator.go:64-93 注释明说"W1b 范围只保证 escalation 决策可独立运行"），意味着 saga 失败 step 永远不会被自动 retry，HIGH； (c) `idempotency_record` 同 TX Insert 路径在 service 层**根本未被任一 saga 调用**——grep 全仓 `idemRepo.Insert(tx,` 在 service 包内零命中；dev round-2 CRIT-2 字面修复了 middleware，但 service 层"在业务 TX 闭包内 Insert"这一**对称必然**条件根本没人执行，HIGH。
3. **Fy-api OVERLAY B-12..B-18 全 7 条文件级落地 ✅，但与 partner-api 的 HMAC 契约存在严重 header 名 + canonical 形式冲突**：partner-api `internal/infra/fyapi/client.go:104-119` 写 `X-Auth-KeyId / X-Auth-Timestamp / X-Auth-Nonce / X-Signature` 4 头 + canonical = `method\npath\nquery\nsha256(body)\nts\nnonce`；而 Fy-api `middleware/internal_auth.go:36-45` 期望 `X-Tnb-Key-Id / X-Tnb-Timestamp / X-Tnb-Nonce / X-Tnb-Signature` 4 头 + canonical = `METHOD\nPATH\nTIMESTAMP\nNONCE\nKEY_ID\nsha256(body)`。**两边 header 名前缀不同（`X-Auth-` vs `X-Tnb-`）、签名字段顺序不同（partner-api 把 query 当独立段，Fy-api 没有 query 段）、字段集合不同（partner-api 多了 query，Fy-api 把 KEY_ID 放进了 canonical 末尾）**。这是端到端契约的根本性断裂，**CRITICAL**——任何一次真正的 partner-api → Fy-api 调用都会被 HMAC 验签 401。

**Verdict（严格 0 CRITICAL / 0 HIGH）：FAIL**

总账：CRITICAL = 2（HMAC 契约、saga 补偿失败静吞）+ HIGH = 5（saga retry sweep 不重做 fn、idempotency 同 TX Insert 在 service 层零调用、middleware/auth.go 全部仍是 W0 stub `c.Next()` 直通、fyapi.Client 全部 endpoint 方法 `not implemented`、dev-round-2 cosmetic M-13 漏改）+ MEDIUM = 9 + LOW = 5。详见 §7。

---

## 2. 跨文档 vs 跨代码契约对齐矩阵（≥ 20 项）

下表把 dev round-2 矩阵的 20 项契约逐条与代码实际状态对齐。`✅` = 文档与代码字面一致；`⚠️` = 部分一致或 stub；`❌` = 文档/代码冲突或代码缺失。

| # | 契约点 | dev round-2 文档侧 | code round-1 代码侧 | 引用 | 结论 |
|---|---|---|---|---|---|
| 1 | JWT 载体 cookie `tnbiz_access` | ✅（ADR-007 v0.2） | ⚠️ | `middleware/auth.go:10`常量定义；但 `JWT()` middleware 函数本身是 W0 stub（auth.go:48-54 直通 `c.Next()`）。前端 `packages/api-client/src/client.ts:11`+11 个 web client 写入正确，但**rendering 端到端无法工作** | ⚠️ |
| 2 | `tnbiz_session` 删除（仅 4 cookie） | ✅（M-6 cosmetic 已 cleanup） | ✅ | `grep tnbiz_session apps/partner-web-*/src` 零命中；`web 4 app + handler/w1a_auth.go:152-154` 仅写 access/refresh/csrf | ✅ |
| 3 | partner 不为 customer 开 token，customer 自助 | ✅（H-7 已收口） | ✅ | `handler/w1a_routes.go:60-66` partner 路由组**无** `/customers/:id/api-keys`；customer 路径在 `c := r.Group("/customer")` 但本轮也没出现 api-keys 路由（W1a 范围内未实现，但**没有错误的 partner 端为 customer 开 token 路径**） | ✅ |
| 4 | audit_log.id ← unsealed.id 拷贝 | ✅（ADR-006 v0.2） | ✅ | `migrations/006_audit_log.up.sql:23-25` 注释 `audit_log.id 非 AUTO_INCREMENT；由 sealer 把 unsealed.id 原样拷贝过来`；`internal/audit/sealer.go:154-159` 字面把 `r.ID` 赋给 SealedRow.ID 后 AppendSealed | ✅ |
| 5 | saga_step UNIQUE(saga_id, step_name) | ✅ | ✅ | `migrations/007_staff_biz_setting_idem_saga.up.sql:74` `UNIQUE KEY uk_saga_step (saga_id, step_name)` + saga.go:7 invariant 注释 + memrepo.go:18 内存结构按 (saga_id, step_name) 复合 key | ✅ |
| 6 | partner_wallet.held_amount drop | ✅（ADR-012 v0.2） | ✅ | `migrations/002_wallet.up.sql:5-17` `partner_wallet` 字段集合 = `id/partner_id/balance/paid_out_total/version/created_at/updated_at` —— 无 `held_amount` 列；hold 由 `wallet_hold` 表（002:19-38）独立计算 | ✅ |
| 7 | partner_debt 方案 A | ✅（ADR-010） | ✅ | `migrations/002_wallet.up.sql:65-80` 表已建；status 枚举 `open/clearing/cleared/written_off` 与文档一致 | ✅ |
| 8 | outbox poller DELETE/UPDATE 统一（ackOne soft-mark + outbox.purge 30d 物理 DELETE） | ✅ | ⚠️ | `internal/outbox/poller.go:25-31` `PurgeOnce` 是占位 stub `return 0, nil`；`internal/outbox/consumer.go:194-230` `TickOnce` 调用 `c.source.Ack/Nack` 但 `Source` 是 interface 没有真实实现；30d cron 接线悬空。**逻辑契约符合，工程未完成** | ⚠️ |
| 9 | idempotency_record 同 TX Insert（service 层） | ✅（CRIT-2 字面修复） | ❌ | `internal/middleware/idempotency.go:50-56` 是 W0 stub `c.Next()`；`internal/idempotency/idempotency.go:22` 暴露 `Insert(tx *gorm.DB, rec *domain.IdempotencyRecord) error` 接口但**全仓 service 包零调用**；saga_allocate / saga_topup / saga_refund / payment / customer 的所有 mutation handler 都没有"业务 TX 闭包内调 idemRepo.Insert(tx,...)"的实际代码。dev round-2 CRIT-2 修复**仅停留在文档层** | ❌ |
| 10 | trace_id 字段持久化（idempotency_record） | ⚠️→ cosmetic v1.0 已补 | ✅ | `migrations/007_staff_biz_setting_idem_saga.up.sql:49` `trace_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'v1.0 cosmetic #1 / M-4'` | ✅ |
| 11 | consume_log_outbox 软删 vs 硬删 | ✅（integration §3.1） | ⚠️ | `internal/outbox/consumer.go:73-78` `Source.Ack/Nack` 接口签名对齐"软删 status=consumed + consumed_at"语义；但 `Source` 实现尚未在仓库中（`grep -l "func.*Pull.*ctx.*region" internal/outbox/` 无具体实现）；30d 物理 DELETE 同 #8，cron stub | ⚠️ |
| 12 | partner-api → Fy-api Idempotency-Key 透传 | ✅（integration §5.2） | ⚠️ | `internal/infra/fyapi/client.go:114-115` 字面 `httpReq.Header.Set("Idempotency-Key", req.IdempotencyKey)`；Fy-api `middleware/internal_idempotency.go:20+29` 字面 `hdrIdempotencyKey = "Idempotency-Key"`。**header 名一致**；但 saga_allocate/service.go:135 / saga_topup/service.go:191 调用 `s.fyapi.TopupCustomer(ctx, fyUserID, amount, sagaID, traceID)` —— 而 fyapi.Client 上**没有 TopupCustomer 方法**（client.go:150-180 只有 5 个 stub 方法 `CreateUser/SetGroupRatioOverride/AdjustQuota/CreateToken/GetUsage`），即 saga 服务层依赖一个**未实现的接口**，集成 wire 不通 | ⚠️ |
| 13 | OpenAPI internal-api.yaml 物理路径 | ⚠️→ cosmetic | ❌ | `apps/partner-api/openapi/admin.yaml` 存在；`internal-api.yaml` 仍未在 Fy-api 或 TraceNexBiz 任一位置；H-10 cosmetic 失约 | ❌ |
| 14 | settlement freshness gate 阈值 60s | ⚠️→ cosmetic | ⚠️ | `internal/outbox/consumer.go:46` `FreshnessGateDefault = 5 * time.Minute`；与 dev round-2 §3 矩阵 #14 给的"60s"严重不符（**5 倍宽**）；§5.5 mermaid `outbox lag p95 < 60s` 在代码里被默认放宽到 5 min，HIGH | ❌ |
| 15 | settlement-runner leader vs single-replica | ✅ | ⚠️ | `internal/audit/sealer.go:74-91` 提供 `LeaderLock` 接口 + `AlwaysLeader` stub；`pkg/leader/leader.go` 已存在但 `internal/service/settlement/runner.go` 仅 stub；W1c 后续 | ⚠️ |
| 16 | wallet drift admin dashboard | ⚠️→ T-15 债务 | ⚠️ | `apps/partner-web-admin/src` 无 wallet-drift 路由；接受为债务但代码无 TODO 标 T-15 | ⚠️ |
| 17 | mTLS K8s STRICT | ✅ | n/a | 不在代码层，K8s 层 ops 配置；本轮跳过 | n/a |
| 18 | LOG_DB 跨库 reporting fallback | ⚠️→ ops debt | ⚠️ | `internal/infra/db/db.go` 双 DB（bizDB / fyDB）已开；reporting 回退路径 W1c 未实现 | ⚠️ |
| 19 | by-idem-key KeyId 多 cmd 共享 | ⚠️→ ops debt | n/a | ops topology Q11+；本轮跳过 | n/a |
| 20 | F-3 客户充值 saga_id = UUIDv7 string | ✅ | ✅ | `migrations/009_payment.up.sql:14` `saga_id VARCHAR(64) NOT NULL COMMENT 'UUIDv7 字符串；用作 Idempotency-Key'` + `uk_topup_saga_id` UNIQUE；`internal/service/saga_topup/service.go:73-86` Validate 强制 `saga.IsValidUUIDv7(r.SagaID)`；orchestrator.go:39 `if !IsValidUUIDv7(idemKey)` 拒绝非 v7 | ✅ |
| 21（新加）| Fy-api ↔ partner-api HMAC header / canonical | ✅（dev doc 字面写"4 头 X-Auth-* + canonical 6 段"） | ❌ **CRITICAL** | partner-api client.go:104-119 写 `X-Auth-*` + canonical(method\npath\nquery\nsha256(body)\nts\nnonce)；Fy-api internal_auth.go:36-45 校验 `X-Tnb-*` + canonical(METHOD\nPATH\nTIMESTAMP\nNONCE\nKEY_ID\nsha256(body))。两端**不可能验签通过** | ❌ |
| 22（新加）| BackoffFor 指数退避语义 | ✅（"1=2s, 2=4s, ..."） | ❌ | `saga.go:155-167` `BackoffFor(1) = 2s * 2 = 4s`（注释写 attempts=1 应得 2s）；off-by-one。saga_test.go 未覆盖 attempts=1 这一基线 | ❌ |
| 23（新加）| `MaxLastErrorLen` 与 `saga_step.last_error TEXT` 类型一致 | n/a（doc 未约束） | ✅ | `outbox/consumer.go:48` 4000 限制；`saga/instance.go:215` truncate 到 4000；MySQL `TEXT` 上限 65535 字节，远裕量。OK | ✅ |

**矩阵小结**：✅ 12 / ⚠️ 6 / ❌ 5（含 2 条新发现 CRITICAL/HIGH） / n/a 2。**远未达到 dev round-2 期望的 ≥ 19 ✅**——即便扣除 2 条 ops debt 项，⚠️ 已 6 条、❌ 5 条，本质是"DDL 层和单测层符合契约，但 wire / middleware / endpoint 实现层严重 stub 化"。

---

## 3. 数据模型审计（实际 DDL）

### 3.1 表清单（11 个 up.sql / 共 33 张表）

| migration | 表 | LOC | 评语 |
|---|---|---:|---|
| 001 | `partner` / `customer` / `invitation_code` / `customer_partner_change_log` | 108 | partner 23 列含 4 处 cipher/HMAC PII；外键集合合理；`chk_partner_status` / `chk_partner_tier 0..9` / `chk_partner_tax_status` 三个 CHECK 严格符合 PRD §14.1 + Compliance HIGH-1。**亮点**：001:11 `kyc_type TINYINT` + `kyc_status TINYINT` 用最小数字类型；001:23 `tax_status` 5 枚举严格闭合 |
| 002 | `partner_wallet` / `wallet_hold` / `partner_wallet_log` / `partner_debt` | 80 | **缺陷**：002:5-17 `partner_wallet` 仅 `chk_wallet_amounts CHECK(paid_out_total >= 0)`；balance 没 CHECK ≥ 0。dev round-2 §10 cosmetic 表第 5 行 + 附加项第 2 行明确要求"加 `chk_wallet_balance CHECK (balance >= 0)`"——M-13 在 v1.0 cosmetic 落地时被遗漏。Phase 2A partner_debt 启用时若 balance 跌破 0 会形成 -∞ 余额。**HIGH** |
| 003 | `partner_pricing_rule` / `revenue_log` | 55 | `customer_id_canon` / `model_name_canon` / `tier_name_canon` 三个生成列 + `uk_pricing_rule_canon` 复合 UNIQUE 是非常聪明的写法（NULL 用 `*` 占位绕开 MySQL UNIQUE NULL 重复问题）。`chk_pricing_markup BETWEEN 1.0 AND 5.0` 与 PRD §8.6 一致。`revenue_log.uk_revenue_log (fy_api_log_id, occurrence)` 严格幂等 |
| 004 | `settlement` / `settlement_item` / `settlement_run` / `settlement_config_change_log` | 78 | `settlement.uk_settlement_period (period)` 防重；`progress_offset` 续跑字段在；`settlement_run.lease_expires_at` 与 Redis SETNX 续约对齐。**轻微**：`settlement.timezone` 默 'Asia/Shanghai' 硬编码，海外副本（如 SG）上线时需 ops 在 biz_setting 覆盖 |
| 005 | `kyc_application` / `seat` / `invoice_title` / `invoice_application` / `red_flush_request` | 136 | `kyc_application` 17 PII cipher 字段（每对 cipher + key_id + 部分 + blind_index）—— 与 dev round-2 M-10 的"接受为 Phase 2A 抽象债务"一致，**未提前抽**（OK，T-8 债务）；005:46-49 `idx_kyc_purge_*` 三个时间索引为 PIPL 删除作业准备齐全；005:73-88 `invoice_title.is_default TINYINT(1)` 缺 `UNIQUE(owner_type, owner_id) WHERE is_default=1` 条件唯一约束（MySQL 不支持，但可用触发器或应用层），允许同 owner 多个 default = 1，BUG-prone，MEDIUM |
| 006 | `audit_log_unsealed` / `audit_log` / `audit_log_pii` | 57 | `audit_log` PRIMARY KEY 显式 `BIGINT NOT NULL`（无 AUTO_INCREMENT）+ 注释 23-25 严格落 ADR-006 v0.2；4 个 `idx_audit_*` 覆盖审计常用查询。`audit_log_pii.diff_cipher VARBINARY(65535)`——**MySQL VARBINARY 最大 65535 字节是表行总长度上限，不是单列限制；这一行声明在 InnoDB row format DYNAMIC/COMPRESSED 下虽可工作，但在 row format COMPACT 下会被压到 ~64KB-行其他字段空间，引入 row size too large 错误潜在风险**。建议改为 `MEDIUMBLOB` 或 `LONGBLOB`，**MEDIUM** |
| 007 | `staff` / `biz_setting` / `idempotency_record` / `saga_step` / `password_reset_token` | 100 | `idempotency_record.response_cipher VARBINARY(16384)` 仍按 dev round-2 H-8 MOOT 决议保留 16KB（acceptable Phase 1）；`biz_setting.value_type CHECK ('plain','secret_ref')` ✅；`saga_step.uk_saga_step (saga_id, step_name)` ✅；**轻微**：007:38-55 `idempotency_record` 没有 partition by `actor_type, actor_id` —— Phase 2A 高 QPS 时表会成 1G+，需提前规划月分区，纳入 LOW |
| 008 | `ticket` / `ticket_reply` / `notification_outbox` / `consent_log` | 76 | M-2 cosmetic ✅ 落地：008:44 `ref_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT 'v1.0 cosmetic #3 / M-2 防重复推送'` + 008:55 `UNIQUE KEY uk_notif_dedup (event_code, recipient, ref_id)`。`consent_log.chk_consent_type` 7 枚举与 PIPL §13 + GB/T 35273 同意分类完全对齐 |
| 009 | `topup_intent` | 25 | F-3 saga_id UUIDv7 字符串契约 ✅ 落地（009:14 + 009:21 `uk_topup_saga_id`）；`uk_topup_channel_trade (channel, out_trade_no)` 双重防重；状态机 6 态 chk 严格 |
| 010 | `content_safety_event` / `content_safety_report` / `pia_report` / `pipl_complaint` / `pipl_request` | 114 | 5 张合规表全部建出；`pipl_request` 字段集（actor + state + deadline + completed_deadline + audit_log_id + trace_id）符合 PIPL §44-§47 + dev round-2 §5 评语完整。**轻微**：010:91 `pipl_request` 缺 `withdrawn_at TIMESTAMP NULL` 列以应对用户撤回（dev round-2 §5 已建议 Phase 2A 补，本轮未补，acceptable）|
| 011 | `biz_setting seed` | 36 | seed 操作 |

### 3.2 字符集 / 字符序

11 张迁移 100% 用 `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`，与 dev doc backend §3 一致。**OK**。

### 3.3 软删除 / 时间戳

`partner` / `customer` / `kyc_application` / `invoice_application` / `partner_pricing_rule` / `staff` / `ticket` 全部带 `deleted_at TIMESTAMP(3) NULL`；流水表（`partner_wallet_log` / `revenue_log` / `audit_log_unsealed` / `audit_log`）正确**不**带 `deleted_at`（append-only）。语义清晰。

### 3.4 注释覆盖

98% 表带 `COMMENT='...'` + 大部分敏感列带行内 COMMENT。**优**。

### 3.5 漏改清单（dev round-2 cosmetic 残余）

| 漏改 | 严重度 | 说明 |
|---|---|---|
| `partner_wallet.balance CHECK >= 0` | **HIGH** | 002:5-17 仍无；dev round-2 §10 必做项第 5 + 附加项第 2 字面要求 |
| backend §6 cron 表 `password_reset.purge` 行登记 | LOW (N-2 残余) | 代码层无 cron 注册；W1c 范围未补 |

---

## 4. Saga / 幂等 / Outbox 端到端走查

### 4.1 UUIDv7 ✅

`internal/saga/saga.go:126-132` 用 `github.com/google/uuid` 的 `uuid.NewV7()`；`IsValidUUIDv7` 在 saga.go:144-150 严格校验 `id.Version() == 7`；orchestrator.go:39 + saga_allocate/service.go:45 + saga_topup/service.go:73 三处入口都做 hard-fail 校验。**OK**。

### 4.2 UNIQUE(saga_id, step_name) ✅

DDL 007:74；服务侧 saga.go:7 注释 + memrepo.go:18 数据结构 + instance.go:97-100 GetStep 路径全部按"按 (saga_id, step_name) 复合 key 查"语义实现。**OK**。

### 4.3 状态机闭合 ✅

7 状态：`pending / in_progress / committed / compensated / failed / escalated / released_pessimistic`，DDL chk（007:75）+ Go 常量（saga.go:32-40）+ instance.go transition 全集闭合。 **OK**。

### 4.4 补偿对称性 — **CRITICAL FINDING**

`internal/service/saga_allocate/service.go:106-166` Run 方法 5 步：deduct → hold → fyTopup → commit → log。失败补偿出现在 4 处：

```go
// L128-132 hold 失败：补偿 deduct
_, _ = sg.Compensate(ctx, StepDeduct, func(_ *gorm.DB) (any, error) {
    return nil, s.wallet.Refund(ctx, req.PartnerID, req.Amount, req.SagaID)
})

// L138-144 fyTopup 失败：补偿 hold + deduct
_, _ = sg.Compensate(ctx, StepHold, ...)
_, _ = sg.Compensate(ctx, StepDeduct, ...)

// L149-158 commit 失败：补偿 fyTopup + hold + deduct
_, _ = sg.Compensate(ctx, StepFyTopup, ...)
_, _ = sg.Compensate(ctx, StepHold, ...)
_, _ = sg.Compensate(ctx, StepDeduct, ...)
```

**所有 7 处 `_, _ = sg.Compensate(...)` 都丢弃 error**。意味着：

- `wallet.Refund` 因 wallet version mismatch 失败 → 该笔 deduct **永远不会被补回**，partner balance 卡在已扣状态，但 customer 没拿到额度。
- `fyapi.RefundCustomer` 调用 5xx → customer 拿到了额度，partner balance 已扣，但补偿没生效。
- saga_step 行可能 stuck 在 `committed`（前一步 commit 完成）但实际业务态已破碎。

**没有 retry sweep 把这些失败的 compensate 重做**（参见 §4.5）。这是**资金一致性 CRITICAL bug**——saga_allocate 是 PRD §8.4 wallet 三阶段的核心 verb，本轮代码就是在该 verb 上把补偿写成"best-effort fire-and-forget"。

dev doc backend §5.3 + integration §4.4 明确要求"补偿失败必须 escalate + retry worker 接管"，code 层完全没落地。

**修复指令**：
1. 把 `_, _ = sg.Compensate(...)` 改成 `if _, err := sg.Compensate(...); err != nil { return fmt.Errorf("compensate %s failed: %w", step, err) }`，让上游 caller 知道补偿失败。
2. 失败的 compensate 必须**立即写 saga_step.status='failed'** 并触发 retry worker（参见 §4.5 的 Sweep 重做修复）。
3. 加 e2e test：`TestAllocate_RefundFailsAfterDeduct_RetryRecovers` —— mock wallet.Refund 第一次失败、第二次成功，断言 partner balance 最终回到原值。

### 4.5 Retry / DLQ — **HIGH FINDING**

`internal/saga/orchestrator.go:64-93` `Sweep` 方法注释字面承认：

> Sweep 不实际重做业务 fn —— 仅判断 escalation。重做需调用方注册 saga handler（W1a 后续）。
> W1b 范围只保证 escalation 决策可独立运行，便于 chaos test。

意思是：当前 retry sweep**只把 attempts >= 30 或 wall-clock >= 1h 的 step 标 escalated，不对 in_progress / failed step 做任何重做**。配合 §4.4 的 `_, _ = Compensate` error 静吞，效果是：

- 失败 step → 永不 retry（Sweep 只关心 escalation）
- 失败 compensate → 永不被察觉
- escalation 后 → 没有重做路径，必须人工 dual-control force-resolve

dev doc integration §4.3.2 + backend §8.2 字面写"retry worker 在 backoff 退避后重做 fn"。本轮 W1b 实际范围**比文档少做了一半核心工作**——这一缺口与 dev round-2 矩阵 #19 / T-9（saga retry worker 内嵌 cmd/api）债务并不重叠：T-9 谈的是 worker 是否拆 cmd，本轮代码是 worker **业务逻辑都没写**。

**HIGH**：必须在 Phase 1 W1c 落地"Sweep 调用注册的 saga handler 重做 fn"；如果做不到，应当在 PRD / dev doc 明确"Phase 1 不支持自动 retry，所有失败 step 都靠人工 force-resolve"——但这与 §9.3 SLA 冲突。

### 4.6 Idempotency Record 同 TX Insert — **HIGH FINDING**

dev round-2 CRIT-2 verdict 字面写："middleware 仅 SELECT；service 在 bizDB.Transaction 闭包内调 idemRepo.Insert(tx, ...)；invariant 三连：grep 反断言 + AST analyzer + e2e panic-rollback"。

代码实际：

- `internal/middleware/idempotency.go:50-56` 是 W0 stub，函数体只有 `c.Next()` + TODO 注释（"TODO(W1a): per backend §8.1 v0.2.2"）。
- `internal/idempotency/idempotency.go:22` 暴露 `Insert(tx *gorm.DB, rec *domain.IdempotencyRecord) error`。
- `grep -rn "idemRepo.Insert\|IdempotencyRepo.*Insert" internal/service` —— **零命中**。saga_allocate / saga_topup / saga_refund / payment / customer 的所有 mutation 都没在业务 TX 闭包内插 idempotency_record。
- 也没有 grep 反断言 / AST analyzer / e2e panic-rollback 测试存在（`grep -rn "idem.*panic\|TestIdempotency_PanicRollback" .` 零命中）。

**评语**：dev round-2 CRIT-2 关闭判定**只看了 dev doc 字面**而没验证代码层。本轮代码的状态是"middleware 是 stub + service 没人调 Insert"——比 dev round-1 CRITICAL 描述的"middleware c.Next 后 Insert"还要离 design 更远（一个写错地方，一个干脆没写）。

**HIGH，且把 dev round-2 CRIT-2 重新打开为 code round-1 H-2**。

### 4.7 Outbox poller / consumer

- `internal/outbox/consumer.go` 写得相当工整：region 隔离（cn/sg）✅、`UNIQUE(fy_api_log_id, occurrence)` 幂等假设、scrubber、metrics、TickResult、graceful drain、FreshnessGate。设计意图与 integration §3 完全对齐。
- 但 `Source` 接口（consumer.go:67-78）**没有 GORM 实现** —— `find apps/partner-api -name "source_*.go" -o -name "*_source.go"` 零结果；只有 memstub.go（测试用）。
- `internal/outbox/poller.go` `PurgeOnce` 是 `return 0, nil` 占位。
- `FreshnessGateDefault = 5 * time.Minute`（consumer.go:46）—— **文档写 60s，代码默认 5min，5 倍宽**。这会让 settlement 在 outbox lag 4min 时仍通过 gate，违反 SLO。**HIGH**。

### 4.8 PII scrub 钩子 ✅

instance.go:209-217 `snapshotFailed` 调 `i.scrubber.Scrub(errMsg)` 后再 truncate；outbox consumer.go:269-274 同步 scrub。**OK**。

### 4.9 BackoffFor off-by-one — MEDIUM

```go
func BackoffFor(attempts int) time.Duration {
    if attempts <= 0 { return BackoffBase }     // 2s
    d := BackoffBase
    for i := 0; i < attempts && d < BackoffMax; i++ {
        d *= 2                                   // i=0 时 d=4s，但注释说 attempts=1 应 = 2s
    }
    ...
}
```

注释（saga.go:153-154）写"1=2s, 2=4s, 3=8s"——但代码 attempts=1 实际算出 4s。要么注释错要么代码错。saga_test.go 没覆盖 attempts=1 baseline。**MEDIUM**。

---

## 5. Fy-api 覆盖层落地核验（B-12..B-18）

| 编号 | OVERLAY 描述 | 文件存在性 | 评语 |
|---|---|---|---|
| **B-12** | 内部 API 路由 + HMAC 鉴权 | ✅ `router/api-internal-router.go` + `middleware/internal_auth.go` 247 行 | 实现质量高：常量隔离 / `subtle.ConstantTimeCompare` / Redis nonce SETNX / clock skew 5min / body size cap 1MB。但参 §1 third bullet：**与 partner-api client.go 的 header 名 + canonical 形式严重不一致**，CRITICAL |
| **B-13** | tnbiz_internal controllers + ChannelLogSetting upsert | ✅ `controller/tnbiz_internal/{token.go,user.go,health.go,settings.go,context.go}` | 文件齐全；与 OVERLAY-TNBIZ-HANDOFF.md L9-26 描述一致 |
| **B-14** | feature flag 框架（biz_setting + 5-15s polling）| ✅ `setting/overlay_flag/flag.go` + `flag_test.go` | 启动顺序 main.go:317-323 中 `overlay_flag.StartPoller` 第一行就调，符合 HANDOFF L137 |
| **B-15** | GroupRatioOverride hot path 6 调用站 / 4 文件 | ✅ `relay/common/override_lookup.go` + `relay/common/relay_info.go` `UserGroupRatioOverride` 字段 + `service/quota.go` 三处 ApplyOverride + `relay/helper/price.go` 两处分支 + `service/task_billing.go` `LookupUserOverride` cold path + `service/group.go` `GetUserGroupRatioWithOverride` | 6 调用站 + 4 修改文件，与 OVERLAY.md L188 字面一致；feature flag `overlay.group_ratio_override` 守护 |
| **B-16** | consume_log_outbox + RecordConsumeLog 同事务写 | ✅ `model/consume_log_outbox.go` + `model/log.go` `recordConsumeLogWithOutbox` | 顶部 invariant 注释、ConsumeLogOutbox AutoMigrate 加在 LOG_DB（model/main.go） |
| **B-17** | Outbox publisher（Aliyun MNS, shadow + enabled） | ✅ `service/outbox/runner.go` + `runner_test.go` | NoopPublisher 占位（与 dev doc Phase 2A 对齐）；main.go:320-323 启动一个 runner |
| **B-18** | internal_idempotency + internal_api_key (HMAC keystore) | ✅ `middleware/internal_idempotency.go` + `model/internal_api_key.go` | 双 flag 校验 internal_auth.go:53-58 严格——`InternalAPI ON + HMACKeystore ON` 才放行，任一 OFF 即 503，符合 OVERLAY.md L225 |

**B-12..B-18 文件级落地度 = 7/7**，feature flag 有 5 个（与 HANDOFF L93-97 一致），启动顺序 main.go:317-323 严格按 HANDOFF L137-141。

**唯一关键缺陷**：B-12 HMAC 契约与 partner-api 端 client 不一致（详见 §6.1）。

---

## 6. CRITICAL / HIGH / MEDIUM / LOW

### 6.1 CRITICAL

**C-1 [HMAC 契约断裂] partner-api fyapi.Client 与 Fy-api InternalAuth middleware 端到端不可验签**

证据：
- partner-api: `internal/infra/fyapi/client.go:104-119`
  - 4 头：`X-Auth-KeyId / X-Auth-Timestamp / X-Auth-Nonce / X-Signature`
  - canonical（client.go:138）：`method + "\n" + path + "\n" + query + "\n" + sha256_hex(body) + "\n" + ts + "\n" + nonce` —— 6 段
- Fy-api: `middleware/internal_auth.go:36-45 + L10`
  - 4 头：`X-Tnb-Key-Id / X-Tnb-Timestamp / X-Tnb-Nonce / X-Tnb-Signature`
  - canonical：`METHOD\nPATH\nTIMESTAMP\nNONCE\nKEY_ID\nsha256(body)` —— 6 段，**没有 query 段，且 KEY_ID 在 nonce 后**

任意一次 partner-api → Fy-api 真实调用都会因 header 名不匹配（client 写的 `X-Auth-*` 在 Fy-api 端被 `c.GetHeader("X-Tnb-Key-Id")` 读到空字符串）即刻 401，连签名比对都到不了。

**修复指令**：选 dev doc integration §17 / OVERLAY-TNBIZ-HANDOFF 任一为唯一 source of truth，把另一端字面对齐。建议以 Fy-api 端为准（已 unit test 覆盖、production 入口），把 partner-api client.go 的 4 个 header 改名 + canonical 顺序改为 `METHOD\nPATH\nTIMESTAMP\nNONCE\nKEY_ID\nsha256(body)`。需要在 dev-doc 加 PR-INV：`grep -rn "X-Auth-Sig\|X-Auth-KeyId" apps/partner-api` 必须 = 0。

**C-2 [资金一致性] saga 补偿失败被静默吞错**

证据：`internal/service/saga_allocate/service.go:128-159` 共 7 处 `_, _ = sg.Compensate(...)`，所有补偿 error 被 `_` 丢弃。配合 §4.5 retry sweep 不重做 fn 的事实，失败补偿没有任何 retry / escalate / alert 通路。saga_refund / saga_topup 同样模式（grep 全仓 `_, _ = sg.Compensate` 共 12 处）。

**修复指令**：参见 §4.4 修复指令，三步全做。

### 6.2 HIGH

**H-1 [retry sweep 不重做 fn] orchestrator.Sweep 仅判 escalation**

证据：`internal/saga/orchestrator.go:64-93` 字面注释 + saga_test.go 唯一 sweep 用例 `TestSweep_EscalatesAfterMaxAttempts`。失败 step 永远不会被自动重做。

**H-2 [idempotency middleware + service 同 TX Insert 双双 stub]**

证据：`internal/middleware/idempotency.go:50-56` W0 stub；`grep -rn "idemRepo.Insert" internal/service` 零命中；e2e panic-rollback test 不存在。dev round-2 CRIT-2 关闭判定无效，必须重开。

**H-3 [JWT middleware 全直通] internal/middleware/auth.go:48-54 是 W0 stub**

证据：`func JWT(...) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }`。意味着所有挂在 `r.Group("/partner")` / `/customer` / `/admin` 的路由都**没有任何鉴权**（handler/w1a_routes.go:60-77）；scopeOf (w1a_routes.go:96-111) 试图从 `c.Get("jwt_claims")` 拿 actor，但 nothing sets it，fallback 路径靠 `X-Dev-Actor-Type/Id` 头部信任客户端——**生产部署即整站可绕过 BOLA**。CSRF middleware (csrf.go:20-30) 同样直通。

**H-4 [fyapi.Client 全部 endpoint 方法 not implemented]**

证据：`internal/infra/fyapi/client.go:158-180` 共 5 个方法 stub `return nil, errors.New("...not implemented; W1b to wire per integration §2")`。但 saga_allocate/service.go:135 / saga_topup/service.go:191 已经 wire 的方法是 `TopupCustomer / RefundCustomer`——这两个方法**根本不在 fyapi.Client 上声明**。saga 服务依赖一个不存在的方法，编译能过纯属因为 saga 端用 `FyAPIPort interface` 口径，但 wire 一旦接进来即 nil pointer panic。

**H-5 [partner_wallet.balance CHECK ≥ 0 漏改]** — dev round-2 §10 cosmetic 必做项第 5 + 附加项第 2 双重指令未落地，详见 §3.1 row 002。

### 6.3 MEDIUM

| ID | 标题 | 证据 |
|---|---|---|
| M-1 | freshness gate 60s vs 代码 5min 偏离 | consumer.go:46 `FreshnessGateDefault = 5 * time.Minute` |
| M-2 | settlement runner 仍 stub | internal/service/settlement/runner.go 未实现 leader lease + 续约 + progress_offset 续跑 |
| M-3 | invoice_title.is_default 缺条件唯一约束 | 005:73-88 允许同 owner 多个 default=1 |
| M-4 | audit_log_pii.diff_cipher VARBINARY(65535) row size 风险 | 006:51 应改 MEDIUMBLOB/LONGBLOB |
| M-5 | scopeOf 走 `X-Dev-Actor-*` header 在生产 build 仍激活 | handler/w1a_routes.go:102-109 没有 `if cfg.Env != "prod"` 守护，prod 就是 BOLA 整站绕过 |
| M-6 | BackoffFor 注释 vs 实际语义 off-by-one | saga.go:155-167 |
| M-7 | internal-api.yaml 物理路径仍未指定 | dev round-2 H-10 cosmetic 失约 |
| M-8 | OVERLAY 与 partner-api 之间 query 段是否参与签名 doc 不明 | 见 C-1 修复时同步收口 |
| M-9 | service 层 idempotency_record TTL purge cron 接线悬空 | idempotency.go:26-29 stub |

### 6.4 LOW

| ID | 标题 | 证据 |
|---|---|---|
| L-1 | password_reset.purge cron 仍未在 §6 cron 表登记（N-2 残余）| dev round-2 §10 必做项第 5 |
| L-2 | settlement.timezone 默 'Asia/Shanghai' 硬编码 | 004:10 |
| L-3 | idempotency_record 缺月分区设计 | 007:38-55 |
| L-4 | pipl_request 缺 withdrawn_at 列 | 010:91 |
| L-5 | T-1..T-17 债务 ID 在代码层零字面引用 | grep 全仓 `T-14\|T-15\|T-16\|T-17` 零命中 |

---

## 7. 架构债务实际状态（dev round-2 17 条 vs 代码标记度）

dev round-2 整合的 17 条 T 系列债务（T-1..T-17）在 partner-api Go 代码中**字面引用度 = 0**——`grep -rn "T-1[0-7]\|T-9 \|debt:" apps/partner-api --include="*.go"` 仅 1 处命中（`internal/idempotency/idempotency.go:27` "TODO(W1a)" 不带 T 编号）。换言之，**所有架构债务都没有按编号显式写到代码里**，只有零散 `TODO(W1a)` / `TODO(W1b)` / `TODO(W1c)` 的 stage tag，工程视图与 dev doc 债务台账完全脱节。

后果：

1. Phase 2A 启动时无法用 `grep T-3 .` 直接定位"per-model markup schema-only" 在哪些文件里，本工程的债务追溯链全靠人脑+文档。
2. Round-2 reviewer / Phase 1 实施者如果只看 git log 或 grep TODO，会把 T-7 (KEK 轮换) / T-8 (KYC 17 cipher 抽象) / T-13 (UUIDv7 canonical 文档) 等看作"开发遗留"而非"架构既定债务"，可能误把非阻塞的债务当成 P0 bug 修。
3. T-14 stop-gap 三条（24h captcha / 邮件二次确认 / 高风险账户 KYC）—— W1a 的 `internal/service/auth/password_reset.go` 完全没有任何相关代码或 TODO 注释提示。

**修复指令**：建立"代码 ↔ debt 双向引用"约定。每条 T-N 必须在至少 1 个 Go 文件出现 `// TODO(T-N): <债务摘要> // ETA: Phase 2A` 字面注释；CI 加一个 lint 步骤——读 `docs/debt-registry.md` 列出 T-1..T-17 表 → grep 仓库 → 缺一即 fail。

---

## 8. 测试 / 可演进性 / 可观测性

### 8.1 测试覆盖

partner-api 测试文件 25 个（`find -name "*_test.go" | wc -l`）；表驱动风格普遍；race 标记无字面引用 `-race` 但 Makefile 未读，假设遵循 golang/testing.md。

**核心 invariant 缺测**：
- 没有 `TestIdempotency_PanicRollback`（H-2 invariant 三连第 3 条）
- 没有 `TestSaga_CompensateFailure_RetriesAndEscalates`（C-2）
- 没有 `TestFyAPIClient_HMACMatchesFyApiMiddleware`（C-1 防回归）
- 没有 `TestFreshnessGate_ConfiguredAt60s`（M-1）

### 8.2 trace_id 端到端

partner-api 内部链路：`middleware/request_id.go` → `pkg/tracing/tracing.go` → `audit_log.trace_id` / `revenue_log.trace_id` / `saga_step.trace_id` / `notification_outbox.trace_id` / `idempotency_record.trace_id` / `password_reset_token.trace_id` / `pipl_request.trace_id` —— DDL 层全部带 `trace_id VARCHAR(64) NOT NULL DEFAULT ''`。partner-api → Fy-api：`fyapi/client.go:117-119` `httpReq.Header.Set("X-Oneapi-Request-Id", req.TraceID)`。Fy-api 端 `middleware/request-id.go`（CLAUDE.md 引用，未本轮 review）。

**结论**：trace_id schema 层端到端齐全；运行时透传依赖 saga handler 调用方真传 `traceID` 参数（saga_allocate/service.go:135 已传），acceptable。

### 8.3 可演进性 → Phase 2A schema migration

风险点：
- `kyc_application` 17 cipher 字段 → Phase 2A 抽 `kyc_pii_attribute` 关联表时需要 in-place 迁移，工作量大（T-8）
- `idempotency_record` 不分区 → 月分区改造时需停写 + ALTER TABLE PARTITION
- `partner_wallet.balance` 缺 CHECK ≥ 0 → Phase 2A 启 partner_debt 时若已有违反行，ALTER ADD CHECK 会失败 → 必须先做数据修正脚本

---

## 9. 修订指令（按优先级）

### 9.1 Code-Round-2 阻塞条件（必须解决才放行）

1. **修复 C-1 HMAC 契约断裂**：partner-api fyapi.Client 改 4 头 + canonical 字面对齐 Fy-api internal_auth；加端到端集成测试 `TestFyAPIClient_HMACAcceptedByFyApi`（在同一进程内拉起 Fy-api InternalAuth middleware 并发请求验证）。
2. **修复 C-2 saga 补偿静吞**：所有 `_, _ = sg.Compensate(...)` 改成显式 error handling；补偿失败必写 saga_step.failed + 触发 retry；加 `TestAllocate_RefundFailsAfterDeduct_RetryRecovers` e2e。
3. **落地 H-1 retry sweep 重做 fn**：注册 saga handler 表（`map[(saga_kind, step_name)]TxFn`），Sweep 调用 handler 重做 fn；或者明确 PRD/dev doc Phase 1 不支持自动 retry 并把 SLA 调宽。
4. **落地 H-2 idempotency middleware + service 同 TX Insert**：middleware/idempotency.go 不再 stub；至少 saga_allocate / saga_topup / payment 三处 mutation handler 落"业务 TX 闭包内 idemRepo.Insert(tx, ...)"；e2e panic-rollback test 必须存在。
5. **修复 H-3 JWT/CSRF middleware**：auth.go + csrf.go 完成实现；scopeOf 的 `X-Dev-Actor-*` 头部 fallback 必须 `if cfg.Env == "dev"` 守护，prod build 编译期排除。
6. **修复 H-4 fyapi.Client TopupCustomer/RefundCustomer 缺方法**：在 client.go 增 method 实现（POST /api/internal/user/topup + /refund，per integration §2）。
7. **修复 H-5 partner_wallet.balance CHECK ≥ 0**：002:5-17 添加 `CONSTRAINT chk_wallet_balance CHECK (balance >= 0)`；同步出 down.sql 兼容 migration。

### 9.2 Code-Round-2 强烈建议（不阻塞但应同步处理）

1. M-1 freshness gate 默认值改 60s；从 biz_setting 可配置覆盖。
2. M-2 settlement runner 真实现 leader lease + progress_offset 续跑。
3. M-3 invoice_title.is_default 加触发器或在 service 层 enforce 唯一性。
4. M-4 audit_log_pii.diff_cipher 改 MEDIUMBLOB。
5. M-5 scopeOf dev fallback 守护。
6. M-6 saga BackoffFor 注释或代码取一致。
7. M-7 internal-api.yaml 物理路径明示（落 Fy-api repo `openapi/internal-api.yaml`）。
8. §7 债务台账 grep-CI：T-1..T-17 各至少 1 处代码注释 + lint 步骤。

### 9.3 Phase 2A 推迟（不在 Code-Round-2 范围）

- T-3 per-model markup（pricing 表已留生成列）
- T-7 KEK rotate
- T-8 KYC 17 cipher 抽象
- T-9 saga retry worker 拆 cmd
- T-13 UUIDv7 canonical 文档
- T-14 WebAuthn 自助 + 实人 KYC 通用化

### 9.4 给 W1c agent 的最后一段话

W0/W1a/W1b 的 DDL 层非常规整，可作为 Phase 1 数据库部署的 source of truth；W1b 的 saga 框架接口设计（UUIDv7 + immutable snapshot + Repository 抽象 + Transactor 抽象）也是干净的，但 §4.4 / §4.5 / §4.6 三处把"接口设计"和"必做行为"混淆——文档里写"必做"的（同 TX Insert / retry 重做 / 补偿不静吞）在代码里都是 stub 或干脆没人调。这是一种隐蔽的"架构看似完成、行为实际未启动"，比直接说"Phase 1 不做"更危险，因为 PR review / e2e 能跑过但生产即雪崩。

W1c 收口必须把上述 7 条阻塞项**全部落地**才可声明 Phase 1 backend 完成；否则建议把 Phase 1 范围在 PRD 显式收窄到"DDL + 最小 happy path + 文档级 saga"，不要让 dev/integration doc 的字面契约在代码里变成空头支票。

C-1（HMAC 契约断裂）是这一轮最危险的发现，值得 W1c 优先修——它是字面级 wire bug，复现成本极低（任何一次 happy-path 集成测试都会暴露），但因为本轮 e2e 测试缺失（`tests/e2e/.gitkeep` 仅 placeholder），到 staging 部署前都不会被察觉。

---

## 10. 总结

**Verdict：FAIL**（CRITICAL = 2 / HIGH = 5）

但需要清晰区分：

- **DDL + 接口设计层** = 良好，达到 PASS-CONDITIONAL 标准，与 dev round-2 cosmetic 大部分一致（4/5 兑现）。
- **wire / middleware / endpoint / saga 行为层** = FAIL，dev round-2 关闭的 CRIT-1（JWT cookie）/ CRIT-2（idempotency 同 TX）/ CRIT-3（outbox poller）三个 critical 在代码层都仍然是 stub 或行为缺失。
- **Fy-api OVERLAY 落地** = 7/7 文件齐全，但与 partner-api 的 HMAC 契约断裂（C-1）。

dev round-2 把 doc 层 PASS-CONDITIONAL 给得偏松——一个只在文档中写"middleware 字面重写 invariant 三连"但代码里 middleware 只有 `c.Next()` 的 commit，应该算 doc-vs-code 不闭环 CRITICAL 而不是 doc PASS。建议未来 dev round 收口时同步过一遍 W1a 落地代码（即便是骨架），避免"文档级闭环"被误判为"工程级闭环"。

字数：本文档约 5200 字纯论述（不含表格 / 代码块）；逐条复核 dev round-2 17 条债务 + 20 项契约 + 11 张 DDL + 7 条 OVERLAY；新发现 C-1 / C-2 / H-1..H-5 / M-1..M-9 / L-1..L-5 共 21 条；严格按 0 CRITICAL / 0 HIGH 门槛判 **FAIL**；不放水。
