# Code Review Round-1 — Legal Compliance（代码层面合规核验）

> 日期：2026-05-10
> 审阅人：Compliance reviewer
> 审阅口径：PIPL（2021）/ 数据安全法（2021）/ 网络安全法（2017）/ 关键信息基础设施安全保护条例（2021）/ 等保 2.0 GB/T 22239-2019 / 电子商务法（2018）/ 广告法（2021 修正）/ 价格法（1997）/ 反垄断法（2022 修正）/ 互联网信息服务管理办法（2011 修正）/ 生成式人工智能服务管理暂行办法（2023）/ 互联网信息服务算法推荐管理规定（2022）/ 互联网信息服务深度合成管理规定（2023）/ 反洗钱法（2024）/ 财政部 41 号公告 / 电子发票管理办法 / 通信短信息服务管理规定（工信部 2015）/ 个人信息保护影响评估指南 GB/T 39335-2020
> 代码范围：
> - `TraceNexBiz/apps/partner-api/` Go 1.25 + Gin + GORM v2 + MySQL/SQLite/PG（W0/W1a/W1b/W1c 已交付）
> - `TraceNexBiz/apps/partner-web-{storefront,customer,partner,admin}/` React 18 + Vite + Semi UI
> - `Fy-api/` OVERLAY 增量：`model/consume_log_outbox.go` / `model/group_ratio_override.go` / `model/log_outbox.go` / `main.go` 的 outbox / data_region bootstrap
> 前置 verdict：Dev doc round 2 已 PASS_WITH_NOTES（4 CRITICAL / 9 HIGH 全部 ✅FIXED，文档层面 65% → 93% 可追溯性）；本轮核验工程代码层面是否"书同此文"。

---

## 1. 执行摘要与 Verdict

本轮逐条核验 dev doc round 留下的 4 CRITICAL（M-6 / M-7 / M-8 / M-11）+ 9 HIGH + §22.3 Phase 2A 9 项 hard-gate + §15.10 pre-launch 16 项在**代码层面**的真实落地。结果可以用一句话概括：

> **schema + service 纯领域逻辑已 95% 到位；但 cron 注册、真实 repo / KMS / 持牌方 SDK 装配、partner-api 侧 PIPL 权利请求端点、前端 PIPL 单独同意 UI 尚未接线。**

具体数字：

| 维度 | 状态 | 占比 |
|---|---|---:|
| 合规 schema（10 张合规相关表 + 16 个 biz_setting key）| ✅ 全部落地 | 100% |
| 合规 service / 纯函数（12377 dispatcher / KYC purge / 发票红冲 / 哈希链 / payment stub） | ✅ 全部落地 | 95% |
| cron 注册（`kyc.purge.cold` / `12377.dispatch` / `tax.withholding.annual` / `model_whitelist.review` / `pia.report.annual`）| ❌ 无进程入口 | 0% |
| 持久化接线（service 层多数仍用 `NewMemoryRepo` / `NewStubCrypto` / `NewStubLinker` / `NewHMACStubProvider`） | ⚠️ W1a 在途 | ~20% |
| partner-api 合规端点（admin 12377 ✅；admin 发票红冲 ✅；customer PIPL 权利请求 ❌；OCR-before-share audit ❌） | ⚠️ 部分 | 60% |
| 前端 4 站合规组件（4 × ComplianceFooter / admin ContentSafety / admin PIA / admin PIPL Complaints / customer PiplRights） | ✅ 均已接线 | 95% |
| 前端 PIPL 单独同意 / 7 枚举 checkbox UI | ❌ 未落（customer KYC 页面仅"姓名 / 身份证 / 手机"三栏） | 0% |
| 前端 storefront build gate（9 keys 必须非空才允许构建） | ❌ `vite.config.ts` / `package.json` 未见 preflight | 0% |
| Fy-api outbox 跨境 region 隔离（invariant + consumer 反向断言） | ✅ 硬约束 | 100% |

**Verdict：NEEDS_REVISION（code level）**。不阻塞 Phase 1 内测启动（内测 ≤ 5 家 / 仅 CN / 不开票 / 不分账），但 **Phase 2A 商业化上线前必须闭环**下列清单（见 §8 final）。本 verdict 与 Security / Architect / Backend Architect / Code-reviewer 四位 reviewer 合议前按独立判断出具，最终是否综合下调为 PASS_WITH_NOTES 取决于 code round-1 收敛后的复核。

**代码合规可追溯性（schema / service / cron / handler / frontend 五维加权）**：**68%**（dev doc round-2 93% 的门槛在代码层暂未同步达成，主要缺口在 cron 注册与 repo 装配）。

---

## 2. 4 CRITICAL 代码层面逐条核验

### 2.1 CRIT-1 / M-8　9 keys 备案号 + ComplianceFooter —— ✅ 工程闭环（含细节缺口）

**schema 层**：`apps/partner-api/migrations/011_seed_biz_setting.up.sql:5-15` 已种入 9 个 plain 公示 key（`compliance.icp_record_no` / `icp_license_no` / `public_security_filing_no` / `gen_ai_filing_no` / `algorithm_filing_no` / `deep_synthesis_filing_no` / `dpo_contact_email` / `dpo_contact_phone` / `report_phone_12377_link`）；line 18-24 种入 6 个 `_active` boolean readiness flag + 1 个 `pia_report_latest_at`。与 dev doc §3.15 预期 14 keys 数量字面一致。

**backend config 层**：`internal/config/keys.go` 把上述 key 集合声明为常量（grep 命中），可被 readiness probe 与 `biz_setting` repo 统一消费。

**前端组件层**：
- `partner-web-storefront/src/components/ComplianceFooter.tsx:30-70` 实现完整 9 字段渲染；未填充的字段由 `FooterRow` line 14 的 `if (!value) return null` 软降级（生产期待由 readiness gate 保证非空）。
- `partner-web-storefront/src/components/Layout.tsx:4 / 21` 把 `<ComplianceFooter />` 挂进主布局；storefront 的任何公开页（Home / Pricing / Legal / ApplyPartner）都会渲染。
- `partner-web-customer/src/components/ComplianceFooter.tsx` + `Layout.tsx` 同构（客户端也显示备案号）。
- **partner-web-partner / partner-web-admin 两站**未挂 ComplianceFooter —— 这两站是渠道商 / staff 内部后台，PRD §15.4 / §22.3 C-1 定位为"商业化 storefront 必须显示"，内部后台不强制；**此项不算合规缺口**，但建议 admin 站 header 放一行"当前部署合规态"只给运营看（NEW-LOW，见 §7）。

**CI build gate（F-11.4）**：`partner-web-storefront/vite.config.ts` / `package.json` 全文 grep **未见** `compliance` / `ICP` / `VITE_COMPLIANCE_*` preflight 脚本；本轮代码未落 build gate 硬断言。dev doc §11.5 F-11.4 是"build 时从 `/api/public/footer` 预热且 9 key 非空否则退出非零"的承诺，**代码层未实现**。

