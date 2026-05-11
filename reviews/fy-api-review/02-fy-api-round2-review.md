# Fy-api 侧 Round-2 复核：TraceNex Partner OVERLAY 集成层（v1.1）

> 作者：Fy-api 后端 / tech lead
> Review 对象：`docs/integration-design.md` v1.1（2221 行）+ `docs/00-architecture-overview.md` v1.1（1196 行）+ `docs/backend-design.md` v1.1（3690 行）
> Review 时间：2026-05-13
> 输入：`reviews/fy-api-review/00-revision-summary-v1.1.md`（架构师修订摘要）+ 上轮 review `01-fy-api-side-review.md`
> Fy-api 基线：HEAD（与 upstream `QuantumNous/new-api/main` 同步，0 commits drift）+ `OVERLAY.md` 现状 B-1..B-11

---

## 1. 执行摘要

**Verdict：ACCEPT**（0 CRITICAL / 0 HIGH）。

- 上轮 4 项 CRITICAL（OVERLAY 编号 / Redis client 版本 / BIGINT migration / mTLS 部署形态）**全部闭环**，且经实地代码核对与 v1.1 文档比对，描述与 Fy-api 现状字字对得上。
- 上轮 5 项 HIGH（GroupRatioOverride 调用站清单 / feature flag / 工作量 + 5-PR 拆分 / 反向请求登记 / 路由组挂载与 GlobalAPIRateLimit）以及 7 项 MEDIUM 全部闭环；新增 §14 + §15 + §16 三节和 §1.4.2 / §1.5.3 / §1.6.3 / §6.4 重写都做到了"照表实施级别"的细度。
- 仅有 2 项 MEDIUM 性的"文档级残余"（§14.1 flag 热加载语义对 PR-3 的隐性依赖未显式写入正文 / §1.3.3 gh-ost 在阿里云 RDS 8.0 触发器 DDL 权限与 fallback 路径缺述）—— 都是非阻塞的 round-3 推荐补强，不影响 Phase 1 PR-1 启动决策。
- 故按"0 CRITICAL / 0 HIGH 即 ACCEPT"门槛，本轮给出 **ACCEPT** 终判；末尾列出 ≤3 项工程开工前 ops / partner-api 团队必须先完成的 BLOCKER。

---

## 2. 4 个 CRITICAL 闭环审计

### CRITICAL-1：OVERLAY 编号 B-12..B-18 ✅

**v1.1 落点**：`integration-design.md` §1.8（行 683-758）。

**实地核对**：
- `Fy-api/OVERLAY.md` 当前已用编号一字未漏：B-1 / B-1.1 / B-2 / B-3 / B-4 / B-5 / B-6 / B-7 / **B-8 [gemini]** / **B-9 [claude]** / **B-10 [relay 500→400]** / **B-11 [/v1/messages 二级反序列化 + 图片块 nil-deref]**（行 24-151）。
- v1.1 §1.8 给出的 B-12..B-18 共 7 条 OVERLAY 模板，每条都齐备：新增文件 / 修改文件 / 冲突风险 / Merge 策略 / Feature flag。例如 B-12（行 690-698）"router/main.go 1 行加在 SetRelayRouter 之后；不挂在 apiRouter Group 下（避开 GlobalAPIRateLimit）"，B-15（行 713-726）8 个 hot-path 文件清单，B-16（行 728-735）"函数顶部加 `// Fy-api overlay: B-16 TX wrap with outbox`"。
- §11 变更管理（行 1946）也同步更新为 "B-12..B-18"，§1.8 末尾的 OVERLAY.md PR 合入规则（行 758）给出了 CI gate（`grep B-1[2-8]` 命中数等同 patch 文件数）。
- `00-architecture-overview.md` §C-1（行 906 区附近）+ §18.1 + 附录 C / `backend-design.md` §24.1 都按 B-12..B-18 索引同步修订。

**Verdict：✅ FIXED**。Fy-api 团队可直接复制 v1.1 §1.8 的 7 段 markdown 到 `OVERLAY.md`（合入 PR 时按 PR 拆分逐条进入），无需任何二次加工。

### CRITICAL-2：Redis go-redis/v8 ✅

**v1.1 落点**：`integration-design.md` §1.1.3（行 96-188）+ §1.6.3（行 601-631）+ §17.1 CHANGELOG（行 2182）。

