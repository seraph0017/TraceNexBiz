# Code Round-1 Review — Security Engineer (Application Security)

> 日期：2026-05-12
> Reviewer：Security agent（OWASP ASVS L2 + 等保 2.0 二级 + PIPL 合规协同口径 + OWASP API Top 10 2023）
> 范围：
> 1. `apps/partner-api/`（cmd / internal / pkg / migrations / openapi / .golangci.yml / .github/workflows）
> 2. `apps/partner-web-{storefront,customer,partner,admin}/src/api/client.ts` 等鉴权链路
> 3. `Fy-api/` OVERLAY：`controller/tnbiz_internal/`、`middleware/internal_auth.go` + `internal_idempotency.go`、`model/internal_api_key.go` + `internal_idempotency.go`、`router/api-internal-router.go`
> 输入：Dev Round-2 Security verdict（`reviews/dev-round-2/03-Security-review.md`）—— PASS-CONDITIONAL，含 7 项强约束 + 4 MED-r2 残余 + 2 ACCEPT-AS-DEBT + 4 项 Phase 1 验收钩
> 门槛：**0 CRITICAL / ≤ 3 已挂账 HIGH** 才能 PASS-CONDITIONAL；任何"声称已实现但代码留 TODO 占位"算 partial fix，不计入 FIXED
> 本轮 verdict：**NEEDS REWORK（PARTIAL ACCEPT）**

---

## 1. 执行摘要 + verdict

### 1.1 总评分

| 维度 | Dev Round-2 | Code Round-1 | Δ |
|---|---|---|---|
| 设计层 CRITICAL（Round-2 已闭合） | 0 | **0** | — |
| 代码层 CRITICAL（应作为 Phase 1 W1a 必交付） | n/a | **5**（C1～C5；7 项强约束中 4 条仍是 TODO 注释） | ❌ |
| 代码层 HIGH | n/a | **9** | ❌ |
| MEDIUM | 4 | 11 | +7 |
| LOW | 2 | 6 | +4 |
| 总体评分 | B+（dev round 准入）| **C（不可在本轮直接进 Phase 1 Week 2/3，需 W1a 收尾补齐 C1～C5）** | — |

### 1.2 核心结论

**Dev Round 设计文档已经把强约束写得很清楚，但 Code Round-1 在 partner-api 里看到的是大量"W0 scaffold + TODO(W1a/W1c)"占位**。具体而言：

1. `internal/middleware/auth.go` 的 `JWT()` 函数体只有 `c.Next()`，所有 cookie 提取、验签、`fail-closed` revocation lookup 全部是注释（行 48-54）——**强约束 #1（JWT 全系 cookie）+ #2（fail-closed）未在中间件层兑现**。
2. `internal/middleware/csrf.go` 的 `CSRF()` 在写方法上不做 token 比较、不做 Origin/Referer 校验（行 27），直接 `c.Next()`——**强约束 #1 后半 double-submit CSRF 同样未兑现**。
3. `internal/middleware/bola_scope.go::BOLAScope()` 没注入 scope（行 19），`internal/middleware/idempotency.go::Idempotency()`、`internal/middleware/audit.go::Audit()`、`internal/middleware/webhook_idempotency.go::WebhookIdempotency()`、`internal/middleware/pii_scrubber.go` 全部是同样的 stub 模式——**§22.2 S-2 / S-6 在中间件层是空壳**。
4. `internal/handler/w1a_routes.go::scopeOf` 在生产二进制里仍读 `X-Dev-Actor-Type`/`X-Dev-Actor-Id` header bypass（行 102-110），且 `cmd/server/main.go::buildW1aDeps` 仍用 `auth.SimpleHasher{Salt:"tnbiz-dev-salt"}` + `auth.HMACSigner{Secret:[]byte("tnbiz-dev-jwt-secret")}` 构建生产二进制（行 135-138）。**这是硬编码弱密钥 + 开发后门同时落到 main 入口**，必须在 Phase 1 Week 1 强制 fail-loud。
5. `internal/infra/kms/kms.go::Stub` 在 `Encrypt` 时直接返回明文 + 固定 keyID `"stub:scope:v0"`（行 52-55），没有 env 校验断言把 stub 拒在 staging/prod 外（注释里说"严禁在 staging/prod 启用"但**没有代码强制**）。配合 `cmd/server/main.go::buildW1aDeps` 用 `kyc.NewStubCrypto()` / `partner.NewStubCrypto()` 直接装入生产 wiring——**意味着 PII 加密链全部是 no-op**。

**结论**：Round-2 的 7 项强约束在文档 / DDL / golangci 配置层确实写明了，但**实际代码 W1a 阶段只交付了 service 层与 force-resolve 决策纯函数**，鉴权 / CSRF / BOLA scope / idempotency / audit / scrubber 这一整套中间件、KMS / OSS / Redis 这一整套 infra、handler 层的 dev-bypass 删除——全部 deferred 到 W1c。

Fy-api OVERLAY 这一侧表现明显较好：`middleware/internal_auth.go` 是一份合格的 HMAC 校验实现（timestamp 5min 窗口 + nonce SETNX 24h fail-closed + endpoint 精确匹配 + body limit 1MB + constant-time compare），`model/internal_api_key.go` 用 AES-GCM 加密落库；但发现两处实质性问题：**KEK 派生只用 `common.CryptoSecret` 单根 + 固定字符串 `"tnbiz/internal-api/v1"` 派生**（行 102-108），既没有 KEK 轮换路径，也没有从 KMS Secret Manager 注入；以及 OVERLAY HANDOFF §3 自陈"per-kid quota（占位，Phase 2A 接 KMS）"——**Round-2 强约束 #2（fail-closed）+ ADR-016 SBOM / cosign（CRITICAL-6）部分签名步骤还是 ops-debt**。

**最终 verdict：NEEDS REWORK**。具体说：

- **partner-api 侧 W1a 必须在 Phase 1 Week 1 内完成 C1～C5 的代码交付**（本轮 5 个 CRITICAL），否则任何 W1b/W1c 业务编码增加的就是受污染基线
- **Fy-api OVERLAY 侧可以 PASS-CONDITIONAL**（H4 + H5 两条 HIGH 在 OVERLAY 文档自陈，但代码未实现，纳入 Phase 1 Week 2 强制门槛）
- 4 项 Phase 1 验收钩中的 #1（BOLA analyzer）+ #3（dual-control e2e）+ #4（BOLA 矩阵 e2e）当前**全部未达成**，Phase 1 mid-gate 必须延期至代码闭合后再评审

