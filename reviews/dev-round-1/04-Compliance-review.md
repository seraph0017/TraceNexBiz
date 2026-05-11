# Dev Docs Round-1 Review — Legal Compliance

> 日期：2026-05-10
> 审阅人：Compliance reviewer（中国监管 / PIPL / 等保 / 电商法 / 广告法）
> 审阅范围：`docs/00-architecture-overview.md` v0.1（766 行）、`docs/integration-design.md` v0.1（1558 行）、`docs/backend-design.md` v0.1（2704 行）、`docs/frontend-design.md` v0.1（1724 行）
> 权威输入：`prd/PRD-v1.0.md`（2295 行，已签字）；上轮 `reviews/round-2/04-Compliance-review.md`
> Verdict：**NEEDS_REVISION**（CRITICAL = 4 / HIGH = 9 / MEDIUM = 7 / LOW = 5）；**不阻塞 Phase 1 编码启动**，但所有 CRITICAL 必须在 Phase 1 验收（Week 4）前回填到 backend/frontend 设计文档，否则 Phase 2 hard-gate（§22.3 C-1..C-9）将出现"工程已写完但合规无法签字"的死锁。

---

## 1. 执行摘要

上轮我签字 PRD v1.0 的核心理由是 v0.2 → v1.0 把 4 项 BLOCK 全部结构性解掉、3 项 PRE_LAUNCH 留到工程层落地、附录 E 把四类备案分清。**本轮 review 的命题不是"PRD 是否合规"，而是"PRD 已经写明的合规要求在四份开发文档里有没有可验证的工程落点"。**

整体评价：

- **集成层（integration-design）** 在去二清、HMAC scope、Idempotency-Key 透传、saga 3（M2-03 客户充值）的设计上**完全咬合 PRD §7.6 / §15.2**，是四份文档里合规质量最高的。**唯一硬伤是 §3 outbox 表 `consume_log_outbox` 与 §4 saga payload 含 `user_id` / `quota` / `model_name` 的字段（lines 411-427、1043-1055），文档中没有声明这些字段在 SG 启用后是否要做 region-isolated 物理隔离**——而 PRD §15.7 明确要求"Redis / outbox 实例必须 region-isolated"。这个 invariant 在 overview I-2.3 只提了 Redis，没提 outbox 表。
- **后端（backend-design）** 在 audit 哈希链（§3.13 + §10）、KYC 信封加密（§3.9 + §5.6）、PIPL §47 删除（§5.11）三个点把上轮 PRE_LAUNCH 项目落到了 DDL 与时序图。但**生成式 AI 提供者义务的工程闭环只完成了一半**：§4.11 列了 4 个 admin endpoint、§17.2 把 content_safety 列入 Phase 2A，但**没有设计 12377 上报通道的表 / service / endpoint / SLA 计时器**。M12-04 在 PRD 附录 E.4 里写了字段（user_id 脱敏 / prompt_hash / 命中类目 / 处置动作），backend-design 完全没有对应 DDL 与流程。这是 CRITICAL。
- **前端（frontend-design）** 把 §7.9 PIPL 单独同意 UI 写得**比 PRD §15.5 更完整**（双勾选 + ConsentTextVersion + hashlock + 撤回入口，lines 888-920），但**违反"中国互联网信息服务 + 生成式 AI 双备案号公示"义务**：storefront 路由树 §3.1 中只有 `/legal/privacy` / `/legal/terms`（lines 237-239），**没有 footer 备案号公示组件**（ICP 备案号、ICP 经营许可证号、生成式 AI 服务提供者备案号、算法备案号、深度合成备案号、网安局公网安备号、12377 / DPO 联系入口）。这是 CRITICAL（《互联网信息服务管理办法》§8、《生成式 AI 服务管理暂行办法》§17）。
- **架构总览（00-architecture-overview）** 在 §10 风险登记里加了 A-8 "LOG_DB 拆分 region 引发跨境"——很重要——但**没把"备案 / 资质前置"作为可机器检查的工程依赖矩阵**写入 §8 Phase 切片或 §12 测试钩子。也就是说，工程团队从这份文档读不到"哪个 endpoint 在 Phase 2 上线前必须由哪张证 / 哪个备案号阻断"。

**评分**：

| 维度 | 分 | 说明 |
|---|---:|---|
| PRD → 开发文档合规可追溯性 | 6.5 / 10 | §15 各小节大多有引用，但 §15.10 pre-launch 16 项中有 5 项无工程对应（PIA 报告生成、违法内容上报通道、深度合成水印、广告法极限词、未成年人模式）|
| §22.3 Phase 2 hard-gate（9 项）落地 | 5 / 10 | C-3 / C-5 ✅；C-4 partial；C-1/C-2/C-6/C-7/C-9 缺工程闭环 |
| 去二清资金流 | 9 / 10 | 集成层质量高；唯独 invoice 开票主体（§5.8）未明示销售方实体 |
| PIPL 数据生命周期 | 7 / 10 | 采集 → 加密 → 删除链路清晰；**5 年冷归档销毁 cron 缺失**；自动化决策同意类型缺 |
| 算法 / 内容安全 | 4 / 10 | 备案号公示、12377 上报、深度合成水印三大义务前端均无组件、后端均无 service |
| 等保 2.0 二级 | 5 / 10 | 访问控制 / 审计 ✅；入侵防范、备份恢复、恶意代码检测 ✗ |

