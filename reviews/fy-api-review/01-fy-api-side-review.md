# Fy-api 侧 Review：TraceNex Partner OVERLAY 集成层（v1.0）

> 作者：Fy-api 后端 / tech lead
> Review 对象：`docs/integration-design.md` v1.0 + `docs/00-architecture-overview.md` v1.0 + `prd/PRD-v1.0.md` 附录 C
> Review 时间：2026-05-12
> Fy-api 基线：HEAD（与 upstream `QuantumNous/new-api/main` 同步，0 commits drift）
> 角色定位：Fy-api 守门员，对要落进 `OVERLAY.md` 的代码、对 `model/log.go::RecordConsumeLog` 这条 hot path 的改动以及未来每周 upstream rebase 成本负责。

---

## 1. 执行摘要

**Verdict：ACCEPT-WITH-CHANGES**

- **大方向 PASS**：覆盖层架构（HMAC + 内部路由 + outbox 同事务 + Pub/Sub 失效 + 内部 idempotency 表）的拆分是合理的；落点全部在我们 OVERLAY 策略允许的范围内（新增文件优先 + 上游 patch 必须打 `// Fy-api overlay:`）。9 项里没有任何一项是"真的不能做"。
- **但文档里有 4 类硬错和大量轻微失真**：(a) `OVERLAY.md` 的 B-N 编号已经被 channel-benchmark / gemini / claude / relay overlay 占满到 **B-11**，integration §1.8 说"从 B-8 起"是错的，必须改成 B-12 起；(b) 伪代码里的 `redis/go-redis/v9` 与 Fy-api 现役 `go-redis/v8` 版本不一致，混用会引入第二个 redis client；(c) 三方言 BIGINT migration 在 MySQL `logs.id` 上用 `ALGORITHM=COPY` 默认会全表重写 + 锁表，对生产 hot table 不可接受；(d) mTLS 章节假设 Istio/Linkerd 网格，但 Fy-api CN/SG 实际跑 Podman + Nginx 反代，整套 `X-Forwarded-Client-Cert` + PeerAuthentication 在我们的部署形态里不存在。
- **工作量 PRD 估算偏低**：附录 C-9 写 "~575 LOC，核心 250-300 LOC"，但 GroupRatioOverride 的 hot-path 替换有 **5+ 个调用点**而不是文档说的"一行替换"，加上 outbox cron / KMS envelope / OpenAPI spec 维护 / 三方言 migration 真实成本，Fy-api 侧的 **首发投入约 14-18 人天**（vs 文档暗示的 1-1.5 周 = 5-7.5 人天，欠估 2-3 倍）。

---

## 2. 代码核对结果（实地读了哪些文件）

| 真读过的 Fy-api 文件 | 关键发现（与文档假设的差异） |
|---|---|
| `router/main.go`（35 行） | `SetRouter(router, assets)` 签名带 `ThemeAssets`，**没有 `server *gin.Engine` 这种命名**。integration §1.1.1 写"main.go 调用 `router.SetInternalRouter(server)`"措辞不准；正确做法是在 `SetRouter` 内部增一行 `SetInternalRouter(router)`，签名得统一。 |
| `router/api-router.go`（394 行） | `apiRouter := router.Group("/api")` 全局挂了 `gzip.Gzip` + `BodyStorageCleanup` + **`GlobalAPIRateLimit()`**。integration 文档把 `/api/internal/*` 放在 `/api` 下时**没说**这条 rate limit 中间件要不要应用。partner-api outbox / saga retry 高频探活会被全局限流误伤。 |
| `model/log.go::RecordConsumeLog`（行 204-253） | 确认现状是单条 `LOG_DB.Create(log)`，**不是事务**。integration §1.5.3 说改成 25 LOC TX 是 OK 的。但实际 `RecordConsumeLog` 末尾还有 `if common.DataExportEnabled { gopool.Go(...LogQuotaData) }`，该 goroutine 不能进事务（fire-and-forget 异步），新加的 outbox.Create 与 LogQuotaData 之间没有事务关系——文档里没强调这一点，实施时容易写错。 |
| `model/user.go`（行 39-46） | 确认 `Quota int` `gorm:"type:int"`、`AffQuota int`、`AffHistoryQuota int` 全是 32-bit。BIGINT 升级真有必要。 |
| `model/log.go`（行 19-40） | `Log.Id int`，且参与了**5 个二级索引**（`idx_created_at_id`, `idx_user_id_id`, `idx_logs_request_id` 等），加上 SG `transnext_db.logs` 是从 legacy MySQL 迁过来的存量表（迁移日期 2026-05-07），生产规模在 100M-1B 行量级。`ALGORITHM=COPY` 直接 `MODIFY id BIGINT` 会复制整表 + 重建所有 index，**至少分钟级锁表**。文档说"choose maintenance window" 太轻描淡写。 |
| `setting/ratio_setting/group_ratio.go`（125 行） | 当前 API 是 `GetGroupRatio(name string) float64` 和 `GetGroupGroupRatio(userGroup, usingGroup) (float64, bool)`，**根本不接受 `*model.User` 参数**。integration §1.4.2 写的"一行替换 `setting_ratio.GetEffectiveGroupRatio(user, user.Group)`"前提是改全部签名，否则得加新函数 + 外层判断。 |
| `service/quota.go`（行 110-121）/ `relay/helper/price.go:53-61` / `service/task_billing.go:276-277` / `service/group.go:60-64` | 实际有 **6 个 hot-path 调用站**用到 `GetGroupRatio` / `GetGroupGroupRatio`。文档暗示的"hot path 一处"是错的。 |
| `model/user.go::GetUserGroup`（行 819-845）+ `getUserGroupCache` | Redis 缓存 + DB fallback 已存在，但**没有 `InvalidateUserCache`**，`updateUserGroupCache` 也只在异步 `gopool.Go` 里写，不能直接当失效函数用。需要新增至少 1 个 invalidator 函数（不是 10 LOC，是 30-50 LOC + 测试）。 |
| `common/redis.go` + `go.mod` | **Fy-api 用 `github.com/go-redis/redis/v8` v8.11.5**。integration §1.1.3 伪代码 `import "github.com/redis/go-redis/v9"` —— **版本不一致**。如果按字面引入 v9 会同时存在两个 redis client，连接池翻倍 + 配置代码重复。**必须统一到 v8**（或先升级整个仓库到 v9，那是另一个 PR）。 |
| `common/init.go::SyncFrequency` 默认 60 + `common/redis.go:32` fallback 60 + `main.go:81/98/102` 启动期 `SyncOptions(common.SyncFrequency)` | 现在的 SyncOptions 是 **polling refresh**（每 60s 全量重读 options），不依赖 Pub/Sub。`UpdateOption` 改加 publish 是 OK 的；但 SyncOptions 这个轮询 goroutine **要不要保留作为兜底**？文档没明示。我倾向"保留，把它作为 publish 失败时的最终一致兜底"。 |
| `OVERLAY.md` | 当前已用编号：B-1, B-1.1, B-2, B-3, B-4, B-5, B-6, B-7, **B-8（gemini）**, **B-9（claude）**, **B-10（relay 500→400）**, **B-11（/v1/messages 二级反序列化）**。integration §1.8 / PRD §C-8 写的 "B-7 ~ B-13" / "B-8 起"已经**完全过时**，新 overlay 必须从 **B-12 起编号**。这是合并前必修。 |
| `model/main.go::LOG_DB` 启动逻辑（行 41-66 / 213-230） | 确认 `LOG_SQL_DSN` 为空时 `LOG_DB = DB`，否则独立连接。`consume_log_outbox` 落 LOG_DB 这条没问题，但当 LOG_DB ≠ DB 时，partner-api 的 `tnbiz_outbox_consumer` 必须**额外**配 LOG_DB DSN——文档 §6.2 GRANT 部分讲了，但没有提醒 Fy-api 侧的 partner-api ConfigMap / Secret 同时要加 `LOG_SQL_DSN`。 |
| `compose.prod.yml` + `scripts/prod/06-deploy-blue-green.sh` | 生产是 **Podman + Nginx 反代**（`127.0.0.1:3001`），**不是 K8s + Istio**。integration §6.4 整段重写后的 mTLS（Istio PeerAuthentication / `X-Forwarded-Client-Cert` / NetworkPolicy）在 Fy-api 现部署形态里**没有任何对应物**，要么换为 Nginx mTLS + IP allowlist，要么集群迁移先做。 |