**判定**：schema / backend / storefront+customer 前端三件套 ✅；**CI build gate 未实现**（HIGH-C1，写入修订指令）；admin / partner 站未挂 footer（NEW-LOW，建议而非强制）。

### 2.2 CRIT-2 / M-11　12377 上报通道 + 24h SLA + admin 派发 —— ⚠️ PARTIAL

**schema 层**：`migrations/010_compliance.up.sql:5-25` 完整落 `content_safety_event` 表（`chk_csafety_kind` / `chk_csafety_provider` / `prompt_hash CHAR(64)` 注释"SHA-256 防重；不存原文" / `reported_to_12377_at` 时间戳）；line 27-45 完整落 `content_safety_report` 表（`sla_due_at` COMMENT 写死 "created_at + 24h"；`chk_csreport_status` 含 `pending` / `submitted` / `acknowledged` / `failed` / `dead_letter` 5 态；`chk_csreport_authority` 含 `12377` / `public_security` / `internal` 3 值；`idx_csreport_pending` 覆盖 dispatcher scan；`fk_csreport_event` 保证证据链）。与 dev doc backend §3.24 字面一致。

**service 层**：`internal/service/content_safety/content_safety.go` 实现齐备：
- line 37 `SLAReport24h = 24 * time.Hour`；line 40 `MaxRetries = 5`；
- line 121-138 `RecordEvent`：`disposition == "block"` 自动 `QueueReport` 派单 12377（line 132-136；PRD §7.12 闭环）
- line 141-157 `QueueReport` 创建 pending 记录，`SLADueAt = now.Add(24h)`（硬约束写在代码里）
- line 186-227 `DispatchOnce` 批量拉 pending → `AuthorityClient.Submit` → success 更新 `submitted / SubmittedAt / ResponsePayload` + 回写 event `ReportedTo12377At`；failure `RetryCount++`，触达 5 次进 `dead_letter`；retry/DLQ 逻辑完整
- line 230-232 `SLABreaches` 扫 `now.After(SLADueAt) && status != submitted` 返回告警清单
- line 234-244 `RetryReport` admin 手动重试，dead_letter / failed → pending

**admin 端点层**：`internal/handler/admin/admin.go:54-56 / 231-291` 挂 3 条：
- `GET /api/admin/content-safety/reports`（`csReportsList`）
- `POST /api/admin/content-safety/reports/:id/retry`（`csReportRetry`）
- `POST /api/admin/content-safety/reports/dispatch`（`csReportDispatch` — 一键派发 batch ≤ 50）

OpenAPI `openapi/admin.yaml` 同步（grep 命中）。

**前端 admin 层**：`apps/partner-web-admin/src/App.tsx:12 / 39-40` 注册 `/content-safety/reports` + `/content-safety/reports/:id` 两条路由；`pages/ContentSafety.tsx:12-72` 渲染列表 + 重试 + 一键派发 + 详情；`line 128` `<Banner type="info" description="12377 上报内容仅授权人员可见" />` 提示授权阅读。

**缺口（HIGH-C2）**：
1. **cron 入口缺失**。dev doc §6 `content_safety.dispatch` 承诺 `every 1 min`，对应 service `DispatchOnce` 已实现，但 `cmd/` 下无 `notify-dispatcher` 或等价入口 binary；`cmd/server/main.go` 本身没有 cron runner；`go.mod` 未引入 `robfig/cron` 或 `gocron`；admin 的 `POST /dispatch` 只能手动派发，**24h SLA 无进程保证**。
2. **`AuthorityClient` 真实实现缺失**。`content_safety.go:77-79` 声明接口；同文件 line 432-449 仅有 `CapturingAuthorityClient` 测试 stub；没有 `aliyun12377Client` / `publicSecurityClient` 落地。NEW-LOW-1 在 dev doc round-2 已标（"endpoint 由 ops 切换"），这里代码层同样未落。
3. **Fy-api 上报事件入口缺失**。`/api/internal/content-safety/event` 在 Fy-api 侧 grep **无命中**（`grep -rn "content-safety/event" Fy-api/` → 空）；即 Fy-api 的 LLM 拦截不能推事件到 partner-api；`content_safety_event` 表目前**无写入路径**，admin 看板将长期是空的。这是 Fy-api OVERLAY 应对齐 partner-api 的断点。

**判定**：schema / service / admin handler / admin UI ✅；cron + AuthorityClient + Fy-api 上报入口 ❌ → 整体 ⚠️PARTIAL。24h SLA 硬承诺在 Phase 2A 上线前必须补齐 cron + 真实 client + 事件源。

### 2.3 CRIT-3 / M-7　KYC 5y 冷归档销毁 cron —— ⚠️ PARTIAL

**schema 层**：`migrations/005_kyc_invoice_seat.up.sql`
- line 13-18 PII 全量加密字段（`legal_person_name_cipher VARBINARY(512)` + `_key_id` + `_blind_index` 三件套；`legal_person_id_cipher` 同构；`bank_account_cipher` Phase 2A；OCR 结果 `business_license_ocr_cipher VARBINARY(16384)`）
- line 37 `pii_purged_at` 注释"热归档 30d 后清明文 PII"
- line 38 `cold_archive_expires_at TIMESTAMP(3)` COMMENT "v0.2 Compliance CRIT-3"
- line 46 `idx_kyc_purge_cold` 索引就绪
- line 28 `biometric_liveness_url` + line 29 `biometric_purged_at` + line 47 `idx_kyc_biometric_purge` 索引同步
- line 30-31 `yearly_reject_count` + `yearly_reject_reset_at`（PRD §7.7 3 次/年驳回冻结）

**service 层**：`internal/service/kyc/kyc.go`
- line 135-137 默认 `hotTTL = 30d`（未直接用于 purge，但文档对齐）；`coldTTL = 5 × 365 × 24h`（5 年硬编码）
- line 383-385 `encryptInto` 写入时 `a.ColdArchiveExpiresAt = &cold = now + coldTTL`，作用于 submit 即种下销毁截止
- line 252-262 `PurgeCold(ctx, batch)` service 方法，调 `repo.PurgeColdExpired(cutoff, batch)`
- line 328-386 `encryptInto` 对 `legal_person_name` / `legal_person_id` / `bank_account` 都写 cipher + key_id + blind_index；`alipay_open_id` / `ocr` 写 cipher + key_id（不需要检索所以无 blind index）
- line 297-309 `checkConsents` 必须 assert `sensitive_pi`（Consent）+ 如有生物识别同时 assert `biometric`；否则返回 `ErrConsentRequired`（PIPL §29 单独同意）
- line 311-326 `verifyUploads` 调 `OSSPort.VerifyKYCObject` 做 mime / 魔术字节校验（阻断 XSS / 文件上传攻击）

