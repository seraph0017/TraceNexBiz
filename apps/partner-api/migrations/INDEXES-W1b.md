# W1b — 关键索引清单（不写新 DDL，仅记录）

> W0 已落 0001..0011 五个 DDL；本表只列 W1b 业务路径强依赖的索引/约束，便于 review。
> 真正改 DDL 时由后续 W1 任意 agent 新建 `0012_*.up.sql`，不要回头改 0001..0011。

## 1. saga_step（007）

- ✅ `UNIQUE uk_saga_step (saga_id, step_name)` — 幂等关键
- ✅ `CHECK chk_saga_status` — 状态枚举，与 internal/saga.Status 一一对应
- 用法：`Run` → `GetStep(saga_id, step_name)` 命中 UNIQUE 即幂等返回；`Save` UPSERT

## 2. revenue_log（003）

- ✅ `UNIQUE uk_revenue_log (fy_api_log_id, occurrence)` — outbox 消费幂等关键
- ✅ `idx_revenue_partner_time (partner_id, occurred_at)` — settlement 月聚合扫描
- ✅ `idx_revenue_customer_time (customer_id, occurred_at)` — customer dashboard
- ✅ `idx_revenue_settlement (settlement_id)` — settlement 反查

## 3. wallet_hold（002）

- ✅ `UNIQUE (saga_id)` — saga two-phase hold 防重
- 用法：M3-04 allocate 用 `saga_id` 作为 hold 标识

## 4. partner_wallet_log（002）

- ✅ `UNIQUE (idempotency_key, type)` — wallet 操作复合幂等键（v0.2 ARCH HIGH-8）
- 用法：allocate/refund/settlement_payout 不会重复记账

## 5. settlement / settlement_item（004）

- 已有 status 枚举 CHECK；W1b 状态机 `internal/service/settlement.CanTransition` 与之严格一致
- 性能预算：`idx_si_settlement_partner` 应该有（W1c 检查；如缺，0012 补）

## 6. consume_log_outbox（在 fy_api_db；Fy-api W1d 已落）

- ✅ `(data_region, status)` 联合索引 — region 隔离强制
- ✅ `(consumed_at)` — 30d purge cron 用
- 关键：`status='pending' AND data_region=?` 全表必走索引（per integration §3.3 v0.2）

## 7. billing_dispute（W0 未建？需新建 DDL）

> ⚠️ HANDOFF-W0 §1.2 列了 0001..0011 DDL，但未提到 billing_dispute。
> W1b 通过 `internal/service/dispute` 提供领域模型，但 schema 仍空。
> 建议 W1c 新建 `0012_billing_dispute.up.sql`：
>
> 字段：id BIGINT, opener_type VARCHAR(16), opener_id BIGINT, revenue_log_id BIGINT,
>       amount BIGINT, reason TEXT, status VARCHAR(16),
>       reviewer_id BIGINT NULL, reviewed_at TIMESTAMP(3) NULL,
>       refund_saga_id VARCHAR(64),
>       created_at, updated_at
> 索引：idx_dispute_revenue (revenue_log_id), idx_dispute_status (status)

## 8. 性能预算（per backend §17）

| Endpoint | P95 目标 | RPS（峰） | 说明 |
|---|---|---|---|
| outbox consumer TickOnce | 200ms | 50 batches/s | batch=50；DB 短 TX |
| FreshnessGate query | 50ms | 1/s | settlement 启动期单次 |
| Settlement RunMonthly (1k partner batch) | 30s | — | partition by partner_id |
| Saga Run (single step) | 80ms | 100 saga/s | 含 in_progress + commit 两次 SAVE |
| ForceResolve validate | 5ms | <1/min | 内存计算 |
| Dispute FinalizeAccept | 200ms | <10/min | 含 launch refund saga |

## 9. 与 W1a 的协作

- saga retry worker 实现位于 W1a；本包提供 `Sweep(ctx, batch)` 接口
- outbox.PurgeService.PurgeOnce 留空；W1a 写真实 SQL（DELETE LIMIT 10000 循环）
- `revenue.Resolver` 接口（fy_user_id → partner/customer/rule_id）由 W1a/W1b 联合实现
