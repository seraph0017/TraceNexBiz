# Dev Round-1 Review — Security Engineer (Application Security)

> 日期：2026-05-10
> Reviewer：Security agent（AppSec / OWASP ASVS L2 / 中国等保 2.0 二级口径）
> 范围：四份开发设计文档（00-architecture-overview v0.1、integration-design v0.1、backend-design v0.1、frontend-design v0.1）相对 PRD v1.0 §15–§19 + §22.2 的工程落地审计
> 上轮 Round-2 PRD verdict 参照：`reviews/round-2/03-Security-review.md`（PRD 接受为 v1.0；留 2 条 HIGH 作为 Phase 1 entry 条件）
> 本轮 verdict：**BLOCK（NEEDS_REVISION 才能进入 Phase 1 编码）**
>
> Dev Round-1 tally（下文 §1 详）：**CRITICAL = 7，HIGH = 11，MEDIUM = 14，LOW = 9**。
>
> 架构师团队已经按 Round-2 Security verdict 把 §8.13 哈希链（sealed-by-async-batcher，overview ADR-006 / backend §3.13 + §10）、§19 DEK per-(scope, key_version) + pprof 关闭（overview ADR-009 / backend §9 / cmd kek-rotator + dek rotator）、§17 HMAC-unix-epoch（integration §1.1.3）、§16.3 404-not-403（overview §11.3 / backend §2.4）、§3.4 permission enum CI gate（overview I-3.2 / backend §7.4）、`idempotency_record.response_cipher` 加密（backend §3.16）这些 Round-2 HIGH/MED 都在设计文档里落到代码骨架级别。**这一部分我确认做到了**。
>
> 但在从 PRD → 设计的过程中，**新出现 7 个 CRITICAL 级别的设计漏洞**：两份文档对同一机制的相互冲突描述（HTTP-only cookie JWT vs Authorization Bearer JWT）、BOLA row-level guard 在服务层伪代码中普遍缺失、JWT revocation fail-open on Redis partition、OSS presigned PUT 服务端强校验缺失、saga dual-control 的 second-approver token 可被时间窗叠加绕过、供应链（Go module / npm / 镜像签名 / SBOM）全面缺失、全局速率限制 / 反爆破 / 反凭证填充中间件未设计。以下全文围绕这些问题展开。

---

## 1. 执行摘要

| 级别 | 数量 | 定义 | 本轮处理 |
|---|:---:|---|---|
| CRITICAL | **7** | 必须在 Phase 1 编码开始前修正；等保二级"身份鉴别 / 访问控制 / 数据机密性"硬条 | BLOCK |
| HIGH | **11** | 应在 Phase 1 内完成，不应进入 Phase 2A；OWASP A01/A02/A05/A07 范畴 | WARN |
| MEDIUM | **14** | 建议 Phase 2A 前完成 | INFO |
| LOW | **9** | 硬化清单，Phase 2B 前完成 | NOTE |

**总评分**：**C-（不可合并）**。Round-2 在 PRD 层把 CRITICAL 清零是实打实的进展；但开发设计文档把 PRD 里"policy-level"的约束落成"engineering-level"规约时，把**3 份文档之间的接缝留得太松**——典型表现就是 JWT/cookie 之争：overview ADR-007 + backend §7.2（Bearer header）vs frontend ADR-F5（httpOnly cookie）相互矛盾，PRD §17.1 又说得模糊（"复用 Fy-api JWT + `tnbiz_session` cookie"）。Phase 1 编码若不先统一，就会出现两版不兼容中间件。

参照 OWASP ASVS L2 控制项分布：A01 (Broken Access Control) 发现 4 处（BOLA / IDOR 未在 repository 默认挡住）；A02 (Cryptographic Failures) 2 处；A05 (Security Misconfiguration) 3 处；A07 (Identification & Authentication) 4 处；A08 (Data Integrity / Supply Chain) 2 处；A09 (Security Logging) 1 处；A10 (SSRF) 1 处（持牌方 webhook 未限制回调 URL 协议族）。中国等保 2.0 二级不符项：身份鉴别 3 条、访问控制 2 条、入侵防范 2 条、数据完整性 1 条。

---

## 2. STRIDE 矩阵审计（组件 × 6 项）

PRD §16.2 给了 7 组件 × 6 字段的"policy"矩阵，**粗粒度但方向正确**。四份开发设计把每个字段落到工程级的程度参差不齐。以下是实际落地情况的重评：

| 组件 | Spoofing | Tampering | Repudiation | InfoDisclosure | DoS | Elev. |
|---|---|---|---|---|---|---|
| **浏览器/partner-web** | ✅ 钓鱼 → MFA（frontend §6.5）+ WebAuthn 可选 ⚠️（未强制 M-r2-3） | ✅ CSP `script-src 'self' 'nonce-...'`（frontend §12.1）；XSS 防御合格 | ➖ 前端不负责存证 | ⚠️ Sentry replay 在 partner/customer 站关闭，但 SSO / actor switcher 无 PII pattern 扫描 | ❌ 无 rate-limit on `/auth/login`、`/auth/refresh`、`/public/partner/apply`；凭证填充未防御 | ✅ actor 切换走 `/api/me/switch-actor`（frontend F-1.3） |
| **partner-api** | ⚠️ JWT 验证路径二义（Bearer vs Cookie，**CRITICAL-1**）；jti revocation fail-open on Redis（backend §15.5，**CRITICAL-3**） | ✅ DTO `validate:"gt=0,lte=1e9"`（backend §16.4） | ✅ audit_log sealed-by-async-batcher（backend §10） | ⚠️ log scrubber 只在 zerolog hook；审计写入通道的 PII redaction 不强制（backend §10.1 伪代码未调） | ❌ 无全局 rate-limit（HIGH-2）；no bulkhead on KMS/OSS/Fy-api 依赖 | ⚠️ dual-control saga force-resolve 的 second-approver token 可被同进程内时间窗叠加（CRITICAL-5） |
| **Fy-api `/api/internal/*`** | ✅ HMAC-SHA256 + unix epoch + nonce dedup（integration §1.1.3） | ✅ HMAC 覆盖 method/path/query/ts/nonce/sha256(body) | ✅ internal_auth 打 `auth_kid` + trace_id | ⚠️ mTLS **"require c.Request.TLS != nil 作为兜底"**（integration §1.1.3）在 K8s Istio sidecar 下等同于关闭（HIGH-8） | ❌ key 无 per-kid rate quota；nonce Redis 作为 DoS 放大面（MED-5） | ✅ endpoint allowlist + erase key 独立 scope（integration §2.2.12） |
| **RDS partner_db** | ✅ GRANT 金表（integration §6.2） | ✅ audit_log no UPDATE/DELETE for app user（backend §3.13 + §15.2 I-A-1） | ✅ append-only + hash chain | ⚠️ KMS 信封加密落地但 **blind index（搜索加密字段）设计缺失**——bank_account / id_no 查重时怎么办？（HIGH-11） | ⚠️ connection pool 上限有但**per-actor 级别的 lock 资源没有隔离** | ✅ 三角色 + 最小权限 |
| **Fy-api `fy_api_db` + LOG_DB** | ✅ partner-api 只读（integration §6） | ✅ RecordConsumeLog TX wrap（integration §1.5.3） | ⚠️ Fy-api 原生 `logs` 无 hash chain | ⚠️ `consume_log_outbox.last_error` TEXT 可能回写 PII（MED-6） | ✅ SKIP LOCKED + dead-letter | ✅ `tnbiz_outbox_consumer` 仅 SELECT + UPDATE(consumed_at) + DELETE |
| **Redis** | ✅ AUTH + TLS + ACL（integration §1.6.4 / M-r2-4） | ✅ SETNX nonce 不可 double-write | ➖ 无原始写入 log | ⚠️ jti revocation list 在 Redis；Redis 重启 → revocation 丢失 → JWT 重入（CRITICAL-3） | ⚠️ Pub/Sub publish-rate 限制在订阅端 coalesce；**生产端**无限流 | ✅ Pub/Sub ACL 禁 tnbiz_app publish |
| **OSS** | ✅ presigned URL（TTL ≤ 300s）GET 已约束 | ⚠️ **presigned PUT 的 Content-Type / size / 客户端直传 ACL 未在服务端强制**（CRITICAL-4） | ✅ 访问日志到 SLS | ✅ 私有桶 + 独立 IAM | ⚠️ 任意客户可在 300s 内反复 PUT 大文件耗尽桶配额 | ✅ IAM + 桶策略 |
| **KMS** | ✅ RAM 角色 | ✅ KMS 不导出 | ✅ KMS 访问审计 → SLS | ✅ DEK cache per-(scope, version) + mlock（backend §9.1） | ⚠️ `Decrypt` 速率无硬 cap；攻击者触发大量 PII fetch 可被速率放大 | ✅ 最小权限 |
| **持牌方 / 支付** | ✅ RSA 验签 + IP allowlist（backend §5.7） | ✅ `(channel, out_trade_no)` UNIQUE + amount cross-check | ✅ callback_payload 留证 | ⚠️ `topup_intent.callback_payload TEXT` 未加密；含客户手机号等持牌方回传数据（MED-7） | ✅ webhook `200 ack` 即使 saga 5xx（避免持牌方反复推送） | ✅ `/webhook/payment/*` 不走 JWT，IP 白名单 |
| **阿里云内容安全** | ✅ SDK 签名 | ✅ SDK | ⚠️ **租户级调用链 trace 未强制；滥用溯源到用户级别缺失**（HIGH-13） | ✅ 不回传 PII | ⚠️ 模型调用层无租户 rate-limit + 未落"内容上报闭环"设计（HIGH-13） | ✅ SDK 调用 |