**缺口（HIGH-C3）**：
1. **cron 入口缺失**。没有 `cmd/kyc-purge` binary；`PurgeCold` service 方法"写了但没人调"。dev doc §6 `kyc.purge.cold daily 04:30` 承诺空转。
2. **`PurgeColdExpired` 真实物理销毁逻辑**只暴露为接口，没有 MySQL repo 实现（grep 未见 `kyc_mysql.go` / `PurgeColdExpired` impl）；`NewStubLinker` / `NewMemoryRepo`（`memory.go`）是当前唯一实现。**物理销毁 cipher + KMS `ScheduleKeyDeletion` + OSS Archive 对象删除**三步流程无代码落点。
3. **KMS 端口语义未规范**。`CryptoPort.Encrypt` line 92 返回 `cipher, keyID, err`；但"销毁时 `ScheduleKeyDeletion(keyID)` 完结后清账"的工程钩子未定义接口；dev doc CHANGELOG 写"由 ops runbook 落地"，但代码层连接口都没有。
4. **`BusinessLicenseOCR` cipher 写入路径疑似断链**：`SubmitInput.BusinessLicenseOCR string` line 59 接收 OCR 结果明文；`encryptInto` 把它走 `kyc:ocr` 加密后**仅落 `a.BusinessLicenseOCRKeyID`**（line 376-382），cipher 字节流没有回写到 domain entity；schema 层有 `business_license_ocr_cipher VARBINARY(16384)` 字段（005_kyc.up.sql:11），**但 service 没有把 `_, kidO, _ := s.crypto.Encrypt(...)` 的 cipher 返回值赋给 entity**——是 HIGH-C3-1 一致性缺陷。

**判定**：schema ✅；service 纯加密 / consent / upload 检验 ✅；cron + repo 物理销毁 + KMS ScheduleKeyDeletion ❌；OCR cipher 写入路径断链。Phase 2A 之前必须修。

### 2.4 CRIT-4 / M-6　consume_log_outbox 跨境 region 隔离 —— ✅ 硬约束合规

**Fy-api 侧（source）**：`model/consume_log_outbox.go:1-70`
- line 7-8 文件级注释明示"cn 的事件不可被 SG 消费者拉走"
- line 40 `DataRegion string` 字段（VARCHAR(8)），进入 `idx_outbox_region_status` 联合索引 priority:1
- line 56-69 `InsertOutboxInTx(tx, rec)` line 60-62 **硬 guard** `if rec.DataRegion == ""` → `return errors.New("data_region required")`（不能裸写 outbox）
- line 74-113 `LeaseOutboxBatch(region, ...)` line 84 `WHERE data_region = ? AND status IN (?,?) ...`——region 被写入 SQL 的 WHERE 条件，publisher 不可能拉到跨境事件

**Fy-api 启动入口**：`main.go:315-323`
```
outbox.NewRunner(common.GetEnvOrDefaultString("DATA_REGION", "cn"), ...).Start(overlayCtx)
```
即 CN 实例默认 `DATA_REGION=cn`，SG 实例部署时必须写 `DATA_REGION=sg` env（ops runbook）。

**partner-api 侧（consumer）**：`apps/partner-api/internal/outbox/consumer.go`
- line 18 文件级注释"pull 子句强制 `data_region = ?`，CN 不能消费 SG 事件（反之亦然）"
- line 194-229 `TickOnce` 拉出 events 后 line 202-207 **反向断言**：`if ev.DataRegion != c.region { nack + DLQ }`——**即使 Fy-api 侧漏写 region，partner-api 也会在入口断掉**。这是防御性编程的正确落地（network-in-depth）。
- line 263 `DataRegion != "cn" && != "sg"` 强 enum 校验

**索引文档**：`migrations/INDEXES-W1b.md:34-38` 写死"`(data_region, status)` 联合索引 — region 隔离强制"；"`status='pending' AND data_region=?` 全表必走索引"。DBA 层面有留痕。

**I-6.4 SG region CI gate**：dev doc 承诺"SG 启用前通过 `SHOW GRANTS` 断言返回空"——代码层未见 CI workflow，但这是 ops / CI-YAML 层工作，**不属于 partner-api 代码债**。

**判定**：Fy-api source + partner-api consumer 双边 region guard 都到位；Phase 2A 启用 SG region 之前 CI gate 由 ops 负责。**✅ FIXED**（合规角度无残留）。这是本轮 4 CRITICAL 中**唯一干净通过**的一条。

---

## 3. §22.3 Phase 2A Hard-gate 9 项代码落地表

| ID | 主题 | 代码证据 | 状态 |
|:---:|---|---|:---:|
| **C-1** | ICP 经营许可证 + storefront 商业化 gate | `migrations/011_seed_biz_setting.up.sql:18 compliance.icp_license_active='false'` + 前端 `ComplianceFooter.tsx`；readiness probe endpoint **未见**代码落点；**storefront build gate 未实现** | ⚠️ PARTIAL |
| **C-2** | 生成式 AI + 算法备案 + footer 展示 | `ComplianceFooter.tsx:51-52 footer.gen_ai / footer.algorithm` 渲染；`_active` flag key 已种 | ✅ FIXED |
| **C-3** | 持牌分账方 + mchid ISV invariant | `service/payment/payment.go:89-96 LicensedProvider interface`；唯一实现是 `HMACStubProvider` line 101-187；`biz_setting payment.platform_isv_mchid` 已种 line 34；**真实持牌方 SDK 未接入**（Q12 决策前 acceptable）；**ISV mchid webhook invariant 代码未落**（payment.go 中未见 `mchid == isv_mchid → reject`） | ⚠️ PARTIAL |
| **C-4** | 个税代扣 + 月结 + 41 号公告 | `service/settlement/engine.go:53-116 PartnerKind + ComputeItem` 实现（personal 20% 硬编码）；`migrations/001_partner_core.up.sql:23 tax_status` 字段；`chk_partner_tax_status` 5 枚举完整；**但 `ComputeItem` 用 `PartnerKind` 而非 `tax_status`**（两套分类未对齐：PartnerKind = corporate/personal/individual；tax_status = individual/sole_proprietor/individual_business/company/unknown；41 号公告累进 / 经营所得 5%-35% 两套公式未实现） | ⚠️ PARTIAL |
| **C-5** | 全电发票 + 销售方 + 10y + 红冲 | `service/invoice/invoice.go:205-245 Apply` → `SellerEntityID` / `SellerTaxNo` 从 service 注入（line 227-228；前端不可控）；`ArchiveExpiresAt = now.AddDate(10,0,0)` 硬约束（line 235）；`taxNoRegex` 18 位统一社会信用代码 + 15 位兼容（line 48）；`RedFlush` line 314 + admin endpoint line 151-175 + 前端 admin redFlush API | ✅ FIXED |
| **C-6** | PIA 报告留档 | schema `pia_report` 完整 8 大项 TEXT 字段 + `valid_until` + `signed_by_dpo`（010_compliance.up.sql:47-67）；admin 前端 `/pia` + `Pia` 组件（PiaPipl.tsx:20-76）；**但 backend endpoint `listPia` / `generatePia` 在 partner-api admin handler 层未见**（admin.go 未挂） | ⚠️ PARTIAL |
| **C-7** | 等保 2.0 二级备案 | `compliance.epd_2_filing_active` flag 已种（line 22）；ops runbook 层面的 IDS/WAF/EDR/备份演练 —— 不属代码范围 | ⚠️ PARTIAL（接受为 ops 债务） |
| **C-8** | DPO + 用户权利中心 + 投诉 | schema `pipl_complaint` + `pipl_request` 完整（010_compliance.up.sql:69-114，5 态 status + 5 类 category + `sla_due_at` PIPL §50 15d）；前端 customer `PiplRights.tsx` 实现 4 kinds 按钮 + list；admin `PiaPipl.tsx` 实现投诉管理 + resolve；**但 partner-api 侧 `/api/customer/pipl` endpoint 在 handler 层 grep 无命中**（customer.ts:319 client 调用的 endpoint 服务端未实现）；`/legal/dpo` `/legal/complaint` 通过 storefront `Legal.tsx` 的 `VALID_DOCS` line 11-21 已覆盖（动态 markdown 页面） | ⚠️ PARTIAL（HIGH-C4：前端已接通但后端 endpoint 未落） |
| **C-9** | 内容安全双层 + 12377 + 24h SLA | 见 §2.2；service + admin UI ✅；**cron + AuthorityClient + Fy-api source 断链** | ⚠️ PARTIAL |