**实地核对**：
- `Fy-api/go.mod:25` 实测 `github.com/go-redis/redis/v8 v8.11.5`，与 v1.1 完全一致。`Fy-api/common/redis.go:12` 也 `import "github.com/go-redis/redis/v8"`，全仓没有 v9。
- v1.1 §1.1.3 行 115：`"github.com/go-redis/redis/v8" // v1.1：与 Fy-api go.mod 对齐（v8.11.5），原 v9 移除`，注释精确。
- 全文 grep `go-redis/v9 / redis/go-redis/v9 / v9 风格 API`：仅命中 §17.1 CHANGELOG（行 2182）作为 v1.0→v1.1 的修订对照（"原 v9 移除"），属于历史叙事而非残留代码示例。`backend-design.md:3676` 也只是 NOTE-FIXED 的同源叙事，正文伪代码是 v8 ctx-first 风格。
- v1.1 §1.1.3 行 158 `rdb.SetNX(c.Request.Context(), nonceKey, "1", NonceTTL).Result()` —— ctx-first 签名，正是 v8.11.5 的 `func (c *Client) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *BoolCmd`。
- v1.1 §1.6.3 行 612-614 `func SubscribeOptionInvalidations(rdb *redis.Client) { sub := rdb.Subscribe(context.Background(), ...) }` —— v8 也是这样的签名，正确。
- 全文未命中任何 v9 独有 API（`Probabilistic` / `redis.NewUniversalClient` 不带 ctx 等），无残留。

**Verdict：✅ FIXED**。v1.1 全部伪代码都是 v8 ctx-first，可直接拷贝到 Fy-api 的 middleware 文件，编译通过 zero-friction。

### CRITICAL-3：BIGINT gh-ost migration ✅

**v1.1 落点**：`integration-design.md` §1.3.3（行 302-392）+ §15.2（行 2134-2138）+ §16 #4（行 2163）。

**实地核对**：
- `Fy-api/model/log.go:19-40` 的 `Log.Id int` 与 5 个二级索引（`idx_created_at_id` / `idx_user_id_id` 等）确认存在；`Fy-api/model/main.go:213-230` 的 LOG_DB 启动逻辑（`LOG_SQL_DSN` 为空时 fallback = DB）确认与 v1.1 §6.2 注释一致。
- v1.1 §1.3.3 给出：
  1. 前置 invariant（`SELECT MAX(id), COUNT(*) FROM logs;` MAX(id) < 1.5e9 留 30% 余量，行 329）—— 是真正可执行的 SQL。
  2. 完整 gh-ost 命令模板（行 332-345）含 `--chunk-size=1000` / `--max-load=Threads_running=25` / `--critical-load=50` / `--max-lag-millis=1500` / `--postpone-cut-over-flag-file` / `--serve-socket-file` —— 是生产级配置，不是占位符。
  3. 预估时长 + 风险表（行 348-356）按 CN 100M-300M / SG 80M 给出 4-8h / 3-6h 复制阶段 + < 1s cut-over 锁，并独立列了 PG <12 全表重写、SQLite table-rebuild 备选。
  4. 回滚方案：`gh-ost cancel + DROP _logs_gho 影子表 + 不动原表`（行 352）—— 是真实可执行的 fail-safe，不是空话。
  5. CN 与 SG 区域不同步执行（先 SG 再 CN，间隔 ≥ 24h，行 357）—— 符合 ops 对蓝绿独立观察的工程惯例。
- §15.2 显式说明 BIGINT 不可回滚 + 24h 独立观察 + 与 Go 代码 PR 解耦（行 2136-2138）。
- §16 hand-off #4 把"gh-ost staging 跑通"列为 ops + DBA 责任的 BLOCKER（行 2163）。

**Verdict：✅ FIXED，但带 1 项 round-3 推荐补强**（详见 §6 自标风险点-2 verdict）。命令模板 + 风险评估 + 回滚 + 区域错峰都齐全，且作为不可回滚 PR-1 单独走，可以安全启动。**唯一遗憾**：阿里云 RDS 8.0 对 `gh-ost` 触发器 DDL 的权限要求（必须 DBA 角色，`tnbiz_app` 没有该权限）和"如果 RDS 锁死 trigger DDL 时的 fallback（pt-osc 或 RDS 原生 INSTANT online DDL）"在 §1.3.3 没有显式列出 —— 这是 ops 在 staging 跑 dry-run 时一定会撞到的真实点，建议补一段。降级为 MEDIUM。

### CRITICAL-4：mTLS = Nginx + Podman ✅

**v1.1 落点**：`integration-design.md` §6.4（行 1660-1759）+ §1.1.3 备注（行 194）+ §17.1（行 2184）。