---

## 3. 9 项附录 C 改动逐项可行性

| ID | 项 | Verdict | 实际工作量（人天） | 主要风险 / 调整 |
|---|---|---|---|---|
| C-1 | 内部路由 + HMAC + mTLS 鉴权 middleware | ⚠️ 需调整 | 2-3 d | 路由组挂载位置必须**避开 `/api` 全局 GlobalAPIRateLimit**；redis 客户端必须用 v8；mTLS 部署模式必须按 Podman+Nginx 重写一份。 |
| C-2 | 内部 controllers（user/token/usage/group/idempotency） | ✅ 可做 | 3-4 d | 13 个 endpoint，每个含 idem 检查 + GORM 事务 + 错误码映射。文档预估 240 LOC 偏低，加上单测会到 ~500-600 LOC。 |
| C-3 | BIGINT 三方言 migration | ❌ 阻塞（必须改方案） | 2 d 编码 + **维护窗口要业务谈** | `logs.id` 在 MySQL 用 `ALGORITHM=COPY` 不可接受。CN 生产 logs 表已经亿级，需要 **gh-ost / pt-online-schema-change** 或者拆"先建影子表再切" 方案。文档应改为"在线 DDL 工具 + 业务静默期"两套预案。SQLite 部分文档说要 table-rebuild 也得真写出来。 |
| C-4 | GroupRatioOverride（per-user 倍率） | ⚠️ 需调整 | 2 d | 调用站不止一处（实测 6 处）；`GetEffectiveGroupRatio` 需要拿 `*model.User`，意味着 hot path 多一次 user 取数（已有缓存可复用），实施时要量化 hot path P99 影响。Pub/Sub 失效需要新写 `InvalidateUserCache`。 |
| C-5 | consume_log_outbox + 同事务写入 | ⚠️ 需调整（**最重要**） | 3 d 编码 + 2 d 压测 | RecordConsumeLog 改 TX **没问题**；但 §1.5.3 注释"上游其他 5 个 LOG_DB.Create 不触发 outbox" 必须**白纸黑字写在代码注释里**，否则下次 upstream 加 `LogTypeXxx` 写 LOG_DB 会被 reviewer 误判要不要进 outbox。需要加 lint 或 review checklist。还要确认 `LogQuotaData` 这个 fire-and-forget goroutine **不进事务**。 |
| C-6 | Pub/Sub publish-after-commit + SyncFrequency 短化 | ⚠️ 需调整 | 1.5 d | publish-after-commit 写法 OK；但 `model.SyncOptions` 老 polling goroutine **不能删**（兜底）。SyncFrequency 从 60 短化到几秒会增加 DB QPS，需要量化。 |
| C-7 | internal_idempotency 表 | ✅ 可做 | 1 d | 单纯建表 + cleanup cron。注意 `UNIQUE(auth_kid, idempotency_key, endpoint)` 在 PG 的字符串长度限制（64+64+128 = 256 byte，OK）。TTL 7d cleanup cron 必须走 leader-only（参考 channel-benchmark 已有 leader 选举模式，或借 `common.IsMasterNode`）。 |
| C-8 | OVERLAY.md 条目编号 | ❌ 阻塞 | 0.5 d | **必须从 B-12 起重编号**。文档目前写的 B-8..B-14 与现状全冲突。 |
| C-9 | LOC 估算 | ⚠️ 偏低 | — | 见 §12 真实工作量。 |