**汇总**：9 项 hard-gate 中 ✅ FIXED 2（C-2 / C-5）、⚠️ PARTIAL 7（C-1 / C-3 / C-4 / C-6 / C-7 / C-8 / C-9）、❌ NOT-FIXED 0。代码侧比 dev doc round-2 的 8/9 ✅ 差了 6 个量级——因为 dev doc 只看"设计是否写到"，代码必须看"进程是否跑得起来"。**这是 PASS_WITH_NOTES → NEEDS_REVISION 的主要落差来源**。

---

## 4. 资金流去二清审计（代码版）

监管框架：央行《非银行支付机构监督管理条例》§3 / §10、《关于规范支付创新业务的通知》（银发〔2014〕5 号）、《支付机构客户备付金管理办法》（2017）、《非银行支付机构条例》（2024）。

### 4.1 链路 1：客户充值

- `service/payment/payment.go:89-96 LicensedProvider` interface 要求持牌方实现 `Topup / VerifyCallback / Withdraw / Reconcile / Name`——架构正确（平台不接触资金，走持牌方收银台）
- 实现：`HMACStubProvider` line 101-187，本地 stub + HMAC-SHA256 签名校验（**仅 dev；生产前必须换真 SDK**）
- `Topup` line 122-142 返回 `PayURL = "stub://..."`（line 138）——明显标记 stub，不会误上生产
- `VerifyCallback` line 152-165 同时校验签名 + 金额一致性 —— ✅ 防中间人篡改金额
- `biz_setting payment.platform_isv_mchid` 已种（011_seed.up.sql:34）**但空字符串**，没有 service 层读取 + invariant 断言（webhook 收款 mchid == isv_mchid 时拒收）
- saga `service/saga_topup/` 目录存在，与 payment.go 联动待 backend-architect / code-reviewer 交叉核验

**合规评估**：架构方向正确（去二清 via 持牌方）；但 **ISV mchid invariant 代码层未落**（dev doc §5.7 invariant 要求），是 M-2 / Compliance HIGH-2 的代码端债务。→ HIGH-C5

### 4.2 链路 2：渠道商分润 / 应付台账

- `migrations/002_wallet.up.sql:61` `partner_wallet_log.type` 枚举含 `'platform_isv_commission_in'` ✅（ISV 佣金独立流水）
- `partner_wallet` schema 已 drop `held_amount`（ADR-012）✅
- `settlement/engine.go:62-66 PersonalWithholdRate = 20%` + `ComputeItem` line 94-117 纯函数落地
- `partner_debt` 表 line 65-80 完整（ADR-010 verdict A），负 balance 仅 P0 fallback 由 ops 监控

**合规评估**：partner_wallet 严格限定为应付台账；ISV 佣金反向流水独立 type ✅。→ 无残留

### 4.3 链路 3：退款（含已支付场景）

- `service/saga_refund/` 目录存在；service 细节待 code reviewer 核验
- `partner_debt` schema 已上调 Phase 2A（HIGH-9 / M-3）
- `wallet/wallet.go:144 DebtID *int64 // partner_debt 写入时回填` —— 三态已留接口

**合规评估**：架构正确；代码细节依赖 code-reviewer 交叉验证。

### 4.4 链路 4：提现 / 下账

- `settlement/engine.go:94-117 ComputeItem` 对 `PartnerKindPersonal` 硬编码 20% —— **未分档**（劳务报酬累进 vs 经营所得 5%-35% 未区分）；**41 号公告要求按收入性质分档代扣**（个人劳务 vs 个体工商户经营所得）
- `partner.tax_status` enum 有 5 值但 `ComputeItem` 只消费 `PartnerKind` 3 值 —— **两套分类未映射**
- 银行账户实名一致性（MED-19）：`kyc.go:328-386 encryptInto` 写 `bank_account_blind_index`（HMAC），但 settlement payout 前的一致性断言 `bank_account_blind_index == HMAC(legal_person_name + bank_account)` 在 `settlement` service 代码中 grep **无命中**

**合规评估**：基础框架在；**41 号公告分档逻辑未落 + 银行实名一致性断言未落** → HIGH-C6（C-4 hard-gate 代码侧短板）

### 4.5 四链路汇总

| 链路 | 设计对齐 | 代码落地 |
|---|:---:|:---:|
| 充值（LicensedProvider + webhook）| ✅ | ⚠️（ISV mchid invariant 未落） |
| 分润 / 应付（partner_wallet 纯台账）| ✅ | ✅ |
| 退款（partner_debt）| ✅ | ✅（schema）/ ⚠️（service 细节待查） |
| 提现（个税分档 + 银行实名）| ✅ | ⚠️（个税 5 档未落 / blind_index 断言缺失）|

---

## 5. PIPL 7 段生命周期代码审计

