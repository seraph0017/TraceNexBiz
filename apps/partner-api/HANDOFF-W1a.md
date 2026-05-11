# HANDOFF-W1a — partner-api 模块组 A（auth / kyc / wallet / partner / customer / invitation）

**日期**：2026-05-11
**承接**：W1b（计费/saga/结算）/ W1c（支付/发票/工单/admin/审计）/ W1e（storefront）/ W1f（customer web）/ W1g（partner+admin web）
**前置**：HANDOFF-W0.md + backend §3/§5/§7/§8/§9 + integration §4

> **本文档作用**：W1a 已经实现了什么，下游 5 个 agent 应该怎样调用 / 替换 stub / 接续。
> 任何不在本文档列出的 W1a 行为都视为占位，请 W1c / W1b 跟进时通过 HANDOFF 增量条目同步。

---

## 1. W1a 已交付

### 1.1 service 包（含内存 repo + memory test）

| 包 | 路径 | 主要类型 |
|---|---|---|
| auth | `internal/service/auth/` | `Service` / `Claims` / `Credentials` / `Session` / `PasswordResetToken` / `Repository` / `RevocationStore` / `PasswordHasher` / `TokenSigner` / `Notifier` / `ConsentRepo` |
| partner | `internal/service/partner/` | `Service` / `ApplyInput` / `Repository` / `CryptoPort` / `ConsentPort` / `InvitationGenerator` / `CustomerOrphaner` |
| customer | `internal/service/customer/` | `Service` / `RegisterInput` / `TransferRequestInput` / `EraseInput` / `Repository` / `InvitationResolver` / `FyAPIPort` |
| kyc | `internal/service/kyc/` | `Service` / `SubmitInput` / `ApprovalInput` / `Repository` / `CryptoPort` / `OCRPort` / `OSSPort` / `ConsentPort` / `PartnerLinker` |
| wallet | `internal/service/wallet/` | `Service` / `Snapshot` / `LogFilter` / `Repository` / `AllocateExecutor`（含 `AllocateInput`/`RefundInput`/`TopupInput` 等 saga DTO 占位） |
| invitation | `internal/service/invitation/` | `Service` / `GenerateInput` / `Repository` |

### 1.2 HTTP routes（`internal/handler/w1a_*.go`）

```
POST   /public/auth/login
POST   /public/auth/logout
POST   /public/auth/refresh
POST   /public/auth/password/forgot
POST   /public/auth/password/reset
POST   /public/partner/apply
POST   /public/customer/register
GET    /partner/me
GET    /partner/wallet
GET    /partner/wallet/logs
POST   /partner/invitation
GET    /partner/invitation
POST   /partner/kyc
POST   /customer/kyc
POST   /customer/transfer
POST   /customer/erase
POST   /admin/partners/:id/approve
POST   /admin/partners/:id/terminate
POST   /admin/kyc/:id/review
```

cmd/server `buildW1aDeps()` 用全内存 repo + stub crypto/fyapi/notify 装配；W1c 接 GORM + JWT middleware 后替换。

### 1.3 测试

`go test -race ./internal/service/auth/... ./internal/service/kyc/... ./internal/service/wallet/... ./internal/service/partner/... ./internal/service/customer/... ./internal/service/invitation/...` 全绿。
`go vet ./...` 通过；`go build ./...` 通过；二进制 ~15MB。

curl 联调（dev 模式无 DB 也可跑）：见本文档 §6。

---

## 2. 给 W1c（鉴权/admin/审计）的契约

### 2.1 auth.Repository

W1c 必须在 `internal/repository/mysql/auth_mysql.go` 落地下面的接口实现，并替换 `cmd/server/main.go::buildW1aDeps` 中的 `auth.NewMemoryRepo()`。

```go
type Repository interface {
    FindCredentials(ctx, actor ActorType, handle string) (Credentials, error)
    FindCredentialsAny(ctx, handle string) (Credentials, error)
    IncFailedAttempts(ctx, actor ActorType, actorID int64) error
    ResetFailedAttempts(ctx, actor ActorType, actorID int64) error
    RecordLastLogin(ctx, actor ActorType, actorID int64, at time.Time) error

    CreateSession(ctx, s Session) (int64, error)
    ListActiveJTIs(ctx, actor ActorType, actorID int64) ([]string, error)
    CloseAllSessions(ctx, actor ActorType, actorID int64, at time.Time) error

    InsertResetToken(ctx, t PasswordResetToken) error
    FindResetTokenByHash(ctx, hash string) (PasswordResetToken, error)
    IncResetFailedAttempts(ctx, id int64, at time.Time) error
    ApplyPasswordReset(ctx, t PasswordResetToken, newHash string, at time.Time) error
}
```

