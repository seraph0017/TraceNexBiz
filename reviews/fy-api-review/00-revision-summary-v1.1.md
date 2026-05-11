# v1.0 → v1.1 修订摘要：合入 Fy-api review

> 作者：Architect (主架构师)
> 日期：2026-05-11
> 输入：`reviews/fy-api-review/01-fy-api-side-review.md`（Fy-api Tech Lead, ACCEPT-WITH-CHANGES, 2026-05-12）
> 输出：`docs/integration-design.md` v1.1（主修）+ `docs/00-architecture-overview.md` v1.1 + `docs/backend-design.md` v1.1（轻交叉对齐）；`docs/frontend-design.md` 不涉及

---

## 1. 4 个 CRITICAL 硬错状态

| # | 来源 | v1.1 状态 | 主落点（FIXED 引用） |
|---|---|---|---|
| C-1 | OVERLAY 编号从 B-8 起 → 实际已用到 B-11，应从 **B-12** 起 | ✅ **FIXED** | `integration-design.md` §1.8 全节重写为 B-12..B-18，每条 OVERLAY 条目命名 + 文件清单 + 冲突风险 + Merge 策略 + Feature flag；overview §C-1 / §18.1 同步；backend §24.1 索引 |
| C-2 | `go-redis/v9` → 应统一 `go-redis/v8` v8.11.5（Fy-api `go.mod` 实测） | ✅ **FIXED** | `integration-design.md` §1.1.3 import 改 `github.com/go-redis/redis/v8`，SetNX 改为 `c.Request.Context()` ctx-first；backend §24.1 注释 |
| C-3 | MySQL `logs.id` `ALGORITHM=COPY` → 必须 gh-ost / pt-osc 在线 DDL | ✅ **FIXED** | `integration-design.md` §1.3.3 重写：gh-ost 命令模板 + 预估时长（CN 100M-300M 4-8h；SG 80M 3-6h）+ cut-over 风险 + 回滚（gh-ost cancel + DROP `_logs_gho`）+ 新增字段默认值 / NULL 处理；BIGINT 单独列为 §15.2 PR-1 不可回滚独立走 |
| C-4 | mTLS 假设 K8s + Istio/Linkerd → 实际 Podman + Nginx | ✅ **FIXED** | `integration-design.md` §6.4 全节重写：Nginx 客户端证书校验（`ssl_client_certificate` + `ssl_verify_client on`）+ 注入 `X-Client-Verified` / `X-Client-CN` + gin loopback 校验 + Nginx vhost 样板 + Phase 矩阵；删除所有 Istio / `X-Forwarded-Client-Cert` / PeerAuthentication / NetworkPolicy 章节；overview §2 流量表 v1.1 同步；附录 A 新增 T-23 标注 mesh 设计 deferred 到 Phase 2B |

---

## 2. HIGH 状态

| ID | Fy-api review § | v1.1 状态 | 主落点 |
|---|---|---|---|
| H-5 | GroupRatioOverride 6 调用站 / 4 文件，非"一行替换" | ✅ FIXED | integration §1.4.2 重写为 RelayInfo 字段方案 + 6 调用站 / 4 文件实施清单（`relay/common/relay_info.go` / `middleware/distributor.go` / `service/quota.go:110-121` / `relay/helper/price.go:53-61` / `service/task_billing.go:276-277` / `service/group.go:60-64`）+ 性能预算重估 |
| H-6 | 每 PR 独立 feature flag | ✅ FIXED | integration §14 新增 Feature flag 框架（5 个 OVERLAY_*）+ 默认值矩阵（首发 prod 全 false，灰度逐个开）+ 回滚条件 + 责任人；overview §18 引用 |
| H-7 | 工作量重估（575 LOC / 1-1.5 周 → 29 人天）+ 5-PR 拆分 | ✅ FIXED | integration §1.9 LOC 表重写 + 29 人天合计；§15 新增 5-PR 拆分（PR-1 BIGINT 不可回滚 / PR-2 路由+controllers 影子 / PR-3 Pub/Sub / PR-4 GroupRatioOverride / PR-5 outbox），每 PR 人天 / 测试 / 回滚 / 依赖前置 |
| H-7-pubsub | 订阅 goroutine rate-limit 改 coalescing | ✅ FIXED | integration §1.6.3 重写为 dirty flag + 独立 ticker 模式 |
| H-8 | RecordConsumeLog 末尾 LogQuotaData 不进事务；5 个非 consume LOG_DB.Create 调用站 lint | ✅ FIXED | integration §1.5.3 v1.1 注释 + lint checklist + `// Fy-api overlay: B-16 outbox scope = consume only; do NOT add outbox here` 注释规约 |
| H-9 | C-7 idempotency 与 §5.3 idempotency_record 字段语义对齐 | ✅ FIXED | integration §1.7.2 v1.1 注释明示两表独立；Phase 1 内部 idem 用 `response_body TEXT`，Phase 2A 评估 KMS（+7 人天） |
| H-10 | `/api/internal/*` 不挂在 apiRouter Group（避开 GlobalAPIRateLimit）| ✅ FIXED | integration §1.1.1 v1.1 注释 + §1.8 B-12 Merge 策略 |
| H-Reverse-Asks | Fy-api §13 反向请求 8 项 | ✅ FIXED | integration §16 新增 Fy-api 跨团队 hand-off 清单 + 责任方 / 截止 / 阻塞性 / 当前状态；overview 附录 C 索引 |

