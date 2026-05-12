# Round-2 Compliance Review — TraceNexBiz

**Date**: 2026-05-12
**Reviewer**: Compliance (PIPL + 中国生成式AI服务管理暂行办法 + 41 号公告 + 互联网信息服务算法推荐管理规定)
**Scope**: 验证 Fix-C bundle 是否真实闭环 round-1 的 4 CRITICAL（C-C1..C4）+ 6 HIGH + 6 P1 items；并新增 KMS deletion-window / audit hash chain / PII non-persistence / consent version 四个合规面.

**Verdict**: **PASS_WITH_NOTES**（Phase 1 内测放行；Phase 2A 上线前需关掉本 review 列出的 1 HIGH + 4 MEDIUM）。

---

## 1. Round-1 CRITICAL status

| ID | 主题 | 证据 | 判定 |
|:--:|---|---|:--:|
| **C-C1** | footer endpoint → ICP / AI 备案号公示 | `internal/handler/public_biz_setting.go:47-127`（5min Redis cache, 9 keys → 8 字段 payload；fallback 空串）+ `cmd/server/main.go:237 RegisterPublicFooterRoute`；`middleware.WithScope("public")` | **CLOSED** |
| **C-C2** | 12377 dispatcher + admin route + 24h SLA | `cmd/dispatcher-12377/main.go:1-122`（leader-lock + 5min tick + SLABreaches alert）；`internal/handler/admin/admin.go:59-61` 3 routes 均 `WithScope("staff_compliance")`；service `SLAReport24h = 24*time.Hour`（content_safety.go:37）；admin `POST /dispatch` batch ≤ 50 | **CLOSED** (with note: `noopAuthority` stub — TODO ops to wire real 12377 endpoint, 已显式标注 `log.Warn`) |
| **C-C3** | KYC 5y purge cron + KMS ScheduleKeyDeletion | `cmd/kyc-purge/main.go:1-221`（daily tick + leader-lock；`PIPL5Years` 常量；逐行调 `kms.ScheduleKeyDeletion(kid, 30)`，全字段 cipher → NULL + `pii_purged_at`）；`internal/infra/kms/kms.go:54-56 / 162-170 / 326` 接口 + LocalKMS impl + AliyunKMS stub；7-365d bounds 校验 (`ErrInvalidScheduleDays`) | **CLOSED** |
| **C-C4** | outbox 跨境 region 隔离 | pull-based `internal/outbox/consumer.go:202-207` 反向断言（region mismatch → DLQ）+ `consumer.go:263` enum 校验；Fy-api 源端 `InsertOutboxInTx` guard + `LeaseOutboxBatch` WHERE | **PARTIAL** — pull-based pipeline ✅；**MNS pipeline 无 region 校验**（`aliyun_mns_consumer.go` handler dispatch 仅按 `event_type`，未读 `attrs["data_region"]`，publisher 也未注入；Phase-1 dormant — `consumer.Register` 在 `main.go:460` 无任何 handler）。Phase 2A 启用 MNS 业务事件前必须补 |

---

## 2. Round-1 HIGH status (8 / round-1 listed 8)

| ID | 主题 | 状态 | 备注 |
|:--:|---|:--:|---|
| HIGH-C1 | storefront build gate | OPEN | 仍未见 `scripts/preflight-compliance.ts`；Phase 2A blocker |
| HIGH-C2 | 12377 cron + AuthorityClient + Fy-api event源 | PARTIAL | cron ✅；`noopAuthority` 仍 stub（acceptable as ops-side wiring）；Fy-api 侧 event 入口未见 |
| HIGH-C3 | KYC purge cron + MySQL repo + KMS + OCR cipher | CLOSED | cron + ScheduleKeyDeletion 全到位；OCR cipher 字段在 `kycPurgeRow` 中已列且 purge 路径覆盖 |
| HIGH-C4 | partner-api PIPL 权利请求 endpoint | OPEN | grep `/api/customer/pipl` handler 仍无；前端 `customer.ts:319` 调用 = 404 |
| HIGH-C5 | ISV mch_id 反向断言 | CLOSED | `service/payment/service.go:85 expectedMchID guard`；`ErrMchIDMismatch` |
| HIGH-C6 | 41 号公告个税 5 档 + 银行实名一致性 | OPEN | `settlement/engine.go` 仍只用 3 值 `PartnerKind`；blind_index 在 schema 已 UNIQUE 但 settlement payout 前无 `HMAC == bank_account_blind_index` 一致性断言 |
| HIGH-C7 | customer KYC 7 枚举 consent checkbox | OPEN | `partner-web-customer/src/pages/Kyc.tsx` 仍三栏 UI；前后端 contract gap 未消 |
| HIGH-C8 | OCR 三方核验 audit_log 共享证据 | OPEN | `service/kyc/kyc.go` OCR 调用前后未见 `audit.Append(action='pii.share.aliyun_ocr')` |