---

## 2. 资质 × 工程依赖矩阵

> 矩阵规则：每张证 / 备案 → 阻断哪些 endpoint / 模块上线。文档中**应当**把这个表落到 `00-architecture-overview.md` §8 或 backend-design §17 Phase 切片中作为"机器可检查的 feature flag"，但 v0.1 没有。本表是建议增补内容。

| 资质 / 备案 | 主管 | 阻断的工程模块 / endpoint | PRD 引用 | 当前文档落点 |
|---|---|---|---|---|
| 公司主体 ≥ 100 万实缴（Q11.1）| 工商 | 所有商业化 endpoint（Phase 2A 起）| §15.1 + §13 Q11 | ❌ 未在 backend `biz_setting` 中预留 `phase_2_unlock_required` flag |
| 1 年经营 / 30 万社保 / 已备 ICP（Q11.2-4）| 通信管理局前置 | ICP 申请前置；不影响 Phase 1 内测 | §13 Q11.2-Q11.4 | ❌ 工程文档未列 |
| ICP 经营许可证 | 通信管理局 | M2-03（持牌方收单）/ M2-04（线下转账）/ M6-01..09（支付）/ M8-01..06（发票）/ 公开商城商业化文案 | §15.1 + §22.3 C-1 | ⚠️ frontend `/apply-partner` SSR 页 PRD §1.4 标注"Phase 1 内测 ≤ 5 家种子"——但 storefront `/`（line 231）公开商城商业化在 Phase 1 已上线，按字面在 Phase 1 即触发"经营性互联网信息服务"边界。需要 backend feature flag 控制 storefront M1-04/06/09 在 ICP 拿证前**仅展示"招商内测"**而非"价目表 + 直接付款" |
| 生成式 AI 服务提供者备案 | 网信办 | 任何 AI 推理 endpoint（含客户后台 `/customer/api-keys` + `/customer/usage` 与 Fy-api `/v1/chat/completions` 链路）| §15.1 + 附录 E.1 + §22.3 C-2 | ❌ backend / frontend 没有"备案号 → biz_setting → footer/响应 header 显示"链路 |
| 算法备案（若启用排序 / 路由）| 网信办 | Fy-api 内部上游渠道路由（`channel_benchmark` 等）| 附录 E.1 + E.3 | ❌ 既没决定是否触发，也没在 integration-design §1 列入"是否需要 algorithm filing" |
| 深度合成备案 | 网信办 | M12-05（深度合成水印）+ 任何上架的图 / 视频 / 音频生成模型 | §15.3 + 附录 E.1 | ❌ backend `content_safety` 未列水印 watermarker service；frontend chat UI 未列"AI 生成"标识符 |
| 大模型上架白名单 | 网信办按月清单 | `/admin/content-safety/models`（backend §4.11）| §15.1 + §22.3 C-2 | ⚠️ endpoint 已建，但**没有"清单维护人 + 月度对齐 cron + 不在白名单的模型自动下架"机制**（PRD M12-01 已列 P0）|
| 持牌分账方接入 | 央行（持牌方资质）| M2-03/M2-04/M6-01..09/payment webhook | §15.1 + §22.3 C-3 | ✅ integration-design §4.5 + backend §3.7 topup_intent + §5.7 saga 3 |
| 等保 2.0 二级 | 公安 | 所有 prod 部署 | §15.1 + §22.3 C-7 | ⚠️ 部分（见 §6 等保审计） |
| PIA 报告 | 内部 + DPO | 任何 PII 处理（KYC、单独同意、跨境）| §15.1 + §22.3 C-6 | ❌ 文档无 PIA 报告生成器 / 模板 |
| CAC 标准合同（SG）| 网信办备案 | SG region 任何 user / customer / partner 数据流入 | §15.7 + §22.3 N/A | ⚠️ overview §6 SG 列已标注"重新评估"；缺"启用前 4 周冻结 SG endpoint"feature flag |
| 微信 / 支付宝 ISV | 微信 / 支付宝 | M6-03（微信/支付宝走 ISV）| §15.1 | ⚠️ integration-design §4.5 与 backend §5.7 设计正确，但 mchid 配置未在 biz_setting 列出 |
| 反洗钱客户身份识别 | 央行 | KYC 模块 + STR 流程 | §15.1 | ⚠️ kyc 表 ✅；STR 流程在 PRD §22.4 M-2 推后，文档无 |
| DPO 任命 + 公示 | 内部（PIPL §52）| frontend footer + backend `/api/legal/dpo` | §15.1 + §22.3 C-8 | ❌ frontend 路由树无 DPO 入口；backend §4.12 PIPL endpoint 列了 5 个但缺 DPO 通道 |
| 个税代扣代缴方案 | 财务 + 税务 | M5-10 settlement_item.WithheldTax + 41 号公告报送 | §15.4 + §22.3 C-4 | ⚠️ DDL ✅（§3.7 line 493-499），**service 端税率分档 + 年度报送 cron 缺失** |
| 全电发票对接 | 国税总局 | M8-01..06 | §15.1 + §22.3 C-5 | ✅ backend §5.8 流程；⚠️ 销售方主体 / 留存周期 10y 未明 |

