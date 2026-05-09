# PRD v0.1 Review — Security Engineer (Round 1)

> Date: 2026-05-09
> Reviewer: Security agent (Application Security)
> Verdict: **BLOCK** — PRD cannot proceed to engineering until §S (Security & Threat Model), §A (Authentication/Session), §I (Idempotency), and §K (KMS / Key Management) are added. Money + PII + multi-tenant + CDC + payment is the maximum-impact intersection of risk surfaces, and v0.1 punts on every one of them.

---

## Threat Summary

TraceNexBiz handles **money** (partner wallets, customer top-ups, monthly settlement payouts), **PII** (national ID numbers, business licenses, legal-rep ID photos, Alipay real-name, phone numbers), **multi-tenancy** (one partner must never see another partner's customer or wallet), and **integrates with two trust boundaries**: Fy-api (internal HTTP API + binlog CDC) and external payment / KYC providers (WeChat Pay, Alipay, Aliyun OCR, Alipay 芝麻认证).

The PRD identifies the headline risks (越权, 资金一致性, PII泄露) but treats them as one-line table entries with hand-waved mitigations ("RepositoryWithPartnerScope middleware", "AES-GCM, KMS 管 key", "saga + 补偿"). None of these are concretely specified at a level engineering can implement without inventing critical security details on the fly. **OWASP Top 10 2021 risks A01 (Broken Access Control), A02 (Cryptographic Failures), A04 (Insecure Design), A07 (Auth Failures), A08 (Software and Data Integrity), and A09 (Logging Failures) are all under-specified.** PIPL Article 17/19/47 obligations (purpose limitation, retention, right to delete) are mentioned only as "30-day purge" and need explicit handling for the audit-vs-purge conflict.

The single most dangerous design choice is the **"绕过钱包 (bypass wallet)"** decision in Phase 1 (M3-04 verbatim: 从平台直拨给客户，绕过钱包). This means engineering will ship a working money-allocation flow without the wallet primitive, then bolt the wallet on in Phase 2. That is an invitation to forget the optimistic-lock + idempotency contract on the second pass and to leave the un-keyed allocation endpoint shipping in production "temporarily."

---

## CRITICAL findings (CVSS ≥ 8.0, money or PII risk)

### C-1. Tenant scoping middleware is hand-waved — BOLA / IDOR is the single largest attack surface

The PRD literally says (§11): *"所有查询走 `RepositoryWithPartnerScope` 中间件；每个 endpoint 必须有越权测试用例"*. That is an aspiration, not a design. The questions that must be answered before code is written:

1. **Where does the `partner_id` come from?** From a JWT claim? From `session.Get("partner_id")`? From a query param (catastrophic)? From the Fy-api `User.id` plus a database lookup of `partner.fy_user_id`?
2. **Nested resources.** `GET /api/customer/:id/usage` requires *two* checks: (a) `customer.partner_id == ctx.partner_id`, and (b) every `usage` row's `customer_id` is in the partner's customer set. The PRD §7.3 lists "客户列表 + 客户详情" but never says what happens if a partner forges `customer_id=999` belonging to another partner. The Fy-api auth pattern in `middleware/auth.go` already shows the foot-gun: the `New-Api-User` header is *compared* to the session id but is otherwise redundant — partners will discover this and try to spoof it.
3. **Spanning queries (admin / staff).** Platform staff legitimately need cross-partner queries (M4-07 全部客户列表). The PRD does not specify how staff queries bypass scoping while still emitting an audit row. Without an explicit "elevated-context" pattern, engineers will either (a) add a `?bypass_scope=true` flag (catastrophic), or (b) duplicate every repository method with a `*ForAdmin` variant (drift-prone).
4. **Privilege escalation: customer API key visibility.** §M3-03 says "客户列表 + 客户详情" without specifying whether the partner sees the customer's `sk-...` API key. **A partner must NEVER see a customer's API key in plaintext** — the partner only allocates quota; the API key is the customer's secret. PRD silence here will lead to a leaky implementation. Apply OWASP A01 / CWE-639.
5. **Customer-as-actor scope.** The PRD never says what `customer.partner_id` means for the customer's *own* requests. A customer's `GET /api/me/usage` must scope to `customer.id == self`, not `customer.partner_id == self`.

**Impact**: One missing `WHERE partner_id = ?` clause leaks the entire customer list of a competing partner (PII), the markup ratio (commercial secret), and potentially API keys (account takeover of the customer). CVSS ~9.1.

**Required fix in v0.2**: A whole new section §S.1 specifying the canonical `Repository.Find(ctx, scope, id)` signature where `scope` carries `(ActorType, ActorID, Elevated)`. Plus a mandatory CI test for every read endpoint: "partner A creates record, partner B's token must get 404 not 200". Pattern below in the code section.

### C-2. Wallet operations have no idempotency contract — double-spend / refund-replay is unmitigated

§9.4 says "退款逻辑：1. 渠道商或平台发起退款 2. TraceNexBiz 在 `revenue_log` 写一条负数补偿日志". There is no idempotency key, no unique constraint preventing re-execution, no description of what happens if the partner clicks "退款" twice in 100ms or if the network retries the request.

§3.4 (partner_wallet) shows `Version int64 // 乐观锁` — good, but optimistic locking does **not** prevent double-spend; it prevents lost updates. Concrete attack:

> Partner has 15 quota. Concurrently:
> - Request A: allocate 10 to customer X (reads version=5, balance=15)
> - Request B: allocate 10 to customer Y (reads version=5, balance=15)
>
> Both check `15 >= 10`. Both call `UPDATE ... WHERE version=5`. The first wins, second retries → reads version=6, balance=5, **fails the >=10 check**. So optimistic locking is OK for this race **iff** the validation is re-done on retry. The PRD never says it is.

Worse: the `partner_wallet` row is the local side of a **distributed** transaction (TraceNexBiz wallet ↔ Fy-api `users.quota`). The PRD §5.2.4 names this "saga 模式" but specifies neither the compensating-action ordering nor the idempotency key on the Fy-api `/api/internal/user/topup` call. Without it:

- **Double-spend**: partner allocates 10, network times out, retry succeeds, network reply lost, second retry succeeds → customer gets 20, partner wallet shows 5 (or maybe 15 if the local commit was rolled back).
- **Negative-amount injection**: PRD never bounds `amount` on the allocate endpoint. A signed `int64` request body with `amount = -1000` would, in a naive impl, *credit* the partner wallet and *debit* the customer.
- **Refund replay across endpoints**: a refund on the partner-side endpoint and a separate adjustment on the platform-staff endpoint, both keyed off the same `RefId`, both fire — twice the compensation.
- **Cross-partner customer takeover**: customer migrates from partner A to partner B mid-billing-period (silently allowed in §9.2 risk note); the in-flight `revenue_log` rows for A become orphans; A's wallet never settles them.

**Impact**: Direct financial loss. Partners can fabricate balance. CVSS ~9.3 (CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:N).

**Required fix in v0.2**: §I (Idempotency) section spelling out:

1. Every state-changing wallet/quota endpoint accepts a client-generated `Idempotency-Key` (UUIDv4) header. Server stores `(actor_id, idempotency_key) → (response_hash, http_status, created_at)` in an `idempotency_record` table with TTL ≥ 24h and a unique index on `(actor_id, idempotency_key)`. RFC 7521 / Stripe-style.
2. The Fy-api `POST /api/internal/user/topup` call MUST accept and persist the same idempotency key. If Fy-api does not support idempotency on its internal API, that's a v0.2 dependency on the Fy-api team — block until they sign up.
3. All `amount` fields are **`uint64` or validated `> 0`** at the DTO boundary. Add `validate:"gt=0,lte=1000000000"` and a unit test that proves `amount=-1` returns 400.
4. Saga compensation is fully specified: "if Fy-api topup fails, the local `partner_wallet_log` is marked `compensated`, *not* deleted; balance is restored via a *new* compensating row, never by rolling back the original." This makes the audit trail consistent.
5. **Explicit ordering**: lock partner wallet → check balance → write log row (status=`pending`) → call Fy-api with idempotency key → on success update log row to `committed` and bump wallet version → on failure write compensating row with `Type=allocate_compensation`.

### C-3. Internal Fy-api API auth (§6.1) is dangerously under-specified

The PRD says "静态 API Key + 时间戳 + HMAC 签名（防重放）". That is the textbook description of an auth scheme that has been broken in dozens of CVEs. The questions:

1. **HMAC over what?** Body bytes? Body + path + method + timestamp? If body only, attacker swaps the path. If timestamp only, replay attack within the timestamp window. The canonical answer is `HMAC-SHA256(method || "\n" || path || "\n" || query || "\n" || timestamp || "\n" || nonce || "\n" || sha256(body))`.
2. **Replay window.** If the timestamp tolerance is ±300s, the same request can be replayed 600 times. Requires a server-side **nonce cache** (Redis SETNX with TTL ≥ timestamp tolerance + clock skew).
3. **Key rotation.** "TraceNexBiz 启动时加载" — so the key is in an env var? Rotated never? Phased-overlap rotation requires the server accepting `kid` in the signed header and looking up which secret to verify against.
4. **mTLS.** The internal API is "internal" — but what does that actually mean? If TraceNexBiz and Fy-api are on the same VPC, mTLS is cheap defense-in-depth and mandatory for SOC2. If they cross AZ or region (CN ↔ SG), this is over the public internet and mTLS is non-negotiable.
5. **Key exposure surface.** The static API key sits in the Fy-api env file *and* TraceNexBiz env file *and* every CI worker that touches either repo *and* every developer's `.env` for local dev. One leak (`git log -p` of an old commit, an SSO-borked CI log dump) compromises every internal API endpoint forever. CWE-321.
6. **Authorization granularity.** A single shared HMAC key authorizes *every* internal endpoint. There is no way to give the CDC ingestion service a read-only key that cannot mint quota. Per-purpose service accounts with scoped permissions are required.

**Impact**: Compromise of the internal-API HMAC secret = full ability to mint quota for any user, modify any user's group (= modify the billing multiplier of any tenant in the system), create users at will. CVSS ~9.8.

**Required fix in v0.2**: §6.1 must specify the canonical AWS SigV4-style scheme above, mTLS as default, a rotation protocol, and per-service-account keys with explicit endpoint allowlists.

### C-4. PII handling is named but not designed — KMS, log scrubbing, OSS presigning are all unspecified

§7.7 M7-06 says "AES-GCM 加密；密钥放 KMS". Concrete questions:

1. **Which KMS?** Aliyun KMS? AWS KMS? HashiCorp Vault? Self-hosted? Each has different access-control models. Aliyun KMS is the obvious choice for a CN-RDS deployment but the PRD never says.
2. **Envelope encryption or direct?** Direct KMS encryption per row is expensive (KMS API call per insert/select, ~5ms each). Envelope encryption (KMS-encrypted DEK cached locally) is the standard, but requires DEK rotation strategy.
3. **Plaintext in process memory and logs.** `LegalPersonIdNo string  // AES-GCM 加密` (§8.9) — encrypted at rest, but **decrypted in process to render the admin-review page**. If a single `c.JSON(200, application)` accidentally serializes the decrypted struct into a Gin error log (very common in Go via `panic` recovery), the ID number lands in `stdout`, which lands in the structured log pipeline, which goes to whatever log retention SaaS the team uses. Compare PIPL Article 51, OWASP A09.
4. **OSS presigned URL.** §M7-01 says "图片只能上传到私有 OSS bucket". §M4-03 says "OSS 私有桶预览". So admin staff fetch the image via a presigned URL. Questions: TTL of the presigned URL (default Aliyun is 3600s; for a business-license image it should be ≤ 300s), referer leakage (presigned URL pasted into Slack with link unfurling = leak), proxy-log leakage (a corporate proxy logging URL paths captures the whole signed URL). Best practice: short TTL (60-300s), `?response-content-disposition=attachment` to discourage embedding, never log the full URL.
5. **30-day purge ambiguity.** §M7-07 says "OSS lifecycle rule + Cron". §11 says "30 天自动清原图". §8.9 says `PiiPurgedAt *time.Time`. Three different mechanisms — which is the source of truth? **What about the audit period requirement?** PIPL Article 47 plus 中国电子证据保全 require certain payment-related records to be retained for *3 years*. If the legal-person ID was the basis of payment-recipient verification, it's evidence. The PRD has no policy on the tension. The safe answer is: the ID *photo* is purged (used only for OCR + face match, derived OCR text is retained per the legal-basis matrix), but `LegalPersonName` + last-4 of ID number remain.
6. **Backup leakage.** §10 says "RDS 自动备份每日". Backups carry PII. Backup encryption + access control on the dump location are not specified. Backups stored in OSS need their own bucket policy.

**Impact**: PIPL violation = administrative fine up to RMB 50M or 5% of prior year's revenue (PIPL Article 66). Brand damage / 36Kr-headline level. CVSS ~8.5.

**Required fix in v0.2**: §K (Key Management) section. §S.PII section listing every PII field, classification (sensitive / general / public), KMS DEK strategy, log-scrubbing requirement (`zerolog.Hook` or middleware that drops fields tagged `pii:"true"`), and explicit retention matrix (field × purpose × retention period × purge mechanism × legal basis).

### C-5. Authentication & session — entire topic is missing from v0.1

The PRD has zero text on:
- How TraceNexBiz authenticates a partner / customer / staff session. Reuse Fy-api JWT? Cookie cross-domain (`partner.tracenex.cn` vs. `api.aitracenex.com`)? Issue its own?
- Session timeout, "remember me" cookie scope and Secure/HttpOnly/SameSite flags.
- CSRF protection. The Fy-api auth middleware (`middleware/auth.go`) uses session cookies + a `New-Api-User` header for double-submit but **does not enforce CSRF on state-changing endpoints**. Because TraceNexBiz state-changing endpoints include "allocate quota" and "approve KYC", a partner-targeted CSRF on `/api/partner/wallet/allocate` is direct money-loss territory.
- 2FA. §8.14 has `Mfa string  // TOTP secret，加密` for staff but no policy: is it required, recommended, opt-in? For partners (who control wallets worth real money) MFA must be required after the wallet exceeds some threshold, with FIDO2/WebAuthn preferred over TOTP.
- Account-takeover paths via email reset / phone reset. The PRD never says how a partner recovers a forgotten password — and the answer determines whether a $50k partner wallet can be drained by SIM-swapping the partner's phone.
- Session revocation. If a partner's account is compromised and they reset their password, are existing JWTs invalidated? JWTs are stateless; without a deny-list (Redis-backed by `jti`), the attacker keeps the old token until it expires.

**Impact**: Without a session policy, every endpoint is potentially CSRF-able and every partner is potentially SIM-swap-able. Combined with C-2 (no idempotency), an attacker can drain a wallet via a single forged form post. CVSS ~9.0.

**Required fix in v0.2**: §A (Authentication & Session) section. Decision: reuse Fy-api JWT (preferred — single source of truth) with an additional TraceNexBiz-issued cookie scoped to `*.tracenex.cn` carrying the partner_id claim, signed by Fy-api JWT secret. CSRF enforced via `Origin`/`Referer` check on all `POST/PUT/DELETE/PATCH`. WebAuthn mandatory for staff, TOTP minimum for partners with non-zero wallet, optional for customers. Session revocation list keyed on `jti` in Redis.

---

## HIGH findings

### H-1. CDC binlog ingestion (§9.3 B-2) — schema drift = silent revenue corruption

§9.3 B-2 says "TraceNexBiz 用 binlog CDC（canal/maxwell）订阅 logs 表 → 解析 group 字段 → 反推 partner_id". This is an architectural choice with three security/integrity consequences the PRD does not address:

1. **Auth on the binlog connection.** Canal needs MySQL replication user creds. These creds = "read everything in Fy-api forever". Where stored? Rotated? In CN, this user can read PII (`users.email`, `users.phone`, hashed password, OAuth tokens, channel keys for upstream providers — *all* in `transnext_db`).
2. **Schema drift.** Fy-api adds a column to `logs` (e.g. `cached_tokens` was added recently for Gemini billing — see `Bug分析-Gemini缓存命中未计费.md`). TraceNexBiz parses positionally → silent miscalculation of partner revenue. Or Fy-api renames a column → CDC silently drops rows. Combined with monthly upstream-sync, this is *guaranteed* to happen at some upstream merge.
3. **Rollback / phantom revenue.** If a row is `INSERT`ed at the binlog source then a transaction is rolled back, MySQL's binlog still emits the events for some isolation-level configurations. Or: replication failover causes binlog position rewind → revenue rows replayed → double-counted in `revenue_log` unless idempotency keyed on `FyApiLogId` with a unique index.
4. **CDC lag during settlement window.** §4.6 says "每月 1 号凌晨 02:00" for settlement. If CDC is 1-3s behind in normal conditions but 30-min behind during a backfill or pause-resume, the settlement runs against an incomplete `revenue_log` and **partners are paid based on partial data**. Need a "CDC freshness gate" before settlement starts (refuse to run if `now - max(revenue_log.occurred_at) > 60s`).

**Required fix in v0.2**: schema-drift gate (canary row count + checksum compared between `Fy-api logs` and `revenue_log` daily; alert if drift > 0.01%). `revenue_log.fy_api_log_id` MUST be a UNIQUE index. Replication user must be a separate, read-only, IP-restricted account, **not** the application DB user. Settlement gate on CDC freshness.

### H-2. Audit log (M4-15) is not append-only / tamper-evident

§8.13 `AuditLog` has `deleted_at` (because of §8 "所有表 ... `deleted_at`（软删除）"). A *soft-delete-able* audit log is not an audit log. A privileged DB user (or a compromised app account with `SUPER` on the DB user) can `UPDATE audit_log SET deleted_at = NOW() WHERE actor_id = X` and erase their own crime trail.

Also: §8.13 stores `DiffJson` — which will frequently contain PII (a KYC approval diff includes the legal-person name). This collides with PIPL right-to-delete at C-4.

**Required fix in v0.2**: `audit_log` must be append-only at the DB level (revoke `UPDATE`/`DELETE` for the application user; only the migration user has `ALL`). Add `prev_hash CHAR(64)` containing SHA-256 of the prior row's serialized contents — chains the log into a Merkle-style structure so any deletion is detectable. Optional: nightly snapshot + signature pushed to immutable storage (OSS with WORM bucket policy). For PII-in-audit-log: store *redacted* diffs by default (`{"legal_person_name": "<redacted>"}`); store the encrypted full diff in a separately-keyed `audit_log_pii` table that gets purged on right-to-delete.

### H-3. Payment webhook security (§7.6 M6-01..03) — totally undefined

The PRD does not say:
- WeChat Pay V3 callbacks are RSA-signed; signature must be verified against WeChat's certificate (not the merchant's). Replays to `/api/webhook/wechat` are trivially possible if not deduped via the `out_trade_no`.
- Alipay callbacks are RSA-signed similarly. Same dedup.
- IP allowlist for both providers' egress IPs (publicly published).
- Order tampering: client-generated order params (amount, currency) MUST NOT be trusted on the callback; the callback amount MUST equal the server-side order amount stored at order-creation time.
- §M6-07 says "防重 + 幂等" with "回调去重表" but doesn't define the dedup key. Must be `(channel, out_trade_no)` UNIQUE.