---

## 2. 评估方法

- **代码 grep + 静态阅读**：所有 critical 主张以"行号锚点 + 函数体内容"为准
- **OWASP ASVS L2 控制项映射**：A01 / A02 / A03 / A07 / A08 / A09 全部抽查
- **STRIDE 矩阵**：6 类威胁 × 10 关键组件，验证每条威胁至少有一处编码层防御（不是文档里的注释）
- **BOLA 端到端走查**：在 partner-web → partner-api → repository / Fy-api → repository 两条链上各抽 ≥ 5 个跨租户 endpoint
- **PII 流转矩阵**：9 字段 × 6 链路（前端展示 / 网络 / 后端入参 / 加密落库 / 日志 / 审计）
- **不接受"有 TODO 注释 = 已 FIXED"**：任何写"W1a 实现"且函数体只是 `c.Next()` 的，记 partial fix（H 级别），影响进 Phase 1 mid-gate

---

## 3. 7 项强约束遵守审计（逐条核实）

### 3.1 强约束 #1：JWT 全系 httpOnly cookie + double-submit CSRF；Bearer 仅 `/api/sdk/*`

| 维度 | 结果 | 锚点 |
|---|---|---|
| 服务端 cookie 名一致 | ✅ | `internal/middleware/auth.go:10-16` 定义 `tnbiz_access` / `tnbiz_refresh` / `tnbiz_csrf`；`handler/w1a_auth.go:152-160` `setAuthCookies` 写三 cookie |
| Bearer 限定 `/api/sdk/*` | ⚠️ PARTIAL | `internal/handler/routes.go:30-32` 仅有 SDK group 占位；但 **JWT middleware 函数体未实现 cookie 提取与 SDK fallback Bearer 分支**（`auth.go:48-54` 只是 `c.Next()`），意味着当前所有 endpoint 实际无鉴权，更谈不上拒绝 user-facing Bearer。**必须在 Phase 1 Week 1 落地**。 |
| 浏览器 client 不发 Bearer | ✅ | `apps/partner-web-{partner,customer,admin}/src/api/client.ts` 均使用 `withCredentials:true`，无 Authorization 头注入；`apps/partner-web-storefront/src/api/client.ts:38-52` 同样仅注入 `X-Csrf-Token` + `Idempotency-Key` + `X-Oneapi-Request-Id` |
| Cookie 标志位 `Secure` / `SameSite` | ❌ HIGH | `handler/w1a_auth.go:151-154`：`SetSameSite(http.SameSiteLaxMode)` + `c.SetCookie(..., "/", "", false, true)` —— **第 6 个参数 `secure=false` 硬编码！** 这是生产二进制里浏览器 cookie 走明文 HTTP 传输的开关。**必须改成由 cfg.Env="prod" 驱动 secure=true**。 |
| CSRF middleware | ❌ CRITICAL | `internal/middleware/csrf.go:20-30` 函数体内对 mutation 请求只有 `// TODO(W1a)` 注释，**直接 `c.Next()`**。Origin / Referer 校验缺失，cookie 与 header 比较缺失。CSRF middleware 是空壳。 |
| Bearer 路径白名单 grep | ⚠️ | 全代码库 grep `"Authorization"` 仅 3 处命中，全部是注释或 SDK 路由组占位（`routes.go:30` / `auth.go:44/50`）；**生产代码无 user-facing Bearer 接收**，但反向"拒收"逻辑同样不存在 |

**verdict：⚠️ PARTIAL → CRITICAL 级别 gap**（中间件本身未实现，cookie 标志位硬编码 false）。强约束 #1 不算遵守。

### 3.2 强约束 #2：JWT revocation fail-closed

| 维度 | 结果 | 锚点 |
|---|---|---|
| service 层 fail-closed | ✅ | `internal/service/auth/auth.go:301-317::VerifyAccessToken`：`s.revoke.IsRevoked(ctx, cl.Jti)` 返错时直接 `return Claims{}, ErrRevocationDown`（不放行）；`Refresh` 的 234-236 行同样在 revocation 错误时返 `ErrRevocationDown` |
| middleware 层 wiring | ❌ CRITICAL | `internal/middleware/auth.go::JWT()` 是空壳；service 层 fail-closed 没有挂到任何路由上。生产二进制根本不会走到 `VerifyAccessToken`。 |
| Fy-api 内部 nonce | ✅ | `Fy-api/middleware/internal_auth.go:95-106`：Redis 不可达 → `return errors.New("nonce store unavailable")` 拒绝；这是 fail-closed 的好例子 |

**verdict：service 层 ✅，middleware 层 ❌（CRITICAL）**。

### 3.3 强约束 #3：JWT 公钥不在 biz_setting，必须 KMS Secret Manager 注入

| 维度 | 结果 | 锚点 |
|---|---|---|
| env 注入路径 | ✅ | `internal/config/config.go:159-163`：`JWT.VerifyKeyPEM = getenv("JWT_VERIFY_KEY_PEM", "")`，env 缺失时为空字符串（启动应当 fail-loud） |
| 启动校验 | ❌ HIGH | `Config.validate()` 行 207-220 没有断言 `cfg.Env=="prod" && JWT.VerifyKeyPEM==""` 即 panic；当前 prod 部署忘配 env 不会被 catch |
| biz_setting 不读公钥 | ✅ | grep `biz_setting.*jwt` 在 cmd / internal 全无命中；服务读取仅 cfg.JWT.VerifyKeyPEM |
| 实际签名器装配 | ❌ CRITICAL | `cmd/server/main.go:135-138`：`signer := auth.HMACSigner{Secret:[]byte("tnbiz-dev-jwt-secret")}` —— **硬编码 16 字节字符串作为 HS256 密钥**，且 `buildW1aDeps` 在所有环境都跑这条路径，没有 env-fence；**意味着如果今天直接 `go build && ./partner-api` 上线，所有 JWT 用同一公开 secret 签**。 |

**verdict：env 路径正确，**但 main 入口装配 stub 签名器进生产二进制 = CRITICAL 级别 fail-loud 缺失**。

### 3.4 强约束 #4：BOLA repository 层 row-level guard + golangci analyzer

