# Round 1 代码 Review — Fy-api 团队侧（B-12 .. B-18）

> 角色：Fy-api / TraceNex tech lead
> 范围：`overlay/tnbiz-b12-b18` 分支 PR-2..PR-5 实际落地的 Go 代码
> 输入文档：`integration-design.md` v1.2、`OVERLAY.md`、`OVERLAY-TNBIZ-HANDOFF.md`、03-fy-api-round3-final.md
> Review 维度：契约一致性 / billing 安全 / 客户端实现 / upstream rebase 风险

---

## 0. 执行摘要 + Verdict

**Verdict：ACCEPT-WITH-CHANGES**

代码层面整体落地度高，OVERLAY B-14 / B-15 / B-16 / B-18 的关键不变量（feature flag 全 prod-off、HMAC 加密落库、同事务双写、AES-GCM、nonce SETNX fail-closed）都按 v1.2 契约拿到。`go build` 应该过，单测覆盖了核心 invariant。回归风险方面，flag 全 off 时 Fy-api 行为与 main 字节级一致这一条 invariant 在代码里 grep 得到证据（`log_outbox.go:36`、`effective_group_ratio.go:19`、`internal_auth.go:53`、router 里 `overlay.IsInternalAPIEnabled` flag-gate），可以接受。

但是有几个 **CRITICAL** 必须在 PR 合并前修复，才能让 partner-api 真正能调通：

1. **HMAC 签名 header 名 Fy-api 用 `X-Tnb-*`，partner-api client 用 `X-Auth-*`，integration-design v1.2 §1.1.3 也写的是 `X-Auth-*`**——三方完全对不上，partner-api 调 Fy-api 100% 401。
2. **HMAC 签名 canonical 串 Fy-api 缺 `canonical_query` 项，且签名编码 hex vs base64 不一致**——即使 header 名修齐，签名比对也必然失败。
3. **HMAC nonce TTL Fy-api 用 24h，integration-design 用 5min**——契约偏差。
4. **`/api/internal/*` 写接口里 idempotency-key 没强制必传**，OpenAPI 写"写接口必传"但 controller 没拒，违反 §1.2.3 骨架要求。

剩下 HIGH / MEDIUM 见 §6。

如果上述 4 条 CRITICAL 在 Round 2 内修齐，就转 ACCEPT。

---

## 1. B-12..B-18 七条 OVERLAY 实际落地度逐项核对

### B-12：`/api/internal/*` 路由组 + HMAC 鉴权

| 契约条款（v1.2 §1.1.1 / §1.1.3）              | 实际落地                                                                                                                       | 结论 |
| --- | --- | --- |
| `/api/internal/*` 独立路由组，**不**继承 `apiRouter` 的 `GlobalAPIRateLimit`     | `router/api-internal-router.go:21` `g := router.Group("/api/internal")` 不挂全局限流 | 符合 |
| HMAC-SHA256 鉴权 middleware                                            | `middleware/internal_auth.go:51 InternalAuth()`                              | 符合 |
| 双 flag 校验：InternalAPI ON + HMACKeystore ON 才放行                       | `internal_auth.go:53` `if !overlay_flag.IsInternalAPIEnabled() \|\| !overlay_flag.IsHMACKeystoreEnabled()` 503 | 符合 |
| HMAC 头名 `X-Auth-KeyId / X-Auth-Timestamp / X-Auth-Nonce / X-Signature`（integration-design §1.1.3 行 119-122） | `internal_auth.go:37-40` 用了 `X-Tnb-Key-Id / X-Tnb-Timestamp / X-Tnb-Nonce / X-Tnb-Signature` | **不符** |
| canonical = `method\npath\ncanonical_query\nts(int)\nnonce\nsha256(body)`（integration-design 行 162、行 166-173） | `internal_auth.go:142-149` canonical 缺了 `canonical_query`，且字段顺序为 `method\npath\nts\nnonce\nkeyId\nbody_hash`，与契约不一致 | **不符** |
| 签名编码 base64（integration-design §1.1.3 行 122 `base64(HMAC-SHA256(...))`）| `internal_auth.go:151-156` 用 `hex.EncodeToString` 比对，又用 `subtle.ConstantTimeCompare` —— 编码档位与契约相反 | **不符** |
| nonce TTL 5min（行 127 `NonceTTL = 5 * time.Minute`）                    | `internal_auth.go:43` `hmacNonceTTL = 24 * time.Hour`                       | **不符** |
| Clock skew ±5min                                                       | `internal_auth.go:42` `5 * time.Minute`                                      | 符合 |
| nonce SETNX go-redis/v8 ctx-first                                      | `internal_auth.go:96-105` `ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second); RDB.SetNX(ctx, ...).Result()` 标准 v8 签名 | 符合 |
| Redis 故障 fail-closed                                                 | `internal_auth.go:99-102` redis err 即 return，401                            | 符合 |
| Endpoint allowlist 精确匹配（防前缀绕过）                                 | `internal_auth.go:117-134` 严格 `==` 匹配                                     | 符合 |
| trace_id 透传（`X-Oneapi-Request-Id`）                                  | middleware **未读取 / 转发** `X-Oneapi-Request-Id` —— integration §1.1.3 行 181-184 要求设入 ctx | **缺失** |

**结论**：B-12 路由层面没问题，但 HMAC 协议字段与契约偏差是阻断级（partner-api 100% 调不通）。详见 §6 CRITICAL。

### B-13：Internal Controllers + ChannelLogSetting upsert

