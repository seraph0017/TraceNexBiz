# PRD v0.2 Review — Software Architect (Round 2)

> Date: 2026-05-09
> Reviewer: Architect agent
> Verdict: **ACCEPT_WITH_NOTES** (promote to v1.0; address H-1 / H-2 / H-3 in a v1.0-rc patch before Phase-1 sprint kickoff)

---

## TL;DR

v0.2 substantively closes every Round-1 CRITICAL with code-level specificity. The five blockers (C1 overlay, C2 propagation, C3 CDC→outbox, C4 BIGINT, C5 saga recovery) are no longer "fictional plumbing" — they are concrete file paths, table DDL, and step-by-step state transitions. §8.5 `wallet_hold` + §9.4 saga together are now a recognizable two-phase commit. §6.3 GRANT spec is real. Appendix C is honest about LOC and conflict surface.

What remains are **integration gaps surfaced by the new design**, not design holes. They are 3 HIGH and 4 MEDIUM. None block v1.0 promotion under the rule (CRITICAL=0, HIGH≤3) — but H-1 (outbox grant + LOG_DB location), H-2 (saga "unknown" bounded retry), and H-3 (SQLite BIGINT migration recipe) want to be patched into a v1.0-rc before code freeze, since they directly affect the Day-1 GRANT statements and the OVERLAY.md plan.

The LOC estimate in App-C is realistic (~575 incl. tests), but the v0.2 banner claim of "200-300 LOC" contradicts §C-9's own number. Cosmetic, but fix it.

---

## CRITICAL resolution audit (Round-1 → v0.2)

### ✅ C1 — "Fy-api 不动一行代码" (撤回)

**Status: RESOLVED at engineering-actionable detail.**

Appendix C lists 7 sub-overlays with file paths, LOC, and risk:

| Overlay | Files | LOC | Verdict |
|---|---|---|---|
| Internal router + auth | `router/api-internal-router.go`, `middleware/internal_auth.go` | 130 | concrete |
| Internal controllers | `controller/internal_{user,token,usage,group,idempotency}.go` | 240 | concrete |
| BIGINT migration | `migrations/2026_05_xx_widen_quota_to_bigint.sql` | 80 | partially concrete (see H-3) |
| `User.GroupRatioOverride` | patch on `model/user.go`, `setting/ratio_setting/group_ratio.go` | 85 | needs cache-invalidation note (M-1) |
| Outbox | `migrations/...consume_log_outbox.sql`, new `model/log_outbox.go`, patch `model/log.go::RecordConsumeLog` | ~50 | mostly concrete (see H-1) |
| Pub/Sub | patch `model/option.go::UpdateOption`, patch `main.go::startup` | 40 | concrete |
| Internal idempotency | new `model/internal_idempotency.go` + migration | 50 | concrete |

**Engineering can implement this without further questions.** Three caveats:

1. The "约 200-300 LOC" framing in the v0.2 change banner contradicts §C-9's own "合计 ~575 / 核心 Go 250-300". Pick one number across the doc; engineers who read only the banner will plan wrong sprints. Recommend "~300 LOC 核心 Go + ~250 LOC SQL/测试 = ~575 LOC 合计" everywhere.
2. App-C touches **upstream-owned files** (`model/option.go::UpdateOption`, `main.go::startup`, `model/log.go::RecordConsumeLog`, `model/user.go`, `setting/ratio_setting/group_ratio.go`, `common/init.go`) — the conflict-prone class per `Fy-api/CLAUDE.md` "Customization Strategy". R-19 acknowledges this; **update `OVERLAY.md` with B-8..B-14 entries (B-7 is already taken by channel-benchmark — renumber) as part of the Phase-1 PR**, not a follow-up.
3. The `UpdateOption` Pub/Sub patch must publish *after* `DB.Save(&option)` commits and *before* returning from `UpdateOption`. If a subscriber pulls the row before commit it sees stale data. Document the order in §C-6.

### ✅ C2 — group_ratio 60-second propagation

**Status: RESOLVED.**

§9.2 + §C-6 specify Redis Pub/Sub publish from `model/option.go::UpdateOption` plus per-pod subscribe. Fallback is `SyncOptions(SyncFrequency)` cut from default 60→5s via `biz_setting.sync_freq_seconds`. §10.1 contracts <2s for `group_ratio` propagation.

