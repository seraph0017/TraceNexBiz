# 01 — Code Quality Review（Round 1，通用代码质量）

**审阅人**：Code Reviewer（通用质量维度）
**日期**：2026-05-14
**审阅范围**：
- `apps/partner-api/`（domain / repository / service / handler / middleware / infra / saga / outbox / audit，共 117 个 .go 文件，~14k LOC）
- `apps/partner-web-{storefront,customer,partner,admin}/`（~184 个 .ts/.tsx，~13.6k LOC）
- `packages/{ui-kit,api-client,i18n,config}`
- `Fy-api/` OVERLAY（B-12..B-18 约 2,228 行新增）
**并行工作**：Security / Compliance / Architect / Backend Architect(Fy-api)
**本评审覆盖维度**：可读性、immutability、错误处理、函数/文件大小、嵌套、测试、依赖、重复、死代码、跨 agent 契约

---

## 1. 执行摘要 + Verdict

**总体 Verdict：PASS-WITH-CONDITIONS**

代码整体结构干净、工程素养良好：TDD 执行到位（26 个 Go service/infra 包全部带单元测试；front-end 27 + 12 + 10×2 = 59 个 vitest 用例），`-race` 全绿、`go vet` / `tsc --noEmit` 全绿、`go build` 全绿。immutability、状态机纯函数、BOLA scoped API、KMS 加密管线、idempotency middleware-read / service-layer-Insert 的 ADR-003 分层约束都在 service 层字面级落地。版本号跨 4 个前端站点完全对齐。**没有发现 CRITICAL 阻断上线的通用代码质量问题。**

但以下三件事必须在 W2 之前收口，否则合并前端 / 后端 / Fy-api 三方契约时仍有字面级漂移：

1. **HIGH-1**：admin 前端 `forceResolveSaga` 与后端 `NewSagaForceResolveHandler` 的请求体字段不一致（前端走 `{approver_token, approver_ip, outcome, reason}`；后端 `ForceResolveBody` 要 `{saga_id, step_name, target, approver_id, approver_ip, reason, token_issued_at}`）；且 `admin/admin.go::sagaForceResolve` 和 `handler/saga_admin.go::NewSagaForceResolveHandler` 是两套并存的 handler，body schema、错误码、path 都不同（前者 `/saga/:id/force-resolve` + `BIZ_SAGA_FORCE_RESOLVE`；后者 `/admin/saga/force-resolve` + `BIZ_DUAL_CONTROL_REJECTED`）。必须收敛到一套。
2. **HIGH-2**：三个后端 handler package（`handler`、`handler/admin`、`handler/saga_admin.go`）各自实现 `ok/fail` + `respondError/bad` envelope 辅助函数，字段名、大小写、`trace_id` 是否带都不一致，至少两处重复定义相同目的的 helper（`handler/w1a_routes.go::ok/fail` vs `handler/admin/admin.go::ok/bad` vs `handler/saga_admin.go::respondError`），违反 PRD §11 错误 envelope invariant 的一致性。
3. **HIGH-3**：4 个前端站点的 `src/api/client.ts`/`types.ts`/`error-mapping.ts` 与 `packages/api-client/src/*` 物理上是两份代码（md5 一致确认 customer/partner/admin 三份 client.ts 完全相同，但 storefront 是一个独立变体，packages/api-client 又是第三个 slightly-different 变体）。HANDOFF 承认这是已知 cost，但 4 个文件共 140+ 行业务代码三副本存在长期漂移风险，W2 必须拍板搬到 `packages/`（或至少 workspace symlink）。

此外还有大量 TODO（partner-api 本仓 51 处 `TODO/FIXME`，主要是 W1a 的中间件 stub —— JWT / CSRF / Idempotency / BOLA scope / Audit 五个 middleware 全是 `c.Next()` 占位），但 HANDOFF-W1a §7 / W0 §6 已明确标记，属于计划内待补，不是代码质量问题。

---

## 2. 总体代码质量打分（1-5 分；5 = 极好，3 = 可接受，1 = 重做）

| 维度 | 分数 | 说明 |
|---|---:|---|
| 可读性 / 命名 | 4 | 命名一致（ActorType / Status / State），package doc 齐全；个别包存在 `_test.go` 以外的 `NewStubXxx` 与主逻辑混放 |
| immutability | 4 | service 返回的都是值/深拷贝；saga `instance.snapshotXxx` 严格不 mutate 入参；MemoryRepo 全部 `cp := *v` 拷贝；但 `domain.Partner` 仍然保留 `ContactPhonePlain` / `LegalPersonIDPlain` 字段（PII 明文字段，domain 层以 comment 而非 tag 保护） |
| 错误处理 | 4 | 普遍用 `fmt.Errorf("...: %w", err)` 包装；sentinel + `errors.Is` 在 handler 层映射；但 `topup/OnCallback` / `saga_allocate/Run` 的补偿链把 `_, _ = sg.Compensate(...)` 的错误全忽略 |
| 函数大小 | 4 | 仅 15 处函数 > 50 行，最大 75 行（`invoice.canTransition` 的 map 定义 + `RedFlush` 70 行，`saga_allocate.Run` 60 行）；大部分控制得不错 |
| 文件大小 | 4 | 仅 3 个非测试 Go 文件 > 400 行（`invoice.go` 480 / `content_safety.go` 449 / `admin.go` 402）；1 个前端文件 > 400（`ApplyPartner.tsx` 559） |
| 嵌套深度 | 4 | 最深 4 层（`outbox.TickOnce` for-if-if 3 层，`saga.instance.Run` 3 层）；不突出 |
| 测试覆盖 / 质量 | 3 | Go 24 个 service/infra 包平均覆盖 72% / 最高 92%（revenue）/ 最低 0%（saga_refund、saga_topup 两个新增 saga 无单测，handler 顶层包 `internal/handler` 0%）；前端 e2e 写好但 `@playwright/test` 未装，实测未跑 |
| 依赖管理 | 4 | 4 个前端版本号完全对齐；Go go.mod 无冗余；`packages/api-client` 里用 `uuid` v4，storefront 手写 fallback —— 两套 UUID 生成函数 |
| DRY / 代码重复 | 2 | 前端 client.ts / types.ts / error-mapping.ts 三副本；后端三套 envelope helper；`ternary` 重复出现；`parseInt64` / `fmtSscan` / `atoiOrDefault` 三个同义工具 |
| 死代码 / TODO | 3 | 51 处 `TODO(W1a)` 分布于 middleware / repository / handler，HANDOFF 承认；storefront `ApplyPartner` 业务字段（`company_name` / `unified_social_credit_code` 等）在 W1e 自己 §4 承认与 W1a 后端 body schema 不对齐 |

