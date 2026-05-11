# HANDOFF-W1b — 计费消费 / saga 框架 / 结算 / 收益 / 争议

**日期**：2026-05-11
**承接**：W1a（基础设施）/ W1c（admin / payment / 结算 runner 主体）/ W1d（Fy-api OVERLAY）
**前置**：HANDOFF-W0 / OVERLAY-TNBIZ-HANDOFF.md / backend §3.x §4.5 §5.4 §6 §8.17 §9.3 §15.4 §17 / integration §3 §4

---

## 1. 交付内容

### 1.1 saga 框架（`internal/saga/`）

| 文件 | LOC | 用途 |
|---|---|---|
| `saga.go` | 178 | Saga / Orchestrator / Repository 接口；Status / Kind 枚举；UUIDv7 工厂；BackoffFor / ShouldEscalate |
| `instance.go` | 215 | Saga 实例；Run / Compensate；Transactor 抽象（GormTransactor + 测试 stub）；step UPSERT 幂等 |
| `orchestrator.go` | 80 | NewSaga（强制 UUIDv7）/ Resume / Sweep |
| `force_resolve.go` | 132 | dual-control 决策纯函数（不同人 + 30min cooldown + IP /24 + 一次性 token + 显式 target）|
| `memrepo.go` | 124 | 内存 Repository（dev / test）|
| `saga_test.go` | 263 | UUIDv7 / 幂等 / 失败补偿 / dual-control 8 例 / IPv6 /48 / sweep escalation |

**关键决策**：
- saga_id 用 `uuid.NewV7()`，不再 BIGINT（v0.2.1 ARCH-HIGH-NEW-D 纠正）
- step 写入 UPSERT；committed step 再 Run 直接 return（幂等）
- `Transactor` 接口抽象 `bizDB.Transaction`，测试用 fakeTx 直接 fn(nil)，让 saga 单测脱离 DB

### 1.2 outbox 消费者（`internal/outbox/`）

| 文件 | LOC | 用途 |
|---|---|---|
| `consumer.go` | 240 | 主循环 `Run(ctx)` / `TickOnce(ctx)`；region 隔离；DLQ；FreshnessGate；幂等 by UNIQUE |
| `memstub.go` | 165 | MemSource + MemSink；测试用 |
| `poller.go` | 26 | PurgeService.PurgeOnce 占位（W1a 落 SQL）|
| `consumer_test.go` | 124 | happy path / region isolation / DLQ after 3 retries / dedupe by unique key / freshness gate / malformed event |

**Source/Sink 接口**：W1a 落 GORM 实现（`source_log_db.go` + `sink_revenue.go`）；本包提供契约 + memstub。

### 1.3 服务层

| 包 | 文件 | 说明 |
|---|---|---|
| `service/revenue` | `service.go` + `_test.go` | outbox.Sink 适配；ComputeRow 纯函数；net<0 clamp 到 0 |
| `service/settlement` | `engine.go` + `runner.go` + `_test.go` | 状态机 + ComputeItem（含 personal 个税 20%）+ 月度 Runner + freshness gate |
| `service/dispute` | `service.go` + `_test.go` | submit/review/accept/reject 状态机；accept 联动 refund saga |
| `service/saga_allocate` | `service.go` + `_test.go` | M3-04 5 步 + 对称补偿 |
| `service/saga_topup` | `service.go` | 客户充值 saga 4 步（intent/provider/fyapi/notify）+ Initiate / OnCallback / Fund / NotifyEscalated |
| `service/saga_refund` | `service.go` | 退款 4 步（reverse/wallet/provider/notify）|

### 1.4 admin handler（`internal/handler/saga_admin.go`）

- `NewSagaForceResolveHandler(deps)` — POST /admin/saga/force-resolve
- `NewDisputeAcceptHandler(deps)` / `NewDisputeRejectHandler(deps)` — admin 终审

W1c 注入 admin middleware（JWT + RBAC + step-up MFA + audit_log）后即可挂路由。

### 1.5 文档

- `migrations/INDEXES-W1b.md` — 关键索引清单 + 性能预算表

---

## 2. 验收

```bash
$ go build ./...                                    # ✅
$ go test -race ./internal/saga/... ./internal/outbox/... \
    ./internal/service/saga_allocate/... ./internal/service/saga_topup/... \
    ./internal/service/saga_refund/... ./internal/service/settlement/... \
    ./internal/service/revenue/... ./internal/service/dispute/... \
    ./internal/handler/...                           # ✅ ok
```

> ⚠️ `internal/service/partner` 单测在 W1b 工作前已存在编译失败（`StatusTerminated` 类型不匹配），属于 W1a/其他并行 agent 范围，不在 W1b 范围内修复。

---

## 3. 与其他 agent 的契约

### 3.1 W1a 需要落地的接口