---

## 4. 路由 + 鉴权层 Review

### 4.1 路由组挂载

`router/api-router.go:15` 已经：`apiRouter := router.Group("/api")` + `apiRouter.Use(gzip, BodyStorageCleanup, GlobalAPIRateLimit)`。如果 `/api/internal/*` 直接挂在这个 group 下，partner-api 会被 `GlobalAPIRateLimit` 限制——它正是要做高频 outbox poller / saga retry 的。**建议**：

```go
// router/api-internal-router.go (NEW)
func SetInternalRouter(router *gin.Engine) {
    g := router.Group("/api/internal")
    // 显式不挂 GlobalAPIRateLimit；改用 per-kid quota（Security MED-5 已有要求）
    g.Use(middleware.RouteTag("api-internal"))
    g.Use(middleware.BodyStorageCleanup())
    g.Use(middleware.InternalAuth(...))
    // ... endpoints
}
```

并且在 `router/main.go::SetRouter` 第二行加 `SetInternalRouter(router)`，这是上游 patch 1 行 + 注释。

### 4.2 HMAC middleware

伪代码逻辑（timestamp ±300s + nonce SETNX + HMAC verify）整体 OK。但：

- **CRITICAL：必须用 `github.com/go-redis/redis/v8`**，不要在伪代码里继续写 v9。`SetNX(c, key, "1", ttl)` v8 的签名是 `client.SetNX(ctx, key, value, ttl).Result()`，返回 `(bool, error)`，与 v9 兼容。但 import 路径必须改。
- **HIGH：mTLS `c.Request.TLS != nil` 兜底**——文档 §6.4 已经撤销了这条，但要明确 Fy-api 部署在 Nginx 后面时怎么确保 mTLS。**建议**：CN/SG 用 Nginx 终结 mTLS，Nginx 设置 `proxy_set_header X-Client-Verified $ssl_client_verify` + `X-Client-CN $ssl_client_s_dn_cn`，middleware 校验这两个 header（仅信任来自 loopback 的请求）。
- **MED：endpoint allowlist**——`KeyStore.Lookup` 返回 `allowedEndpoints []string`，必须 case-sensitive 精确匹配 `c.Request.URL.Path`，**不能前缀匹配**（防 `/api/internal/user/topup-extra` 绕过）。

### 4.3 Idempotency middleware

`internal_idempotency` 表 + 手写 middleware 可以做。**但**整个 Fy-api 现在没有任何 idempotency 中间件（grep 全空），意味着上游也不会有冲突——但同时也意味着这是 100% 自己造，需要充分单测。建议封装为独立 file `middleware/internal_idempotency.go`，函数签名与 controller 解耦（controller 只调 `idem.Lookup` / `idem.Save`），便于单测。

---

## 5. 数据模型 + DDL Review

### 5.1 BIGINT 升级（C-3）—— **CRITICAL，需重写**

| 表/列 | MySQL 文档方案 | 真实风险 | 推荐方案 |
|---|---|---|---|
| `users.quota / used_quota / aff_quota / aff_history_quota / request_count` | `ALGORITHM=INPLACE, LOCK=NONE` | OK，users 表 < 100K 行 | 按文档执行 |
| `logs.id`（PK widening） | `ALGORITHM=COPY` | **生产不可接受**，CN/SG `logs` 已亿级，复制 + 重建 5 个二级索引会数小时 | 用 `gh-ost` / `pt-online-schema-change`；备选：**新表 `logs_v2(id BIGINT)`** + dual-write + 切流 + 旧表 archive |
| `logs.quota` | INPLACE OK | 小风险 | OK |

**建议改 integration §1.3.3**：

- 删掉 `ALGORITHM=COPY` 这一行，改写为"`logs.id` 必须走 gh-ost，参考 docs/Phase3-DB-migration-runbook.md 的零停机模式"
- 加一项前置 invariant：执行前必须 `SELECT MAX(id) FROM logs` 确认未越 INT32（21亿）边界；如果接近就要预留 emergency rollback
- PG <12 全表重写也要点名"必须维护窗口"
- SQLite 的 table-rebuild 至少给一个完整 SQL 模板，不能只写"重建所有索引"

### 5.2 `consume_log_outbox`（C-5）—— PASS

DDL 设计合理：`(status, id)` 索引 + `(status, locked_until)` 索引 + `data_region` cross-border guard + 5 态状态机。**唯一问题**：

- `status` enum `dead_letter` 的值用 `VARCHAR(16)` 装得下，但 PG 8.x 没有 `varchar` 长度强制约束差异，OK。MySQL utf8mb4 16 字节也够。
- **MED：`last_error TEXT`**——TEXT 类型 + scrubPII 在三方言里都要测。Fy-api `Rule 2` 强制三库兼容。
- `INDEX idx_inflight_lease (status, locked_until)`——过滤 `status='in_flight'` 时该索引 OK，但 partial index 在 MySQL 8 不支持，PG 支持。请文档显式说"在 PG 上额外建 partial index `WHERE status='in_flight'`，MySQL 用全字段索引"。

### 5.3 GroupRatioOverride 字段

`User.GroupRatioOverride float64 gorm:"default:0"`：上游 `User` struct 已经在频繁演进（最近一次是 SubscriptionId / WebhookEnabled 等加字段），加字段冲突概率中等。**建议在 `User` struct 末尾加** + `// Fy-api overlay: per-customer pricing override (TraceNex Partner)` 注释，merge 时只删除 / 还原这一行。

### 5.4 `internal_idempotency` 表