---

## 3. MEDIUM / LOW 状态

| ID | Fy-api review § | v1.1 状态 | 主落点 |
|---|---|---|---|
| M-9 / M-13 | SyncFrequency 60s → 缩短建议 5-15s + polling 不删 | ✅ FIXED | integration §1.6.3 v1.1 备注 |
| M-10 | Pub/Sub 选型 Aliyun MNS vs RocketMQ | ✅ FIXED | integration §1.6.3 v1.1 备注：保留 Redis Pub/Sub + DB outbox，否决 MNS / RocketMQ（Fy-api 已依赖 Redis，零增量；MNS / RocketMQ 当前部署无 broker） |
| M-11 | partner-api 同实例不同 DB GORM 多 DB + 连接池 | ✅ FIXED | integration §6.2 v1.1 注释（bizDB maxOpen=50 / fyReadDB maxOpen=20 / logDB maxOpen=20；ConfigMap 同步加 LOG_SQL_DSN） |
| M-12 | 监控 dashboard 集成 | ✅ FIXED | integration §9.3 v1.1 备注：Prometheus + Grafana 共享，dashboard 按 tag 分两块（Backend / Fy-api Overlay），既有 Fy-api dashboard 保留独立 |
| M-14 | Fy-api 侧 4 个新 metrics | ✅ FIXED | integration §9.3 metrics 列表 v1.1 新增 5 个：`internal_idempotency_hits_total{kid,endpoint}` / `internal_idempotency_conflicts_total` / `consume_log_outbox_writes_total{result}` / `consume_log_outbox_tx_duration_seconds` / `internal_scope_mismatch_total{kid}` |
| L-15 | `Fy-api/openapi/internal-api.yaml` 必须随 OVERLAY PR 创建 | ✅ FIXED | integration §1.8 B-12 文件清单已列；§11 变更管理已锚定 |
| L-16 | F-1 per-model markup 延后 Phase 2A | ✅ NO-CHANGE | integration §1.4.4 已是 Phase 2A schema-only，T-3 架构债务记 |

---

## 4. 跨文档联动哪些章节

### `integration-design.md` v1.1（主修，~1900 行 → ~2200 行）

- 顶部版本头 v1.0 → v1.1
- §1.1.1 路由挂载注意（独立 Group）
- §1.1.3 Redis v8 import + ctx-first 调用 + Nginx mTLS 备注 + KeyStore 实现归属
- §1.3.3 BIGINT 三方言 SQL 重写（gh-ost）
- §1.4.2 GroupRatioOverride 6 调用站 / 4 文件 + 性能预算
- §1.4.3 InvalidateUserCache LOC 重估 30-50
- §1.5.3 LogQuotaData fire-and-forget 注释 + lint checklist
- §1.6.3 coalescing 模式 + SyncFrequency + Pub/Sub 选型理由
- §1.7.2 字段语义对齐
- §1.8 OVERLAY B-12..B-18 全节重写（8 条 entry 完整模板）
- §1.9 LOC + 29 人天工作量重估
- §6.2 GORM 多 DB + 连接池
- §6.4 mTLS Nginx + Podman 全节重写 + Phase 矩阵
- §9.3 Fy-api 侧 metrics + dashboard 集成
- §14 Feature flag 框架（新增节）
- §15 5-PR 拆分（新增节）
- §16 Fy-api 跨团队 hand-off 清单（新增节）
- §17 v1.0 → v1.1 CHANGELOG（新增节）
- §11 变更管理"B-8..B-14" → "B-12..B-18"
- §C1.4 测试钩子 mTLS 描述更新
- §1.1.2 路由树 mTLS 注释更新
- §0 阅读说明同步

### `00-architecture-overview.md` v1.1（轻修）

