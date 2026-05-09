# PRD v0.2 Review — Product Manager (Round 2)

> Date: 2026-05-09
> Reviewer: PM agent
> Verdict: **ACCEPT_WITH_NOTES** (effectively v1.0; three HIGH notes to fold into a v1.0.1 patch or Phase-2A kickoff doc)

## Summary

v0.2 is a substantive, mostly-honest revision. All five Round-1 CRITICAL items have been materially addressed (not just banner-claimed): churn/wind-down lifecycle is now §4.8–§4.17 with backing state machines in §14; Phase 1 has been re-scoped so the wallet+saga path is exercised from day one (no more "绕过钱包"); the markup model is now `partner_pricing_rule` with rule resolution and price-temporality semantics; success metrics live in §1.4 with per-phase numeric exit criteria; and settlement-period changes have an audit table + state-machine story. Eight of nine Round-1 HIGH items are addressed; the ninth (KYC URL post-purge nulling) is partially addressed via the hot/cold dual-column design but the purge-time policy isn't stated explicitly. The newly added §15 (compliance), §16 (STRIDE), §17 (auth/session), §18 (idempotency) and §19 (key management) substantially raise this from a product-only spec to something architecture and security can actually plan against.

The remaining concerns are real but not blocking for Phase 1. The biggest is that **§8.6's per-model markup is data-modeled but has no execution path through Fy-api** — the §9.2 `group_ratio` + `GroupRatioOverride` chain is a per-call single scalar and cannot vary by model. Phase 1 doesn't need it (M3-13 is "single layer"), but Phase 2A's M3-08 will hit a wall on day one unless either (a) a per-(user, model) override hook is added to the Fy-api overlay (附录 C-4 needs an extension), or (b) per-model markup is explicitly deferred to v1.x and §8.6's `ModelName` column documented as "schema-only, not honored in v1.0." This needs to be settled in a v1.0.1 patch before Phase 2A kicks off, but does not block engineering from starting Phase 1 today.

Two other HIGH items: (i) the refund-of-already-settled-and-paid-out flow allows `partner_wallet.balance` to go negative ("balance go negative is OK") with no formal debt object or partner-side repayment UI/process; and (ii) the customer-payment → Fy-api quota top-up flow (持牌方 callback → `POST /api/internal/user/topup`) isn't drawn anywhere in §7.6. Both are tractable but should be filled in before Phase 2A code starts.

Net: engineering can credibly begin Phase 1 against this PRD without inventing answers. Treat this as v1.0 with three flagged follow-ups.

## CRITICAL issues (must-fix before v1.0)

**None.** All five Round-1 CRITICALs are addressed (see "Round-1 verification" below).

## HIGH issues (should fix before Phase 2A starts; not Phase 1 blockers)

1. **Per-model markup is in the schema (§8.6 `partner_pricing_rule.ModelName`) but absent from the execution chain (§9.2).** §9.2 v1.0 supports two enforcement points: per-tier group_ratio, and per-customer `User.GroupRatioOverride` — both are single scalars per request. Neither varies by model. So a partner who creates a rule "markup 1.05 on gpt-4o, 1.50 on gpt-3.5-turbo" cannot have it honored: at request time Fy-api has only one ratio number to apply. Phase 1 ducks this because M3-13 is "single layer" — but **M3-08 in Phase 2A explicitly requires multi-layer including model**, and the PRD as written does not say how that lookup happens at the hot path. Decide before Phase 2A: (a) extend §C-4 with a per-(user, model) override map (probably another override field on `User` or a small `user_model_ratio` table read by Fy-api), accepting more overlay LOC; or (b) explicitly out-of-scope per-model for v1.0 and document that `ModelName` in `partner_pricing_rule` is schema-forward-only. Either is fine; ambiguity is not.

2. **Refund of already-settled-and-paid-out revenue creates an undefined "partner debt" state** (§9.4 step 3, §4.10 column 3, §8.3). The saga explicitly says "balance go negative is OK（追偿到下个 period）," but the data model has no `partner_debt` row, no partner-facing UI for "you owe the platform ¥X," no statute-of-limitations rule for how long platform can claw back, and no failure mode for "partner has zero future revenue from which to claw back." `partner_wallet_log.type='refund_clawback'` is named but not formally described. Settlement code will silently produce negative balances and ops will discover them in the daily drift report. Add either a `partner_debt` table (with state machine: open → repaying → settled / written_off) or document the clawback as "negative `partner_wallet.balance` is the canonical debt representation" with a max-debt threshold + auto-suspension trigger.