| 契约 | 落地 | 结论 |
| --- | --- | --- |
| `controller/tnbiz_internal/*` 独立子包                                  | `controller/tnbiz_internal/{health,user,token,settings,context}.go` 全在  | 符合 |
| envelope `{success, data, message/error}`                              | `health.go:36-51` 走两套 helper（成功用 `data`，失败用 `error`）           | 符合 |
| Token create 不返回明文 sk-key                                          | `token.go:90-95` 只返回 `TokenId / MaskedKey / DeliveryHandle`            | 符合（设计很谨慎） |
| sk-key 明文走 5min 一次性 redis handle                                  | `token.go:103-115` `RDB.Set(... 5*time.Minute)`                          | 符合 |
| 写接口必传 Idempotency-Key（OpenAPI §2 / §1.2.3 行 252-258 显式要求 400 BIZ_IDEM_KEY_REQUIRED） | `user.go::Topup` / `AdjustQuota` / `Refund` / `token.go::CreateToken` / `settings.go` 都**没有**前置检查 idem-key 为空就 400 | **不符** |
| Saga compensate / refund 接口签名（`saga_id`、`order_ref`）              | `user.go:99 AdjustQuotaRequest{SagaId}` / `144 RefundRequest{SagaId, OrderRef}` 有                                 | 符合 |
| Quota delta 0 拒绝                                                     | `user.go:108-111` 显式拒                                                   | 符合 |
| 错误码常量 `BIZ_IDEM_KEY_REQUIRED / BIZ_VALID_BODY / BIZ_IDEM_REUSED_DIFFERENT_BODY` | `user.go` 用 `"invalid_request" / "user_not_found" / "topup_failed"`，**没用** integration §6.5 / §1.2.3 规定的 BIZ_* 命名 | **不符** |
| ChannelLogSetting upsert，phase-1 schema-only                           | `model/channel_log_settings.go:14-42` schema + Upsert 都到位；不接 channel hot path（OVERLAY.md B-13 注明） | 符合 |

### B-14：Feature flag 框架

| 契约（OVERLAY.md B-14 / Round-2 §11.5） | 落地 | 结论 |
| --- | --- | --- |
| 5 个 flag key                                    | `flag.go:24-30` 5 个常量                                                      | 符合 |
| atomic-cached 高频 flag（hot path 不走锁）      | `flag.go:43-49` `atomic.Bool / atomic.Value`                                  | 符合 |
| 5-15s polling 兜底刷新                           | `flag.go:54` 默认 10s                                                          | 符合 |
| ctx-first poller，ctx 取消即停                  | `flag.go:123-137`                                                              | 符合 |
| 默认值 prod-safe（all off / shadow）             | `flag.go:66-72` 全部 `false / OutboxOff`                                       | 符合 |
| backend：`common.OptionMap`（biz_setting 表）   | `flag.go:101-108`                                                              | 符合 |
| 测试注入点                                       | `flag.go:91-97 SetLoader`、`flag.go:201-207 SetForTest / SetPollIntervalForTest` | 符合 |

**风险**：`flag.go:53-55` `pollInterval` 是包级 var，没用 `sync.Once` / 没读 OptionMap 里的 `overlay.poll_interval_sec`。生产想从 OptionMap 调 polling 频率得改代码、重启。MEDIUM。

### B-15：GroupRatioOverride hot path（6 调用站 / 4 文件）

| 调用站                                                  | 落地行号 | 串到 ApplyOverride？ | 结论 |
| --- | --- | --- | --- |
| `relay/helper/price.go::HandleGroupRatio` user-group 分支 | 行 57 `userGroupRatio = ratio_setting.ApplyOverride(relayInfo.UserGroupRatioOverride, userGroupRatio)` | yes | 符合 |
| `relay/helper/price.go::HandleGroupRatio` normal-group 分支 | 行 65 `groupRatioInfo.GroupRatio = ratio_setting.ApplyOverride(...)` | yes | 符合 |
| `service/quota.go::PreWssConsumeQuota` 默认分支            | 行 112                       | yes | 符合 |
| `service/quota.go::PreWssConsumeQuota` autoGroup 分支     | 行 119                       | yes | 符合 |
| `service/quota.go::PreWssConsumeQuota` userGroupRatio 分支 | 行 128                      | yes | 符合 |
| `service/task_billing.go` task 路径（无 RelayInfo）        | 行 281-284 直接 `model.LookupUserOverride` 回库 | yes | 符合 |
| 加分项：`service/group.go::GetUserGroupRatioWithOverride` cold-path helper | 行 75-84 | yes | 符合 |

**hot path 安全**：

- `effective_group_ratio.go:18-26 ApplyOverride` 实现：1 atomic load + 1 浮点比较；flag off 时直接 return fallback，零成本。✅
- `relay_info.go:498-508` 在 `genBaseRelayInfo` 里 best-effort 从 ctx 拷 override，缺失时 `overrideLookup(userId, usingGroup)` **回库一次**；这是新加的 DB 查询，每个非 ctx 注入的 relay 请求都会触发。MEDIUM 性能影响（详见 §3）。
- `override_lookup.go:14 atomic.Value` callback registry 优雅地避开 `relay/common -> model` 循环导入。✅
- `model/group_ratio_override.go:62 LookupUserOverride` 没缓存，每次回库；hot path 命中频率高时建议加 LRU。MEDIUM。
- `LookupUserOverride` 走 `(user_id, group, status=1)` 索引但 schema 上 `idx_gro` 是 `(partner_kid, user_id, group)` UNIQUE 索引（行 18-20），**前导列不是 user_id**——查询会全表扫或用次级索引。需补一个 `(user_id, group, status)` 二级索引。HIGH。

**Feature flag**：`overlay.group_ratio_override` 默认 false，逐 partner 灰度。✅
**回滚路径**：flag off → `ApplyOverride` 直接返回 fallback。✅

### B-16：consume_log_outbox + RecordConsumeLog 同事务写 outbox