**修订指令 M-1**（CRITICAL）：在 `00-architecture-overview.md` §8 Phase 切片表后追加一节 **§8.5 资质 × 模块 gating**，把上表落地，并要求 backend `biz_setting` 增加 `compliance.icp_license_active` / `compliance.gen_ai_filing_active` / `compliance.algorithm_filing_active` / `compliance.deep_synthesis_filing_active` / `compliance.epd_2_filing_active` 五个 boolean flag；CI 在 Phase 2A 上线前自动断言这五项全为 true，否则 readiness probe 不通过。

---

## 3. §22.3 Phase 2 hard-gate（9 项）落地核查表

| ID | 主题 | 状态 | 评估 + 引用 |
|:---:|---|:---:|---|
| C-1 | ICP 经营许可证拿证 | ❌ | 文档级提及（overview §10 A-8 / backend §17.2 / frontend §3.1）但**无 feature flag、无 endpoint-level 阻断**。frontend `/apply-partner` 与 storefront `/pricing` 在 Phase 1 SSR 上线（frontend lines 231-244），缺 ICP 前的"内测旗"。 |
| C-2 | 生成式 AI 提供者 + 算法备案核准 | ❌ | backend `/admin/content-safety/models` whitelist endpoint 存在（line 1294），但**没有"备案号 → footer 显示 → 响应 header 注入"链路**。frontend storefront 无 footer 备案号组件。 |
| C-3 | 持牌分账方上线运行（含 mchid ISV） | ✅ | integration-design §4.5 saga 3 + backend §3.7 topup_intent + §5.7。`partner_wallet` 语义重定义贯彻到 DDL（§3.3 line 326 注释 "应付台账"）。 |
| C-4 | 个税代扣 + 跑通月结 | ⚠️ | settlement_item.WithheldTax / TaxEvidenceUrl 字段 ✅（line 493-499）；§5.5 流程描述了 `compute platform_fee, withheld_tax`；但**计算函数 `ComputeWithheldTax(NetBeforeTax, kyc_type, tax_status)`（PRD §15.4）缺实现**；`partner` 表缺 `tax_status` 字段（v0.2 review INFO 项升级为 HIGH，因为 §15.4 实际需要区分 individual / sole_proprietor / individual_business / company 才能选择不同税率链路）；**41 号公告年度报送 cron 缺失**。 |
| C-5 | 全电发票打通 | ⚠️ | backend §5.8 ✅；invoice_application schema ✅。**但缺：①开票主体（销售方）未在 schema 中明示——`invoice_application` 没有 `seller_entity_id` / `seller_tax_no`，而《电子发票管理办法》§7 要求每张发票必须明示销售方主体。**②留存周期：电子会计档案应 ≥ 10 年（《会计档案管理办法》§14），但文档中所有 KYC / 财务字段保留期均按 5 年讨论，**缺发票 / 银行流水 10 年保留计划**。③红冲：状态 'red_flushed' 仅一个枚举（§3.12 line 675），缺红字发票号关联、红冲申请单（红冲必须经过税局《信息确认单》流程，建议加 `red_flush_request` 表）。 |
| C-6 | PIA 报告留档 | ❌ | **完全缺失**。backend / frontend 无 PIA 报告生成器、无 PIA 模板、无 audit_log 标注 PIA 事件。PIPL §55 + GB/T 39335-2020 要求的 PIA 8 大项（处理目的、范围、必要性、合法性基础、影响、风险、措施、剩余风险）在文档中无对应数据结构。 |
| C-7 | 等保 2.0 二级备案 | ⚠️ | 详见 §6 等保审计章节。访问控制（GRANT 表 ✅）/ 安全审计（哈希链 ✅）落地良好；入侵防范（IDS / WAF）/ 恶意代码检测 / 备份恢复测试 / 数据完整性校验 cron / 集中管控等不达标。 |
| C-8 | DPO 公示 + 用户权利中心 §7.13 上线 | ⚠️ | backend §4.12 / frontend `/pipl-rights` ✅（M13-01..05 端点齐）；**DPO 联系入口缺失**——frontend storefront 路由树（lines 230-244）+ portal footer 均无 `/legal/dpo`、no `/legal/complaint`；backend 无 `pipl_complaint` 表 / endpoint。 |
| C-9 | 内容安全双层审核 + 12377 上报通道闭环 | ❌ | M12-02 输入 / M12-03 输出在 backend §17 Phase 表标注（line 124）但 §3 DDL **没有 `content_safety_event` 表的字段定义**（§17.2 line 2453 仅一行提及，无 schema）；**12377 上报：完全缺失 service / cron / endpoint / SLA 计时器**——PRD 附录 E.4 已写明字段，backend 应补 `content_safety_report` 表 + 24h SLA cron。 |

---

## 4. 资金流去二清审计（4 条链路 trace）

> 监管框架：央行《非银行支付机构监督管理条例》§3 / §10、《关于规范支付创新业务的通知》（银发〔2014〕5 号）、央行 2017《支付机构客户备付金管理办法》。

### 链路 1：客户充值（M2-03 持牌方收单）

```
客户浏览器 → POST /customer/topup-intent → backend INSERT topup_intent (state='created', amount, partner_id, customer_id)
                                         → call 持牌分账方 PrepareOrder(out_trade_no=intent.id)
                                         → 客户重定向到持牌方收银台付款 ★ 钱在此进入持牌方备付金池，绕开 partner-api ★
持牌方 webhook → POST /webhook/payment/{provider}（HMAC + IP allowlist）
              → verifySignature + amount cross-check + (channel, out_trade_no) UNIQUE
              → UPDATE topup_intent.state='paid'
              → call Fy-api POST /api/internal/user/topup (Idempotency-Key=intent.id)
              → UPDATE topup_intent.state='funded'
```