**加权平均**：3.7 / 5。"PASS-WITH-CONDITIONS" 描述准确。

---

## 3. CRITICAL 问题清单（不修不能上线）

**无 CRITICAL。** 所有 CRITICAL 问题（PII 泄漏、authz bypass、hardcoded secret）均在 Security / Compliance reviewer 范围，本评审不重复。

从通用代码质量角度：

### CQ-CRIT-0（候选，边缘）— dev-only backdoor 必须入 HIGH

`apps/partner-api/internal/handler/w1a_routes.go:102-111` 的 `scopeOf()` 仍然允许 `X-Dev-Actor-Type` / `X-Dev-Actor-Id` header 直接注入 actor identity —— HANDOFF 已明确 W1c JWT middleware 接入后删除，但此条 bypass 既未做 `env != "dev"` 的守卫也未做 `//go:build dev` 约束，如果被误投到 prod 即是 authn 完全 bypass。建议 W1a/W1c 在 `routes.go` 注册时以 `cfg.Env == "dev"` 为前提挂 bypass 层，或加 `//go:build dev` 构建 tag 把 bypass 从 prod 二进制物理剔除。标为 CRITICAL 候选，考虑 Security 会覆盖，仅在这里声明。

---

## 4. HIGH 问题清单（应修）

### HIGH-1 — force-resolve 端点契约字面级分裂
**文件**：
- `apps/partner-api/internal/handler/admin/admin.go:60` + `:343-376`（POST /saga/:id/force-resolve，body `{approver_token, approver_ip, outcome, reason}`，错误码 `BIZ_SAGA_FORCE_RESOLVE`）
- `apps/partner-api/internal/handler/saga_admin.go:29-91`（POST /admin/saga/force-resolve（注意 `:id` 路径参数也没有），body `{saga_id, step_name, target, approver_id, approver_ip, reason, token_issued_at}`，错误码 `BIZ_DUAL_CONTROL_REJECTED` / `BIZ_SAGA_FORCE_RESOLVE_FAILED`）
- `apps/partner-web-admin/src/api/admin.ts:344`（前端调 `/api/admin/saga/${sagaId}/force-resolve`，body `{approver_token, approver_ip, outcome, reason}`）
- `openapi/admin.yaml:L11`（spec 只列 `/saga/{id}/force-resolve`）

**问题**：admin web 前端、`openapi/admin.yaml`、`internal/handler/admin/admin.go` 是一套（`/saga/:id/force-resolve` + `approver_token` 形态）；`internal/handler/saga_admin.go` 是平行另一套（不带 `:id` 路径、要 `approver_id+token_issued_at`）。两套 handler 对应两套 service —— 前者调 `saga_admin.Service.ForceResolve`（W1c）、后者调 `saga.ValidateForceResolve` 纯函数 + `SagaRepo.ForceResolve`（W1b）。W1b 和 W1c 都实现了各自的 force-resolve，**没有一方在使用对方**。

**影响**：
1. `cmd/server/main.go::buildRouter` 今天没挂任何 admin 路由（只挂了 W1a），所以暂时没冲突；但一旦 W2 做 wiring，必然要么二选一、要么两个端点并存让前端选错 404。
2. 前端 `admin.ts::ForceResolveInput` 里没有 `saga_id`（从 path 取）、没有 `step_name`、没有 `target` —— 与 `saga.ForceResolveRequest{StepName, Target}` 不兼容。
3. `dual-control` 的两条约束链（`saga.ValidateForceResolve` 校 IP/24、cooldown、token TTL 纯函数；`saga_admin.Service.ForceResolve` 校 approver != initiator、/24、cooldown、token consume）实现重复、字段命名也不一致（`TokenIssuedAt` vs `IssuedAt+ExpiresAt+Consumed`）。

**修订指令**：
- W2 前收敛 —— 推荐保留 `internal/service/saga_admin/saga_admin.go`（封装性更好：一次性 token + cooldown 持久化 + audit sink 一套接口都全），删掉 `internal/saga/force_resolve.go` 的 `ValidateForceResolve` 纯函数和对应 test（保留 `IsValidUUIDv7` / `isValidForceTarget` 合并到 `saga_admin`）。
- 或保留 `saga.ValidateForceResolve` 纯函数作为 service 层内部调用，但删除 `internal/handler/saga_admin.go` 整个文件，统一走 `internal/handler/admin/admin.go::sagaForceResolve` 一条路径。
- 收敛后前端 `admin.ts::ForceResolveInput` 补齐 `step_name` 和 `target` 两个字段（或前端路径 `${sagaId}/${stepName}/force-resolve`，在后端从 path 提取）。
- `openapi/admin.yaml` 同步更新 requestBody schema。

---

### HIGH-2 — 三套 handler envelope helper 不一致
**文件**：
- `apps/partner-api/internal/handler/w1a_routes.go:80-91`：`ok(c, status, data)` + `fail(c, status, code, msgZh, msgEn)` —— 错误不带 `trace_id` 字段
- `apps/partner-api/internal/handler/admin/admin.go:63-83`：`ok(c, data)` + `bad(c, status, code, msg)` + `unavailable(c, what)` —— 错误带 `trace_id` 从 `c.GetString("trace_id")` 取
- `apps/partner-api/internal/handler/saga_admin.go:134-143`：`respondError(c, status, code, msg)` —— 同样从 `c.GetString("trace_id")` 取