| 契约 | 落地 | 结论 |
| --- | --- | --- |
| flag off → 单语句 `LOG_DB.Create(log)`                                 | `log_outbox.go:36-39` `if !IsOutboxTxEnabled() \|\| OutboxMode() == OutboxOff { return LOG_DB.Create(log).Error }` | 符合 |
| flag on → `LOG_DB.Transaction(Create log + Create outbox)`             | `log_outbox.go:40-71`                                                                                                | 符合 |
| `data_region` 字段                                                     | `consume_log_outbox.go:40` `varchar(8); not null; index idx_outbox_region_status priority 1`                         | 符合 |
| region 来自 `DATA_REGION` env                                          | `log_outbox.go:26-32 dataRegion()`                                                                                    | 符合 |
| status 5 态：pending/in_flight/published/failed/dead_letter           | `consume_log_outbox.go:19-24`                                                                                         | 符合 |
| 乐观锁 lease（`(status, locked_until)` 索引）                          | `consume_log_outbox.go:74-113 LeaseOutboxBatch`                                                                       | 符合 |
| retry≤10 → DLQ                                                         | `consume_log_outbox.go:138-142 MarkOutboxFailed`                                                                      | 符合 |
| 仅 `LogTypeConsume` 走 outbox（其他 LOG_DB.Create 调用站不动）          | `model/log.go:244-249` 顶部加了 invariant 注释                                                                          | 符合 |
| LogQuotaData fire-and-forget 在 TX 之后                                 | `model/log.go:253-258` TX commit 后 `gopool.Go`                                                                        | 符合 |
| outbox payload schema                                                  | `log_outbox.go:45-58` 序列化 `log_id / user_id / channel_id / model_name / quota / prompt_tokens / completion_tokens / request_id / created_at` | 符合 v1.2 §1.5.3 字段集 |

**安全审计**（详见 §3）：
- TX 失败回滚的语义保证 quota 已扣 ↔ log + outbox 同时存在；不会出现 quota 扣了但 log 缺失的孤儿态。✅
- payload `Marshal` 失败也会让 TX 回滚 → 失败模式正确。✅
- 但是 `log_outbox.go:62-69` outbox 记录没写 `partner_kid`——partner 维度 fan-out 时 publisher 拿不到归属。HIGH。
- `MarkOutboxFailed:130-143` 用 `Save(&rec)` 更新整行，并发场景下可能覆盖 publisher 同时设 `LockedUntil` 的字段。建议显式 `Updates(map)`。MEDIUM。

### B-17：Outbox publisher

| 契约 | 落地 | 结论 |
| --- | --- | --- |
| Publisher interface + NoopPublisher                                    | `service/outbox/runner.go:30-47`                                       | 符合 |
| Runner batch=50 / lease=30s / interval=2s                              | `runner.go:50-53`                                                      | 符合 |
| shadow 模式只 simulate 不真发                                          | `runner.go:114-119` shadow 时 inject `noopShadow`                       | 符合 |
| MNS SDK 真实接入留 Phase 2A（避免引入 aliyun-sdk-go 依赖）             | OVERLAY.md B-17 + HANDOFF §9 显式登记                                   | 符合 |
| ctx-cancel 即停                                                        | `runner.go:81-95`                                                       | 符合 |
| flag off 即跳过 lease                                                  | `runner.go:98-102 if mode == OutboxOff { return }`                      | 符合 |
| region 从 env 注入 + 强制 region 隔离 invariant                        | `main.go:320 outbox.NewRunner(common.GetEnvOrDefaultString("DATA_REGION", "cn"), ...)` | 符合 |

**问题**：
- `runner.go:131 var noopShadow = &NoopPublisher{}` 是包级单例，长期运行下 `Sent / LastBody` 单调累积 + `LastBody` 一直 hold 一份 byte slice，**内存泄漏 / 误导诊断**。MEDIUM。
- runner.go 没有任何 metric（已发数 / DLQ 数 / lease 抖动）。HIGH（运维盲飞）。
- `runner.go:114 process` 在 shadow 模式下直接绕开了真实 publisher，**`r.publisher = NoopPublisher{}` 时 enabled 模式行为退化为 noop**——日志里看到 enabled 但是没真发，运维难调。HIGH（详见 §6）。

### B-18：internal_idempotency + internal_api_key（HMAC keystore + idempotency 表）

| 契约 | 落地 | 结论 |
| --- | --- | --- |
| HMAC keystore: AES-GCM 加密落库                                        | `model/internal_api_key.go:110-125 encryptAESGCM` GCM mode               | 符合 |
| KEK 从 `common.CryptoSecret` 派生                                      | `internal_api_key.go:103-108 deriveKEK = sha256(secret \|\| "tnbiz/internal-api/v1")` | 符合 |
| 明文 secret 永不入库                                                   | `internal_api_key.go:76-97 CreateInternalAPIKey` 仅写 cipherText          | 符合 |
| `LookupInternalAPIKey` 未找到 / disabled 都返回相同 error（不可探测） | `internal_api_key.go:48-61` 全部 `ErrInvalidInternalAPIKey`               | 符合 |
| `internal_idempotency` 表 UNIQUE(auth_kid, idem_key, endpoint)        | `internal_idempotency.go:21-23 idx_internal_idem unique priority 1/2/3` | 符合 |
| 7 天 TTL 由 cron 清理                                                  | `internal_idempotency.go:67-74 CleanupExpiredIdempotency` 函数已写，但 HANDOFF §9 自认未挂调度 | 符合（debt 已登记） |
| middleware 命中三元组 → 200 replay；body hash 不一致 → 409             | `middleware/internal_idempotency.go:53-73`                              | 符合 |
| 同事务写 idempotency 记录 + 业务（v1.2 §1.2.3 行 271-273 关键约束）    | controller 当前用 `persistIdem(c, http.StatusOK, resp)` **在业务 commit 之后**调用 `SaveIdempotencyResponse` —— **不在同 TX**。controller 层 `model.IncreaseUserQuota` 没暴露 TX，硬要做也得改 model 层接口 | **不符** |