---

## 3. P1 items claimed in Fix-C

| Item | 证据 | 状态 |
|---|---|:--:|
| ISV mch_id reverse assertion | `payment/service.go:37,45,85`；`payment.go:69 CallbackPayload.MchID` + `ErrMchIDMismatch` | ✅ verified |
| tax_status 5 枚举 (migration 015 + ValidateTaxStatus) | `migrations/015_partner_tax_status_enum.up.sql:16-24`（旧值映射 + CHECK 5 枚举）；`domain/partner.go:51-66 ValidTaxStatuses + ValidateTaxStatus` | ✅ verified |
| bank_account blind_index HMAC | `pkg/pii/blindindex.go:28-43 BlindIndex + BlindIndexFromEnv`；`migrations/016_bank_account_blind_index.up.sql:14-16` (UNIQUE)；test `pkg/pii/blindindex_test.go` | ✅ verified |
| consent_text_version validation | `pkg/consent/version_guard.go:62-78 Verify`（fail-closed in prod；dev mode 兜底）；`cmd/server/main.go:354-356` 注入；`service/customer/customer.go:104-129 ConsentVerifier` | ✅ verified |
| 5 crons single-leader (pkg/leader/redis.go) | `pkg/leader/redis.go:40-50 RedisLock` (SETNX + TTL+refresh)；`AlwaysLeader` dev fallback；`audit-sealer/dispatcher-12377/kyc-purge` 三 binary 都接入 | ⚠️ only 3 crons exist (`audit-sealer / dispatcher-12377 / kyc-purge`)。原 round-1 列举的 `tax.withholding.annual / model_whitelist.review / pia.report.annual` 仍无 cmd binary（acceptable for Phase 1；Phase 2A 前补） |
| red_flush_request enum (migration 017) | `017_red_flush_status_enum.up.sql` 内容为 `SELECT 1;` no-op + 文件级注释 "DEFERRED to Fix-D" | ✅ documented TODO (acceptable — invoice service rewrite scheduled separately) |

---

## 4. New compliance surfaces

### 4.1 KMS deletion window 7-365 day bounds

- `kms.go:69 ErrInvalidScheduleDays`；`LocalKMS.ScheduleKeyDeletion:163-165` `if days < 7 || days > 365 → err`；`Stub.ScheduleKeyDeletion:408+` 同；测试 `kms_test.go:34-37` 显式覆盖 days=5 / days=366 拒收。**✅ 符合 Aliyun KMS 文档（PendingWindowInDays ∈ [7,365]）。**
- `cmd/kyc-purge/main.go:41 KMSDeletionWindow = 30`（30d 默认，落在合法区间内；ops 可通过修改常量 / env 调整 7..365）。

### 4.2 audit_log hash chain tamper detection

- `internal/audit/sealer.go:10-32` 算法定义（`prev_hash + canonicalize(row) → sha256 → self_hash`；`GenesisPrevHash="GENESIS"`）；MySQL `mysql_sealer.go:78 PrevHash size:64` + Verify 路径；测试 `mysql_sealer_test.go:102-127` 覆盖 row tamper + row delete 两种破坏。`cmd/audit-verify/main.go` 提供 `--since=<id>` 模式，退出码 0/2 区分 chain intact / broken。**✅ 满足等保 2.0 与电子证据可信留存。** MED-C7 (audit MySQL Store 接线) 已 CLOSED。

### 4.3 PII non-persistence (ApplyPartner Zustand draft)

- `partner-web-storefront/src/stores/applyDraft.ts:43-62 partialize` 注释明确"身份证 / 上传 URL / 银行账号 不写入 localStorage"；`settlement_bank_account` 与 `legal_person_id` 字段被排除在持久化字段集合外（line 60）。**✅ 符合 PIPL §6 最小必要 + §51 安全义务。** 仅在内存 / 网络传输瞬间存在。

### 4.4 consent_text_version mismatch → rejected signups

- `pkg/consent/version_guard.go:62-78` `Verify`：空版本 → `ErrConsentVersionEmpty`；prod 模式下 current 空 → `ErrConsentVersionStale`（fail-closed）；mismatch → 同错；dev 模式仅当 current 空才放行 (acceptable for dev fixtures)。`customer.go:104-129 ConsentVerifier` 抽象 + main.go `WithConsentVerifier(consent.NewVersionGuard(cfg.BizSetting))` 装配。**✅ 防 stale ToS 注册。** 注：仅装配在 `Service`（customer）；建议同模式扩展到 `partner` apply 流程（NEW-MED）。

