# PRD v0.2 Review — Security Engineer (Round 2)

> Date: 2026-05-09
> Reviewer: Security agent (Application Security)
> Scope: verify each Round-1 CRITICAL/HIGH is addressed at engineering-implementation level; check whether v0.2 introduces new attack surface; recommend v1.0 promotion.
> Round-1 verdict: BLOCK (7 CRITICAL, 6 HIGH, ~10 MEDIUM).
> Round-2 verdict: **ACCEPT_AS_V1.0** (with 2 HIGH must-fix-before-Phase-1-merge notes).
> Round-2 tally: **CRITICAL = 0, HIGH = 2, MEDIUM = 8, LOW = 5** → meets the v1.0 threshold (CRITICAL = 0, HIGH ≤ 3).

---

## TL;DR

v0.2 does the work. The four "critical sections that matter" (§16 Threat Model, §17 Auth/Session, §18 Idempotency, §19 KMS) plus §3.4 permission matrix, §8.5 `wallet_hold`, §8.13 append-only `audit_log` + `audit_log_pii`, §8.16 `idempotency_record`, §8.17 `saga_step`, §8.18 `consent_log`, §10.6 PII scrubbing — all land at a level where engineering can implement without inventing security on the fly. The Round-1 demand for AWS SigV4-style internal-API signing, mTLS, per-service keys with endpoint allowlists, two-sided idempotency with explicit `by-idem-key` probe, `wallet_hold` two-phase commit, and append-only audit log with hash chain is all there with concrete code/SQL snippets.

The two HIGH findings I'm raising are **implementation traps**, not design holes:

- **HIGH-r2-1**: The hash-chain mechanism (§8.13) is unsafe under concurrent inserts as written ("trigger 检查 PrevHash 一致性"). Two parallel `INSERT`s can both read the same `prev_hash` → silent chain corruption.
- **HIGH-r2-2**: §19.2's KEK rotation is fine, but the per-tenant **DEK rotation cadence is not specified**, and §19.3's `dekCache` (LRU, 1 h TTL) is not explicitly tenant-keyed; a single heap dump compromises every cached tenant's data with no rotation horizon.

Both are fixable in v1.0.0 with one paragraph each in the PRD plus implementation discipline. Neither blocks merge of v0.2 → v1.0; they are recorded as **Phase 1 entry-criteria for the audit-log subsystem and the PII vault subsystem**.

---

## Round-1 CRITICAL re-verification

### C-1. Tenant scoping middleware (BOLA / IDOR) — **ADDRESSED**

| Round-1 demand | v0.2 location | Evaluation |
|---|---|---|
| Where does `partner_id` come from? | §16.3 `RequirePartnerScope` reads `c.GetInt64("fy_user_id")` from JWT, then `repo.FindByFyUserID` → loads `partner.id` server-side | ✅ No client-supplied partner_id |
| Cross-tenant leak returns 404 not 403/200 | §16.3 explicit `ErrNotFound`; §3.4 footer "**404 而非 403**（不泄露存在性）" | ✅ |
| Spanning queries (admin / staff) | §16.3 `case "staff": if !scope.Elevated { return ErrForbidden }` + `r.audit.Record(... "customer.read.elevated", ...)`; §3.4 marks `🅰` for elevated rows | ✅ |
| Customer-as-actor scope | §16.3 `case "customer": q.Where("id = ?", scope.ActorID)` (NOT `partner_id = self`) | ✅ |
| Customer API key plaintext invisible to partner | §M3-03 "客户 API Key **永不可见**", §M2-05 same | ⚠️ Stated as policy. **Field-level type enforcement not specified** — see M-r2-1. |
| CI BOLA test mandatory | §16.3 `TestCustomerDetail_BOLA` template; §3.4 footer "CI 必须有 matrix BOLA test" | ✅ Make it a CI gate, not a guideline. |
| §3.4 permission matrix | 22 verbs × 6 roles; covers `wallet.adjust`, `wallet.refund`, `customer.allocate_quota`, `customer.disable`, `pricing.set`, `kyc.review`, `kyc.export_pii`, `audit_log.read`, `staff.create`, `system.config_write` | ⚠️ Covers high-risk verbs. The §7 surface contains ~80 functional IDs and not every one is enumerated. See M-r2-2. |

**Net**: design is implementable. Two soft gaps (M-r2-1, M-r2-2) downgraded to MEDIUM because the contract is correct; what's missing is enumeration and type-system discipline that engineering can finish in CI.