**风险**：B-18 的 idempotency 落库与业务非同事务，意味着：业务 commit 成功 → 服务器在 `SaveIdempotency` 之前 OOM / panic → 客户端用同 key 重试，会被当成新请求**再扣一次 quota**。HIGH（详见 §6）。

`internal_idempotency.go:25` `ResponseBody string text` 落明文响应 7 天，HANDOFF §9 已登记 Phase 2A 切 KMS envelope。当前 prod-flag-on 期间 partner 调用 / token / quota 数据是落明文 DB 的，需要运维知情。MEDIUM。

---

## 2. 路由 + 鉴权 + Keystore 契约对齐汇总

### Header 名

| 文件                                       | KeyId 头           | TS 头              | Nonce 头           | Sig 头              |
| --- | --- | --- | --- | --- |
| `Fy-api/middleware/internal_auth.go:37-40`  | `X-Tnb-Key-Id`     | `X-Tnb-Timestamp`  | `X-Tnb-Nonce`     | `X-Tnb-Signature`   |
| `partner-api/internal/infra/fyapi/client.go:109-112` | `X-Auth-KeyId`     | `X-Auth-Timestamp` | `X-Auth-Nonce`    | `X-Signature`       |
| `integration-design.md:119-122`            | `X-Auth-KeyId`     | `X-Auth-Timestamp` | `X-Auth-Nonce`    | `X-Signature`       |

**partner-api 与 integration-design 一致；Fy-api 完全对不上**。这是一条 PR-2 与 partner-api W1d 集成时被忽略的 drift——可能由于 review v1.0 早期某个版本用的是 `X-Tnb-*`，没跟着 v1.2 修订。CRITICAL-A。

### Canonical 串字段顺序

| 来源 | canonical 串 |
| --- | --- |
| Fy-api `internal_auth.go:142-149`                 | `METHOD\nPATH\nTIMESTAMP\nNONCE\nKEY_ID\nsha256(body)`            |
| partner-api `client.go:138`                       | `method + "\n" + path + "\n" + query + "\n" + sha256(body) + "\n" + ts + "\n" + nonce` |
| integration-design `行 166-173`                   | `method\npath\ncanonical_query\nts(int)\nnonce\nsha256(body)`     |

三方互不相同。CRITICAL-B。partner-api 与 integration-design 顺序差异（query 后置 vs 前置）也得修齐。

### 签名编码

| 来源 | 编码 |
| --- | --- |
| Fy-api `internal_auth.go:151,156`           | `hex.EncodeToString(expected)`，`subtle.ConstantTimeCompare(expected, givenBytes)` |
| partner-api `client.go:142`                 | `hex.EncodeToString(mac.Sum(nil))`                                                  |
| integration-design `行 122 / 174-177`       | `base64.StdEncoding.EncodeToString(mac.Sum(nil))` + `hmac.Equal([]byte(want), []byte(sig))` |

partner-api 和 Fy-api 用 hex 是一致的，但和 integration-design 用 base64 不一致。**两边代码已经对齐 hex**——这其实是 partner-api W0 客户端 + Fy-api 实现都偏离了 v1.2 文档（v1.2 文档比 W0 client 写晚？）。建议：保持 hex 不变（更紧凑、URL-safe-ish、对齐了），改 integration-design 文档以 hex 为准。LOW（文档修订）。

### nonce TTL

| 来源 | TTL |
| --- | --- |
| Fy-api `internal_auth.go:43`                | 24h                                                                |
| integration-design `行 127 NonceTTL`       | 5 min                                                              |

**24h vs 5min** 是个看起来"防御性更强"的实现，但带来：(a) Redis key 占用是 5min 版的 288 倍，CN 节点单 key 64 字节 × 高峰 100 QPS × 86400s ≈ 550 MB 常驻；(b) partner-api 5xx 重试时 nonce 已用过（client 每次 `uuid.NewString` 不会复用，所以 (b) 不是问题，但 (a) 是）。建议对齐 5 min / 1 h；至少不能 24h。HIGH。

### `X-Oneapi-Request-Id` trace_id 透传

`integration-design.md` 行 124 / 181-184 要求 middleware 把 `X-Oneapi-Request-Id` 提取并 `c.Set("trace_id", tid)`。
`internal_auth.go` 整文件没有任何 `X-Oneapi-Request-Id` / `trace_id` 处理。MEDIUM。

但是注意 Fy-api 主流程已经在 `middleware/request-id.go` 里有自己的 RequestId middleware 跑在所有路由前面。如果 `RequestId` middleware 跑在 InternalAuth 之前，trace_id 应该已经在 `common.RequestIdKey` 里。需要 confirm middleware 顺序：`router/api-internal-router.go` 没显式挂 `RequestId()`，而 `router/main.go:21 SetInternalRouter(router)` 在 `SetRouter` 之后调用，那 internal 路由组是否继承了全局 middleware？需要在 PR 描述里明确。MEDIUM（debt：写一个 e2e 验证 trace_id 从 partner-api → Fy-api logs 全链路可见）。

---

## 3. Billing Hot Path 安全审计

### Hot path 改动盘点

| 文件 | 行 | 改动 |
| --- | --- | --- |
| `relay/common/relay_info.go`        | +160 / +495-508 | struct 字段 + ctx/DB best-effort 注入 |
| `relay/helper/price.go`             | +57, +65        | user-group / normal-group 都 ApplyOverride |
| `service/quota.go`                  | +112, +119, +128 | PreWssConsumeQuota 三处 |
| `service/task_billing.go`           | +281-284         | task 路径回库 |
| `service/group.go`                  | +75-84           | cold-path helper                       |
| `setting/ratio_setting/effective_group_ratio.go` | (new) | hot-path 唯一入口                            |
| `model/log.go::RecordConsumeLog`    | +244-249         | 单语句 → `recordConsumeLogWithOutbox`        |

