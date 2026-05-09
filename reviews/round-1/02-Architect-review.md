# PRD v0.1 Review — Software Architect (Round 1)

> Date: 2026-05-09
> Reviewer: Architect agent
> Verdict: **NEEDS_REVISION** (block until §6/§9 are reworked; the rest is fixable in v0.2)

---

## Summary

The PRD has the right product instinct: keep Fy-api as the authoritative AI-gateway and put a lightweight commercial layer on top. The **two-database, same-instance** boundary, the saga shape for cross-system money moves, and the deliberate scoping of MVP-vs-Phase-2 are sound calls.

But several load-bearing claims are **factually wrong against the Fy-api code I just read**. In particular:

1. The PRD says "Fy-api 不动一行代码" (§9.2). That is **false**. Fy-api today has **no `/api/internal/*` namespace**, no per-user-group writable endpoint, no public `group_ratio` upsert API, and group_ratio is propagated via a **60-second polling cycle** (`SyncOptions(SyncFrequency)` in `main.go:95`), not a push. At minimum we need ~7 internal endpoints and a Redis-pubsub-style invalidation hook merged as Fy-api overlay or the system simply will not work as drawn.
2. `User.Quota` is `int` with `gorm:"type:int"` (32-bit MySQL INT), not `int64`. The PRD's `Wallet.Balance int64` design is right, but **mapping to Fy-api's 32-bit quota will overflow** at ~2.1 B quota units (≈ $4,294 USD at QuotaPerUnit=500000) for any large partner. This is an inherited Fy-api ceiling, but the PRD must address it.
3. The CDC plan in §9.3 silently ignores that Fy-api supports **MySQL, PostgreSQL, and SQLite** simultaneously (Fy-api/CLAUDE.md "Rule 2"). canal/maxwell are MySQL-binlog-only. If TraceNex SG ever moves to PG, the entire revenue pipeline breaks.
4. Logs may live in a **separate database** (`LOG_DB` controlled by `LOG_SQL_DSN`, see `Fy-api/model/main.go:41-66`). The PRD's "same instance, different db, free JOINs" assumption (§5.2 #3) is wrong for any deployment that splits log-DB out — and SG production specifically reflects this configuration. Cross-DB JOIN will not work there.

There are also concrete risks in the wallet concurrency model, the saga's idempotency story, the cron lock claim, and the "TraceNexBiz is read-only" enforcement story. Details below.

This is a v0.1, not a hopeless v0.1 — but **the integration boundary is currently a fiction** and that's the riskiest part of the system. Fix that, then this becomes an ACCEPT_WITH_NOTES.

---

## CRITICAL architectural issues

### C1. "Fy-api 不动一行代码" is not achievable

§9.2 lists this as a strict optimization. §6.1 lists 8 internal endpoints, all under `/api/internal/*`. **None of these exist in Fy-api today.** I verified:

- `Fy-api/router/api-router.go` has only `/api` (with sub-groups `/user/`, `/admin/`, etc.). Topup is `selfRoute.POST("/topup", ...)` — a user-context endpoint, not a service-to-service one.
- There is no machine-to-machine auth middleware. `middleware/auth.go` is JWT/session/access-token oriented.
- `controller/topup.go::TopUp` reads `userId` from the JWT context, not from a path/body parameter — i.e. you cannot top up "user X" as a service caller without spoofing.
- There is no controller for "set user.group" (we have admin Edit, but it's a full UPDATE, scoped to admin auth and non-idempotent).

Concrete consequence: **TraceNexBiz on day 1 must include an Fy-api overlay PR** that adds:

- `router/api-internal-router.go` (new) registering 7-8 endpoints.
- `middleware/internal_auth.go` (new) — HMAC + timestamp + key-id rotation.
- `controller/internal_*.go` (new files, per overlay-strategy in `Fy-api/CLAUDE.md`).
- `controller/internal_user.go::SetUserGroup` (writes `User.Group`, invalidates Redis cache via `model/user_cache.go`).
- `controller/internal_group.go` to upsert `OptionMap["GroupRatio"]` and trigger immediate `loadOptionsFromDatabase` on the local instance, plus publish a Redis pubsub event so **other** instances pick it up immediately rather than after the 60-s sync.

This is non-trivial. Estimate: 2-3 weeks of Fy-api engineer time, plus ongoing maintenance during monthly upstream syncs (which is exactly what `Fy-api/OVERLAY.md` exists to track).

**Recommendation**: drop the "no Fy-api changes" framing in v0.2. Replace with a section "§6.A Fy-api overlay surface" that owns the contract. Treat the overlay PR as a Phase-1 dependency.

### C2. group_ratio propagation has a 60-second eventual-consistency window

§9.2 row 3 says "调价立刻生效（改 group_ratio 即可）" — **wrong**.

Ground truth from `Fy-api/main.go:95`:

```go
go model.SyncOptions(common.SyncFrequency)  // SyncFrequency defaults to 60s (common/init.go:102)
```

`SyncOptions` in a tight loop calls `loadOptionsFromDatabase()` every `SyncFrequency` seconds. There is **no push/pubsub** path. So when TraceNexBiz writes a new group_ratio via the new internal API:

- The Fy-api **instance that handled the write** updates its in-memory `groupRatioMap` immediately (because `model.UpdateOption` calls `updateOptionMap` synchronously).
- **All other Fy-api instances** in the cluster (CN + SG, multi-pod blue/green) see the change **only after their next 60-s tick**.

For a billing system this is unacceptable. Imagine: partner sets price 1.50 → calls land on pod B → pod B still has ratio 1.30 → 60 s of mis-priced traffic across multiple customers.

**Mitigations**, ordered by preference:

1. **Push-invalidate via Redis pubsub** (smallest overlay) — overlay adds a publish on `UpdateOption`, all Fy-api pods subscribe, on receive call `loadOptionsFromDatabase()` (or just the changed key). This is the right answer.
2. Cut `SyncFrequency` to 5 s and document the window. Cheaper to ship; more DB load.
3. Add a `Cache-Control: no-store` for group_ratio reads on the billing path. **No** — would gut performance, group ratio is read on every request.

§11 row 3 acknowledges this risk and accepts "秒级窗口". 60 s is not seconds. Update v0.2 honestly.

### C3. CDC plan is MySQL-only; Fy-api supports MySQL/PG/SQLite

§9.3 recommends "binlog CDC（canal/maxwell）". Both are MySQL-binlog parsers.

`Fy-api/CLAUDE.md` Rule 2: "All database code MUST be fully compatible with [SQLite, MySQL ≥ 5.7.8, PostgreSQL ≥ 9.6]." If we adopt canal, we **architecturally lock TraceNexBiz to MySQL deployments only**, and any future move to RDS PostgreSQL (which Alibaba is steadily pushing) will require a rewrite.

Worse: `LOG_SQL_DSN` (Fy-api/model/main.go:41) lets the operator point logs at a **separate database**, which canal then has to be configured to subscribe to separately. SG production may already do this — verify before designing CDC.

**Alternatives**:

1. **Application-side outbox via Fy-api overlay**: in `model/log.go::RecordConsumeLog`, after `LOG_DB.Create(log)`, write a row to `consume_log_outbox(log_id, user_id, group, partner_id_resolved, customer_id_resolved, quota, channel_id, model_name, occurred_at)` in the **same transaction**. TraceNexBiz polls/streams outbox at 1-s tick, idempotent on `log_id`, truncates after consume. Pros: DB-agnostic, latency ~100 ms-1 s, no binlog plumbing, schema-stable contract. Cons: small Fy-api change.
2. **Pure outbox + Redis Stream emitter**: outbox row inserted in same TX, separate goroutine drains outbox to Redis Stream, TraceNexBiz consumes Redis Stream. Adds a hop but decouples consumer scaling. Either is fine; outbox-only is simpler.
3. **Stick with canal, but limit blast radius**: declare MySQL the only supported deployment for TraceNexBiz, document explicitly, and ban PG migration until v2.

I'd pick **(1) outbox** — Redis is already a hard dep of Fy-api, this introduces no new infra, and it's DB-portable.

### C4. `User.Quota` is int32, but `Wallet.Balance` is int64 → overflow risk

`Fy-api/model/user.go:39`: `Quota int`. Go's `int` on Linux/amd64 is 64-bit at runtime, but `gorm:"type:int"` pins the **column type to MySQL INT (32-bit, max 2,147,483,647)**.

PRD §8.3 declares `Wallet.Balance int64`. So the wallet can hold large balances, but the moment we top up a customer's `User.Quota` past INT MAX, the topup endpoint silently truncates (or errors out at the column constraint depending on MySQL strict mode).

This is an **inherited Fy-api bug** — but the PRD's commercial-distribution model makes large balances normal, where today's individual users rarely hit the limit. The PRD must:

1. Document the per-customer ceiling.
2. Decide: do we widen `User.Quota` (and `Log.Id`, `Log.Quota`) to BIGINT in an overlay migration (Fy-api Rule 2 — must work on SQLite/PG/MySQL), or do we shard balances by issuing many low-balance tokens per customer?

I'd push for the BIGINT widening as a Phase-1 dep — it's a `model/main.go` migration that all three DBs handle (PG `INTEGER` → `BIGINT`, MySQL `INT` → `BIGINT`, SQLite is dynamically typed). Document in OVERLAY.md.

### C5. Saga step 4 ("Fy-api topup succeeds, TraceNexBiz crashes before revenue_log") has no clear recovery story

§9.4 describes the saga in 4 lines. The hardest case is missing:

> Step 1: Lock partner_wallet (-$X) ✅
> Step 2: Call Fy-api `/api/internal/user/topup` (+$X to customer) ✅
> Step 3: **Crash before** writing partner_wallet_log + customer-side ledger ❌

Now the customer has $X they didn't pay for, the partner has $X locked but no audit trail. On restart, how does TraceNexBiz know the topup actually succeeded?

Required design:

- **Idempotency keys**: every `POST /api/internal/user/topup` carries a UUID; Fy-api must store the (key, result) and short-circuit on retry. Need to add an `internal_idempotency` table in Fy-api overlay (TTL ~ 7 days).
- **Outbox / saga log table** in TraceNexBiz: `saga_step(saga_id, step, status, payload, attempts, last_error, idempotency_key)`. Steps move forward only via single-transaction commits. On startup, replay any step in `in_progress`.
- **Bounded compensation**: if step 2 returns ambiguous (timeout, 5xx with no response body), TraceNexBiz must call **`GET /api/internal/topup/by-idem-key`** to find out the truth before attempting compensation. This endpoint is missing from §6.1.

The PRD says "saga 模式" but doesn't specify the bookkeeping. v0.2 must.

---

## HIGH issues

### H1. `partner_wallet.Version` optimistic locking is not enough for the wallet-deduct → fy-api-topup saga

§8.3 puts a `Version int64` on the wallet. That's fine for in-process retries on a single deduction. But the saga is:

```
T1: SELECT wallet WHERE id=X → version=v
T2: UPDATE wallet SET balance=balance-100, version=v+1 WHERE id=X AND version=v
T3: COMMIT
T4: HTTP POST Fy-api topup customer
T5: if 4xx/5xx → start compensation
```

Between T3 and T5, **other partner-wallet operations can run** (other concurrent allocations). The compensation `+100` is just another optimistic-lock UPDATE — that's fine, not the issue.

The real issue: **inconsistent intermediate state on `LockedBalance`**. PRD has `Balance` and `LockedBalance`. If we model "during saga the money is in `LockedBalance`, only released to `Balance` on confirm", we need a **two-phase commit table**, not just optimistic lock. Otherwise: lock at T1 → before T3 commit, partner views dashboard → they see Balance not yet decreased and click "allocate again" → over-allocation.

**Recommendation**: add a `wallet_hold` table:

```go
type WalletHold struct {
    ID         int64
    WalletID   int64    `gorm:"index"`
    Amount     int64
    SagaID     string   `gorm:"uniqueIndex"`  // also serves as idempotency key
    Status     string   // 'held' | 'committed' | 'released'
    HeldAt     time.Time
    ResolvedAt *time.Time
}
```

Wallet operations compute "available = balance - sum(holds where status='held')". This is the standard distributed-finance pattern (PayPal, Stripe).

### H2. "TraceNexBiz 只读 transnext_db" enforcement is hand-waved

§5.2 #1 says "GORM 用两个 connection，`transnext_db` 的连接配置成只读". This is correct in principle, but the PRD doesn't lock down **how**:

- **Option A: MySQL-level read-only user.** Create user `tracenex_biz_ro@%` with `GRANT SELECT ON transnext_db.* TO tracenex_biz_ro`. Strongest. Any code path that accidentally tries to UPDATE/INSERT on that connection gets a hard SQL error in dev, never reaches prod.
- **Option B: GORM-level connection flag.** No real flag; `db.Session(&gorm.Session{DryRun: true})` doesn't do what you want. The "read-only" enforcement is purely a code review convention. **Insufficient.**

Pick A. Document in v0.2 with the actual GRANT statements. Plus: have ops automation verify the `tracenex_biz_ro` user has no write privileges as a deploy-time gate.

Side note: if logs really do live in `LOG_DB` (separate instance), the PRD §5 architecture diagram's "JOIN tracenex_biz_db.partner JOIN transnext_db.users" claim is doubly wrong — the JOIN target may not even be in the same instance. v0.2 must verify and rewrite the cross-DB report strategy if so.

### H3. The §9.3 decision diagram understates B-1 cost and overstates B-2 latency advantage

The "B-1 vs B-2" table is biased. Real numbers:

- B-1 webhook adds **~5 ms** to a typical 1-3 second LLM completion. That's <0.5% latency overhead. "性能影响每次 API 调用多一次 HTTP" is real but fractional.
- B-2 (CDC) latency: 1-3 s on a healthy binlog. **On a binlog backlog (replication lag, broker lag)**, can be minutes to hours. "binlog 自然有失败重试" is true but "natural" hides operational pain.
- B-2 hidden cost: **schema-coupling**. The TraceNexBiz consumer has to parse `logs.group` and reverse-engineer `(partner_id, customer_id)`. If Fy-api ever changes `Log` columns or adds Other-blob fields, the consumer breaks silently. With B-1 webhook (or outbox), the contract is explicit.

Better answer: **B-1.5 — outbox pattern (see C3)**. Best of both: Fy-api overlay writes a row in a `consume_log_outbox` table inside the same transaction as the `logs` insert. TraceNexBiz polls outbox at 1-s tick. This is:

- DB-portable (works on SQLite/PG/MySQL).
- Lossless (outbox is the source of truth, polled-and-deleted).
- Schema-stable (outbox row has explicit `partner_id` because the overlay computes it at write time using the user's group at billing moment).

I'd reject B-2 in v0.2 and make outbox the recommended path.

### H4. "10万 group OK" claim is unverified and probably understated as risk

§9.2 risk row 1 says "MySQL 字符串索引能扛 10 万级". That's true for the index, but **not the whole story**. `groupRatioMap` is a `types.RWMap[string, float64]` (`setting/ratio_setting/group_ratio.go:18`) — held in memory in **every** Fy-api process. With 100k entries:

- Memory: 100k × (~40 B key + 8 B value + map overhead) ≈ 8-15 MB per pod. Acceptable.
- Snapshot/load: on every `SyncOptions` tick, `UpdateGroupRatioByJSONString(jsonStr)` parses a 100k-entry JSON. That JSON is several MB and lives as a single row in the `options` table. **Parsing this every 60 s in every pod**, and the row size approaches MySQL `TEXT` row limits.
- The DB option write path: every group-ratio mutation rewrites the **whole JSON blob**. With 100k entries and 50 partners changing prices/hour, you have hot-row contention on `options.GroupRatio` plus the entire blob churn at write.

§11 row 9 says "超过 10 万行时考虑迁移到独立 group_ratio 缓存层" — that's not a plan, it's a kicked can. v0.2 should either:

1. Prove via load test the 100k claim holds, and codify the upper bound (e.g. "max 50 partners × 1k customers = 50k groups").
2. Switch to per-user `User.GroupRatioOverride float64` (overlay schema change). Then group is just `partner_X` (one per partner), and the per-customer markup lives on the user row — bounded growth. This is the better architecture but a bigger overlay.

I lean toward (2) for v1.0, B-2-as-described for MVP only.

### H5. M5-01 distributed lock — Redis SETNX is not enough for cron resumability

§7.5 M5-01 says "go robfig/cron, distributed lock". §11 row 6 mentions "Redis SETNX 或 MySQL GET_LOCK".

Redis SETNX with TTL is a **mutex**, not a **lease with ownership**. Failure modes:

- Pod A acquires lock → starts settling → 30 min into a 45-min batch, Pod A pauses for GC for 90 s exceeding TTL → lock expires → Pod B starts a parallel run on the same period → double settlement.
- Pod A crashes → lock TTL expires (let's say 60 s) → Pod B picks up → **but** Pod B has no idea where A stopped.

Required:

- **K8s Lease object** (preferable if we're on K8s anyway) — has built-in renewal & ownership semantics. Or **etcd lease**, or Redlock with proper renewal.
- **Settlement batch state machine**: settlement row has `status in {generating, generated, paying, paid, failed}` and a `progress_offset` (last partner_id processed). On takeover, resume from offset. Idempotent per-partner-settlement-item creation (UNIQUE on (settlement_id, partner_id)).
- Heartbeat: lease renewer goroutine writes a row to `settlement_run` every 10 s; an alarm fires if no heartbeat for >60 s.
- Time zone: §4.6 says "1号 02:00". Pin this to UTC platform-wide and present localized periods in UI; or store `settlement.timezone` per partner. Either is fine; commit to one.

§7.5 + §8.7 don't model resumability. v0.2 must.

### H6. P95 < 500 ms budget is unrealistic for endpoints that touch Fy-api

§10 says "API P95 < 500ms（不含 Fy-api 调用时间）". The "不含" parenthetical hides the actual SLO.

In practice many TraceNexBiz screens **must** call Fy-api (M2-01, M2-02, M2-05, M2-08, M3-04, M3-05). Each Fy-api call is a network hop + Fy-api work + return. For inter-AZ in Aliyun: ~5-15 ms RTT + ~10-100 ms Fy-api work. That's another 50-100 ms baseline.

Either:

1. Restate as **two budgets**: "TraceNexBiz internal P95 < 500 ms; round-trip P95 < 800 ms".
2. Cache aggressively on the TraceNexBiz side (with 30-60 s staleness). Most dashboards tolerate this.

Don't ship a budget that's effectively unmeasurable.

---

## MEDIUM issues

### M1. FK pattern `fy_user_id`, `fy_token_id`, `fy_api_log_id` to read-only Fy-api tables — no enforcement

There is no MySQL-level FK constraint possible (cross-database FKs are unsupported in InnoDB across schemas in some setups; also broken if logs are in a separate instance per LOG_SQL_DSN).

Risks:

- Fy-api hard-deletes a user (`Fy-api/model/user.go:323 HardDeleteUserById`) → TraceNexBiz `customer.fy_user_id` dangles.
- ID reuse: Fy-api uses MySQL `AUTO_INCREMENT`, but on PostgreSQL with sequence reset there's a small risk of reuse.
- `gorm.DeletedAt` soft-delete on `User` (line 48) — TraceNexBiz queries that JOIN to `users` won't see soft-deleted users without `Unscoped()`. Surprising behavior.

**Mitigations**:

1. Sentinel-only deletes for users with TraceNexBiz business roles. Add a Fy-api overlay check: if `User.Id` exists in `tracenex_biz_db.partner` or `customer`, refuse hard-delete. (Cross-DB read in Fy-api — uncomfortable; consider a "deletion-protect" flag on User instead.)
2. Periodic referential-integrity check job: scan `customer.fy_user_id` for orphans, flag for ops.
3. Always `Unscoped()` cross-DB JOINs from TraceNexBiz, or always JOIN with `WHERE users.deleted_at IS NULL` explicitly. Document.

### M2. `Customer.GroupNameInFyApi` is denormalized; will go stale

§8.2 stores `GroupNameInFyApi`. But the canonical value is `User.Group` in transnext_db. Two stores means two truths. If the partner-update-group internal API succeeds but TraceNexBiz crashes before updating its own row, customer is orphaned.

Easier: don't store; derive on read with a tiny cache. Or rename to `ExpectedGroupName` and add a reconciliation job that asserts equality.

### M3. Settlement timezone

§4.6 says "每月 1 号凌晨 02:00（Cron）". §10 doesn't pin timezone. CN partners want CST; SG partners want SGT; international want UTC. This is a billing primitive — not a UI nicety. v0.2 must decide.

### M4. Observability section is one row in §10 ("Prometheus 指标")

Concrete questions unanswered:

- Where do logs go? Same SLS as Fy-api? (Fy-api `Fy-api/CLAUDE.md` mentions `system_monitor_*` and Aliyun SLS.)
- Trace_id propagation: Fy-api emits `X-Oneapi-Request-Id` (`middleware/request-id.go`). TraceNexBiz must propagate it on internal API calls and log it everywhere. Then JOINs across SLS/Loki can correlate "user clicked refund → fy-api topup → revenue_log entry".
- Wallet-balance-consistency metric: `partner_wallet.balance` should always equal `sum(partner_wallet_log.amount)`. A nightly job emits a `partner_wallet_drift` gauge and pages on non-zero.

### M5. Phase 1 "绕过钱包" creates a one-shot data migration

§12 Phase-1 scope: "M3-04 额度分配 - 从平台直拨给客户，绕过钱包". Then Phase-2 adds the wallet. **At Phase-2 cutover, what's the partner's opening wallet balance?**

Either:

1. Phase 1 records "would-have-been wallet ledger entries" in a placeholder table that Phase 2 sums into opening balance. (Best.)
2. Phase 2 cuts over and partners get a $0 starting balance, retroactive computation per partner manually. (Awful UX, accounting nightmare.)

Pick (1) and call it out in v0.2.

### M6. Operational topology unspecified

PRD doesn't say:

- Is TraceNexBiz a **separate Podman container in the same `/opt/fy-api/` host** (today's CN-prod single-host) or a **separate K8s deployment** (SG today is blue/green via Nginx, `scripts/prod/06-deploy-blue-green.sh`)?
- Same `scripts/prod/06-deploy-blue-green.sh` or its own?
- Do we need a third domain (`partner.tracenex.cn` per §5.1) and a TLS certificate procurement track?

This isn't blocking for v0.2 but should appear in §12 milestones with a "Week 0: ops topology decided" task.

### M7. Migration of existing TraceNex direct customers is unaddressed

PRD §3 mentions "个人直销用户：不通过渠道商，直接和平台签约（即 TraceNex 现有用户）". Schema has `customer.partner_id` non-nullable in §8.2 (no `*int64`). But existing direct users are not customers of any partner — they don't get a row in `customer` at all. v0.2 must confirm the schema (yes — direct users live only in Fy-api `users`) and document the rule in §6.4 explicitly. Reports that aggregate "platform-side customers" must filter `users` by "no row in `customer` AND no row in `partner`".

---

## Section-by-section feedback

### §5 Architecture

- Diagram lacks Redis (which both systems use as hard dep). Add it.
- Diagram lacks log destination (SLS/Loki/Promtail). Add it.
- "同实例不同库" decision matrix is fine. But add a row "迁移路径到独立实例" — the answer is: physical separation requires (a) breaking the JOIN-based reports into application-side aggregation; (b) replacing reads-via-direct-DB with API calls to Fy-api. Nontrivial. Document the migration trigger ("if `transnext_db` exceeds 50% of RDS connection pool, split").
- "事务跨库需 XA（不推荐）" — fine, but the saga design (§9.4) needs to be the documented contract for ALL cross-system mutations, including refunds and adjustments.
- The diagram should show the LOG_DB possibility — even if today CN-prod has logs in the same instance, the architecture allows split, and the partner reports must work either way.

### §6 Fy-api integration

- §6.1 — make `Idempotency-Key` header mandatory on every mutating endpoint. Add `GET /api/internal/topup/by-idem-key` and `GET /api/internal/group-mutation/by-idem-key` for crash-recovery (see C5).
- §6.1 auth — pick **HMAC-SHA256 over (timestamp || method || path || body-sha256) + 5-min skew window + key-id rotation header**. Static API key alone is a footgun for log leaks. Document in v0.2 with the canonical-string-to-sign spec; this is non-trivial security surface and the security reviewer will (rightly) bounce hand-waving. mTLS is overkill for in-cluster traffic if the Fy-api/TraceNexBiz pods share an internal network and are TLS-terminated at the LB; HMAC + cert-pinned client suffices.
- §6.2 "无回调" — **wrong** if we adopt outbox (§H3). Even with CDC, "Fy-api → TraceNexBiz" is a logical reverse flow. Restate as "no synchronous callback; eventual outbox/CDC consumer subscribes".
- §6.3 the read-only-DB enforcement story needs the GRANT spec (see H2).
- §6.4 身份升级 — what about **downgrade** (revoke partner status)? Edge case but inevitable. Spec the partner-revocation flow: customers → reassign to platform-direct? Or freeze partner customers' tokens? v0.2 must answer.

### §8 Data model

- `Wallet.Balance int64` is correct, `User.Quota int` (int32 in MySQL) is the bottleneck — see C4.
- Add `wallet_hold` table (H1).
- `revenue_log.FyApiLogId int64` — `Log.Id` in Fy-api is `int` (`model/log.go:20`), MySQL INT (32-bit). Same overflow concern at scale. Either widen Fy-api side, or accept INT range and document.
- `audit_log.DiffJson string` — at scale this grows fast. Add an `audit_log_archive` partition strategy and a retention period (PRD §10 says "≥6 月"; pin a default of 12 months).
- `Partner` is missing a `closed_at` / `frozen_reason` for revocation flow.
- `Customer.QuotaLimit` is per-customer; what about per-partner aggregate limit (e.g. partner can't allocate more than X to their customers in aggregate)? Probably needed for risk control.
- `RevenueLog` lacks an idempotency key — if outbox is replayed twice (replay during incident), we'll double-count revenue. Add `(fy_api_log_id, occurrence)` as a UNIQUE index, where `occurrence` is 1 normally and >1 only for explicit reversals/adjustments.
- `KycApplication.LegalPersonName/IdNo/AlipayOpenId/AlipayRealName` — encrypting in app works, but you need a KMS key-id column to support re-encryption when keys rotate. Add `EncryptionKeyId int` to every encrypted-PII table.

### §9 Billing pipeline

Reworked recommendation in priority order:

1. Drop B-2 (CDC) as the recommended path. **Use outbox** in a Fy-api overlay (`logs` insert + `consume_log_outbox` insert in same TX). TraceNexBiz polls outbox at 1-s tick; truncates after consume.
2. Move the markup mechanism from "one group per (partner, customer)" to **per-user `GroupRatioOverride`** (overlay schema change on `User`). Reduces group cardinality from (partners × customers) to (partners + 0). Big architectural win; bigger overlay. If we keep one-group-per-customer, cap at MVP+Phase-2 then plan migration.
3. Pin the propagation latency contract: <2 s on price changes, achieved via Redis pubsub on `UpdateOption(GroupRatio*)` — overlay change.
4. §9.4 saga: spec idempotency keys, saga_step table, recovery loop, by-idem-key lookup.

### §11 Risks

- Add: "Fy-api overlay maintenance burden during monthly upstream sync". Each overlay file is conflict-prone. Mitigate by isolating overlays in new files per `Fy-api/CLAUDE.md` overlay strategy.
- Add: "Outbox or CDC backlog blows DB disk during incident". Cap outbox table size; alert at 80% threshold.
- Upgrade "渠道商 group_ratio 改价时计费瞬间不一致" to 🔴 if we accept a 60-s window. Or fix and downgrade after pubsub overlay lands.

### §12 Milestones

Phase 1 must include:

- Fy-api overlay PR (internal API + auth + GroupRatio pubsub).
- Ops topology decision.
- DB widen `User.Quota`/`Log.Id` to BIGINT migration.

Otherwise Phase 1 ships a system that physically cannot talk to Fy-api at scale, or does so via API endpoints that don't exist yet.

---

## Concrete code-level checks performed

- **`Fy-api/model/user.go:23-53`** — `User` struct: `Id int`, `Group string` is writable (no read-only tag), `Quota int` with `gorm:"type:int"` → MySQL INT 32-bit. `Group` defaults to `'default'`. `GroupRatioOverride` does **not** exist — would need overlay.
- **`Fy-api/model/user.go:818-843` `GetUserGroup`** — caches in Redis via `getUserGroupCache`. So writing User.Group via internal API must invalidate the cache; existing path is `Update`/`Edit` which presumably touches the cache, but I don't see a single-key Redis invalidation in `Edit`. Verify before implementing the internal endpoint, or piggyback on `Edit`.
- **`Fy-api/setting/ratio_setting/group_ratio.go`** — `groupRatioMap` is in-process `RWMap[string, float64]`. Persisted as a single JSON blob in `OptionMap["GroupRatio"]` = single row in `options` table. `GetGroupRatio(name)` returns 1 silently when not found ("group ratio not found: X" is logged). **This is the propagation choke point.**
- **`Fy-api/model/option.go:191-207` `loadOptionsFromDatabase` + `SyncOptions`** — pull-only, every `SyncFrequency` (default 60 s, `common/init.go:102`). No pubsub.
- **`Fy-api/main.go:95`** confirms `go model.SyncOptions(common.SyncFrequency)`.
- **`Fy-api/model/log.go:189-253` `RecordConsumeLog`** — writes via `LOG_DB.Create(log)`. `LOG_DB` may be a separate instance (`Fy-api/model/main.go:41-66, 214-230`). `Log.Id int`, `Log.Group string`, `Log.Quota int`. Outbox could live alongside in the same `LOG_DB` transaction.
- **`Fy-api/router/api-router.go`** — confirmed: no `/api/internal/*` group. Self-route topup at line 88 is `selfRoute.POST("/topup", middleware.CriticalRateLimit(), controller.TopUp)` — user-context, not service-context.
- **`Fy-api/controller/topup.go:495-507` `AdminCompleteTopUp`** — admin-only path. Uses admin auth, takes `tradeNo`. Could be repurposed for internal-API if augmented with idempotency key, but the cleaner answer is a new `controller/internal_topup.go`.
- **`Fy-api/CLAUDE.md`** Rule 2 — confirms triple-DB compat (SQLite/MySQL/PG). Confirms canal/maxwell as MySQL-only is a violation of the project's portability charter.
- **`Fy-api/CLAUDE.md`** overlay strategy — "Prefer new files over edits to upstream files" — internal API is a perfect candidate for new files; just must be tracked in `OVERLAY.md`.
- **`Fy-api/service/text_quota.go:142-260`** — billing math uses `decimal.NewFromFloat(summary.GroupRatio)` and reads `relayInfo.PriceData.GroupRatioInfo.GroupRatio`. So group ratio is resolved per-request from in-memory map (via the `GetGroupRatio` path) — confirming C2's concern about pod-local cache.
- **`Fy-api/model/subscription.go:1-30`** — uses `cachex` and `samber/hot`. Subscription billing is heavily cached. Subscription as a partner-overlay candidate exists in `controller/subscription.go` but PRD doesn't lean on it; fine.

---

## Decisions still missing

- Fy-api overlay scope and ownership (who owns the PR? the same Fy-api maintainer?).
- DB user privileges spec for `tracenex_biz_ro` MySQL user.
- Settlement timezone policy.
- BIGINT widening for `User.Quota` and `Log.Id` — Phase 1 must include.
- group_ratio propagation push mechanism (Redis pubsub vs polling cut to 5 s).
- Outbox vs CDC vs webhook for revenue capture (recommend outbox).
- Per-user `GroupRatioOverride` vs per-customer-group strategy for v1.0.
- Ops topology (Podman on shared host vs K8s deployment).
- Wallet hold/two-phase-commit modeling.
- Saga idempotency key contract and `GET /api/internal/...by-idem-key` endpoints.
- Cron lock backend (K8s lease > Redis SETNX).
- Customer/partner trace_id propagation contract (`X-Oneapi-Request-Id` everywhere).
- Partner-revocation flow.
- Migration plan for existing TraceNex direct customers (PRD doesn't address — they probably stay in Fy-api as `partner_id IS NULL` customers, but spec it).
- Phase-1 → Phase-2 wallet opening-balance reconciliation.
- KMS key rotation and encrypted-PII column versioning.

---

## Recommendation

**Verdict: NEEDS_REVISION.** Author should produce v0.2 addressing CRITICAL items C1-C5 with explicit answers, plus HIGH items H1-H6. The architectural skeleton is right; the integration plumbing is fictional in the current draft and that's the highest-risk surface area.

**Concrete next steps**:

1. **Spike a Fy-api overlay PR** (1 day) for internal-API + HMAC auth + Redis-pubsub option invalidation. Validates that "Fy-api needs ~10 files of overlay, not 0" is true and gives a real estimate.
2. **Spike outbox alternative** (1 day) — write a 50-line Fy-api overlay doing TX-coupled outbox-write; benchmark consume latency.
3. **Spike wallet hold pattern** (0.5 day) — confirm two-phase-commit table works under contention with optimistic-lock retries.
4. Rewrite §6 and §9 around the spike findings; rewrite §12 Phase 1 to include the overlay.
5. Resubmit as v0.2.

When v0.2 lands, I expect this becomes ACCEPT_WITH_NOTES — the rest of the data model and the role/scope decisions are sound.
