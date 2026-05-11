# Fy-api Round-3 闭门确认 v1.2 定稿

**日期**：2026-05-14
**审核范围**：v1.2 三份文档（integration-design / 00-architecture-overview / backend-design）相对 v1.1 的 4 项 cosmetic 修订是否真闭环 + 架构师自标 2 个 LOW 风险点
**审核人**：Fy-api Tech Lead
**前置**：Round-1 ACCEPT-WITH-CHANGES（4 CRITICAL / 5 HIGH / 7 MEDIUM）/ Round-2 ACCEPT（0 C / 0 H，4 项 cosmetic 残余）

---

## 1. 执行摘要 + 最终 verdict

v1.2 4 项 cosmetic 全部按 round-2 §7 表逐条落地，文字精度高于预期（不仅添注释，还把 §14.1.1 / §16.1 写成完整决策框架），未见"敷衍式贴标签"。架构师自标的 2 个 LOW 措辞偏弱但风险残量在可接受区间。无新引入问题，无遗漏。

**最终 verdict：FINAL ACCEPT — v1.2 定稿，可进入 Phase 1 PR-1 启动决策。**

---

## 2. 4 项 cosmetic 闭环逐条核查

### Cosmetic 1（flag 热加载语义）— ✅ 完整闭环

- `integration-design.md` §14.1（行 2122-2128）OVERLAY_INTERNAL_API / GROUP_RATIO_OVERRIDE / OUTBOX 行末注入路径全部追加"polling SyncOptions 5-15s 兜底"语义。
- §14.1.1（行 2130-2143）新增独立小节，5 条澄清覆盖：① polling 不是 push ② PR-3 上线后才有 < 200ms ③ Phase 1 中间窗口"全 false 影子模式可承受"业务理由 ④ PR 拆分依赖（PR-3 不是 PR-2 前置但建议不晚于 1 周）⑤ 不支持 SIGHUP。**第 ④ 条尤其重要 —— 把"PR-3 不是 PR-2 前置"写在框架级，避免被误读为 PR-2 阻塞 PR-3。**
- §15.1（行 2190）PR 概览表末尾备注重复了"PR-3 不晚于 PR-2 prod 首发后 1 周"硬约束。

> 比 round-2 推荐补强一句话注释**做得更彻底**。✅

### Cosmetic 2（gh-ost RDS 8.0 trigger 权限 fallback）— ✅ 完整闭环

`integration-design.md` §1.3.4（行 391-432）四段全齐：
1. 权限预检 SQL（CREATE/DROP TRIGGER 实测，非只查 GRANT）—— ✅ 比 round-2 推荐严
2. F-1..F-4 fallback 优先级表（DBA 临时授权 / DMS 无锁 / DTS 切流 / pt-osc）—— ✅
3. 监控限速对比 + DMS staging dry-run 接近 prod 峰值的硬要求 —— ✅
4. 强制流程：staging dry-run 必须先跑 + CN/SG 各一份《gh-ost dry-run 报告》归档 + 进入 §16 #4 BLOCKER 闭环交付物 —— ✅
- `integration-design.md` §16 表 #4（行 2221）行末更新为"按 §1.3.4 v1.2 fallback 优先级 F-1..F-4 选路"，引用对齐。

### Cosmetic 3（旧设计 deprecated 标注）— ✅ 闭环

- `integration-design.md` §10.1 SEC-HIGH-8 行（行 2013）行末追加"⚠️ **v1.1 已 deprecated**：v1.0 这条历史 CHANGELOG 描述...保留此行供历史考古，**实际以 §6.4 v1.1 + §16.1 v1.2 决策树为准**" —— ✅
- `00-architecture-overview.md` §14.2 SEC-HIGH-8 行（行 906）同款 deprecated 标注 + 引用本文 §18.1 / §18.4 —— ✅
- `00-architecture-overview.md` §18.4（行 1191-1208）新增 v1.1 撤销项汇总表（7 行 + 说明）—— ✅ 信息密度高于 round-2 推荐，把"已字面替换"vs"保留供历史参考"两类清晰区分，避免日后翻 git blame 时困惑。

### Cosmetic 4（跨主机部署强制 mTLS）— ✅ 完整闭环

`integration-design.md` §16.1（行 2229-2273）：
- 5 档决策树齐全：单机 Podman / 同 VPC 跨 host / 跨 AZ / 跨 VPC / 跨 region —— ✅
- 4 条强制规则齐全：① ops 必选定档 ② "跨主机即必 true"（覆盖 §6.4 默认 false）③ 跨 AZ/VPC/region 强制 cipher 白名单 ④ 选定档变化时必须先切 flag staging 验证 ≥ 24h 再迁 —— ✅
- §16 表 #1 行（行 2218）已引用 §16.1，链路通。