| 维度 | 结果 | 锚点 |
|---|---|---|
| repository 接口签名 | ⚠️ PARTIAL | `internal/repository/repository.go:42-49::CustomerRepository.FindByIDForPartner(ctx, partnerID, customerID)` 强制 scope 入参——✅；但 `PartnerRepository.FindByID(ctx, id int64)` / `FindByFyUserID(ctx, fyUserID)` 不带 scope（行 33-36），其它 5 个 Repository 全部是 TODO；`AuditRepository` / `BizSettingRepository` / `StaffRepository` / `IdempotencyRepository` / `SagaRepository` 完全空 |
| MySQL 实现 | ❌ HIGH | `internal/repository/mysql/partner_mysql.go:37-58`：`FindByID(ctx, id int64)` 直接 `WHERE id=?`，**无 partner_id 过滤**；这正是 OWASP API1 BOLA 反例 |
| BOLA scope middleware | ❌ CRITICAL | `internal/middleware/bola_scope.go:17-23` 函数体只有 `c.Next()`；不注入 scope，repository 层根本拿不到 |
| golangci analyzer | ❌ CRITICAL | `apps/partner-api/.golangci.yml:34-37` 只是 TODO 注释 + exclude `internal/repository/` 文本 "bola-scope-required"；**该 analyzer 在 golangci-lint 工具链里不存在**，等于把"未来要写的 lint 规则"提前 exclude 掉，**当前 CI 不会 build fail 任何 BOLA 违规** |
| 备援：service 层人工 scope | ✅ 部分 | `internal/service/customer/customer.go` HANDOFF 描述 `FindByIDForPartner` cross-partner 返 nil；但 wallet / kyc / partner / saga 多处 service 直接 `repo.FindByID(ctx, id)` |

**verdict：⚠️ PARTIAL（CustomerRepository 一处已带 scope；其它 6 个 repo 为空）+ CRITICAL（middleware 空壳 + analyzer 不存在）**。

### 3.5 强约束 #5：OSS presigned PUT 强校验 + 异步病毒扫

| 维度 | 结果 | 锚点 |
|---|---|---|
| 接口契约 | ✅ | `internal/infra/oss/oss.go:27-55::PresignRequest{AllowedMime, MaxBytes, TTL}` + `Service.VerifyMagicBytes` + `EnqueueVirusScan` |
| Stub 校验 | ✅ 部分 | `internal/infra/oss/oss.go:65-82::Stub.PresignPut`：`maxBytes ∈ (0,10MB]` + `ttl ∈ (0,5m]` + `len(allowedMime)>0` 三条；**但是没有断言 allowedMime 是 AllowedMime 子集**（即调用方可以传 `["application/x-msdownload"]` stub 也接受）。Round-2 §15.3 要求 AST scan 调用方字面量 + 真实 SDK 强约束 |
| VerifyMagicBytes 实现 | ❌ HIGH | `oss.go:89-92` 函数体直接 `return nil`，没读 first 8 bytes 做 magic 比对；**KYC 上传无二次校验**（`internal/service/kyc/kyc.go:311-325::verifyUploads` 调的是这个 no-op stub），意味着 attacker 可以上传 `<script>` HTML 命名 `.jpg` 直接通过 |
| 病毒扫 | ❌ HIGH | `oss.go:94-97::EnqueueVirusScan` 同样空函数 |
| 真实 SDK | ❌ | 整个 `infra/oss/` 没有 Aliyun OSS / S3 SDK 接入 |
| 调用方使用 | ⚠️ | `kyc.go:144-172::Submit` 调 `s.oss.VerifyKYCObject(...)` 路径正确，等待 stub 替换 |

**verdict：⚠️ PARTIAL / 强校验逻辑设计完整但实际为 no-op**。Phase 1 必须接 SDK + magic byte + 病毒扫。

### 3.6 强约束 #6：dual-control force-resolve 严格约束

| 维度 | 结果 | 锚点 |
|---|---|---|
| 6 项检查（≠人 / ≠角色 / ≠/24 / 一次性 token / cooldown / TTL） | ⚠️ PARTIAL | 有两套实现并存：(a) `internal/saga/force_resolve.go::ValidateForceResolve` 纯函数（行 58-92）覆盖 ≠人 / ≠ /24 / 30min cooldown / token TTL 30min / target 白名单 / reason 必填——✅；**但没检查 ≠角色**；token 是否消费由调用方决定（`TokenConsumed bool`）。(b) `internal/service/saga_admin/saga_admin.go::ForceResolve`（行 136-173）覆盖 ≠人 / ≠ /24 / cooldown 30min / 一次性 token store 消费 / outcome 白名单——✅；**但 token TTL 5min（saga_admin.go:39）** 与 force_resolve.go 的 30min 不一致 |
| 不同角色检查 | ❌ HIGH | Round-2 强约束 #6 列了"≠角色"（"approver.Role != initiator.Role"），两套实现都没做角色比较；只有"≠人 + ≠ /24"。这与 backend §7.4 行 2417-2428 的设计有差距 |
| 实际接入 | ❌ HIGH | `handler/admin/admin.go:343-376::sagaForceResolve` 调 `saga_admin.Service.ForceResolve(...)`；`handler/saga_admin.go:46-92::NewSagaForceResolveHandler` 调 `saga.ValidateForceResolve(...)`。**两个 handler 同时存在，路径不收敛**——前者由 W1c 接 admin router，后者是 W1c 待挂载 placeholder。Phase 1 必须只保留一套调用链，否则等于"存在一个未做角色检查 + token TTL 不一致的旁路" |
| 双 audit 落库 | ⚠️ PARTIAL | `saga_admin.go::ForceResolve:168-172::audit.WriteForceResolve` 只写一条事件；Round-2 要求 initiator + approver 双 audit + `audit_log_unsealed.second_approver_id` 字段。Audit middleware（`middleware/audit.go`）是空壳——所以即便 service 层写了 sink，也没人调 |
| Cooldown / token store 持久化 | ❌ HIGH | `saga_admin.go::MemoryTokenStore` + `MemoryCooldownStore` 是内存实现；进程重启 = token 全丢、cooldown 全清。生产必须接 Redis 持久化 |

**verdict：⚠️ PARTIAL**（6 项检查有 5 项落地纯函数，但 ≠角色未实现 + 两套并存 + persistence stub）。