**审计结论**：✅ 合规。链路全程客户付款不进 TraceNex 主体账户，与 PRD §7.6 / §15.2 / ADR-002 完全一致。`partner_wallet` 在本链路**不参与**（backend §5.7 line 1664）。

**残留风险（HIGH）**：
- 平台主体作为 ISV 服务商收取的"佣金"——backend / integration 无 schema 字段 `platform_isv_commission_account_no`，无法在审计期证明"平台 mchid 仅作 ISV 佣金接收"（上轮 BLOCK #1 残留）。**修订指令 M-2**：backend `biz_setting` 增 `payment.platform_isv_mchid` 字段并在 §5.7 加 invariant：所有 webhook 收款方 mchid 等于 isv_mchid 时拒收（应该走持牌方分账，不走平台直收）。

### 链路 2：渠道商分润 / 应付台账

- backend §5.5 settlement_runner → settlement_item → 持牌方 Payout → `partner_wallet_log (settlement_payout, amount=-payout)` ✅
- **审计结论**：合规。`partner_wallet.balance` 是应付台账（line 326 注释），不是沉淀客户付款。
- **HIGH 残留**：backend `partner_wallet_log` 类型枚举（line 363）缺 `platform_isv_commission_in`（平台主体作为 ISV 收到的佣金的台账）—— 这是反向流水，与渠道商分润对偶，应该有独立 type 以便财务对账。

### 链路 3：退款

- integration-design §4.4 ✅ 三分支处理（未结算 / 已结算未支付 / 已支付）；backend §5.10.2 implement ✅
- **MEDIUM 残留**：已支付场景下 `balance go negative is OK` （integration line 1182），F-2 决策推迟到 Phase 2B。**当前文档默认"负 balance"——这意味着平台事实上对渠道商形成"应收"。在持牌方语境下，这相当于"平台作为放款人"，可能被解读为"未持牌经营借贷"**。建议在 backend §3.16 `partner_debt` 表（已在 line 968-985 起草）的 P0 化，并在退款 service 中默认走 partner_debt 路径而非负 balance；负 balance 仅在 P0 期 fallback 且必须有阈值告警。**修订指令 M-3**：把 partner_debt 从 Phase 2B 上调到 Phase 2A，配合首次退款上线。

### 链路 4：提现 / 下账

- backend §5.5 settlement_item → 持牌方 Payout → `provider_trade_no` 留档（line 1568）✅
- **HIGH 残留**：提现到渠道商个人银行卡（个人渠道商场景）需触发个税代扣 + 41 号公告报送。当前 settlement_item 已含 WithheldTax，但**银行卡核验 / 与 KYC 实名一致性校验**未在 service 中体现。建议在 §5.5 加一步 "Payout 前 assert 银行账户实名 == kyc_application.legal_person_name"，否则容易被税局认定为"以他人账户走帐 / 虚开"。

---

## 5. PIPL 数据生命周期审计

完整链路：**采集 → 同意 → 存储 → 加密 → 共享 → 跨境 → 删除/导出**

### 5.1 采集（C - Collection）

- frontend `PartnerEnterpriseSchema`（lines 665-690）✅ 单独标注 PII / 敏感（lines 672-673 注释）
- ⚠️ **MEDIUM**：身份证号 / 法人姓名直接走 zod 校验落到客户端 form state，frontend §9.4 line 1057 "form draft sessionStorage 离开 form 自动 clear"——但 sessionStorage 在 form 提交前可被同源 JS 读取，建议 KYC 表单的 PII 字段使用 `useState` 内存而非 react-hook-form 的 form state（避免 react-hook-form 默认序列化到 dev-tools）。

### 5.2 同意（Consent）

- frontend §7.9 双勾选 UI ✅
- backend `consent_log.consent_type` CHECK 枚举（line 904）：`'privacy_policy','sensitive_pi','biometric','cross_border','device_fingerprint'` —— **缺两类**（HIGH）：
  - `automated_decision`（自动化决策同意，PIPL §24）—— 智能模型路由 + 风控判定均触发
  - `third_party_share`（向阿里云内容安全 / 腾讯天御 / OCR 服务商共享个人信息，PIPL §23）

**修订指令 M-4**：backend §3.18 `consent_log.chk_consent_type` CHECK 枚举追加 `'automated_decision','third_party_share'`；frontend §7.9 单独同意 UI 增 2 个 checkbox；附录 A 隐私政策第 5 章对应章节落实。

### 5.3 存储 + 加密

- backend §3.9 KYC 表所有 PII 字段走 VARBINARY + KMS DEK（lines 551-562）✅
- backend §9 信封加密 + DEK rotation 90d ✅
- ⚠️ **MEDIUM**：`audit_log_pii.diff_cipher VARBINARY(8192)`（line 730）——8KB 上限对法人身份证图片 OCR 结果（含全文）可能不够；建议提升至 65535 或将"重 PII"放 OSS 引用。

### 5.4 共享

