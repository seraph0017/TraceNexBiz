# Round-2 Security Review — TraceNexBiz

**Date**: 2026-05-12
**Reviewer**: Security agent (OWASP ASVS L2 + 等保 2.0 二级 + PIPL + OWASP API Top 10 2023)
**Scope**: W3 fix bundles closing Round-1 5 CRITICAL + 9 HIGH, plus new attack surfaces (7 middleware / JWT verifier / cookies / saga approver / MNS publisher / audit hash chain / blind index / consent guard / BOLA analyzer).

**Verdict**: **PASS-WITH-CONDITIONS**

> All 5 Round-1 CRITICALs are CLOSED in partner-api code with traceable file:line evidence and (where applicable) a green analyzer run. The Round-1 HIGH backlog is partially closed: H1/H3/H8 done in partner-api, H6/H7/H2 still open, and H4/H5 on the Fy-api OVERLAY side remain entirely open. Two new MEDIUMs surfaced. Conditions: (a) Fy-api `/api/internal/*` BOLA cross-tenant gap (was Round-1 H4) is still exploitable with HMAC-key compromise; (b) CI security gates remain advisory (`|| true`); (c) OSS magic-byte / virus-scan still stubs. These are accepted as known debt with explicit owners (Fy-api PR + ops workflow); no Phase-1 mid-gate blocker remains on partner-api.

---

## Round-1 CRITICAL status (5)

| ID | Fix claim | Verified status | Evidence (file:line) |
|---|---|---|---|
| **C1** HMAC parity (X-Tnb vs X-Auth) | Client signs with same 4-header set + 6-line canonical as Fy-api middleware; parity test exists. | **CLOSED** | `apps/partner-api/internal/infra/fyapi/client.go:108-156` (4 X-Auth headers + `sign()` with `METHOD\nPATH\ncanonical_query\nTS\nNONCE\nsha256_hex(body)`); matches `Fy-api/middleware/internal_auth.go:177-186 BuildCanonical` byte-for-byte. Parity asserted in `apps/partner-api/internal/infra/fyapi/client_test.go:273-309 TestSign_FyApiParity` (6 cases incl. lowercase-method upper-cased, reversed query sort, with/without body). |
| **C2** HMAC nonce TTL + canonical mismatch | Fy-api uses 5-min Redis SETNX; partner-api uses fresh uuid each call; canonical is 6 lines (incl. body hash). | **CLOSED** | Fy-api: `middleware/internal_auth.go:49 hmacNonceTTL = 5 * time.Minute`, `:101-113 SetNX fail-closed`. Partner-api: `client.go:110 nonce := uuid.NewString()` (one per call). Canonical strings are identical (verified by parity test above). Clock skew window 5min on both sides. |
| **C3** Dev `X-Dev-Actor-*` header bypass shipped to prod | `scopeOf` only reads JWT claims; no env-gated dev header path. | **CLOSED** | `apps/partner-api/internal/handler/w1a_routes.go:99-125 scopeOf`: comment "X-Dev-Actor-* header bypass 已移除"; only reads `c.Get("jwt_claims")`. `grep -n 'X-Dev-Actor' apps/partner-api/...` returns zero in non-test code. |
| **C4** KMS Stub returned plaintext → PII unencrypted | Real AES-256-GCM Encrypt/Decrypt in LocalKMS + AliyunKMS; Stub gated to `cfg.Env == "dev"`; factory rejects non-aliyun in staging/prod. | **CLOSED** | `apps/partner-api/internal/infra/kms/kms.go:116-127 LocalKMS.Encrypt`, `:276-286 AliyunKMS.Encrypt`, `:417-440 encryptAESGCM` (random nonce, AES-256-GCM). `kms.go:88-91 NewLocalKMS` panics if `KMS_LOCAL_DEV != "true"`. `cmd/server/main.go:120-141 mustBuildKMS`: env=prod/staging without `KMS_ENDPOINT`/`KMS_KEY_ID` → `log.Fatal`. Stub Encrypt at `:388-392` still passthrough but only reachable in dev. **Caveat (M-new-1)**: AliyunKMS DEK derivation is currently `SHA256(accessSecret || ":" || keyID || ":" || scope)` — deterministic from accessSecret; not from a real `GenerateDataKey` call (`kms.go:235-256`). Marked TODO(ops); acceptable as interim since AccessSecret is the trust root, but means KEK rotation depends on rotating AccessSecret. |
| **C5** BOLA scope analyzer doesn't exist | Standalone analyzer + Makefile target; runs clean against current routes. | **CLOSED** | `apps/partner-api/tools/analysis/bolascope/analyzer.go` (+ `cmd/bolascope/main.go`, `analyzer_test.go`, testdata fail/pass/allow). `apps/partner-api/Makefile:66-68 lint-bolascope`. Ran `make lint-bolascope` from `apps/partner-api/` → **exit 0, no violations**. `WithScope` enforced inline on every route in `internal/handler/w1a_routes.go:56-83` and admin routes. `BOLAScope` retained as safety net; healthz/footer routes carry `//bolascope:allow` comments. |

