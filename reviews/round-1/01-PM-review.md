# PRD v0.1 Review — Product Manager (Round 1)

> Date: 2026-05-09
> Reviewer: PM agent
> Verdict: **NEEDS_REVISION**

## Summary

PRD v0.1 is a strong engineering-leaning first draft — the data model (§8), Fy-api integration boundary (§6), and the calculation chain (§9) are unusually mature for an initial document. As a **product** specification, however, it has significant gaps: §4's seven scenarios are happy-path only and ignore essentially every wind-down flow (churn, suspension, partner-switch, partner-termination, refund, dispute-as-journey, account-merge); the Phase-1 MVP as scoped is **not a usable alpha for any real partner** — it is an internal integration demo wearing alpha clothes; pricing/markup is modeled as a single scalar per partner-customer pair which will not survive contact with a real distributor; and the document defines **zero success metrics**, so we have no way to decide when Phase 1 is "done enough" to graduate. Recommend a focused v0.2 pass on lifecycle completeness, MVP repackaging, pricing semantics, and KPIs before architect/security/compliance go deeper — otherwise their reviews will redo half the work.

## CRITICAL issues (must-fix before v1.0)

1. **No churn / wind-down lifecycle anywhere** (§4). Seven scenarios in §4 cover acquisition + happy-path billing only. Completely missing: customer suspension by partner, customer-leaves-platform refund, **customer wants to switch partners** (orphaning, balance transfer, group remap), partner-initiated customer churn, account merge, billing-dispute-as-a-journey (M5-09 is one bullet, not a journey), partner termination/offboarding (acknowledged in §13 Q8 but not designed), data-deletion request (PIPL/GDPR). Each of these requires product decisions that materially shape the schema (e.g., who owns the residual customer balance when a partner is terminated? does the customer's API key keep working during a 30-day grace? does the customer auto-migrate to direct?). Without designing them now, engineering will invent answers ad hoc and they'll be wrong.

2. **Phase 1 MVP is not a usable alpha** (§12.1). §12 explicitly excludes wallet, markup, native payment, KYC, CDC, monthly settlement. What remains is "platform creates partner manually → partner generates invite link → customer signs up → customer calls API at platform list price → partner sees a list of their customers." A seed partner cannot make money in this state — there is no markup, no settlement, no payout, no commercial proof point. This is an **internal integration demo**, not an alpha. Either (a) honestly relabel Phase 1 as "internal verification" and shorten the timeline to 2 weeks, or (b) move a thin slice of markup-via-group-ratio (a single scalar markup configured by ops) AND a manual settlement path (CSV export + bank transfer) into Phase 1 so a seed partner can actually transact. As currently written, the timeline misleads stakeholders about when revenue can flow.

3. **Markup pricing is dangerously under-specified** (§7.3 M3-08, §8.5, §9.2). `partner_pricing` is modeled as a single `Markup float64` per partner. Real partners will demand within their first month: per-model markup (cheap models can absorb 50%+ markup; frontier models barely 5%), time-limited promotions, volume tiers (rate A on first 1k tokens, rate B above), per-customer-segment pricing, and feature-gated markup (e.g. only mark up image generation). The §9.2 "B-plan" relies on group_ratio, which is **per-group, not per-model-within-group** — meaning the moment markup must vary per model, group_ratio alone is insufficient and §9 needs a rewrite. Decide for v1.0: are we shipping single-scalar forever (and telling partners "tough"), or is the data model `partner_pricing_rule (partner_id, customer_id?, model?, valid_from, valid_to, markup, tier_threshold?)`? This decision cascades through §7.3 (partner UI), §8.5 (schema), §9 (calculation), and §11 (risk).

4. **Zero success metrics / KPIs** (entire doc). Nowhere does the PRD define what "Phase 1 succeeded" means quantitatively. §12 declares phases by feature checklist, not by business outcome. Need at minimum: # partners onboarded, # customers per partner (median + P90), GMV via partner channel vs. direct, partner activation rate (apply → first revenue dollar), 30-day partner retention, average effective markup across partners, settlement-on-time rate, KYC pass rate, KYC time-to-decision P95, customer churn under-partner, wallet-reconciliation drift events. Without these, every milestone debate becomes "the features exist, ship it" and we'll learn the alpha was a flop only after Phase 2 is half-built.

5. **Settlement-period configurability is asserted but not modeled** (§2, §7.5, §8.7). §2 promises "configurable T+0/T+7/monthly," but §8.7 `settlement` has `Period string` like `'monthly_2026_05'` and a single `PeriodStart/PeriodEnd` pair — adequate for fixed-cadence monthly only. **What happens when a partner is moved from monthly to weekly mid-month?** (Real case: partner negotiates faster payout terms.) Pro-rate the in-flight period? Force-close current and start fresh? The data model has no answer. v1.0 must add an explicit cutover rule and probably a `settlement_config_change_log` (or similar audit) so a partner whose terms change can be reconstructed forensically.

## HIGH issues (should fix)

1. **Phase 2 is a 4-week kitchen sink** (§12.2). KYC + wallet + markup + CDC + monthly Cron + native WeChat + native Alipay + revenue dashboard, in 4 weeks, while WeChat/Alipay merchant qualification (realistically 4–6 weeks, not the 2–3 stated) runs in parallel. CDC infra (canal/maxwell + ack tracking + replay tooling) is by itself a 1–2 week effort if no one has done it. Realistic Phase 2 = 6–8 weeks. Recommend splitting into Phase 2A (wallet+markup+CDC+KYC — unblocks revenue recognition) and Phase 2B (settlement+native payment — unblocks payouts), with an explicit go/no-go between them.

2. **Roles model under-specifies platform staff** (§3, §8.14). §3 enumerates 3 roles. §8.14 `staff` has 4 sub-roles (`super_admin / operations / finance / support`) and §3 alludes to them, but no workflow is defined per sub-role. Operations does what? Finance does what? Support does what? Each implies different views and write permissions (support can issue refunds up to ¥X but cannot adjust markup; finance can mark settlements paid but cannot approve KYC; operations approves KYC but cannot touch wallet). v1.0 must add an explicit role × action permission matrix.

3. **No partner sub-accounts / partner staff model** (§3, §8.1). A real partner is rarely a single person — they have a sales rep, a finance person, a tech contact. Currently a partner is one Fy-api `User` and one `partner` row. By Phase 2 partners will request: "I want to give my finance person read-only access to billing without seeing customer markup." Either commit to single-seat partners forever (will block enterprise sales) or scope a `partner_member` table for v1.x with documented out-of-scope status now.

4. **Customer-side experience is invisible** (§7.2). The PRD never answers: does the customer **know** they are under a partner? Is the partner's brand visible? Is white-label support an option? What happens if the customer Googles "TraceNex topup" and lands on the direct platform — do they get bounced to their partner, or do we let them buy direct (and if so, who gets the commission)? **This is the #1 revenue-leak failure mode of channel programs** and the PRD is silent on it. Partners will not invest in customer acquisition if they can be silently bypassed.

5. **Partner activation funnel is one paragraph** (§4.2, §7.7). The PRD describes "submit → review → approved/rejected" but does not specify: how many fields on the form, what the partner sees on the rejection screen, whether they can re-apply (M7-05 says yes, with no rate limit), what happens after 3 rejections (auto-block? escalation?), how long "pending review" can sit before SLA breach, what notifications fire at each stage. From "click apply" to "first revenue dollar" is realistically 8–12 discrete steps; the PRD names ~3.

6. **Refund / reversal semantics are too casual** (§9.4). "Write a negative `revenue_log`" is one bullet covering: customer-initiated refund within window, customer-initiated refund outside window, partner-initiated refund, platform fraud reversal, chargeback, disputed-billing correction. Each has different actors, different effects on partner wallet, different timing windows, different receipts. Particularly bad: if the partner has already been **settled and paid** for a piece of revenue and the underlying transaction is then refunded, the schema offers no "partner debt" or "future-period claw-back" concept. This will produce real corruption.

7. **CDC is a hidden single point of failure** (§9.2, §9.3). Settlement correctness depends entirely on canal/maxwell behaving. §11's CDC-loss row has a "缓解" but no concrete latency budget, no DR plan if canal is wedged for 6 hours, no replay tool spec, no documented reconciliation cadence. Either add an operational runbook section, or define a fallback batch reconciliation as the **primary** path with CDC as the live overlay.

8. **`§7.7 M7-07` says "30 天后自动清原图" but `§8.9 kyc_application` retains `*Url` columns indefinitely.** Schema does not encode the policy. After purge the URL is dead but the row still references it. Either (a) NULL the URL columns at purge time and rely on `PiiPurgedAt`, or (b) move artifact references to a separate `kyc_artifact` table with TTL. Otherwise a future bug or migration will silently re-expose dead links.

9. **No "customer hits zero balance" flow.** Partner has a wallet, customer has Fy-api quota. When customer quota goes to zero, does Fy-api just 402? Does TraceNex Partner notify the partner? Does the customer see "contact your partner to recharge"? Real-world this is the highest-volume support ticket and there is zero design for it.

## MEDIUM issues (nice to have)

1. **Missing §13 questions.** Add at minimum: Q11 multi-currency / FX policy on payouts (Q5 only grazes it); Q12 platform→partner contract (click-through? per-partner? where stored?); Q13 right-to-deletion (PIPL/GDPR) — when a customer requests deletion, does this also nuke `revenue_log`, which would corrupt settlement?; Q14 attribution window (if a partner-invited customer pays direct 6 months later, does the partner still get commission?); Q15 minimum payout threshold (do we pay ¥0.01 or accumulate to ¥100?); Q16 NDA on partner-visible customer data (customer list = competitor mailing list); Q17 max markup ceiling (anti-gouging policy).

2. **§13 not ranked by blocking severity.** Q3 (individual-partner tax) and Q10 (where the platform takes its fee) are **development-blocking** because they directly affect the calculation in §9. Q4 (content moderation) is legal-blocking but not engineering-blocking. Q6 (refund window), Q8 (partner abandonment), and Q9 (cross-partner poaching) all change schema. Tag each Q with BLOCK / WARN / INFO and a target due date.

3. **Trial-quota abuse vector** (M1-08). $0.50 free quota × N self-registered fake customers under one partner = a partner inflates their numbers for free. No anti-abuse described — needs at least IP/device fingerprint + delayed quota grant.

4. **Re-seller / multi-tier story is hand-waved.** §1.3 says "二级 only" but `partner.Tier int8 (v1.1+)` is a dangling field in §8.1. Distribution programs always grow tiers eventually. v1.0 should at minimum document the upgrade plan: do we change `customer.partner_id` to a parent_id graph, or add `partner.parent_partner_id`? An hour of design now saves a migration nightmare.

5. **Pricing transparency is mis-prioritized** (M3-12). "Partner can see wholesale price" is P1, but the moment markup ships in Phase 2, partners cannot price intelligently without it. M3-12 should be P0 alongside M3-08.

6. **Audit log lacks a query story** (§8.13). `audit_log.DiffJson` accumulates millions of rows. The schema as written has no index strategy and no UI for ops to search by `actor_id`, `target_type+target_id`, or `action`. Spec the read path or it'll be useless when most needed.

7. **Dashboard data freshness not surfaced** (M3-01). With CDC at 1–3s lag, the dashboard should display "as of HH:MM:SS" so support can answer "why don't I see my new charge yet?" Not asserted anywhere.

8. **No demo/sandbox mode for partners.** Sales will ask "can I show this to a prospect?" — currently the only way is to fully provision them. A sandbox-data partner login is cheap and valuable.

9. **Notification preferences absent.** PRD assumes email everywhere (M5-06, M4-02). What about SMS for high-value events (settlement >¥100k)? In-app? Webhooks for partner CRM integration? Document or scope out.

## Specific section feedback

### §3 User roles

Three primary roles is the right top-level split, but in practice there are at least 6 effective roles: super_admin, operations, finance, support, partner-owner, customer. Plus future: partner-staff, customer-staff, system-cron-actor (used by audit logs). v1.0 must include a role × verb permission matrix (~20 verbs: create_partner, approve_kyc, adjust_wallet, view_markup, refund, mark_settlement_paid, etc.). Drop the "个人直销用户" bullet — it adds nothing and confuses the scope conversation. Add explicit "system" actor for the audit-log section.

### §4 Scenarios

Currently 7 happy-path scenarios. Add at least:

- **Scenario H** — Customer wants to switch partners (orphan flow, group remap, balance handling)
- **Scenario I** — Partner suspension or termination (wallet, customers, pending settlement)
- **Scenario J** — Customer offboarding / refund (claw back already-recognized revenue)
- **Scenario K** — Billing dispute end-to-end (customer files → partner sees → platform arbitrates → revenue reverses if upheld)
- **Scenario L** — KYC rejection and re-submission, with rate limits and after-3-rejection escalation
- **Scenario M** — Customer self-registers direct without invite code (default-partner attribution? lost to direct sales?)
- **Scenario N** — Partner invites a customer who is *already* a TraceNex direct user (merge? reject? reattribute?)
- **Scenario O** — Partner onboarded mid-period (when does first settlement period start? pro-rated?)

§4.5 (calc chain) should explicitly cross-link to §9 — easy to miss.

### §7 Functional requirements

- **§7.1 M1-04** "复用 `/api/user/register`" — partner-invited customers need a different flow that consumes an invitation code atomically. Different endpoint or parameter? Spec it.
- **§7.2 M2-11** role switcher — UX needs clarity. Single login, dropdown? Or separate logins? What if a user is a partner *and* a customer of another partner — allowed?
- **§7.3 M3-07** "移除/禁用客户" P1 — should be P0. Partners will demand it on day one (abusive customer, non-payer).
- **§7.3 M3-08** must be split: per-partner-default-markup, per-customer-override, per-model-override, time-bound promo. Today it's one bullet (CRITICAL #3).
- **§7.5 M5-09** dispute is P1 — disagree, P0 the moment money flows. Even a primitive "partner flags an item with a comment thread."
- **§7.6 M6-07** idempotency P0 is correct, but the keying strategy isn't specified. Recommend client-supplied `idempotency_key` header, 24h TTL.
- **§7.7 M7-02** "支付宝芝麻认证" — be explicit we receive only success/fail, never the raw ID number. Currently §8.9 has `LegalPersonIdNo` encrypted, but document the boundary clearly.
- **§7.8 M8-03** "财务审核" — needs a rejection-reason taxonomy (≥5 standard reasons).
- **Missing module: customer-support / ticketing**. M2-07 references "工单流" and M5-09 mentions disputes, but no schema, no workflow exists. Tickets are P0 from day one.
- **Missing module: notifications**. Email is referenced ad hoc; no central event catalog, no partner-configurable channel preferences.

### §12 Milestones

- Phase 1 success criteria are feature-existence ("can create account") not business outcome. Replace with: "5 seed partners onboarded; ≥3 have ≥2 customers each; ≥¥10k cumulative platform GMV via partner customers; 0 wallet-reconciliation drift events."
- Phase 1 must include either a primitive markup or be honestly relabeled (CRITICAL #2).
- Phase 2 must split (HIGH #1).
- Phase 3 says "2–4 weeks" — what's the variance source? If invoice integration with 税控 slips, what's deferred?
- "Week 0" is referenced but not enumerated as a deliverable. Add: PRD v1.0, repo init, technical spike for CDC, WeChat/Alipay qualification submission.

### §13 Open questions

The 10 questions are mostly the right ones. Tag for blocking severity:

- **BLOCK (must answer before code)**: Q3 (individual-partner tax), Q10 (platform fee mechanism), Q6 (refund window).
- **WARN (must answer before Phase 2 launch)**: Q1 (default share), Q5 (海外 payment), Q8 (partner abandonment), Q9 (cross-partner poaching).
- **INFO (must answer before Phase 3 / public launch)**: Q2 (target industry), Q4 (content moderation), Q7 (税控 system).

Add Q11–Q17 per MEDIUM #1.

## Missing entirely

- **Success metrics / KPIs** (CRITICAL #4)
- **Churn / wind-down lifecycle** (CRITICAL #1)
- **Per-model markup decision** (CRITICAL #3)
- **Settlement cycle change handling** (CRITICAL #5)
- **Partner sub-accounts / partner staff** (HIGH #3)
- **Customer experience / branding / white-label** (HIGH #4)
- **Direct-traffic attribution / anti-bypass policy** (HIGH #4)
- **Customer-zero-balance flow** (HIGH #9)
- **Permission matrix** (HIGH #2)
- **Lifecycle state machines** for partner / customer / settlement / KYC / dispute
- **Customer-support / ticketing module**
- **Central notification / messaging system**
- **Anti-abuse policy** (trial quota, fake customers, content abuse)
- **Partner contract storage** (signed PDF location, version, e-signature?)
- **Partner sandbox / demo environment**
- **Pricing temporality** — when markup changes, do historical `revenue_log` entries reprice or stay at old rate?
- **Capacity / inventory model** — can a partner oversell? Customer prepaid quota vs. partner empty wallet?
- **Glossary** — "席位 / seat / License", "group", "quota", "wholesale", "retail", "markup", "revenue share", "payout" are used inconsistently. A 1-page glossary at the front of v1.0 would cut half the future review confusion.
- **i18n scope** — §10 says zh+en but most KYC is China-only (营业执照, 支付宝芝麻, 法人身份证). What's the SG/EN flow?
- **Onboarding playbook for ops** — when a new partner is approved, what does ops do? Welcome email? First-time content? Demo call?

## Suggested additions for v0.2

- **§1.4 Success Metrics & KPIs** — per-phase numeric exit criteria
- **§3.x Permission Matrix** — staff sub-roles × actions, partner × actions, customer × actions
- **§4.8–§4.16** — Scenarios H–O (churn, suspension, switch, dispute, refund, deletion, merge, mid-period onboard)
- **§7.9 Customer Experience & Branding** — direct-traffic attribution, white-label hooks
- **§7.10 Support / Ticketing** module
- **§7.11 Notifications** module (event catalog + channel preferences)
- **§9.5 Pricing Temporality** — `partner_pricing` versioning vs. `revenue_log` historical reads
- **§10.x Operational Runbook hooks** — CDC failure, settlement Cron failure, KMS rotation
- **§14 Lifecycle State Machines** — partner, customer, settlement, KYC, dispute
- **§15 Glossary**
- **Restructure §12** per HIGH #1 (Phase 2A/2B split) and CRITICAL #2 (Phase 1 honest re-scope)
- **Expand §13** to Q1–Q17, tagged BLOCK/WARN/INFO with due dates

## Recommendation

**Verdict: NEEDS_REVISION.** The technical foundation is unusually strong for v0.1 — §6, §8, §9 are nearly architect-review-ready. As a *product* spec, however, this draft is missing the unhappy-path lifecycle, customer-facing experience, success metrics, a credible MVP, and a defensible pricing model. **Do not pass this to architecture/security/compliance for deep review yet** — they will burn cycles on flows that get re-cut in v0.2.

Concrete next steps for Nathan, in order:

1. Decide markup semantics (CRITICAL #3): single scalar forever, or `partner_pricing_rule` with model + time dimensions? Document in §1.3 (out of scope) or expand §8.5 / §9.
2. Spend half a day writing Scenarios H–O (CRITICAL #1). The act of writing them will surface 3–5 missing tables.
3. Repackage Phase 1 (CRITICAL #2): either add primitive markup + manual settlement so seed partners can transact, or honestly relabel as "internal demo" and shrink the timeline.
4. Add §1.4 Success Metrics (CRITICAL #4) — these will guide all future scoping arguments and prevent feature-checklist victory declarations.
5. Tag §13 questions BLOCK/WARN/INFO and circulate to legal/finance/biz with due dates; resolve Q3 + Q6 + Q10 before any §9 code is written.
6. Hold v0.2 review (PM + architect + security + compliance) once 1–4 land. Settlement-timing edge cases (CRITICAL #5) and the HIGH list can be folded into that round.

Estimated effort to reach v0.2: 2–3 focused days. Worth it before Phase 1 dev begins; cheaper than discovering these gaps in week 6 of build.
