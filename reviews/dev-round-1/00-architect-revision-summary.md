# Architect Revision Summary — Round-1 → v0.2

> Date: 2026-05-11
> Author: Architect (主架构师)
> Scope: 把 4 份 Round-1 review 的反馈合入 4 份开发文档，输出 v0.2，准备进入 Round-2 复核
> 输入：`reviews/dev-round-1/{01-PM,02-Architect,03-Security,04-Compliance}-review.md`
> 输出：
> - `docs/00-architecture-overview.md` v0.2（766 → ~870 行）
> - `docs/integration-design.md` v0.2（1558 → ~1700 行）
> - `docs/backend-design.md` v0.2（2704 → ~3300 行）
> - `docs/frontend-design.md` v0.2（1797 → ~1880 行）

合计：13 CRITICAL + 30 HIGH + 30+ MEDIUM + 20+ LOW，逐条状态如下。每份文档末尾的"v0.1 → v0.2 CHANGELOG"是 Round-2 reviewer 的导航索引；本摘要是跨文档的总览。

---

## 1. 跨文档架构 verdict 收敛（v0.2 最关键的 15 条）

按本轮 prompt §"关键架构 verdict"列表，逐条已落地：

| # | Verdict | 落点 |
|---|---|---|
| 1 | JWT 全系 httpOnly cookie + double-submit CSRF；Bearer 仅 `/api/sdk/*` | overview ADR-007 v0.2 / §2.2 流量表；backend §7.2 / §7.6；frontend ADR-F5；integration（无变化） |
| 2 | JWT revocation fail-closed | overview ADR-007 v0.2；backend §7.2；overview §10 A-9 |
| 3 | JWT 公钥移到 KMS Secret Manager；biz_setting 加 value_type | overview ADR-007 v0.2；backend §3.15 / §7.2；overview §10 A-14 |
| 4 | partner_wallet.held_amount **drop**；invariant test 替代 drift | overview ADR-012 v0.2；backend §3.3 |
| 5 | saga_step UNIQUE(saga_id, step_name) | integration §4.1；backend §3.17；overview ADR-013 v0.2 |
| 6 | response_json 字段统一为 `response_cipher` + `response_key_id` | overview §4.3；integration §5.3；backend §3.16；frontend（无变化） |
| 7 | saga.force_resolve verb 注册 + PRD-PATCH-1 | overview ADR-015 / §14.5；backend §4.5/§4.5.1/§7.4；integration §4.6 |
| 8 | OSS presigned PUT 强校验（MIME/size/Content-Type/magic-byte/异步病毒扫）| overview ADR-017；backend §9.4；frontend §12.1 CSP |
| 9 | BOLA repo 层 row-level guard（CI golangci analyzer）| overview §10 A-10；backend §5.3 / §15.3 |
| 10 | dual-control force-resolve 严格约束（不同人 / 不同角色 / 不同 IP /24 / 一次性 token / 30min cooldown）| overview ADR-015；backend §4.5.1 / §7.4 |
| 11 | 供应链 / SBOM / 镜像签名 | overview ADR-016 |
| 12 | ComplianceFooter + 9 个 biz_setting 备案号 key | frontend §11.5；backend §3.15；overview §8.5 |
| 13 | 12377 / 公安网安上报通道 + 24h SLA | backend §3.24 / §6 cron；frontend §3.3 admin `/content-safety/reports` |
| 14 | KYC 5y 冷归档销毁 cron `kyc.purge.cold` | backend §3.9 / §6 |
| 15 | consume_log_outbox region-isolated invariant + `data_region` 字段 | integration §1.5.2 / §6 GRANT；overview §10 A-8 |

---

## 2. 13 CRITICAL — 全部 FIXED