### 3.7 强约束 #7：供应链 / SBOM / 镜像签名

| 维度 | 结果 | 锚点 |
|---|---|---|
| govulncheck | ✅ | `.github/workflows/security.yml:22-25`，install + run |
| nancy（SCA） | ⚠️ | 行 26-28：`go list -json -m all \| docker run nancy sleuth \|\| true`——`\|\| true` **结果不会 fail CI**，等于报告而非 gate |
| pnpm audit | ⚠️ | 行 42-43：`pnpm audit --prod \|\| true`——同样不 fail |
| syft SBOM | ✅ | 行 52-58：CycloneDX JSON + artifact upload |
| cosign sign | ❌ HIGH | 行 60：`# TODO(ops): cosign 签名镜像在 release workflow（不在 CI）`——**release 流水线未交付**，镜像未签 |

**verdict：⚠️ PARTIAL**。CI 跑了，但 nancy/pnpm-audit `\|\| true` 等于不 gate；cosign 未交付（与 dev-round 自陈一致 ACCEPT-AS-DEBT，但 Phase 1 Week 1 必须实际能跑出一次签名）。

### 3.8 7 项强约束总览

| # | 强约束 | 设计层 | 代码层 | 距离 PASS 还差 |
|---|---|---|---|---|
| 1 | cookie + CSRF | ✅ | ❌ CRITICAL | 实现 `JWT()` + `CSRF()` middleware；Cookie `Secure` 改为 cfg.Env 驱动 |
| 2 | revocation fail-closed | ✅ | ❌ CRITICAL（middleware 空） | 在 `JWT()` middleware 调 `service.VerifyAccessToken` 并把 ErrRevocationDown 映射 503 |
| 3 | JWT 公钥不在 biz_setting | ✅ | ❌ CRITICAL（main 装 HMAC stub）+ HIGH（缺 prod fail-loud） | 把 stub 路径包到 `cfg.Env=="dev"` 守门；prod / staging 必须 panic-on-empty |
| 4 | BOLA repo guard + analyzer | ⚠️ | ❌ CRITICAL（middleware 空）+ ❌ analyzer 不存在 | analyzer 实装到 build；7 个 Repository 接口加 scope 入参 |
| 5 | OSS PUT 强校验 + 病毒扫 | ✅ | ⚠️ PARTIAL（接口完整但函数体空） | 接 Aliyun SDK + magic byte 实读 + 病毒扫队列 |
| 6 | dual-control 6 项 | ✅ | ⚠️ PARTIAL（缺 ≠角色 + Redis 持久化 + 两套并存） | 收敛单一路径；加角色比较；token store / cooldown 落 Redis |
| 7 | SBOM / 镜像签名 | ✅ | ⚠️ PARTIAL（cosign 待 release workflow） | release 工作流落地 cosign sign + SBOM attest |

---

## 4. STRIDE 矩阵（10 组件 × 6 字段）

> 仅列出代码层可观察到的状态；F=fail（CRITICAL/HIGH） / P=partial / O=ok。

| 组件 | Spoofing | Tampering | Repudiation | InfoDis | DoS | ElevPriv |
|---|---|---|---|---|---|---|
| partner-api 中间件链 | F（JWT 空壳）| F（CSRF 空壳）| F（Audit middleware 空）| P（PII scrubber 实装但未挂）| P（无全局 rate-limit 中间件，只有 cfg.HTTPBodyLimit）| F（BOLA scope 空 + dev header bypass）|
| partner-api auth.Service | O（cookie + revoke 设计）| O（refresh rotation）| P（仅 service 层日志，未串到 audit）| P（错误信息差异化，恒等响应已落 handler）| O | O |
| partner-api KYC | O | P（VerifyMagicBytes no-op）| P | F（CryptoPort stub 不加密）| O | O |
| partner-api Wallet | O | P | F（无 audit middleware）| O | P（无 rate-limit）| F（repo.FindByPartner 是接口空）|
| partner-api Saga / dual-control | O | P（ValidateForceResolve 缺 ≠角色）| P（audit 单写非双写）| O | O | P（两套并存）|
| Redis 抽象 | O | O | O | F（fail-closed 路径在 service 层但 middleware 不调）| O | O |
| OSS 抽象 | O | F（VerifyMagicBytes / VirusScan 空）| O | O | P（maxBytes ≤10MB + ttl ≤5min 已写）| O |
| KMS 抽象 | F（Stub Encrypt 返 plaintext）| F（DEK 不加密）| O | F（明文落库）| O | O |
| Fy-api InternalAuth | O（HMAC 完整）| O（canonical 6 段）| P（缺 audit 写入）| O | P（缺 per-kid quota）| O |
| Fy-api InternalIdempotency | O | O | P | O | P（无 SLA 限制）| O |

**矩阵审计结论**：partner-api 有 **6 个 F + 6 个 P**；Fy-api 有 0 个 F + 4 个 P。partner-api 的 F 主要集中在 middleware 层（4 处）+ KMS infra（2 处）。任何一个 F 都阻塞 Phase 1 mid-gate。

---

## 5. BOLA 端到端走查（≥ 10 个跨租户 endpoint）

### 5.1 partner-api 路径

| # | endpoint | scope guard 现状 | 评估 |
|---|---|---|---|
| 1 | `GET /partner/wallet`（`handler/w1a_routes.go:62`） | `scopeOf(c)` 取 actor，调 `walletGetHandler`；wallet repository 接口 `FindByPartner(ctx, partnerID)` 强制 scope ✅；**但 actor 来源是 `X-Dev-Actor-Id` header（行 102-110）** | ❌ HIGH（dev bypass） |
| 2 | `GET /partner/wallet/logs`（行 63） | 同上 | ❌ HIGH |
| 3 | `POST /partner/invitation`（行 64） | 同上 + invitation.Service 接口未读 | P |
| 4 | `POST /partner/kyc`（行 66） | 同上 + KYC.Service.Submit 用 `FyUserID` 字段，不直接接受外部 ID | P |
| 5 | `POST /customer/transfer`（行 70） | `customerTransferHandler` 走 `customer.Service.TransferRequest`，scope 来源同 dev header | ❌ HIGH |
| 6 | `POST /customer/erase`（行 71） | 同上 | ❌ HIGH |
| 7 | `POST /admin/partners/:id/approve`（行 74） | actor staff_id 来源同 dev header；目标 ID 直接从 path 取，**未挂 admin/staff RBAC middleware** | ❌ HIGH |
| 8 | `POST /admin/partners/:id/terminate`（行 75） | 同上 + dual-control 未走（强约束 #6 PARTIAL） | ❌ HIGH |
| 9 | `POST /admin/kyc/:id/review`（行 76） | 同上 | ❌ HIGH |
| 10 | `POST /admin/saga/:id/force-resolve`（`admin/admin.go:60`） | dual-control service 调用链有；缺 admin RBAC middleware | ⚠️ |
| 11 | `POST /admin/invoice/:id/red-flush`（`admin/admin.go:49`） | actor 通过 `c.Get("staff_id")` 取（行 166-167），假定 JWT middleware 注入；**但 JWT middleware 是空壳**，所以 actor 可能为 0 / nil 不被拒 | ❌ HIGH |