**Required fix in v0.2**: §7.6 must enumerate signature verification, IP allowlist, order amount cross-check, and idempotency on `out_trade_no` for every payment provider.

### H-4. Cross-partner customer migration is silently allowed in §9.2 — wallet implications

"客户跨渠道商迁移时 group 要改" (§9.2 risk table) — but §3.x and §6.4 do not specify the consent model. A partner-A customer's allegiance shifts to partner B by what mechanism? If partner B can do this without partner A's consent, partner A's wallet still has obligations to fulfill (already-billed-but-not-delivered quota). The PRD also does not address: who eats the unsettled `revenue_log` rows from partner A's last billing period?

This also collides with Q9 in §13 ("能否相互'挖墙脚'") — the answer to Q9 is a *security and integrity* question, not just a product question.

**Required fix in v0.2**: explicit "customer transfer" workflow with bilateral consent (partner A approves), settlement freeze on the in-flight billing period for that customer, audit log of the transfer.

### H-5. Markup ratio bounds — partner can set absurd values

§3.5 `partner_pricing.Markup float64` with no bounds. Attacks:
- Partner sets Markup = 0 → free quota for their customer at platform cost.
- Partner sets Markup = -1 → undefined behavior, may crash billing or *credit* the customer.
- Partner sets Markup = 1e308 → float overflow somewhere downstream.
- Partner sets Markup = 9999 → triple-digit invoice → customer charges back → platform eats the cost.