- ⚠️ **HIGH**：backend `kyc.Service` 调阿里云 OCR（§5.6 line 1607）+ content_safety 调阿里云内容安全 / 腾讯天御——这两条都属于 PIPL §23 "向第三方提供个人信息"，**必须 ① 与每家服务商签 DPA、② 在隐私政策中列名、③ 单独同意（M-4 修订指令的一部分）、④ 在 audit_log 中留 trace**。当前文档无 ①③④ 工程落点。

**修订指令 M-5**：backend `kyc.Service.OCR` / `content_safety.Service.Audit` 入口 service 强制写 audit_log（action='pii.share.aliyun_ocr' / 'pii.share.tencent_yuce'）+ assert consent_log 中 third_party_share 同意有效。

### 5.5 跨境

- overview §6 / §10 A-8 ✅；integration-design 隐含
- ❌ **CRITICAL**：`consume_log_outbox` schema（integration line 411-427）含 `user_id` / `quota` / `model_name` / `trace_id`——`user_id` 是 PIPL 一般 PI（不是敏感），`model_name` 在某些医疗 / 法律场景下可能间接揭示用户用途——SG 启用前必须确认这张表**物理 region-isolated**。当前 overview I-2.3 / A-8 仅约束 Redis；outbox 表无 CI gate。

**修订指令 M-6**（CRITICAL）：integration-design §6.1 GRANT 表追加一行："SG region 启用后，CN 实例 `tnbiz_outbox_consumer` 在 SG `LOG_DB` 上**完全无权**；CI 在 deploy 后断言 `SHOW GRANTS FOR 'tnbiz_outbox_consumer'@'%' ON sg_log_db.*` 返回空"；overview §10 A-8 增加 outbox 维度。

### 5.6 删除（PIPL §47）+ 导出（§44-46）

- backend §5.11 删除流程 ✅；frontend §3.2 portal `/pipl-rights/*` 5 个端点 ✅
- ❌ **CRITICAL**：**5 年冷归档销毁 cron 缺失**。backend §6 cron 清单（lines 1833-1847）有 `kyc.purge.hot`（30d 热删）但**无 `kyc.purge.cold`（5y 冷桶销毁）**。PRD §15.6 明确"5 年后不可逆删除"。如果不实现，5 年后 OSS Archive 桶会持续累积，等到运营第 6 年才发现合规漏洞。
- ⚠️ **MEDIUM**：删除流程 §5.11 line 1818 调 `KMS.ScheduleKeyDeletion(... 5y)`——KMS 把 DEK 在 5 年后销毁可以让密文不可解，但 OSS Archive 桶里的密文文件本身仍在，这是事实上的"可重新加密恢复"风险；建议同步 OSS Archive lifecycle 5y 自动 expiration。

**修订指令 M-7**（CRITICAL）：backend §6 cron 表增 `kyc.purge.cold daily 04:30 / 5y 边界`；同时在 OSS bucket 配置 lifecycle expiration 5y（ops runbook，不止 cron）。

### 5.7 导出

- frontend `/pipl-rights/data-portability`（line 270）+ backend `/customer/pipl/data-access-request`（line 1300）✅
- ⚠️ **MEDIUM**：导出格式应为机器可读（PIPL §45）；文档未明示。建议 JSON + 关键字段 schema 文档化。

---

## 6. 算法 / 内容安全审计

> 法律依据：《生成式 AI 服务管理暂行办法》§4 / §7 / §10 / §14 / §17；《算法推荐管理规定》§24；《深度合成管理规定》§16-17、§19。

### 6.1 备案号公示（CRITICAL）

❌ **frontend storefront 路由树（lines 230-244）无 footer 备案号公示组件**。

按以下法规义务必须公示：
- 《互联网信息服务管理办法》§8：**ICP 备案号** + **ICP 经营许可证号**（前者所有网站，后者经营性站点）
- 《计算机信息网络国际联网安全保护管理办法》：**网安局公网安备号**
- 《暂行办法》§17：**生成式 AI 服务提供者备案号**
- 《算法推荐管理规定》§16：**算法备案号**
- 《深度合成管理规定》§19（如适用）：**深度合成服务备案号**
- PIPL §52：DPO 联系方式
- 《暂行办法》§14：**违法内容举报通道（12377 + 公司专用渠道）**

**修订指令 M-8**（CRITICAL）：frontend `packages/ui-kit` 增 `<ComplianceFooter>` 组件，挂载到 storefront / portal / admin 三个 app 的全局 layout；后端 `biz_setting` 增 `compliance.icp_record_no` / `compliance.icp_license_no` / `compliance.public_security_filing_no` / `compliance.gen_ai_filing_no` / `compliance.algorithm_filing_no` / `compliance.deep_synthesis_filing_no` / `compliance.dpo_contact_email` / `compliance.dpo_contact_phone` / `compliance.report_phone_12377_link` 共 9 个 key。CI 在 prod readiness probe 中断言这 9 项均非空。

### 6.2 模型白名单维护（HIGH）

- backend `/admin/content-safety/models` whitelist endpoint ✅（line 1294）
- ❌ **缺月度对齐网信办清单的流程**——上轮 review 已点出 "谁负责每月对齐网信办第 N 批清单？"；当前 backend §6 cron 表无对应 job。

**修订指令 M-9**：backend §6 增 `model_whitelist.review` cron（monthly），其工作是从合规人员维护的"网信办最新一批清单 URL"拉取，diff 当前 enabled 模型，不在清单的自动 disable 并发 ops 工单。