I verified `Fy-api/model/option.go:209-223` (`UpdateOption`) is the exact integration point — single function, one Save + one updateOptionMap. Hooking a publish here is ~6 LOC; the subscriber goroutine in `main.go:95` is ~30 LOC as estimated.

**Realistic worst case** is the 5s fallback window (a pod that crashed mid-Pub/Sub and missed the message — Redis Pub/Sub has no persistence). Document the contract as "<2s nominal, ≤5s worst case" — surface this in §10.1 row 4. Currently §10.1 says "<2s" full stop.

If a stronger guarantee is wanted later, switch to Redis Streams with consumer groups (each pod has a unique consumer name, missed messages are XPENDING-replayed on reconnect). Not blocking for v1.0.

### ✅ C3 — CDC plan was MySQL-only

**Status: RESOLVED.**

§9.3 swaps canal/maxwell for application-level outbox. The B-1/B-2/B-3 comparison table in §9.3 is honest about the trade-off. Outbox table DDL in §8.19 is portable in concept (BIGINT, VARCHAR, TIMESTAMP, no MySQL-isms in the column types).

But the swap surfaces a new wrinkle the doc hasn't fully internalized — see H-1 below (where the outbox table physically lives, and what GRANTs the consumer needs). Architecture-correct; deployment-incomplete.

Also: §8.19 declares the table via raw `CREATE TABLE … AUTO_INCREMENT …`. That works on MySQL only. Per `Fy-api/CLAUDE.md` Rule 2 the migration must use GORM AutoMigrate or three-dialect SQL (`BIGSERIAL` / `INTEGER PRIMARY KEY AUTOINCREMENT` / `BIGINT AUTO_INCREMENT`).

### ✅ C4 — `User.Quota` int32 overflow

**Status: RESOLVED at strategy level; SQLite recipe missing (see H-3).**

§C-3 commits to a 3-dialect migration widening `users.quota` / `logs.id` / `logs.quota` to BIGINT. I verified the columns are actually 32-bit at `Fy-api/model/user.go:39-47` (`Quota int gorm:"type:int"`) and `Fy-api/model/log.go:20,28` (`Id int`, `Quota int`). The fix is necessary and correct.

Two gaps:

- The **SQLite recipe** is hand-waved. Per `Fy-api/CLAUDE.md` Rule 2: *"`ALTER COLUMN` in SQLite (unsupported — use column-add workaround)"*. So a `migration.sql` with `ALTER TABLE users ALTER COLUMN quota TYPE BIGINT` will pass on PG, fail on MySQL (`ALTER` syntax differs), and fail on SQLite (no ALTER COLUMN at all). See H-3.
- v0.2 widens 3 columns but `Fy-api/model/user.go:40-47` shows 4 more `type:int` fields that follow the same logic: `UsedQuota`, `RequestCount`, `AffQuota`, `AffHistoryQuota`. `UsedQuota` is the cumulative consumption counter — overflows just as silently as `Quota`, just less visibly. Add to App-C migration.

### ✅ C5 — Saga step-4 recovery

**Status: RESOLVED for the deterministic branches; "unknown" branch is unbounded (see H-2).**

§9.4 specifies all four cases (2xx confirmed, 4xx deterministic-fail, 5xx ambiguous, timeout). The "by-idem-key probe" path is real:

- §6.1 lists `GET /api/internal/topup/by-idem-key?key=` and `GET /api/internal/group/by-idem-key?key=` (controller `controller/internal_idempotency.go`, App-C C-2).
- §C-7 plus §18.3 give Fy-api-side idempotency a 7-day TTL (vs §18.2 TraceNexBiz-side 24h — see L-2).
- §8.17 `saga_step` table persists in-progress state for after-restart replay.
- §14.6 saga state machine has explicit `fy_topup_unknown` state with retry edge.

The wallet_hold semantics in §8.5 + §9.4 step 3 are correct two-phase commit:

- held → balance unchanged, available = `balance - Σ(held)`
- committed → balance reduced, hold row stays in committed (auditable)
- released → no balance change, hold marked released

This is the right pattern; matches Stripe / PayPal money-movement docs.

---

## NEW HIGH issues introduced/exposed by v0.2

### 🟠 H-1. Outbox table physical location vs the read-only GRANT contradict each other