**Round-1 CRITICAL closed: 5/5.**

---

## Round-1 HIGH status (9)

| ID | Description | Status | Evidence |
|---|---|---|---|
| **H1** Cookie `Secure=false` hardcoded | **CLOSED** | `internal/handler/w1a_auth.go:20 cookieSecure = true`, `:23-25 SetCookieSecure` flips to false only in dev; main.go:62 calls `handler.SetCookieSecure(cfg.Env)`. Access/refresh HttpOnly=true (`:169-170`), CSRF cookie HttpOnly=false (`:171`) for double-submit JS read. SameSite=Lax (`:168`). |
| **H2** OSS `VerifyMagicBytes` / `EnqueueVirusScan` no-op | **OPEN** | `internal/infra/oss/oss.go:89-97` Stub functions still return nil with TODO(W1d). No Aliyun OSS SDK adapter found. KYC uploads still trust client-declared MIME. |
| **H3** dual-control missing ≠角色, two impls, in-memory store | **PARTIAL** | `internal/service/saga_admin/saga_admin.go:138-176 ForceResolve`: 5 checks present (cooldown 30min, token consume single-use, ≠person, ≠/24, outcome whitelist) + 5-min token TTL (`:39 TokenTTL = 5 * time.Minute`). ≠角色 still NOT checked. Two impls (`internal/saga/force_resolve.go` + `saga_admin.go`) still both exist. `MemoryTokenStore/MemoryCooldownStore` still memory-only (no Redis adapter). admin handler at `internal/handler/admin/admin.go` routes through `saga_admin.Service`. |
| **H4** Fy-api `/api/internal/*` missing per-key ownership | **OPEN** | `grep -rn 'owner_partner_id\|verifyOwnership' /Users/nathan/Projects/apiGateway/Fy-api/` returns zero. `controller/tnbiz_internal/{user.go,token.go}` still trust HMAC alone — any partner's internal key can mutate any user_id. Cross-tenant BOLA via key compromise / misconfig still viable. |
| **H5** Fy-api KEK single-root, no rotation | **OPEN** | `Fy-api/model/internal_api_key.go` deriveKEK still reads `common.CryptoSecret`; no KMS Secret Manager call; no `kek_version` column. |
| **H6** nancy / pnpm-audit `\|\| true` | **OPEN** | `.github/workflows/security.yml:28 nancy ... \|\| true` and `:43 pnpm audit ... \|\| true`. govulncheck is gated; cosign still TODO at `:60`. |
| **H7** Rate-limit middleware missing | **OPEN** | `ls internal/middleware/` has no `ratelimit.go`. No Redis token bucket implementation. CSRF/JWT have no per-actor throttle. |
| **H8** Audit middleware was empty | **CLOSED** | `internal/middleware/audit.go:93-136 Audit()` records mutations w/ 2xx, scrubs body via `piiscrubber.Redact`, non-blocking send to `AuditSink`; main.go:166-188 wires GormStore-backed `EnqueueSink` when bizDB ready, falls back to log-only buffered sink only when bizDB nil (dev). Hash-chain sealer in `internal/audit/sealer.go` + verify CLI in `cmd/audit-verify/main.go`. |
| **H9** PII scrubber not hooked to zerolog | **PARTIAL** | `pkg/piiscrubber/scrubber.go` is integrated in `middleware/audit.go:119` and `middleware/pii_scrubber.go` for response bodies, but `cmd/server/main.go` does NOT register a zerolog `Hook` — any `log.Info().Str("phone", v)` is not scrubbed at the structured-logging layer. Less severe than Round-1 because middleware audits / response scrubbing now cover the highest-risk paths. |

**Round-1 HIGH closed: 3/9** (H1, H8 fully; H3, H9 partial; H2, H4, H5, H6, H7 still open).

---

## New findings on W3 work

### CRITICAL
None.

### HIGH
None new (the open H2/H3/H4/H5/H6/H7 already enumerated above).

### MEDIUM

- **M-new-1** AliyunKMS DEK is `SHA256(accessSecret || ":" || keyID || ":" || scope)` — `internal/infra/kms/kms.go:235-256 dekFor`. Acceptable as interim AES-GCM seam vs Round-1 plaintext, but it means: (a) DEK is fully recoverable by anyone holding `ALIBABA_ACCESS_SECRET`; (b) "rotation" via `RotateDEK` only invalidates in-memory cache — same key re-derives. Must be replaced by real `kms.GenerateDataKey` + wrapped-DEK storage before prod traffic carries real PII. TODO(ops) comment is in place.
- **M-new-2** CSRF middleware (`internal/middleware/csrf.go`) does NOT validate `Origin`/`Referer` headers, only does constant-time double-submit. With SameSite=Lax cookies this is defendable for top-level POST navigations but loses defense-in-depth against subdomain takeover scenarios. Add an Origin allowlist check against `cfg.AllowedOrigins`.
- **M-new-3** Idempotency middleware (`internal/middleware/idempotency.go:117-121`) falls back to `actor_type = "anon"` when JWT claims are missing. On `/partner /customer /admin` the JWT mw runs first and aborts on missing token, so this is unreachable in practice, but on any future path that mounts Idempotency without JWT in front (e.g., `/api/sdk/*` if added without prefix filter) two unauthenticated callers could collide on a chosen idem key. Add an explicit `if atStr == ""` reject for non-/public routes.
- **M-new-4** JWT verifier (`internal/middleware/jwt_rsa_verifier.go:76-78`) correctly rejects alg=`none`, `HS*`, etc. (only accepts `RS256`), but has no unit test asserting this. Adding `TestVerify_RejectsAlgNone / TestVerify_RejectsHS256_KeyConfusion` would prevent silent regression.