### 安全验证

1. **flag off 时行为字节级一致**
   - `effective_group_ratio.go:19-21 if !IsGroupRatioOverrideEnabled() return fallback` ✅
   - `log_outbox.go:36-39 flag off → LOG_DB.Create(log).Error` ✅
   - `internal_auth.go:53 flag off → 503` ✅
   - `relay_info.go:503-507` flag off 时仍然会 callback 到 `model.LookupUserOverride`，**但 fn 实现里也不会做任何 flag 检查**，所以 flag off 时仍然会回库一次（即便 ApplyOverride 后丢弃结果）。**HIGH 性能问题**：flag off 时不应该回库。需要在 `relay_info.go:503` 前加 `if !overlay_flag.IsGroupRatioOverrideEnabled() { return }` 或者在 callback 内 short-circuit。

2. **TX 失败模式** —— `recordConsumeLogWithOutbox`
   - quota 已扣（`PreConsumeQuota` 在 RelayInfo 阶段就扣了），TX 内 `Create(log) + Create(outbox)` 任一失败 → TX 回滚，**log 和 outbox 都没落，但 quota 已扣**。这不是新 bug（upstream 原 `LOG_DB.Create(log)` 失败也是 log 缺失 + quota 已扣），但 v1.2 §1.5.3 关注的是不让"扣了 quota 又没 outbox 又没 log"这种**在 outbox 半路出错的额外失败模式**——current 实现因为是同 TX，要么都有要么都没，没引入新的孤儿态。✅
   - 但是 `model/log.go:251-252` TX 失败时只记 log，**没把错误冒泡给 caller**——`RecordConsumeLog` 是 `void` 函数，调用方拿不到错误。这是 upstream 行为，不算回归，但 outbox 灰度期需要 metric 知道 TX 失败率。需补一个 `metric_tx_failed_total{reason}` counter。HIGH。

3. **payload size**：`log_outbox.go:45-58` payload 序列化 9 个字段，单事件 < 1 KB。CN 高峰 200 QPS × 1KB × 区域隔离 → 17 GB / day / 区，prepublish 期持久驻留 LOG_DB 直到 publisher 拉走。TTL 没在表里设，published 后只 mark 不删——长期需 leader-only cron 清理。HANDOFF §9 已登记。OK。

4. **Hot path 性能**
   - `ApplyOverride` 1 atomic load + 1 float compare：< 0.01ms。✅
   - `relay_info.go:503-507` flag off 时回库（见上一点）：100 QPS × 0.5ms DB → 50ms 占用，但请求级别 0.5ms，影响 < 1%。
   - flag on 时的 `LookupUserOverride` 回库：(user_id, group) 没专用索引（详见 B-15 §1），需补。HIGH。

### Partner-API saga 与 Fy-api 半信任契约

partner-api 在 `client.go` 里用 `Idempotency-Key` 透传。Fy-api `middleware/internal_idempotency.go` 命中即 replay。**但是 controller 层 `persistIdem` 是业务 commit 之后才落 idempotency 记录**——这段窗口内（业务 commit 完成 → SaveIdempotency 完成）如果 Fy-api crash，partner-api 用同 key 重试时 Fy-api 看不到 idem record，**会再次执行业务**。这不是 partner-api 客户端能修的，必须 Fy-api 把 SaveIdempotency 拉进业务 TX。CRITICAL-D（详见 §6）。

---

## 4. partner-api 侧 Fy-api Client 审计

文件：`TraceNexBiz/apps/partner-api/internal/infra/fyapi/client.go`（180 LOC）

### 优点

- HMAC 签名 4 元组实现 `Do:104-119` 路径清晰 ✅
- 强制 path prefix `/api/internal/` `Do:80-82` ✅
- 自 marshal body / 自动设 `Content-Type: application/json` ✅
- 不内置重试，留给 saga 层决定（注释明示）✅
- `httpClient.Timeout = cfg.FyAPI.Timeout` 配置驱动 ✅
- `IdempotencyKey` 与 `TraceID` 透传 ✅

### 问题

1. **header 名与 Fy-api middleware 不匹配**（`X-Auth-*` vs `X-Tnb-*`）—— CRITICAL-A，已在 §2 详述。
2. **canonical 串顺序与 Fy-api 不匹配 + integration-design 也不完全一致**：partner-api `client.go:138` 把 `query` 放在第三段，Fy-api 没读 query，integration-design 把 `query` 放第三段——partner-api 与 integration-design 一致，但与 Fy-api 不一致 → CRITICAL-B。
3. **`Do:121-124 httpClient.Do(httpReq)` 错误处理裸返回**，没区分超时 / 5xx / 4xx 类型。caller 拿到 `error` 之后无法决策"应不应该 retry"——integration-design 行 81-86 / §4.4 saga retry 要求区分 retryable / 非 retryable（5xx / timeout 是 retryable，4xx 不是）。HIGH。
4. **`Do` 没 envelope 解析**：返回 `Response{Status, Body []byte}` 让 caller 自己 unmarshal。但是 5 个 placeholder 方法（`CreateUser` / `SetGroupRatioOverride` / `AdjustQuota` / `CreateToken` / `GetUsage`）全都返回 `nil, errors.New("not implemented; W1b to wire ...")`——意味着 partner-api 业务层目前**完全没有可用的 Fy-api 客户端**。这是一个 W1b 未完成的 placeholder PR。CRITICAL-E。
5. **HMAC secret 注入**：`NewClient:45-57` 从 `cfg.FyAPI.HMACSecret` 直接读 string —— 配合 ADR-010 的"从 KMS 注入"还没接，目前是 env-file 明文。Phase 1 接受，但 secret rotation Pub/Sub 没有设计 → HMAC keystore Fy-api 有 `RotatedAt` 字段但 partner-api 没动态 reload。MEDIUM。
6. **`uuid.NewString` 用 `github.com/google/uuid`**：integration-design §1.1.3 行 158 nonce TTL 5min，nonce 长度 36 字符，与 Fy-api 24h × 8-128 字符 length-only check 不冲突，OK。
7. **没有 retry-after 处理**：integration-design §1.1.3 / §4.4 提到 5xx 之后客户端要尊重 `Retry-After` header；client.go 完全没读。MEDIUM。