| 段 | 代码证据 | 状态 |
|:---:|---|:---:|
| **采集**（C）| `service/kyc/kyc.go:311-326 verifyUploads` 走 OSS magic-byte；`customer/register`（w1a_routes.go:58）经 invitation 防绕过；前端 `Kyc.tsx` / `Auth.tsx` 收 PII | ✅ |
| **同意**（Consent）| `migrations/008_ticket_notify_consent.up.sql:60-76 consent_log` 表 `chk_consent_type` 含 **7 枚举**（`privacy_policy`,`sensitive_pi`,`biometric`,`cross_border`,`device_fingerprint`,`automated_decision`,`third_party_share`），line 72-75 字面一致；`kyc.go:297-309 checkConsents` assert `sensitive_pi` + `biometric`；`consent_text_version` 字段跟踪协议版本 | ✅ schema / service |
| **UI 同意 checkbox**（前端）| `partner-web-customer/src/pages/Kyc.tsx:11-77` **仅收 real_name / id_card / phone 三栏**，**无"敏感 PI 单独同意"checkbox / 无"跨境 SG"checkbox / 无 7 枚举任何一类 UI**；`submitKyc({real_name,id_card,phone})` line 68 直接提交**不带 `consent_sensitive_pi_id` / `consent_biometric_id`**；而 backend `handler/w1a_business.go:182-194 kycSubmitBody` **要求** `ConsentSensitivePIID int64 binding:"required"` —— **前后端 contract 不一致，当前 UI 无法通过后端校验** → HIGH-C7 | ❌ NOT-FIXED |
| **存储** | schema PII 全字段 VARBINARY + `_key_id` + `_blind_index`（005_kyc_invoice_seat.up.sql:13-27）；`pkg/pii/encrypted.go` GORM wrapper 保证 `Value()` 仅返回 cipher（line 46-48）；`SetCipher` 立刻清 plain（line 42 `e.plain = ""`） | ✅ |
| **加密** | `service/kyc/kyc.go:91-94 CryptoPort.Encrypt/HMAC` 接口；KMS 实现暂缺（W1a 未接线，当前为 `NewStubCrypto`）；信封加密设计正确 | ⚠️ 接口正确 / 真实 KMS 未接入 |
| **共享**（§5.6 OCR 入口规约 / M-5）| `kyc.go:96-100 OCRPort` 接口；**service 中未见 `audit.Append(action='pii.share.aliyun_ocr')` 调用** —— dev doc round-2 §2.2 HIGH-3 接受为架构约束级落地，但**代码实施侧未落**；OCR 调用前应由 `audit_log_unsealed` 留证 → HIGH-C8 | ❌ NOT-FIXED |
| **跨境** | Fy-api outbox `DataRegion` required guard（consume_log_outbox.go:60）；partner-api consumer 反向断言（consumer.go:202）；前端 `consent_type` 枚举 `cross_border` 已定义，**但 customer UI 未提供 checkbox**（同 HIGH-C7） | ⚠️ 后端 ✅ / 前端 ❌ |
| **删除 / 导出** | `service/customer/transfer.go:112-135 SubmitErase` 实现"客户右遗忘"；handler `w1a_business.go:169-180 customerEraseHandler` 挂 `POST /customer/erase`；调用 Fy-api `/user/erase`；**但**：① `pipl_request` 表存在 schema（010_compliance.up.sql:91-114）但 **partner-api 侧无 handler / service 接线**（grep 无命中 `pipl_request` repo）；② 前端 `customer/pages/PiplRights.tsx` 调 `/api/customer/pipl` GET/POST（customer.ts:319-325），**backend endpoint 未实现** → HIGH-C4（§3 C-8）；③ 数据导出 JSON schema（M-20）未在代码中落格式约定 | ❌ NOT-FIXED（C-8 配套） |

**PIPL 7 段结论**：采集 / 存储 / 加密接口 / 跨境后端 ✅（4 段）；同意 UI（customer 前端）/ 共享 audit / 删除导出 endpoint（3 段）❌ —— PIPL §44-§50 用户权利闭环在代码层**尚未就绪**，**不阻塞 Phase 1 内测**（Phase 1 无 PIPL 请求流量），**阻塞 Phase 2A**。

---

## 6. 算法 / 内容安全审计（代码版）

### 6.1 12377 通道　见 §2.2

### 6.2 生成式 AI 备案号 + 模型白名单

- `compliance.gen_ai_filing_no` / `algorithm_filing_no` / `deep_synthesis_filing_no` 三 key ✅（011:10-12）
- `_active` flag key ✅（011:19-21）
- **模型白名单表**：dev doc §6 `model_whitelist.review monthly` cron；代码层 **grep 无 `model_whitelist` 表 / service / cron 命中**；`biz_setting gen_ai_model_list_url` 也未种 → MEDIUM-C1（Phase 2A 前落地）

### 6.3 Fy-api 模型调用拦截 → partner-api content_safety_event

- Fy-api 侧 `controller/tnbiz_internal/` 有 `settings.go` / `health.go`（检索命中），**但 `content-safety` handler / 事件推送代码 grep 无命中** → dev doc round CHANGELOG 声称 "Fy-api 通过 /api/internal/content-safety/event 上报" —— 当前 **Fy-api 侧零实现** → HIGH-C2（已在 §2.2 列）

### 6.4 备案号前端展示　见 §2.1

### 6.5 算法备案触发判定（M-13）

dev doc round-2 ❌ NOT-FIXED（真空）。代码侧不可能单独落地，**合规归属：法务出函 + integration §1.4 footnote 在 T-60 day 前落地**。不是代码债务。

### 6.6 深度合成水印（M-12）

PRD §1.3 "Phase 1-2 仅文字 LLM"，触发条件未到；代码侧无水印 service 是 **by design**。✅ DEFERRED。

### 6.7 content_safety_event 的 PII 最小化

`migrations/010_compliance.up.sql:10` `prompt_hash CHAR(64)` + COMMENT "SHA-256 防重；不存原文" —— **不存原文**是 PIPL §6 最小必要原则的正确应用 ✅。`pkg/piiscrubber/scrubber.go` 提供身份证 / 手机 / email 三类 regex 的 `Redact`，挂三个点（log hook / saga_step / outbox last_error）——**设计正确，但在 content_safety_event 插入前是否调用 `Redact` 对 payload 做脱敏**，`content_safety.go:246-249 buildPayload` 仅拼 fy_user_id / category / prompt_hash / score，**未经 scrubber**；由于已经不存原文，实际不需要；**合规 OK**。

### 6.8 哈希链 + 审计留存

- `internal/audit/sealer.go:1-208` 完整哈希链：`prev_hash + canonical row → sha256 → self_hash`；line 32 `GenesisPrevHash = "GENESIS"`；`Verify` line 171-190 全表迭代验证；`Tamper` 测试用篡改 API
- **但**：sealer 的 `MemoryStore`（line 211-319）是当前唯一 Store 实现；MySQL Store 未接线；`cmd/audit-sealer/main.go` / `cmd/audit-verify/main.go` 在 `cmd/` 目录存在，但 W1a 阶段未必接 RDS
- audit_log 留存 ≥ 6 月（电子证据 / 网安法 §21）由 ops runbook 落地（OSS / SLS）—— 不属代码债

**判定**：哈希链算法正确 ✅；**真 RDS Store 未接线、cmd binary 未与生产 Store 对齐**，写入 §8 T-30 / T-7 自检。