**实地核对**：
- `Fy-api/compose.prod.yml:25` 注释 "只绑 loopback,Nginx 反代再暴露"；`scripts/prod/06-deploy-blue-green.sh:25,95-105` 全是 `podman` + `nginx -t / systemctl reload nginx` —— 与 v1.1 §6.4 描述完全一致。
- v1.1 §6.4 给出：
  1. 部署形态层级图（行 1664-1682）—— Nginx 终结 mTLS 注入 `X-Client-Verified` / `X-Client-CN`，gin middleware 1) loopback 信任域 2) X-Client-Verified == "SUCCESS" 校验 3) CN allowlist 4) HMAC 应用层。
  2. 完整 Nginx vhost 样板（行 1686-1723）含 `ssl_client_certificate /opt/fy-api/certs/internal-ca.crt` + `ssl_verify_client on` + `ssl_verify_depth 2` + `proxy_set_header X-Client-Verified $ssl_client_verify` + `limit_req_zone` —— 可直接交给 ops 落到 `/etc/nginx/conf.d/fy-api-internal.conf`。
  3. gin middleware 校验逻辑（行 1727-1748）—— `isLoopback(c.Request.RemoteAddr)` 兜底 + `OverlayMTLSEnabled` flag 控制 + HMAC 永远启用。
  4. Phase 矩阵（行 1750-1757）—— Phase 1 默认 OVERLAY_MTLS_ENABLED=false 走 VPC + HMAC；Phase 1 staging 验证打开；Phase 2A 强制开；Phase 2B 集群迁移后回到 sidecar mesh。
- 全文 grep `Istio / PeerAuthentication / X-Forwarded-Client-Cert / NetworkPolicy / ServiceAccount`：
  - `integration-design.md` 仅命中 §6.4 "v1.0 错处"段落（明确撤销）+ §17.1 CHANGELOG（修订对照叙事）+ Phase 2B 注释（"届时回到 v1.0 mesh 设计"）。
  - **§10.1 行 1972 残留 1 行旧 CHANGELOG**：`SEC-HIGH-8 mTLS 在 mesh 下 c.Request.TLS 失效 | FIXED | §6.4 新增：Istio STRICT + X-Forwarded-Client-Cert CN 白名单 + NetworkPolicy` —— 这是 v0.2 历史 CHANGELOG 的原始落点描述，与 v1.1 §6.4 当前内容自相矛盾（v1.1 §6.4 已经撤销了 Istio/STRICT）。
  - `00-architecture-overview.md:906` 同款历史 CHANGELOG 残留：`§2.2 流量表明示 Istio STRICT + X-Forwarded-Client-Cert CN 白名单`。
  - 这两条都是历史 CHANGELOG 索引，**不会**作为正文规范被实施 reviewer 直接照抄；但读者偶尔翻历史 CHANGELOG 时会困惑。属于 LOW 级 cosmetic，不阻塞。

**Verdict：⚠️ FIXED-WITH-COSMETIC**。规范主体 §6.4 完整、可实施，Fy-api 团队 + ops 可基于此 vhost 样板直接 PR Nginx 配置。建议 round-3 把 §10.1 / overview §906 两行旧 CHANGELOG 加一句"（已被 v1.1 §6.4 撤销重写）"标注，避免日后历史考古时混淆。降级为 LOW，不计入 HIGH 计数。

---

## 3. HIGH 闭环审计

### HIGH-5：GroupRatioOverride 6 调用站 / 4 文件 ✅

**v1.1 落点**：`integration-design.md` §1.4.2（行 412-460）+ B-15 OVERLAY 条目（行 713-726）。

**实地代码核对**（`grep -nE "GetGroupRatio|GetGroupGroupRatio"` 在 4 个文件）：

| 文件 | v1.1 文档行号 | 实地行号 | 一致性 |
|---|---|---|---|
| `service/quota.go::CalculatePostConsumeQuota` | 110-121 | **110, 115, 121** | ✅（v1.1 写"行 110-121"，三个调用全在范围内）|
| `relay/helper/price.go` | 53-61 | **53, 61** | ✅ |
| `service/task_billing.go::BillingByPostConsumePrice` | 276-277 | **276, 277** | ✅ |
| `service/group.go` | 60-64 | **60, 64** | ✅ |

合计 9 个 grep 命中，对应 6 个语义独立的"调用站"（quota.go 110+121 是同一段、115 是 autoGroup 分支；其余各 1）。v1.1 §1.4.2 行 442-451 表格 6 行清单与代码现状一字不差。

`relay/common/relay_info.go:87` 的 `RelayInfo` struct 含 `UserGroup string`（行 93），v1.1 §1.4.2 行 432-435 说 "struct 末尾 +1 字段 `UserGroupRatioOverride float64`" —— 字段加法可行。`middleware/distributor.go:111` 已经在写 RelayInfo（实测 `userGroup := common.GetContextKeyString(c, ContextKeyUserGroup)`），v1.1 说 "在 distributor 阶段从 user.GroupRatioOverride 拷贝" 可以直接挂在那段附近。