---

## 5. Upstream Rebase 风险评估

新增 `// Fy-api overlay: B-1X ...` 注释在 hot path 4 个文件总计 **9 处 patch**。下次 `git fetch upstream && git merge upstream/main` 时这 4 个文件冲突概率排序：

### Hotspot 1（HIGH 冲突概率） — `model/log.go::RecordConsumeLog`

- patch 在第 244-249 行（内嵌在函数主体内），把 `LOG_DB.Create(log).Error` 改为 `recordConsumeLogWithOutbox(c, log, userId, params)`。
- upstream 在 `RecordConsumeLog` 函数活跃区，过去 6 个月该函数有 20+ 提交（subscription 接入、IP 记录、settings hook、user_setting reload 等）。
- 风险：每周 sync 会有 20-30% 概率冲突，且冲突点在函数主体内不在文件边缘 —— 自动合并 N。
- 缓解：B-16 已经在 patch 周围加了 invariant 注释（行 244-248）登记 5 个非-consume `LOG_DB.Create` 调用站，merge 时 grep `B-16` 即可定位。但仍需 `Fy-api/CLAUDE.md` 周度 sync 章节明确写"merge 后必须 grep `Fy-api overlay: B-16` 确认 wrap 仍然存在"。

### Hotspot 2（MEDIUM 冲突概率） — `relay/helper/price.go::HandleGroupRatio`

- patch 在 53-65 行，紧贴 user-group / normal-group if-else 主分支。
- upstream 在 v1.0 后给 `HandleGroupRatio` 加过 `auto_group` / `GroupSpecialRatio` 等支线（这些已经合在 base 里了，看到行 41-50）。
- 风险：上游若再扩张该函数（如加 subscription tier ratio），会和 ApplyOverride 调用碰撞。
- 缓解：注释打满了 ✅。

### Hotspot 3（MEDIUM 冲突概率） — `relay/common/relay_info.go::genBaseRelayInfo`

- struct 末尾 +`UserGroupRatioOverride`（行 160） + 函数 `genBaseRelayInfo` 末尾 +13 行（行 495-508）。
- upstream 在 `RelayInfo` struct 持续加字段（subscription / billing / tiered 等），struct 字段碰撞概率高。
- 缓解：字段加在末尾 + 注释。但 `genBaseRelayInfo` 的 13 行嵌入在函数主体内，下次 upstream 加 token 字段会有问题。
- 建议：把 `relay_info.go:495-508` 这段抽到一个独立 helper `applyOverlayB15(c, info)`，调用一行；upstream merge 时只需保留 helper 一行 call。MEDIUM 重构建议。

### Hotspot 4（MEDIUM 冲突概率） — `service/quota.go::PreWssConsumeQuota`

- 三处 patch（行 112 / 119 / 128）分散在函数体内 if/else。
- upstream realtime/wss 路径相对稳定（最近修订少）。
- 风险：低-中。

### Hotspot 5（LOW） — `service/task_billing.go` / `service/group.go`

- task_billing.go 单点 patch（行 281-285），cold path。
- group.go 是 +新函数。
- 风险：极低。

### 综合 Rebase 难度

- **per-week**：估计每周 sync 有 30-40% 概率在 `model/log.go` 或 `relay/common/relay_info.go` 命中冲突，每次冲突 < 30min 解决（注释和 invariant 都齐全）。
- **per-quarter**：3 个月一次 upstream 大 reflactor 风险（如 subscription 第三次重写），可能 1-2 处 patch 完全失效，需要重新 wire。
- **缓解措施**（必须做）：
  1. `Fy-api/CLAUDE.md` 增加"周度 sync 后强制 grep `Fy-api overlay: B-1[2-8]`"清单 step。
  2. `make ci-check-overlay` target，CI 跑 `grep -r 'Fy-api overlay: B-15' relay/common/relay_info.go relay/helper/price.go service/quota.go service/task_billing.go` 行数硬验，少一处即 fail。HIGH 建议。

---

## 6. CRITICAL / HIGH / MEDIUM / LOW

### CRITICAL（合并前必须修）

- **CRITICAL-A：HMAC header 名 Fy-api 与 partner-api / integration-design 不一致**
  - Fy-api `middleware/internal_auth.go:37-40` 用 `X-Tnb-*`
  - partner-api `client.go:109-112` 与 integration-design 都用 `X-Auth-*`
  - 后果：partner-api 调 Fy-api 100% 401。
  - 修法：统一改 Fy-api middleware 头名为 `X-Auth-KeyId / X-Auth-Timestamp / X-Auth-Nonce / X-Signature`；如要保留 `X-Tnb-Idempotent-Replay` 这种纯 Fy-api 私有响应头可以独立。

- **CRITICAL-B：HMAC canonical 串字段顺序 / 是否包含 query 三方互不相同**
  - Fy-api `internal_auth.go:142-149`：`METHOD\nPATH\nTS\nNONCE\nKEY_ID\nbody_hash`（没有 query，多了 KEY_ID）
  - partner-api `client.go:138`：`method\npath\nquery\nbody_hash\nts\nnonce`
  - integration-design 行 166-173：`method\npath\ncanonical_query\nts\nnonce\nbody_hash`
  - 后果：即便 header 名修齐，签名仍然 100% 不匹配。
  - 修法：以 integration-design 为准三方对齐。partner-api 客户端需要把 body_hash 移到末尾，Fy-api middleware 需要补 canonical_query 段并去掉 KEY_ID 段。`canonical_query` 用排序+url-encode 标准化（参考 AWS SigV4 风格）。

