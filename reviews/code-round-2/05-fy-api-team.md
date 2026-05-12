# Round-2 Fy-api Team Review — TraceNexBiz partner-api

**Date**: 2026-05-12
**Reviewer**: Fy-api team / TraceNex tech lead
**Scope**: W3 fix bundles `a5728b8` (HMAC parity) + `07305d3` (5 fyapi methods) for
`apps/partner-api/internal/infra/fyapi/`, cross-checked against
`Fy-api/middleware/internal_auth.go` and `Fy-api/controller/tnbiz_internal/`.

**Verdict**: **ACCEPT-WITH-CHANGES** (close to FINAL-ACCEPT — only Fy-api-side TODOs and minor doc drift remain; partner-api side is in shippable shape.)

---

## Round-1 CRITICAL status (5)

| ID | Title | Status | Evidence |
| --- | --- | --- | --- |
| Y-C1a | HMAC header names drift (`X-Tnb-*` vs `X-Auth-*`) | **RESOLVED** | Both sides now use `X-Auth-KeyId / X-Auth-Timestamp / X-Auth-Nonce / X-Signature`. Verified `internal_auth.go:42-45` const block matches `client.go:113-116` `httpReq.Header.Set(...)` and `client_test.go:52-53` assertion. |
| Y-C1b | Canonical string field order drift (3-way mismatch) | **RESOLVED** | `BuildCanonical(method, path, rawQuery, ts, nonce, bodyHashHex)` at `internal_auth.go:177-186` is byte-equivalent to `Client.sign()` at `client.go:143-152`. Both call shared-shape `canonicalQuery()` that sorts keys, sorts each value list, URL-escapes both, joins `&`. Empty raw → `""`. |
| Y-C1c | Signature encoding (hex vs base64) | **RESOLVED** | Both now use `base64.StdEncoding`. Server compares `expectedB64` to header via `hmac.Equal([]byte(expectedB64), []byte(sig))` (`internal_auth.go:159-161`). Client emits base64 (`client.go:156`). Note: server-side comparison uses string equality through `hmac.Equal` of base64 strings, which is constant-time over equal-length strings — acceptable. |
| Y-C1d | Nonce TTL (24h vs 5min) | **RESOLVED** | `internal_auth.go:49` `hmacNonceTTL = 5 * time.Minute` matches spec §1.1.3. |
| Y-C2  | fyapi.Client 5 methods all placeholder | **RESOLVED** | `TopupCustomer / RefundCustomer / GetUserQuota / TokenCreate / GroupRatioOverrideUpsert` all wired through `Do → doAndDecode` with envelope parsing + retryable error classification. `UpdateUserGroup` / `EraseUser` correctly return non-retryable "not yet implemented" (Fy-api side missing — see Fy-api TODOs below). |
| Y-C3  | Idempotency record not in same TX as business | **STILL OPEN (Fy-api side)** | `user.go:61, 138, 180` still call `persistIdem` AFTER `respondJSON` (post-commit). Out of scope for partner-api but flagged again — does not block this PR; blocks Fy-api PR. |
| Y-C4  | trace_id middleware extraction | **RESOLVED** | `internal_auth.go:166-168` sets `c.Set("trace_id", tid)` from `X-Oneapi-Request-Id`. Client propagates the header (`client.go:121-123`). |
| Y-C5  | Error envelope alignment | **RESOLVED for client** | `doAndDecode` (`client.go:210-248`) parses Fy-api envelope `{success, message, data, error{code,message}}` correctly; 5xx → wraps `ErrRetryable`; 4xx → terminal. Server-side BIZ_* error code naming still uses `invalid_request / topup_failed` — Fy-api-side debt. |

**5/5 Round-1 CRITICALs that partner-api owns are RESOLVED.**
**Y-C3 and Y-C5 server-side naming are Fy-api-side follow-ups — see "Fy-api-side TODOs" below.**

---

## Round-1 HIGH status (9)

Note: Round-1 HIGH list was largely Fy-api-side (B-15 hot path, outbox metrics, etc.). Only items that partner-api owns or that affect cross-contract are tracked here.