**矩阵审计结论**：7/10 组件合规，**3/10 有 CRITICAL 级 gap**（partner-api Auth 二义 + Redis jti fail-open + OSS PUT server-side）。STRIDE 每列都至少有 1 个 CRITICAL 或 HIGH。

---

## 3. BOLA / IDOR 端到端审计（≥10 个跨租户 endpoint）

PRD §16.3 给了 `RequirePartnerScope` + `CustomerRepo.FindByID(scope, id)` 的 pattern，是正确的。Backend §7.3 把它落成 `RequirePartnerScope` / `RequireCustomerScope` / `RequireStaffScope` 三个中间件；§7.4 加了 `permission.Require(verb)` 做矩阵 check，§2.4 要求 404 而非 403。Frontend §3.4 `<PermissionGuard>` 再做一层前端 guard。**方向全对**，但开发设计里**具体 endpoint 的 repository 查询伪代码里 row-level guard 普遍缺失**，说明架构师写 design 的时候假定了"service 层自会把 scope 带下去"，但实际代码骨架里没写。以下逐条走查：

| # | Endpoint | 跨租户风险点 | 文档位置 | Guard 落地情况 | 评估 |
|---|---|---|---|---|---|
| 1 | `POST /partner/customers/{id}/allocate-quota` | partner A 给 partner B 的客户充额度 | backend §4.4.1 + §5.3 `AllocateToCustomer` | service 层仅 `walletRepo.LockForUpdate(in.PartnerID)`，**未 SELECT customer WHERE id=? AND partner_id=?** | ❌ **CRITICAL-2 首例**：伪代码漏 `customer.partner_id == scope.PartnerID` 断言 |
| 2 | `POST /customer/api-keys` | customer A 创 customer B 的 Token | backend §4.3 + §5.2 隐含 | 服务层未展示；只 bind `fy_user_id` | ⚠️ 依赖 JWT sub 正确提取；未看到明确的 `customer.fy_user_id == jwt.sub` assert 代码 |
| 3 | `DELETE /customer/api-keys/{id}` | customer A 删 customer B 的 Token | backend §4.3 | Fy-api 侧 `/api/internal/token/*` 无 customer 维度 scope | ❌ IDOR 风险：依赖 partner-api 在 DELETE 前 lookup `token.user_id == self.fy_user_id`；未写出 |
| 4 | `GET /partner/customers/{id}` | partner A 读 partner B 的客户详情 | backend §4.4 | 在 PRD §16.3 有 pattern；backend 未显式调用 | ⚠️ pattern 存在，但没在 §5.x 任何一段 service 代码里 demonstrate |
| 5 | `POST /partner/customers/{id}/disable` | 同 #1 路径 | backend §4.4 | service impl 未展开 | ❌ 同 CRITICAL-2 |
| 6 | `GET /partner/wallet/holds` | partner A 读 partner B 的 holds | backend §4.4 | `wallet_hold.partner_id` 索引有；但 service 层 list 未带 `WHERE partner_id=scope.PartnerID` | ❌ 同 CRITICAL-2 |
| 7 | `POST /partner/pricing-rules/{id}/archive` | partner A 归档 partner B 的规则 | backend §4.4 | 未写 service 代码 | ❌ 同 CRITICAL-2 |
| 8 | `POST /customer/transfer-request` | customer 要求转到 partner X，但 X 是其宿主的竞争对手想跨租户读 | backend §5.10.1 | `INSERT customer_partner_change_log (initiator='customer', to=to_partner_id)` 未校验 `to_partner_id` 是否活跃 / invite-code 真实 | ⚠️ MED |
| 9 | `POST /customer/pipl/erase-request` | actor A 触发 actor B 的 erase | backend §5.11 `SubmitErase(customer_id)` | 签名接收 `customer_id` 但 pseudocode **未写 `customer_id == scope.ActorID`** | ❌ **CRITICAL-2 第二例**：PIPL 删除尤为敏感 |
| 10 | `GET /admin/customers/{id}` | staff 读任意 customer，**必须** elevated + audit | backend §4.5 `customer.read (🅰)` + PRD §16.3 | PRD 有 `r.audit.Record(... "customer.read.elevated", ...)`；backend §5.x 未 demonstrate | ⚠️ MED（pattern 对，但未在 backend design 写出） |
| 11 | `POST /admin/refund` | staff 退别 partner 的 revenue_log | backend §5.10.2 `Refund` | `orig := s.revenueRepo.FindByID(ctx, in.RevenueLogID)` 未带 scope；依赖调用方传 `in.PartnerID` | ❌ **CRITICAL-2 第三例**：审批层可能只看 UI 限制，service 层没守 |
| 12 | `POST /admin/saga/{id}/force-resolve` | staff A 解决 staff B 正在介入的 saga | backend §4.5.1 | dual-control 有；但未校验**发起审批的 staff 与 approver 的角色不能相同**（避免 super_admin 自签） | ❌ **CRITICAL-5** |
| 13 | `POST /admin/kyc/{id}/export-pii` | staff 导出任意 KYC | backend §4.8 `kyc.export_pii (🅰 elevated)` | 要求 elevated + audit + KMS decrypt | ⚠️ MED：需要补"导出后短期 URL 过期 + 水印 + 审计跳转"完整链路 |
| 14 | `GET /admin/audit-log` | staff 读 PIPL-tombstoned rows | backend §4.5 + §10.4 | 哈希链完整性保留；PII 单独表 tombstone | ✅ OK |
| 15 | `PUT /admin/biz-settings/{key}` | staff 改 saga_wall_clock / idempotency_ttl 破坏 invariant | backend §4.5 + §14 | 无 schema 校验 / allowlist | ❌ HIGH-7 |