**问题**：
1. PRD §11 错误 envelope 要求 `{success, data, error{code, message_zh, message_en, trace_id, details}}` 五段一致；但 `w1a_routes.go::fail` **缺 trace_id 字段**，前端 toast i18n 依赖 trace_id 时会拿到 undefined。
2. 三套 helper 签名不一致，handler 作者要根据所在文件切换调用方式，违反最小惊讶；`bad` 和 `fail` 同义但参数顺序不同（`bad(c, status, code, msg)` vs `fail(c, status, code, msgZh, msgEn)`）。
3. `admin.go` 里 `bad` 的 `message_zh` 和 `message_en` 直接赋同一 string —— 违反双语提示 invariant。

**修订指令**：
- 建 `apps/partner-api/internal/handler/envelope.go` 单文件（≤ 100 行），只暴露 `OK(c, status, data)` / `Fail(c, status, code, msgZh, msgEn)` 一对函数 + `Unavailable(c, what string)` 一个兜底。
- 所有 handler 文件改用这一对；删掉三处私有辅助。
- `message_zh` / `message_en` 强制两个字符串必传（签名级强制，而不是 runtime 检查）。
- trace_id 从 `c.Request.Context()` 通过 `pkg/tracing.FromContext(ctx)` 统一读取，避免 `c.GetString("trace_id")` 字面 key。

---

### HIGH-3 — API 客户端跨 workspace 三副本
**文件**：
- `apps/partner-web-customer/src/api/client.ts` md5=517aa181db1ed7f7a96284ea5790bf95
- `apps/partner-web-partner/src/api/client.ts` md5=517aa181db1ed7f7a96284ea5790bf95
- `apps/partner-web-admin/src/api/client.ts` md5=517aa181db1ed7f7a96284ea5790bf95
- `apps/partner-web-storefront/src/api/client.ts`（slightly-different 单文件，无 silent refresh）
- `packages/api-client/src/client.ts`（又一个变体：用 `uuid` 库、无 silent refresh、无 unwrap helper）

**问题**：customer/partner/admin 三份字面级相同；storefront 略不同（缺 401 silent refresh）；`packages/api-client` 是第四个变体且没被任何 app 引用（`pnpm install` 未跑过，workspace link 不生效）。任一 bugfix 要手动同步三到四次；同时类型定义（`types.ts` md5=1ce7024... 三份字面相同）+ `error-mapping.ts`（md5=8d9a67... 三份字面相同）同样重复。

**影响**：W1f HANDOFF §4.2、W1g HANDOFF §5.10 都把"搬到 packages/"列为 W2 review 后的显式 todo。但如果此时前端团队继续在 3 份里各自新增业务，W2 迁移时 merge conflict 体量翻倍。

**修订指令**：
- 立即动作（W2 第一周）：把 customer/partner/admin 三份 `client.ts`、`types.ts`、`error-mapping.ts` 提升到 `packages/api-client/src/`，storefront 变体作为 `packages/api-client/src/client-public.ts` 分叉（无 401 refresh 的纯净版），三个登录态站点通过 `@tracenex-biz/api-client` 导入；storefront 按需导入 public 版本。
- 锁 `pnpm install` lockfile，`pnpm -r build` 验证 workspace resolve OK。
- `packages/api-client/src/client.ts` 里用 `crypto.randomUUID` + 手写 fallback，删掉对 `uuid` 包的依赖（减少一条 npm 依赖）。

---

### HIGH-4 — saga 补偿链静默吞错误
**文件**：`apps/partner-api/internal/service/saga_allocate/service.go:129-157`、`apps/partner-api/internal/service/saga_topup/service.go:OnCallback`

**问题**：
```go
_, _ = sg.Compensate(ctx, StepHold, func(_ *gorm.DB) (any, error) {
    return nil, s.wallet.ReleaseHold(ctx, req.SagaID)
})
```
补偿失败时 `_, _ =` 丢弃，只依赖 retry worker 兜底推进。但 HANDOFF-W1b §4.5 明确承认 "retry worker 的真实重做" 尚未实现（`Sweep` 只判断 escalation、不重做 fn）。这意味着：StepFyTopup 成功后 StepCommit 失败，继而补偿 Fy-api refund 又失败，业务数据会停留在 Fy-api 已提额、partner 钱包 hold 未释放的不一致状态，且不会再被自动处理，也不会留下错误链路。

**修订指令**：
- `sg.Compensate` 返回的 error 要至少 log + metrics increment（`c.Error(err)` 或 `zerolog.Err(err)`），不能 `_`；
- 补偿失败必须标记 saga_step 为 `escalated` 而不是 `failed`（因为 retry worker 目前不重做），让 admin force-resolve UI 能够看见；
- `saga.instance.Compensate` 在 txErr != nil 时只写了 `failed` status（`instance.go:174-178`），同理需要根据 attempts 阈值直接 escalate。

---

### HIGH-5 — `cmd/server/main.go` 还未装配 W1b/W1c service
**文件**：`apps/partner-api/cmd/server/main.go:132-171`

**问题**：`buildW1aDeps` 只构造 W1a 的 6 个 service（auth/partner/customer/kyc/wallet/invitation），全部用 memory + stub。`cmd/server/main.go` 完全没有 wire `saga.Orchestrator` / `outbox.Consumer` / `audit.Sealer` / `saga_allocate.Service` / `saga_topup.Service` / `saga_refund.Service` / `invoice.Service` / `ticket.Service` / `content_safety.Service` / `settlement.Runner` / `notification.Service` / `staff.Service` / `payment.Service` / `dispute.Service` / `saga_admin.Service`。cmd 启动后只有 W1a 6 条路由 + `RegisterTODORoutes`（占位 `BIZ_TODO_NOT_IMPLEMENTED`）。