### 6.3 输入 / 输出审核（HIGH）

- backend §17.2 列入 Phase 2A `content_safety` service（line 2588）但**§3 没有 `content_safety_event` 表 DDL**——PRD §22.1 没把这张表 freeze，但 frontend `/admin/content-safety/events`（line 1291）已经在调用。

**修订指令 M-10**（HIGH）：backend §3 增 `content_safety_event` 表，字段：`id, fy_user_id, kind('input'|'output'), provider('aliyun'|'tencent'), prompt_hash CHAR(64), category VARCHAR(64), score DECIMAL(5,4), disposition VARCHAR(32), reviewed_by, reviewed_at, reported_to_12377_at, audit_log_id`。

### 6.4 12377 / 公安网安上报（CRITICAL）

❌ **完全缺失**。PRD 附录 E.4 已写字段表，但 backend / integration / frontend 无 service、无 endpoint、无 SLA 计时器。

**修订指令 M-11**（CRITICAL）：backend 增 `content_safety_report` 表 + `content_safety.Reporter` service（24h SLA cron + 字段对齐附录 E.4）；admin frontend `/admin/content-safety/reports` 提供"已上报记录"+"应上报但超期"的看板；audit_log 类型增 `content_safety.report.submitted`。

### 6.5 深度合成水印（HIGH）

❌ frontend chat UI / image gen UI 无水印组件；backend 无 watermarker service。

**修订指令 M-12**：若 Phase 2A 有任何深度合成（图 / 视频 / 音频）模型上架，必须在响应通道中插入"AI 生成"显式水印。当前 PRD §1.3 说"Phase 1-2 仅文字 LLM"——**建议在 backend `model_whitelist` 表加 `is_deep_synthesis` 列，true 时强制要求水印 service 已就位才能 enable**。

### 6.6 算法备案对象（MEDIUM）

PRD 附录 E.3 说算法备案对象是"模型路由 / 渠道分发"——但 integration-design §1 没有把"渠道分发是否构成《算法推荐管理规定》§2 第（1）项的'信息合成 / 排序'"写明。如果是，那么 Fy-api 上游的 channel_benchmark / 健康度路由全部需要算法备案；如果不是，仍需要在文档里说明否定理由。

**修订指令 M-13**：integration-design §1.4（C-4）增 footnote 说明渠道路由不对用户内容做个性化排序、不基于用户画像做差别推送，因此**不**触发《算法推荐管理规定》§2 第（1）项 "个性化推送"——但仍触发第（5）项"调度决策"，需做算法备案。

---

## 7. CRITICAL 合规缺口清单（不修不能上线）

> 对应 §22.3 hard-gate；任一未关闭则 Phase 2A 不得开工。

1. **CRIT-1：备案号公示（M-8）**——frontend 无 ComplianceFooter；backend `biz_setting` 无 9 个备案 key。**违反《互联网信息服务管理办法》§8 + 《暂行办法》§17。**
2. **CRIT-2：12377 / 公安网安上报通道（M-11）**——backend 无 service / 表 / cron。**违反《暂行办法》§14。**
3. **CRIT-3：5 年冷归档销毁 cron（M-7）**——backend §6 cron 表缺 `kyc.purge.cold`。**违反 PIPL §47 + PRD §15.6。**
4. **CRIT-4：outbox 跨境隔离 invariant（M-6）**——integration-design §6 GRANT 表无 SG 维度断言；CI 无对应 gate。**违反 PIPL §38 + 《数据出境安全评估办法》。**

---

## 8. HIGH 合规缺口

5. **HIGH-1：partner.tax_status 字段 + ComputeWithheldTax 实现 + 41 号公告年度报送 cron**（C-4 关闭依赖；§15.4）
6. **HIGH-2：自动化决策 + 第三方共享 单独同意类型（M-4）**——consent_log CHECK 枚举缺两类（PIPL §23 / §24）
7. **HIGH-3：第三方 PII 共享审计（M-5）**——OCR / 内容安全调用未走 audit_log
8. **HIGH-4：模型白名单月度对齐（M-9）**——cron 缺失（PRD M12-01）
9. **HIGH-5：content_safety_event 表 DDL（M-10）**
10. **HIGH-6：发票销售方主体 + 10y 留存（C-5 partial）**——`invoice_application` 缺 `seller_entity_id` / `seller_tax_no`；OSS lifecycle 无 10y 计划
11. **HIGH-7：PIA 报告生成器（C-6）**——文档完全无对应工程
12. **HIGH-8：DPO 联系入口 / PIPL 投诉受理通道（C-8 partial）**——frontend footer + backend `pipl_complaint` 表无
13. **HIGH-9：partner_debt 表上调到 Phase 2A（链路 3 残留）**——避免负 balance 被解读为"未持牌经营借贷"

---

## 9. MEDIUM / LOW 缺口

**MEDIUM**：
14. KYC 表单 PII 字段建议从 react-hook-form 内存改为 useState 内存（XSS 隔离）
15. audit_log_pii.diff_cipher 8KB 上限可能截断 OCR 结果
16. PIPL 导出格式（JSON + schema 文档化）
17. 红冲申请单（红字发票号关联 + 红冲申请 entity）
18. partner_wallet_log 类型枚举增 `platform_isv_commission_in`
19. Settlement Payout 前银行账户实名一致性校验
20. storefront /pricing 在 ICP 拿证前应仅显示"招商内测"

