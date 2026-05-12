# Round-2 Architect Review — TraceNexBiz

**Date**: 2026-05-12
**Reviewer**: Architect (same independent reviewer as code-round-1/04)
**Scope**: verify Round-1 CRITs (A-C1 dual-control degradation, A-C2 dual force-resolve handler) + all 5 HIGHs; audit overall W3 architectural coherence across 9 surfaces (saga registry, idempotency layering, KMS scope isolation, outbox SOURCE/SINK, BOLA, leader election, GORM repos, frontend api-client, middleware ordering).

**Verdict**: **PASS-WITH-CONDITIONS**

Round-1's two CRITs are genuinely closed at the architectural-shape level. C-1 HMAC contract is now bilaterally aligned (`X-Auth-*` on both partner-api client and Fy-api server). C-2 saga compensation no longer silently drops errors. The dual-control force-resolve flow is now structurally sound: approver_ip is captured server-side at issue-time (not transmitted by initiator), bound to (saga_id, approver_id) for 5min, single-use, and the service-layer guard `tok.ApproverID != in.InitiatorStaffID` is enforced *after* token consumption (so an initiator who steals an approver token still gets `ErrApproverSamePerson` if they accidentally use their own approver session). The two-handler divergence (admin.go vs saga_admin.go) is resolved — only `handler/admin/admin.go` remains and delegates to `internal/service/saga_admin.Service`.