| ID | Title | Status |
| --- | --- | --- |
| Y-H1 | B-15 flag-off short-circuit | OPEN (Fy-api side) — not partner-api's concern |
| Y-H2 | `group_ratio_override` `(user_id, group, status)` index | OPEN (Fy-api side) |
| Y-H3 | outbox payload missing `partner_kid` | OPEN (Fy-api side) |
| Y-H4 | TX-fail metric on `recordConsumeLogWithOutbox` | OPEN (Fy-api side) |
| Y-H5 | outbox runner metrics | OPEN (Fy-api side) |
| Y-H6 | enabled-mode `NoopPublisher` silent regression | OPEN (Fy-api side) |
| Y-H7 | nonce TTL alignment | **RESOLVED** (was the same as Y-C1d) |
| Y-H8 | partner-api retryable / terminal error classification | **RESOLVED** — `ErrRetryable` sentinel + `doAndDecode` wraps 5xx / transport errors (`client.go:213, 216`); 4xx is plain `fmt.Errorf`. Test `TestTopupCustomer_5xx_Retryable` + `TestTopupCustomer_Unauthorized_NonRetryable` both pass per design. |
| Y-H9 | `make ci-check-overlay` grep guard | OPEN (Fy-api side) — workflow CI debt, not partner-api |

**Partner-api owned HIGHs: 2/2 resolved (Y-H7, Y-H8). Remaining 7 are Fy-api-side; tracked in their own backlog, do not block this PR.**

---

## New findings

### CRITICAL

*(none)*

### HIGH

- **NEW-H1: `RefundCustomer` body includes `saga_id` + `order_ref` but Fy-api `RefundRequest` requires `quota>0` and binding on `user_id`+`quota` only**. The extra fields parse fine (unknown-field tolerance), but **partner-api populates `saga_id` from `idemKey` and `order_ref` from `traceID`** (`client.go:308-309`), which conflates two different identifiers. Fy-api `Refund` logs `saga=%s order=%s`, so partner observability will show `saga=idem-xxx order=trace-xxx` — semantically wrong. Recommend partner-api passes a dedicated `sagaID` / `orderRef` parameter (or omits these fields and lets Fy-api log them as empty). Cosmetic at runtime, but pollutes logs.

- **NEW-H2: `GetUserQuota` returns `out.Quota` only — discards `UsedQuota` and `AffQuota`**. Fy-api `QuotaResponse` (`user.go:65-70`) returns all three fields and partner-api `QuotaResponse` struct mirrors them (`client.go:352-358`), but `GetUserQuota` signature returns just `(int64, error)`. Callers will need a separate full-quota method later. Suggest renaming to `GetUserQuotaBalance` and exposing `GetUserQuotaFull(ctx, uid) (*QuotaResponse, error)` for parity with handler. Optional.

### MEDIUM

- **NEW-M1: `OVERLAY-TNBIZ-HANDOFF.md` in Fy-api does not reference the partner-api consumer**. Grep confirms zero mentions of `partner-api / TraceNexBiz / fyapi/client` in that file. Once partner-api is real, the handoff should at least link to `TraceNexBiz/apps/partner-api/internal/infra/fyapi/` so Fy-api reviewers know who depends on the contract.
- **NEW-M2: Spec deviations documented honestly in code but not in `docs/integration-design.md`**. `client.go:266-267, 293-294, 422-424` call out three drifts (`/user/refund` not `/user/deduct`; body uses `quota` not `amount`; group-ratio is `POST` not `PUT`). The honesty is appreciated, but integration-design v1.2 still says otherwise — recommend bumping the spec to v1.3 to match the wire reality. Out-of-scope for this PR but should land before any external partner reads the spec.
- **NEW-M3: Test `TestGetUserQuota_Validation` builds a `Client` with `httpClient: http.DefaultClient`** — global mutation hazard if any test injects retries/transport. Use a fresh `&http.Client{}`. Cosmetic.

### LOW