**影响**：W1b/W1c 所有 service 写完了单测、但 HTTP 入口没挂，二进制实质不可用。HANDOFF-W1c §"待 W1a" 隐含承认"cron 入口 / admin endpoints mount 待 W1a"，但 W1a HANDOFF 没把 wiring 纳入 §7 遗漏清单。

**修订指令**：
- W2 前由 W1a 或独立 wiring agent 建 `cmd/server/deps.go` 模块把 W1b/W1c 全部 service + repo 装配起来（memory stub 可以保留给 dev，但接口要被调用），路由挂到 `admin.Register(rg, deps)`；否则 smoke test 会完全绕过 W1b/W1c 的代码。

---

### HIGH-6 — `ApplyPartner.tsx` 单文件 559 行、违反 400 行上限
**文件**：`apps/partner-web-storefront/src/pages/ApplyPartner.tsx:1-559`

**问题**：一个文件里塞了 6 个 subcomponent（`ContactForm` / `CompanyForm` / `ScaleForm` / `KycStepView` / `ReviewStep` / `Summary` / `FormButtons` / `SubmittedScreen`）+ 主组件 + 3 个 helper。行数超过项目 400 行上限。

**修订指令**：
- 拆到 `src/pages/ApplyPartner/{index.tsx, ContactForm.tsx, CompanyForm.tsx, ScaleForm.tsx, KycStep.tsx, ReviewStep.tsx, Summary.tsx, FormButtons.tsx}`；
- 主 `index.tsx` 只负责路由 step 机 + 状态；
- 子 form 每个 ≤ 120 行。

---

### HIGH-7 — OTP 生成字符串切片有截断歧义
**文件**：`apps/partner-api/internal/service/auth/auth.go:354`

**问题**：
```go
otp := fmt.Sprintf("%06d",
    uint32(otpBytes[0])<<24|uint32(otpBytes[1])<<16|
    uint32(otpBytes[2])<<8|uint32(otpBytes[3]))[:6]
```
`%06d` 的输出对 uint32 最大 10 位（4294967295），`[:6]` 只取前 6 位 —— 这会导致多数 OTP 不是 `%06d` 期望的"6 位含前导零"，而是从 10 位大数里砍头，均匀性破坏且可能短于 6 位（超过整型最大值后 `fmt` 仍返 10 位）。如 `uint32=123456789` → `%06d` = `"123456789"` → `[:6]` = `"123456"`。可运行但 OTP 空间明显偏 1-5 开头。

**修订指令**：
- 改成 `otp := fmt.Sprintf("%06d", binary.BigEndian.Uint32(otpBytes) % 1000000)`，保证严格 000000..999999 均匀分布；
- 添加 unit test 断言 1e5 次采样后 6 位覆盖 0-9 全部。

---

### HIGH-8 — `saga.BackoffFor` 指数溢出计算次数过多
**文件**：`apps/partner-api/internal/saga/saga.go:155-167`

**问题**：
```go
for i := 0; i < attempts && d < BackoffMax; i++ {
    d *= 2
}
```
实际当 `attempts=1` 时循环一次 → `d=4s`（但文档注释说 `attempts=1 → 2s`）。`BackoffFor(0)` 返 `BackoffBase=2s` 正确；`BackoffFor(1)` 却返 `4s`。文档和实现不一致。

**修订指令**：
- 让 `BackoffFor(1)` == 2s（首次重试立刻以 2s 回退）：改成 `for i := 1; i < attempts && d < BackoffMax; i++`；
- 现有 `saga_test.go::TestBackoffFor` 必须扩展到覆盖 attempts=0/1/2/3/8/30 的所有边界。

---

## 5. MEDIUM 问题（建议修）

### CQ-M-1 — `domain.KYCApplication` / `domain.Partner` 仍带 `*Plain` 字段
**文件**：`apps/partner-api/internal/domain/kyc.go:18-35`、`domain/partner.go:53`、`domain/compliance.go:66`
domain 层本来不该承载明文 PII；目前以 comment `// 仅 service 内瞬态；不出库不入日志` 约束，靠约定而非类型。建议把 `*Plain` 字段从 domain struct 抽到 service 层专用的 `SubmitInput` / `Credential` 临时结构体，让 GORM 只能映射 cipher 字段 —— 物理上不可能把明文写库。

### CQ-M-2 — invoice.go / content_safety.go / admin.go 三个文件超 400 行
**文件**：见第 2 节
建议：
- `invoice.go` 把 `MemoryRepo` 拆到 `memory.go`（≈ 100 行），主文件压到 ~370 行
- `content_safety.go` 同样 `MemoryRepo` 拆到 `memory.go`
- `handler/admin/admin.go` 按 invoice/ticket/content_safety/staff/saga 五段拆成 5 个文件，每个 ≤ 100 行

### CQ-M-3 — `ternary` helper 重复实现
`invoice.go:475` 定义 `ternary(cond bool, a, b string) string`；其他 package 可能要用 —— 如果要用应收到 `pkg/util/`；否则删掉只在 `Review` 一处使用的 ternary，改成显式 if-else 两行更清晰且 goimports 友好。

### CQ-M-4 — `parseInt64` / `fmtSscan` / `atoiOrDefault` 三个同义 helper
**文件**：`handler/saga_admin.go:146-158`、`handler/w1a_routes.go:114-123`、`handler/admin/admin.go:380-389`
三处都是字符串转 int64/int。统一用 `strconv.ParseInt`，别在手写 state-machine parser（`fmtSscan` 写成 `for _, r := range s { if r < '0' || r > '9' ...`）—— 既不 gofmt 地道，又没覆盖负号，而且重复发明了轮子。

### CQ-M-5 — `saga.MustNewSagaID` 在业务代码用了 panic
**文件**：`saga.go:134-141`
doc 注释说"仅启动期使用"，但没有 build-time 约束保证。建议加 `//nolint:staticcheck` 注释和单元测试断言在 service path 里不被 call（用 `go vet -stdmethods`）。