- `endpoint VARCHAR(128)`：长度够。
- `request_hash VARCHAR(64)`：sha256 hex 64 字符，OK。
- `response_body TEXT`——和 §5.3 integration §5.3 重写为"加密 `response_cipher VARBINARY + response_key_id`" 的字段不一致。**这是 v0.2 文档内部矛盾**：integration §1.7.2（C-7 表 DDL）还是明文 `response_body`，但 §5.3 跨服务幂等表又改成 cipher。**Fy-api 侧的 `internal_idempotency`（C-7）和 partner-api 侧的 `idempotency_record`（§5.3）是两张表，本来就允许字段不同**——但请文档明示这一点，避免实施时混淆。我建议 C-7 也写成 `response_body TEXT` 暂存（Phase 1），KMS envelope 留到 Phase 2A。

---

## 6. Billing Hot Path Review（最重要）

### 6.1 GroupRatioOverride hot-path 改造

当前 hot path（`service/quota.go::CalculatePostConsumeQuota` 等）的 ratio 解析：

```go
groupRatio := ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)
```

要插 override，必须在这两步之前/之间检查 `user.GroupRatioOverride > 0`。但 `relayInfo` 当前**只带了 `UserGroup string`，没带 `*User`**。改造选项：

1. **A：扩展 RelayInfo**——加 `UserGroupRatioOverride float64` 字段，distributor 阶段从 `User.GroupRatioOverride` 拷贝进去。**推荐**，只动 `relay/common/relay_info.go` 一处。
2. **B：传 user pointer**——侵入 6 处调用站签名，merge 风险大。

整个改造影响的文件：
- `relay/common/relay_info.go`（加字段，1 行）
- `middleware/distributor.go` 或 `middleware/auth.go`（distributor 写入 RelayInfo，5 行）
- `service/quota.go`（行 110-121）+ `service/task_billing.go`（行 276）+ `relay/helper/price.go`（行 53-61）+ `service/group.go`（行 60-64）：每处加 `if relayInfo.UserGroupRatioOverride > 0 { groupRatio = override; goto skip }` 风格的分支，**总共 15-20 LOC，分布在 4 个文件**。

文档说的"15 LOC patch"严重偏低；**真实是 30-40 LOC + 4 文件 + 单测**。每周上游同步会在这 4 个文件里都可能撞到（ratio 系统是上游持续演进区）。

### 6.2 outbox 同事务对计费 hot-path 的延时影响

`PostConsumeQuota` 现在的链路（service/quota.go:408+）：

```
DecreaseUserQuota (DB.Update users)
DecreaseTokenQuota (DB.Update tokens)
RecordConsumeLog → LOG_DB.Create(log)   ← 这里要扩为 TX
LogQuotaData (async goroutine)
```

把 `LOG_DB.Create` 扩成 TX `(Create log, Create outbox)` 的延时增量 = 多 1 次 INSERT + 1 次 commit fence。以 RDS 内网为例典型增量 0.5-2 ms（冷连接更高）。**预算**：

- 当前 `RecordConsumeLog` P99 ≈ 5-10ms；扩 TX 后预计 P99 ≈ 7-15ms。
- 整个 `/v1/chat/completions` 响应路径 P99 在 100ms-2s 量级，**绝对值上可忽略**。

但有两个隐患：

- **HIGH：LOG_DB 与 DB 不同库时，TX 在 LOG_DB 内**——OK，没有跨库事务问题。但需要确保 partner-api outbox poller 用的 GORM 连接和 Fy-api 的 LOG_DB 是同实例（已经在 §6.1 GRANT 矩阵里说明，但要在部署 runbook 里再强调一次）。
- **MED：失败语义**——如果 outbox.Create 失败，整个 logs.Create 也回滚，意味着用户的 quota 已扣但 logs 表无记录（`PostConsumeQuota` 里 DecreaseUserQuota 是先发生的，独立事务）。**结论**：这是新增的 failure mode，需要在 PR 描述里明示，并加 alert（`outbox_tx_failures_total` metric）。

### 6.3 同事务 outbox 写入的 lock contention

logs 表 INSERT 是 append-only，没有 row lock。outbox 表也是 append-only。两者同 TX 会持有更长的事务（多约 1ms），但不会引入新的死锁路径。**Fy-api 侧无阻塞，PASS**。

---

## 7. 异步事件 / Pub/Sub Review

- **Redis ACL（Security M-r2-4）**：要求 partner-api 的 `tnbiz_app` 角色无 `option_update` PUBLISH 权限。Fy-api 当前 Redis 是阿里云托管 + AUTH，Redis 6+ 支持 ACL；但**SG/CN 当前实例的 ACL 配置文档里没体现**。这是 ops 侧前置条件，PR 合入前必须确认。
- **Pub/Sub 选型**：integration §3 明确 outbox 走 DB 表 + poller，**没用 RocketMQ / MNS**——这是好事，不引入新中间件。但 `option_update` / `user_update` 走 Redis Pub/Sub，Fy-api 已经依赖 Redis，零增量。
- **SyncFrequency 缩短**：当前 60s。如果短到 5s，`SyncOptions` polling 在 Pub/Sub 失败时能 5s 内兜底，**OK**。但 5s 全量 reload options 对小集群 OK，对大集群（>50 pod）会让 DB QPS 翻倍。Fy-api 当前 CN 单 pod、SG 单 pod，**短化无压力**。
- **rate limit 200ms**：integration §1.6.3 说订阅 goroutine 用 `time.NewTicker(200ms)` 防 reload-storm。**这一段写法有 bug**：用 `<-rateLimit.C` 在每个 message 之前会让消息严格降级到 5 msg/s，但中间的消息会被丢弃（`rdb.Subscribe.Channel()` buffer 满会断订阅）。**建议改成 coalescing**：消息进来设 `dirty = true`，独立 ticker 每 200ms 检查 `if dirty { reload(); dirty = false }`。

---