### 5.2 Fy-api `/api/internal/*` 路径

| # | endpoint | scope guard 现状 | 评估 |
|---|---|---|---|
| 12 | `GET /api/internal/health` | HMAC 通过即放行；无业务侧 BOLA | ✅ |
| 13 | `POST /api/internal/token/create`（`controller/tnbiz_internal/token.go:37`） | 校验 `userExists(req.UserId)`，但**没有断言"该 partner 与 customer 之间存在被邀请关系"**——partner A 的 internal-key 也能给 partner B 的 customer 发 sk-key | ❌ HIGH（跨租户）|
| 14 | `POST /api/internal/user/topup`（`user.go:32`） | 同上：HMAC 鉴权后任何 key 可对任意 user_id 操作 | ❌ HIGH |
| 15 | `POST /api/internal/user/quota/adjust`（行 102） | 同上 | ❌ HIGH |
| 16 | `POST /api/internal/user/refund`（行 150） | 同上 | ❌ HIGH |
| 17 | `POST /api/internal/group_ratio_override/upsert`（`settings.go`） | 同上 | ❌ HIGH |

**关键发现**：`Fy-api/middleware/internal_auth.go` HMAC 鉴权通过后只把 `key_id` 写入 ctx（`ContextKeyInternalKeyId`）；controller 层**完全不做"该 key 是否有权操作该 user_id"** 的二次比对。这意味着如果 partner A 拿到了 partner B 的 internal-key（密钥泄漏 / 误配），A 可以全权操作 B 的客户余额、发 token、调倍率。Phase 1 必须在 controller 层加 `partner_id × user_id` ownership 校验（`internal_api_key` 表加 `owner_partner_id` 字段，每个 user_id 操作前 join 一次 customer.partner_id 比对）。

**BOLA 走查结论**：12 个 endpoint 中 9 个 ❌HIGH；3 个 ⚠️/✅。**Phase 1 验收钩 #1（BOLA analyzer）必须实装**，否则后续 W1b/W1c 增加 endpoint 只会让 BOLA 面爆炸式扩大。

---

## 6. PII 流转矩阵（9 字段 × 6 链路）

| 字段 | 前端展示 mask | 网络 (HTTPS / cookie / CSRF) | 后端入参验证 | 加密落库 | 日志 scrubber | 审计 |
|---|---|---|---|---|---|---|
| 邮箱 | ✅（前端 ZodResolver） | ✅ | ✅（zod / binding） | ❌（明文 + HMAC blind index 缺，dev-round LOW-r2-2） | ✅（`pkg/piiscrubber/scrubber.go:19` reEmail） | ⚠️（audit middleware 空） |
| 手机号 | ✅ | ✅ | ✅ | ⚠️（contact_phone_cipher 字段在 domain 中，但 KMS Stub 不加密） | ✅（rePhone） | ⚠️ |
| 身份证号 | ✅ | ✅ | ✅ | ⚠️（KYC.Service.encryptInto 调 CryptoPort.Encrypt；Stub 返明文）| ✅（reIDCard） | ⚠️ |
| 法人姓名 | ✅ | ✅ | ✅ | ⚠️（同上） | ⚠️（无专用正则）| ⚠️ |
| 银行卡号 | ✅ | ✅ | ✅ | ⚠️（同上） | ❌（无银行卡正则）| ⚠️ |
| 身份证图片 URL | ✅ | ✅ | ⚠️（OSS magic-byte no-op） | n/a OSS | ⚠️ | ⚠️ |
| 营业执照图片 URL | ✅ | ✅ | ⚠️（同上） | n/a | ⚠️ | ⚠️ |
| 人脸 liveness URL | ✅ | ✅ | ⚠️ | n/a | ⚠️ | ⚠️ |
| 持牌方 callback | n/a | ✅ | ✅ | ⚠️（设计 callback_payload_cipher Phase 2A）| ⚠️ | ⚠️ |

**PII 矩阵结论**：9/9 字段在前端展示与网络层符合 ASVS V8/V9；后端"加密落库"全部依赖 `kms.Stub` 这个 no-op，等于**全部明文**——这是 Phase 1 Week 2 必修。`pkg/piiscrubber/scrubber.go` 实装了 3 条 baseline 正则（手机/身份证/邮箱），但**没挂到 zerolog hook**（dev-round MED-3 自陈），日志层 scrubber 就是死代码。

---

## 7. CRITICAL / HIGH / MEDIUM / LOW

> 编号采用 `C{n}` / `H{n}` / `M{n}` / `L{n}` 体系；引用具体文件 + 行号。

### 7.1 CRITICAL（5 条；阻塞 Phase 1 Week 2/3）