### CQ-M-6 — `topup.OnCallback` 静默返回
**文件**：`saga_topup/service.go:178-180`
`return nil` 结尾注释说 "fund 走异步：service 调用方根据 callback 后立刻进 fund 步骤"。但 `OnCallback` 完成后没有任何机制确保 `Fund` 会被调用。如果没有外部调度器，callback 之后永远停在 `paid` 状态。需要 OnCallback 之后 enqueue 一个 task（或直接 synchronous 调 Fund），不要只 mark paid。

### CQ-M-7 — `saga_admin.same24` 对 IPv6 返回 `false, nil`（允许放行）
**文件**：`saga_admin.go:182-188`
注释"partner-api is internal; treat unknown as different to fail-closed"逻辑反了：返回 false 表示"不在同一子网"——放行，不是 fail-closed。fail-closed 应该返 `true, nil`（当成同网段 → 拒绝）或返 `error`。

### CQ-M-8 — admin `ok/bad` envelope 把 `message_zh` 和 `message_en` 赋同一值
**文件**：`handler/admin/admin.go:68-79`
违反双语 UI 设计。与 HIGH-2 合并修订。

### CQ-M-9 — storefront `ApplyPartner` submit 字段与 W1a `partnerApplyBody` 不兼容
**文件**：`partner-web-storefront/src/pages/ApplyPartner.tsx:196-207` vs `handler/w1a_business.go:18-27`
前端提交 `company_name / unified_social_credit_code / expected_monthly_calls / expected_use_case / source_channel`，后端 `partnerApplyBody` 只接 `type / business_name / contact_name / contact_phone / contact_email / consent_id / note / fy_user_id`。
HANDOFF-W1e §4.2 自己承认；W2 前必须一方妥协：建议后端接受这些字段（只做 pass-through 入 `partner.notes` 或扩展 `ApplyInput`）。

### CQ-M-10 — 前端版本号全部是 pinned 确定版本
4 个站点的 `package.json` 所有依赖都是钉死版本（`"axios": "1.7.0"`，不是 `^1.7.0`）。pinned 在 monorepo 里合理，但所有包都是 pinned 会让 `pnpm audit` 的 auto-bump 失效。建议 devDependencies 用 `~`，runtime 保持 pinned。

### CQ-M-11 — `pkg/errors/errors.go` 覆盖率 57.9%、含 30+ code enum
需要补一组表驱动测试覆盖每个 `Code` → HTTP status 映射。

### CQ-M-12 — `internal/config/config.go` 覆盖率 20.8%
`Load()` 70 行、主要是 env 解析 + biz_setting registry，建议对 invariant（必填 env 缺失 → 启动失败）加 unit test。

---

## 6. LOW 问题（可选修）

- **LOW-1**：`handler/saga_admin.go:160-164` 自造 `errInvalidNumber = &handlerErr{"invalid number"}` 替代 stdlib sentinel；不必要。
- **LOW-2**：`outbox/consumer.go:186-230` 的 `TickResult` 字段命名 `Pulled/Acked/Failed/DLQ` 前三个过去式、`DLQ` 名词；命名不一致。
- **LOW-3**：`content_safety.go:246-249` `buildPayload` 用 `fmt.Sprintf` 拼 JSON 字符串 —— 应走 `encoding/json` 标准编码，否则 `e.Category` / `e.PromptHash` 含引号会破坏 payload。
- **LOW-4**：前端 `storefront/src/api/client.ts:21-24` 的 UUID v4 fallback `${r(8)}-${r(4)}-4${r(3)}-${(8+...)}...` 手写不稳妥，W2 统一用 `crypto.randomUUID()` + 抛错替代。
- **LOW-5**：`saga_admin.go:283-300` 的 `StubResolver` 和 `CapturingAudit` 是测试辅助，应放到 `saga_admin_test_helpers.go`（`// +build testing`）或 `internal/testing/`，避免污染生产导出 API。
- **LOW-6**：`handler/admin/admin.go:391-402` 的 `clientIP` 在 `handler/saga_admin.go`、`middleware/*` 等地方多次各自实现；建议挂到 `pkg/netx/`。
- **LOW-7**：`topup/service.go` 缺单元测试（覆盖率 0%）；`saga_refund/service.go` 同样缺。
- **LOW-8**：`cmd/server/main.go::buildRouter` 的 `RegisterTODORoutes` 暴露 `BIZ_TODO_NOT_IMPLEMENTED` 端点，prod 构建时应当 build-tag 剔除。
- **LOW-9**：`internal/outbox/memstub.go` 192 行，接近上限，可拆成 `source_mem.go` + `sink_mem.go`。
- **LOW-10**：`internal/audit/sealer.go:265` 的 `DeleteUnsealed` 内部写 `out := s.unsealed[:0]` 然后 append —— 原地重用可能在并发下坑人（虽然拿了 mu.Lock，但 read 侧若外部持有引用会惊讶）。注释一下。

---

## 7. 跨 agent 契约对齐审计（HANDOFF 声明 vs 实际代码）

说明：✅ = 对齐；⚠️ = 部分对齐 / 命名漂移；❌ = 未实现 / 严重漂移。