**走查结论**：BOLA pattern 在 **顶层 middleware / repository 接口层面设计对了**，但**在 §5.x `核心 service 流程`伪代码里，90% 的 service 函数没有把 scope 带到 repository query**。Phase 1 编码时很容易把"scope middleware 已经 check 过 actor"当作"所有下游 query 都带了 partner_id filter"——**这就是典型 IDOR 入侵点**。

**强制修复**：
- backend §5.x 每段 service 伪代码**必须显式带上**：
  ```go
  cust, err := s.customerRepo.FindByID(ctx, scope, in.CustomerID)
  if cust.PartnerID != scope.PartnerID {
      return nil, errs.ErrNotFound   // 404 不暴露
  }
  ```
- CI 加 golangci 自定义 analyzer：任何 `repo.Find*` / `Update*` / `Delete*` 调用若第 2 个参数不是 `scope`/`ActorContext` 类型 → build fail（overview I-3.2 升级版）
- §22.2 S-6（CI BOLA 矩阵测试）必须在 Phase 1 第 1 周落地，不能推到 Phase 1 末尾

---

## 4. AuthN / AuthZ / Session 审计 + **鉴权方案 verdict（强约束）**

### 4.1 鉴权路径冲突（CRITICAL-1）

| 位置 | 叙述 |
|---|---|
| PRD §17.1 (line 1854-1856) | "TraceNexBiz 复用 Fy-api JWT（单一源真理）" + "TraceNexBiz 自发的浏览器 cookie：`tnbiz_session`...**HttpOnly**" |
| overview ADR-007 (line 591-595) | "JWT 单一鉴权来源 + tnbiz_session 仅做 CSRF 引导；tnbiz_session cookie **不参与鉴权决策**" |
| backend §7.2 (line 1881-1903) | `tok := extractBearer(c)` —— **Authorization: Bearer header** 提取 |
| backend §13.1 env | 无 JWT cookie 相关配置 |
| frontend ADR-F5 (line 559-567) | "access_token 在 `tnbiz_access` **httpOnly cookie**；refresh_token 在 `tnbiz_refresh` httpOnly cookie；csrf_token 在 `tnbiz_csrf` non-httpOnly cookie（double-submit pattern）" |
| frontend §6.1 (line 545-557) | "不存 access_token 在 localStorage（防 XSS 提权）"——隐含走 cookie |
| frontend §12.4 | 表格列 `tnbiz_access` 为 JWT access cookie |

**结论**：backend 按 Bearer 实现，frontend 按 cookie 实现。这意味着 Phase 1 编码会 100% 碰到联调失败。

两者利弊：

| 方案 | 优点 | 缺点 | 适配本项目 |
|---|---|---|---|
| Authorization: Bearer header | 无 CSRF 风险；与 Fy-api `/api/*` 调用一致（Fy-api SDK 用 Bearer）；微服务友好 | XSS 可盗 token（token 在 JS 可及）；需 in-memory store + refresh race | ❌ 不适合纯浏览器 partner-web（XSS 窗口） |
| httpOnly cookie（access + refresh）+ double-submit CSRF | XSS 无法偷 token；SameSite=Lax 够做跨子域 | 必须解决 CSRF（double-submit + Origin check）；跨子域部署有细节 | ✅ 适合浏览器；前端已设计好 |

**我的强约束 verdict**：