### C-2. Wallet idempotency / double-spend / negative-amount injection — **ADDRESSED**

| Round-1 demand | v0.2 location | Evaluation |
|---|---|---|
| Per-actor `Idempotency-Key` header + `idempotency_record` table | §18.1 + §8.16 with `UNIQUE(actor_id, idempotency_key, endpoint)` and 24 h TTL | ✅ |
| 409 on key-reuse-with-different-body | §18.2 "case found AND request_hash != record_hash: return 409 Conflict" | ✅ Stripe-style. |
| Two-sided idempotency probe (TraceNexBiz ↔ Fy-api) | §6.1 `GET /api/internal/topup/by-idem-key?key=` and `GET /api/internal/group/by-idem-key?key=`; §9.4 "on 5xx/timeout: GET .../by-idem-key" | ✅ Explicit. |
| `wallet_hold` two-phase | §8.5 hold table; §9.4 step 1 holds, step 3 commits / releases | ✅ |
| Compensation via NEW row, never delete | §8.4 `Status` ∈ {`pending`, `committed`, `compensated`}; §9.4 step 4 retains log row + writes reversing entry; §4.10 "**绝不**直接物理删除 `revenue_log`" | ✅ |
| Amount validation `> 0` at DTO boundary | §16.4 `validate:"gt=0,lte=1000000000"` | ✅ |
| Wallet drift reconciliation | §10.6 "wallet drift 每日对账 → page on > 0" | ✅ |
| Saga state machine | §8.17 `saga_step` + §14.6 saga state machine | ✅ |

**Race condition recheck**: §9.4 step 1 wraps `LOCK PartnerWallet FOR UPDATE` + `available = Balance - SUM(holds.held) >= Amount` inside a single TX. The Round-1 race ("two concurrent allocates of 10 from a balance of 15") is now blocked by the row-level FOR UPDATE lock. ✅

**Saga ordering recheck**: hold → call Fy-api → on 2xx confirm → commit hold + decrement balance + write log + bump version. On 4xx → release. On 5xx / timeout → enqueue retry that probes `/by-idem-key` and converges. Stripe-style finality. ✅

### C-3. Internal Fy-api API auth (§6.1) — **ADDRESSED**

| Round-1 demand | v0.2 §6.1 location | Evaluation |
|---|---|---|
| HMAC over what? | `HMAC-SHA256(secret, "POST\n/api/internal/user/topup\nq=...\n${ts}\n${nonce}\n${sha256(body)}")` — covers method, path, query, timestamp, nonce, body hash | ✅ Canonical form. Equivalent to AWS SigV4 minus regional/service scoping. |
| Replay window + nonce dedup | `±300 s` window + Redis SETNX nonce dedup 5 min | ✅ |
| Key rotation (N+1) | `X-Auth-KeyId` header; "支持 N+1 滚动" | ✅ Engineering still has to wire kid → secret lookup; spec is enough. |
| mTLS | "内网 mTLS（K8s Service-to-Service mTLS 或 sidecar）—— **强制要求**，明文 HTTP 一律拒收" | ✅ Hard-gate. |
| Per-service-account keys + endpoint allowlist | "key 绑定 service-account（CDC consumer 与 Topup writer 不同 key），各自有 endpoint allowlist" | ✅ |
| Key exposure surface (env files, CI logs) | §19.5 "Aliyun KMS Secret Manager 注入；不允许 plaintext .env 写仓库；CI 用 sealed secret" | ✅ |

**One nit (LOW-r2-1)**: `X-Auth-Timestamp` "RFC3339, ±300 秒" — RFC3339 strings have variable representations (timezone offset, fractional seconds, with/without `Z`). Canonicalize to Unix epoch seconds (integer) **inside the signing string**, accept either format on the wire. Otherwise an attacker can grind two equivalent timestamp strings that both validate but only one is in the nonce dedup cache. Known SigV4 lesson.

### C-4. PII / KMS / log scrubbing / OSS — **ADDRESSED (with one HIGH on DEK rotation)**