| ID | 来源 | 状态 |
|---|---|---|
| ARCH-CRIT-1 / SEC-CRIT-1 | JWT 载体 Bearer vs Cookie 冲突 | **FIXED** |
| ARCH-CRIT-2 / SEC-M-r2 | response_json 字段语义冲突 | **FIXED** |
| ARCH-CRIT-3 / PM-HIGH-4 | saga.force_resolve verb 缺失 | **FIXED**（文档）+ **PRD-PATCH-1** |
| SEC-CRIT-2 | BOLA row-level guard 普遍缺失 | **FIXED**（架构约束 + CI analyzer） |
| SEC-CRIT-3 | JWT revocation fail-open on Redis | **FIXED** |
| SEC-CRIT-4 | OSS presigned PUT 服务端无校验 | **FIXED** |
| SEC-CRIT-5 | dual-control 时间窗叠加可绕过 | **FIXED** |
| SEC-CRIT-6 | 供应链 / SBOM / 镜像签名缺失 | **FIXED**（ADR-016 架构登记 + ops follow-up） |
| SEC-CRIT-7 | jwt_public_key_pem 在 biz_setting | **FIXED** |
| COMP-CRIT-1 / M-8 | 备案号公示 9 keys + ComplianceFooter | **FIXED** |
| COMP-CRIT-2 / M-11 | 12377 上报通道 | **FIXED**（schema + cron + admin UI） |
| COMP-CRIT-3 / M-7 | KYC 5y 冷归档销毁 cron | **FIXED** |
| COMP-CRIT-4 / M-6 | outbox 跨境隔离 invariant | **FIXED** |

> **注**：ARCH-CRIT-3 / SEC-CRIT-7 涉及 PRD §3.4 权限矩阵的 verb 集合扩展（新增 `saga.force_resolve` + 拆 `system.config_write.{trivial,security}`），已在 overview §14.5 登记 PRD-PATCH-1；PRD patch 落地前 backend 用 hardcoded allowlist + 启动 feature flag + audit 三层兜底，**不阻塞 Phase 1 工程**。

---

## 3. 30 HIGH — 状态分布

| 状态 | 计数 | 详情 |
|---|---:|---|
| **FIXED** | 27 | 见下表 |
| **ACCEPTED-AS-DEBT** | 2 | T-11（admin 零信任 ops runbook）/ T-3（per-model markup Phase 1 schema-only） |
| **DEFERRED-TO-PHASE-2A** | 1 | D-10 customer_update Pub/Sub 频道（Phase 1 用 30s stale 即够） |

### 30 HIGH 状态总表