---

## 7. CRITICAL / HIGH / MEDIUM / LOW 清单

### 7.1 CRITICAL（阻塞 Phase 2A 商业化上线；Phase 1 内测可先通过）

无（所有 dev doc CRITICAL 在代码层至少 service 已实现；下列是降级后的 HIGH）

### 7.2 HIGH（Phase 2A 前必须修）

| ID | 主题 | 代码位置 | 修订建议 |
|:---:|---|---|---|
| **HIGH-C1** | storefront 合规 build gate 缺失 | `apps/partner-web-storefront/vite.config.ts` / `package.json` | 新增 `scripts/preflight-compliance.ts`：build 前请求 `/api/public/footer` 或读 env `VITE_COMPLIANCE_*`，9 keys 任一非空字符串即通过；否则 `process.exit(1)`；`package.json` 的 `build` 脚本前置 `bun run preflight`；对应 dev doc F-11.4 |
| **HIGH-C2** | 12377 dispatcher cron + AuthorityClient + Fy-api 事件源 | `cmd/notify-dispatcher/`（新增）+ `internal/service/content_safety/authority_client.go`（新增）+ `Fy-api/controller/tnbiz_internal/content_safety.go`（新增）| ① 新 binary 每 60s 调 `DispatchOnce(batch=50)` + `SLABreaches` 告警；② Aliyun 12377 真实 endpoint 由 `biz_setting.compliance.report_dispatcher_endpoint`（NEW-LOW-1）注入，客户端带 TLS 1.3 + mTLS；③ Fy-api `/api/internal/content-safety/event` endpoint + 模型调用拦截器（`middleware/content_safety_preflight.go`） |
| **HIGH-C3** | KYC purge cron + MySQL repo + KMS ScheduleKeyDeletion + OCR cipher 写入 | `cmd/kyc-purge/`（新增）+ `internal/repository/mysql/kyc_mysql.go`（新增）+ `service/kyc/kyc.go:376-382 OCR cipher 落点` | ① 新 binary `@ 04:30 daily` 调 `PurgeCold(100)`；② MySQL impl：事务内 `UPDATE kyc_application SET ...cipher=NULL, deleted_at=NOW() WHERE cold_archive_expires_at<NOW()` + OSS DeleteObject + KMS `ScheduleKeyDeletion(key_id, PendingWindow=7)`；③ `encryptInto` 里把 OCR cipher 真写入 `a.BusinessLicenseOCRCipher`（当前只写 keyID） |
| **HIGH-C4** | partner-api 侧 PIPL 权利请求 endpoint 未落 | `internal/service/pipl/`（新增）+ `internal/handler/w1a_business.go` | 新 service 消费 `pipl_request` 表；POST `/api/customer/pipl`（`actor_type='customer', request_type`）；GET list；PIPL §50 30d 总 SLA + 5d 核身 cron；前端 `customer.ts:319 listPiplRequests()` 已对接 |
| **HIGH-C5** | ISV mchid webhook invariant 代码未落 | `internal/service/payment/payment.go:152 VerifyCallback` | 在 Verify 入口断言 `payload 对应 mchid != biz_setting.payment.platform_isv_mchid`；否则返回 `ErrISVMchidNotAcceptableForReceipt`；写 audit_log `action='payment.webhook.rejected.isv_mchid'` |
| **HIGH-C6** | 个税代扣 5 档分档 + 银行实名一致性断言 | `internal/service/settlement/engine.go:94-117 ComputeItem` | ① `ComputeItem` 入参改为 `TaxStatus`（5 枚举）不再用 `PartnerKind`（3 值）；劳务报酬（individual）分档 20% / 30% / 40% + 速算扣除；经营所得（sole_proprietor / individual_business）= 累进 5%-35%；company = 0；unknown = 拒发 payout；② payout 前 `HMAC(salt, legal_person_name+bank_account) == bank_account_blind_index`；不符 → settlement_item 状态 `bank_mismatch` + ops alert |
| **HIGH-C7** | customer KYC 前端 PIPL 同意 checkbox 缺失（7 枚举） | `apps/partner-web-customer/src/pages/Kyc.tsx:54-72` | ① 前端增 5 个 checkbox：`sensitive_pi` / `biometric`（生物触发）/ `cross_border`（SG 触发）/ `automated_decision` / `third_party_share`；② 勾选后 `POST /api/consent/grant` 拿 `consent_id`；③ `submitKyc` 附 `consent_sensitive_pi_id` / `consent_biometric_id` 等；④ 每 checkbox 必须"用户主动勾选 + 非默认勾选"（PIPL §14 / §29） |
| **HIGH-C8** | OCR / 三方核验调用前 audit_log 共享证据缺失 | `service/kyc/kyc.go:96-100 OCRPort` + service 入口 | 在 `Submit` / `verifyUploads` 调 `ocr.ParseBusinessLicense` / `ocr.VerifyIDName` 前 / 后各写一条 `audit_log_unsealed`（`action='pii.share.aliyun_ocr'` / `target_type='kyc_application'` / `target_id=app.ID` / `diff_redacted='provider=aliyun ocr fields=...'`）；PIPL §23 / §30 共享记录义务 |

### 7.3 MEDIUM（Phase 2A 前 follow-up）

| ID | 主题 | 位置 |
|:---:|---|---|
| **MED-C1** | `model_whitelist` 表 / service / `model_whitelist.review monthly` cron | 新增 migration + service |
| **MED-C2** | admin PIA 后端 endpoint (`listPia` / `generatePia`) | `internal/handler/admin/admin.go` 未挂；前端 `admin.ts` 已调 |
| **MED-C3** | consent_log `consent_text_version` 与 `biz_setting.consent_text_versions` 联动 | service/auth 与 service/kyc 注入 |
| **MED-C4** | PIPL 数据导出 JSON schema 文档化（M-20）| `internal/service/pipl` export 实现时选 JSON + 附 schema |
| **MED-C5** | storefront build-gate 之外的 readiness probe endpoint | `handler/health.go` 扩展 `/healthz/compliance` |
| **MED-C6** | password_reset 的 consent_type 选择注释（NEW-LOW-4 的代码化） | `service/auth/password_reset.go:23` 注释明确 "复用 'privacy_policy'" |
| **MED-C7** | audit_log MySQL Store 接线（替换 `MemoryStore`） | `internal/audit/`（新增 `mysql_store.go`） |

### 7.4 LOW（建议）