| Round-1 demand | v0.2 location | Evaluation |
|---|---|---|
| KMS choice | §19.1 Aliyun KMS | ✅ Same ecosystem as RDS / OSS / Fy-api → minimum operational surface. |
| Envelope encryption (KEK in KMS, DEK cached, encrypted DEK on row) | §19.2 + §19.3 | ✅ Standard pattern. |
| `EncryptedString` GORM type | §19.3 + §8.9 KYC fields use `EncryptedString` | ✅ |
| `pii:"true"` struct tag → never serialized + log scrubber drops | §19.3 ``plain string `pii:"true" json:"-"` ``; §16.6 `LogScrubber` + pattern matching for ID / phone / email | ✅ Concrete. |
| OSS presigned URL TTL ≤ 300 s + attachment + URL not logged | §19.6 explicit; §M4-03 / §M7-01 reference | ✅ |
| Backups encrypted + access-controlled | §10.3 "加密 + 独立 OSS bucket + 只读 IAM" | ✅ |
| 30-day vs 5-year reconciliation | §M7-07 "30 天热存储清原图 + 5 年冷归档"; §16.5 retention matrix; §15.6 反洗钱 5 y; §4.17 right-to-delete keeps audit hash chain but tombstones `audit_log_pii` | ✅ Tension resolved. |
| Audit-vs-purge tension (PIPL 47 vs 反洗钱 retention) | §8.13 split into `audit_log` (immutable hash chain, no PII) + `audit_log_pii` (encrypted, tombstoneable on PIPL 47) | ✅ |

**HIGH-r2-2 — DEK rotation under-specified** (full description further down).

### C-5. Authentication & session — **ADDRESSED**

| Round-1 demand | v0.2 location | Evaluation |
|---|---|---|
| Session source of truth (Fy-api JWT vs own) | §17.1 reuse Fy-api JWT + own `tnbiz_session` cookie scoped `*.tracenex.cn` | ✅ |
| Cookie flags (Secure / HttpOnly / SameSite) | §17.1 SameSite=Lax, Secure, HttpOnly | ✅ Lax (not Strict) is correct because partner cross-subdomain navigation is needed. |
| `jti` revocation list | §17.1 Redis `revoked:jti:{id}` TTL = JWT exp | ✅ Stateful revocation on stateless JWT. |
| CSRF | §17.3 Origin / Referer allowlist + SameSite=Lax | ✅ Defense-in-depth. |
| MFA matrix per role | §17.2: `super_admin` TOTP + WebAuthn forever; `partner` TOTP req when wallet > 0 | ⚠️ See M-r2-3. |
| Password reset flow | §17.5 email + SMS dual-factor (partner mandatory); reset → all `jti` revoked | ✅ |
| SIM-swap defense | §17.5 dual-factor (email + SMS) means attacker needs **both** email account and SIM. With §17.4 HIBP password check, the easy paths are closed. WebAuthn would be stronger — see M-r2-3. |
| Session timeout | §17.1 5 min idle (staff) / 8 h (partner / customer) | ✅ Staff is correctly aggressive. |
| Account enumeration | §17.4 "错误信息统一" | ✅ |
| CORS allowlist | §17.6 explicit, no wildcard | ✅ |
| Security headers | §17.7 HSTS preload, CSP nonce, XFO DENY, Referrer-Policy, Permissions-Policy | ✅ |

---

## Round-1 HIGH re-verification

| ID | Round-1 ask | v0.2 status |
|---|---|---|
| H-1 | CDC schema drift / freshness gate | **CLOSED**: §9.3 replaces CDC with application-layer outbox (`consume_log_outbox`, same-TX write). `revenue_log.fy_api_log_id UNIQUE`. §9.3 settlement freshness gate explicit. Schema drift is now a per-column overlay-API contract, not binlog parsing. |
| H-2 | Audit log append-only + hash chain | **PARTIALLY CLOSED**: §8.13 has the right shape (no UPDATE/DELETE for app user, prev_hash / self_hash, `audit_log_pii` split). But the **trigger spec is unsafe under concurrent inserts** — see HIGH-r2-1. |
| H-3 | Payment webhook signing + IP allowlist + amount cross-check + dedup | **CLOSED**: §M6-07 RSA verification + IP allowlist + `(channel, out_trade_no)` UNIQUE; §M6-08 amount cross-check vs server-side order. |
| H-4 | Cross-partner customer migration consent | **CLOSED**: §4.8 dual-consent (partner A + partner B) + staff arbitration; settlement freeze for in-flight period; `customer_partner_change_log` audit. §4.14 "已是直营客户" path explicit, no silent transfer. |
| H-5 | Markup ratio bounds | **CLOSED**: §M3-13 + §16.4 `1.0 ≤ markup ≤ MaxMarkup` (default 5.0); `decimal.Decimal` everywhere; server-side enforced. |
| H-6 | Phase 1 "绕过钱包" rollback | **CLOSED**: §12.1 phase-1 honest re-scope; wallet primitive day 1; initial balance pre-funded by platform. M3-04 says "**永远走 wallet hold**（不绕过）". |