## 8. 上游 rebase 影响评估（Fy-api 团队最痛的章节）

### 8.1 本次会动到的上游文件清单（按冲突概率排序）

| 文件 | 改动量 | 上游活跃度 | 冲突概率 | 缓解 |
|---|---|---|---|---|
| `model/log.go::RecordConsumeLog` | +25 LOC（TX wrap） | **极高**（日志路径，几乎每周改） | **HIGH** | 集中改一处函数，注释打满，merge 时容易识别 |
| `model/user.go`（加 `GroupRatioOverride` 字段） | +1 LOC | 高（最近 SubscriptionId 等加字段） | **MEDIUM** | 字段加在 struct 末尾 |
| `model/option.go::UpdateOption` | +10 LOC（publish） | 中 | MEDIUM | 加在函数末尾 |
| `main.go::startup` | +30 LOC（订阅 goroutine + idle worker） | 中（启动序列偶尔重排） | MEDIUM | 集中放在 `startup` 末尾，标 `// Fy-api overlay: TraceNex Partner integration` |
| `setting/ratio_setting/group_ratio.go` | +15 LOC（`GetEffectiveGroupRatio`） | 中（ratio 体系上游迭代中） | MEDIUM | 新函数，不修改老 `GetGroupRatio` 签名 |
| `service/quota.go` + `relay/helper/price.go` + `service/task_billing.go` + `service/group.go`（hot-path branch） | 4 文件 × 5 LOC | **极高**（计费 hot path） | **HIGH** | 用 RelayInfo 字段方案最大限度集中 |
| `router/main.go`（加 `SetInternalRouter`） | +1 LOC | 低 | LOW | trivial |
| `router/api-router.go`（无需改，新文件 `api-internal-router.go`） | 0 | — | — | — |

总冲突概率：**每周 sync 期望 1-2 个文件冲突**（log.go + quota.go + 偶尔 user.go），按 `OVERLAY.md` B-2 / B-8 / B-10 / B-11 等已有先例的处理强度判断，**每周需多投入 ops 0.5-1 小时**做冲突 resolve。

### 8.2 能否下沉到上游（开 PR 给 QuantumNous/new-api）

| 改动 | 是否适合 PR 上游 | 理由 |
|---|---|---|
| `consume_log_outbox` 表 + outbox 模式 | ❌ 不建议 | 上游是产品本体，不该承担"分销系统集成钩子"。但**outbox pattern 本身可以发到 upstream 作为 generic event log 能力** —— 那是更大的设计讨论，3 个月以上周期。 |
| `User.GroupRatioOverride` 字段 + `GetEffectiveGroupRatio` | ⚠️ 可考虑 | 这是相对通用的 per-user pricing 能力，上游也有 `GroupRatioSettings` 已经够用，但**单标量 override** 概念上游可能愿意接。**建议先观察 Phase 1 实施稳定后，2026 Q3 提 RFC**。 |
| `option_update` Pub/Sub | ⚠️ 可考虑 | 上游已经有 `SyncOptions` polling，缩短 polling 周期或加 publish 是明显改进。**可发 PR**。 |
| BIGINT 升级 | ✅ **强烈建议下沉** | 这是上游本身欠的债，`Log.Id INT` 在大型部署都会撞。Fy-api 提 PR 给上游同时缩短我们的债务期。 |
| 内部 API 路由 + HMAC 鉴权 | ❌ 不下沉 | 完全是 TraceNex 业务定制，不该污染上游。 |
| `RecordConsumeLog` 改 TX | ⚠️ 上游应该要做 | 当前非事务的 logs.Create 是个一致性 bug（系统重启时丢日志可能性）。**可单独发 PR 上游做"`RecordConsumeLog` 加 TX wrapper（不带 outbox）"**，把 TX 框架融进 upstream，我们 OVERLAY 只在 TX 里加 outbox.Create 一行。这样冲突面从"覆盖整个函数"缩到"加一行 INSERT"。**强烈推荐**。 |

### 8.3 OVERLAY.md 新条目建议

```markdown
### B-12 [tnbiz] TraceNex Partner 集成内部 API 路由 + HMAC 鉴权
- **新增文件**：
  - router/api-internal-router.go
  - middleware/internal_auth.go
  - openapi/internal-api.yaml（新建目录）
- **修改文件**：router/main.go（+1 行 SetInternalRouter）
- **冲突风险**：低
- **Merge 策略**：router/main.go 的 1 行加在 SetRelayRouter 之后即可

### B-13 [tnbiz] 内部 controllers
- **新增文件**：controller/internal_user.go / internal_token.go / internal_usage.go / internal_group.go / internal_idempotency.go
- **冲突风险**：极低（独立文件）

### B-14 [tnbiz] BIGINT 升级（users / logs）
- **新增文件**：migrations/2026_05_xx_widen_quota_to_bigint.go
- **冲突风险**：中（与 model/main.go 的 AutoMigrate 启动期协同）
- **执行说明**：MySQL 走 gh-ost；PG <12 维护窗口；SQLite table-rebuild

### B-15 [tnbiz] User.GroupRatioOverride + GetEffectiveGroupRatio
- **修改文件**：model/user.go（+1 字段）；setting/ratio_setting/group_ratio.go（+1 函数）；relay/common/relay_info.go（+1 字段）；service/quota.go / relay/helper/price.go / service/task_billing.go / service/group.go（每处 +5 LOC）
- **冲突风险**：HIGH（hot-path 多文件）
- **Merge 策略**：每个 hot-path 修改点都加 `// Fy-api overlay: TraceNex Partner pricing override (B-15)` 注释

### B-16 [tnbiz] consume_log_outbox 表 + RecordConsumeLog TX wrap
- **新增文件**：model/log_outbox.go；migrations/2026_05_xx_consume_log_outbox.go
- **修改文件**：model/log.go::RecordConsumeLog（包成 TX）
- **冲突风险**：HIGH（log.go 是上游高活跃区）