| ID | 名称 | 文件 / 行 | 描述 | 修复指令 |
|---|---|---|---|---|
| **C1** | JWT middleware 空壳，所有 endpoint 实际未鉴权 | `internal/middleware/auth.go:48-54` | 函数体只有 `c.Next()`，强约束 #1/#2 在中间件层失效 | Phase 1 Week 1：实装 cookie 提取 → service.VerifyAccessToken → fail-closed 503 → c.Set("jwt_claims") |
| **C2** | CSRF middleware 空壳 | `internal/middleware/csrf.go:21-30` | 写方法不做 token 比较 + Origin/Referer 校验 | 实装 constant-time double-submit + Origin allowlist；webhook 路径单独 group 跳过 |
| **C3** | dev 后门 actor header 入主二进制 | `handler/w1a_routes.go:96-110`、`cmd/server/main.go:135-138` | `X-Dev-Actor-Type/Id` header 让任何请求伪装成 partner-1 / staff-99；同时 `auth.SimpleHasher` + `auth.HMACSigner{Secret:"tnbiz-dev-jwt-secret"}` 是默认装配 | `cfg.Env=="dev"` 才挂 dev header 路径；prod / staging 装配 RSASigner + Argon2idHasher，env 缺失 panic-on-startup |
| **C4** | KMS Stub 装入 prod wiring → PII 全部明文 | `internal/infra/kms/kms.go:52-55`、`cmd/server/main.go:158-165`（kyc.NewStubCrypto / partner.NewStubCrypto） | `Encrypt` 直接 return plaintext；现实部署 = 身份证、银行卡、法人姓名全明文落库 | 接 Aliyun KMS SDK；`cfg.Env != "dev" && KMS.Endpoint == ""` 必须 panic |
| **C5** | BOLA scope middleware 空壳 + golangci analyzer 不存在 | `internal/middleware/bola_scope.go:17-23`、`apps/partner-api/.golangci.yml:34-37` | analyzer 还没写 + middleware 不注入 scope；CI 不会拦截任何 BOLA 违规；现有 `partner_mysql.go::FindByID(ctx, id)` 已是反例 | 写 golangci 自定义 analyzer（go/analysis）扫 `repo.Find*/Update*/Delete*` 公共方法，第 2 参必须为 `ActorContext` / `partnerID/customerID`；middleware 从 jwt_claims 派生 scope |

### 7.2 HIGH（9 条）

| ID | 名称 | 文件 / 行 | 描述 |
|---|---|---|---|
| **H1** | Cookie `Secure=false` 硬编码 | `handler/w1a_auth.go:152-154` | prod 走 HTTPS 时 cookie 标志位不带 Secure，BCP38 中间网络可截取 |
| **H2** | OSS VerifyMagicBytes / EnqueueVirusScan no-op | `internal/infra/oss/oss.go:89-97` | KYC 上传放行任意文件类型；强约束 #5 等待落地 |
| **H3** | dual-control 缺 ≠角色检查 + 两套并存 + 内存 token store | `internal/saga/force_resolve.go:58-92`、`internal/service/saga_admin/saga_admin.go:136-173`、`MemoryTokenStore`/`MemoryCooldownStore` | 强约束 #6 partial |
| **H4** | Fy-api `/api/internal/*` 缺 owner-partner ownership 校验 | `Fy-api/controller/tnbiz_internal/{token,user}.go` | HMAC 通过后任何 key 可操作任何 user_id；BOLA 跨租户 |
| **H5** | Fy-api KEK 派生单根 + 缺轮换 | `Fy-api/model/internal_api_key.go:102-108` | `deriveKEK = sha256(CryptoSecret \|\| "tnbiz/internal-api/v1")`；CRYPTO_SECRET 默认回退到 SESSION_SECRET（`Fy-api/common/init.go:59-63`）；无 KEK rotation 路径 |
| **H6** | nancy / pnpm audit `\|\| true` 不 gate CI | `.github/workflows/security.yml:28,43` | 强约束 #7：CI 报告但不失败 |
| **H7** | rate-limit 中间件未实装 | `internal/middleware/` 全目录无 ratelimit.go | dev-round HIGH-2 设计 6 条限速；代码层只挂 BodyLimit |
| **H8** | Audit middleware 空壳 → dual-control / KYC 等高敏 verb 无审计 | `internal/middleware/audit.go:23-29` | sealer / verify CLI / 哈希链都建好了，但入队点缺失 |
| **H9** | PII scrubber 未挂 zerolog hook | `pkg/piiscrubber/scrubber.go` 仅 30 行；`cmd/server/main.go` 未注册 hook | dev-round MED-3 残余 |

### 7.3 MEDIUM（11 条）

| ID | 名称 | 文件 / 行 | 描述 |
|---|---|---|---|
| **M1** | Stub OSS 不校验 AllowedMime 子集 | `infra/oss/oss.go:65-82` | 调用方可传任意 MIME |
| **M2** | webhook idempotency Redis fail-OPEN 文档与代码注释一致，但缺 alert 路径 | `middleware/webhook_idempotency.go:13-26` | dev-round 已 ACCEPT，代码尚未接 SLS alert |
| **M3** | `idempotency` middleware 不调 repo.Insert，但 service 层 wiring 未呈现 | `internal/middleware/idempotency.go:50-56` + `internal/idempotency/idempotency.go` | 业务 TX 闭包内 Insert 路径仍 TODO |
| **M4** | session 表 migration 0006 未交付 | `apps/partner-api/migrations/` | HANDOFF-W1a §2.1 已声明，未实现 |
| **M5** | `Encrypted.Reveal` 不存在；`pkg/pii/encrypted.go` 没有 zero-after-use | `pkg/pii/encrypted.go:30-43` | dev-round MED-r2-4 残余 |
| **M6** | `consume_log_outbox.last_error` PII 风险 | `Fy-api/model/log_outbox.go` 等 | dev-round MED-r2-2 残余 |
| **M7** | Fy-api `internal_idempotency.response_body` 明文落 TEXT | `Fy-api/model/internal_idempotency.go:25-27` | 即便不含强 PII，response 可能含 user_id / quota，建议 Phase 2A KMS envelope |
| **M8** | `parseInt64` 自实现 + 不接受负数 / overflow | `handler/saga_admin.go:146-158` | 用 strconv.ParseInt 更安全 |
| **M9** | `handler/w1a_routes.go::fmtSscan` 自实现 atoi | 同上：避免引入 strconv —— 但负号 / overflow 不处理 | 同上 |
| **M10** | CSP 仍是占位（无 nonce / strict-dynamic） | `middleware/headers.go:18-21` | dev-round MED-10 自陈"W1a 收紧"，目前 connect-src 'self' 不含 OSS 域 |
| **M11** | password reset OTP 弱熵（SHA256 hex truncate） | `service/auth/auth.go:345-356::generateResetMaterial` | OTP 用 4 字节 BE int 取前 6 位字符，分布合规但 entropy 与字面 6 位 BCD 一致 |

### 7.4 LOW（6 条）