DDL 提示：W1a 用了 `password_reset_token`（DDL 已有 §3.28），但 **session 表 W0 未建**；W1c 增 migration `0006_session.up.sql`：

```sql
CREATE TABLE session (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    actor_type VARCHAR(16) NOT NULL,
    actor_id BIGINT NOT NULL,
    access_jti CHAR(32) NOT NULL,
    refresh_jti CHAR(32) NOT NULL,
    device_fingerprint VARCHAR(255),
    ip VARCHAR(45),
    user_agent VARCHAR(512),
    issued_at TIMESTAMP(3) NOT NULL,
    expires_at TIMESTAMP(3) NOT NULL,
    closed_at TIMESTAMP(3) NULL,
    KEY idx_session_actor (actor_type, actor_id, closed_at),
    UNIQUE KEY uk_session_access_jti (access_jti),
    UNIQUE KEY uk_session_refresh_jti (refresh_jti)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 2.2 RevocationStore

W1c 接 `infra/redis.Client` → `redis.Exists("revoked:jti:"+jti)`，**fail-closed**（错误 → 503），与 backend §7.2 SEC CRIT-3 对齐。

### 2.3 PasswordHasher / TokenSigner

W1c 替换 `auth.SimpleHasher` → `argon2id`（time=3, memory=64MB, parallelism=2）；
`auth.HMACSigner` → `RS256` + 公钥从 env `JWT_VERIFY_KEY_PEM`（KMS Secret Manager 注入）。

### 2.4 admin handler

W1c 在 `/admin/*` 路由组挂 RBAC + step-up middleware：
- `POST /admin/partners/:id/approve` → permission verb `partner.approve`
- `POST /admin/partners/:id/terminate` → `partner.terminate`（Compliance：dual-control）
- `POST /admin/kyc/:id/review` → `kyc.approve` / `kyc.reject`（kyc_reviewer 角色）

---

## 3. 给 W1b（计费/saga/结算）的契约

### 3.1 wallet.Service / wallet.AllocateExecutor

`wallet.Service` 是 **read-only**；W1a 不实现 saga。W1b 实现 `wallet.AllocateExecutor` 接口（已锁定签名，handler 不需改）：

```go
type AllocateExecutor interface {
    Allocate(ctx, in AllocateInput) (AllocateOutput, error)
    Refund(ctx, in RefundInput) (RefundOutput, error)
    Topup(ctx, in TopupInput) (TopupOutput, error)
}
```

DTO 字段已固化（`PartnerID` / `CustomerID` / `Amount` / `IdempotencyKey` / `OperatorID` / `OperatorType` / `TraceID`）。W1b 加 saga 流程时不要新增必填字段；可选字段以 `Note string` / `Source string` 这种向后兼容方式扩展。

W1b 还需要替换 `cmd/server/main.go::buildW1aDeps` 里 `wallet.NewService(walletRepo)` 后接 `wallet.NewSagaService(repo, allocator)` 类似的复合 wrapper（具体 wrapper 接口由 W1b 自己定，不要污染 W1a 的 wallet.Service）。

### 3.2 wallet.Repository

```go
type Repository interface {
    FindWallet(ctx, partnerID int64) (*PartnerWallet, error)
    SumHeldByPartner(ctx, partnerID int64) (sum int64, count int, err error)
    ListLogs(ctx, partnerID int64, filter LogFilter) ([]PartnerWalletLog, error)
    ListHolds(ctx, partnerID int64) ([]WalletHold, error)
}
```

W1b 在落 GORM 实现时注意 `SumHeldByPartner` 必须用 `wallet_hold WHERE partner_id=? AND status='held'` —— I-W-7 invariant。

---

## 4. 给 W1f / W1g（前端）的 OpenAPI 契约（节选）

完整 OpenAPI 由 W1c 生成 `openapi/internal-api.yaml`（W0 §6.8 已登记）；W1a 范围 endpoints 字段约定如下。错误 envelope 与 backend §11 一致 `{success, data, error{code, message_zh, message_en}}`。

### 4.1 POST /public/auth/login

请求：
```json
{"site":"partner|customer|admin","handle":"...","password":"...","otp":"123456","device_fingerprint":"..."}
```
响应（成功）：
```json
{"success":true,"data":{"actor_type":"partner","actor_id":42,"fy_user_id":1042,"expires_at":"2026-05-11T10:15:00Z"},"error":null}
```
cookie：`tnbiz_access`(httpOnly) + `tnbiz_refresh`(httpOnly) + `tnbiz_csrf`(non-httpOnly)。

错误码：`BIZ_AUTH_INVALID` / `BIZ_AUTH_MFA_REQUIRED`。

### 4.2 POST /public/partner/apply

请求：
```json
{"type":"individual|enterprise","contact_name":"Alice","contact_phone":"+8613...","contact_email":"a@x.com","consent_id":1,"fy_user_id":100}
```
响应：`201 {"success":true,"data":{"id":1,"status":"applied"}}`。

错误：`BIZ_VALID_CONSENT` / `BIZ_PARTNER_EMAIL_DUP`。

### 4.3 POST /public/customer/register

请求：`{"fy_user_id":100,"invitation_code":"INV..."}`，**强制要求 invitation_code**（防绕过 §7.9）。
响应：`201 {"data":{"id":1,"partner_id":7,"status":"active"}}`；
错误：`BIZ_CUSTOMER_INVITATION_REQUIRED` / `BIZ_CUSTOMER_DUP`。

### 4.4 GET /partner/wallet

响应：
```json
{"data":{"wallet":{"partner_id":1,"balance":10000,...},"held_total":3000,"available":7000,"open_holds_count":1}}
```

### 4.5 POST /partner/invitation

请求 `{"type":"permanent|one_time|limited","usage_limit":10}`；
响应 `{"data":{"code":"ZJJOYVHSNOBUCQ5UOXFHJFGQ5U"}}`。

### 4.6 POST /(partner|customer)/kyc

请求字段见 `internal/handler/w1a_business.go::kycSubmitBody`：
`type / business_license_url / legal_person_name / legal_person_id / legal_person_id_url / bank_account / alipay_open_id / biometric_liveness_url / consent_sensitive_pi_id / consent_biometric_id`。
响应：`202 {"data":{"id":1,"status":"submitted"}}`。

### 4.7 POST /customer/transfer

请求 `{"to_partner_id":2,"reason":"..."}`；
响应 `202 {"data":{"change_log_id":1,"status":"pending_a"}}`。

### 4.8 POST /customer/erase

无 body；响应 `200 {"data":{"erased":true}}`。

---

## 5. 不变量 / review 必读

W1a 严格遵守 HANDOFF-W0 §5 全部 10 条不变量。**新增 W1a-specific invariant**：

| ID | invariant | 实现锚点 |
|---|---|---|
| AUTH-1 | login / refresh / reset 全部 5 次失败 → 锁账（账户级，不是设备级） | `auth.Service.Login` + `Repository.IncFailedAttempts` |
| AUTH-2 | refresh-token rotation：旧 jti 立即 revoke；复用旧 refresh = 攻击 → 全 session revoke | `auth.Service.Refresh` |
| AUTH-3 | 双因子重置 PR-INV-1..5 全部由 service 强制；token TTL 15min；失败 5 次永久 invalidate | `password_reset.go::PasswordResetConfirm` |
| PART-1 | partner Apply 必查 consent_log（5 min 内 + sensitive_pi）；email HMAC 全局唯一 | `partner.Service.Apply` |
| PART-2 | Terminate 同时调 `CustomerOrphaner.OrphanByPartner` 把客户置 orphaned + 30d grace | `partner.Service.Terminate` |
| CUST-1 | 注册必走 invitation_code；空码 → `BIZ_CUSTOMER_INVITATION_REQUIRED` | `customer.Service.Register` |
| CUST-2 | BOLA scope：partner 视角必走 `FindByIDForPartner`，cross-partner 返 nil（不返 403） | `customer.MemoryRepo.FindByIDForPartner` |
| KYC-1 | PII 字段 `legal_person_id` / `bank_account` 加密落库；blind_index = HMAC(scope+plain) | `kyc.Service.encryptInto` |
| KYC-2 | 同身份证不可两份申请（blind_index 全局唯一） | `kyc.Service.Submit` |
| KYC-3 | 年度驳回 ≥ 3 次 → status = `frozen_yearly_limit`；后续不再可 reject（直到 reset_at） | `kyc.Service.rejectFreeze` |
| KYC-4 | 上传必经 OSS magic-byte 二次校验 | `kyc.Service.verifyUploads` |
| INV-1 | code 长度 ≥ 16，base32(16 bytes CSPRNG) → 26 字符；UNIQUE 冲突自动 5 次重试 | `invitation.Service.randomCode` + `GenerateWith` |
| INV-2 | one_time 消费后状态置 `used_up`；后续 Resolve 返 `ErrCodeInactive` | `invitation.Service.Consume` |

---

## 6. 联调 cheatsheet（dev mode）

```bash
HTTP_ADDR=:18811 ENV=dev ./bin/partner-api &

# partner apply（场景 B）
curl -X POST http://localhost:18811/public/partner/apply \
  -H "Content-Type: application/json" \
  -d '{"type":"individual","contact_name":"Alice","contact_phone":"+8613000000001","contact_email":"alice@x.com","consent_id":1,"fy_user_id":100}'
# → 201 {"id":1,"status":"applied"}

# staff approve
curl -X POST -H "X-Dev-Actor-Type: staff" -H "X-Dev-Actor-Id: 99" \
  http://localhost:18811/admin/partners/1/approve
# → 200 {"status":"approved","approved_at":"..."}

# 生成 invitation
curl -X POST -H "X-Dev-Actor-Type: partner" -H "X-Dev-Actor-Id: 1" \
  http://localhost:18811/partner/invitation \
  -H "Content-Type: application/json" -d '{"type":"permanent"}'
# → 201 {"code":"ZJJ..."}

# customer 用 invitation 注册
curl -X POST http://localhost:18811/public/customer/register \
  -H "Content-Type: application/json" \
  -d '{"fy_user_id":200,"invitation_code":"ZJJ..."}'
# → 201 {"id":1,"partner_id":1,"status":"active"}

# 提交 KYC
curl -X POST -H "X-Dev-Actor-Type: customer" -H "X-Dev-Actor-Id: 200" \
  http://localhost:18811/customer/kyc \
  -H "Content-Type: application/json" \
  -d '{"type":2,"legal_person_name":"Alice","legal_person_id":"110101199001010010","legal_person_id_url":"https://oss.example.com/id.jpg","biometric_liveness_url":"https://oss.example.com/v.mp4","consent_sensitive_pi_id":1,"consent_biometric_id":2}'
# → 202 {"id":1,"status":"submitted"}

# wallet（dev 模式无 seed → 404）
curl -H "X-Dev-Actor-Type: partner" -H "X-Dev-Actor-Id: 1" \
  http://localhost:18811/partner/wallet
# → 404 BIZ_RES_NOT_FOUND
```

> dev 模式 `X-Dev-Actor-Type` / `X-Dev-Actor-Id` 仅作 W1a 联调入口；W1c JWT middleware 接入后必须删除该 header bypass（`scopeOf` 只读 `jwt_claims` ctx key）。

---

## 7. W1a 已知遗漏 / 后续 agent 补

1. **GORM repository 实现**：W1a 全部用 memory；W1c / W1b 在 `internal/repository/mysql/{auth,partner,customer,kyc,wallet,invitation}_mysql.go` 增补 GORM 实现 + row→domain 映射 + KMS 加密管线。
2. **JWT middleware 接入**：W1c 实现 `middleware.JWT(verifier, revoke)` 后，handler 层 `scopeOf` 只读 `jwt_claims`；删 `X-Dev-Actor-*` header bypass。
3. **CSRF + idempotency middleware**：W1a 所有写操作目前未走 idempotency middleware；W1c 接入 `middleware.Idempotency(idemRepo, kms)` 后，service 层在业务 TX 内调 `idemRepo.Insert(tx, ...)` 兜底。已在 service 函数注释里指明拦截点。
4. **真 KMS / OSS / FyAPI**：W1a stub 不加密；W1c 接 `infra/kms.AliyunService` / `infra/oss.AliyunService` / `infra/fyapi.Client` 后替换 dev wiring。
5. **Allocate / Refund / Topup saga**：wallet.AllocateExecutor 接口已锁定；W1b 实现 saga + 替换 dev wiring（不需要改 handler）。
6. **session 表 migration**：W1c 加 `0006_session.up.sql`（schema 见 §2.1）。
7. **Notify**：`auth.Notifier` 用 `auth.NoopNotifier{}`；W1c notify dispatcher 接入后替换。
8. **partner_wallet seed**：W1b 接 `wallet.AllocateExecutor` 时确保 partner approve 后建 `partner_wallet` 行（saga step `wallet.create_for_partner`）；目前 wallet 查询 dev 模式恒为 404，handler / 前端能正确渲染空态。
9. **e2e 测试**：`tests/e2e/` 仅占位；W1a 提交了 6 条 unit test 套件但 e2e（chaos / BOLA matrix）仍是 W0 §6 遗留项 #10。
10. **OpenAPI 文件**：本 HANDOFF 列出了 W1a endpoint 的字段；正式 `openapi/internal-api.yaml` 由 W1c 协同导出。

---

## 8. 验收复跑

```bash
cd ~/Projects/apiGateway/TraceNexBiz/apps/partner-api
go build ./...              # ok
go vet ./...                # ok
go test -race ./internal/service/auth/... ./internal/service/kyc/... \
              ./internal/service/wallet/... ./internal/service/partner/... \
              ./internal/service/customer/... ./internal/service/invitation/...
# 6 packages, 全 PASS
HTTP_ADDR=:18811 ENV=dev ./bin/partner-api &
# 跑 §6 cheatsheet 的 5+ 条 curl，全部得到预期 200/201/202/404
```

W1a 交付完毕。下一个 agent（W1b）请基于本 HANDOFF 的 §3 wallet.AllocateExecutor 接口起步。