**LOW**：
21. 平台 mchid 写入 biz_setting + invariant
22. KMS ScheduleKeyDeletion 与 OSS lifecycle 5y 一致性
23. 算法备案否定理由（M-13）
24. 防沉迷 / 老年人模式（暂可推到 v1.x）
25. 屏幕水印 hash 写 audit_log（frontend §9.3 line 1051 已写 ✅，但需 backend 配合接入）

---

## 10. 等保 2.0 二级落地审计（C-7）

| 等保 2.0 项 | 依据 | 文档落点 | 状态 |
|---|---|---|---|
| 安全物理环境 | GB/T 22239 §7.1.1 | overview §6 阿里云 VPC + Aliyun ACK | ✅（继承云厂商） |
| 安全通信网络 | §7.1.2 | overview §2.2 流量分类（mTLS / TLS 1.3） | ✅ |
| 安全区域边界 | §7.1.3 | overview §2 K8s VPC + Istio sidecar | ⚠️（无 IDS / WAF 条款）|
| 安全计算环境 | §7.1.4 | backend §3 GRANT 矩阵 + KMS 信封 | ✅ |
| 安全管理中心 | §7.1.5 | — | ❌（无集中权限管控 / 集中审计平台条款） |
| 入侵防范 | §7.1.4.5 | — | ❌（无 IDS / IPS / WAF 设计） |
| 恶意代码防范 | §7.1.4.6 | — | ❌（容器镜像扫描 / 节点 EDR 缺） |
| 数据完整性 | §7.1.4.8 | audit hash chain ✅ | ✅ |
| 数据备份恢复 | §7.1.4.9 | overview §6 "RDS 备份" | ⚠️（无定期恢复演练） |
| 安全审计 | §7.1.4.7 | audit_log 哈希链 + GRANT | ✅；**留存周期 ≥ 6 个月（网安法 §21）未明示** |
| 个人信息保护 | §7.1.4.11 | KYC 信封加密 + PIPL 删除 | ✅ |

**修订指令 M-14**：overview §11 文档约定增 "等保 2.0 二级映射节"；ops runbook 增 IDS / WAF / 镜像扫描 / 备份恢复演练四项；audit_log + SLS 留存最少 6 个月在 §6 环境矩阵明示。

---

## 11. §15.10 pre-launch 清单 v0.1 → v0.2 演进建议

| 清单项（PRD §15.10） | v0.1 工程文档落点 | v0.2 建议（本轮新增） |
|---|---|---|
| 持牌分账方上线 | ✅ integration §4.5 + backend §5.7 | — |
| ICP 经营许可证拿证 | ⚠️ 仅 overview §10 A-3 风险 | + biz_setting flag + readiness gate（M-1）|
| 生成式 AI 备案 | ❌ | + ComplianceFooter（M-8）+ readiness gate |
| 算法备案 | ❌ | + 备案号 key + 否定理由（M-13）|
| 大模型白名单 | ⚠️ endpoint ✅, cron ❌ | + monthly cron（M-9）|
| 个税方案 + 系统嵌入 | ⚠️ DDL ✅, service ❌ | + ComputeWithheldTax + tax_status（HIGH-1）|
| 全电发票 | ⚠️ schema ✅, 主体 ❌ | + seller_entity_id + 10y 留存（HIGH-6）|
| 律师定稿协议 | ❌ | 文档 + 律师签字流程 |
| PIA 报告 | ❌ | + PIA 模板 + 生成器（HIGH-7）|
| consent_log | ✅ + 2 类增补 | + 自动化决策 / 第三方共享（M-4）|
| KYC 全流程 | ⚠️ 30d ✅, 5y ❌ | + cold purge cron（CRIT-3）|
| CAC 标准合同 | ⚠️ Redis ✅, outbox ❌ | + outbox region invariant（CRIT-4）|
| 等保 2.0 二级 | ⚠️ 部分 | + 等保映射节（M-14）|
| DPO 任命 + 公示 | ❌ | + ComplianceFooter + 投诉表（HIGH-8）|
| 内容安全双层 | ⚠️ Phase 2A 标注；缺 schema | + content_safety_event（HIGH-5）|
| 违法内容上报 | ❌ | + content_safety_report + 24h SLA（CRIT-2）|
| 深度合成水印 | ❌ | + watermarker service / 强制 gate（HIGH-12 暂列 LOW，看 Phase 2 模型）|

---

## 12. 修订指令汇总（哪份文档哪节加什么条款）