性能预算（行 455-458）：纳秒级 `> 0` 比较 + 1 次 float64 读，对 `/v1/chat/completions` P99 影响 < 0.01ms，可接受。

**Verdict：✅ FIXED**。RelayInfo 字段方案是真正侵入最小的解，比 B 方案（传 `*User` 改 6 处签名）冲突面降一个数量级。

### HIGH-6：每 PR 独立 feature flag ✅（带 1 项 MEDIUM 残余）

**v1.1 落点**：`integration-design.md` §14（行 2073-2114）。

**核对**：
- 5 个 `OVERLAY_*` flag 名称齐全（INTERNAL_API / BIGINT / GROUP_RATIO_OVERRIDE / OUTBOX / PUBSUB），每个挂一个 PR。
- §14.1（行 2079-2086）每个 flag 都说明：控制范围 / 关掉后行为 / 注入路径。
- §14.2 默认值矩阵覆盖 5 环境（dev / CI / staging / prod 首发 / prod 各 PR 灰度后），prod 首发全 false 为影子模式 —— 思路正确。
- §14.3 回滚条件 + 责任人 + 时限（行 2105-2112）—— 6 条触发条件，从 outbox 写入失败率到计费 hot-path P99 退化 > 20% 都涵盖。
- 额外 `OVERLAY_MTLS_ENABLED`（部署层）单独由 §6.4 矩阵管理，分离合理。

**MEDIUM 残余**：§14.1 行 2081 称 INTERNAL_API flag "可热加载，订阅 `option_update` 后即生效"，但 `option_update` Pub/Sub 是 PR-3（B-17）才上线的能力。如果 PR-2 先于 PR-3 部署、且 prod 首发 flag = false，需要从 false 切 true 时，没有 PR-3 = 没有 publish + subscribe，热加载承诺**实际上要靠 polling SyncOptions 兜底（5-15s 滞后）或重启 pod**。架构师在修订摘要 §5 担心点 1 自陈了这个隐患但没写进 §14.1 正文，也没写入 §15.1 PR 依赖表。建议 round-3 在 §14.1 OVERLAY_INTERNAL_API 行尾补："（热加载需 PR-3 OVERLAY_PUBSUB 已部署；否则 polling SyncOptions 5-15s 兜底或 pod 重启）"。降级为 MEDIUM，不阻塞。

### HIGH-7：工作量 29 人天 + 5-PR 拆分 ✅

**v1.1 落点**：`integration-design.md` §1.9（行 760-779）+ §15（行 2118-2150）。

**核对**：
- §1.9 LOC + 工作量重估表（行 765-777）11 个模块每个有 LOC / 编码 / 单测 / 集成测 / 文档 拆分；合计 29 人天 = 6 周（1 工程师）/ 3 周（2 工程师），与上轮 review §12 完全一致。
- §15.1 PR 概览（行 2124-2130）5 个 PR 每个有 OVERLAY 覆盖 / 编码人天 / 测试覆盖 / 回滚方式 / 依赖 PR —— 表头齐全，PR-1 不可回滚 / PR-2 影子模式 / PR-5 同事务 + 压测都有专节论证（§15.2 / §15.3 / §15.4）。
- 串并行节奏（行 2132）：PR-1 + PR-3 可并行；PR-4 + PR-5 上线后可并行测；总体 5-6 周（单人）/ 3 周（双人）。

**Verdict：✅ FIXED**。是排期可直接 commit 的拆分。

### HIGH-7-pubsub：订阅 goroutine coalescing ✅

**v1.1 落点**：`integration-design.md` §1.6.3（行 608-630）。

**核对**：v1.1 用 `var dirty atomic.Bool` + 独立 ticker 200ms + `dirty.CompareAndSwap(true, false)` 触发 reload，标准 coalescing 模式。`for range msgs { dirty.Store(true) }` 在 channel 满时上游会丢消息但 dirty 仍然 true 等下次 ticker，**不会断订阅**（v1.0 的 `<-rateLimit.C` per-message 模式有此风险）。代码正确。

**Verdict：✅ FIXED**。

### HIGH-8：RecordConsumeLog TX + LogQuotaData fire-and-forget ✅

**v1.1 落点**：`integration-design.md` §1.5.3（行 521-558）。

**实地代码核对**：`Fy-api/model/log.go:204-253` 实测 RecordConsumeLog 行 244 是 `LOG_DB.Create(log).Error`、行 248-252 是 `if common.DataExportEnabled { gopool.Go(LogQuotaData) }`。v1.1 §1.5.3 行 555-558 明示：
1. `gopool.Go(LogQuotaData)` 必须在 `Transaction` 闭包外（commit 之后）
2. 若放闭包内则 TX 失败时也不触发，整体一致
3. 推荐位置：TX 返回成功后 —— 与现状的"末尾追加"一致