**Required fix**: validation at DTO boundary `validate:"gte=1.0,lte=10.0"` (or whatever the business cap is — needs Q1 from §13). Server-side enforced, *not* client-side. Apply OWASP A04.

### H-6. Phase 1 "绕过钱包" bypass is a security-architecture rollback

§12 Phase 1 verbatim: *"M3-04 客户额度分配 - 从平台直拨给客户，绕过钱包"*. This means the v1 production endpoint that allocates quota does not go through the wallet primitive. Any test, idempotency contract, audit trail wired to the wallet does not protect this path. When Phase 2 lands the wallet, the team will rewrite the allocation endpoint — and the old endpoint will linger as an internal "platform wallet" backdoor unless explicitly retired.

**Required fix**: Phase 1 still uses the wallet primitive. Seed the partner's wallet with a "trial balance" so the API surface is identical to Phase 2. The Phase 2 work then becomes "let partners top up their own wallet" rather than "introduce wallets at all".

---

## MEDIUM findings

### M-1. Decimal precision drift
§8.3 `Balance int64  // 单位：quota` is good — integer math, no float drift. But §3.5 `Markup float64` is bad: `int64(base * markup)` over millions of calls accumulates rounding bias. Use `decimal.Decimal` (shopspring/decimal) for the markup multiplication step, only convert to int64 at final step with banker's rounding.