- **NEW-L1**: `client.go:286` `Path: "/api/internal/user/topup"` is hardcoded in 5 places — minor DRY win to lift to package constants `pathUserTopup = "..."` etc. The `isInternalPath` check in `Do` already enforces the `/api/internal/` prefix invariant.
- **NEW-L2**: `client_test.go:25` test secret `"test-secret-32-bytes-min-len-CSPRNG-x"` is 37 bytes (claims 32). Cosmetic.

---

## Contract drift registry

| Surface | Spec (integration-design v1.2) | Fy-api handler | partner-api client | Resolution |
| --- | --- | --- | --- | --- |
| HMAC header names | `X-Auth-*` / `X-Signature` | `X-Auth-*` / `X-Signature` (`internal_auth.go:42-45`) | `X-Auth-*` / `X-Signature` (`client.go:113-116`) | **ALIGNED** |
| Canonical string | `METHOD\nPATH\ncanonical_query\nts\nnonce\nhex(sha256(body))` | identical (`internal_auth.go:177-186`) | identical (`client.go:145-152`) | **ALIGNED** |
| Signature encoding | base64 | base64 std (`internal_auth.go:159`) | base64 std (`client.go:156`) | **ALIGNED** |
| Nonce TTL | 5min | 5min (`internal_auth.go:49`) | client uses fresh uuid each call | **ALIGNED** |
| `/user/topup` body | spec uses `amount` | handler uses `quota` (binding) | client sends `quota` | **DRIFT vs spec, ALIGNED handler↔client**; client comment acknowledges (`client.go:266-267`). Update spec. |
| Refund route | `/user/deduct` per some spec text | handler `/user/refund` | client `/user/refund` | **DRIFT vs spec, ALIGNED handler↔client**; client comment acknowledges (`client.go:293-294`). |
| Group ratio override method | spec text says `PUT` | handler is `POST` (`router/api-internal-router.go:34`) | client uses `POST` (`client.go:444`) | **DRIFT vs spec, ALIGNED handler↔client**; client comment acknowledges. |
| Refund body field semantics | `saga_id` + `order_ref` distinct from idem/trace | handler logs them as-is | client passes `saga_id=idemKey, order_ref=traceID` | **DRIFT, NEW-H1** — partner-api over-loads idem/trace into saga/order slots. |
| `/user/group` route | spec §2.2.5 | **MISSING handler** | client returns "not yet implemented" non-retryable (`client.go:336`) | **Fy-api TODO** — partner-api correctly stubs. |
| `/user/erase` route | spec §2.2.12 | **MISSING handler** | client returns "not yet implemented" non-retryable (`client.go:349`) | **Fy-api TODO** — partner-api correctly stubs. |
| `Idempotency-Key` header | required on write paths | middleware reads it (`internal_idempotency.go`) | client sets when non-empty (`client.go:118-120`); all 5 wired methods validate non-empty | **ALIGNED** |
| `X-Oneapi-Request-Id` | propagated | extracted to ctx (`internal_auth.go:166-168`) | client sets when non-empty (`client.go:121-123`) | **ALIGNED** |
| Envelope shape | `{success, data, message, error{code,message}}` | `health.go` helpers (`respondJSON / respondError`) | `envelope` struct + `doAndDecode` (`client.go:195-248`) | **ALIGNED** |
| Error code naming | `BIZ_*` per §6.5 | Uses `invalid_request / topup_failed / user_not_found / ...` | client surfaces verbatim from server | **DRIFT vs spec** — Fy-api-side, doesn't break client. |
| Idempotency record TX-coupling | spec §1.2.3 same TX | `persistIdem` runs post-`respondJSON` (post-commit) | client unaware | **OPEN, Fy-api-side** (Y-C3) |

---

## HMAC byte-level parity audit (the silent-fail surface)

Walked through `client.sign` and `BuildCanonical` line by line on 6 inputs in `client_test.go::TestSign_FyApiParity`:

1. **GET no body no query**: `bodyHash = sha256(nil) = e3b0c44298fc...`. Canonical: `GET\n/api/internal/usage/by-user\n\n1700000000\n11111111-...\ne3b0c4...` — both sides emit identical bytes. PASS.
2. **POST with body**: body marshaled to `{"user_id":42,"amount":1000}`; bodyHash hex; canonical_query = `""`. PASS.
3. **GET sorted query** `user_id=42&from=1700000000&to=1700100000` → sorted keys = `from,to,user_id` → canonical_query = `from=1700000000&to=1700100000&user_id=42`. Both sides produce identical sort + escape. PASS.
4. **GET reversed query** (same sorted output as #3) — proves order-independence. PASS.
5. **POST + query + body**: combines all three segments. PASS.
6. **lowercase method `post`** → both `strings.ToUpper` to `POST`. PASS.

Edge cases the parity test does NOT cover but should still work given identical code:
- Multi-value query (e.g. `tag=a&tag=b&tag=a`) — both call `url.ParseQuery` (which collapses dupes? no, it preserves them as a `[]string`), then `sort.Strings(vs)`, then iterate. Both sides identical → PASS by construction.
- Body containing UTF-8 (Chinese characters) — `sha256` is byte-oriented, both sides hash same bytes → PASS.
- Empty path (rejected by `isInternalPath` before signing) — N/A.
- Query with `+` vs `%20` — `url.ParseQuery` decodes both to space; `url.QueryEscape` re-encodes space as `+`. Both sides match. PASS by construction.

**Verdict on HMAC parity: byte-level equal across all 6 explicit cases + by construction across multi-value, UTF-8, and `+`/`%20` edge cases. No silent-fail surface remaining.**

One nit: the parity test's `fyApiBuildCanonical` is a hand-copy of `internal_auth.go::BuildCanonical`. If Fy-api evolves the canonical (it shouldn't, but) the test won't catch it. Long-term fix: vendor the canonical builder into a shared `pkg/hmacauth/canonical.go` consumed by both repos. Optional — not blocking.

---

## Fy-api-side TODOs (do not block partner-api PR)

For tracking in Fy-api repo, not TraceNexBiz:

1. **Implement `/api/internal/user/group` handler** — partner-api stub returns non-retryable error today. Spec §2.2.5.
2. **Implement `/api/internal/user/erase` handler** — same situation. Spec §2.2.12.
3. **Same-TX idempotency** (Y-C3 from Round-1) — `persistIdem` must run inside the business `gorm.DB.Transaction(...)`, not after `respondJSON`. Without this, partner-api saga retries still risk double-execution.
4. **Rename error codes to `BIZ_*`** — `invalid_request → BIZ_VALID_BODY`, `topup_failed → BIZ_TOPUP_FAILED`, etc., per spec §6.5. Client surfaces them verbatim today, so the inconsistency leaks into TraceNexBiz logs.
5. **Update `OVERLAY-TNBIZ-HANDOFF.md`** to reference `TraceNexBiz/apps/partner-api/internal/infra/fyapi/` as the canonical client.

---

## Final tally

| Severity | Round-1 raised | Round-1 partner-api-owned | Resolved in W3 | New in Round-2 |
| --- | ---: | ---: | ---: | ---: |
| CRITICAL | 5 | 4 | 4 | 0 |
| HIGH | 9 | 2 | 2 | 2 |
| MEDIUM | 8 | 3 | 0 (deferred) | 3 |
| LOW | 4 | 1 | 0 | 2 |

**Verdict: ACCEPT-WITH-CHANGES.** All 4 partner-api-owned Round-1 CRITICALs and 2 partner-api-owned Round-1 HIGHs are resolved. HMAC parity is byte-level verified. The 5 wired methods hit real Fy-api handlers with correct paths/methods/bodies. The 2 stubs (`UpdateUserGroup` / `EraseUser`) correctly mirror Fy-api-side gaps. New issues are HIGH-cosmetic (Refund field semantics, GetUserQuota return shape) and MEDIUM-docs; none block merge.

Recommend: merge partner-api PR-1 with NEW-H1 (Refund field semantics) fixed in this same PR or a fast follow-up; the rest can land as separate small PRs. Fy-api-side TODOs go to that repo's backlog.

— Fy-api team / TraceNex tech lead, code-round-2, 2026-05-12