注释规约（行 553）：5 个非 consume 的 LOG_DB.Create 调用站（行 87/112/139/183/292，对应 RecordTopupLog / RecordLogWithAdminInfo / RecordErrorLog / RecordTaskBillingLog 等）必须打 `// Fy-api overlay: B-16 outbox scope = consume only; do NOT add outbox here` 注释；lint check 用 `grep LOG_DB.Create | wc -l` 数目监控（>5 命中报警）。

**Verdict：✅ FIXED**。lint checklist 是 production-ready 级别。

### HIGH-9：内部 idempotency vs idempotency_record 字段语义 ✅

**v1.1 落点**：`integration-design.md` §1.7.2（行 657-676）。

行 676 v1.1 注释明示："本表（`internal_idempotency`）与 §5.3 `idempotency_record`（partner-api 侧）是两张独立的表"，Phase 1 用明文 `response_body TEXT`，Phase 2A 视审计需要再决定是否做 KMS envelope（+7 人天）。两表语义独立解释清楚。

**Verdict：✅ FIXED**。

### HIGH-10：路由组挂载避开 GlobalAPIRateLimit ✅

**v1.1 落点**：`integration-design.md` §1.1.1（行 67）+ §1.8 B-12 Merge 策略（行 697）。

行 67 明示 "`/api/internal/*` 不挂在 `apiRouter := router.Group(\"/api\")` 之下；改为在 `SetInternalRouter` 内独立 `router.Group(\"/api/internal\")` —— 显式不挂 `GlobalAPIRateLimit`，改用 §6.3 per-kid quota；保留 `BodyStorageCleanup`；新加 `RouteTag(\"api-internal\")`"。完全采纳上轮 review §4.1 的建议。

**Verdict：✅ FIXED**。

### HIGH-Reverse-Asks：8 项反向请求登记 ✅

**v1.1 落点**：`integration-design.md` §16（行 2154-2169）。

8 项一一登记：每项有 责任方 / 截止 / 阻塞性（5 BLOCKER + 2 NICE-TO-HAVE + 1 工作量重估对齐）/ 当前状态。第 5 项 "HMAC keystore 接口契约" v1.1 已锚定 Fy-api 内嵌实现（§1.1.3 行 196）+ 表存于 fy_api_db `internal_api_key`（B-18 OVERLAY 条目），等 partner-api 团队签认即闭环。

**Verdict：✅ FIXED**。

---

## 4. MEDIUM 闭环审计

| # | 项 | v1.1 落点 | Verdict |
|---|---|---|---|
| M-9 / M-13 | SyncFrequency 60s → 5-15s + polling 不删 | §1.6.3 行 633 备注 | ✅ FIXED |
| M-10 | Pub/Sub 选型（保留 Redis Pub/Sub + DB outbox，否决 MNS / RocketMQ）| §1.6.3 行 635 备注（理由 3 条：Fy-api 已依赖 Redis 零增量 / 部署无 broker / 已覆盖可靠性场景）| ✅ FIXED |
| M-11 | partner-api 同实例不同 DB GORM 多 DB + 连接池 | §6.2 行 1615-1626 v1.1 注释（`bizDB maxOpen=50 / fyReadDB maxOpen=20 / logDB maxOpen=20` + ConfigMap 同步加 LOG_SQL_DSN）| ✅ FIXED |
| M-12 | 监控 dashboard 集成 | §9.3 行 1898-1902 v1.1 备注（共享 Prometheus + Grafana，按 tag 分两块；既有 Fy-api dashboard 保留独立）| ✅ FIXED |
| M-14 | Fy-api 侧新 metrics | §9.3 行 1890-1895（5 个 Prometheus metric：`internal_idempotency_hits_total{kid,endpoint}` / `internal_idempotency_conflicts_total` / `consume_log_outbox_writes_total{result}` / `consume_log_outbox_tx_duration_seconds` / `internal_scope_mismatch_total{kid}`）| ✅ FIXED |
| L-15 | `Fy-api/openapi/internal-api.yaml` 必须随 OVERLAY PR 创建 | §1.8 B-12 文件清单 + §11 行 1945 已锚定 | ✅ FIXED |
| L-16 | F-1 per-model markup 延后 Phase 2A | §1.4.4 行 469-475 已声明 schema-only 不开发 | ✅ NO-CHANGE |

全部 MEDIUM 闭环。无残余。

---

## 5. 自标 3 个 round-2 风险点 verdict

### 风险点-1：Feature flag 热加载语义对 PR-3 的隐性依赖 ⚠️ MEDIUM 残余