### M-2. File upload — MIME type, magic bytes, virus scan
§M7-01 says "OSS 上传". Standard upload defenses missing: magic-byte validation (don't trust `Content-Type`), max-size enforcement at gateway and OSS bucket policy, ClamAV scan before exposing to admin reviewer (admins open the file = stored-XSS / RCE on admin browser).

### M-3. Invitation code redemption (M3-02) — no rate limiting
Without per-IP and per-account rate limits, a botnet enumerates partner invitation codes (likely short alphanumeric) to identify which partners exist and enumerate their partner-id range. Apply OWASP A07. Codes should be ≥ 16 chars from a high-entropy alphabet, rate limited at 5 attempts / IP / hour.

### M-4. XSS in partner-set fields
`partner.Notes`, `customer.Name`, `seat.Name`, partner-set invitation banner text (M1-05 / M3-02) all displayed in admin or customer UI. Must be HTML-escaped on render; CSP `script-src 'self' 'nonce-...'` on the admin domain. `partner.Notes` is internal but is rendered to admin staff — staff session compromise could escalate via stored XSS.

### M-5. Outbound third-party data flows (DPA inventory)
Aliyun OCR (legal name + ID), Alipay 芝麻认证 (real name + face), Aliyun SMS (phone for OTP), email provider (PII at minimum the email address). Each is an external data processor under PIPL Article 23 — DPA (数据处理协议) required, plus "individual consent" (单独同意) for each onward transfer. PRD doesn't list this.

### M-6. Logging & observability gotchas
§10 says "结构化日志：JSON 格式，关联 trace_id". No mention of PII scrubbing. A partner's email in a stack trace = PII in logs. Production log access controls — who can grep prod logs? CI engineers? Customer support? Without RBAC on the log pipeline, log access is a side-channel into all PII. Add a structured-log middleware that drops any field tagged `pii:"true"` and converts known patterns (ID number, phone, email) to redacted forms.

### M-7. Supply chain
No SBOM, no `go mod` version pinning policy, no Dependabot/Renovate rule, no policy on Alipay/WeChat SDK versions (these SDKs have a long CVE history). Add `gosec`, `govulncheck`, and `bun audit` to CI pre-merge. Lock `go.sum` integrity check in CI.

### M-8. Secret management
DB password, Fy-api shared HMAC key, OSS access key, KMS root credentials, payment merchant keys. PRD never names where they live. Required: a secret manager (Aliyun KMS Secrets, Vault, or at minimum env-injected from a sealed CI secret store), no plaintext in repo, no `.env.production` checked in, rotation runbook in §11.

### M-9. Per-partner rate limiting
Not just for security but for reliability. A partner whose admin UI hammers `/api/customer/list` in a tight loop should be throttled before they DoS the cross-DB JOIN (§6.3 cross-library queries are not free).

### M-10. JWT secret rotation
If TraceNexBiz reuses Fy-api's JWT secret (recommended in C-5), rotation of that secret invalidates all sessions in *both* systems. Need a key-set (`kid` header) with N+1 keys during rotation.

---

## LOW findings / hardening recommendations

1. **Rate limiting**: per-partner, per-IP, per-account. Default 60 req/min on read endpoints, 10 req/min on write, 3/min on auth endpoints (login, password reset, OTP).
2. **CORS**: explicit allowlist (`partner.tracenex.cn` only); never `*`.
3. **Security headers**: `Content-Security-Policy`, `Strict-Transport-Security` (HSTS, preload), `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`, `Permissions-Policy` to disable unused browser features.
4. **Cookie flags**: `Secure; HttpOnly; SameSite=Strict` (or `Lax` if cross-subdomain navigation required).
5. **Password policy**: min length 12, breach-corpus check (HIBP API or k-anonymity offline list), rate-limited login.
6. **Account enumeration**: login error messages must be uniform regardless of "user not found" vs "wrong password".
7. **KYC reviewer's session timeout**: short (15min idle) since they handle ID photos.
8. **Backup encryption**: §10 says "RDS 自动备份每日 + 月结后单独 dump". Backup encryption at rest + access control on dump location is unspecified.
9. **Disaster recovery**: an attacker who compromises a partner account can issue a `DELETE` cascade via the soft-delete column. Recovery procedure (point-in-time restore vs. soft-delete revival) needs runbook coverage.
10. **Time-of-check-time-of-use** in saga: `partner.Status` is checked in middleware, then 200ms later the saga runs — meanwhile a staff member has frozen the partner. Re-check status inside the wallet transaction.
11. **Internal cron**: M5-01 distributed lock via Redis SETNX is fine, but the lock TTL must be > expected job duration; otherwise a long-running settlement gets a duplicate sibling.

---

## Concrete code-level patterns required for v1.0

### Tenant scoping middleware (Go)

```go
// middleware/scope.go
package middleware

type ActorContext struct {
    ActorType string  // "partner" | "customer" | "staff" | "system"
    ActorID   int64
    Elevated  bool
    TraceID   string
}

const ctxKeyScope = "scope"

func RequirePartnerScope(repo PartnerRepo) gin.HandlerFunc {
    return func(c *gin.Context) {
        userID := c.GetInt64("id") // from Fy-api JWT
        partner, err := repo.FindByFyUserID(c.Request.Context(), userID)
        if err != nil || partner == nil || partner.Status != PartnerStatusActive {
            c.AbortWithStatusJSON(http.StatusForbidden, errorBody(ErrNotPartner))
            return
        }
        c.Set(ctxKeyScope, ActorContext{
            ActorType: "partner",
            ActorID:   partner.ID,
            TraceID:   c.GetString("trace_id"),
        })
        c.Next()
    }
}

// Repository contract — every read takes scope. There is no "unscoped" version.
func (r *CustomerRepo) FindByID(ctx context.Context, scope ActorContext, id int64) (*Customer, error) {
    q := r.db.WithContext(ctx).Model(&Customer{}).Where("id = ?", id)
    switch scope.ActorType {
    case "partner":
        q = q.Where("partner_id = ?", scope.ActorID)
    case "customer":
        q = q.Where("id = ?", scope.ActorID)
    case "staff":
        if !scope.Elevated {
            return nil, ErrForbidden
        }
        // Elevated staff queries MUST be audited
        r.audit.Record(ctx, scope, "customer.read.elevated", id, nil)
    default:
        return nil, ErrForbidden
    }
    var c Customer
    if err := q.First(&c).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, ErrNotFound // 404, never 403 (don't leak existence)
        }
        return nil, fmt.Errorf("customer find: %w", err)
    }
    return &c, nil
}
```

Acceptance test (one per endpoint, mandatory in CI):

```go
func TestCustomerDetail_BOLA(t *testing.T) {
    a, b := seedTwoPartners(t)
    custOfA := seedCustomer(t, a)
    resp := getAs(t, b.Token, "/api/customer/"+strconv.FormatInt(custOfA.ID, 10))
    require.Equal(t, http.StatusNotFound, resp.Code, "must not be 200 or 403")
}
```

### Wallet idempotent operation (Go)

```go
func (s *WalletService) AllocateToCustomer(
    ctx context.Context, scope ActorContext, req AllocateRequest, idemKey string,
) (*AllocateResponse, error) {

    if req.Amount <= 0 || req.Amount > maxAllocateAmount {
        return nil, ErrInvalidAmount
    }
    if cached, ok, err := s.idem.Lookup(ctx, scope.ActorID, idemKey); err != nil {
        return nil, err
    } else if ok {
        return cached.(*AllocateResponse), nil
    }
    return s.idem.Do(ctx, scope.ActorID, idemKey, func() (any, error) {
        var logRow PartnerWalletLog
        err := s.db.Transaction(func(tx *gorm.DB) error {
            var w PartnerWallet
            if err := tx.Where("partner_id = ?", scope.ActorID).
                Clauses(clause.Locking{Strength: "UPDATE"}).
                First(&w).Error; err != nil {
                return err
            }
            if w.Balance < req.Amount {
                return ErrInsufficientBalance
            }
            res := tx.Model(&PartnerWallet{}).
                Where("partner_id = ? AND version = ?", scope.ActorID, w.Version).
                Updates(map[string]any{
                    "balance": gorm.Expr("balance - ?", req.Amount),
                    "version": gorm.Expr("version + 1"),
                })
            if res.Error != nil {
                return res.Error
            }
            if res.RowsAffected != 1 {
                return ErrConcurrentUpdate
            }
            logRow = PartnerWalletLog{
                PartnerID:    scope.ActorID,
                Type:         "allocate_to_customer",
                Amount:       -req.Amount,
                BalanceAfter: w.Balance - req.Amount,
                RefID:        idemKey,
                Status:       "pending",
            }
            return tx.Create(&logRow).Error
        })
        if err != nil {
            return nil, err
        }
        // Call Fy-api with the same idempotency key.
        if err := s.fyApi.Topup(ctx, req.CustomerFyUserID, req.Amount, idemKey); err != nil {
            // Compensate via a NEW row, never roll back.
            s.compensate(ctx, scope, logRow.ID, req.Amount, err)
            return nil, fmt.Errorf("upstream topup failed: %w", err)
        }
        s.markCommitted(ctx, logRow.ID)
        return &AllocateResponse{IdemKey: idemKey, NewBalanceAfter: logRow.BalanceAfter}, nil
    })
}
```

### PII encryption at rest (envelope)

```go
// pkg/crypto/pii.go
type PIIVault struct {
    kms      KMSClient    // Aliyun KMS
    dekCache *lru.Cache   // KEK-encrypted DEK -> plaintext DEK, TTL 1h
}

func (v *PIIVault) Encrypt(ctx context.Context, plaintext []byte, tenantID int64) (Ciphertext, error) {
    dek, encDek, err := v.fetchOrGenerateDEK(ctx, tenantID)
    if err != nil {
        return Ciphertext{}, err
    }
    nonce := make([]byte, 12)
    if _, err := rand.Read(nonce); err != nil {
        return Ciphertext{}, err
    }
    aead, _ := cipher.NewGCM(aesCipher(dek))
    ct := aead.Seal(nil, nonce, plaintext, nil)
    return Ciphertext{Nonce: nonce, EncryptedDEK: encDek, Body: ct, KeyID: v.activeKeyID()}, nil
}

// GORM custom type that marshals to encrypted blob and is dropped from logs.
type EncryptedString struct {
    plain  string `pii:"true" json:"-"`        // never serialized
    cipher Ciphertext
}
```

### Audit log append-only

```sql
-- Migration only, app user has no UPDATE/DELETE on this table
CREATE TABLE audit_log (
    id           BIGINT       PRIMARY KEY AUTO_INCREMENT,
    actor_type   VARCHAR(16)  NOT NULL,
    actor_id     BIGINT       NOT NULL,
    action       VARCHAR(64)  NOT NULL,
    target_type  VARCHAR(64),
    target_id    BIGINT,
    diff_redacted JSON,                 -- PII removed
    diff_pii_id  BIGINT,                -- pointer to audit_log_pii (purged on right-to-delete)
    ip_address   VARCHAR(45),
    user_agent   VARCHAR(255),
    occurred_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    prev_hash    CHAR(64)     NOT NULL,
    self_hash    CHAR(64)     NOT NULL
) ENGINE=InnoDB;

-- Trigger: prev_hash MUST match prior row's self_hash (or all-zeros for row 1).
-- Application user GRANTs: SELECT, INSERT only. No UPDATE, no DELETE.
```

---

## Compliance hooks (PIPL / GDPR-style)

- **PIPL Article 17 (notice)**: every form collecting PII must show purpose, retention, third-party processors. Add as a UI requirement to M1-06, M2-12, M7-*.
- **PIPL Article 19 (retention minimization)**: explicit per-field retention table. Build a `retention_policy` config table consumed by purge jobs.
- **PIPL Article 23 (onward transfer)**: each external processor (Aliyun OCR, Alipay 芝麻, SMS, email) needs a DPA + individual consent record stored in `consent_log` keyed by `(user_id, processor, purpose, version, timestamp)`.
- **PIPL Article 47 (deletion)**: customer right-to-delete must crawl `customer`, `kyc_application`, `seat`, `invoice_application`, plus emit Fy-api `/api/internal/user/erase` request. Audit log entries must be retained but PII fields within them tombstoned (the "diff_pii_id" pointer above). This is the audit-vs-purge tension flagged in C-4.
- **PIPL Article 51 (security obligations)**: encryption, access control, training records, incident response runbook.
- **PIPL Article 38 / 跨境数据流动**: §11 mentions "中国用户的 KYC 在 CN 库；其他 SG 库" — that's correct in spirit but needs a *legal-basis* record, not just a routing rule. SG users whose KYC is in SG RDS but who later log in from CN must not have their PII auto-pulled cross-border.

---

## Required additions for v0.2

- **§S Threat Model** — STRIDE per component, attacker capabilities, trust boundaries (browser, partner-app, TraceNexBiz, Fy-api, RDS, OSS, KMS, payment providers).
- **§A Authentication & Session** — session source of truth, cookie scoping, CSRF, MFA matrix per role, JWT revocation, password reset flow.
- **§I Idempotency Contract** — every wallet, quota, refund, payment endpoint; idempotency record schema; TTL.
- **§K Key Management** — KMS choice (Aliyun KMS recommended), envelope encryption, DEK rotation, secret-rotation runbook.
- **§S.PII** — PII inventory table, retention matrix, log-scrubber spec.
- **§S.Audit** — append-only audit log, hash-chained, immutable backup cadence.
- **§6.1 expanded** — canonical signing scheme, mTLS, key-rotation protocol, per-service-account scope.
- **§9.3 expanded** — CDC auth, schema-drift detection, idempotency on `revenue_log.fy_api_log_id`, settlement freshness gate.
- **§7.6 expanded** — payment webhook signature verification, IP allowlist, replay defense, order tampering check.
- **§11 expanded** — security risks at the same level of granularity as engineering risks; explicit owners.
- **§13 (open Q's)** — add Q11 (which KMS), Q12 (DPA owner for each external processor), Q13 (PIPL retention exception for payment evidence), Q14 (markup ratio bounds), Q15 (customer-transfer consent model).

---

## Recommendation

**BLOCK.** v0.1 is a *product* PRD masquerading as a *system* PRD. The product story is coherent and the data model is reasonable, but the security primitives that the entire money + PII story rests on are one-line aspirations. Engineering cannot start Phase 1 against this document without inventing the auth scheme, the idempotency contract, the scoping middleware, the KMS strategy, and the audit trail on the fly — and inventions made under deadline pressure are exactly what produces the BOLA + double-spend + PII-leak triple-header that ends Chinese SaaS on the front page of 36Kr.

Concrete next steps:

1. Spend ~3 days writing §S, §A, §I, §K. These are the four sections that matter; everything else is downstream.
2. Convene a threat-modeling session (1 product, 1 architect, 1 security, 1 SRE) before v0.2.
3. Promote §11's risk table from "table" to "treatment plan with owner + SLA + accept/mitigate/transfer decision".
4. Defer Phase 1's "绕过钱包" shortcut (M3-04). Build the wallet primitive first; allocate via wallet from day one even if balance is seeded magically. The Phase 2 work then becomes "let partners top up their own wallet" rather than "introduce wallets at all."
5. Make the BOLA test mandatory in CI: every read endpoint that returns data scoped to an actor must have a "cross-tenant request returns 404" test. Block merge if missing.
6. Confirm with Fy-api team that they will support an `Idempotency-Key` header on every internal-API write endpoint, before Phase 2 begins.

Once v0.2 lands those four sections at the level of detail above, this becomes a realistic green-light candidate.