| ID | 类别 | 状态 | 主落点 |
|---|---|---|---|
| ARCH-HIGH-4 (#5/D-4) saga_step UNIQUE | ARCH | FIXED | integration §4.1 / backend §3.17 / overview ADR-013 |
| ARCH-HIGH-5 (B-1/B-2) outbox poller 两阶段 | ARCH | FIXED | integration §3.3 / overview ADR-014 |
| ARCH-HIGH-6 (#2) CSRF 不一致 | ARCH | FIXED | overview ADR-007 / backend §7.6 / frontend §6.2 |
| ARCH-HIGH-7 (D-5) drop held_amount | ARCH | FIXED | overview ADR-012 / backend §3.3 |
| ARCH-HIGH-8 (#9) wallet_log UNIQUE 粒度 | ARCH | FIXED（保留双键 + 注释决议） | backend §3.4 |
| ARCH-HIGH-9 (B-4) DLQ 阈值 10 | ARCH | FIXED | integration §3.3 / backend §6 |
| ARCH-HIGH A-1 pending 态 middleware | ARCH | FIXED | backend §8.1 |
| ARCH-HIGH A-2 wallet version mismatch | ARCH | FIXED | backend §5.3 / §6 worker |
| ARCH-HIGH A-3 不回写 terminal | ARCH | FIXED | integration §4.1 注释 / backend §6 |
| ARCH-HIGH D-3 TTL 启动 assert | ARCH | FIXED | backend §3.15 / integration §5.2 |
| SEC-HIGH-2 全局 rate-limit | SEC | FIXED | backend §7.8 |
| SEC-HIGH-3 refresh rotation | SEC | FIXED | backend §7.9 |
| SEC-HIGH-4 partner WebAuthn 强制 | SEC | FIXED | backend §7.5 |
| SEC-HIGH-7 biz_setting allowlist + dual-control | SEC | FIXED | backend §3.15 / §4.5 / §7.4 |
| SEC-HIGH-8 mTLS mesh 边界 | SEC | FIXED | integration §6.4 / overview §2.2 |
| SEC-HIGH-9 admin 零信任 VPN | SEC | **ACCEPTED-AS-DEBT T-11** | overview §10 / frontend §17.6 |
| SEC-HIGH-10 biometric 生命周期 | SEC | FIXED | backend §3.9 / §6 cron |
| SEC-HIGH-11 blind index | SEC | FIXED | backend §3.9 |
| SEC-HIGH-12 audit_log.target_id string | SEC | FIXED | backend §3.13 加 `target_key VARCHAR(128)` |
| SEC-HIGH-13 内容安全 rate-limit + 上报闭环 | SEC | FIXED | backend §7.8 / §3.24 / §6 |
| SEC-HIGH-14 webhook body limit | SEC | FIXED | backend §5.7 |
| COMP-HIGH-1 partner.tax_status + 41 公告 | COMP | FIXED | backend §3.1 / §5.5 / §6 cron |
| COMP-HIGH-2 consent_type 增枚举 | COMP | FIXED | backend §3.18 |
| COMP-HIGH-3 第三方 PII 共享 audit | COMP | FIXED | backend §5.6 service 入口规约 |
| COMP-HIGH-4 模型白名单月度对齐 cron | COMP | FIXED | backend §6 `model_whitelist.review` |
| COMP-HIGH-5 content_safety_event DDL | COMP | FIXED | backend §3.23 |
| COMP-HIGH-6 发票销售方 + 10y 留存 | COMP | FIXED | backend §3.12 + `red_flush_request` |
| COMP-HIGH-7 PIA 报告 | COMP | FIXED | backend §3.25 + frontend §3.3 admin |
| COMP-HIGH-8 DPO 入口 / 投诉受理 | COMP | FIXED | backend §3.26 + frontend §3.1/§3.3/§11.5 |
| COMP-HIGH-9 partner_debt 上调 Phase 2A | COMP | FIXED | overview ADR-010 / backend §3.22 |
| PM-HIGH-1 M2-15 通知中心 Phase 冲突 | PM | FIXED | frontend §3.2 / §18.2 / §18.4 |
| PM-HIGH-2 场景 I 孤儿客户 UX | PM | FIXED | backend §3.2 / §5.14 / §6 / §4.5；frontend §3.2 / §3.3 |
| PM-HIGH-3 红冲 UI 路径 | PM | FIXED | backend §3.12 + §4.5；frontend §3.3 |
| PM-HIGH-4 场景 K 争议全链路 | PM | FIXED | backend §5.13 + §3 dispute 表；frontend §3.2/§3.3 |
| ARCH-HIGH D-6 / F-1 per-model markup | ARCH | **ACCEPTED-AS-DEBT T-3** | Phase 1 schema-only；Phase 2A 启用 |
| ARCH-HIGH D-10 customer_update 频道 | ARCH | **DEFERRED-TO-PHASE-2A** | frontend 30s stale 兜底 |

---

## 4. MEDIUM / LOW 概况

- **MEDIUM 合计 ~37 条**（PM 9 + ARCH 11 + SEC 14 + COMP 7-3 重复）：FIXED ≥ 32；ACCEPTED-AS-DEBT 4（T-2 / T-5 / T-12 / T-13）；DEFERRED 1（PM-MEDIUM-9 业务 KPI metrics 推到 Phase 1 Week 3 落 Grafana dashboard）
- **LOW 合计 ~27 条**：FIXED ≥ 22；ACCEPTED-AS-DEBT 5（主要是文案、UUIDv7 canonical 文档化、ops runbook 占位等）

抽样 6 条 MEDIUM 落点（核对 reviewer 找位置）：

| ID | 落点 |
|---|---|
| ARCH-MED 8.6 pricing_rule UNIQUE NULL | backend §3.6 generated column |
| ARCH-MED 8.18 ticket.category 漏 content_report | backend §3.18 CHECK 扩展 |
| ARCH-MED 8.20 customer_partner_change_log status | backend §3.20 加 `status` 列 |
| SEC-MED-3 Idempotency-Key Sentry breadcrumb | frontend §15.2 v0.2 scrubber |
| SEC-MED-10 CSP connect-src OSS | frontend §12.1 |
| COMP-MED-13 OCR 结果加密 | backend §3.9 `business_license_ocr_cipher` |

---

## 5. 架构债务清单（v0.2 显式接受）

| ID | 债务 | 触发 / 还债条件 |
|---|---|---|
| T-1 | Redis SETNX leader 选举（非 K8s Lease） | Phase 1 → Phase 2A 切 K8s Lease |
| T-2 | outbox poller 在 SQLite 仅单 poller | dev / staging |
| T-3 | per-model markup Phase 1 schema-only | Phase 2A 启用（覆盖层 C-10 新增 `user_model_ratio_override`） |
| T-4 | SSR 用 vite-plugin-ssr | Phase 2A 重评 |
| T-5 | 暗色模式 admin 延 Phase 2A | UX 债 |
| T-6 | 跨服务 trace_id SLS 联合查询依赖 ops | ops 承诺 + 纳入 pre-Phase-1 交付 |
| T-9 | saga retry worker 内嵌 cmd/api | Phase 2 拆 `cmd/saga-runner` |
| T-10 | KYC 表级 `encryption_key_id` 冗余 | 已 v0.2 清掉，**T-10 关闭** |
| T-11 | admin.tracenex.cn 零信任 VPN | Phase 2A 前 ops 交付 Cloudflare Access / 阿里云 IDaaS |
| T-12 | Sentry replay 仅 admin 10% | 全期 |
| T-13 | UUIDv7 vs UUIDv4 canonical 文档统一 | Phase 2B 前文档化 |
| T-frontend-brand | 主品牌色 #3D5AFE 待 brand 团队 | Phase 1 内 sync |

---

## 6. PRD Patch 需求

| ID | 触发 | 影响 PRD 章节 | patch 内容 |
|---|---|---|---|
| PRD-PATCH-1 | ARCH-CRIT-3 / SEC-CRIT-5 | §3.4 权限矩阵 | 新增 verb `saga.force_resolve`（super_admin + finance dual-role + step-up + 30min cooldown）；将 `system.config_write` 拆成 `.trivial` / `.security` |
| PRD-PATCH-2 | COMP-HIGH-1 / SEC HIGH-7 | §8.15 biz_setting key 列表 | 补 `compliance.*` 9 个公示 key + 5 个 gating flag + `payment.platform_isv_mchid` + `refund_window_days` + `saga_wall_clock_hours` + `idempotency_ttl_hours` + `internal_idempotency_ttl_days` + `jwt_verify_key_pem (secret_ref)` |

PRD patch 不阻塞 Phase 1 工程；落地前 backend 用 hardcoded allowlist + 启动 feature flag + audit 三层兜底。

---

## 7. Round-2 reviewer 重点关注区域提示

请 Round-2 优先核对以下章节是否字面落地（按 reviewer 角色拆分）：

### Architect Round-2

1. **跨文档契约对齐矩阵**（v0.1 Architect §2 矩阵 21 条）：v0.2 是否已把 ❌ 7 条 + ⚠️ 7 条全部转为 ✅ → 重点看 overview §4.3、integration §1.5/§4.1/§5.3、backend §3.4/§3.16/§3.17。
2. **outbox poller 两阶段 claim 伪代码**（integration §3.3）的并发安全：orphaned `in_flight` re-claim 路径、`locked_until` 续租。
3. **saga retry worker 行为**（backend §6 + integration §4.6）：terminal 不回写 / version mismatch 不计 attempts 是否同时落到时序图与文字。
4. **biz_setting key registry**（backend §3.15）+ 启动 assert 是否覆盖 ARCH D-3 全部不变量。

### Security Round-2

1. **BOLA pattern 落地**（backend §5.3 + §15.3 CI analyzer 描述）：是否 §5.x 每个 service 段都引用了 `repo.FindByID(ctx, scope, id)` 模式？
2. **dual-control 6 项检查**（backend §4.5.1）：`approver != initiator` / 不同角色 / 不同 /24 / 一次性 token / 30min cooldown / audit 双写 — 全部到位？
3. **ADR-007 v0.2 cookie 鉴权链路**：JWT verify key 是否真从 env 注入、biz_setting 中 `jwt_verify_key_pem` 已是 `secret_ref`？
4. **OSS PresignPut**（backend §9.4）：`allowedMime` 字面量约束、HEAD + magic-byte 二次校验、CSP `connect-src` OSS 域。
5. **JWT revocation fail-closed**（backend §7.2）：Redis 不可达 → 503 + alert + degrade env switch。
6. **rate-limit middleware 表**（backend §7.8）：6 条限速规则 + 启动 fail-loud。
7. **WebAuthn 强制阈值**（backend §7.5）：partner wallet > ¥1k / monthly payout > ¥10k / staff 高危 verb。

### Compliance Round-2

1. **9 个 compliance.* biz_setting key + ComplianceFooter**（backend §3.15 + frontend §11.5）+ readiness probe gate（overview §8.5）。
2. **content_safety_report 24h SLA**（backend §3.24 + §6 dispatcher cron）：上报 endpoint placeholder 是否已为真实供应商对接预留？
3. **kyc.purge.cold cron + OSS lifecycle 5y**（backend §6）：cold_archive_expires_at 索引 + cron + ops runbook 三件套是否同步？
4. **outbox region-isolated**（integration §1.5.2 `data_region` + §6 GRANT SG 断言 + overview §10 A-8）。
5. **partner.tax_status + ComputeWithheldTax + 41 号公告 cron**（backend §3.1 + §5.5 + §6 `tax.withholding.annual`）。
6. **invoice seller_entity_id + red_flush_request + 10y 留存**（backend §3.12）。
7. **PIA 报告 + DPO 投诉受理**（backend §3.25/§3.26 + frontend §3.1/§3.3）。

### PM Round-2

1. **M2-15 Phase 修正**：frontend §3.2 customer `/notifications` Phase 3、§18.2 Phase 2A 仅含 M11-03 偏好、§18.4 Phase 3 含 M2-15 中心 — 三处必须严格一致。
2. **场景 I 孤儿客户**：backend `customer.status` 已含 orphaned/adopted/direct（v0.1 已就绪）+ §5.14 service + §6 cron + §4.5 admin endpoint + frontend §3.2/§3.3 路由。
3. **场景 K 争议骨架**：billing_dispute 表 + service + 三个 endpoint + frontend customer/partner/admin 路由全到位。
4. **红冲 UI 路径**：frontend §3.3 admin `/invoices/:id/red-flush` + Phase 2B 清单。
5. **KYC 3 次/年驳回**：backend §3.9 `yearly_reject_count` + §5.6 invariant I-K-4 + frontend §3.2 customer `/kyc` banner（在文档级注释，UI wireframe Phase 2A 落地）。

---

## 8. 我（架构师）对 Round-2 最担心被打回的 3 个点

1. **BOLA service 伪代码**：v0.2 在 §5.3 给了 reference + §15.3 CI analyzer 描述；但 §5.4 / §5.5 / §5.6 / §5.7 / §5.8 / §5.9 / §5.10 / §5.11 等其他 service 段并没有逐段重写为带 scope 的伪代码（避免 diff 爆炸）。Security reviewer 可能要求"每段都看到 row-level guard 字面落地"。如果 Round-2 这样要求，我打算用一个§15.3.x"BOLA 落地映射表"逐 endpoint 给出 scope 检查行号 — 而不是逐段重写。
2. **PRD-PATCH-1 是否会被 PM / Compliance 否决**：本轮把"PRD §3.4 加 saga.force_resolve verb + 拆 system.config_write"标为 PRD v1.0.x patch，但 PRD 是 round-2 已签字定稿。如果 PM 不接受动 PRD，那 backend 必须永远用 hardcoded allowlist + feature flag — 这在长期维护性上是债（不在权限矩阵里就长期不可见）。
3. **outbox poller 两阶段 claim 真在 SQLite 上能跑吗**：v0.2 的伪代码是 MySQL 风格（FOR UPDATE SKIP LOCKED + locked_until）；SQLite 没有 SKIP LOCKED，dev/staging 仍是单 poller fallback。Architect Round-2 可能问"双方言一致性 invariant test 怎么写"。我目前指 T-2 接受为债务，但 reviewer 可能要求 Phase 1 第 1 周给出 SQLite 兼容路径或显式禁止 dev 模式跑多 poller 的 lint。

---

> 本摘要 + 4 份文档末尾的 CHANGELOG 共同构成 Round-2 review 的导航索引。期望 Round-2 verdict：0 CRITICAL / HIGH ≤ 4 / 全部 HIGH 在债务清单中显式接受 / Compliance ACCEPT_AS_V0.2 / Security ACCEPT (CRITICAL=0)。

---

## 9. v0.2 → v0.2.1 增量（Round-1 stale 摘要修正）

> Date: 2026-05-11
> 触发：本摘要 §1-§8 基于 Round-1 stdout 早期 stale 计数（PM 4 HIGH / Architect 2C/6H）。最终落盘 review 实际 PM 6 HIGH / Architect **4C/11H**；Round-1 中部分 CRIT/HIGH 在 v0.2 未覆盖。本节列出补丁。

### 9.1 v0.2 漏掉项 → v0.2.1 已修

| 增量 ID | 来源 | 处置 | 落点 |
|---|---|---|---|
| **ARCH-CRIT-NEW-A** | idempotency_record 同 TX 矛盾（ADR-003 vs §8.1 middleware c.Next 后 Insert） | **FIXED** | backend §8.1 末段标注删除 + service 同 TX 写入正解块 |
| **ARCH-CRIT-NEW-B** | outbox poller DELETE 三义（integration §3.3 / §3.4 / backend §5.4 不一致） | **FIXED** | overview ADR-014 v0.2.1 收敛；integration §3.1/§3.3/§3.4 统一"成功 soft-mark consumed + 30d 批量 DELETE" |
| **ARCH-CRIT-NEW-C** | 缺失 DDL `pipl_request` / `password_reset_token`（`content_safety_event` v0.2 已加）| **FIXED** | backend §3.27 / §3.28 完整 DDL；§14.3 phase 演进表登记 |
| **ARCH-HIGH-NEW-D** | F-3 saga `Idempotency-Key` 类型违约 | **FIXED** | backend §3.21 `topup_intent.saga_id VARCHAR(64) UNIQUE`（UUIDv7）；§5.7 时序图 + integration §4.5 全链路改 saga_id |
| **ARCH-HIGH-NEW-E** | webhook 路径无独立 idempotency middleware | **FIXED** | backend §7.1 v0.2.1 webhook 中间件链（IPAllowlist → SignatureVerify → WebhookIdempotency Redis SETNX (provider,signer,event_id)）；与 user-facing idempotency 隔离 |
| **PM-HIGH-5** | 发票 §3 DDL 缺失 | **CONFIRMED-ALREADY-FIXED**（v0.2 §3.12 已有 invoice_application/title + red_flush_request；本轮抽查无遗漏）|
| **PM-HIGH-6** | 客户充值 saga escalated UX | **FIXED** | frontend §7.5 时序图增 `pending_unknown` + `escalated` 两态 + inapp/email 升级路径；backend §5.7 invariant 触发 notification_outbox |

### 9.2 Security / Compliance 抽查复核（v0.2 声称 FIXED）

抽查 5 条 Security CRITICAL：

- **SEC-CRIT-2** BOLA repo guard：✅ §5.3 `repo.FindByID(ctx, scope, id)` 字面落地；§15.3 CI analyzer 描述齐
- **SEC-CRIT-4** OSS PresignPut magic-byte：✅ backend §9.4 `PresignPut + VerifyUploadedKYCObject`（HEAD + magic-byte + 病毒扫）+ CI AST scan
- **SEC-CRIT-5** dual-control 30min cooldown：✅ backend §7.4 `cooldownStillActive` + §4.5 endpoint 表标注
- **SEC-CRIT-6** SBOM/cosign：✅ overview ADR-016 工具链（govulncheck / nancy / cosign / syft / distroless）字面落地；ops 实际 CI 配置 ACCEPTED-AS-DEBT
- **SEC-CRIT-7** jwt_public_key_pem 移走：✅ backend §3.15 `value_type=secret_ref` + §7.2 env 注入注释 + §7.4 `system.config_write.security` verb 拆分

抽查 5 条 Compliance：

- **COMP-CRIT-3** kyc.purge.cold cron：✅ backend §6 daily 04:30 + §3.9 `cold_archive_expires_at` 索引
- **COMP-CRIT-2** 12377 24h SLA：✅ backend §3.24 `content_safety_report` + §6 dispatcher cron
- **COMP-HIGH-1** partner.tax_status：✅ §3.1 字段 + §5.5 ComputeWithheldTax + §6 yearly cron
- **COMP-HIGH-6** 发票 seller_entity_id + 10y：✅ §3.12 字段 + §5.8 invariant
- **COMP-HIGH-7** PIA 报告：✅ §3.25 + §6 cron + frontend admin 路由

**抽查结论**：未发现"声称 FIXED 但其实没改"的项。v0.2 落地质量优于早期 stdout 摘要预期。

### 9.3 文档行数变更

| 文档 | v0.2 行数 | v0.2.1 行数 | 增量 |
|---|---:|---:|---:|
| 00-architecture-overview.md | 952 | ~1010 | +58 |
| integration-design.md | 1688 | ~1755 | +67 |
| backend-design.md | 3269 | ~3415 | +146 |
| frontend-design.md | 1851 | ~1885 | +34 |

合计补丁条数：**3 CRITICAL（A/B/C）+ 4 HIGH（D/E/PM-5 confirm/PM-6）**。

### 9.4 Round-2 仍可能被打回的 ≤ 3 个点（架构师自评）

1. **§8.1 middleware "正解块" 仍是文字补丁，未真重写代码**：v0.2.1 在末段加注"整段删除 + service 同 TX 写"，但 §8.1 上方 v0.2 代码块本身未删除 `repo.Insert` 那行。Architect Round-2 可能要求"代码骨架级别真改"。
2. **outbox `outbox.purge` cron 在 backend §6 cron 表未显式登记**：v0.2.1 只在 integration §3.4 和 ADR-014 文字说"30d 批量 DELETE"，backend §6 cron 表行未加。Round-2 可能要求 backend §6 表新增一行登记。
3. **password_reset_token 与 §7.9 密码重置流程未做完整时序对照**：§3.28 给了 DDL 但 §7.9 文字没贴二因子流程的时序图（mermaid）。Round-2 PM 视角可能要求 wireframe / e2e 配套。

---

## 10. v0.2.1 → v0.2.2 闭环（Round-2 预防性补丁）

主架构师在 §9.4 自评的 3 项 Round-2 可能仍打回点 → 进入 Round-2 之前提前关闭，输出 v0.2.2。

### 10.1 闭环 3 项

| Risk | 处置 | 主落点 |
|---|---|---|
| **R2-Risk-1**（§8.1 middleware "正解块"仅文字补丁） | backend §8.1 v0.2 中间件代码块**字面整段重写**：middleware 仅做 SELECT 命中检查 + 重放 / 透传，不再调 `repo.Insert`；新增 service 层 `bizDB.Transaction` 闭包内 `idemRepo.Insert(tx, ...)` 骨架；invariant 三连（middleware grep 反断言 + AST analyzer 强制 service 双写同 TX + e2e panic 回滚）；ADR-003 承诺不变 | backend §8.1 v0.2.2 |
| **R2-Risk-2**（`outbox.purge` cron 未登记 backend §6） | backend §6 cron 表新增 `outbox.purge` 行：周期 `15 3 * * *` Asia/Shanghai（每日 03:15 off-peak，避 02:00 settlement / 03:00 kyc.purge.hot）/ 触发条件 `consumed_at < NOW() - INTERVAL 30 DAY` LIMIT 10000 循环 / 幂等（leader-only + 自然幂等）/ 失败补偿（退避 30s × 5 → DPO page）/ SLO（< 30 min；残留 `consumed_at < NOW()-31d` 行 = 0）/ ops `@platform-ops` / Phase 1；同步 integration §3.4 + overview ADR-014 v0.2.2 注脚字面引用 | backend §6 + integration §3.4 + overview ADR-014（v0.2.2）|
| **R2-Risk-3**（password_reset_token 缺时序图 / e2e） | backend §7.9 拆 4 子节：§7.9.1 mermaid（双通道送 token+OTP / 同步 jti revoke / IP/UA 软约束 / PIPL 同意快照 / 信息恒等）+ §7.9.2 8 条 invariant（PR-INV-1～8）+ §7.9.3 ≥3 条 e2e（E2E-PR-1 单次有效+全设备下线、E2E-PR-2 5 次失败 invalidate、E2E-PR-3 信息恒等防枚举）+ §7.9.4 frontend 路由对齐；frontend §6 路由表追加 `/auth/reset/:token` | backend §7.9.1～7.9.4 + frontend §6（v0.2.2）|

### 10.2 文档行数变更

| 文档 | v0.2.1 行数 | v0.2.2 行数 | v0.2.2 增量 |
|---|---:|---:|---:|
| 00-architecture-overview.md | 996 | 1026 | +30（§16 ADDENDUM + ADR-014 注脚）|
| integration-design.md | 1726 | 1742 | +16（§12 ADDENDUM + §3.4 注脚改写）|
| backend-design.md | 3429 | 3625 | +196（§8.1 重写 + §6 新增行 + §7.9.1～7.9.4 + §22 ADDENDUM）|
| frontend-design.md | 1879 | 1901 | +22（§6 路由表 + §23 ADDENDUM）|

合计：**3 项 Round-2 预防性 CRITICAL 闭环**。

### 10.3 v0.2.2 自查 Round-2 残余风险

- **R2-Risk-4（HIGH 候选，移交 Round-2 评估）**：§7.9 双因子 = email 链接 + SMS OTP；用户 email 被攻陷 + SIM swap 同时发生 → 仍可重置。Mitigation：staff 路径 §7.5 已强制 WebAuthn step-up；customer / partner 接受残余风险；v0.2.3 评估"高风险账户额外 KYC 实人比对"。**v0.2.2 不处理**，写入债务清单。
- 未发现新的 CRITICAL；未发现 v0.2.1 已修但 v0.2.2 又回归的项。

### 10.4 Round-2 verdict 期望

0 CRITICAL / HIGH ≤ 4（含 R2-Risk-4 候选）/ 全部 HIGH 在债务清单中显式接受。