- **CRITICAL-C：HMAC nonce TTL 24h vs 5min**
  - Fy-api `internal_auth.go:43` `24*time.Hour`，integration-design 行 127 `5*time.Minute`
  - 后果：Redis key 存量 ≈ 288 倍，CN/SG 各占 ~500 MB+；不是 CRITICAL 安全风险但运维风险显著。
  - 修法：5 min（与 clock skew 5min 对齐）。

- **CRITICAL-D：Idempotency 落库与业务非同事务**
  - `controller/tnbiz_internal/user.go::Topup,AdjustQuota,Refund` / `token.go::CreateToken`：业务（`IncreaseUserQuota` / `tok.Insert()`）已 commit 之后才调用 `persistIdem` → `SaveIdempotency`
  - integration-design 行 271-273：「internal_idempotency 表落库与业务 TX **同事务**（避免幂等记录有但业务回滚的边缘）」
  - 后果：业务 commit 成功后、SaveIdempotency 之前 crash → partner-api 重试会**重复扣 quota**。这是一个真实的 saga 双扣窗口，不是理论问题。
  - 修法：要么在 model.IncreaseUserQuota 暴露 TX 版本 + middleware 在同 TX 内插 idempotency 记录；要么按 §1.2.3 行 263-264 改为先用 `internalIdem.Lookup` 提前落 placeholder（pending），业务 commit 后 Update 为 published（这种设计在 §1.7 idem 表设计里有讨论）。
  - 这是 PR 合并最大阻断点。

- **CRITICAL-E：partner-api Fy-api client 5 个业务方法是 placeholder 全部 return error**
  - `client.go:158-180` `CreateUser / SetGroupRatioOverride / AdjustQuota / CreateToken / GetUsage` 全都 `errors.New("not implemented; W1b to wire ...")`
  - 后果：partner-api 业务层调任何 Fy-api endpoint 都会失败 —— W1d 落地了 `Do(...)` 但没接进 saga / handler。
  - 修法：把 W1b 待办拆出来阻塞合并，或至少把 5 个方法的 W1b → `c.Do(ctx, Request{Method: ..., Path: ..., Body: ..., IdempotencyKey: ..., TraceID: ...})` wiring 实现完整。

### HIGH（强烈建议合并前修）

- **HIGH-1：B-15 hot path flag off 时仍然回库**：`relay_info.go:503-507` 无条件触发 `overrideLookup`。修法：`if !overlay_flag.IsGroupRatioOverrideEnabled() { return }` 短路。
- **HIGH-2：`group_ratio_override` 表缺 `(user_id, group, status)` 二级索引**：`LookupUserOverride` 查询前导列不命中 `idx_gro` UNIQUE 索引（前导列是 partner_kid）。建议加：`gorm:"index:idx_user_group_status,priority:1"` 等。
- **HIGH-3：outbox payload 缺 `partner_kid`**：`log_outbox.go:45-58` 没序列化 partner 归属，publisher 下游无法做 fan-out。
- **HIGH-4：`recordConsumeLogWithOutbox` TX 失败不冒泡 + 无 metric**：log + outbox TX 失败时只 log.LogError，没 counter，灰度期看不到失败率。补 `model.outbox_tx_failed_total{reason=marshal\|create\|tx}` counter。
- **HIGH-5：outbox runner 完全没 metric**：`runner.go:98-129` 没 counter / latency。建议补 `outbox_published_total / outbox_failed_total / outbox_lease_lag_seconds`。
- **HIGH-6：outbox `noopShadow` 包级单例 + Fy-api `r.publisher = NoopPublisher{}` 时 enabled 模式行为退化**：`runner.go:114-119` 只在 mode=shadow 时切换到 noopShadow，但当 mode=enabled 而 r.publisher 又是 NoopPublisher（main.go 注入了 nil → NewRunner 内部 default to NoopPublisher）时，enabled 模式跑 noop 而无 warning。修法：mode=enabled + publisher=Noop 时 `runner.go:Start` 启动期 SysLog warning，或 require 显式注入。
- **HIGH-7：HMAC nonce TTL 与 partner-api retry 策略不匹配 → 24h 期间客户端任何 nonce 重用都会 401**：partner-api saga retry 用 fresh nonce 是 OK，但是文档化合规要求跟 integration-design 一致。
- **HIGH-8：partner-api client 没有 retry / error classification**：`Do` 直接 return error，5xx vs 4xx 一视同仁。在 saga handler 里把 5xx 包成 retryable.Error，4xx 包成 terminal.Error。
- **HIGH-9：`make ci-check-overlay` CI 守卫缺失**：上游 sync 时静默吞掉 B-15 patch 的风险无防御。

### MEDIUM

- **MED-1：trace_id (`X-Oneapi-Request-Id`) middleware 未显式提取 / 注入 ctx**：依赖现有 RequestId middleware，没有 e2e 验证。
- **MED-2：flag poll interval 无 OptionMap 配置**：`flag.go:54` pollInterval 包级 var，要改得改代码。
- **MED-3：`LookupUserOverride` 无 LRU 缓存**：每次 hot path 回库（即便 ApplyOverride 不用 ratio 也已经查过了）。
- **MED-4：`MarkOutboxFailed` 用 `Save(&rec)` 整行写**：与 publisher 并发场景可能覆盖字段；改 `Updates(map[string]any{...})`。
- **MED-5：`internal_idempotency.response_body` 落明文 7 天**：含 quota / token_id 等业务数据；HANDOFF §9 已登记 Phase 2A 切 KMS envelope，先 SysLog warning 提醒运维数据敏感性。
- **MED-6：partner-api client secret rotation 无热加载**：`hmacSecret` 是构造时固化；keystore.RotatedAt 不会触发 client reload。
- **MED-7：partner-api client 不读 `Retry-After`**。
- **MED-8：Fy-api 错误码命名 `invalid_request / topup_failed` 没用 BIZ_*** 前缀，与 v1.2 §6.5 不一致。