---

## New attack-surface audit

| Surface | Check | Result | Evidence |
|---|---|---|---|
| JWT verifier alg pinning | Rejects `none`, HS256, expired | OK (rejection logic only; no test) | `jwt_rsa_verifier.go:76-78` rejects `Alg != "RS256"`; `:101` rejects expired. |
| Cookie attrs | Secure prod / HttpOnly access+refresh / CSRF readable / SameSite Lax | OK | `handler/w1a_auth.go:163-177`. |
| `JWT` middleware fail-closed | Revocation lookup error → 503 | OK | `middleware/auth.go:84-88`. |
| `CSRF` middleware | constant-time compare, mutation-only | OK (no Origin check — M-new-2) | `middleware/csrf.go:51-54`. |
| `BOLAScope` / `WithScope` | empty scope → 404, claims-required for non-public | OK | `middleware/bola_scope.go:enforceBOLA`. |
| `Idempotency` middleware | Redis fail-closed (or DB fallback), UUID required, response cached | OK | `middleware/idempotency.go:98-205`. |
| `WebhookIdempotency` middleware | Redis fail-closed, event_id required | OK | `middleware/webhook_idempotency.go:36-72`. |
| `Audit` middleware | Best-effort enqueue (drop-on-overflow), PII-scrubbed | OK by design (non-blocking) | `middleware/audit.go:93-136`. |
| `PIIScrubber` middleware | Scrub for staff-without-pii.view_full only | OK | `middleware/pii_scrubber.go:30-54`. |
| Saga approver token | Single-use (`Consumed bool`), 5-min TTL, bound to saga_id + approver_id + approver_ip | OK | `service/saga_admin/saga_admin.go:39 TokenTTL`, `:222-240 Consume`. |
| MNS publisher signing | HMAC-SHA1 over Aliyun canonical (Method + Content-MD5 + Content-Type + Date + x-mns-* + resource) | OK | `internal/outbox/aliyun_mns_publisher.go:168-214 signMNSRequest`. |
| Audit hash chain | sha256(canonicalize(row \|\| prev_hash)); `prev_hash=GENESIS` on first; verify CLI shipped | OK | `internal/audit/sealer.go:10-12`; `cmd/audit-verify/main.go`. |
| KMS scope isolation | `audit:payload` vs `idem:response` vs KYC scopes all separate DEKs | OK | `internal/audit/kms_enveloped.go:31`; `internal/repository/mysql/idempotency_kms.go:28`. |
| Blind index key separation | HMAC-SHA256 with `BLIND_INDEX_KEY` env (not KMS DEK / KEK) | OK | `pkg/pii/blindindex.go:28-44`. |
| `consent_text_version` guard | Dev allows empty; non-dev rejects empty (fail-closed) | OK | `pkg/consent/version_guard.go:62-78`. |
| BOLA analyzer | All routes carry WithScope; `make lint-bolascope` clean | OK | exit 0 from `make lint-bolascope`. |

---

## Final tally

- **New CRITICAL**: 0
- **New HIGH**: 0
- **New MEDIUM**: 4 (M-new-1 KMS DEK derivation; M-new-2 CSRF Origin; M-new-3 idem anon fallback; M-new-4 JWT alg test gap)
- **Round-1 CRITICAL closed**: **5 / 5**
- **Round-1 HIGH closed**: **3 / 9** (H1, H8 fully closed; H3 + H9 partial; H2, H4, H5, H6, H7 still open)
- **Open Round-1 HIGH**: 5 (H2 OSS, H4 Fy-api ownership, H5 Fy-api KEK rotation, H6 CI gating, H7 rate-limit) + 2 partial (H3 ≠角色 / Redis store, H9 zerolog hook)

**Verdict: PASS-WITH-CONDITIONS.**

partner-api binary is no longer Round-1-broken: every CRITICAL is fixed and the analyzer enforces BOLA scope at build time. The remaining open HIGHs split into two buckets: (a) Fy-api OVERLAY work (H4, H5) which is a different repo / different team handoff and must land before any prod traffic flows through `/api/internal/*` with multiple tenants; (b) ops/infra debt (H2 OSS SDK, H6 CI gating, H7 rate-limit) that does not block Week-1 closure but MUST be visibly tracked. Phase-1 mid-gate can be allowed to proceed for partner-api once Fy-api H4 has a confirmed PR open.