架构师自陈 §14.1 写 "可热加载，订阅 `option_update` 后即生效" 但 `option_update` Pub/Sub 正是 PR-3（B-17）。架构师预案是补一句"flag 热加载需要 PR-3 + OVERLAY_PUBSUB=true；否则只能重启或 polling 兜底"——这句话**没写进 §14.1 正文**。

**Fy-api verdict**：风险**真实**但**不阻塞**。理由：
1. `common/init.go::SyncFrequency` 默认 60s polling SyncOptions 永远启用（M-9 / §1.6.3 备注），即使 PR-3 还没上，flag 切换最多滞后 5-60s 生效（v1.1 建议缩短到 5-15s 后，5-15s）。Phase 1 prod 首发 flag = false 影子模式，实际不会有"切 true 必须秒级生效"的硬场景。
2. §15.1 PR 依赖表（行 2124-2130）写明 PR-3 无依赖前置，与 PR-1 可并行；只要 ops 顺序不出错（PR-3 不晚于 PR-2 太久）即不踩坑。

**Round-3 推荐补强**（不阻塞）：在 §14.1 OVERLAY_INTERNAL_API 行末加 "（PR-3 未到位时，热加载靠 polling SyncOptions 5-15s 兜底；prod 首发可承受）"，把架构师自陈的预案明文化。

### 风险点-2：gh-ost 在阿里云 RDS 8.0 触发器 DDL 权限 ⚠️ MEDIUM 残余

架构师自陈 §1.3.3 给的 gh-ost 命令模板假设 RDS 8.0 支持 binlog row 模式 + 用户能创建 trigger，但**阿里云 RDS 8.0 创建 trigger 需要 DBA 角色**（`tnbiz_app` 没此权限），且部分高版本 RDS 可能限制 trigger DDL；fallback 是 pt-osc 或 RDS 原生 INSTANT online DDL（INSTANT 不支持 widen PK，可能没救）。

**Fy-api verdict**：风险**真实**且**只有在 staging 跑 dry-run 时才会暴露**。理由：
1. §16 hand-off #4 已经把"gh-ost staging 跑通"列为 ops + DBA 责任的 BLOCKER（行 2163），形式上闭环。
2. 但 §1.3.3 没列出 fallback 路径（pt-osc / 原生 online DDL / 拆 logs_v2 表 dual-write 切流）。如果 ops staging 撞墙，PR-1 会卡死等架构方案，影响排期。

**Round-3 推荐补强**（不阻塞）：在 §1.3.3 末尾或 §16 #4 备注里加一段 "若阿里云 RDS 8.0 拒绝 gh-ost trigger DDL，按优先级 fallback：(a) pt-online-schema-change（同样需 trigger）→ (b) 新表 `logs_v2(id BIGINT)` dual-write + 切流 + 旧表 archive（参见 docs/Phase3-DB-migration-runbook.md）"。给 ops 一个备选路径再去 staging。

### 风险点-3：B-15 GroupRatioOverride 4 hot-path 文件长期冲突成本 ✅ ACCEPTED-AS-DEBT

架构师自陈每周 sync 期望 1-2 文件冲突 / ops 0.5-1 小时是主观估计；6 个月内若上游对计费系统大重构，4 文件可能同时冲突 → 单次 sync 4-8 小时。RelayInfo 字段方案已是侵入最小的解，没有更好备选。

**Fy-api verdict**：**接受为长期架构债务**。理由：
1. RelayInfo 字段方案确实是最小侵入解（替代方案是改 6 处签名，冲突面更大）。
2. `00-architecture-overview.md` 附录 A T-24 已经登记 "把 `RecordConsumeLog TX wrap` 上游下沉作为长期解"（这条对 B-16 有效，但对 B-15 hot-path 4 文件不通用 —— per-user override 不是上游愿意接的能力）。
3. Fy-api 团队接受这条债务，前提是 v1.1 §1.4.2 行 453 强制每处 patch 打 `// Fy-api overlay: TraceNex Partner pricing override (B-15)` 注释（grep 即可定位），冲突时用 `git mergetool` 半小时内可处理。
4. 一旦 Phase 1 跑稳后（约 2026 Q3），评估能否给 upstream 提 RFC "GroupRatio per-user 标量 override" 把它下沉。

不计入 round-3 必修。

---

## 6. 新引入风险审计

### 风险-A：mTLS 改 Nginx 后 partner-api ↔ Fy-api 互相调用的证书 chain ✅ 可控