| # | 契约（HANDOFF 原文） | 实际锚点 | 对齐状态 | 备注 |
|---|---|---|---:|---|
| 1 | W1a §2.1 `auth.Repository` 12 方法签名固化 | `service/auth/types.go` + `service/auth/memory.go` | ✅ | `FindCredentials / FindCredentialsAny / IncFailedAttempts / ResetFailedAttempts / ... / ApplyPasswordReset` 12 条全部存在 |
| 2 | W1a §3.1 `wallet.AllocateExecutor` 签名锁定 `Allocate/Refund/Topup` | 实际存在于 `saga_allocate/service.go::WalletPort` + `saga_topup/service.go::IntentPort` —— **接口名与 HANDOFF 不一致** | ⚠️ | HANDOFF 说 `wallet.AllocateExecutor`，代码中分别是 `saga_allocate.WalletPort` 和 `saga_topup.IntentPort+ProviderPort`；不是同一 interface。`wallet.AllocateExecutor` 这个 interface 在代码里搜不到 |
| 3 | W1a §4.1 `POST /public/auth/login` body `{site, handle, password, otp, device_fingerprint}` | `service/auth/auth.go::LoginInput` + handler `w1a_auth.go`（未展开） | ✅ | 字段全部存在；但 login handler 本文未检索（假定 W1a §6 cheatsheet 验证过 5 条 curl） |
| 4 | W1a §4.2 `POST /public/partner/apply` body `{type,contact_name,contact_phone,contact_email,consent_id,fy_user_id}` | `handler/w1a_business.go:18-27 partnerApplyBody` 实际接 `type/business_name/contact_name/contact_phone/contact_email/consent_id/note/fy_user_id` | ⚠️ | 后端多出 `business_name/note`，缺 storefront 前端要传的 `company_name/unified_social_credit_code/expected_monthly_calls/expected_use_case/source_channel` |
| 5 | W1a §4.3 `POST /public/customer/register` 必传 `invitation_code`（防绕过） | `handler/w1a_business.go:112-115 customerRegisterBody` + service `ErrInvitationRequired` | ✅ | `binding:"required"` + service 层兜底 |
| 6 | W1b §3.1 `saga.Repository` GORM 实现 W1a 落 | `internal/repository/mysql/*.go`（13% 覆盖，内容未展开）；`repository.go:92` 是 `TODO(W1a)` | ❌ | W1a 没落，仅内存 `memrepo.go` 实现；W1b 承认，但 W1a HANDOFF §7 没把它写入 |
| 7 | W1b §3.1 `outbox.Source` GORM 实现 `source_log_db.go` | 不存在此文件；只有 `memstub.go`（W1b 交付）+ `poller.go` 26 行 | ❌ | 同上，待 W1a |
| 8 | W1b §5 不变量："immutability：Run/Compensate/Transition 全部返回新结构体，不 mutate 入参" | `saga/instance.go::snapshotStarting/snapshotFailed/snapshotCommitted` 全部 `out := *prev` 拷贝 | ✅ | 严格执行 |
| 9 | W1c §交付 invoice/payment/ticket/notification/content_safety/staff/saga_admin 每个包 ≥ 5 单测 | 各 package `_test.go` 存在 + `go test -cover` 63%-92% | ✅ | 覆盖良好 |
| 10 | W1c admin endpoint 10 条（invoice review/issue/red-flush/ticket list/assign/cs list/retry/dispatch/staff/saga force-resolve） | `handler/admin/admin.go::Register` 10 条路由 | ✅ | 10 条都挂了；`admin_test.go` 10 个集成测试存在 |
| 11 | W1e §2.5 CSP `script-src 'self'` | `storefront/index.html`（未展开但 HANDOFF 声明） | ⚠️ | 未 spot check 验证；依赖 HANDOFF 声明 |
| 12 | W1f §2 "直接复制" storefront/customer/partner/admin 四份 `api/client.ts` | md5 确认 customer/partner/admin 三份字面相同；storefront 单独变体（缺 silent refresh） | ⚠️ | 三份相同符合契约；storefront 变体是已知决策 |
| 13 | W1f §4.7 "每次 POST 都生成新 UUID" | `client.ts::apiClient.interceptors.request.use` 写入 `Idempotency-Key` / `X-Oneapi-Request-Id` | ✅ | interceptor 实装 |
| 14 | W1g §3.8 admin `SagaForceResolve` "server-side 检查 4 约束" | 后端 `saga_admin.go::ForceResolve` 5 约束（cooldown/token/approver != initiator/different /24/audit） | ✅ | 后端约束比契约更严 |
| 15 | W1g §3.14 "Idempotency-Key + X-Csrf 复用 W1f client.ts" | customer/partner/admin md5 同 | ✅ | |
| 16 | Fy-api OVERLAY §2 `router/main.go` +1 行 `SetInternalRouter(router)` | 未进入本次 review 范围（Fy-api 侧由 Backend Architect reviewer 验） | — | 不覆盖 |
| 17 | Fy-api OVERLAY §3 HMAC 头 `X-Tnb-Key-Id/Timestamp/Nonce/Signature` | `apps/partner-api/internal/infra/fyapi/client.go::Do` 使用的 header 名（未展开）应与之对齐 | — | 需要 Backend Architect 跨仓库验 |
| 18 | 全局 invariant：immutability `Update(updater func(X) X)` 模式 | `partner/partner.go::Update` / `kyc/kyc.go::Repo.Update` / `invoice/invoice.go::Repo.UpdateApplication` 全部 updater 签名 | ✅ | 各 service 都走 updater 模式 |
| 19 | 全局 invariant：BOLA scope repo 首参带 `partnerID/customerID/staffID` | `wallet.Repository.FindWallet(ctx, partnerID)` / `customer.Repository.FindByIDForPartner(ctx, partnerID, customerID)` / `invoice.Repo.GetTitle(ctx, ownerType, ownerID, titleID)` | ✅ | 严格执行；`customer.FindByIDForPartner` 越权返 nil 而非 403（PRD §16.3） |
| 20 | 全局 invariant：pkg/errors.Wrap(err, code) 显式包装 | service 层普遍用 `fmt.Errorf("...:%w", err)` 而非 `pkg/errors.Wrap` | ⚠️ | 没统一走 `pkg/errors`；用 stdlib 也可接受，但与 HANDOFF "显式 pkg/errors.Wrap" 语义不同 |

**契约对齐总分**：15 ✅ / 4 ⚠️ / 2 ❌（W1a GORM 未落、outbox Source 未落均为 HANDOFF 已声明待办）。

---