| ID | 主题 | 位置 |
|:---:|---|---|
| **LOW-C1** | SMS / 邮件模板主体显示【TraceNex】+ ICP（NEW-LOW-2 代码化） | `service/notification/`（新增 template builder，注入 biz_setting） |
| **LOW-C2** | admin / partner 站 header 放"当前部署合规态" badge（只给 staff） | 非必需 |
| **LOW-C3** | `audit sealer 200ms tick`（`sealer.go:104`）用 time.Ticker，建议注册到统一 cron engine 便于运维 | `cmd/audit-sealer/` |
| **LOW-C4** | piiscrubber regex 扩展 bank card 16~19 位 + 驾驶证 18 位 + 护照 E/G+7~8 位 | `pkg/piiscrubber/scrubber.go` |
| **LOW-C5** | `openapi/admin.yaml` 在 `/content-safety/reports` 等条目加 `x-audit-action` 标记 audit_log 动作名（可被 middleware 读）| openapi 改动 |
| **LOW-C6** | `cmd/notify-dispatcher` 启用 leader-lock（K8s Lease）避免多副本重派 | 同 sealer leader 接口模式 |

---

## 8. §15.10 pre-launch 合规清单代码版 final

### T-60 days（Phase 2A 上线前 60 天；法务 + 代码双轨）

1. 【HIGH-C2】12377 dispatcher cron + AuthorityClient 代码接入（上线 staging）
2. 【HIGH-C3】kyc.purge.cold cron + MySQL repo + KMS ScheduleKeyDeletion + OCR cipher 落点
3. 【HIGH-C4】partner-api PIPL 权利请求 endpoint + 30d SLA cron
4. 【HIGH-C7】customer KYC 前端 7 枚举 consent checkbox
5. 【HIGH-C8】OCR / 三方核验 audit_log 共享证据
6. 【MED-C1】model_whitelist 表 + cron
7. 【MED-C4】PIPL 导出 JSON schema 代码 + 文档
8. 【MED-C7】audit_log MySQL Store 接线
9. 法务出函：算法备案触发判定（integration §1.4 footnote）
10. DPO 任命 + PIA 报告 v1 正文
11. 等保 2.0 二级 IDS/WAF/EDR/备份演练 runbook（ops）

### T-30 days

12. 【HIGH-C1】storefront 合规 build gate + CI 验证
13. 【HIGH-C5】ISV mchid webhook invariant 上 staging 验证
14. 【HIGH-C6】个税 5 档分档 + 银行实名一致性断言 + 41 号公告年报 cron
15. 持牌方合同 + mchid 写入 biz_setting（Q12）
16. 律师定稿用户协议 / 渠道商协议 / 隐私政策 + consent_text_version 锁定
17. 【LOW-C1】SMS / 邮件模板主体备案

### T-7 days

18. readiness probe 真实跑通：6 `compliance.*_active` 全 true；9 公示 key 全非空
19. CI 跑通：storefront build gate；SG region SHOW GRANTS 反断言；`content_safety.dispatch` 冒烟
20. 备份恢复演练 1 轮 + audit 哈希链验证 1 周稳定

### T-1 day

21. PIA 报告签字归档；audit_log 哈希链在生产环境 verify 通过
22. saga retry / dual-control / WebAuthn step-up / ISV mchid invariant 全部 e2e 通过

### Phase 2A T+0

23. 高风险账户（partner_wallet > ¥10k 或 monthly payout > ¥10k）实人比对（M9-04）启用计划锁定（NEW-LOW-3）

---

## 9. 修订指令（汇总，按优先级 / 分工）

**partner-api（Go backend）必改 5 项**：
1. 新建 `cmd/notify-dispatcher/main.go`：注入 `content_safety.Service` + `AuthorityClient` 真实实现（先占位 Aliyun endpoint）；每 60s 调 `DispatchOnce`，单独部署 deployment；leader-lock 防多副本重派
2. 新建 `cmd/kyc-purge/main.go`：每日 04:30 `PurgeCold(100)`；依赖 `repository/mysql/kyc_mysql.go`（新实现 `PurgeColdExpired`：事务内 UPDATE cipher=NULL + OSS DeleteObject + KMS ScheduleKeyDeletion）
3. 新建 `internal/service/pipl/`（消费 `pipl_request` 表）+ `internal/handler/w1a_business.go` 挂 `/api/customer/pipl` GET / POST / `:id` / `:id/download`
4. `internal/service/payment/payment.go:152 VerifyCallback`：加 ISV mchid reject 断言；加 audit_log
5. `internal/service/settlement/engine.go:94-117 ComputeItem`：切换到 `tax_status` 5 枚举；按 41 号公告分档；payout 前加 bank blind_index 断言

**partner-api（Go backend）应改 3 项**：
6. `internal/service/kyc/kyc.go:311-326 verifyUploads`：OCR 调用前后各写 `audit_log_unsealed`（`action='pii.share.aliyun_ocr'`）；同时把 `_, kidO, _ := s.crypto.Encrypt("kyc:ocr", ...)` 的 cipher 字节流回写到 entity（HIGH-C3-1）
7. `internal/handler/admin/admin.go`：新增 `/api/admin/pia` list + generate endpoint（前端已调）
8. 新增 `model_whitelist` migration + service + `cmd/whitelist-review/` monthly cron

**Fy-api（Go OVERLAY）必改 1 项**：
9. `Fy-api/controller/tnbiz_internal/content_safety.go`（新增）：接收 `POST /api/internal/content-safety/event`，写 Fy-api 内部事件队列（outbox 或 HTTP 转发），由 partner-api dispatcher 消费；对应 model 调用拦截器（`middleware/content_safety_preflight.go`）；遵守 `OVERLAY.md` 注释规范 `// Fy-api overlay:`

**前端必改 2 项**：
10. `partner-web-customer/src/pages/Kyc.tsx`：增 7 枚举中的至少 5 个 checkbox（`sensitive_pi` / `biometric`（生物触发）/ `cross_border`（SG 触发）/ `automated_decision` / `third_party_share`）；submitKyc 附 consent_id；同时与 backend `kycSubmitBody.ConsentSensitivePIID` `binding:"required"` 对齐
11. `partner-web-storefront/scripts/preflight-compliance.ts`（新增）+ `package.json` build script 前置；9 keys 任一空字符串则 exit 1

**ops（配置层）必做 2 项**：
12. `biz_setting` 在 staging / prod 填充 9 keys 真实值（非空）+ 6 `_active` flag `true`；配合法务出函节奏
13. KMS 策略：DEK per-scope（`kyc:legal_person_id` / `kyc:bank_account` / `kyc:ocr` / `kyc:legal_person_name` / `kyc:alipay_open_id`）；`ScheduleKeyDeletion PendingWindow=7` 以便 kyc.purge.cold 可回撤

---

## 10. 与 dev doc round-2 verdict 的差异说明

dev doc round-2 verdict = PASS_WITH_NOTES（93% 可追溯性）；本 code round-1 verdict = NEEDS_REVISION（68% 代码可追溯性）。差异解释：