| ID | 名称 | 锚点 |
|---|---|---|
| **L1** | `splitCSV` 自实现 | `config/config.go:266-286`，`strings.Split` 即可 |
| **L2** | dev cookie `domain=""` 跨子域不工作 | `handler/w1a_auth.go:152-154`，prod 要 `*.tracenex.cn` |
| **L3** | `verifyTOTP` 占位仅识别 `test:` 前缀 | `service/auth/auth.go:377-386` |
| **L4** | csrfHandler 注释里说 `webhook/*` 跳过但 middleware 函数未实现该跳过 | `middleware/csrf.go:19` |
| **L5** | `same24` IPv6 fallback 返 false（应严格 same /48） | `service/saga_admin/saga_admin.go:184-187` |
| **L6** | golangci-lint `go: "1.22"` 与 module `go 1.25.1` 不一致 | `.golangci.yml:6` vs 项目 |

---

## 8. §22.2 八项 Security gates 最终表

| # | 验收项 | Dev R-2 | Code R-1 实测 | gap |
|---|---|---|---|---|
| **S-1** | audit_log 哈希链一致性 + verify CLI | ✅ | ✅（`internal/audit/sealer.go` 完整 + sealer_test.go 通过；MemoryStore + verify 可跑） | — |
| **S-2** | saga 卡死 dual-control 解锁 | ✅ | ⚠️ 缺 ≠角色 + 两套并存 + 内存 store | H3 |
| **S-3** | partner KYC pass 强制 MFA / WebAuthn 阈值 | ✅ | ❌ middleware 空壳 + WebAuthn 未实装 | C1 |
| **S-4** | Staff Elevated step-up MFA | ✅ | ❌ 同 C1 | C1 |
| **S-5** | outbox SKIP LOCKED / 单 leader | ✅ | ⚠️ Fy-api OVERLAY `service/outbox/runner.go` 已有；partner-api outbox 包仅占位 | M3 |
| **S-6** | CI BOLA 矩阵测试 wired | ✅ | ❌ analyzer 不存在；e2e 矩阵未生成 | C5 |
| **S-7** | app DB user 无 audit_log UPDATE/DELETE | ✅ | ⚠️ migration 未交付，无 GRANT 金文件 | M4 |
| **S-8** | `Encrypted*` 字段 `json:"-"` | ✅ | ⚠️ `pkg/pii/encrypted.go` 没有 json 序列化保护（无 `MarshalJSON`），domain 多处直接暴露 `KeyID string` 字段 | M5 |

**通过率：1/8 已落地（S-1）；6/8 PARTIAL；1/8 fail（S-6）**。Phase 1 mid-gate 不能开。

---

## 9. 修订指令（Phase 1 W1c / W1b 必交付）

### 9.1 阻塞类（必须在 Phase 1 Week 1 内闭合，否则不允许并入 main）

1. **C1**：实装 `middleware.JWT()` —— 从 cookie `tnbiz_access` 取 token；`/api/sdk/*` 路径 fallback `Authorization: Bearer`；调 `auth.Service.VerifyAccessToken`；`ErrRevocationDown` 映射 503；正常 case `c.Set("jwt_claims", claims)`；e2e 验证两条路径。
2. **C2**：实装 `middleware.CSRF()` —— 对 POST/PUT/DELETE/PATCH，校验 `Origin`/`Referer` 在 `cfg.AllowedOrigins`；`subtle.ConstantTimeCompare(cookie, header)`；`/webhook/*` 跳过；e2e 验证 missing / mismatch / origin 三类。
3. **C3**：`cmd/server/main.go::buildW1aDeps` 增加 env-fence——`if cfg.Env != "dev"`，禁止用 `auth.SimpleHasher` / `auth.HMACSigner{Secret:"tnbiz-dev-jwt-secret"}` / `kms.NewStub` / `oss.NewStub` / `auth.NewMemoryRepo` 等 stub；缺真实 wiring → `log.Fatal`。`handler/w1a_routes.go::scopeOf` 把 `X-Dev-Actor-*` 路径包到 `cfg.Env=="dev"`，prod 二进制 grep 不到该字面量。
4. **C4**：接 Aliyun KMS SDK；`kms.Service.Encrypt` 真正调 `KMS.Encrypt(scope→keyArn, plaintext)`；DEK cache 走 (scope, key_id) 二元组；`Encrypted.SetCipher` 之后 `e.plain = ""`（已实现 `pkg/pii/encrypted.go:42`）+ `runtime.GC()`（dev-round MED-r2-4 升级为 `subtle.ConstantTimeCopy([]byte{0,...})`）。
5. **C5**：写 golangci 自定义 analyzer（包名 `bolascope`，注册到 `golangci-lint custom plugin` 体系）：扫 `internal/repository/**/*.go`，对每个 `func (r *XxxRepository) (Find\|Update\|Delete\|List)*(ctx context.Context, ...)` 签名断言第 2 参类型 ∈ {`int64 with name partnerID/customerID/staffID`, `*ActorContext`, `ActorContext`}；CI fail-on-miss。`middleware.BOLAScope()` 从 `c.MustGet("jwt_claims")` 派生 scope。

### 9.2 高优先（Phase 1 Week 2）