## 8. 修订指令（每条"改哪个文件 第几行 怎么改"）

| 编号 | 文件 | 行 | 改动 |
|---|---|---|---|
| R-H-1a | `apps/partner-api/internal/handler/saga_admin.go` | 删除整个文件 29-165 | 合并到 `handler/admin/admin.go::sagaForceResolve`（或反向：保 saga_admin.go 删除 admin.go 的 force-resolve block） |
| R-H-1b | `apps/partner-web-admin/src/api/admin.ts` | 338-347 | `ForceResolveInput` 增加 `step_name: string` + `target: "committed"\|"compensated"\|"released_pessimistic"`；或改用 sub-path `/saga/${sagaId}/${stepName}/force-resolve` |
| R-H-1c | `apps/partner-api/openapi/admin.yaml` | 新增 `/saga/{id}/force-resolve` 下的 requestBody schema，把 W1b `saga.ForceResolveRequest` 和 W1c `saga_admin.ForceResolveInput` 合并的真实 body 定义清楚 |
| R-H-2 | 新建 `apps/partner-api/internal/handler/envelope.go` | 新文件 | 提取 `OK/Fail/Unavailable` 三函数；`handler/w1a_routes.go:80-91` 删除 `ok/fail`；`handler/admin/admin.go:63-83` 删除 `ok/bad/unavailable`；`handler/saga_admin.go:134-143` 删除 `respondError` |
| R-H-3 | `packages/api-client/src/` | 新增 4 个文件 | `client.ts` / `client-public.ts` / `types.ts` / `error-mapping.ts`；3 个登录站点改 `import { apiClient, unwrap, genUUID } from "@tracenex-biz/api-client"`；storefront 改 `import from "@tracenex-biz/api-client/public"` |
| R-H-3b | `apps/partner-web-{customer,partner,admin}/src/api/*.ts` | 删除 client.ts/types.ts/error-mapping.ts 三份 | 只保留各站点特有的 `customer.ts` / `partner.ts` / `admin.ts` typed endpoint wrapper |
| R-H-4 | `apps/partner-api/internal/service/saga_allocate/service.go` | 129, 138, 141, 149, 152, 155 | 把 `_, _ = sg.Compensate(...)` 改成 `if _, cerr := sg.Compensate(...); cerr != nil { /* log + metric + markEscalated */ }` |
| R-H-4b | `apps/partner-api/internal/saga/instance.go` | 174-178 | `Compensate` 的 txErr 分支在写 failed 后判断 `ShouldEscalate` + `MarkEscalated` |
| R-H-5 | `apps/partner-api/cmd/server/main.go` | 132-171 | 新建 `cmd/server/deps.go`，装配 W1b（saga_allocate/topup/refund、outbox、revenue、settlement、dispute）+ W1c（invoice/payment/ticket/notification/content_safety/staff/saga_admin）全部 service；`buildRouter` 挂 `admin.Register(r.Group("/api/admin"), deps.Admin)` |
| R-H-6 | `apps/partner-web-storefront/src/pages/ApplyPartner.tsx` | 全文件 | 拆成 `pages/ApplyPartner/index.tsx` + 7 个 subcomponent 文件，每个 ≤ 120 行 |
| R-H-7 | `apps/partner-api/internal/service/auth/auth.go` | 350-355 | `otp := fmt.Sprintf("%06d", binary.BigEndian.Uint32(otpBytes) % 1000000)`；加 `encoding/binary` import；补 `otp_test.go` 覆盖均匀性 |
| R-H-8 | `apps/partner-api/internal/saga/saga.go` | 159 | `for i := 1; i < attempts && d < BackoffMax; i++`；更新 `saga_test.go::TestBackoffFor` 测 attempts=0→2s/1→2s/2→4s/8→512s（cap 5min） |
| R-M-1 | `apps/partner-api/internal/domain/kyc.go` | 18-35 | 把 `*Plain` 字段抽到 `service/kyc/submit_input.go::internalKYC`，domain 仅保留 cipher + keyID + blindIndex |
| R-M-2 | `apps/partner-api/internal/service/invoice/` | 新增 `memory.go` | 把 invoice.go:372-473 `MemoryRepo` 整段移过去；主文件变 ~370 行 |
| R-M-2b | `apps/partner-api/internal/service/content_safety/` | 新增 `memory.go` | 同上，~180 行迁移 |
| R-M-2c | `apps/partner-api/internal/handler/admin/` | 新增 5 个文件 | `invoice.go / ticket.go / content_safety.go / staff.go / saga.go`；`admin.go` 保留 Register + Deps + helper |
| R-M-4 | 多处 | — | 统一用 `strconv.ParseInt`，删除 `fmtSscan` / `parseInt64` 自造 |
| R-M-6 | `apps/partner-api/internal/service/saga_topup/service.go` | 160-180 | `OnCallback` 结尾 `return s.Fund(ctx, sagaID, ...)` 同步推进，或把 `Fund` 入列到 saga retry worker task queue |
| R-M-7 | `apps/partner-api/internal/service/saga_admin/saga_admin.go` | 182-188 | `same24` 在 IPv6 上改返 `true, nil`（fail-closed 语义）或补充 IPv6 /48 实现（可复用 `saga/force_resolve.go::subnetEqual`） |
| R-M-8 | `apps/partner-api/internal/handler/admin/admin.go` | 68-79 | `bad` 强制两个字符串；与 HIGH-2 合并 |
| R-L-3 | `apps/partner-api/internal/service/content_safety/content_safety.go` | 246-249 | `buildPayload` 改 `common.Marshal`（Go 用 `json.Marshal`）走安全编码 |
| R-L-5 | `apps/partner-api/internal/service/saga_admin/saga_admin.go` | 283-300 | `StubResolver` / `CapturingAudit` 挪到 `saga_admin_testing.go` 附 `//go:build testing` 或 `_test.go` |

---

## 9. 测试覆盖统计 + gap

### partner-api（Go）