> **采用 httpOnly cookie 方案**，拒绝 Bearer header 方案。
>
> 具体规约（Phase 1 编码前必须 amend 三份文档）：
> 1. PRD §17.1 / overview ADR-007 / backend §7.2 / frontend ADR-F5 **全部统一为**：
>    - access token 在 `tnbiz_access` HttpOnly + Secure + SameSite=Lax cookie（**Path=/**，Domain = 具体子域不带点前缀，避免跨 admin.tracenex.cn 泄露）
>    - refresh token 在 `tnbiz_refresh` HttpOnly cookie，Path=`/auth/refresh` 限定作用域
>    - CSRF token 在 `tnbiz_csrf` non-HttpOnly cookie + `X-Csrf-Token` header 双提交（PRD §17.3 已 baseline）
>    - backend §7.2 `extractBearer` 改成 `extractFromCookie("tnbiz_access")` + fallback Bearer（后者仅用于 server-to-server OAuth2 client creds，**不给浏览器用**）
> 2. 服务端 admin / partner / customer 的三个子域 cookie Domain **严格各自写死**，不用 `.tracenex.cn` 通配（Security M-r2-8）
> 3. JWT sub 直接从 cookie 校验后的 claims 取；不走 Authorization header
> 4. Phase 1 要硬要 Bearer 的场景（例如 partner SDK / CI 测试）单独开一条 `/api/sdk/*` 路径，走独立 Bearer + 强制 HMAC over body（不混入浏览器流量）
>
> **理由（强制）**：
> - 浏览器 XSS 窗口是最常见入侵路径（OWASP A03）；cookie HttpOnly 将 token 直接挡在 JS 之外
> - CSRF 已在 PRD §17.3 要求双提交；落地成本低
> - Fy-api Bearer 用于 API 端 Token 调用（客户 sk-xxx），两者场景不同，不会冲突
> - 与 Fy-api JWT "单一源真理" 不冲突——cookie 承载的仍然是 Fy-api 签发的 JWT，只是传输媒介变化

### 4.2 Session 其他审计

| 项 | 审计 |
|---|---|
| jti revocation list | backend §7.2 `redis.Exists("revoked:jti:"+jti)`；存在 ⚠️ Redis 分区下 fail-open（backend §15.5），**CRITICAL-3**——Redis 宕 → 已撤销 JWT 复活；应 fail-closed，即 Redis 不可达 → 请求拒绝 + SLO 告警 + 手动 bypass 开关（with audit） |
| refresh token 轮换 | frontend §6.4 有单 in-flight refresh promise；backend 未写 refresh endpoint 细节；**不清楚 refresh 是否 rotate（即每次 refresh 下发新 refresh token + 旧 refresh 加入 revoked）**——HIGH-3 |
| MFA 矩阵 | backend §7.5 + frontend §6.5 实现；但 partner **WebAuthn 仍"可选"**（M-r2-3 未闭合），HIGH-4 |
| 登录失败锁定 | backend §7.9 "5 次 / 15 min per (account, IP)"；**但 partner-api 没看到 rate-limit 中间件**，backend §11 没 rate-limit，是"设想"未实现——HIGH-2 |
| 密码重置 | backend §7.9 revoke 全部 jti；PRD §17.5 双因子邮件+SMS；OK |
| argon2id 参数 | backend §7.9 time=3 memory=64MB parallel=2；符合 OWASP；OK |
| password 长度 | PRD §17.4 min 12 + HIBP；OK |
| 账户枚举 | PRD §17.4 统一错误信息；OK |

**AuthN verdict**：CRITICAL-1 + CRITICAL-3 + HIGH-2/3/4 合计 5 条阻塞。Phase 1 前必须全部闭合。

---

## 5. HMAC 签名鉴权（`/api/internal/*`）

integration §1.1.3 的 `internal_auth.go` 做得工整：HMAC-SHA256 over `method\npath\ncanonical_query\nts\nnonce\nsha256(body)`，unix epoch seconds，±300s 窗口，Redis SETNX nonce TTL 5min，endpoint allowlist per kid，mTLS 由 sidecar 层终结。**设计方向正确**。

**问题**：

- **HIGH-8**：`c.Request.TLS != nil` 作为 mTLS 兜底在 K8s Istio/Linkerd mesh 下永远为 nil（TLS 在 sidecar 终结，进 gin 时已是明文 HTTP loopback）。正确做法是：
  - 在 Istio 一侧设置 `AuthorizationPolicy` + `PeerAuthentication(STRICT)`
  - 在 gin 中间件信任 **sidecar 注入的 header**（如 `X-Forwarded-Client-Cert`）并验签发 CN 白名单
  - 同时在 K8s NetworkPolicy 把 Fy-api Pod 只暴露给 partner-api ServiceAccount
- **MED-1**：`KeyStore.Lookup(kid)` 没有规约 refresh 机制；key rotation 期间旧 pod 不知道新 kid。需加 Redis Pub/Sub `hmac_key_update` + 启动期 watch
- **MED-2**：`KeyStore` 返回 `(secret, allowedEndpoints, ok)`；**HMAC secret 长度和 entropy 无约束**——规约必须 `len(secret) >= 32 bytes && CSPRNG-generated`
- **MED-5**：Redis 作 nonce store，rider 攻击者可用巨量 nonce 塞满 key space（SETNX + TTL）导致 Redis 内存耗尽。对策：per-kid nonce 命名空间 + per-kid quota；integration §9.3 `internal_auth_nonce_replays_total` metric 有但 per-kid 上限没有

---

## 6. 幂等键安全（PRD §18 + backend §8）

**设计合格点**：
- `idempotency_record` `UNIQUE(actor_type, actor_id, idempotency_key, endpoint)`（backend §3.16）**隔离了 actor 命名空间**——partner 的 key 伪造成 customer 的不会命中
- response body 用 `system DEK` 加密存储（backend §3.16 `response_cipher`），Round-2 M-r2-5 已闭合
- TTL 24h（client）< 7d（Fy-api `internal_idempotency`），覆盖 saga 1h wall-clock cap

**问题**：

- **MED-3**：客户端可控 UUIDv7 的 idempotency-key **可能被前端代码通过日志漏出**（Sentry breadcrumb / 错误 toast 带 trace_id 时一并带了 idem-key）。应在前端 scrubber 中把 `Idempotency-Key` 列入必过滤 header 名单（frontend §15.1 未列）
- **LOW-1**：backend §4.3 `/customer/api-keys POST` 标 `idempotency: required`，但 `/customer/invoice POST` 也标 required——发票申请的幂等 key 若被偷，攻击者能"重放"同一 PDF 下载链接（response_cipher 里含 invoice_url）。应在 service 层对"含 PII/敏感下载 URL 的 response"额外加短 TTL（1h 而非 24h）
- **LOW-2**：`saga_step.Payload`（backend §3.17）TEXT 存 JSON；未加密。虽然 Security M-r2-7 已改走 redacted view 输出日志，但**DB 里 payload 仍然明文**——saga 用到的 saga_id + customer_id + amount 不是 PII，但如果未来有 KYC saga，payload 可能含 PII；必须禁止 saga 跨域使用（不允许 KYC saga 用 payload 字段存 id_no）

---

## 7. PII 流转审计（每条 ✅/⚠️/❌）

按 PRD §16.5 PII 矩阵 9 类字段 × "流转链路"：

| PII | 前端 → partner-api | partner-api → DB | DB 存储 | partner-api → Fy-api | partner-api 对前端回显 | 日志 | 评估 |
|---|---|---|---|---|---|---|---|
| 邮箱 | ✅ HTTPS | ✅ service 标 `pii:"true"` | ⚠️ `partner.contact_email` VARCHAR(128) **未加密**（backend §3.1）——PRD §16.5 说"否加密"，但至少应加 blind index + 不输出到 error | ➖ 不透传 | ⚠️ frontend §9.1 `j***@example.com` | ✅ scrubber pattern | ⚠️ |
| 手机号 | ✅ | ✅ AES-GCM 信封（`contact_phone_cipher`, backend §3.1） | ✅ | ➖ | ✅ 脱敏 | ✅ scrubber | ✅ |
| 身份证号 | ✅ | ✅ KMS 信封（`legal_person_id_cipher`, backend §3.9）| ✅ | ➖ | ⚠️ admin KYC export 才可见 | ✅ | ⚠️ **blind index 缺失**——同一身份证号二次提交不能被 partner-api 检测到重复（HIGH-11） |
| 法人姓名 | ✅ | ✅ | ✅ | ➖ | ⚠️ 脱敏 `张*` | ✅ | ⚠️ 同上 blind index |
| 法人身份证图片 | ✅ presigned PUT | ⚠️ ossKey 明文落 `legal_person_id_url`（backend §3.9）；URL 本身不是 PII 但若 URL 可直接访问桶则等价 | ✅ OSS KMS 加密 | ➖ | ✅ presigned GET TTL ≤ 300s | ✅ URL 不进 log（§19.6） | ⚠️ **关键：PUT 面没有 server-side 大小/类型/magic-byte 校验**（CRITICAL-4） |
| 营业执照 | 同上 | 同上 | 同上 | — | 同上 | ✅ | 同 CRITICAL-4 |
| 支付宝实名 | ✅ | ✅ AES-GCM | ✅ | — | ⚠️ admin 见 | ✅ | ✅ |
| 银行卡号 | ✅ | ⚠️ backend design 内**没有 bank_account 字段**（也许 Phase 2A 才上），但 PRD §16.5 在矩阵里列了——HIGH-11 blind index | — | — | — | — | ⚠️ 等 2A |
| 人脸 | ⚠️ 前端 §11.3 Permissions-Policy `camera=(self)` 允许；但**存储策略**——"完成认证后立即清"（PRD §16.5）未在 backend 落到 migration/cron job | — | — | — | — | — | ❌ **HIGH-10**：biometric 信息生命周期未落实 |

**PII 流转总体评估**：加密通道 ✅，存储加密 ✅（但 email 未加密不一致），**blind index 缺失是 HIGH**，**生物识别清理机制缺失是 HIGH**。日志 scrubber 合规。前端回显脱敏合规。

---

## 8. 加密 / 密钥审计（KMS / 信封 / OSS）

**合格点**：
- Round-2 HIGH-r2-2 闭合：`dekCache` per `(scope, key_version)`（backend §9.1），`scope=tenant:{partner_id}` 或 `system`
- `cmd/kek-rotator` + DEK rotator（backend §6 worker 清单）quarterly rotation
- `mlock` 锁内存页 + pprof 关闭（ADR-009）
- OSS presigned GET `ttl <= 300s`（backend §9.4 `PresignGet` 硬 assert + CI AST scan）
- envelope encryption pattern 落地正确（DEK 加密形态 + `encryption_key_id` 列）

**问题**：

- **CRITICAL-4**：`PresignPut` 函数签名在 backend §9 / frontend §7.2 都没有；只讨论 GET。presigned PUT 必须：
  - 服务端签名时带 `x-oss-content-md5` + `Content-Length`（上限 10MB）+ `Content-Type` allowlist（image/jpeg|png|webp）
  - bucket policy 拒绝 Content-Type header 与签名不匹配的 PUT
  - 不在客户端直接暴露 `accesskey` —— 仅 presign URL
  - HEAD 校验 upload 完成 + magic-byte 二次验证（不信任 Content-Type）
  - integration-design / backend-design 必须在 Phase 1 内把 `PresignPut(..., allowedMime, maxBytes, ttl<=300)` 补上
- **HIGH-11**：bank_account / id_no 加密字段的**blind index**（deterministic HMAC-based）缺失。不能查重复，不能精确查询。设计需加 `*_blind_index VARCHAR(64) AS (SHA-256 HMAC(row_key, plain))` 列（Phase 2A 前）
- **HIGH-10**：生物识别（人脸）生命周期未设计。PRD §16.5 要求"完成认证后立即清"，但 backend design 没有对应 migration / cron / service 代码
- **MED-4**：KEK 12 个月手工轮换（PRD §19.4）；**prod 实际很少真的轮**。应加 KMS 审计事件"KEK 未轮超 380 天"→ PagerDuty
- **MED-8**：DEK rotation 90d（ADR-009）——**re-encrypt batch 期间的一致性**：老数据 Decrypt 用旧 DEK、新写用新 DEK；中间态如果 batch crash，需有 resume cursor（backend §9 kek-rotator 提到 progress_offset，DEK rotator 伪代码没有）
- **MED-9**：`Encrypted` GORM wrapper (backend §9.2) 的 `Reveal(kms)` 未明确 zero-after-use；文档建议 `runtime.GC() + zero []byte` 但不应依赖 GC。应用 `unsafe` / `subtle.ConstantTimeCopy` + 显式覆写

---

## 9. OSS presigned URL（§19.6）

backend §9.4 `PresignGet(bucket, key, ttl)` 强 assert ≤ 300s + `content-disposition=attachment` + CI AST scan。✅

**PresignPut 缺失**——见 CRITICAL-4。

**其他问题**：
- **MED-10**：frontend §12.1 CSP `connect-src 'self' https://api.partner.tracenex.cn wss://api.partner.tracenex.cn` **没有允许 OSS 域**！客户端浏览器无法 PUT 到 OSS。应加 `https://*.oss-cn-hangzhou.aliyuncs.com`（region-specific）
- **LOW-3**：presigned URL 没绑定 client IP（阿里云 OSS STS 支持 `ipaddr` 策略）。增加"签发 IP ± /24"缩小横向攻击面
- **LOW-4**：OSS 桶未设置 `Referer 白名单`+`Object Lock`；即使 presigned URL 正确也应限制请求来源

---

## 10. 审计日志 / 哈希链（§8.13 + backend §10）

Round-2 HIGH-r2-1 已闭合：`audit_log_unsealed` + `cmd/audit-sealer` leader + sealer 用户 UPDATE，app 用户仅 INSERT（backend §3.13 + §10.1 + GRANT integration §6.2）。✅

**问题**：
- **HIGH-12**：backend §3.13 `target_id BIGINT NOT NULL`；但 `biz_setting` 的 key 是 string（`jwt_public_key_pem` 等）；无法审计配置变更。应改 `target_id VARCHAR(128)` 或加 `target_key VARCHAR(128) NULL`
- **MED-11**：sealer 单 leader fallback 依赖 K8s Lease（overview ADR-006）；如 leader 长时间 down 且 `audit_log_unsealed` 持续堆积，`verify-chain` 在 PIPL 47 tombstone 期间可能产生误报
- **LOW-5**：`audit-verify CLI`（backend §10.3）daily cron 没定义 failure 响应 ladder（mismatch → page on-call；但 page 后的 runbook 未写）

---

## 11. 安全 headers / CSP / CORS / Cookie

- **CSP**：frontend §12.1 `script-src 'self' 'nonce-{NONCE}' 'strict-dynamic'` ✅；`object-src 'none'` ✅；`frame-ancestors 'none'` ✅；但 `connect-src` 漏 OSS 域（MED-10）；`img-src 'self' data: https://*.aliyuncs.com` OK
- **HSTS**：`max-age=31536000` ≥ preload baseline ✅
- **COOP/COEP**：frontend §12.5 admin 启用 `Cross-Origin-Embedder-Policy: require-corp`；但 portal 没启（混合 iframe 场景下 COEP 会打破第三方 OAuth popup 等）——**LOW-6**：审视是否按需启用
- **CORS allowlist**：frontend §12.3 + backend PRD §17.6 明示列举，无 `*`，允许 credentials ✅；但**缺**：preflight `OPTIONS` 未限制 `Access-Control-Max-Age`（默认 5 min 偏长，建议 600 + 强制 recheck on origin change）
- **CSRF**：double-submit + Origin/Referer allowlist ✅；但 backend §7.6 `RequireOriginAllowlist` 伪代码在 `origin == ""` 时 fallback Referer——**Referer 在某些浏览器可能被隐藏**（Referrer-Policy 就是自己设的 `strict-origin`，同源仍会带，OK，但要 audit 一遍跨站 flow）
- **Cookie flags**：SameSite=Lax（跨子域 partner↔admin 可能被阻断；但前端已声明两站 cookie 不共享，OK）；HttpOnly + Secure ✅；**Domain 明示**（不用 `.tracenex.cn`）是正确的

---

## 12. 支付 / 资金安全（§7.6）

backend §5.7 + integration §4.5 设计合格：
- `(channel, out_trade_no)` UNIQUE
- RSA 验签 + IP allowlist
- amount cross-check
- saga_id = topup_intent.id（服务端生成）
- webhook 总是 200 ack 避免持牌方重推

**问题**：
- **HIGH-14**：`/webhook/payment/{provider}` 没有 request body size limit；持牌方可推大 body 进行 ReDoS / 反序列化攻击
- **MED-7**：`topup_intent.callback_payload TEXT` 未加密，可能含客户手机号（持牌方回传）
- **MED-12**：退款 saga（backend §5.10.2）对**三种 settlement 状态**分支处理，但**已支付后退款** fallback 到 "负 balance + partner_wallet_log"——F-2 待落地。如果 partner 账户已 terminate，负 balance 无法回收——清算逻辑必须先解决

---

## 13. KYC 安全（§7.7）

backend §5.6 presigned PUT + OCR + KMS 信封 + audit。

**已覆盖**：
- TTL ≤ 300s presigned
- DEK per-tenant
- tombstone 机制

**问题**：
- **CRITICAL-4**（复述）：PUT 服务端校验
- **HIGH-10**（复述）：biometric 清理
- **MED-13**：OCR 返回字段（公司名 / 法人名 / id_no）直接落 `business_license_ocr TEXT` 明文（backend §3.9）——PRD §16.5 说法人名/id_no 敏感，需加密；**设计层把 OCR 解析结果存明文 TEXT 违反自己的矩阵**

---

## 14. 内容安全 / 滥用防护（§7.12）

backend §1 / §3 / §4.11 有 `content_safety` 模块 + admin 审核；frontend §7.10 UI。

**问题**：
- **HIGH-13**：模型调用层**没有 per-tenant rate-limit**——攻击者借其控制的一批 customer 账户向 Fy-api 发送大量命中审核的请求，Fy-api 反应合规，但内容安全平台费用由 TraceNex 买单。需在 `/api/internal/*` 一侧加 per-kid + per-user rate-limit
- **HIGH-13**：内容**上报闭环**（M12-04 12377）对接时序未工程化（backend §3 `content_safety_event` Phase 2A 才建表，Phase 1 无表但允许"mock/真实开关"）——Phase 1 上线前若启用真实供应商，事件必须已有存储

---

## 15. 依赖 / 供应链（CRITICAL-6）

**四份文档几乎没有讨论这块**：
- backend §14 只讲数据库迁移，没讲 Go module audit / vuln DB
- frontend §20 F-R11 提到 `npm audit + Snyk` 但没写入 CI gate 矩阵
- **无 SBOM 生成规约**
- **无镜像签名 / Cosign / Notary 规约**
- **无 base image 钦定**（`FROM golang:X.Y-alpine` vs distroless？）
- **无 Fy-api 覆盖层 upstream vuln 追踪机制**

这是 OWASP A06 + A08 + 等保 2.0 "入侵防范" 必查项，**必须在 Phase 1 前补上**。

---

## 16. secret 管理（§19.5）

backend §13.2 `external-secrets-operator` 从 KMS Secret Manager 同步；OK。
- **LOW-7**：biz_setting 表 TEXT 无字段分类；不能防止运维把"AK/SK"当作"配置项"写进去。应加 `biz_setting.value_type ENUM('plain','secret_ref')`，secret 类只存 KMS Secret ARN
- **LOW-8**：CI sealed secret → 应禁 `echo $SECRET` 类 runner log 泄露；`actions/secret-*` masker 是否 on？未 doc

---

## 17. 暴露面审计

| Endpoint prefix | 暴露面 | 访问控制 | 评估 |
|---|---|---|---|
| `/public/*` (storefront) | 互联网 | 无鉴权 | ✅（无状态内容 + 招商申请 idempotency） |
| `/customer/*` / `/partner/*` (portal) | 互联网 | JWT + scope + MFA | ✅ |
| `/admin/*` (admin.tracenex.cn) | 互联网 | JWT + step-up MFA + WebAuthn（部分）+ 水印 | ❌ **HIGH-9**：平台管理后台**无 VPN / IP 白名单**——即使 MFA 齐全也应限制源 IP 为公司办公网 / VPN 出口 + 零信任（Cloudflare Access / AWS Verified Access 级） |
| `/webhook/payment/*` (callback.partner.tracenex.cn) | 互联网 | RSA 签名 + IP allowlist | ✅ |
| `/api/internal/*` (Fy-api) | VPC 内网 | mTLS + HMAC | ✅（但 HIGH-8 提到 mTLS 边界） |
| `/healthz/*` | 见 backend §13.3；live/ready | 未限制 | ⚠️ 应限制 live 端点 body 大小 + ready 端点禁止暴露依赖详情到公网 |
| pprof / metrics | backend §9.1 提到 pprof prod 关闭；metrics 端点未 doc | — | ⚠️ metrics 端点（Prometheus `/metrics`）若暴露内含 saga / wallet 信息，等于信息泄露——**MED-14** |

---

## 18. §22.2 Pre-launch 安全验收 8 项 gates 落地表

| # | 验收项 | 文档落地 | Phase 1 可机器检测？ | 评估 |
|---|---|---|---|---|
| S-1 | F-7 完成 + audit_log 并发压测 + hash chain 一致性校验 | backend §3.13 + §10 + §15.2 I-A-2/3 + §15.5 chaos | ✅ integration test 10k 并发写 + 每日 verify CLI | ✅ |
| S-2 | F-8 完成 + 卡死 saga 可被 staff 解锁 | backend §4.5.1 force-resolve + integration §4.3.2 | ⚠️ dual-control 可绕（CRITICAL-5）；解锁机制存在但不安全 | ❌ |
| S-3 | F-9 完成（partner KYC pass 强制 MFA） | backend §7.5 + frontend §6.5 | ✅ e2e 验证 | ✅（WebAuthn 强制未落 → HIGH-4） |
| S-4 | Staff Elevated step-up MFA | backend §7.5 | ✅ e2e 不带 step-up 被拒 | ✅ |
| S-5 | outbox SKIP LOCKED / 单 leader | integration §3.3 + backend §5.4 + overview ADR-011 | ✅ 多 poller 压测 | ✅ |
| S-6 | CI BOLA 矩阵测试 wired | overview I-3.2 + backend §15.3 | ⚠️ 设计说"从 matrix 生成"；**实际 §5.x service 伪代码未带 scope**（CRITICAL-2）——即使有 CI 测试也会发现 10+ endpoint fail | ❌ |
| S-7 | app DB user 无 audit_log UPDATE/DELETE | integration §6.2 GRANT + backend §15.2 I-A-1 | ✅ `SHOW GRANTS` CI 金文件 | ✅ |
| S-8 | 所有 `Encrypted*` 字段含 `json:"-"` | backend §9.2 + PRD §19.3 + ADR-F6 | ✅ go AST check | ✅ |

**gate 通过率**：6/8 设计到位，2/8（S-2 / S-6）因 CRITICAL-2/5 连带失败。

---

## 19. CRITICAL 漏洞清单（必须修，否则不能上 Phase 1）

| ID | 名称 | 位置 | 风险 | 修复要求 |
|---|---|---|---|---|
| **CRITICAL-1** | 鉴权方案 Bearer vs Cookie 二义 | backend §7.2 vs frontend ADR-F5 vs PRD §17.1 / overview ADR-007 | 编码期必出联调失败；XSS 时 token 泄露窗口不清 | 按本文 §4.1 **verdict**：全系走 httpOnly cookie；backend §7.2 改 `extractFromCookie`；PRD / overview / frontend 同文变更 |
| **CRITICAL-2** | BOLA / IDOR — service 伪代码普遍漏 row-level guard | backend §5.3 / §5.10.2 / §5.11 / §4.4 multiple endpoints | 跨租户读写；wallet / KYC / refund / erase 可越权 | §5.x 每段 service 代码必须显式 `repo.FindByID(ctx, scope, id)`；CI golangci analyzer 强制第 2 参 `ActorContext`；Phase 1 第 1 周落地 S-6 矩阵测试 |
| **CRITICAL-3** | JWT revocation fail-open on Redis partition | backend §15.5 | Redis 宕 → 撤销的 JWT 复活 → session fixation / compromised token 继续可用 | 改 fail-closed（Redis 不可达 → 拒请求 + alert + 人工 open-switch）；备选：JWT 加极短 TTL（≤ 2min） + refresh 在服务器 side session table（消除 revocation list 必要） |
| **CRITICAL-4** | OSS presigned PUT 服务端强校验缺失 | backend §9.4（无 `PresignPut`）+ frontend §7.2 | 任意用户可上传任意 MIME、任意大小、任意位置的文件到 KYC 桶；SSRF / 病毒 / PII 跨租户污染 | 新增 `PresignPut(bucket, key, ttl<=300, mime allowlist, maxBytes, hmac content-length & content-type)`；桶 policy 拒无签 Content-Type 的 PUT；partner-api 在 `/kyc/applications` 入口 HEAD + magic-byte re-validate；CI AST scan |
| **CRITICAL-5** | saga force-resolve dual-control 可被时间窗叠加 | backend §4.5.1 | 一个超管连续两次 step-up → 叠加两次 elevated → 自签 dual-control，对应 PRD §8.17 的"资金卡死解决"被单人绕过 | dual-control 必须：① 两个不同 `staff.id`；② `staff.role` 分别为 super_admin + finance（不可同角色）；③ second_approver_token 是一次性 + 服务端 challenge-response（不复用 `elevated_until` 窗口）；④ 两次审批都写 audit_log；⑤ 两个审批 staff 的 IP 不得同一 /24 |
| **CRITICAL-6** | 供应链 / 依赖 / SBOM 全缺失 | 四份文档均无 | Go module / npm / 镜像层漏洞无防御；Log4shell 级别事件不可控 | Phase 1 编码前新增章节（overview §10 或 backend §13.5）：Go `govulncheck` + `nancy` CI gate；npm `audit high`=0 CI gate；镜像 `cosign sign` + SBOM 生成（`syft`）；base image 钦定 distroless；Fy-api 覆盖层追踪 upstream CVE |
| **CRITICAL-7** | `biz_setting.jwt_public_key_pem` 存入 biz_setting TEXT 列 | backend §7.2 + §13.1 | biz_setting 允许 super_admin 改（§4.5 PUT /admin/biz-settings/{key}）——**改 JWT 公钥等于签发任意 JWT**；权限矩阵只有 `system.config_write` 一层（PRD §3.4 未单拆） | JWT 公钥**必须**从 KMS Secret Manager 注入；biz_setting 仅存**非安全关键**配置；新增 `biz_setting.value_type` 列限制；`jwt_public_key_pem` 等移出 biz_setting；PRD §3.4 `system.config_write` 拆成`config_write.trivial` / `config_write.security`，后者要求 dual-control |

---

## 20. HIGH 漏洞清单

| ID | 名称 | 位置 |
|---|---|---|
| **HIGH-2** | 全局 rate-limit 中间件未设计（`/auth/login` 爆破、`/auth/refresh` 滥用、content-safety 费用放大） | backend §7 / §11 缺；integration §1.1 key-level quota 缺 |
| **HIGH-3** | refresh token rotation 策略不清（是否每次 refresh rotate + revoke 旧 refresh） | backend §7 + frontend §6.4 |
| **HIGH-4** | partner WebAuthn 未按 M-r2-3 阈值强制（wallet > ¥1k / monthly payout > ¥10k） | backend §7.5 + frontend §6.5 + PRD §17.2 |
| **HIGH-7** | `biz_setting` 改动在 `/admin/biz-settings/{key}` 无 schema / allowlist / dual-control | backend §4.5 |
| **HIGH-8** | mTLS 在 K8s mesh 下 `c.Request.TLS != nil` 永 false；无 sidecar cert header 校验 | integration §1.1.3 |
| **HIGH-9** | `admin.tracenex.cn` 无 VPN / IP 白名单 / Zero-Trust 访问层 | frontend §1.2 ADR-F1 + backend §13 |
| **HIGH-10** | 生物识别（人脸）"完成认证后立即清"未落 migration / cron | backend §3.9 + PRD §16.5 |
| **HIGH-11** | 加密字段 blind index 设计缺失（id_no / bank_account 查重） | backend §3.9 + PRD §19 |
| **HIGH-12** | `audit_log.target_id BIGINT` 不支持 biz_setting 等 string target | backend §3.13 |
| **HIGH-13** | 内容安全 per-tenant rate-limit + 上报闭环 | backend §4.11 + PRD §7.12 |
| **HIGH-14** | `/webhook/payment/*` body size limit / ReDoS 防御 | backend §5.7 |

---

## 21. MEDIUM 清单

| ID | 名称 | 位置 |
|---|---|---|
| MED-1 | KeyStore 缺 refresh 机制（HMAC key rotation 期间新 kid 未知） | integration §1.1.3 |
| MED-2 | HMAC secret 长度 / 熵约束未规约 | integration §1.1.3 |
| MED-3 | Idempotency-Key 在前端 Sentry breadcrumb / toast trace_id 未列入 scrubber 名单 | frontend §15.1 |
| MED-4 | KEK 12 个月未轮 alert 缺失 | backend §9.3 |
| MED-5 | Redis nonce store 可被塞爆（per-kid 配额缺） | integration §1.1.3 |
| MED-6 | `consume_log_outbox.last_error TEXT` 可能回写 PII（Fy-api 业务 error） | integration §1.5.2 + §3.1 |
| MED-7 | `topup_intent.callback_payload TEXT` 未加密（含持牌方回传 PII） | backend §3.21 |
| MED-8 | DEK rotator re-encrypt batch 无 resume cursor | backend §9.3 + §6 worker 清单 |
| MED-9 | `Encrypted.Reveal` 后明文 `[]byte` zero-after-use 未强制 | backend §9.2 |
| MED-10 | frontend CSP `connect-src` 未列 OSS 域（PUT 会被阻断） | frontend §12.1 |
| MED-11 | sealer leader 长时间 down + PIPL tombstone 时 verify-chain 误报 | backend §10 |
| MED-12 | "已支付后退款" 负 balance 对已 terminate partner 无回收 | backend §5.10.2 |
| MED-13 | OCR 解析结果 `business_license_ocr TEXT` 明文存 | backend §3.9 |
| MED-14 | Prometheus `/metrics` 端点未限制访问（内含 saga / wallet gauge） | backend §12 |

---

## 22. LOW 清单

| ID | 名称 |
|---|---|
| LOW-1 | 发票下载类 response_cipher TTL 24h 偏长（含 invoice_url），建议 1h |
| LOW-2 | `saga_step.Payload` 明文（当前字段非 PII 但未来演进需约束） |
| LOW-3 | OSS presigned URL 未绑定 client IP |
| LOW-4 | OSS 桶未启用 Object Lock / Referer 白名单 |
| LOW-5 | audit-verify CLI failure ladder runbook 未写 |
| LOW-6 | portal COEP/COOP 启用策略未审（部分浏览器需要） |
| LOW-7 | `biz_setting.value_type` 未分类（plain vs secret_ref） |
| LOW-8 | CI sealed secret masker 未验证 |
| LOW-9 | pricing markup 上界 100.0（PRD 建议 default 5.0）偏松 |

---

## 23. 修订指令（具体修复方向）

> 每条给出：**改哪份文档 / 哪节 / 加哪段**。Phase 1 编码前必须 amend，否则 BLOCK。

1. **CRITICAL-1**：PRD §17.1 + overview ADR-007 + backend §7.2 + frontend ADR-F5 **同文修订**：
   - PRD §17.1 第二段改为："JWT 承载方式 = HttpOnly cookie `tnbiz_access`（Path=/, Domain=子域明示, Secure, SameSite=Lax）；不使用 Authorization Bearer header（除 `/api/sdk/*` 的 server-to-server 场景）"
   - backend §7.2 `extractBearer` → `extractFromCookie("tnbiz_access")`，加 fallback 到 Bearer 仅用于 `/api/sdk/*`
   - overview ADR-007 增一段 "cookie 承载 JWT"
   - frontend ADR-F5 与上统一

2. **CRITICAL-2**：backend §5.3 / §5.10.2 / §5.11 及 §4.4 所有 service 伪代码**增加 row-level guard**；overview §3 增 I-3.4 "每个 repository Find*/Update*/Delete* 第 2 参必须 ActorContext"；`backend §15.2` 增"BOLA 矩阵 CI gate 在 Phase 1 第 1 周上线"