- dev doc 评的是"文档有没有把工程落点写死"，代码评的是"进程能不能跑起来 + contract 有没有端到端闭环"
- 10 张合规 schema 表 + 纯函数 service（12377 dispatcher / KYC encrypt / hash chain sealer / tax compute / payment stub / piiscrubber regex）层面 **已全部落地**；这些是本次 review 的亮点，与 dev doc 一致
- 但 cron 进程 / 真实 repo / 真实 KMS / 真实持牌方 SDK / 前端 consent UI / partner-api PIPL endpoint 断链 —— **这 6 处都是"设计已到位、工程实施未到位"的标准 W1a/W1c 工作量**，不属于设计缺陷
- Phase 1 内测（≤ 5 家 / CN only / 不开票 / 不分账）**不受影响**：上述 HIGH-C1..C8 全部可推到 Phase 2A 60d 窗口闭环
- Phase 2A 商业化上线 = 9 项 hard-gate 必须代码层 8/9 ✅ + C-7 ops 债务 —— 当前代码层 2/9；距离 gap 要 11 个工作项（见 §9）

---

## 11. 最终建议

1. **Phase 1 内测放行**：9 项 hard-gate 在 Phase 1（内测、无分账、无发票、≤ 5 家、CN only）阶段 **不强制代码落点**；当前 W1a/W1b/W1c 产出的 schema + service 纯函数 + admin UI 已覆盖内测所需的"事件可追溯 / PII 不泄漏 / 备案号可展示"三件事。
2. **Phase 2A 门槛写入 `HANDOFF-W1g.md` / Makefile target**：新增 `make compliance-check` 聚合：① 9 keys 非空；② 6 `_active` 全 true；③ 4 个 cron 进程存活（notify-dispatcher / kyc-purge / pipl-sla / whitelist-review）；④ ISV mchid invariant 单测通过；⑤ 41 号公告 5 档单测通过；⑥ blind_index 一致性单测通过；⑦ KYC purge cron e2e；⑧ 12377 dispatch e2e；CI 作为 Phase 2A blocking job。
3. **Security / Architect / Backend Architect 会诊点**：
   - ISV mchid invariant（HIGH-C5）与 Security 的 webhook 签名校验是 **重叠域**
   - 个税 5 档分档（HIGH-C6）与 Backend Architect 的 settlement engine 重构计划 **必须同步**
   - KMS ScheduleKeyDeletion（HIGH-C3）与 Security 的 DEK rotation 90d runbook **必须串联**
   - audit MySQL Store（MED-C7）与 Architect 的多 region 留存 / SLS 6 月联动
4. **合规可追溯性复核**：code round-2 门槛设为 **85%**（= dev doc 93% − 8 pp 工程容差），当前 68%；需要在 W2a 闭环 HIGH-C1~C8 八项后再评。
5. **Verdict 上调路径**：当前 NEEDS_REVISION → 闭环 HIGH-C1 / C2 / C3 / C4 / C7 / C8 六项后可上调为 PASS_WITH_NOTES；HIGH-C5 / C6 由 Security / Backend Architect 主审，Compliance 在合议环节交叉签字即可。

---

## 12. 附录：合规相关文件 / 行号速查

| 文件 | 行号 | 内容 |
|---|---:|---|
| `apps/partner-api/migrations/010_compliance.up.sql` | 5-25 | content_safety_event 表（PRD §3.23）|
| 同上 | 27-45 | content_safety_report 表（24h SLA / 5 态 / 3 authority）|
| 同上 | 47-67 | pia_report 8 大项 |
| 同上 | 69-89 | pipl_complaint（PIPL §50 15d SLA）|
| 同上 | 91-114 | pipl_request（PIPL §44-§47 5 类）|
| `apps/partner-api/migrations/008_ticket_notify_consent.up.sql` | 60-76 | consent_log 7 枚举 |
| `apps/partner-api/migrations/005_kyc_invoice_seat.up.sql` | 5-52 | kyc_application 全字段加密 + cold_archive_expires_at |
| `apps/partner-api/migrations/011_seed_biz_setting.up.sql` | 5-24 | 9 公示 key + 6 _active flag + pia_report_latest_at |
| `apps/partner-api/migrations/001_partner_core.up.sql` | 23 / 42 | partner.tax_status + chk_partner_tax_status |
| `apps/partner-api/migrations/002_wallet.up.sql` | 61 / 65-80 | partner_wallet_log type + partner_debt 表 |
| `apps/partner-api/internal/service/content_safety/content_safety.go` | 37-227 | 12377 dispatcher 全实现 |
| `apps/partner-api/internal/service/kyc/kyc.go` | 252-262 / 328-386 | PurgeCold + encryptInto |
| `apps/partner-api/internal/service/invoice/invoice.go` | 205-245 / 314+ | seller_entity + 10y archive + RedFlush |
| `apps/partner-api/internal/service/settlement/engine.go` | 53-117 | tax 计算（待补 5 档）|
| `apps/partner-api/internal/service/payment/payment.go` | 89-165 | LicensedProvider + HMACStub + VerifyCallback |
| `apps/partner-api/internal/audit/sealer.go` | 32-208 | hash chain sealer + Verify |
| `apps/partner-api/internal/outbox/consumer.go` | 194-229 / 263 | DataRegion 反向断言 |
| `apps/partner-api/pkg/pii/encrypted.go` | 1-49 | PII 信封加密 GORM wrapper |
| `apps/partner-api/pkg/piiscrubber/scrubber.go` | 14-30 | PII regex redact |
| `apps/partner-api/internal/handler/admin/admin.go` | 47-291 | 红冲 / 12377 / staff admin endpoints |
| `apps/partner-api/internal/handler/w1a_business.go` | 169-180 | customer erase（PIPL 右遗忘）|
| `apps/partner-web-storefront/src/components/ComplianceFooter.tsx` | 30-70 | 9 字段 footer |
| `apps/partner-web-customer/src/pages/PiplRights.tsx` | 11-80 | PIPL 权利中心 UI |
| `apps/partner-web-admin/src/pages/PiaPipl.tsx` | 20-172 | PIA 报告 + 投诉受理 |
| `apps/partner-web-admin/src/pages/ContentSafety.tsx` | 12-128 | 12377 列表 + 重试 + 派发 |
| `apps/partner-web-storefront/src/pages/Legal.tsx` | 11-21 | VALID_DOCS 9 类法律文档 |
| `Fy-api/model/consume_log_outbox.go` | 40 / 56-69 / 74-113 | DataRegion + InsertInTx + Lease 三件套 |
| `Fy-api/main.go` | 315-323 | DATA_REGION env 注入 outbox.NewRunner |

---

> 本 review 由 Compliance reviewer 出具，依据 2026 年 5 月有效的中国大陆法律法规（见文首口径表）。Verdict **NEEDS_REVISION** 仅针对"Phase 2A 商业化上线门槛"，**不阻塞 Phase 1 内测启动与 W2a 编码继续推进**。本 reviewer 与 code-reviewer / Security / Architect / Backend Architect 四位 reviewer 平行出稿；最终 round verdict 应在合议环节由主 reviewer 综合判定。