| ID | 等级 | 目标文档 / 章节 | 内容 |
|---|---|---|---|
| M-1 | CRIT | overview §8 / backend §13.1 biz_setting | 增 §8.5 资质 × 模块 gating；增 5 个 compliance.* flag；readiness probe gate |
| M-2 | HIGH | backend §13.1 biz_setting + §5.7 invariant | 增 `payment.platform_isv_mchid`；webhook 收款方 mchid == isv_mchid 时拒收 |
| M-3 | HIGH | backend §17.2 / §3.16 partner_debt | partner_debt 表从 Phase 2B 上调到 Phase 2A；退款 service 默认走 partner_debt |
| M-4 | HIGH | backend §3.18 chk_consent_type / frontend §7.9 / 附录 A 第 5 章 | consent_type 枚举增 `automated_decision` / `third_party_share`；UI 各加 1 个 checkbox |
| M-5 | HIGH | backend §5.6 / §4.11 service 入口 | OCR / 内容安全调用前 assert 同意 + audit_log（pii.share.*） |
| M-6 | CRIT | integration §6.1 GRANT / overview §10 A-8 | 增 SG outbox region-isolated invariant + CI gate |
| M-7 | CRIT | backend §6 cron 表 + ops OSS lifecycle | 增 `kyc.purge.cold` daily / OSS Archive 5y expiration |
| M-8 | CRIT | frontend ui-kit / backend biz_setting | `<ComplianceFooter>` 组件 + 9 个 compliance.* key + readiness gate |
| M-9 | HIGH | backend §6 cron | 增 `model_whitelist.review` monthly cron |
| M-10 | HIGH | backend §3 增表 | `content_safety_event` 表 DDL（字段见 §6.3） |
| M-11 | CRIT | backend §3 增表 + §6 cron + §4.11 endpoint + frontend admin | `content_safety_report` 表 + 24h SLA cron + `/admin/content-safety/reports` 看板 |
| M-12 | HIGH(条件) | backend / frontend Phase 2A 上线前 | 若有深度合成模型，watermarker service + biz_setting gate |
| M-13 | MED | integration §1.4 footnote | 算法备案否定 / 触发理由说明 |
| M-14 | MED | overview §11 + ops runbook | 等保 2.0 二级映射 + IDS/WAF/EDR/备份演练；6 月留存声明 |
| M-15 | HIGH | backend `partner` schema + §5.5 service | 增 `partner.tax_status enum` + `ComputeWithheldTax(net, kyc_type, tax_status)` + 41 号公告年度报送 cron |
| M-16 | HIGH | backend §3.12 invoice_application | 增 `seller_entity_id` + `seller_tax_no`；电子档案留存 10y 在 ops lifecycle |
| M-17 | HIGH | frontend storefront `<ComplianceFooter>` 内 + backend `pipl_complaint` 表 | DPO 邮箱 + 投诉受理 endpoint + audit |
| M-18 | HIGH | backend 增 `pia_report` 表 + §6 周期任务 | PIA 8 大项模板 + 年度生成 + 留档 ≥ 3 年 |
| M-19 | MED | backend §3.13 audit_log_pii | diff_cipher 提到 65535 或将"重 PII"放 OSS 引用 |
| M-20 | MED | frontend `/customer/pipl/data-portability` | 导出格式 JSON + schema 文档化 |

---

## 13. Recommendation

**Verdict：NEEDS_REVISION**。

四份开发文档相对 PRD v1.0 的合规可追溯性平均 **65%**。**集成层（integration-design）合规质量最高（90%），后端（backend-design）70%，前端（frontend-design）60%，架构总览（00-architecture-overview）50%——主要因为它没有把 §22.3 hard-gate 和资质矩阵下沉为可机器检查的 invariant。**

不阻塞 Phase 1 开发启动，但 **Phase 1 验收前（Week 4）必须关闭 4 个 CRITICAL**（M-6 / M-7 / M-8 / M-11），否则 Phase 2A 商业化 hard-gate（C-1..C-9）将出现"工程已写完但合规无法签字"的死锁。**Phase 2A 上线前必须关闭 9 个 HIGH**。

下一步给四份文档维护者：

1. **Architect**（overview）：合入 §8.5 资质 × 模块 gating（M-1）+ §11 等保映射节（M-14）+ §10 A-8 outbox 维度（M-6 配合）。
2. **Architect**（integration）：合入 §1.4 算法备案 footnote（M-13）+ §6.1 GRANT SG region 维度（M-6）。
3. **Backend**：合入 4 张新表（content_safety_event / content_safety_report / pia_report / pipl_complaint）+ 5 个新 cron（kyc.purge.cold / model_whitelist.review / content_safety.report.dispatcher / pia.report.annual / tax.report.annual）+ partner.tax_status 字段 + ComputeWithheldTax service + invoice_application.seller_entity_id 字段 + biz_setting 9 个 compliance.* key。
4. **Frontend**：合入 `<ComplianceFooter>` 组件（M-8）+ §7.9 同意 UI 增 2 个 checkbox（M-4）+ portal 增 `/legal/dpo` / `/legal/complaint` 路由（M-17）+ admin 增 `/admin/content-safety/reports` + `/admin/pia` 路由。

外部依赖（不属于工程团队，但必须并行推进，否则上述工程合入也无意义）：

- Q11.1-Q11.4 公司主体前置 4 项核查（PRD §13）
- Q12 持牌方选定（Phase 2A 上线前）
- Q13 DPO 任命（Phase 1 内）
- Q14 算法备案文本定稿（Phase 1 内）
- 律师定稿用户协议 / 渠道商协议 / 隐私政策（Phase 1 内）

建议本轮 review 修订完成后开 dev round-2（≤ 2 周），届时如本表 CRITICAL = 0、HIGH ≤ 4，可签字进入 Phase 1 实施期。

---

> 本 review 由 Compliance reviewer 出具，依据 2026 年 5 月有效的中国大陆法律法规及网信办、央行、国家税务总局、公安部、国务院最新规章。
> 本轮通过即代表四份开发文档可进入 v0.2；Phase 2 商业化上线门槛见 PRD §22.3 hard-gate。