3. **CRITICAL-3**：backend §7.2 中间件改 fail-closed；backend §15.5 chaos 删除"revocation fail-open"语句；overview §10 风险表新增 A-9 "Redis 分区 → JWT revocation 降级"

4. **CRITICAL-4**：backend §9.4 新增 `PresignPut(bucket, key, mime, maxBytes, ttl<=300)`；integration §1 + backend §5.6 KYC 流补服务端 HEAD + magic-byte 校验；CI AST scan；frontend §12.1 CSP 加 OSS 域

5. **CRITICAL-5**：backend §4.5.1 dual-control 条件改为"两不同 staff + 不同角色 + 不同 /24 IP + 一次性 challenge-response token"；audit 两次审批；PRD §8.17 增 "saga.force_resolve 的 approver 约束"

6. **CRITICAL-6**：overview §10 / backend §14.5（新节）/ frontend §17 加入"供应链防御"：
   - Go: `govulncheck` + `nancy` CI gate；Go module 锁 checksum
   - Node: `pnpm audit --audit-level=high` CI gate；`pnpm-lock.yaml` 必交付
   - Image: distroless base + cosign sign + SBOM (syft)
   - Fy-api 覆盖层 upstream CVE 追踪机制（weekly-upstream-sync-runbook 已有，纳入 Phase 1 Gate）