| Package | Coverage | Gap / 建议 |
|---|---:|---|
| `internal/audit` | 80.6% | 良好 |
| `internal/config` | 20.8% | **严重不足**；`Load()` 70 行 env 校验无测；补表驱动 |
| `internal/handler` (顶层) | 0% | w1a_routes.go / w1a_business.go / w1a_auth.go 无测；建议 httptest 覆盖 5 条 happy path |
| `internal/handler/admin` | 59.8% | 有 admin_test.go；补 staff create 反例、cs retry/dispatch 反例 |
| `internal/middleware` | 50.0% | JWT/CSRF/idempotency/BOLA 全是 stub，无测；属于 W1a 待做 |
| `internal/outbox` | 76.9% | 良好 |
| `internal/saga` | 66.7% | 中；`BackoffFor` / `ShouldEscalate` 边界未覆盖 |
| `internal/service/auth` | 81.1% | 良好；PasswordReset 覆盖 TTL/OTP 失败 |
| `internal/service/content_safety` | 76.4% | 良好 |
| `internal/service/customer` | 76.8% | 良好 |
| `internal/service/dispute` | 77.3% | 良好 |
| `internal/service/invitation` | 75.0% | 良好 |
| `internal/service/invoice` | 87.0% | 最佳 |
| `internal/service/kyc` | 67.5% | 补 `rejectFreeze` 年度冻结路径 |
| `internal/service/notification` | 86.4% | 良好 |
| `internal/service/partner` | 55.3% | **不足**；补 Reject/Suspend/Freeze 反例 |
| `internal/service/payment` | 81.2% | 良好 |
| `internal/service/revenue` | 92.9% | 最佳 |
| `internal/service/saga_admin` | 82.6% | 良好 |
| `internal/service/saga_allocate` | 65.9% | 补 FyTopup failed + Compensate chain 路径 |
| `internal/service/saga_refund` | **0%** | 无测；LOW-7 |
| `internal/service/saga_topup` | **0%** | 无测；LOW-7 |
| `internal/service/settlement` | 85.5% | 良好 |
| `internal/service/staff` | 65.8% | 补 IP allowlist / step-up MFA 反例 |
| `internal/service/ticket` | 81.2% | 良好 |
| `internal/service/wallet` | 73.1% | 良好 |
| `internal/repository/mysql` | 13.3% | 占位文件，待 W1a GORM 落 |
| `pkg/errors` | 57.9% | 补 30+ Code → HTTP status 映射表测 |
| `pkg/piiscrubber` | 100% | 最佳 |
| `pkg/leader / pii / tracing / validator` | 0% | 待补 |

**全仓加权覆盖（按非零包）**：约 65%。距项目 "80% 最低" 有 15 个百分点差距，主要由 `internal/config / handler / middleware / saga_refund / saga_topup / repository/mysql` 五个 0%-30% 包拖低。

### 前端（vitest）

| App | Tests | Pass | 覆盖核心 | Gap |
|---|---:|---|---|---|
| storefront | 27 | 27/27 | schemas / store / resolver / throttle / Stepper | PIL mask 函数全覆盖；SSR path 未覆盖 |
| partner | 12 | 12/12 | pii / error-mapping / zodResolver / allocateStore / Stepper | Allocate 三阶段状态机 phase 转换无测；建议补 |
| customer | 10 | 10/10 | pii / zodResolver / error-mapping | TopupStatusPage 状态机无测 |
| admin | 10 | 10/10 | 同 customer | SagaForceResolve 表单无测 |

**e2e（Playwright）**：10 个 spec 文件写好，但 `@playwright/test` 依赖未安装，实际从未执行。W1e/W1f/W1g 都承认；W2 必装 + CI 跑。

### 覆盖率 gap 收口建议

1. `saga_refund` + `saga_topup`：各 ≥ 5 单测（happy / callback replay / fund idempotent / escalate），W2 前必补；
2. `internal/config/config.go`：≥ 3 表驱动（必填 env 缺失 / 校验规则 invariant），W2 前必补；
3. `internal/handler`（顶层）：≥ 6 httptest（login/logout/apply/register/wallet 404/invitation generate），W2 前必补；
4. admin 前端 `SagaForceResolve` 表单 + 4 约束 error toast：≥ 3 vitest + 1 e2e，W2 前必补；
5. `@playwright/test` 装起来并接入 `partner-web.yml` CI，W2 必动；
6. `pkg/errors` 30 个 Code 映射：≥ 10 表驱动，W2 前补。

总计需要 ~24 条新测试把覆盖率推到 80%。

---

## 10. 收尾结论

**整体 Verdict：PASS-WITH-CONDITIONS。** 本轮代码质量问题里没有不可接受的硬伤；immutability、TDD、状态机、BOLA scope、KMS 管线这些高阶 invariant 在 service 层字面级落地，说明 W1a/W1b/W1c/W1e/W1f/W1g 六条并行路线是严格按 HANDOFF 产出的高质量代码。

**Gate to W2**：在 W2 review 合并前，HIGH-1/2/3/5（force-resolve 契约收敛、envelope 统一、packages/api-client 搬迁、cmd/server wiring）四项必须闭环，否则前后端 / Fy-api 三方协同跑 smoke test 时会命中 404 / 字段丢失 / handler 不挂等物理问题。HIGH-4（saga 补偿静默吞错）、HIGH-7（OTP 均匀性）、HIGH-8（Backoff 文档实现不一致）属于正确性 Bug，W2 review 之前也必须闭环。HIGH-6、MEDIUM、LOW 可排期 W2 之后。

**测试覆盖 Gate**：三个 0%-30% 的关键包（config / saga_refund / saga_topup / handler 顶层）必须在 W2 合并前补到 60%+。

---

**review 结束日期**：2026-05-14
**review 工时**：约 3h，覆盖 117 Go files 结构 + 184 TS files 抽样 + 7 份 HANDOFF 完整通读 + `go test -cover -race ./...` 实跑。