v1.1 §6.4 把 mTLS 终结从 mesh 改到 Nginx 反代后，partner-api → Fy-api 走 Nginx → loopback gin。partner-api 持有 client cert，Nginx 持 internal-ca + server cert，CA chain 关系清晰。**反向调用**（Fy-api → partner-api）目前**不存在**（Phase 1 是单向：partner-api 调 Fy-api `/api/internal/*`；outbox 是 partner-api 主动 poll Fy-api LOG_DB，不是 Fy-api 调 partner-api）。所以 §6.4 的单向 mTLS 拓扑覆盖了所有真实流量。

无新风险。

### 风险-B：Phase 1 默认 OVERLAY_MTLS_ENABLED=false 走 VPC 内网 + HMAC 是否成立 ✅ 可控但需 ops 确认

v1.1 §6.4 行 1754 矩阵写 "Phase 1 默认 = CN/SG 单 ECS，partner-api 与 fy-api 同主机；HMAC + 内网即可"。

**实地核对**：Fy-api 现在 CN（`8.136.146.211:58422`）和 SG（`47.236.133.70:58422`）确实是单 ECS。如果 partner-api 决定**同主机部署**（同一台 ECS 上跑两个 Podman 容器），那走 127.0.0.1 loopback + HMAC 完全够用。但如果 partner-api 跑在另一台 ECS（不同 VPC subnet 甚至跨可用区），则 §6.4 默认 false + HMAC 是不够的（HMAC 防 replay 不防 MITM；公网或跨 VPC 必须叠 mTLS 或 IPsec）。

**§16 hand-off #1**（行 2160）已经把"mTLS 终结层选型确认 + Nginx vhost 草案"列为 BLOCKER。Fy-api 团队在 PR-2 编码 Week 1 之前必须收到 ops 给的部署拓扑确认（同主机 vs 跨主机）—— 这是 §16 #1 的天然延伸。

**Round-3 推荐补强**（不阻塞）：在 §16 #1 注释里点名 "如果 partner-api 与 fy-api 不同主机部署，OVERLAY_MTLS_ENABLED 必须 = true（不允许走 false 默认）"。

### 风险-C：BIGINT PR-1 不可回滚 vs Phase 1 砍掉 partner 功能 ✅ 可控

§15.2 明示 BIGINT 不可 narrow，真实意义上不可回滚。**最坏情况**：PR-1 上线后业务侧决定 Phase 1 砍掉 partner 功能 → schema 已经升 BIGINT 但代码没用上 → 浪费一次 ops 维护窗口 + 磁盘空间 +30%（gh-ost 影子表清理后释放）。

**Fy-api verdict**：风险**可承受**。理由：
1. `users.quota / used_quota / aff_*` 等字段升 BIGINT 在 Fy-api 上游也是迟早的债（参考上轮 review §8.2 表，建议下沉给 upstream）。即使 partner 功能砍掉，Fy-api 自己也用得着。
2. `logs.id` BIGINT widening 是真正的"为 partner 而做"的 schema 改动 —— 但 logs.id 撞 INT32 上限本来就是迟早的事（CN/SG 各 100M+ 80M 行，几年内一定会撞 21 亿）。提前做不亏。
3. 真砍掉 partner 功能 → BIGINT 留下不影响任何运行时行为（GORM int 字段读 BIGINT 列 zero-friction）；浪费的只是 ops 一次维护窗口。

不计入 round-3 必修。

### 风险-D：partner-api 同 ECS 同主机情况下的 Redis ACL 隔离 ✅ 已锚定

v1.1 §1.6.4（行 638-641）+ §16 #2 已锚定 Redis 6+ ACL（`tnbiz_app` 无 PUBLISH `option_update` 权限）+ ops staging 验证。同主机 = 共享同一 Redis 实例，ACL 是唯一隔离手段，已经被 v1.1 显式列为 BLOCKER。

无新风险。

---

## 7. 残余 / Round-3 推荐补强清单

> 全部为 MEDIUM / LOW 级别，**不阻塞** ACCEPT 决策；建议在 Phase 1 PR-1 启动后、PR-2 编码前的 1-2 天文档窗口内补齐。

| # | 严重度 | 文档落点 | 推荐补强 |
|---|---|---|---|
| 1 | MEDIUM | `integration-design.md` §14.1 OVERLAY_INTERNAL_API 行 2081 | 行末加："（PR-3 未到位时，热加载靠 polling SyncOptions 5-15s 兜底；prod 首发可承受）"|
| 2 | MEDIUM | `integration-design.md` §1.3.3 末尾 或 §16 #4 备注 | 加 fallback 路径段落："若阿里云 RDS 8.0 拒绝 gh-ost trigger DDL：(a) pt-osc 备选 / (b) 新表 logs_v2 dual-write 切流（参 docs/Phase3-DB-migration-runbook.md）" |
| 3 | LOW | `integration-design.md` §10.1 行 1972 + `00-architecture-overview.md` 行 906 旧 CHANGELOG | 在两条 "Istio STRICT + X-Forwarded-Client-Cert CN 白名单" 历史叙事行末加 "（已被 v1.1 §6.4 撤销重写）" 标注，避免日后翻历史时混淆 |
| 4 | LOW | `integration-design.md` §16 #1 备注 | 加一句 "若 partner-api 与 fy-api 不同主机部署，OVERLAY_MTLS_ENABLED 必须 = true（不允许走 Phase 1 默认 false）" |