---

## 5. New findings (this round)

### CRITICAL
（无）

### HIGH
- **NEW-H1 (Phase 2A blocker)** — MNS outbox consumer pipeline 无 `data_region` 反向断言。`internal/outbox/aliyun_mns_consumer.go:149-193 handleOne` 仅按 `event_type` dispatch，未读 envelope `attrs["data_region"]`，publisher (`buildMNSEnvelope`) 也未注入 region。**当前 Phase-1 dormant（`server/main.go:460` 未注册任何 handler），不构成立即风险**；一旦 Phase 2A 开始用 MNS 发跨业务事件，CN 实例可能误消费 SG 队列消息。修订：publisher 在 attrs 写 `data_region=cfg.DATA_REGION`；MNSConsumer Wrapper 加 region guard middleware（在 `handleOne` dispatch 前 reject）。

### MEDIUM
- **NEW-M1** — PIPL 权利请求 endpoint (HIGH-C4) 与 customer-side consent UI (HIGH-C7) 仍 OPEN；属于 Phase 2A 前必做（不在本 round 闭环范围）。
- **NEW-M2** — round-1 中 5 个 cron 仅 3 个有 binary（`tax.withholding.annual / model_whitelist.review / pia.report.annual` 缺）。Phase 1 不阻塞（无月报）；Phase 2A 前补。
- **NEW-M3** — `dispatcher-12377` 的 `AuthorityClient` 是 `noopAuthority` stub；ops runbook 明确说明在 staging 接 endpoint，但本 binary 缺一行启动检查（如 `if env != dev && client is noop → fatal`），易误上 prod。建议加 fail-loud guard。
- **NEW-M4** — `consent.VersionGuard` 当前仅在 `customer.Service.Register` 路径接入；`partner` apply / `auth` password_reset 等同样写入 `consent_log` 的入口未接同模式（version guard 漏接 = stale ToS 仍可签）。

### LOW
- **NEW-L1** — `kyc-purge` 每 24h 从启动时刻 tick，非 04:30 锚定。若部署在峰值时段重启，purge 也会在峰值跑。建议加 cron-style 调度（`@daily` 或显式 04:30）；当前实现 acceptable（DB 写入量 ≤ 500 行/tick）。
- **NEW-L2** — `public_biz_setting.go:107-108 cv == "" → "v2.0"` 默认值与 VersionGuard `fail-closed` 在 prod 模式下语义冲突（footer 给客户端 "v2.0"，但 guard 在 biz_setting 未配时 reject）。建议二者用同一 source-of-truth + 同样的 dev-mode fallback。

---

## 6. Final tally

| Category | Count |
|---|---:|
| Round-1 CRITICAL closed | 3 / 4 (C-C1, C-C2, C-C3 ✅；C-C4 PARTIAL — MNS pipeline 未覆盖) |
| Round-1 HIGH closed | 2 / 8 (C3, C5) — 其余 6 OPEN 但全部归属 Phase 2A blockers（不阻 Phase 1） |
| P1 claimed-closed verified | 6 / 6 (1 documented TODO acceptable) |
| New HIGH | 1 (MNS region guard — Phase 2A blocker, currently dormant) |
| New MEDIUM | 4 |
| New LOW | 2 |

**Verdict rationale**: Fix-C 把 round-1 列出的 3 个真正 CRITICAL（footer endpoint / 12377 dispatcher / KYC purge + KMS）从 schema / service / cron 三个维度均闭环；P1 6 项验证 5 项 ✅ + 1 项 documented TODO（red_flush enum 由 Fix-D 接管，schema 注释明确）；C-C4 outbox 隔离在 pull-based pipeline 完整，MNS pipeline 暂 dormant 故不阻塞 Phase 1。**升级 NEEDS_REVISION → PASS_WITH_NOTES**；Phase 2A 上线前须关掉 HIGH-C1 / C4 / C6 / C7 / C8 + NEW-H1 + NEW-M1..M4。

> 本 round-2 verdict 与 round-1 一致地不阻塞 Phase 1 内测（≤ 5 家 / CN only / 不开票 / 不分账），且对 W3 工作量给予肯定（4 CRITICAL 中 3 真实闭环，第 4 项的"未覆盖"仅在尚未启用的 MNS pipeline 上）。