### LOW

- **LOW-1**：签名编码 hex vs base64 → 三方代码已经对齐 hex；改 integration-design 文档以 hex 为准。
- **LOW-2**：`overlay_flag.OutboxOff / Shadow / Enabled` 3 个常量散落在 outbox 包外，建议提到 `setting/overlay_flag/` 公开枚举型 + iota，避免 string typo（已经定了 const 但仍用 string compare）。
- **LOW-3**：`runner.go:131 noopShadow` 包级单例 `Sent / LastBody` 单调累积 → 长跑内存泄漏，shadow 模式专用——建议私有化 + 不存 LastBody，仅计数。
- **LOW-4**：`controller/tnbiz_internal/health.go::healthResponse.OverlayInternal bool` 字段命名风格不一致（`OverlayInternal` / `OverlayHMAC` / `OverlayOutbox`）；用 `overlay_internal_api / overlay_hmac_keystore / overlay_outbox` json tag 已经对齐了，code 里 struct 名风格调下即可。

---

## 7. 修订指令（Round 2 必做）

按优先级排序。我估计 W1d agent 一个工作日能搞定 CRITICAL，2-3 天搞 CRITICAL+HIGH。

### 阻断合并（CRITICAL，必须 Round 2 内全部完成）

1. **统一 HMAC 头名为 `X-Auth-KeyId / X-Auth-Timestamp / X-Auth-Nonce / X-Signature`**
   - 改 `Fy-api/middleware/internal_auth.go:37-40`
   - 改 `Fy-api/middleware/internal_idempotency.go:69` `X-Tnb-Idempotent-Replay` 可保留（响应头不冲突）
   - 同步更新 OVERLAY.md / OVERLAY-TNBIZ-HANDOFF.md 表格 §3、单测里 header 名

2. **统一 canonical 串顺序为 `method\npath\ncanonical_query\nts\nnonce\nsha256_hex(body)`**
   - 改 `Fy-api/middleware/internal_auth.go:142-149`
   - 改 `partner-api/client.go:136-142` 把 ts 移到第 5 段、nonce 第 6 段、body_hash 第 4 段
   - 添加 `canonicalQuery(rawQuery string) string` 工具函数（key 排序 + url.QueryEscape）双方共享一份测试 vector
   - 加一组共享测试向量（hardcoded canonical / signature pair）让两边都能复现验签

3. **nonce TTL 改 5min**
   - 改 `Fy-api/middleware/internal_auth.go:43 hmacNonceTTL = 5 * time.Minute`

4. **idempotency 落库改为同业务 TX**
   - 在 `model.IncreaseUserQuota / DecreaseUserQuota / Token.Insert` 暴露 `*Tx` 版本
   - controller 改用 `DB.Transaction(func(tx *gorm.DB) error { ... 业务 ...; return SaveIdempotencyInTx(tx, rec) })`
   - 至少从 `Topup / AdjustQuota / Refund / CreateToken` 4 个写接口落实

5. **partner-api 完成 5 个 endpoint 实际 wiring**
   - `CreateUser / SetGroupRatioOverride / AdjustQuota / CreateToken / GetUsage`
   - 每个方法走 `c.Do(ctx, Request{Method, Path, Body, IdempotencyKey, TraceID})` + envelope unmarshal

### 强烈建议（HIGH，Round 2 内若来不及，必须在 PR-2 合并后立刻 follow-up）

6. `relay_info.go:503` 加 `if !overlay_flag.IsGroupRatioOverrideEnabled() { return }`
7. `model/group_ratio_override.go` 加 `(user_id, group, status)` 二级索引
8. outbox payload 加 `partner_kid` 字段
9. `recordConsumeLogWithOutbox` TX 失败 metric counter
10. outbox runner publish/fail/lease 三组 metric
11. outbox runner enabled+Noop 启动期 SysLog warning
12. partner-api client 加 retryable / terminal error 分类
13. `make ci-check-overlay` CI 守卫，grep `Fy-api overlay: B-1[2-8]` 行数硬验

### MEDIUM 列表 PR-3+ 跟进

14-21. 见 §6 MEDIUM。

---

## 8. 给其他 reviewer 的 cross-check 备忘

- **Security 同事**：CRITICAL-A/B/C/D 这 4 条建议你也独立验一遍，特别是 D（idem-key 双扣窗口）的攻防意义；shadow → enabled 的灰度切换 runbook 应该把"shadow 期间至少跑 N 天"写死。
- **code-reviewer 同事**：B-15 hot path 4 个文件 9 处 patch 的注释规范我做了，请审一下是否符合 OVERLAY 文件的命名 / 风格。
- **Architect 同事**：partner-api client `Do` 抽象 + 5 个 placeholder 方法的设计是 W0 / W1 协作期合理，但 W1d 落 `Do` 后 W1b 没接 → 这是 task 拆分 / acceptance 漏的问题，请在 round 2 acceptance gate 里加"5 个 endpoint 全部能跑通的 e2e 测试"硬条件。
- **Compliance 同事**：`internal_idempotency.response_body` 7 天 TEXT 明文 + `internal_api_key.SecretCipher` AES-GCM 是当前 Phase 1 的最佳实践。Phase 2A KMS envelope 切换的 milestone 我建议钉死在 `partner-api` 第一个 prod release 之前。

— Fy-api 团队 / TraceNex tech lead，code-round-1，2026-05-10