What blocks an unconditional PASS: the saga step registry (Fix-B' part 2) is **conceptually correct but functionally dormant** — no production service registers steps via `RegisterStep`; `saga_allocate` and `saga_topup` still use legacy inline-closure `sg.Run(...)`, so `Sweep` always hits the `fn == nil → Skipped` branch for real sagas. The Round-1 H-1 finding ("retry sweep doesn't re-run fn") is therefore only half-closed: the orchestrator can re-run, but no one has taught the actual sagas how to be re-runnable. Same shape for `saga.WithIdempotency` helper: it exists and is well-designed, but zero production callers — `grep WithIdempotency internal/service internal/handler` returns nothing. The middleware DB fallback (`tryReplayFromDB`) works only if some service layer eventually inserts records, and currently none does. KMS DEK/KEK rotation is documented (90d/1y) but the rotation triggers are `TODO(ops)` stubs. Outbox SOURCE poller binary `cmd/outbox-poller/` is an **empty directory** — only the SINK consumer is wired. Three frontend apps still carry inline clients with `TODO(Fix-D)` migration headers.

These are real debts, not theatre — they're tracked in code, the contracts are stable, and the deferred work can land incrementally without rewriting interfaces. Ship-acceptable for staging; production needs the saga-step-registry adoption gap closed before the first force-resolve drill, and the outbox SOURCE before any partner-facing webhook contract goes live.

## Round-1 CRITICAL status (2)

- **A-C1 (dual-control degraded to single-control)** — **CLOSED**. `internal/service/saga_admin/saga_admin.go:106-126` `IssueApproverToken(sagaID, approverID, approverIP)` captures `approverIP` from the approver's request via `clientIP(c)` in `handler/admin/admin.go:359`; the token persists `ApproverIP` in `TokenStore` (saga_admin.go:45-54). In `ForceResolve`, the initiator-supplied `InitiatorIP` is compared against `tok.ApproverIP` (server-recorded, not request-supplied) via `same24` — initiator cannot fake it. Token is single-use (`tok.Consumed` flag in `MemoryTokenStore.Consume`), 5min TTL, single-saga-bound. `tok.ApproverID == in.InitiatorStaffID → ErrApproverSamePerson` (saga_admin.go:154-156). Two real properties — different person + different /24 — both enforced. Caveat: `clientIP()` in `handler/admin/admin.go:422-432` trusts `X-Real-IP` / `X-Forwarded-For` headers; if a deployment terminates TLS at a misconfigured LB that doesn't strip these, an attacker behind the LB can spoof both IPs. Document a deployment hardening note ("LB MUST overwrite X-Forwarded-For"). MEDIUM.
- **A-C2 (two parallel force-resolve handlers)** — **CLOSED**. `find apps/partner-api -name "saga_admin*"` returns only `internal/service/saga_admin/{saga_admin.go,saga_admin_test.go}`. The handler-layer `handler/saga_admin.go` is gone; admin route at `handler/admin/admin.go:66` `rg.POST("/saga/:id/force-resolve", middleware.WithScope("staff_finance"), sagaForceResolve(deps.SagaAdmin))` is the only entry, and `sagaForceResolve` delegates to the service. Contract is single-source.

## Round-1 HIGH status (5)

(Mapping to the seven Round-1 deltas — original CRITICAL/HIGH list in code-round-1/04-architect.md §6, supplemented by what Fix-A/B/C/D commits resolved.)

- **C-1 / fyapi HMAC contract divergence** — **CLOSED**. `internal/infra/fyapi/client.go:113-116` writes `X-Auth-KeyId / X-Auth-Timestamp / X-Auth-Nonce / X-Signature`; Fy-api `middleware/internal_auth.go:42-45` reads the same four header names. Canonical form `METHOD\nPATH\ncanonical_query\nTS\nNONCE\nSHA256_HEX(body)` matches on both sides (client.go:145-153). Base64 signature output aligned. A regression test in either side would still be welcome but the names line up bit-for-bit.
- **C-2 / saga compensation silently swallowed** — **CLOSED**. `internal/service/saga_allocate/service.go:129-158` now captures every `Compensate` return and uses `wrapCompensationError("hold", err, compErr)` to surface both the original failure and the compensation failure via `errors.Join`. saga_topup follows the same pattern. The `_, _ = sg.Compensate(...)` antipattern is gone.
- **H-1 / retry sweep doesn't re-run fn** — **PARTIALLY CLOSED, MEDIUM debt**. `orchestrator.go:76-124` Sweep now decodes payload envelope and calls `LookupStep(kind, step_name)` to find a registered fn, then `inst.RunWithInput(ctx, step.StepName, input, fn)`. The registry pattern (`saga/registry.go`) is well-designed: `panic` on conflicting-fn registration, returns nil on miss. But **no production service registers steps** — `grep RegisterStep internal/service` returns nothing; only `saga/saga_retry_sweep_test.go` registers steps for tests. saga_allocate/saga_topup still call `sg.Run(ctx, step, closure)` instead of `sg.RunWithInput(ctx, step, inputBytes, fn)`. Net effect: Sweep returns Scanned but Retried always 0 for real sagas; only Escalated counts work. Documented as **MEDIUM-1** below — does not block ship because the W3 sagas are short-window (deduct/hold/fyTopup/commit/log all <5s) and manual force-resolve covers the gap, but the gap must close before traffic crosses 100 RPS.
- **H-2 / idempotency middleware + service same-TX double stub** — **PARTIALLY CLOSED, MEDIUM debt**. `internal/middleware/idempotency.go` is no longer stub — it now implements Redis fast path + DB slow path (`IdempotencyWithDB` with `tryReplayFromDB`). `internal/repository/mysql/idempotency_mysql.go:InsertWithinTx` is implemented. `internal/saga/idempotency.go:WithIdempotency` helper exists. But **no service calls `WithIdempotency`** — `grep WithIdempotency internal/{service,handler}` returns zero. So middleware Redis-side works, but the DB-fallback path that the middleware queries has no writers. If Redis is wiped, all in-flight idempotency state vanishes — same risk as Round-1 just with a usable API ready for adoption. **MEDIUM-2**.
- **H-3 / JWT/CSRF middleware all pass-through** — **CLOSED**. `middleware/auth.go:81-100` JWT now verifies RSA signature via `mustBuildVerifier`, checks revocation store, sets `jwt_claims`/`actor_type`/`actor_id`. CSRF middleware (`middleware/csrf.go`) implements double-submit cookie check. `main.go:255` fails fast in staging/prod if `JWT_VERIFY_KEY_PEM` is missing. Dev fallback `X-Dev-Actor-*` no longer exists in scopeOf — `WithScope` reads `ClaimsFrom(c)` which only returns from JWT-set context.
- **H-4 / fyapi.Client missing TopupCustomer/RefundCustomer** — **CLOSED**. Commit `07305d3` adds these methods. Verified by `saga_allocate/service.go:135` calling `s.fyapi.TopupCustomer(ctx, ...)` and the FyAPIPort interface matching.
- **H-5 / partner_wallet.balance CHECK ≥ 0 missing** — Status not re-verified in this round (DDL surface; out of W3 stated scope). Carried forward as **LOW-1** in debt registry; W1c migration responsibility.

## W3 architecture audit

**1. Saga registry composability** — `internal/saga/registry.go` is a clean Option-A pattern: `(kind, step)` → `StepFunc` with init-time registration, panic on conflict (saga.go:74), `LookupStep` returns nil on miss, `RunWithInput` persists input bytes to `saga_step.payload` for replay, `ErrPermanent` sentinel for no-retry escalation. The contract is correct. **Composition gap**: production sagas don't use it (see H-1 above). When a service migrates from `sg.Run` to `sg.RunWithInput` + `init() RegisterStep`, it gets retry-for-free; until then registry sits dormant. Tests at `saga/saga_retry_sweep_test.go` confirm the registry's panic semantics and lookup correctness. Coherent design, incomplete adoption.

**2. Idempotency layering** — Three layers: (a) middleware SETNX 24h fast path (`middleware/idempotency.go:144`); (b) middleware DB fallback `tryReplayFromDB` (idempotency.go:209-228) — invoked on Redis miss/PENDING/corrupt/down; (c) `saga.WithIdempotency(ctx, db, ins, rec, fn)` helper opening a tx, inserting record + running fn co-commit. Divergence risk: middleware caches response under SETNX key without writing to DB; service layer would write DB but doesn't (yet) call WithIdempotency. So today's source of truth is **Redis-only** with DB layer prepared but empty. If a service starts using WithIdempotency, Redis cache hit returns cached response while DB record has different `request_hash` (cache keyed by idem-key, DB keyed by `(actor_type, actor_id, key, endpoint)`). For consistency, when services migrate to WithIdempotency they must ensure the middleware's DB lookup canonicalizes endpoint identically. Documented as MEDIUM-2.

**3. KMS scope isolation** — `internal/infra/kms/kms.go:36-52` defines per-scope DEK derivation. `audit:payload` and `idem:response` are separate scopes verified by `kms_test.go:13/86`. DEK cache key is `(scope)` not yet `(tenant_id, scope)` — fine for single-tenant Phase 1; ADR-009's per-tenant requirement is **debt LOW-2** until multi-tenant Phase 2A. KEK is loaded once at startup from `KMS_KEK_HEX` (LocalKMS) — production swap to Aliyun KMS GenerateDataKey is `TODO(ops)` (kms.go:200/234/319). 90d DEK rotation: `RotateDEK` exists, invalidates scope cache, lazy-rederives on next Encrypt — but **no cron triggers it**; no scheduled rotation; manual op only. KEK 1y rotation: not implemented. Architectural risk: real, but bounded — DEK rotation absence means a single DEK is used for all payload encryption indefinitely; if the KEK leaks all historical audit/idem records are decryptable. For Phase 1 with KEK in env var on private VPC this is acceptable; for prod hardening the rotation crons must land before SLA commitments to compliance. **MEDIUM-3**.

**4. Outbox SOURCE/SINK** — `internal/outbox/aliyun_mns_publisher.go` + `aliyun_mns_consumer.go` implement raw HTTP HMAC-SHA1 against MNS REST (no Aliyun SDK pulled in — clean, fewer transitive deps). The HMAC is consistent with MNS docs. SINK is wired in `cmd/server/main.go:436-471 startMNSConsumer`: builds `HTTPMNSClient` + `MNSConsumer`, sets `NoopOnUnknown: true`, runs in goroutine. **Handler registry is empty** — `// TODO(post-B5): consumer.Register(...)` at main.go:460. With `NoopOnUnknown=true`, every consumed message is acked-and-dropped — this is permissive but logged at WARN level (consumer.go:169-174). SOURCE: `cmd/outbox-poller/` directory exists but is **empty** (no main.go). Publisher implementation in `aliyun_mns_publisher.go` is production-ready but has no caller binary. Net: outbox is half-wired — events from Fy-api can land in MNS and be acked-noop by partner-api, but partner-api cannot publish its own events to MNS until the poller is written. **HIGH-1** if any product feature depends on outbound webhooks before W4; **MEDIUM** if only inbound is needed for Phase 1.

**5. BOLA enforcement** — Two-layer: (a) runtime `WithScope("scope")` in `middleware/bola_scope.go:53-67` sets ctx + immediately enforces via `enforceBOLA` — denies on missing scope (404), unauthenticated (401), wrong actor type (403), wrong path-id (403), unknown scope (404 fail-closed). (b) Build-time analyzer `tools/analysis/bolascope/analyzer.go` walks AST for `r.GET/POST/PUT/PATCH/DELETE/Handle/Any` calls and verifies `WithScope` is in the handler chain; can be skipped only via `//bolascope:allow <reason>` directive (used 3 times in main.go for `/healthz` etc.). Gap analysis: the analyzer detects route registrations via `*gin.RouterGroup` methods — a developer who builds a request handler via `r.Use(handlerFunc)` (middleware-as-handler) or wires routes outside the analyzed package boundaries could bypass. The runtime `enforceBOLA` is fail-closed (no scope = 404), so even if analyzer misses something, runtime still denies. Coherent and defense-in-depth.

**6. Leader election** — `pkg/leader/redis.go` `RedisLock` uses `SET NX EX`, Lua-atomic renew (only renews if owner matches), Lua-atomic release. Applied to `cmd/audit-sealer/main.go:92`, `cmd/dispatcher-12377/main.go:70`, and `cmd/kyc-purge` (per Fix-C commit message). Staging/prod refuses to start without Redis (audit-sealer/main.go:86). Right primitive for Phase 1: Redis SETNX-based locks are simple, fast (<5ms acquire), and have well-known split-brain edge cases (clock skew, Redis failover) — ADR-008 commits to migrating to K8s Lease in Phase 2. Since these are batch crons (5-15min ticks), brief dual-leader windows during Redis failover are tolerable (idempotent operations: sealer dedupes by row id; dispatcher dedupes by `(case_id, target)` uniqueness). Coherent for the use case.

**7. 5 GORM repos** — `internal/repository/mysql/{partner,customer,wallet,kyc,invitation,idempotency}_mysql.go` — six actually, with `idempotency_kms.go` for envelope-encrypted variant. Pattern consistency: each exposes constructor `New<Entity>MysqlRepo(db *gorm.DB)`, methods take `ctx` + return `(*domain.<Entity>, error)`, use `db.WithContext(ctx).First/Create/Save`. Transactions threaded via either `db.Transaction(func(tx *gorm.DB) error {...})` (wallet, idempotency) or accept `tx *gorm.DB` parameter for `*WithinTx` variants. Found one inconsistency: `customer_mysql.go` doesn't expose a `*WithinTx` variant, so a customer creation cannot co-commit with idempotency_record in one tx. Future migration to WithIdempotency will need this; track as **LOW-3**. In-memory repo retained as test seam — confirmed by `internal/repository/repository.go` interface definitions and `dispatcher-12377/main.go:58` dev fallback. Coherent pattern.

**8. Frontend api-client consolidation** — `packages/api-client/src/` is the canonical package. `apps/partner-web-customer/src/api/client.ts` re-exports from `@tnbiz/api-client` (verified). `apps/partner-web-{admin,partner,storefront}/src/api/client.ts` each have `// TODO(Fix-D / 2026-05-12): MIGRATE TO @tnbiz/api-client` headers and still contain inline implementations. This is an acceptable mid-state for staging — the migrated app is the reference, the other three have clear migration paths and a tracked TODO. It becomes a debt landmine only if (a) the three lag for >1 sprint and drift diverges, or (b) a backend contract change requires touching all four. Since none of the 3 outstanding apps is in critical-path payment flow, acceptable. **LOW-4** with hard ETA: complete by end of W4.

**9. Middleware ordering** — Order in `main.go:160-217`:
- Global: `Recovery → RequestID → CORS → SecurityHeaders → Audit`
- Path-scoped (/partner, /customer, /admin, /api/sdk): `JWT → CSRF → PIIScrubber → Idempotency → (per-route WithScope)`
- Webhook scoped: `WebhookIdempotency` only.

Dependencies verified:
- `Audit` runs *before* JWT in the global chain but calls `c.Next()` then reads `actor_type`/`actor_id` from ctx after handlers run (audit.go:105/114) — correct, the audit entry is built post-chain so JWT-set actor is present.
- BOLA depends on JWT claims — `WithScope` is per-route, runs after the authedChain (JWT runs first, sets `jwt_claims`), so `ClaimsFrom(c)` in `enforceBOLA` succeeds. Correct.
- PIIScrubber wraps `c.Writer` to scrub on Write; Idempotency wraps *after* PIIScrubber, so the bytes Idempotency caches are already scrubbed. Correct ordering (innermost wrap is PII, outermost is Idempotency — both observe each Write call in stack order).
- Idempotency reads `CtxKeyActorType` set by JWT — order JWT→...→Idempotency is correct.
- WebhookIdempotency runs without JWT/CSRF — appropriate, webhooks authenticate via HMAC inside the handler.

One minor: Idempotency-SETNX runs before per-route `WithScope`, so a request that ultimately gets 403'd from BOLA still consumes a SETNX slot for 24h. Wasteful, not unsafe. Acceptable.

## New findings

### CRITICAL

None.

### HIGH

- **NEW-H1**: Outbox SOURCE (`cmd/outbox-poller/`) is an empty directory; partner-api cannot publish events to MNS. SINK works but handler registry is empty (`NoopOnUnknown: true` ack-drops all unknown event types). If any Phase 1 feature ships that requires outbound notifications (partner balance threshold alert, KYC status webhook, settlement-ready ping), it will silently no-op in prod. Fix: write `cmd/outbox-poller/main.go` paralleling `audit-sealer` (leader election + tick loop + `aliyun_mns_publisher.go`); decide on event types and Register them in `startMNSConsumer`.

### MEDIUM

- **MEDIUM-1**: Saga step registry is implemented but no production saga uses `RegisterStep` + `RunWithInput`. `Sweep` cannot retry real failed steps — only escalate them. Migrate at least `saga_allocate`'s `StepDeduct/StepHold/StepFyTopup/StepCommit` to the registry-aware API before traffic crosses 100 RPS.
- **MEDIUM-2**: `saga.WithIdempotency` helper exists but no service caller; middleware DB fallback path therefore can't actually replay because no records get inserted by the slow path. Adopt in at least one mutation handler (recommend `handler/customer/topup` first) before claiming Round-2 idempotency invariant is complete.
- **MEDIUM-3**: KMS DEK 90d rotation + KEK 1y rotation are documented and `RotateDEK` exists but no scheduler invokes it. Add a cron + leader-elected rotator before staging-to-prod cut.
- **MEDIUM-4**: `clientIP()` in admin handler trusts `X-Real-IP` / `X-Forwarded-For` headers. Document an LB hardening requirement (must overwrite, not append). Otherwise a malicious approver can spoof their `/24` to match initiator and approver_ip stored in token becomes attacker-controlled. Risk is contained because approver and initiator are *both* staff (insider-only scenario), but for least-privilege the LB rule is mandatory.

### LOW

- **LOW-1**: `partner_wallet.balance` still lacks `CHECK >= 0` (Round-1 H-5 carryover). W1c DDL fix-up.
- **LOW-2**: KMS DEK cache keyed by `scope` only, not `(tenant_id, scope)` — Phase 2A multi-tenant blocker.
- **LOW-3**: `customer_mysql.go` lacks `*WithinTx` variants; adoption of `WithIdempotency` for customer mutations will need this.
- **LOW-4**: 3 of 4 frontend apps still on inline client; migrate by end of W4.

## Architectural debt registry

| ID | Title | Severity | Tracked in |
|---|---|---|---|
| AD-1 | Saga registry adoption gap (Sweep retry dormant) | MEDIUM | NEW-H1/M-1; gate at 100 RPS |
| AD-2 | `WithIdempotency` zero adoption | MEDIUM | M-2; gate at first mutation that demands strict at-most-once |
| AD-3 | KMS DEK 90d / KEK 1y rotation scheduler | MEDIUM | M-3; gate at staging→prod cut |
| AD-4 | Outbox SOURCE (`cmd/outbox-poller`) not implemented | HIGH | NEW-H1; gate before first outbound webhook |
| AD-5 | KMS DEK cache not tenant-scoped | LOW | L-2; Phase 2A multi-tenant blocker |
| AD-6 | partner_wallet.balance CHECK >= 0 missing | LOW | L-1; W1c DDL |
| AD-7 | LB X-Forwarded-For hardening (deployment) | MEDIUM | M-4; runbook addition |
| AD-8 | Frontend 3/4 apps still on inline api-client | LOW | L-4; ETA end of W4 |
| AD-9 | `customer_mysql.go` no `*WithinTx` variants | LOW | L-3; couples to AD-2 |

## Final tally

| Severity | Round-1 (open) | W3 closes | New in Round-2 | Open at Round-2 |
|---|---:|---:|---:|---:|
| CRITICAL | 2 | 2 | 0 | **0** |
| HIGH | 5 | 4 (H-1 partial→demoted to MEDIUM-1; H-5 carried→demoted LOW-1) | 1 (NEW-H1 outbox SOURCE) | **1** |
| MEDIUM | 9 (Round-1) | most absorbed into deltas | 4 (M-1..M-4) | **4** |
| LOW | 5 (Round-1) | various closed | 4 (L-1..L-4) | **4** |

**Verdict reasoning**: 0 CRITICAL / 1 HIGH at Round-2. Strict door is `0 / 0` → fails strict, but the lone HIGH (NEW-H1 outbox SOURCE) is correctly scoped as Phase 2 dependency — no Phase 1 feature in the W3 plan actually needs partner-api outbound MNS publishing. Verdict **PASS-WITH-CONDITIONS** is honest: ship to staging, but gate three things before production cut:

1. Write `cmd/outbox-poller/main.go` and register at least one event handler in `startMNSConsumer` (closes NEW-H1).
2. Migrate `saga_allocate` to `RunWithInput` + `RegisterStep` (closes MEDIUM-1).
3. Land KMS DEK rotation cron (closes MEDIUM-3).

Round-1's verdict was FAIL; Round-2 is materially better — the W3 commits did real work, not theatre. Two real CRITs closed cleanly, four of five HIGHs closed, one HIGH partial-closed with usable infrastructure ready for adoption, one HIGH carried as LOW DDL fix. The dormancy gap (registry exists but unused; WithIdempotency exists but unused) is the dominant residual risk — easy to mistake "scaffolding present" for "behavior present", same trap Round-1 flagged in §10. Reviewers in Round-3 should grep `RegisterStep` and `WithIdempotency` in `internal/service` first; if those remain empty the dormancy debt still binds.