`Fy-api/model/log.go::RecordConsumeLog` (line 244) writes into `LOG_DB`, which **may be a separate instance** — verified at `Fy-api/model/main.go:41,214-218`: when `LOG_SQL_DSN` is set, `LOG_DB` is a distinct `*gorm.DB`. §C-5 says the outbox is written **in the same transaction** as the `logs` insert. That is only achievable if `consume_log_outbox` lives in `LOG_DB` (cross-instance transactions don't exist without XA, which §10.4 explicitly forbids).

Now §6.3 says: *"TraceNexBiz 应用使用 user `tnbiz_app@%`（GRANT SELECT/INSERT/UPDATE/DELETE on `tracenex_biz_db.*`，**仅 SELECT** on `transnext_db.*`）"*. But §9.3 outbox poller does:

```sql
SELECT FROM consume_log_outbox WHERE consumed_at IS NULL ...
UPDATE consume_log_outbox SET consumed_at = NOW() WHERE id IN (...)
```

The poller **must UPDATE** outbox to mark consumed. Two issues:

1. The GRANT in §6.3 forbids it — TraceNexBiz needs `UPDATE` (or at least `UPDATE (consumed_at)`) on `consume_log_outbox`.
2. If `LOG_DB` is on a separate instance (verify SG via `fab info --target=sg`), the GRANT must be issued against the **log instance**, not the gateway instance. The GRANT spec in §6.3 says nothing about the LOG_DB scenario, despite §5.2 #3 acknowledging it elsewhere.

**Recommended patch to §6.3** (5 lines):

```sql
-- on the LOG instance (which may be transnext_db's instance, or a separate log RDS)
GRANT SELECT, UPDATE (consumed_at) ON <log_db>.consume_log_outbox TO 'tnbiz_app'@'%';
GRANT SELECT ON <log_db>.logs TO 'tnbiz_app'@'%';
-- application-side: detect LOG_SQL_DSN at startup, fail loud if grants missing
```

The column-level UPDATE limit (`UPDATE (consumed_at)`) is the safety belt — even with a buggy ORM call, you can't trash the rest of the row.

### 🟠 H-2. Saga "unknown" branch is unbounded

§9.4 step 4 (5xx / timeout) loops `GET /api/internal/topup/by-idem-key?key=` until "succeeded" or "failed". If Fy-api never persisted the idem-key (request lost in flight, e.g. LB→pod kill before DB write), the probe returns `unknown` indefinitely. The saga sits in `fy_topup_unknown` forever, the wallet_hold sits at `held`, and that partner's available balance permanently shrinks.

Required (1-screen patch to §9.4 + §14.6):

- Max retries `N=30`, exponential backoff (2s, 4s, …, capped at 5 min), wall-clock cap **1 hour**.
- After cap → flip saga to **`released_pessimistic`** state: `wallet_hold.status='released'`, write a `partner_wallet_log` of type `saga_aborted_unknown`, page on-call.
- Define an **operator override** action (§3.4 staff verb `saga.force_resolve`) that allows `super_admin` + `finance` dual-control to mark the saga as `succeeded` or `failed` after manual reconciliation.
- §14.6 currently has `fy_topup_known → committed | released`. Add `fy_topup_unknown → released_pessimistic` (timeout edge) and `released_pessimistic → committed | released` (operator override edge).

This is also a security/financial-consistency concern (R-7), but architecturally it's: don't ship a state that has no exit.

### 🟠 H-3. BIGINT migration on SQLite is hand-waved

§C-3 promises "PG / SQLite 兼容（migration 脚本三方言分支）". That is one line in the design doc; in practice SQLite < 3.35 has no `ALTER COLUMN`, and SQLite ≥ 3.35 only supports a narrow subset (RENAME COLUMN, DROP COLUMN). Per `Fy-api/CLAUDE.md` Rule 2 explicit prohibition: *"`ALTER COLUMN` in SQLite (unsupported — use column-add workaround)"*.

A concrete recipe must appear in §C-3 (or its own runbook) before any engineer commits to the sprint:

```
SQLite path (table-rebuild):
  BEGIN;
  CREATE TABLE users_new (id INTEGER PRIMARY KEY, quota INTEGER NOT NULL DEFAULT 0, ...);
  INSERT INTO users_new SELECT * FROM users;
  DROP TABLE users;
  ALTER TABLE users_new RENAME TO users;
  -- recreate indexes
  COMMIT;

PostgreSQL path:
  ALTER TABLE users ALTER COLUMN quota TYPE BIGINT;
  ALTER TABLE users ALTER COLUMN used_quota TYPE BIGINT;
  ...

MySQL path:
  ALTER TABLE users
    MODIFY COLUMN quota BIGINT NOT NULL DEFAULT 0,
    MODIFY COLUMN used_quota BIGINT NOT NULL DEFAULT 0,
    ALGORITHM=INPLACE, LOCK=NONE;
```

Also widen the columns v0.2 missed: `User.UsedQuota` (`model/user.go:40`), `User.RequestCount` (`:41`), `User.AffQuota` (`:45`), `User.AffHistoryQuota` (`:46`). All are `type:int`. Existing pattern for similar cross-dialect migrations is `migrateTokenModelLimitsToText` in `Fy-api/model/main.go` — reference it from §C-3.

---

## MEDIUM (not blocking, fix in v1.0-rc or Phase-1)

### 🟡 M-1. `User.GroupRatioOverride` cache invalidation is unspecified

App-C C-4 adds `User.GroupRatioOverride` to `model/user.go`. Fy-api's per-user cache is `getUserGroupCache` (`model/user.go:818-843`, Round-1 finding). The PRD specifies Pub/Sub for **option** changes (§9.2) but not for **per-user** field changes. When a partner sets a customer-level override via `PUT /api/internal/user/group_ratio_override`, the local pod can refresh, but other pods will see the old value until their `getUserGroupCache` TTL expires.

Fix (10 LOC): publish `Redis: user_update {user_id}` after the GORM Save in `internal_user.go::SetGroupRatioOverride`; subscribe in `main.go` and call `model.InvalidateUserCache(uid)`. State the contract in §C-4 and add a row to §10.1 ("user override propagation <2s").

### 🟡 M-2. Outbox row growth is unbounded

§8.19 marks `consumed_at = NOW()` but never deletes consumed rows. With production traffic (§10.1 target 200 QPS, plus all `logs.type=consume` entries), the outbox grows by the same row count as `logs`. The `idx_unconsumed (consumed_at, id)` partial-index helps the poll, but the table itself becomes a duplicate of `logs` over time.

§11 R-16 mentions "outbox 积压" but conflates *consumption lag* (a paging concern) with *long-term growth* (a disk concern). Two different problems. Required:

- TTL/archive: drop rows where `consumed_at < NOW() - 30d` (consumed_at is non-null; safe). Run as a daily cron alongside the existing `model/log.go::DeleteOldLog` mechanism (`Fy-api/model/log.go:520`).
- Capacity alarm in §10.6 observability: row count > 10M, page.

### 🟡 M-3. Settlement freshness gate is misformed

§9.3 says: *"settlement Cron 启动时检查：`MAX(occurred_at) FROM revenue_log` ≥ NOW - 60s. 如果不满足... refuse to run + 告警."*

Settlement runs at 02:30 monthly per §4.6. If the last billable request happened at 23:45 on month-end, `MAX(occurred_at)` is ~3h old at 02:30 the next day, the gate triggers, settlement refuses. False positive every single month-end.

Correct gate: *"`SELECT 1 FROM consume_log_outbox WHERE consumed_at IS NULL AND occurred_at <= period_end LIMIT 1` returns no rows"* — i.e., all outbox events that fall inside the closing period are consumed. Plus: outbox lag (= `consumed_at - logs.created_at`) p95 < 60s in the last hour.

Restate in §9.3 and §10.6.

### 🟡 M-4. `partner_wallet.HeldAmount` is denormalized vs `wallet_hold` table

§8.3 declares `partner_wallet.HeldAmount int64` and §8.5 declares the `wallet_hold` table. Round-1 H-1 asked for the latter; v0.2 added both. Now there are two sources of truth.

Either:

- **(preferred)** Drop `partner_wallet.HeldAmount`. Compute available as `balance - SUM(wallet_hold.amount WHERE status='held' AND wallet_id = X)`. Indexed by `wallet_id`; cardinality of held holds per partner is small, query is sub-ms.
- Or: keep both, but commit to a **drift detector** as a sibling of "wallet drift" gauge in §10.6 — assert `partner_wallet.HeldAmount == SUM(wallet_hold.amount WHERE status='held')` per partner; alarm on inequality.

Pick one, document the decision.

---

## LOW / nits

### L-1. `RevenueLog.Occurrence` allocation under concurrency

§8.7 declares `UNIQUE(fy_api_log_id, occurrence)` and `Occurrence int8`. §9.4 refund step 1 inserts a negative row with `occurrence=2`. Spec says nothing about how `occurrence` is allocated under concurrency (two refunds on the same log race for `occurrence=2`). Either:

- (a) allocate via `(SELECT COALESCE(MAX(occurrence),0)+1 FROM revenue_log WHERE fy_api_log_id=X) FOR UPDATE`, OR
- (b) define `occurrence` as a per-saga slot tied to `idempotency_key`.

Add the rule.

### L-2. Idempotency TTL inconsistency

§18.2 says `idempotency_record TTL = 24h`. §18.3 says `internal_idempotency TTL = 7d`. Saga retries can exceed 24h. After H-2 caps the wall-clock at 1h, both TTLs comfortably exceed it — but **document the invariant**: `idempotency_record TTL ≥ saga max wall-clock`.

### L-3. Banner LOC inconsistency

Banner: "约 200-300 LOC". §C-9: "合计 ~575". Pick one number, use everywhere.

---

## File-boundary check (Appendix C)

Cross-referenced each App-C entry against the actual Fy-api repo state:

| App-C item | Real path / line | Verdict |
|---|---|---|
| C-1 `router/api-internal-router.go` (NEW) | new — repo has only `router/api-router.go`, `router/relay-router.go`, `router/web-router.go` | ✅ clean new file |
| C-1 `middleware/internal_auth.go` (NEW) | new | ✅ clean new file |
| C-2 `controller/internal_*.go` (NEW) | new — pattern matches existing `controller/relay.go`, `controller/topup.go` | ✅ clean new files |
| C-3 BIGINT migration | touches `users`, `logs` schemas | ⚠️ recipe missing (H-3); column list incomplete |
| C-4 `User.GroupRatioOverride` patch on `model/user.go` | adds field at `model/user.go:23-53` struct | ⚠️ upstream-file patch — conflict-prone |
| C-4 `setting/ratio_setting/group_ratio.go::GetEffectiveGroupRatio` | new method on existing file | ⚠️ upstream-file patch — also bills hot path; benchmark before/after |
| C-5 `model/log_outbox.go` (NEW) | new | ✅ |
| C-5 `model/log.go::RecordConsumeLog` patch (~5 LOC) | patches body of `Fy-api/model/log.go:204-253` | ⚠️ **structurally undercounted**: existing `LOG_DB.Create(log)` is *not* inside a transaction; outbox-in-same-TX requires wrapping in `LOG_DB.Transaction(func(tx *gorm.DB) error {...})`. Real cost: ~25 LOC, not ~5. |
| C-6 `model/option.go::UpdateOption` patch (~10 LOC) | `Fy-api/model/option.go:209-223` | ✅ minimal patch achievable; ordering note required (C-1 caveat 3) |
| C-6 `main.go::startup` patch (~30 LOC) | adds subscribe goroutine alongside `go model.SyncOptions(common.SyncFrequency)` at `main.go:95` | ⚠️ upstream-file patch but trivial |
| C-6 `common/init.go` default 60→5 | edits `common/init.go:102` | ❌ **don't change the default in upstream-shared file** — set via `biz_setting.sync_freq_seconds`, read from `App` overlay's startup and call `common.SyncFrequency = N`. Otherwise upstream sync churns this row every month. |
| C-7 `model/internal_idempotency.go` (NEW) + migration | new | ✅ |

**Net assessment**: 5 of 7 sub-overlays are clean new files (good per `Fy-api/CLAUDE.md` strategy). 6 patches to upstream files (`model/option.go`, `main.go`, `model/log.go`, `model/user.go`, `setting/ratio_setting/group_ratio.go`, `common/init.go`) — all small individually, but each is a monthly-sync conflict point. R-19 in §11 already names this risk; please:

1. Drop the `common/init.go` default change (override at startup via biz_setting instead).
2. Re-estimate C-5 patch from "~5 LOC" to "~25 LOC" (TX-wrap is structural).
3. Tag every upstream-file patch with `// Fy-api overlay:` per `Fy-api/CLAUDE.md` Customization Strategy point 2.
4. Update `Fy-api/OVERLAY.md` with B-8..B-14 entries (B-7 is already taken by `channel-benchmark` — renumber) **as part of the Phase-1 PR**, not a follow-up.

Also: confirm scope of outbox in C-5. `Fy-api/model/log.go` has 6 `LOG_DB.Create(log)` callsites (lines 87, 112, 139, 183, 244, 292). The outbox should write *only* for `LogTypeConsume` (line 244, called from `RecordConsumeLog`). Other types — `RecordTopupLog`, `RecordErrorLog`, `RecordTaskBillingLog` — should **not** trip the outbox; revenue-log only cares about consume events. Spell this out in §C-5.

---

## Section-by-section delta from Round-1

| Round-1 issue | v0.2 location | State |
|---|---|---|
| C1 overlay | App-C, §6.1, §12.0 | ✅ resolved |
| C2 propagation | §9.2, §C-6, §10.1 | ✅ resolved (≤5s worst case; nominal <2s) |
| C3 CDC→outbox | §9.3, §8.19, §C-5 | ✅ resolved (modulo H-1 grant location) |
| C4 BIGINT | §C-3 | ⚠️ resolved at strategy; recipe missing (H-3) |
| C5 saga recovery | §9.4, §14.6, §8.17 | ✅ resolved (modulo H-2 bounded retry) |
| H1 wallet_hold | §8.5, §9.4 | ✅ resolved (one denorm to clean — M-4) |
| H2 read-only DB GRANT | §6.3 | ⚠️ partial (LOG_DB scenario unaddressed — H-1) |
| H3 B-1 vs B-2 cost | §9.3 (B-3 chosen) | ✅ resolved |
| H4 group cardinality | §9.2 (per-tier + override) | ✅ resolved (M-1 caching gap) |
| H5 cron lock | §M5-01, §14.3, §8.8 (settlement_run) | ✅ resolved |
| H6 P95 budget | §10.1 (split internal/round-trip) | ✅ resolved |
| M1 FK orphans | §11 R items + §4.17 PIPL | ✅ |
| M2 GroupNameInFyApi denorm | §8.2 | ⚠️ still denormalized; not blocking |
| M3 timezone | §4.6 explicit UTC+8 + `biz_setting.timezone` | ✅ |
| M4 observability | §10.6 trace_id, wallet drift, SLS | ✅ |
| M5 wallet opening balance | §12.1 "Phase 1 wallet from day 1" | ✅ |
| M6 ops topology | §12.0 "ops 拓扑决定" Week 0 task | ✅ |
| M7 direct customers | §4.13, §6.4, schema (`partner_id *int64`) | ✅ |

---

## Verdict

**ACCEPT_WITH_NOTES — promote v0.2 to v1.0.**

- CRITICAL = **0** (was 5)
- HIGH = **3** (was 6) — all are deltas exposed by v0.2's better architecture, not architectural holes
- MEDIUM = **4**, LOW = **3**

Under the "CRITICAL=0, HIGH≤3" rule, this passes. The 3 HIGH should be patched into a v1.0-rc (≤2-page diff, ~half-day of editing) before the Phase-1 sprint kicks off, since they all directly affect `Fy-api/OVERLAY.md` planning and the GRANT statements ops will issue on day 1.

### v1.0-rc patch list (mandatory before sprint kickoff)

1. **§6.3 GRANT spec**: add `LOG_DB` scenario with column-level UPDATE on `consume_log_outbox.consumed_at`. Spell out the GRANT for both same-instance and split-LOG_DB topologies.
2. **§9.4 + §14.6 saga**: bound the `fy_topup_unknown` retry (N=30 / 1 hour wall-clock cap → `released_pessimistic` + alarm + dual-control operator override).
3. **§C-3 BIGINT recipe**: explicit three-dialect SQL (SQLite table-rebuild path, PG `ALTER COLUMN`, MySQL `MODIFY COLUMN ALGORITHM=INPLACE`). Add `UsedQuota`, `RequestCount`, `AffQuota`, `AffHistoryQuota` to the column list. Reference `migrateTokenModelLimitsToText` pattern in `Fy-api/model/main.go`.

### v1.0 follow-ups (post-kickoff, before Phase-1 close)

4. M-1 user-cache pubsub for GroupRatioOverride (§C-4)
5. M-2 outbox TTL + capacity alarm (§8.19, §10.6, §11)
6. M-3 settlement freshness gate restated (§9.3, §10.6)
7. M-4 drop `partner_wallet.HeldAmount` denorm OR add drift detector (§8.3, §10.6)
8. L-1 occurrence allocation rule (§8.7, §9.4)
9. L-2 idempotency TTL ≥ saga wall-clock (§18.2/3)
10. L-3 LOC numbers consistent across banner / §C-9
11. C-1 Pub/Sub publish-after-commit ordering note (§C-6)
12. C-5 re-estimate to ~25 LOC; clarify scope (consume only, not all log types)
13. C-6 drop `common/init.go` default change; route through `biz_setting`
14. Tag every upstream-file patch with `// Fy-api overlay:`; append B-8..B-14 to `Fy-api/OVERLAY.md` in the same PR (B-7 is taken by channel-benchmark)

---

## Concrete code-level checks performed (Round-2)

- **`Fy-api/model/option.go:191-223`** — verified `loadOptionsFromDatabase` + `SyncOptions` + `UpdateOption`. App-C C-6 integration point is exact: `UpdateOption` is a single function with one `DB.Save(&option)` + one `updateOptionMap`. Pub/Sub publish hook is ~6 LOC right before `return updateOptionMap(...)`.
- **`Fy-api/model/log.go:189-253`** `RecordConsumeLog` — confirmed `LOG_DB.Create(log).Error` is a single Create *outside* any transaction. App-C C-5 "~5 LOC patch" is structurally undercounted; needs a `LOG_DB.Transaction(func(tx *gorm.DB) error {...})` wrap covering both the `logs` insert and the `consume_log_outbox` insert. Re-estimated ~25 LOC. Five other `LOG_DB.Create(log)` callsites (87, 112, 139, 183, 292) are non-consume types — not in scope.
- **`Fy-api/model/main.go:41-66, 214-218`** — confirmed `LOG_DB` may be a separate `*gorm.DB` (separate connection, possibly separate instance). Cross-instance TX is impossible. Outbox MUST live in `LOG_DB`. v0.2 §6.3 GRANT spec doesn't acknowledge.
- **`Fy-api/model/user.go:39-47`** — confirmed `Quota int gorm:"type:int"`. Same `type:int` constraint applies to `UsedQuota`, `RequestCount`, `AffQuota`, `AffHistoryQuota`. v0.2 §C-3 widens 3 columns; should widen 7.
- **`Fy-api/model/log.go:20,28`** — confirmed `Log.Id int`, `Log.Quota int`. v0.2 §C-3 widens both. ✅
- **`Fy-api/CLAUDE.md` Rule 2** — confirmed *"`ALTER COLUMN` in SQLite (unsupported — use column-add workaround)"*. v0.2 §C-3 hand-waves this.
- **`Fy-api/CLAUDE.md` Customization Strategy** — confirmed: prefer new files, tag upstream-file patches with `// Fy-api overlay:`, update `OVERLAY.md` in the same commit. v0.2 §C-8 mentions OVERLAY.md but doesn't enumerate the conventions.
- **`Fy-api/OVERLAY.md`** — confirmed B-1..B-7, B-1.1, F-1..F-6, X-1..X-7 layout. App-C will add B-8..B-14 (B-7 already taken by `channel-benchmark`; renumber to avoid collision).
- **`Fy-api/main.go:95`** — confirmed `go model.SyncOptions(common.SyncFrequency)`. App-C C-6 will add a sibling `go model.SubscribeOptionInvalidations(...)` line. Trivial diff.
- **`Fy-api/router/api-router.go`** — confirmed no `/api/internal/*` namespace; clean greenfield for App-C C-1.

---

## Summary

PRD v0.2 passes the v1.0 promotion gate (CRITICAL=0, HIGH=3 ≤ 3). The above v1.0-rc patch list (items 1-3) is the editorial pass before sprint kickoff. The remaining items (4-14) can be cleaned up during Phase-1 implementation without blocking.

The architectural skeleton is now load-bearing: integration boundaries are real code paths, not narrative; the saga is a recognizable two-phase commit; the cross-DB story honors `Fy-api/CLAUDE.md` Rule 2; and the Fy-api overlay surface is small enough that the monthly upstream-sync ritual can absorb it.