### B-17 [tnbiz] Pub/Sub option_update + user_update
- **修改文件**：model/option.go::UpdateOption（+publish）；model/user.go::InvalidateUserCache（NEW 函数）；main.go::startup（+订阅 goroutine）
- **冲突风险**：MEDIUM

### B-18 [tnbiz] internal_idempotency 表
- **新增文件**：model/internal_idempotency.go；migrations/2026_05_xx_internal_idempotency.go
- **冲突风险**：极低
```

**整体上游 rebase 成本**：在月度 sync 节奏下，平均**每月+2-4 小时人工 resolve**；如果 PR 上游 `RecordConsumeLog TX wrap` 成功（建议），降回 1-2 小时。

---

## 9. 部署 + 运维 Review

### 9.1 partner-api 与 Fy-api 同实例不同 DB

PRD §6.3 的"同实例不同 DB"——Fy-api 现在通过 `SQL_DSN` + `LOG_SQL_DSN` 已支持双库，再加 partner-api 的 `partner_db` 等于第三个 DSN。**Fy-api 进程不需要直连 partner_db**（应用层 batch lookup），所以 Fy-api 的连接池配置不变。

**风险**：如果 SG/CN 阿里云 RDS 实例选了"两个 schema 在同一实例"还是"两个独立 RDS 实例"。文档假设是前者（跨库 JOIN 可用），但 SG region-isolated 时通常是**独立 RDS**——文档 §6.3 已经说"LOG_DB 拆分时跨库 JOIN 不可用 → fallback HTTP API"，OK。

### 9.2 Secret 管理（HMAC 共享密钥）

Fy-api 现在的 secret 注入方式是 `/opt/fy-api/config/fy-api.env`（compose.prod.yml `env_file`）。新增 HMAC `KEY_STORE_*` 配置走同样路径即可，**但**：

- KMS / Vault 这类中央 secret manager 在 Fy-api 当前部署里**没有**。文档 §6.3 说"CSPRNG-generated 32 字节 + KMS Secret Manager" 是个未来状态。Phase 1 用 env-file 是 acceptable，必须文档明示"Phase 1: env-file，Phase 2A 接 KMS"。
- HMAC secret rotation 走 Pub/Sub `hmac_key_update` 频道：Fy-api 进程订阅，热加载。**实施需要**：env 变更后还要给运行中的 Fy-api 推消息（或者重启）。整套机制需要新写一个 admin endpoint `/api/admin/internal/keys/rotate` 触发——文档里没明确这一段。

### 9.3 监控

新增的 metrics（§9.3）`outbox_lag_seconds` / `internal_auth_failures_total` 这些是 partner-api 暴露的（partner-api 跑 outbox poller）。**Fy-api 侧需要新增的 metrics**：

- `internal_idempotency_hits_total{kid, endpoint}`
- `internal_idempotency_conflicts_total{kid, endpoint}`（409）
- `consume_log_outbox_writes_total{result}`（写 TX 成功 / 失败）
- `consume_log_outbox_tx_duration_seconds`（histogram，把 `RecordConsumeLog` TX 延时纳入）

Fy-api 当前 Prometheus 端点（`/api/perf-metrics` 与 admin route）需要扩展。这是**约 0.5 人天**的活，文档没单列。

---

## 10. 回归 + 回滚策略

### 10.1 回归风险

- **公开 `/v1/*` 路径**：完全不受影响，新路由在 `/api/internal/*`。✅
- **dashboard `/dashboard/*`**：不受影响。✅
- **既有 `/api/log/export`（B-2）/ `/api/channel/*` admin**：不受影响。✅
- **billing path P99**：见 §6.2，绝对值增量 1-2ms，可接受。
- **i18n / web/classic/dist**：完全不受影响。
- **upstream-sync CI**：因为新 overlay 加了 6+ 个上游文件 patch，`merge` 阶段冲突率上升 — 需要更新 `docs/Weekly-upstream-sync-runbook.md` checklist。

### 10.2 回滚策略

**CRITICAL：必须有 feature flag**。建议：

- 加 `biz_setting.tnbiz_internal_api_enabled bool`（默认 false）：控制 `SetInternalRouter` 是否注册
- 加 `biz_setting.tnbiz_outbox_enabled bool`（默认 false）：控制 `RecordConsumeLog` TX 是否写 outbox
- 加 `biz_setting.tnbiz_user_override_enabled bool`（默认 false）：控制 hot-path 是否查 `GroupRatioOverride`

这三个 flag 让我们能在生产事故时**不用回滚 deploy 就能关掉**OVERLAY。文档里没提任何 feature flag——**强烈要求加上**。

回滚路径：

1. flag 关掉（秒级生效，需要 SyncOptions / Pub/Sub 推一次）
2. 如果 flag 不够，git revert OVERLAY 提交并紧急 deploy
3. DB schema 变更（BIGINT、新表）**只前向兼容，不回滚**——这是 SQL migration 通行准则；新表 drop 风险低，BIGINT 不能再 narrow

---

## 11. Fy-api 侧建议的修改清单

### CRITICAL（必须修改文档才能进 Phase 1）

| # | 改 integration-design 哪节 | 怎么改 |
|---|---|---|
| 1 | §1.8（C-8 OVERLAY 编号） | 改成"从 **B-12** 起"（B-7..B-11 已被占）；本 review §8.3 给了完整模板 |
| 2 | §1.1.3（HMAC middleware 伪代码） | 把 `import "github.com/redis/go-redis/v9"` 改成 `"github.com/go-redis/redis/v8"`；调用形态对齐 v8 ctx-first 签名 |
| 3 | §1.3.3（BIGINT migration MySQL） | 删除 `ALGORITHM=COPY` 单行方案；改写为"`logs.id` 必须用 gh-ost / pt-osc 在线 DDL，全量复制 + cutover；必须维护窗口走压力低谷"，引用 `Phase3-DB-migration-runbook.md` |
| 4 | §6.4（mTLS 终结层） | 文档假设 Istio/Linkerd；Fy-api 实际跑 Podman+Nginx。增加一节"Phase 1 部署在 Nginx + mTLS 模式：Nginx 终结 client cert，注入 X-Client-CN 给 gin middleware"，把 K8s mesh 那一套标"Phase 2A 集群迁移后启用" |
| 5 | 新增"feature flag 节" | 必须列出三个 `tnbiz_*_enabled` flag 作为回滚开关；文档现在完全没有 |

### HIGH

| # | 节 | 改法 |
|---|---|---|
| 6 | §1.4.2（GetEffectiveGroupRatio 一行替换） | 改为"扩展 RelayInfo 字段 + 4 个 hot-path 文件分别 patch"；列清单 |
| 7 | §1.6.3（订阅 goroutine rate-limit） | 改 ticker 模式为 coalescing（dirty flag + 独立 ticker） |
| 8 | §1.5.3（RecordConsumeLog TX wrap） | 增加注释要求"`LogQuotaData` fire-and-forget goroutine 必须放在 TX commit 后"；增加 5 个非 consume LOG_DB.Create 调用站的 lint checklist |
| 9 | §1.7.2（C-7 internal_idempotency 表 DDL） | 与 §5.3 的 cipher 字段语义对齐；明示 Phase 1 是 `response_body TEXT`，Phase 2A 切 `response_cipher` |
| 10 | §1.1（路由组挂载） | 明示 `/api/internal/*` **不**继承 `apiRouter` 的 GlobalAPIRateLimit；改用 per-kid quota |

### MEDIUM

| # | 节 | 改法 |
|---|---|---|
| 11 | §6.2（GRANT） | 新增提醒"partner-api 在 LOG_DB 与 DB 不同实例时必须配双 DSN（Fy-api ConfigMap 的 LOG_SQL_DSN 同步给 tnbiz_outbox_consumer）" |
| 12 | §3.1（outbox struct） | `last_error TEXT` 在三方言下都得测；`ChannelId int` 与 PG 默认 INTEGER (32) 兼容 |
| 13 | §1.6.1（SyncFrequency 短化） | 明示"polling SyncOptions goroutine 不删（兜底）"；给 SyncFrequency 默认值变更建议范围（5-15s） |
| 14 | §9.3（metrics） | 加 Fy-api 侧 4 个新 metrics（§9.3 上面列出） |

### LOW

| # | 节 | 改法 |
|---|---|---|
| 15 | §11（变更管理） | 写明 `Fy-api/openapi/internal-api.yaml` 必须随 OVERLAY PR 创建（目录现不存在） |
| 16 | §1.4.4 | F-1 决策延后到 Phase 2A，文档已经写 "Phase 1 不开发"——OK，但建议加 explicit `_TODO_PHASE2A` lint |

---

## 12. Fy-api 团队工作量估算（vs PRD 附录 C-9 暗示的 1-1.5 周）

| 模块 | 编码 | 单测 | 集成测 | 文档 / OVERLAY | 小计 |
|---|---|---|---|---|---|
| 内部路由 + HMAC middleware | 1.5 d | 1 d | 0.5 d | 0.5 d | **3.5 d** |
| 内部 controllers（13 endpoint） | 2.5 d | 2 d | 1 d | 0.5 d | **6 d** |
| BIGINT migration（含 gh-ost runbook） | 1.5 d | 0.5 d | 1 d（dev/staging） | 0.5 d | **3.5 d** |
| consume_log_outbox + RecordConsumeLog TX | 1.5 d | 1 d | 1 d（含压测） | 0.5 d | **4 d** |
| GroupRatioOverride（4 文件 hot-path） | 1.5 d | 1 d | 0.5 d | 0.5 d | **3.5 d** |
| Pub/Sub option_update + user_update + InvalidateUserCache | 1 d | 1 d | 0.5 d | 0.5 d | **3 d** |
| internal_idempotency 表 + cleanup cron | 0.5 d | 0.5 d | 0.5 d | — | **1.5 d** |
| OpenAPI spec `internal-api.yaml` + dredd / schemathesis 接入 | 0.5 d | — | 0.5 d | — | **1 d** |
| feature flag 框架（3 个 biz_setting） | 0.5 d | 0.5 d | — | — | **1 d** |
| 部署 runbook 更新 + Nginx mTLS 配置 | — | — | 0.5 d | 1 d | **1.5 d** |
| 上游 sync runbook 更新（B-12..B-18 条目） | — | — | — | 0.5 d | **0.5 d** |
| **合计** | | | | | **~29 人天** ≈ **6 周（1 工程师）** 或 **3 周（2 工程师并行）** |

vs PRD §C-9 暗示的 ~575 LOC / 1-1.5 周 = **欠估 4-5 倍**。**这必须在排期前对齐**，不能让 partner-api 团队基于 1 周假设规划他们的 Phase 1。

---

## 13. Fy-api 团队的反向请求

在 Fy-api 开工前，**TraceNex Partner 团队 / 业务方 / ops 必须先解决以下事项**，否则 Fy-api 不能启动：

1. **【ops】mTLS 终结层选型确认**：Phase 1 走 Nginx mTLS 还是 Istio？如果是 Nginx，谁负责 SG/CN Nginx config 的 PR？我们不要求做完，但要求 **ops 在 Fy-api 编码 Week 1 前给出 confirmed 选型 + Nginx vhost 草案**。
2. **【ops】Redis ACL 矩阵**：CN/SG 阿里云 Redis 实例是否升到 6.x 并启用 ACL？`tnbiz_app` / `fy_api_app` 角色的 PUBLISH/SUBSCRIBE 权限矩阵谁配？前置条件，PR 合入前必须 ready。
3. **【ops】LOG_DB 拓扑确认**：CN/SG 当前 logs 表在不在主 RDS（`SQL_DSN`），还是已经拆到独立实例（`LOG_SQL_DSN`）？outbox 表必须落 LOG_DB。如果尚未拆，Phase 1 是不是要先拆？
4. **【ops】gh-ost / pt-osc 工具链就绪**：BIGINT 在 MySQL `logs.id` 上必须用在线 DDL。CN 的 RDS 是阿里云托管 MySQL 8.0，gh-ost 兼容 OK；SG 同。但 ops 需要预先在 staging 跑通一次。
5. **【partner-api 团队】HMAC keystore 接口契约**：`KeyStore.Lookup(kid)` 这个接口的实现方是 partner-api 还是 Fy-api？我倾向 **Fy-api 内嵌实现**（key 数据放 fy_api_db `internal_api_key` 表），partner-api 通过 staff verb 管理。需要 partner-api 团队确认。
6. **【partner-api 团队】Phase 1 是否真的需要 KMS envelope（§5.3 idempotency_record cipher）**？如果是 partner-api 那张表的事，与 Fy-api 无关；如果延伸到 Fy-api 的 `internal_idempotency` 表，那 Fy-api 需要新接 KMS SDK，**估算 + 7 人天**。要求文档明示。
7. **【业务方】PRD §C-9 LOC + 1-1.5 周估算与本 review §12 的 29 人天**之间的差异谁来认账？这关系到 Phase 1 排期 commit。Fy-api 团队 push back 不能在 1.5 周内交付。
8. **【业务方】feature flag 名称 + 默认值确认**：`tnbiz_internal_api_enabled` / `tnbiz_outbox_enabled` / `tnbiz_user_override_enabled` 三个 flag 的默认值是 false（影子模式）还是 true（直接生效）？Fy-api 倾向**初次 deploy 默认 false**，灰度开起来。

---

## 14. 建议的 PR 拆分计划（Fy-api 侧）

把 OVERLAY 改动拆成 **5 个独立 PR**，每个可以独立 merge / 独立回滚：

### PR-1：`B-14 BIGINT migration（仅 schema）`
- **范围**：`migrations/2026_05_xx_widen_quota_to_bigint.go`；不改任何 Go 代码
- **测试**：dev / staging 三方言全跑；CN/SG staging 用 gh-ost 跑通
- **回滚**：BIGINT 字段不能 narrow，**实际上不可回滚**——所以这个 PR 必须最稳，单独走
- **依赖**：无
- **预计 lead time**：1.5 周（含 ops 调度维护窗口）

### PR-2：`B-12 + B-13 + B-18 内部路由 + controllers + idempotency 表（影子模式）`
- **范围**：所有 controllers / middleware / migrations，但用 `tnbiz_internal_api_enabled=false` 守护，路由不挂载
- **测试**：单测 + 内部 staging 打开 flag 跑契约测试（dredd / schemathesis）
- **回滚**：flag 关掉
- **依赖**：PR-1（需要 BIGINT 已上）
- **预计**：2 周

### PR-3：`B-17 Pub/Sub option_update + user_update + InvalidateUserCache`
- **范围**：`model/option.go::UpdateOption` + `main.go::startup` + `model/user.go::InvalidateUserCache`
- **测试**：多 pod 集成测试（CN 是单 pod，但 SG 蓝绿期间是双 pod）
- **回滚**：删 publish 不影响功能（polling 兜底）
- **依赖**：无
- **预计**：1 周

### PR-4：`B-15 GroupRatioOverride hot-path（默认关闭）`
- **范围**：`User.GroupRatioOverride` 字段 + `RelayInfo` 字段 + 4 个 hot-path 文件 patch + `GetEffectiveGroupRatio` 函数；用 `tnbiz_user_override_enabled=false` 守护
- **测试**：hot-path 单测 + 端到端 chat completion + 性能回归（P99 不退）
- **回滚**：flag 关掉，hot-path 退回 `GetGroupRatio`
- **依赖**：PR-2（需要 internal endpoints 才能写 override）
- **预计**：1.5 周

### PR-5：`B-16 consume_log_outbox + RecordConsumeLog TX（默认关闭）`
- **范围**：建表 + `model/log_outbox.go` + `RecordConsumeLog` TX wrap；用 `tnbiz_outbox_enabled=false` 守护——flag 关时仍走原 `LOG_DB.Create(log)` 单语句
- **测试**：billing 端到端 + outbox poller mock + 失败注入（outbox.Create 失败 → logs.Create 也回滚验证）+ P99 回归
- **回滚**：flag 关掉
- **依赖**：PR-1（需要 BIGINT），PR-2（不严格依赖，但便于联调）
- **预计**：1.5 周（含压测）

**总周期**：5 PR 串行最坏 7-8 周；**PR-1 + PR-3 可并行**，PR-4 + PR-5 上线后可并行测，**实际 5-6 周可全部上完（1 工程师专职）**。

---

## 15. 收尾

文档质量整体在 v1.0 是合格的（4 reviewer round-2 都 PASS），架构方向我们认可。但 **对 Fy-api 的实施成本欠估、对 Fy-api 当前部署形态（Podman+Nginx，非 K8s）信息不准、OVERLAY.md 编号错位** 这三件事必须在 Phase 1 启动前对齐。

如果 TraceNex Partner 团队接受本 review §11 的 CRITICAL 5 项 + HIGH 5 项修订、§13 的 8 个反向请求得到 ops/业务方 commit，Fy-api 团队可以在确认排期后启动 PR-1。本 review 的最终 verdict 维持 **ACCEPT-WITH-CHANGES**，下一步等 v1.1 修订版 + 反向请求闭环。

—— Fy-api Tech Lead，2026-05-12