> **第 ②、④ 条是 Fy-api 团队最关心的**：避免"ops 沿用同 host 的 false 缺省"和"边迁边切"两种事故路径。文本明确，可作为 ops PR review checklist 直接套。✅

---

## 3. 架构师自标 2 个 LOW 风险点 verdict

### 风险点-5：§16.1 第 5 档跨 region 措辞"本不应" — ✅ 接受为 LOW

- 现行措辞（行 2262）："Phase 1 的 CN/SG 是数据隔离两区，**本不应** partner-api ↔ Fy-api 跨 region 调用；若有特殊审计 / 监控通道需跨 region，必须 mTLS + IPsec / 阿里云高速通道"。
- 我判断**不需要改成"禁止"**。理由：① 跨 region 数据隔离已由合规层（PIPL / PDPA + ADR-005 fy_api_db 物理分库 + GRANT）硬约束，**架构层"禁止"是冗余强化**；② "本不应...若有...必须 mTLS + IPsec"的写法保留例外通道（审计 / 监控），与 Phase 2A 可能的中央化监控演进兼容；③ 强制规则 #2 已把"跨 region"明确归为"必须 mTLS=true"档，并在规则 #3 加 cipher 白名单 + IPsec —— **执行面已经收紧**，叙述层"本不应"不松绑。
- 残余风险：若 ops 误把"本不应"读成"建议避免"而开了一条无 mTLS 的跨 region 调用 —— 但规则 #2 + §6.4 部署矩阵会兜底拦下。可接受。

### 风险点-6：§1.3.4 F-3 工作量 3.5 → 8.5 人天 — ✅ 接受"需重估"足够

- 现行（行 430）："若进入 F-3（DTS 切流）→ PR-1 工作量从 3.5 人天上调到 ≈ 8.5 人天，§1.9 / §15.1 排期需重估并通知 PM 重新对齐 §16 #7"。
- 我判断**不需要预排一份 fallback 排期**。理由：① F-3 是 dry-run 失败后的应急路径，触发概率 ≤ 20%（F-1 DBA 临时授权 + F-2 DMS 通常足够），预排一份 8.5 人天排期 = 在 Phase 1 排期 commit 阶段引入"概率 < 20% 的悲观估算"，反而扰乱 PM 决策；② §16 #7 BLOCKER 已明示"业务方 + PM 接受工作量"是 Phase 1 PR-1 启动前置 —— **触发 F-3 的瞬间**就有人负责重估，不会无主滑过；③ §1.3.4 步骤 4 + §16 #4 BLOCKER 闭环（dry-run 报告归档）形成闭环触发器，PM 不会"被动收到事故"。
- 残余风险：PR-1 staging dry-run 失败 + 业务上线压力下 PM 可能想"先按 3.5 人天上"—— 但 §16 #7 BLOCKER 状态写明"业务方须接受"，只要不签认就不开工。可接受。

---

## 4. 是否还有遗漏 / 新发现

无。

- 三份文档跨文档同步状态（integration §18.3 / overview §19.3 / backend §24 末尾）一致，未见 dangling 引用。
- v1.2 修订均集中在 Fy-api 覆盖层 / 部署形态 / 历史 CHANGELOG 索引层，**未触碰 §1 OVERLAY 字段 / §2 OpenAPI / §3 outbox event schema / §4 saga 状态机 / §5 跨服务幂等 / §6 GRANT** —— 即 partner-api 团队 + Security + Compliance 已 PASS 的 v1.0 / v1.1 决策无变更，符合"cosmetic 不重启 4 方 review"的承诺。
- §16 8 项 hand-off 表中 6 项 BLOCKER 状态在 v1.1 → v1.2 期间未变化（仍等 ops / DBA / partner-api 团队闭环），与本轮 cosmetic 范围一致。

---

## 5. Hand-off 一句话

> **v1.2 定稿可进入 Phase 1 PR-1 启动**：ops + DBA 在 §16 hand-off #4 BLOCKER 完成 CN/SG staging gh-ost dry-run 报告归档（按 §1.3.4 流程，含权限预检 SQL + fallback 选档）后，Fy-api 团队即可启动 PR-1（B-14 BIGINT migration，独立观察 24h）；partner-api 团队在 PR-2 编码 Week 1 前签认 §16 #5 HMAC keystore 接口契约 + ops 按 §16.1 决策树选定 mTLS 部署档（同主机 = false / 任何跨主机 = true 强制）即可解锁 PR-2 编码。

—— Fy-api Tech Lead，2026-05-14（FINAL ACCEPT）