---

## NEW HIGH findings introduced or surfaced in v0.2

### HIGH-r2-1 — Audit log hash chain is unsafe under concurrent inserts (silent corruption)

**Where**: §8.13 (`audit_log`). The spec says: *"trigger 检查 PrevHash 一致性"*.

**Problem**: The natural implementation is a `BEFORE INSERT` trigger that does:

```sql
SET NEW.prev_hash = (SELECT self_hash FROM audit_log
                    WHERE id < NEW.id ORDER BY id DESC LIMIT 1);
SET NEW.self_hash = SHA2(CONCAT_WS('|', NEW.id, NEW.actor_type, ..., NEW.prev_hash), 256);
```

Under InnoDB's default REPEATABLE READ with two concurrent inserts (txn A and txn B) on a high-traffic endpoint (e.g. 200 req/s saga commits, each writing audit rows):

1. Both txns auto-allocate IDs N and N+1 (autoinc lock mode 2 = interleaved, the InnoDB default since 8.0).
2. Both `SELECT self_hash WHERE id < NEW.id` *snapshot* before either has committed.
3. Txn A reads prior row's hash (id N-1) → installs as `prev_hash` for row N.
4. Txn B *also* reads prior row's hash (id N-1) → installs as `prev_hash` for row N+1.
5. Row N+1's `prev_hash` should equal row N's `self_hash`, not row N-1's. **Chain is broken at row N+1 the first time concurrency exceeds 1.**

The break is **silent**: a verifier walking `id` ascending and comparing `prev_hash[i] == self_hash[i-1]` finds the mismatch only on offline audit. Until then the chain is corrupt and an inserted-and-then-deleted row in the gap is now indistinguishable from a legitimate concurrent insert. The system that exists *to provide* tamper evidence stops doing so.

**Required fix in v1.0.0** — pick one:

1. **Sealed-by-async-batcher** (recommended, mirrors §M5-01 Cron pattern): app inserts with `prev_hash = NULL`, `self_hash = NULL`. A single-instance K8s-Lease-locked worker reads unhashed rows in `id` order, computes hashes, fills the columns. App user has `INSERT (excluding prev_hash, self_hash) + SELECT`; batcher user has `UPDATE (only prev_hash, self_hash) WHERE prev_hash IS NULL`. The hash chain is never under concurrent write. (This is what Cloudflare's transparency log does.)
2. **Serialized via SKIP LOCKED queue**: insert into a `audit_log_unsealed` queue first; a single consumer drains in id order, writes to `audit_log` with chain. Same end state.
3. **Application-side sequencer**: dedicated `audit-writer` goroutine with a buffered channel; only this goroutine writes `audit_log`. Simple — but only OK if audit volume < 1000 rows/s.

**Do NOT** rely on `SERIALIZABLE` isolation on the audit table — kills throughput under load and still has phantom-read edge cases on autoincrement.

CVSS 7.5 (Tampering / Repudiation; integrity loss in the very system supposed to provide tamper evidence). **Must fix before any production write to `audit_log`.** PRD §8.13 must be amended with the chosen mechanism.

### HIGH-r2-2 — DEK rotation cadence + tenant-keyed cache discipline

**Where**: §19.2 / §19.3 / §19.4.

**Problem**: §19.4 describes **KEK rotation** ("每 12 个月手工轮换 KEK") but the **per-tenant DEK rotation strategy is missing**. §19.2 says: *"明文 DEK 在内存 LRU 缓存 1h"*. Implications:

1. The cache key in §19.3 is shown abstractly. If implemented as `(tenantID, keyID)` it's fine; if implemented as just `keyID`, multiple tenants share a DEK → catastrophic blast radius. PRD doesn't say which.
2. A single heap dump (Go panic core, OOM dump, `pprof goroutine?debug=2` leaked through a public health endpoint, an SRE attaching `dlv`) leaks every cached plaintext DEK. Without DEK rotation, that DEK encrypts that tenant's PII forever — KEK rotation does **not** help (KEK only encrypts DEKs at rest, not the live ciphertext on rows).
3. Aliyun KMS supports DEK rotation; PRD must say "DEK rotates every N days, batch task re-encrypts ciphertext from old DEK to new DEK; old DEK is erased from cache and KMS after re-encryption completes."
4. Defense-in-depth missing for the in-memory plaintext: `mlock` to prevent swap, `runtime.SetMutexProfileFraction(0)` to suppress profile leakage, disable pprof on prod listener, redact KMS material from panic recovery handlers.

**Required fix in v1.0.0 (before Phase 1 PII data lands)**: §19.4 must add:
- `dekCache` is keyed by `(scope_id, key_id)`; `scope_id = tenant_id` for per-tenant DEKs and `system` for system-scope.
- Per-tenant DEK rotation every 90 days (rolling); old DEK retained encrypted for re-encryption only; deleted after re-encryption batch completes and verifies.
- pprof endpoints disabled (or auth-gated) on prod; sealed-memory or `mlock` on the cache page where feasible (Go on Linux: `unix.Mlock`).
- KMS audit log streamed to SLS, alert on `Decrypt` rate spike.

CVSS 7.0 (high impact if a heap dump leaks; lowered likelihood by Go memory safety + restricted prod access; raised by long-lived plaintext DEKs).

---

## NEW MEDIUM findings (v0.2 surface)

### M-r2-1 — Customer API key field-level visibility relies on policy, not type

§M3-03 / §M2-05 say "partner 不可见客户 API Key 明文". That's policy, not enforced. If `Customer` struct embeds `ApiKey string` and a developer adds `c.JSON(200, customer)` in a partner-side endpoint, the policy fails silently and `sk-...` lands in JSON.

**Fix**: define a separate view type — e.g. `CustomerForPartnerView struct { ID, Name, Email, ... }` with no `ApiKey` field. Repository `CustomerRepo.FindByID(scope=partner, ...)` returns the view type directly. Ban `c.JSON(200, *Customer)` in partner-side handlers via a `golangci-lint` custom analyzer or a code-review macro.

### M-r2-2 — Permission matrix §3.4 has 22 verbs; §7 has ~80 functional IDs

Verbs/endpoints that are **not** in §3.4 but should be: `customer.transfer`, `saga.retry`, `ticket.create`, `ticket.assign`, `kyc.submit`, `pricing.archive`, `wallet_hold.list`, `settlement_run.terminate`, `consent.revoke`, `dispute.create`, `notification.dispatch`, `outbox.replay`, `seat.allocate`, `invitation.revoke`, `staff.suspend`. The §3.4 footer mandates a "matrix BOLA test" but the matrix isn't complete.

**Fix in v1.0.0**: expand §3.4 to enumerate every verb that maps to a state-changing endpoint. Generate the matrix from a Go enum at build time so drift is caught (every new endpoint constructor must reference a `permission.X` enum, otherwise CI fails).

### M-r2-3 — Partner WebAuthn optional even at high wallet thresholds

§17.2 marks WebAuthn `可选` for partner. Partners hold both money and customer PII. TOTP is phishable; WebAuthn (FIDO2) is the only phish-resistant factor.

**Fix**: make WebAuthn `必` once partner cumulative monthly payout exceeds ¥10k or live wallet exceeds ¥1k (configurable in `biz_setting`). TOTP minimum below that threshold. Not a blocker because dual-factor reset already closes SIM-swap; this is hardening for the high-value partner segment.

### M-r2-4 — Redis Pub/Sub channel `option_update` (§9.2 / §C-6) is a DoS amplifier

Any actor with `PUBLISH` permission on that Redis namespace can force every Fy-api instance to call `loadOptionsFromDatabase()`. Mass publish → reload-storm → DB connection exhaustion under load.

**Fix**:
- Redis ACL: only the `fy-api` role has `+publish` on `option_update`; TraceNexBiz role gets nothing; `default` user has no publish.
- Subscriber-side rate limit: at most one `loadOptionsFromDatabase()` per 200 ms regardless of message rate; coalesce.
- mTLS / AUTH on Redis — should be implied by §10.5, but PRD doesn't say it. State explicitly.

### M-r2-5 — `idempotency_record.ResponseJson` may store PII at rest, unencrypted

§8.16 caches the full response body for 24 h. Wallet topup or KYC review responses may include partner / customer PII. The table is not flagged as `pii`.

**Fix**:
- Strip `pii:"true"` fields from the cached response before storing (use the same struct-tag introspection from §19.3), OR encrypt `ResponseJson` with the system DEK.
- Add a TTL-based purge job (not soft delete) on `expires_at < NOW()`.

### M-r2-6 — Outbox poller (§9.3) is DoS-able and gates settlement

`SELECT FROM consume_log_outbox WHERE consumed_at IS NULL ORDER BY id LIMIT 1000` runs every 1 s. If Fy-api floods `logs` (legitimate burst, accidental loop, or attacker holding a cluster of trial-quota tokens), outbox lag spikes. §9.3 freshness gate then **refuses to start settlement**. Customers continue to consume API while `revenue_log` is delayed → partner billing delayed. In the lag window, customers can drain quota without revenue ever materialising (if banned or org goes bankrupt before outbox catches up).

**Fix**:
- §10.6 add explicit metric `outbox_lag_seconds` with PagerDuty threshold (already implied by R-16; make it explicit, with numeric SLO).
- Backpressure: if outbox unconsumed > 1 M rows, Fy-api side throttles new consumes (feature flag in `consume_log_outbox` writer to slow-path or 429).
- Dead-letter queue for outbox rows that can't resolve `partner_id` (e.g. customer was deleted between log write and outbox consume).

### M-r2-7 — `saga_step.Payload` and `LastError` plaintext business state

If saga retries leak via `LastError` or `Payload` into application logs (very common in Go via `errors.Wrap` or `log.Printf("%+v", sagaStep)`), business state leaks. Less severe than C-4 PII but still a privacy / business-secret concern (allocation amounts, customer ids, idempotency keys).

**Fix**: §8.17 add note that `Payload` and `LastError` are subject to §16.6 log scrubber redaction; do NOT log saga rows in full; if the row must be logged, marshal via a redacted view.

### M-r2-8 — Cookie scope vs domain layout

§17.1 says `tnbiz_session` is scoped to `*.tracenex.cn`. §5.1 architecture diagram shows two domains: `partner.tracenex.cn` (TraceNexBiz) and `api.aitracenex.com` (Fy-api). §M2-11 allows role switching customer ↔ partner. Cookie domain story is muddy — a customer logging in on `aitracenex.com` and then switching to "渠道商" view will not naturally have the `tnbiz_session` cookie set.

**Fix**: spell out the canonical front-door domain (`tracenex.cn` or `aitracenex.com`) for v1.0 (CN region) and the cookie scope. If two domains, document which cookies are issued where, and whether `aitracenex.com` is in the CORS allowlist for `partner.tracenex.cn`.

---

## Confirmed LOW

1. **LOW-r2-1**: `X-Auth-Timestamp` should be canonicalized to Unix epoch seconds **inside the signing string** (RFC3339 string ambiguity). Accept either format on the wire.
2. **LOW-r2-2**: §17.4 "登录失败 5 次 → 15 min lockout" — per-account or per-IP? An attacker who knows the account list can DoS-lock partners by spraying. Use per-(account, IP) tuple with a global per-account rate-cap that is higher than 5/15 min.
3. **LOW-r2-3**: §16.4 "字符串字段长度限制（防 DoS）" — make this concrete: `validate:"max=N"` on every string DTO field. Default `max=255` for short, `max=10240` for free-form notes.
4. **LOW-r2-4**: §17.5 password reset — dedicate a separate rate limit (3 / hour / account); ensure the SMS provider doesn't queue messages to a phone number that has changed within the last 7 days (recent-change → require email-only or staff-assist).
5. **LOW-r2-5**: §M1-08 trial quota anti-fraud "IP / 设备指纹" — device fingerprinting is regulated under PIPL as an indirect identifier. §15.5 should add device-fingerprint consent UI.

---

## Sanity check: integrity invariants engineering must wire as CI gates

Before Phase 1 closes, these MUST fail-the-build:

1. **BOLA grid test** (§16.3 + §3.4): for every (verb, role) pair where the matrix says ❌, a CI test creates two partners A and B and asserts B's token gets 404 (not 200) when accessing A's resource. Auto-generate from §3.4; if the matrix has a row without a corresponding test, build fails.
2. **Wallet drift test** (§10.6): `partner_wallet.balance == SUM(partner_wallet_log.amount WHERE partner_id=...)` enforced as daily Prometheus alert. Integration test simulates 1 k concurrent allocates and asserts post-state has zero drift.
3. **Audit chain verifier** (HIGH-r2-1): integration test writes 10 k audit rows concurrently and asserts the prev_hash chain is intact. Fails immediately if HIGH-r2-1 isn't fixed.
4. **DTO amount fuzz test** (§16.4): for every endpoint accepting `amount`, fuzz with `-1`, `0`, `int64.MaxValue`, `1e18+1` — assert 400.
5. **Idempotency replay test** (§18): replay same request 100× with the same `Idempotency-Key`; assert balance moves once, response identical.
6. **CSRF test** (§17.3): for every POST/PUT/DELETE/PATCH endpoint, send with no `Origin` and with a forged `Origin: evil.com` — assert reject.
7. **Permission enum drift detector** (M-r2-2): every router-registered handler must reference a `permission.X` enum at registration time; build fails if a handler exists without one.
8. **MySQL grant gate** (§6.3): post-deploy, `SHOW GRANTS FOR tnbiz_app@%` must match the expected golden file. Block deploy on mismatch. Same for `tnbiz_migrator`.
9. **OSS signed URL TTL gate** (§19.6): runtime check on every signed-URL generation path that TTL ≤ 300 s. Test calls every OSS-using endpoint and inspects URL signature TTL.
10. **PII scrubber test** (§16.6): inject a known ID number / phone / email / RFC5322 address into a log line; assert it appears redacted in the SLS sink.

---

## Did v0.2 introduce new attack surface beyond the above?

- **Pub/Sub channel** — see M-r2-4. Manageable.
- **Outbox poller** — see M-r2-6. Manageable.
- **`/api/internal/user/erase`** — destructive, but §6.1 per-service-key scoping plus §17 idempotency + audit means a compromised partner key cannot mass-erase. Verify the erase service-account has `erase` in its allowlist *only*, never `topup`. Make this explicit in §C-2 (controllers) — one line.
- **`saga_step` retry loops** — bounded by §10.6 metric `saga 失败率`; explicit retry logic not specified. Add `max_attempts = 5` with exponential backoff and after exhaustion, page to ops + write to dead-letter table. (LOW.)
- **Internal idempotency table on Fy-api side (§18.3 / §C-7)** — keyed by what? "Per (endpoint + idem-key)" should be UNIQUE. Worth one line in §C-7.

None of these are blocking.

---

## Verdict

**ACCEPT_AS_V1.0**, with the following promotion conditions documented as Phase 1 entry criteria:

| ID | Condition | Owner | Gate |
|---|---|---|---|
| HIGH-r2-1 | Audit log hash chain implementation must use sealed-by-async-batcher (or equivalent), not naive `BEFORE INSERT` trigger. PRD §8.13 amended with the implementation note. | Backend / Security | Phase 1 audit-log code review |
| HIGH-r2-2 | DEK rotation cadence + tenant-keyed cache + pprof disabled in prod + heap-dump KMS hygiene documented. PRD §19.4 amended. | Security / SRE | Phase 1 PII vault code review |
| MEDIUM-r2-{1..8} | Tracked as v1.0.x backlog; M-r2-2 (matrix completeness) and M-r2-4 (Redis ACL) recommended for Phase 1; others by Phase 2A. | PM / Eng | v1.0.1 patch review |
| LOW-r2-{1..5} | Hardening backlog, complete by Phase 2B. | Eng | — |

**Rationale for ACCEPT_AS_V1.0** (rather than NEEDS_REVISION):

- Round-1 had 7 CRITICAL covering session, idempotency, KMS, scope, audit, internal API, and wallet primitive. Every single one is now concretely specified at code-snippet level, with §3.4 / §16.3 / §17 / §18 / §19 reading like an engineering spec rather than a wishlist.
- The two new HIGH findings are implementation-detail bugs that will surface on first code review, not architectural rewrites; they do not propagate to other sections of the PRD.
- The §11 risk table now has owners and explicit mitigations (R-1 .. R-25), not aspirations.
- The "绕过钱包" rollback is gone. Phase 1 ships the wallet primitive day 1.
- v0.2 is **honest**: the change banner at the top admits which v0.1 promises were lies (Fy-api zero-touch, instant price propagation, no CDC) and replaces them with implementable contracts.

This document, plus the Phase 1 entry criteria above, is enough for engineering to start writing code without inventing security on the fly. The remaining findings are matters of code-review discipline and CI gating, which is exactly where they belong.

— Security agent, Round 2