7. **CRITICAL-7**：backend §13.1 env 表 `JWT_VERIFY_KEY_PEM` 从 KMS Secret Manager 注入（非 biz_setting）；backend §3.15 biz_setting 加 `value_type ENUM('plain', 'secret_ref')`；PRD §3.4 `system.config_write` 拆 `.trivial` / `.security`，`.security` dual-control

8-11. HIGH 2/3/4/7：backend §7 + frontend §6 + integration §1.1 新增 rate-limit middleware 清单（`/auth/login` 5/min/account+IP / `/auth/refresh` 30/min/account / `/public/partner/apply` 3/hour/IP / content_safety 100/min/tenant）+ refresh rotation 规约

12. HIGH-8：integration §1.1.3 删除 `c.Request.TLS != nil` 兜底；改为 sidecar header CN 校验 + K8s NetworkPolicy 规约

13. HIGH-9：frontend §12 / backend §13 新增 "admin.tracenex.cn 访问控制"——企业 VPN / IP allowlist / Zero-Trust（Cloudflare Access 或阿里云 IDaaS）

14. HIGH-10：backend §3.9 / §6 worker 清单新增 `cmd/biometric-purge`（完成认证后立即清）+ 单元测试

15. HIGH-11：backend §3.9 + PRD §19.3 新增 `*_blind_index VARCHAR(64)` 列 + HMAC-based scheme