以上 4 条均为文字级补强，零代码风险，可在 round-3 一次性收口。

---

## 8. ACCEPT 后 Phase 1 工程开工前的 ≤3 项 ops / partner-api 团队前置

> v1.1 §16 hand-off 表共 8 项；Fy-api 团队作为代码侧开工方，**必须在 PR-1 编码周开始之前**收到以下 3 项 commit；其余 5 项可与 PR-2 / PR-3 并行推进。

### 前置-1（ops + DBA，BLOCKER）：gh-ost 在 CN/SG staging RDS 上 dry-run 跑通

- **必须输出**：CN staging 用 `--alter="MODIFY COLUMN id BIGINT NOT NULL AUTO_INCREMENT"` 在 logs 表跑完整复制 + cut-over，时长记录归档；SG 同。
- **额外校验**：阿里云 RDS 8.0 的 `gh-ost --execute` 是否需要 DBA 账号（不是 `tnbiz_app`）；trigger DDL 是否被 RDS 高版本 lock；不通过则切 fallback 路径（参 round-3 补强 #2）。
- **截止**：PR-1 编码启动前。
- **责任方**：ops + DBA。

### 前置-2（ops，BLOCKER）：mTLS / 部署拓扑确认 + Nginx vhost 草案

- **必须输出**：(a) partner-api 是否与 fy-api 同主机部署 / 同 VPC / 跨 region；(b) Phase 1 是否启用 OVERLAY_MTLS_ENABLED（同主机 = false 可；跨主机 = 必须 true）；(c) Nginx `/etc/nginx/conf.d/fy-api-internal.conf` PR 草案（基于 v1.1 §6.4 行 1686-1723 样板）+ internal-ca / server cert 准备。
- **截止**：PR-2 编码 Week 1 前。
- **责任方**：ops。

### 前置-3（partner-api 团队，BLOCKER）：HMAC keystore 接口契约签认 + KeyStore 表写入路径

- **必须输出**：签认 v1.1 §1.1.3 行 196 + B-18 OVERLAY 条目里的方案 —— `KeyStore` 由 Fy-api 内嵌实现，key 数据存 fy_api_db 新表 `internal_api_key`，partner-api 通过 staff verb 调内部 admin endpoint 写入；同时确认 Phase 1 不对 Fy-api `internal_idempotency` 表启用 KMS envelope（保持明文 TEXT）。
- **截止**：PR-2 编码前。
- **责任方**：partner-api 团队 + Security。

> 其余 §16 表中 #2 Redis ACL（与 PR-3 并行）/ #3 LOG_DB 拓扑（PR-1 之前 ops 确认）/ #6 KMS（已锚定 Phase 1 不做）/ #7 工作量重估对齐（v1.1 §1.9 已正式覆盖）/ #8 flag 默认值（v1.1 §14.2 已固定首发 false）—— 这 5 项 v1.1 已经文档侧闭环或可与 PR-2/PR-3 并行，不进 PR-1 BLOCKER 队列。

---

## 9. 收尾

v1.1 修订把上轮 review 的 4 项 CRITICAL + 5 项 HIGH + 7 项 MEDIUM 全部按"照表实施"级别细度落地，文档质量从 v1.0 的"方向正确但实施侧失真"提升到 v1.1 的"PR 编码可直接照抄"。Fy-api 团队作为代码侧守门员，本轮 0 CRITICAL / 0 HIGH 残余，4 项 MEDIUM/LOW 文字级补强不阻塞。最终 verdict **ACCEPT**。

下一步：
1. PM 把上述 §8 三项 BLOCKER 派发给 ops / DBA / partner-api 团队，目标 1 周内闭环。
2. ops 完成 §8 前置-1 staging dry-run 后，Fy-api 团队启动 PR-1（B-14 BIGINT migration）—— 单独走、独立观察 24h，参 §15.2。
3. PR-1 上线后并行启动 PR-2（B-12 + B-13 + B-18 影子模式）+ PR-3（B-17 Pub/Sub）。
4. 工程师投入按 v1.1 §1.9 = 29 人天（双人 3 周 / 单人 6 周）排期对齐到 PRD 排期评审。

—— Fy-api Tech Lead，2026-05-13