3. **Customer payment → Fy-api quota top-up flow not drawn in §7.6.** §7.6 shows fund flow up to "持牌方按 TraceNex 指令做实时分账 / T+1 分账" with payments going to platform/partner sub-accounts — but **how does the customer's `User.quota` actually get incremented?** Presumably 持牌方 success-callback → TraceNexBiz handler → `POST /api/internal/user/topup` (idempotent). This is a state-changing path that touches Fy-api, has a saga shape, and has all the same crash-recovery concerns as M3-04. It needs the same §9.4-style spec. Right now M2-03 says only "持牌分账方收单." Engineering will reinvent the saga for this on day-1 of Phase 2A. Add a §9.4 sub-section "客户充值 saga (M2-03)" with the same idempotency/by-idem-key/timeout treatment.

## MEDIUM issues

1. **KYC URL nulling on PII purge is implied but not stated** (§8.9, §7.7 M7-07). Two columns exist (`LegalPersonIdUrl` hot, `LegalPersonIdArchiveUrl` cold), and `PiiPurgedAt` is a timestamp, but the PRD never says "at purge time, set `LegalPersonIdUrl = NULL` and copy/move artifact to cold archive bucket." Without that explicit step, a future bug or migration could re-expose dead presigned-URL paths. One sentence in §7.7 fixes this. (Round-1 HIGH #8 was partially addressed; this finishes it.)

2. **`partner_pricing_rule` UNIQUE constraint allows overlapping `valid_from`/`valid_to` ranges** (§8.6). UNIQUE is on `(partner_id, customer_id, model_name, tier_name, valid_from)` — two rules with the same scope and overlapping but different `valid_from` (e.g. 5/1–5/31 and 5/15–6/15) both pass UNIQUE. Rule resolution becomes nondeterministic. Either add an application-layer check ("no two active rules with overlapping windows for the same scope") or change the constraint shape to "at most one active rule per scope at any wall-clock instant." Important because §9.5 promises historical revenue stays at the rule then in force — if two rules were "in force" simultaneously, the audit trail breaks.

3. **§13 Q6 (refund window) is BLOCK-Week 1 but §4.10 already references "客户主动 7 日内"** — i.e. the document is internally pre-deciding a question that's still listed as unresolved. Either drop the "7 日内" assumption from §4.10 (replace with `${refund_window_days}`) or strike Q6 from §13.

4. **Audit log query/read path remains unspec'd** (§8.13). v0.2 added the hash chain (good) but didn't address the Round-1 MEDIUM #6 concern: how does ops search audit_log by `actor_id`, `target_type+target_id`, `action`? With millions of rows + hash-chain integrity, indexes are non-trivial (PrevHash chain forces ordered append, but read patterns will need actor_id + occurred_at composites). Either spec the indexes or punt explicitly to "ETL to OLAP" (and document who uses what).

5. **Phase 1 partner initial-balance provenance not in schema flow.** §12.1 says "渠道商初始余额由平台 staff 预拨." That is an `partner_wallet_log` row with `type='adjustment'`, `OperatorType='platform_staff'`, written by `wallet.adjust` (per §3.4 a finance-only verb). Fine — but worth saying explicitly so the seed-partner runbook doesn't reinvent it.

6. **§14.2 Customer state machine "5d 后软删除生效" vs §4.17 immediate.** §4.17 reads as immediate (`customer.deleted_at = NOW()`), but the state-machine line implies a 5-day grace. Pick one and align.

7. **`system` actor in §3.4 has no row in §8.14 `staff`.** §3.2 calls it implicit ("不能登录"), but `audit_log.ActorType = 'system'` rows still need a stable actor_id. Document: "system actor uses ActorType='system', ActorId=0 (or per-cron well-known IDs)" — otherwise FK orientation between `audit_log.actor_id` and `staff.id` is ambiguous.

8. **§17.2 MFA threshold "wallet > 0 时强制" needs re-grounding given the wallet redefinition.** Wallet is now an *应付台账负债* — a brand-new partner has wallet=0, accumulates earnings into wallet>0, then gets paid out (back to ~0). So in practice every partner that ever earned anything must have MFA forever. Probably the intended behavior. Just say so explicitly.

9. **Phase 2B "Week 8-10" overlaps Phase 3 "Week 10-13"** (§12.3, §12.4). Fine if intentional (parallel tracks), but call it out.

10. **EPay edge case** (§7.6 M6-04). EPay is "P1 — 仅直营客户." What happens if a partner-attached customer's account is mistakenly funded via EPay (e.g. legacy direct user converted to partner-attributed via §4.14)? Reject? Convert to direct-flow with audit? Define.

## LOW issues

1. **§9.1 worked example uses single scalar** — it doesn't exercise the new per-customer-or-model rule lookup. A second worked example showing rule-resolution in action would help engineers.
2. **Glossary §20 / Appendix D duplication.** §20 *is* the glossary; appendix D just says "see §20." Drop one.
3. **Phase 1 includes M2-12 KYC stub but §1.4 Phase 1 exit criteria don't mention KYC** — slightly inconsistent (Phase 1 ships an inert KYC button; what does "stub" mean for the exit gate?).
4. **§4.16 (zero-balance) calls Fy-api 402 "Insufficient Quota"** — confirm Fy-api actually returns that exact code/string, or document the substring contract.
5. **`PartnerWallet.Version int64` (§8.3) optimistic lock** — saga in §9.4 uses both `LOCK ... FOR UPDATE` (pessimistic) and `version+=1` (optimistic). Pick lane or document why both are needed.

## Round-1 verification (each prior CRITICAL/HIGH)

### CRITICAL

| # | Round-1 CRITICAL | v0.2 status | Where |
|---|---|---|---|
| C1 | Churn / wind-down lifecycle | ✅ Addressed | §4.8–§4.17 (H–Q), §14 state machines |
| C2 | Phase 1 not a usable alpha | ✅ Addressed | §12.1 — wallet+saga from day 1, initial balance pre-funded; M3-04 via wallet hold |
| C3 | Markup pricing under-specified | ✅ Addressed (with HIGH #1 caveat on per-model execution path) | §8.6, §9.5 |
| C4 | Zero KPIs | ✅ Addressed | §1.4 with phase exit criteria + ongoing KPIs |
| C5 | Settlement-period configurability | ✅ Addressed | M5-11, §8.8 `SettlementConfigChangeLog`, §14.3 |

### HIGH

| # | Round-1 HIGH | v0.2 status |
|---|---|---|
| H1 | Phase 2 4-week kitchen sink | ✅ Split into Phase 2A (Week 5-7) / 2B (Week 8-10) |
| H2 | Roles model under-specifies platform staff | ✅ §3.4 permission matrix with verbs × roles |
| H3 | No partner sub-accounts | ✅ Explicitly v1.x out-of-scope (§3.3); schema-compatible |
| H4 | Customer-side experience invisible | ✅ §7.9 (M9-01 强制可见, M9-02 防绕过) |
| H5 | Partner activation funnel | ✅ Reasonably covered in §4.1/§4.2/§4.12; KPI-tracked in §1.4 |
| H6 | Refund / reversal semantics | ✅ §4.10 matrix + §9.4 refund saga (HIGH #2 leftover on settled-and-paid clawback) |
| H7 | CDC SPOF | ✅ Replaced with application-layer outbox (§9.3); freshness gate added |
| H8 | KYC PII URL post-purge | ⚠️ Partially — dual-column structure exists (§8.9), but explicit NULL-at-purge step missing (now MEDIUM #1) |
| H9 | Customer-zero-balance flow | ✅ §4.16 + M2-13 |

## Section-level notes

### §1.4 Success Metrics

Solid. One nit: "平均有效 markup（按 GMV 加权）业务回填基线" is target=baseline-tbd, which is honest but brittle. Suggest fixing a numeric placeholder (e.g. "≥ 1.15 weighted") so the gate is testable; revise after Phase 1 data lands.

### §3.4 Permission Matrix

Clear and CI-testable. Two gaps:
- Add `dispute.arbitrate` and `ticket.assign` rows.
- Add a column for `system` actor (cron, outbox poller) so audit_log has a clean source attribution. Currently `system` is in §3.2 narrative but absent from §3.4 grid.

### §4 Scenarios

H–Q now exist. Of these, scenario H (cross-partner switch) is the only one that touches schema (`customer_partner_change_log`, frozen-revenue-to-A semantics). Worth adding one explicit invariant: **`revenue_log.partner_id` is immutable** (set at write time and never reassigned by the switch flow). Otherwise a future migration may "fix" it and silently re-route past revenue.

### §7.6 Payment

The fund-flow diagram is good, the 二清 anti-pattern is correctly disowned. Missing: the customer-payment → quota-top-up sequence (HIGH #3). M6-08 金额校验 is correctly P0.

### §8.6 partner_pricing_rule

Resolution priority is documented ("exact (customer_id + model_name) > customer_id > model_name > partner default"). Good. Issues: TierName isn't in the priority chain (MEDIUM #2 cousin); UNIQUE doesn't prevent overlapping windows (MEDIUM #2); execution path doesn't honor model_name (HIGH #1).

### §9.3 Outbox

Crisp design. The freshness-gate addition ("settlement Cron refuses to run if outbox MAX(occurred_at) is stale") is exactly the right defensive move. 

### §11 Risk register

R-1 through R-25 with type tags + owners is now genuinely actionable. Notable absences (re-add as MEDIUM if pursued): risk of `partner_pricing_rule` execution-path drift (HIGH #1); risk of partner-debt-overflow (HIGH #2); risk of customer-payment saga not being designed (HIGH #3).

### §12 Milestones

Phase 1 scope is now defensible — a seed partner CAN transact (against pre-funded wallet), and the saga is exercised. Phase 2A/2B split tracks the dependency reality. Week 0 is now a proper deliverable list.

One concern: **Phase 1 also includes M12-01 (model whitelist) + M12-02 (input-side keyword review)**. Combined with KYC stub, consent UI, BIGINT migration, six Fy-api overlay PRs, and the Pub/Sub machinery, Phase 1 is no longer a "thin alpha" — it's a non-trivial 4 weeks of work. That's fine, but be honest in scheduling: this is a 4-week sprint that can't absorb a 1-week slip without slipping Phase 2A. Add 1 week of buffer or accept the cascade.

### §13 Open questions

Tagging is the right move. Three of the new BLOCKs are genuinely Week-0/Week-1 critical: Q11 (注册资本), Q12 (持牌方 selection), Q16 (合同模板). Q13 (DPO) and Q14 (算法备案) realistic at Week 1. **Action: hold a Week-0 close-out checkpoint where Q11/Q12/Q13/Q16 are answered before any Phase 1 schema migration runs.**

### §15 Compliance

This is the section that elevates the document. The 15.1 table is an executable program, not a wishlist. One additional hardening: §15.10 "Pre-launch 合规清单" is blockbox-style; add an owner column so each box has a name attached.

### §16–§19 (security/auth/idempotency/keys)

These are largely architecture/security territory and I'll defer detailed assessment to those reviewers, but from a product-completeness standpoint they're sufficient for engineering to plan against. The §16.3 BOLA test pattern (return 404 not 403 on cross-tenant access) is precisely correct and CI-enforceable.

## Missing entirely (still)

- **Owner column on §15.10 pre-launch compliance checklist** (LOW lift)
- **`partner_debt` model OR explicit "negative wallet = debt" doctrine with auto-suspend threshold** (HIGH #2)
- **Customer-payment saga spec** (HIGH #3)
- **Per-model markup execution path** (HIGH #1)
- **`partner_pricing_rule` overlapping-window enforcement** (MEDIUM #2)

Notably *resolved* missing items from Round-1: pricing temporality (§9.5), customer-experience/branding (§7.9), permission matrix (§3.4), state machines (§14), customer-support module (§7.10), notifications module (§7.11), anti-abuse on trial (M1-08), partner sandbox (M9-04), notification preferences (M11-03), glossary (§20), i18n scope (§10.7).

## Recommendation

**Verdict: ACCEPT_WITH_NOTES.** Promote this to v1.0 (or v1.0-rc) and start Phase 1 against it. The three HIGH items above do not block Phase 1 — they block Phase 2A. Resolve them in a small v1.0.1 patch during weeks 1–4 of Phase 1, in parallel with implementation. Concretely:

1. **Week 1**: Decide HIGH #1 (per-model markup). Likely answer: extend §C-4 with a `user_model_ratio_override` table read by Fy-api hot path, OR explicitly defer per-model to v1.x and strike `ModelName` from §8.6 (keep it nullable, document v1.0 contract). Either path is a 1-day decision.
2. **Week 2**: Spec HIGH #2 (settled-then-paid clawback). Either `partner_debt` table or "balance can go negative, threshold = -X triggers auto-suspend per §14.1." 1-day decision + 0.5-day schema add.
3. **Week 3**: Spec HIGH #3 (customer-payment saga). Mirror the §9.4 M3-04 saga structure; reuse `wallet_hold` semantics or introduce `topup_intent`. 1-day spec.
4. Issue v1.0.1 with all three resolved + the MEDIUM cleanups, in time for Phase 2A kickoff.

Phase 1 engineering can begin immediately on: data model (sans `partner_debt`), §6.1 internal API + HMAC + mTLS, BIGINT migration, GroupRatioOverride field, outbox table + same-tx write, Pub/Sub plumbing, permission matrix middleware, BOLA matrix tests, KYC stub UI + consent_log, M3-13 single-layer markup, model whitelist (M12-01), basic content-safety hooks (M12-02). All of these are stable in v0.2.

Confidence: high that v0.2 is a credible foundation; medium-high that v1.0.1 closes the remaining product gaps without disturbing Phase 1 work in flight.

## Issue counts

| Severity | Count |
|---|:---:|
| CRITICAL | 0 |
| HIGH | 3 |
| MEDIUM | 10 |
| LOW | 5 |

CRITICAL = 0 and HIGH = 3 meets the "graduate to v1.0" bar. Ship.