6. **H1**：cookie `Secure` 由 `cfg.Env != "dev"` 驱动；`SameSite=Strict` 候选评估（partner / customer / admin 子域是否需要跨子域 OAuth）。
7. **H2**：OSS 真实 SDK + magic byte（HEAD object → 读 first 16 bytes → match table `image/jpeg=FFD8FF / image/png=89504E47 / application/pdf=25504446 / video/mp4=...ftyp`）+ ClamAV / 阿里云内容安全文件扫 webhook。
8. **H3**：dual-control 单一路径——保留 `service/saga_admin.Service.ForceResolve`（含一次性 token store + cooldown），删除 `internal/saga/force_resolve.go::ValidateForceResolve` 或将其变为内部 helper；新增 ≠角色检查（`approver.Role != initiator.Role`）；`MemoryTokenStore` → Redis SETEX with single-shot DEL on Consume（fail-closed）；`MemoryCooldownStore` → Redis `cooldown:saga:{id}` SETEX 30min。
9. **H4**：Fy-api `internal_api_key` 表加 `owner_partner_id BIGINT NOT NULL DEFAULT 0`；`controller/tnbiz_internal/user.go::Topup/Adjust/Refund` 入口加 `verifyOwnership(authKid, req.UserId)`：查 `Token` 找到 user → user.partner_id == key.owner_partner_id 否则 404（不暴露存在性）。
10. **H5**：Fy-api `deriveKEK` 改为从 KMS Secret Manager / env `TNBIZ_INTERNAL_KEK` 注入（不读 `common.CryptoSecret`）；`internal_api_key` 表加 `kek_version` 字段，rotation cron 重写所有 cipher（双写）。
11. **H6**：security workflow 删除 `\|\| true`；govulncheck 已是 fail-on-miss；nancy / pnpm audit 走 `--threshold=high` 退出码 1；CRITICAL 漏洞 block 合并。
12. **H7**：实装 `middleware.RateLimit(rules)`（Redis token bucket）；6 条规则按 dev-round backend §7.8 落地。
13. **H8**：`middleware.Audit()` 在 c.Next 后判 `c.Writer.Status() ∈ 2xx && c.Request.Method ∈ {POST,PUT,DELETE,PATCH}`，从 `c.Get("jwt_claims")` 取 actor → `audit.UnsealedRow` enqueue；diff_redacted 经 piiscrubber.Redact 处理。
14. **H9**：`cmd/server/main.go` 注册 zerolog hook：`log.Logger = log.Logger.Hook(piiscrubber.LogHook{})`；保证所有 `log.Info().Str("phone", ...)` 经 hook 重写。

### 9.3 中等 / 低优先（Phase 1 Week 3 / 4）

- M1～M11 / L1～L6 详见 §7。

### 9.4 Fy-api OVERLAY 侧

- 现状基本可 PASS-CONDITIONAL；H4 + H5 必须在 Phase 1 Week 2 内闭合；其余 M2 / M6 / M7 进 Phase 2A。
- **额外建议**：`controller/tnbiz_internal/token.go::stashPlaintextKey` 使用 `common.GetRandomString(48)`——确认该函数走 `crypto/rand`（`Fy-api/common/*` 应有，但本轮未贴出代码）；若是 `math/rand` 立即升 CRITICAL。

### 9.5 Phase 1 验收钩状态（dev round §9.4）

| 钩 | 状态 |
|---|---|
| #1 BOLA analyzer wired | ❌ 未达成（C5） |
| #2 govulncheck/nancy/pnpm-audit/cosign/syft 跑过一次 | ⚠️ 跑了但 nancy/pnpm-audit `\|\| true`（H6）+ cosign 缺（强约束 #7 自陈） |
| #3 dual-control 6 失败路径 e2e | ❌ 未达成（H3） |
| #4 BOLA 矩阵 e2e ≥30 + scope_mismatch metric | ❌ 未达成（C5） |

**Phase 1 mid-gate 必须在 C1～C5 全部闭合 + H1～H9 进度 ≥ 70% 后再次 review**；建议 Security agent 在 Phase 1 Week 1 末做一次代码层 mini-review，cherry-pick C1～C5 合并 PR 验收。

---

## 10. 附录：跨文件 grep 证据

```text
# 1. Bearer 检查（仅 SDK + 注释）
internal/handler/routes.go:30:    // W1a: SDK / server-to-server（Bearer fallback，backend §7.2）
internal/middleware/auth.go:44:   //   - 从 cookie tnbiz_access 取 token；/api/sdk/* 路径 fallback Bearer
internal/middleware/auth.go:50:   // TODO(W1a): per backend §7.2 — extract cookie/Bearer

# 2. Stub / dev wiring
cmd/server/main.go:135-138:       hasher := auth.SimpleHasher{Salt: "tnbiz-dev-salt"}
                                  signer := auth.HMACSigner{Secret: []byte("tnbiz-dev-jwt-secret")}
cmd/server/main.go:158-165:       kyc.NewStubCrypto(), kyc.NewStubOSS()...

# 3. dev header bypass
internal/handler/w1a_routes.go:102-110:
        if v := c.GetHeader("X-Dev-Actor-Type"); v != "" { ... }

# 4. cookie secure=false 硬编码
internal/handler/w1a_auth.go:152: c.SetCookie(middleware.CookieAccess, out.AccessToken, maxAge, "/", "", false, true)
                                                                                                          ^^^^^ Secure

# 5. golangci analyzer occluded
.golangci.yml:34-37:    # TODO(W1a): 启用 bola-scope-required 自定义 analyzer
                        - path: internal/repository/
                          text: "bola-scope-required"

# 6. KMS Stub 直返明文
internal/infra/kms/kms.go:52-55:
        return plaintext, "stub:" + scope + ":v0", nil

# 7. CI \|\| true
.github/workflows/security.yml:28: ... \| docker run ... nancy:latest sleuth || true
.github/workflows/security.yml:43: pnpm audit --prod || true
```

---

## 11. 最终意见

四份 v0.2.2 设计文档在 Round-2 PASS-CONDITIONAL 是合理的，但**Code Round-1 暴露的差距说明：W1a 这一波只交付了 service 层 + scaffold，安全控制项大量积压在 W1c 的 middleware/infra wiring 任务里**。在这种状态下，任何 W1b 业务编码 / W1e/f/g 前端联调都会基于一份**鉴权与 BOLA 控制全部 stub** 的基线，等 W1c 来补时已经有几十处 endpoint 假定 actor 上下文存在 / scope 正确，retrofit 成本远高于 Round-2 的预算。

**结论：本轮 Security verdict = NEEDS REWORK**。

接受路径：

1. W1c agent 立即起一个安全紧急 PR，实装 C1～C5（≤ 1 周）
2. W1b agent 在该 PR 合并前不允许在 `internal/service/` 下新增任何 mutation endpoint
3. Security agent 在 PR 通过后做一次 30 分钟 mini-review（cherry-pick 比较），确认 main 分支零 dev-bypass、零 hardcoded JWT secret、零 kms stub
4. 4 项 Phase 1 验收钩在 Week 1 末再考核一次；不达成则 Phase 1 mid-gate 推迟一周

> Round-1 7 项强约束**设计层 100% 遵守**，**代码层只有 1 项（#7）部分到位**；W1a 必须把 middleware / infra wiring 这一块作为 Phase 1 第一优先级。Fy-api OVERLAY 可作为参照样板（`internal_auth.go` 是合格实现）。

— Security agent，Code Round-1 final