| 接口 | 位置 | 用途 |
|---|---|---|
| `saga.Repository` GORM 实现 | `internal/repository/mysql/saga_mysql.go` | UPSERT by (saga_id, step_name) UNIQUE |
| `outbox.Source` GORM 实现 | `internal/outbox/source_log_db.go` | LOG_DB FOR UPDATE SKIP LOCKED + region 过滤 |
| `outbox.Sink` GORM 实现（用 revenue.Service） | `internal/outbox/sink_revenue.go` | 注入 `revenue.NewService(repo, resolver)` |
| `revenue.Resolver` | `internal/service/revenue/resolver.go` | fy_user_id → (partner_id, customer_id, rule_id) |
| `outbox.PurgeService.PurgeOnce` 真实 SQL | 同包 | 30d 物理 DELETE |
| `saga.Sweep` retry worker（30s ticker） | `cmd/saga-retry-worker/main.go` 或 main.go goroutine | partition by hash(saga_id) |
| `pii.Scrubber` | `pkg/piiscrubber` | 已有 W0 占位；saga.Save / outbox last_error 调用 |

### 3.2 W1c 需要装配的 handler / service

| 服务 | 注入路径 | 备注 |
|---|---|---|
| `saga_allocate.Service` 的 `WalletPort / FyAPIPort / CustomerLookup` | service/wallet + infra/fyapi + repository/customer | 必须保证 idempotency_key=saga_id 透传 |
| `saga_topup.Service` 的 `IntentPort / ProviderPort / FyAPIPort / Notifier` | repository/topup_intent + service/payment + infra/fyapi + service/notification | callback handler 由 W1c 写 |
| `saga_refund.Service` 的 `RevenuePort / WalletPort / ProviderPort / Notifier` | service/revenue + service/wallet + service/payment + service/notification | accept-launches-refund |
| `dispute.Service` 的 `Repository / RefundLauncher / SagaIDFactory` | repository/dispute + 包装 saga_refund.Service.Run + saga.NewSagaID | dispute schema 缺，见下 |
| admin handler routes mount | `internal/handler/routes.go` admin group | 接 mw.AuthAdmin + RequireRole + RequireStepUp + audit_log |

### 3.3 与 W1d 对齐

| 端点 | W1d 已实现 | W1b 调用方 |
|---|---|---|
| POST /api/internal/user/topup | ✅ controller/tnbiz_internal/user.go | saga_allocate / saga_topup（用 Idempotency-Key=saga_id）|
| POST /api/internal/user/refund | ✅ | saga_refund.ProviderPort impl 之一 |
| POST /api/internal/user/quota/adjust | ✅ | saga 内的 quota.adjust 步骤（按需） |
| consume_log_outbox 表 | ✅ schema + DAO | outbox.Source GORM 实现拉取此表 |

**HMAC 4 元组**用 `infra/fyapi.Client.Do`；trace_id 头透传 `X-Oneapi-Request-Id`，端到端贯通。

---

## 4. 已知风险 / 未做事项

1. **billing_dispute 表 DDL 缺**：W0 未建表；W1b 提供领域模型 + service。需 W1c/W1a 新建 `0012_billing_dispute.up.sql`（schema 见 `migrations/INDEXES-W1b.md` §7）。
2. **个税 20% 是 placeholder**：PRD §15.4 实际税率由财务终审决定；常量 `PersonalWithholdRate = 20` 待 ADR-Q10 决议改成 biz_setting 热加载。
3. **revenue_log fee 字段语义未定**（PRD §13 Q10）：当前 PartnerFee/PlatformFee 计为 0，由 settlement 阶段重算。已在 `service/revenue/service.go` 注释标注 ADR-NEW。
4. **outbox.PurgeService.PurgeOnce 占位**：W1b 不写 DDL/SQL；W1a 落地。
5. **saga retry worker 的真实重做**：当前 `Sweep` 只判断 escalation，不重做 fn。W1a 把 saga handler registry 落地后才能真正 retry；现状下失败 step 会留在 failed 状态等下一次 service 调用幂等推进。
6. **dual-control token store**：handler 假定 caller 已用 Redis SETNX 标记消费；token store 落地由 W1c 或 W1a 与 admin 流程一并接入。

---

## 5. 不变量（W1b 保证）

- ✅ saga_id 全部 UUIDv7（NewSaga / NewSagaID 强制）
- ✅ saga step 幂等：committed step 二次 Run 直接 return nil
- ✅ outbox 消费幂等：UNIQUE(fy_api_log_id, occurrence) 冲突 → return false, nil
- ✅ region 隔离：Consumer 启动绑 region；mismatch 立即 nack to DLQ
- ✅ settlement lock 后不可再改字段（状态机 CanTransition + AssertLockable）
- ✅ trace_id 端到端：outbox.Event.TraceID → revenue_log.TraceID → saga_step.TraceID
- ✅ immutability：Run/Compensate/Transition 全部返回新结构体，不 mutate 入参
- ✅ 无 PII 入 saga_step.payload（写入路径预留 PIIScrubber 接口；W1a 注入 `pkg/piiscrubber`）
- ✅ 文件 ≤ 400 行；最长 `consumer.go` 240 行

---

— W1b agent，2026-05-11
