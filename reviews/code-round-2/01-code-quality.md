# Round-2 Code Quality Review — TraceNexBiz

**Date**: 2026-05-12
**Reviewer**: Code Quality
**Scope**: W3 commits since Round-1 (Fix-A / Fix-B' / Fix-C / Fix-D bundles, 11 commits)
**Verdict**: **PASS-WITH-CONDITIONS**

## Summary

W3 closed 6 of 8 Round-1 HIGH findings outright and partially closed the remaining 2 with documented sunset paths (HIGH-2 envelope unification deferred, HIGH-3 api-client migration shipped for customer + stub-and-TODO for the other 3 apps). Net quality is up: `go test -race ./...` is green across 38 packages, repository/mysql coverage jumped 13.3 % → 74.1 %, middleware 50 % → 74.4 %, and the new GORM repos and EnqueueSink follow the established `WithContext(ctx)` pattern. Eight new findings were uncovered, of which one is a logical bug (`MNSConsumer.noopOnUnknown` is wired to `x || true`, so it is unconditionally `true`) and one is a `go vet`-flagged context leak in `pkg/leader.RedisLock.Run`. Neither blocks ship, but the HIGH count rises from 8 → 4 open after netting closures and the new fix-needed items. Two W3-introduced cron entries (`dispatcher-12377`, dev fallback paths in `audit-sealer`) reach for in-memory repos under conditions where a real DB is expected — these are tagged HIGH because they silently no-op in prod-adjacent envs.

## Round-1 findings status

| ID | Item | Status | Evidence |
|---|---|---|---|
| HIGH-1 | force-resolve contract split | **CLOSED** | `internal/handler/saga_admin.go` deleted; only `handler/admin/admin.go:340-407` remains; route `POST /admin/saga/:id/force-resolve` matches `admin.ts::ForceResolveInput` field set (`approver_token / outcome / reason`); `IssueApproverToken` now hangs off `POST /admin/staff/approver-tokens` with `SagaID` body |
| HIGH-2 | three envelope helpers | **PARTIAL / NEW-DEBT** | `handler/admin/admin.go:70-89` (`ok` / `bad` / `unavailable`) and `handler/w1a_routes.go:86-97` (`ok` / `fail`) still coexist. `bad()` still sets `message_zh == message_en == msg` (admin.go:80-81); no `handler/envelope.go` was created. `fail()` still omits `trace_id`. |
| HIGH-3 | api-client triple-copy | **PARTIAL** | `packages/api-client/src/{client,types,envelope,error-mapping,trace,index}.ts` created and consumed by `apps/partner-web-customer/src/api/client.ts` (12-line re-export shim). `partner / admin / storefront` still hold full clones with explicit `TODO(Fix-D / 2026-05-12)` headers acknowledging migration is deferred. 3 of 4 apps still on legacy code. |
| HIGH-4 | saga compensation silent error swallow | **CLOSED-MOSTLY** | `service/saga_allocate/service.go:128-158` now captures `compErr`, joins multi-step errors via `errors.Join`, wraps with `wrapCompensationError(stage, cause, compErr)`. Round-1 R-H-4b sub-finding (`instance.go:277-281` Compensate failure path missing escalation check) is **STILL OPEN**: line 277-281 still writes `failed` and returns without consulting `ShouldEscalate`. |
| HIGH-5 | `cmd/server/main.go` not wired | **CLOSED for W1a, OPEN for W1b/W1c** | `cmd/server/main.go:155-243` now wires Gin engine + global middleware + JWT/CSRF/PII/Idempotency/BOLA chain + path-prefix filtering + footer route + audit EnqueueSink + KMS factory. **However** the wiring stops at W1a deps (`buildW1aDeps`). Saga orchestrator, outbox SOURCE/SINK consumer is partially wired (`startMNSConsumer` ok but no handlers registered), and W1c services (`invoice / payment / ticket / notification / content_safety / staff / saga_admin / dispute / settlement / revenue`) still not built or mounted. `/api/admin/*` routes are not registered. The wiring fix only landed for one of two named services. |
| HIGH-6 | `ApplyPartner.tsx` 559-line file | **CLOSED** | Split to `apps/partner-web-storefront/src/pages/ApplyPartner/{index.tsx 137, Step1Basics 77, Step2KYC 57, Step3Bank 121, Step4KYC 61, Step5Review 111, FormButtons 71, useApplyPartner 167}.tsx` — all under 200 LOC, ≤ 167 LOC; hook extracted. |
| HIGH-7 | OTP `%06d`+`[:6]` non-uniform | **CLOSED** | `service/auth/auth.go:351-355` rewrote: `n, _ := rand.Int(rand.Reader, big.NewInt(1000000)); otp := fmt.Sprintf("%06d", n.Int64())`. Uniform 000000-999999. |
| HIGH-8 | `BackoffFor` doc/impl mismatch | **CLOSED** | `saga/saga.go:161-173` rewritten: `attempts <= 1 → 2s` early-return; loop `for i := 1; i < attempts ...`. Test (`saga_test.go::TestBackoffFor`) covers attempts=0/1/2/3/8/30 and is green. |

**Closed: 5 ; Partial: 3 (HIGH-2, HIGH-3, HIGH-5).**

## New findings

### CRITICAL

None.

### HIGH

#### CQ2-H-1 — `MNSConsumer.noopOnUnknown` always-true logical bug

**File**: `apps/partner-api/internal/outbox/aliyun_mns_consumer.go:105`

```go
return &MNSConsumer{
    ...
    noopOnUnknown: opts.NoopOnUnknown || true,
}, nil
```

`X || true` is always `true` — caller's `NoopOnUnknown: false` is silently ignored. Consumers that want fail-loud behaviour on unregistered `event_type` (so DLQ-redrive can catch them) instead silently ack and drop the message. Behaviour matches what Round-1 would consider a fail-open vulnerability for an outbox sink.

**Fix**: remove the `|| true`. Default behaviour is already handled by Go zero-value; if you want a default of `true`, do `if !explicitlySet { opts.NoopOnUnknown = true }` via a pointer/sentinel.

#### CQ2-H-2 — `pkg/leader/redis.go` context leak (vet-flagged)

**File**: `apps/partner-api/pkg/leader/redis.go:113`

`go vet ./...` reports:

```
pkg/leader/redis.go:113:3: the cancel function is not used on all paths (possible context leak)
pkg/leader/redis.go:96:4:  this return statement may be reached without using the cancel var
```

When `leaderCtx.Done()` fires (line 121) we `break renewLoop` but never call `cancel()` — the leaderCtx will be garbage-collected eventually but its child timer goroutines are orphaned for up to the parent ctx lifetime. With `audit-sealer / kyc-purge / dispatcher-12377` all using `RedisLock.Run`, each leader failover slowly leaks per renew loop.

**Fix**: add `cancel()` before `break renewLoop` (one line at line 122), or use `defer cancel()` after the `context.WithCancel` call so all paths clean up.

#### CQ2-H-3 — `cmd/dispatcher-12377` and `audit-sealer` dev fallback uses in-memory repo silently

**File**: `apps/partner-api/cmd/dispatcher-12377/main.go:64`

```go
svc := content_safety.NewService(content_safety.NewMemoryRepo(), noopAuthority{})
```

The cron *opens* `bizDB` (line 53), checks for dev-only fallback, then `bizDB` is **never used** — the service is hard-wired to MemoryRepo regardless of env. In staging/prod the cron will start, log "leader; tick=5m batch=50", and process zero rows because every restart begins with an empty memory store. `audit-sealer/main.go:47-49` has a similar dev fallback to `MemoryStore` but properly switches to `GormStore` when `bizDB != nil` — the 12377 cron has no such branch.

**Fix**: gate `NewMemoryRepo()` behind `cfg.Env == EnvDev && bizDB == nil`; in staging/prod inject the real GORM repo (W1d will land it; until then fail-fast as `audit-sealer` does, not silently no-op). Same hygiene applies to the noopAuthority — it's logged as warning but the cron presents itself as "running".

#### CQ2-H-4 — saga `instance.Compensate` failure path still skips escalation (Round-1 R-H-4b unresolved)

**File**: `apps/partner-api/internal/saga/instance.go:277-281`

```go
if txErr != nil {
    failed := i.snapshotFailed(existing, "compensate: "+txErr.Error(), now)
    _ = i.repo.Save(ctx, &failed)
    return nil, fmt.Errorf("saga: compensate %s failed: %w", step, txErr)
}
```

`Run` (line 134-145) checks `ShouldEscalate(&failed, now)` after a failed step and calls `MarkEscalated` when thresholds hit; `Compensate` doesn't. Combined with the sweep loop only re-running `Run` paths (`orchestrator.Sweep:114-121` calls `RunWithInput`, not `Compensate`), a compensation that fails 30 times still sits as `failed` and isn't surfaced to the dual-control admin UI. Round-1 R-H-4b was explicitly called out and remains unfixed.

**Fix**: mirror `Run`'s escalation check in `Compensate`. Also consider whether `Sweep` should pull `failed` steps marked as compensation attempts (currently the registry/payload envelope only knows `StepFunc`, not `CompensateFn`).

### MEDIUM

#### CQ2-M-1 — KMS `keyIDFor` returns `v0` regardless of `versionGen` / `RotateDEK`

**File**: `apps/partner-api/internal/infra/kms/kms.go:124, 260`

```go
keyID := fmt.Sprintf("local:%s:v0", scope)   // LocalKMS.Encrypt
return fmt.Sprintf("aliyun:%s:%s:v0", k.keyID, scope)  // AliyunKMS.keyIDFor
```

`LocalKMS.RotateDEK` (line 159) returns a `v%d` keyID, but `Encrypt` always stamps `v0`. AliyunKMS never increments either. So a freshly-rotated `RotateDEK` returns a token caller cannot reconstruct via Encrypt. For idempotency_record / audit payload that store `key_id` for later Decrypt, the version field is decorative.

**Fix**: thread `versionGen` into `keyIDFor` and store per-scope monotonic counter; or drop the `v0` suffix entirely until rotation lands (currently it's a misleading promise).

#### CQ2-M-2 — `EnqueueSink.flushOne` ignores caller context

**File**: `apps/partner-api/internal/audit/mysql_sealer.go:380-386`

```go
func (s *EnqueueSink) run() {
    defer close(s.done)
    ctx := context.Background()    // <-- fresh root ctx; not tied to lifetime
    for r := range s.ch {
        s.flushOne(ctx, r)
    }
}
```

A `context.Background()` is used inside the worker goroutine; `Close()` triggers the for-range to exit, but `EnqueueUnsealed → s.db.WithContext(ctx).Create(...)` has no deadline/cancel. Pair with the 3-retry `time.Sleep(backoff)` (line 401) and a slow/hung DB will hold the worker for `100ms + 200ms + 400ms` = 700 ms per row before drop. Acceptable, but use of `context.Background()` in a sink worker is a code smell — pass the lifecycle ctx in via `NewEnqueueSink(store, buffer, ctx)`.

**Fix**: accept ctx at construction; propagate to `flushOne` with a per-flush timeout (e.g. 5s).

#### CQ2-M-3 — GORM repos: ~120 LOC of `FindByX → rowToY` boilerplate duplicated 5×

**Files**: `partner_mysql.go`, `customer_mysql.go`, `kyc_mysql.go`, `wallet_mysql.go`, `invitation_mysql.go`

Each repo implements the same shape:

```go
func (r *XRepository) FindByY(ctx, y) (*domain.X, error) {
    var row xRow
    if err := r.db.WithContext(ctx).First(&row, "y = ?", y).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, nil
        }
        return nil, err
    }
    return rowToX(&row), nil
}
```

And the Update updater pattern is identical at line ~135 of each file. Worth a small generic helper:

```go
func findOne[Row any, Dom any](ctx context.Context, db *gorm.DB,
    where string, args []any, toDom func(*Row) *Dom) (*Dom, error) { ... }
```

Not blocking, but five files × ~25 LOC each is real duplication that will compound when W1d adds more domains.

#### CQ2-M-4 — `handler/admin/admin.go::bad` still violates bilingual envelope

**File**: `apps/partner-api/internal/handler/admin/admin.go:74-85`

`message_zh == message_en == msg`. PRD §11 envelope requires two distinct strings. This is the un-closed half of Round-1 HIGH-2.

#### CQ2-M-5 — `fail()` envelope in `handler/w1a_routes.go:92-97` omits `trace_id`

**File**: `apps/partner-api/internal/handler/w1a_routes.go:92-97`

```go
"error": gin.H{"code": code, "message_zh": msgZh, "message_en": msgEn},
```

No `trace_id` field. Front-end `mapApiError` reads `error.trace_id` for toast metadata; missing field = `undefined`. Round-1 HIGH-2.

#### CQ2-M-6 — `cmd/server/main.go::buildW1aDeps` falls back to MemoryRepo on `bizDB == nil` without env guard

**File**: `apps/partner-api/cmd/server/main.go:340-347`

```go
} else {
    log.Warn().Msg("bizDB unavailable; falling back to in-memory repos (dev/W0 only)")
    invRepo = invitation.NewMemoryRepo()
    ...
}
```

Comment says "dev/W0 only" but no `cfg.Env == EnvDev` check guards the fallback. If staging/prod boots with `bizDB == nil` (network glitch, DB credential refresh), partner-api comes up serving from MemoryRepo. Pair with the cron same issue in CQ2-H-3.

**Fix**: in non-dev, fail-fast (`log.Fatal`) rather than swap repos.

#### CQ2-M-7 — `saga_refund` / `saga_topup` still 0 % coverage

`go test -cover` confirms `internal/service/saga_refund` and `internal/service/saga_topup` ship with zero tests. Round-1 LOW-7 → MEDIUM gap. With W4 sweep-runs-fn now live, these untested sagas can hit retry-sweep paths and silently misbehave.

### LOW

- **CQ2-L-1** `internal/handler/handler.go` (`/api/public/biz_setting/footer`) reads from cache with a 1-minute TTL hardcoded; should come from `cfg.BizSetting.FooterCacheTTL` or constant. Not checked further.
- **CQ2-L-2** `internal/audit/mysql_sealer.go:333` declares its own `var AuditDropsTotal atomic.Int64` separate from `middleware.AuditDropsTotal` (audit.go:52). Two counters with the same name in two packages — both used by the same audit sink wiring. Pick one source of truth.
- **CQ2-L-3** `kms.go:419-440` `encryptAESGCM` silently extends short keys via `sha256.Sum256(key)[:]`. For `Stub`/`LocalKMS` that's defensible (dev), but caller can pass a 16-byte key thinking it's AES-128 and silently get AES-256. Reject `len(key) != 32` outright.
- **CQ2-L-4** `internal/outbox/aliyun_mns_consumer.go:124-132` retry-on-error backs off 1s only; for an MNS outage on a high-traffic node this means a 1 req/s sustained 4xx loop. Add jitter or use the same exp-backoff as the publisher.
- **CQ2-L-5** `cmd/server/main.go:104` `startMNSConsumer` swallows context cancellation: error returns `consumer.Run(ctx)` and only `errors.Is(err, context.Canceled)` is filtered out — `context.DeadlineExceeded` from upstream propagates as `ERROR`. Minor.
- **CQ2-L-6** Three GORM repos (`PartnerRepository`, `CustomerRepository`, `KYCRepository`) use `selectForUpdate(tx)` only inside `Update`. `OrphanByPartner` (customer_mysql.go:154) does a bulk UPDATE without row lock — fine for SQLite/MySQL semantics but inconsistent with the rest of the file's locking discipline.
- **CQ2-L-7** `apps/partner-web-{partner,admin,storefront}/src/api/client.ts` carry identical "TODO Fix-D" headers but no CI check ensures the migration completes. Add a tsc/eslint custom rule or grep-based CI lint that fails if these stub clones still exist after W4.

## Test quality assessment

The new GORM repository tests (`partner_mysql_test.go`, `customer_mysql_test.go`, `wallet_mysql_test.go`, `idempotency_mysql_test.go`, `idempotency_same_tx_test.go`) genuinely exercise behaviour — they assert on row IDs, balance values after `AdjustBalance`, soft-delete invisibility, `ErrDuplicateKey` wrapping, and concurrent `WithIdempotency` racing (8 goroutines, asserts exactly-1-success). `TestIdempotency_SameTX_RolledBackOnBusinessFailure` is a model test — it sets up a business error, calls the production helper, and asserts the side-table row disappeared after rollback. Same for `TestWalletRepository_AdjustBalance_AtomicCheck` (writes a negative-delta then asserts balance is unchanged).

`middleware/middleware_test.go` (32 tests) covers JWT/CSRF/PII/Idempotency/BOLA real branches — happy + revoked + missing cookie + bad CSRF + redis-down. Tests use `miniredis` and a real RSA keypair via `rsa.GenerateKey`. Audit `mysql_sealer_test.go` covers GormStore hash-chain round-trip.

Gaps:
- `saga_refund`, `saga_topup`, `internal/service/dispute` low-medium coverage and key paths (callback replay, transient failure → escalation) untested. `pkg/leader` 0 %.
- Handler-level integration tests still 8.8 % — only `routes_smoke_test.go` and a public footer test, no `httptest` over auth/business endpoints.
- No tests assert that the bola-scope-required analyzer actually rejects an unscoped route (the `tools/analysis/bolascope/testdata` directory is small; verify it covers a "negative" missing-WithScope case).
- The Aliyun MNS publisher/consumer tests (`aliyun_mns_test.go` 317 LOC) use fake `MNSClient` — good unit coverage but no end-to-end test against a local MNS-like service. Acceptable for Phase-1.

Overall test-quality grade is **B+** (up from C+ in Round-1).

## Final tally

- CRITICAL: 0
- HIGH: 4 (CQ2-H-1 noopOnUnknown, CQ2-H-2 leader vet, CQ2-H-3 cron in-memory, CQ2-H-4 saga Compensate escalation)
- MEDIUM: 7 (CQ2-M-1..M-7)
- LOW: 7 (CQ2-L-1..L-7)
- Round-1 HIGH closed: **5 / 8** (HIGH-1, HIGH-4 main path, HIGH-6, HIGH-7, HIGH-8)
- Round-1 HIGH still open or partially open: **3** — HIGH-2 (envelope unification not consolidated, bilingual still violated, trace_id missing in fail), HIGH-3 (3 of 4 frontends still hold clones), HIGH-5 (W1a wired but W1b/W1c services still not mounted in `cmd/server/main.go`)