- 顶部版本头 v1.0 → v1.1 + 新增 Fy-api review 引用
- §2 流量表 partner-api → Fy-api 行：mTLS Istio → Nginx
- §C-1 OVERLAY 编号 B-8..B-14 → B-12..B-18
- 附录 A 新增 T-23（部署形态 deferred Phase 2B）+ T-24（hot-path 上游下沉）
- §18 v1.0 → v1.1 CHANGELOG（新增节）
- 附录 C Fy-api 跨团队 hand-off（新增节，索引 integration §16）

### `backend-design.md` v1.1（轻交叉对齐）

- 顶部版本头 v1.0 → v1.1 + 新增 Fy-api review 引用
- §24 v1.0 → v1.1 CHANGELOG（新增节）：声明 partner-api service 层不变；OVERLAY 编号 / Redis client / 部署形态以 integration v1.1 为准

### `frontend-design.md`

不涉及。保持 v1.0。

---

## 5. 我最担心 Fy-api 团队复核仍会打回的 ≤ 3 个点

### 担心点 1：Feature flag 注入路径与热加载语义

`integration-design.md` §14.1 写"env 或 `biz_setting.tnbiz_internal_api_enabled` 注入；可热加载（订阅 `option_update` 后即生效）"。但这意味着 `OVERLAY_INTERNAL_API` flag 切换需要先 PUBLISH `option_update`，而 PR-3（Pub/Sub）尚未上时 flag 切换需要重启 / 全量 polling 兜底（最长 60s 不一致）。**Fy-api 团队可能要求**：明确"PR-2 上线 ≤ PR-3 上线"或允许 flag 在 env 启动期独立解析，避免 PR 顺序耦合到 Pub/Sub。**我的预案**：可在 §14.1 补"flag 热加载需要 PR-3 + `OVERLAY_PUBSUB=true`；否则只能重启生效（polling 兜底窗口 60s）"。

### 担心点 2：gh-ost 在阿里云 RDS 8.0 真的能跑通吗

`integration-design.md` §1.3.3 给的 gh-ost 命令模板假设 RDS 8.0 支持 binlog row-mode + 用户能创建 trigger。**实际**：阿里云 RDS 8.0 默认开启 binlog row 模式没问题；**但 trigger DDL 需要 DBA 角色**（`tnbiz_app` 没这权限），ops 必须用 DBA 账号跑 gh-ost。Fy-api 团队 review §13 反向请求-4 已经把"gh-ost 工具链就绪"列为 BLOCKER，我们也只在 §16 表里登记了责任方为 ops + DBA，**没给具体的 staging 试跑剧本**。**我的预案**：staging 必跑 dry-run；若阿里云 RDS 不允许 trigger（部分高版本可能限）则 fallback 到 pt-osc 或 RDS 原生 online DDL（`ALGORITHM=INSTANT` 仅支持 add column，不支持 widen PK；可能没救）。这是真实施时可能撞墙的点。

### 担心点 3：B-15 hot-path patch 在 4 个文件的 conflict cost 长期是否可承受

v1.1 §1.4.2 + §15 把 GroupRatioOverride 的 hot-path 改造分布在 4 个上游高活跃文件（`service/quota.go` / `relay/helper/price.go` / `service/task_billing.go` / `service/group.go`）。Fy-api review §8.1 估算"每周 sync 期望 1-2 个文件冲突，ops 0.5-1 小时"，但**这是主观估计**；如果未来 6 个月内上游对计费系统做大重构，hot-path 4 文件可能同时冲突，单次 sync 可能 4-8 小时。**我的预案**：T-24（overview 附录 A 新增）已登记把 `RecordConsumeLog TX wrap` 上游下沉作为长期解，但 GroupRatioOverride 的 4 文件分支没法上游下沉（per-user override 不通用）。Fy-api 团队复核可能再次要求"减少 hot-path 文件分布"，但 RelayInfo 字段方案已经是侵入最小的解，**我没有更好备选**。

---

## 6. 待 Fy-api 团队复核确认事项

1. §14.2 默认值矩阵（首发 prod 全 false 影子模式）是否接受
2. §15 5-PR 拆分顺序（PR-1 BIGINT 独立、PR-2 PR-3 可并行、PR-4 PR-5 顺序）是否符合他们的实施节奏
3. §16 8 项 hand-off 清单的责任方分配（ops / partner-api / 业务方）是否准确
4. T-23 / T-24 架构债务列项是否需要补充

> 等 Fy-api 团队 v1.1 复核后，闭环 BLOCKER 即可启动 Phase 1 PR-1。