16. HIGH-12：backend §3.13 `target_id` 改 VARCHAR(128) 或加 `target_key`

17. HIGH-13：backend §4.11 + PRD §7.12 补 per-tenant rate-limit + content_safety_event Phase 1 schema（即使值默认 mock 也要建表）

18. HIGH-14：backend §5.7 webhook handler 增 `BodyLimit(1MB)` + JSON parser strict + timeout 3s

19. MEDIUM 1..14：见 §21 逐条；集中修到对应文档位置

20. LOW 1..9：Phase 2B 前完成

---

## 24. 附：与 Round-2 verdict 的对照

Round-2 的 `round-2/03-Security-review.md` 验收：CRITICAL=0，HIGH=2，MEDIUM=8，LOW=5，**ACCEPT_AS_V1.0**。

Dev Round-1 结果：CRITICAL=7，HIGH=11，MEDIUM=14，LOW=9，**BLOCK**。

看似"退步"，其实是**抽象层变化**：Round-2 审的是 PRD（policy），Dev Round-1 审的是三份 dev 文档（engineering）。PRD 正确不等于实现正确——我们现在终于看到了 goroutine、GRANT、middleware、伪代码——这才是 bug 栖息地。

Round-2 的两条 HIGH（hash chain concurrent insert + DEK per-scope）在 dev-round-1 都已闭合；这是架构师团队的工作成果。但把 PRD 的 §16.3 BOLA pattern 翻译成 `internal/{domain}/service.go` 的伪代码时，90% 的 service 函数没有把 scope 带下去——这就是 CRITICAL-2 的来源；把 PRD §17.1 翻译成 cookie / header 时，前端和后端朝两个方向走——这就是 CRITICAL-1。

**结论**：此为"翻译阶段缺陷"，不是"设计方向错误"。修复工作量在 2-4 人日内可收敛（7 条 CRITICAL 每条 0.5 天文档修订 + 对应代码约束定义）。

---

## 25. 下一步

1. **架构师 + Backend + Frontend 三方在 2 天内同文 amend**四份文档（§23 修订指令全执行）
2. Security round-2（本轮的下一轮）再审；若 CRITICAL=0、HIGH≤3 则 ACCEPT，进入 Phase 1 编码
3. Phase 1 编码第 1 周交付：
   - BOLA 矩阵 CI gate（§22.2 S-6）实操
   - `PresignPut` 工程实现 + CI AST scan
   - rate-limit middleware baseline
   - 供应链防御 CI gate baseline
4. Phase 1 验收：§22.2 S-1..S-8 全通过 + 本文 CRITICAL/HIGH 全部闭合

**当前 verdict：BLOCK。** Dev Round-1 不通过。等修订后走 Dev Round-2。

— Security agent，Dev Round 1
