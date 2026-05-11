# HANDOFF-W1e — partner-web-storefront 前端

**日期**：2026-05-12
**承接**：W1f（partner web）/ W1g（customer + admin web）—— 学共享模式
**前置**：HANDOFF-W0.md / HANDOFF-W1a.md / HANDOFF-W1c.md / docs/frontend-design.md v1.2

> 本文档作用：W1e 已实现的页面 / 接口契约 / 共享模式，W1f/W1g 接手时可直接复用。

---

## 1. 已交付

### 1.1 路由（per frontend §3.1）

| Path | Page | 说明 |
|---|---|---|
| `/` | `Home` | hero + 4 feature cards + CTA → /apply-partner |
| `/models` | `Models` | 拉 `/api/public/models` 渲染表格 + `icp_license_active=false` 时显示招商内测 banner |
| `/pricing` | `Pricing` | 计费规则三卡（按 token / 渠道加价 / 月结发票）|
| `/apply-partner` | `ApplyPartner` | 5 步表单：联系人 → 主体 → 业务规模 → KYC → 单独同意 + 提交 |
| `/legal/:doc` | `Legal` | 9 个 doc slug 白名单；安全 markdown 子集渲染（无 dangerouslySetInnerHTML）|
| `*` | `NotFound` | meta=noindex |

旧路由 `/partner-apply` → 301 → `/apply-partner`。`/legal` → `/legal/privacy`。

### 1.2 文件结构（src/ 共 ~2,900 LOC TS/TSX）

```
src/
├── api/                         本地 API 客户端（与 packages/api-client 同形）
│   ├── client.ts                axios + 自动 Idempotency-Key / X-Csrf-Token / X-Oneapi-Request-Id
│   ├── error-mapping.ts         错误码 → toast i18n key 表
│   ├── public.ts                5 个公开 endpoint 类型 + 函数
│   ├── types.ts                 ApiEnvelope / ApiError / ApiException
│   └── index.ts                 re-export 入口
├── components/
│   ├── ComplianceFooter.tsx     9 备案号渲染（来自 useComplianceFooter）
│   ├── ErrorBoundary.tsx        顶层错误边界
│   ├── KycUploader.tsx          OSS direct upload (presigned PUT) + magic-byte 由后端二次校验
│   ├── Layout.tsx               Nav + Outlet + ComplianceFooter
│   ├── NavBar.tsx               含 skip-to-main + lang switch
│   ├── Toast.tsx                轻量 toast，aria-live=polite
│   └── apply/
│       ├── ConsentBox.tsx       PIPL 单独同意 fieldset；勾选时调 /api/public/consent 落库
│       ├── Field.tsx            受控 Field（text / textarea），aria-describedby + 错误聚合
│       └── Stepper.tsx          视觉步骤指示器
├── hooks/
│   ├── useComplianceFooter.ts   TanStack Query 拉 footer
│   ├── useSeo.ts                document.title + meta + canonical 注入（无 react-helmet 依赖）
│   └── useThrottledSubmit.ts    客户端节流（默认 1500ms cooldown）+ in-flight lock
├── i18n/
│   ├── index.ts                 i18next init；SUPPORTED zh-CN/en-US；localStorage detect
│   └── locales/{zh-CN,en-US}.json   ≥ 100 条文案（含 errors / footer / apply）
├── lib/
│   └── zodResolver.ts           react-hook-form 极简 zod resolver（替代 @hookform/resolvers，免新增依赖）
├── pages/                        Home / Models / Pricing / ApplyPartner / Legal / NotFound
├── schemas/
│   └── applyPartner.ts          ContactStep / CompanyStep / ScaleStep / KycStep / Consent zod schema
│                                + maskIdCard / maskPhone（PII 脱敏，仅 review 步骤用）
├── stores/
│   └── applyDraft.ts            Zustand persist；partialize 仅持久化非敏感字段
├── App.tsx                       路由树
└── main.tsx                      bootstrap：initI18n → render
```

`public/sitemap.xml` + `robots.txt` + `favicon.svg`。

### 1.3 测试

- **vitest**：6 个 test 文件 / **27 个 test，全绿**
  - `schemas/applyPartner.test.ts` ×15（表单校验 + PII mask）
  - `api/error-mapping.test.ts` ×3
  - `stores/applyDraft.test.ts` ×2（**immutability** 验证）
  - `hooks/useThrottledSubmit.test.tsx` ×3（重复点击 / 错误状态 / reset）
  - `components/apply/Stepper.test.tsx` ×2（aria-current / 已完成 ✓）
  - `lib/zodResolver.test.ts` ×2
- **Playwright**：`e2e/apply.spec.ts`（首页可达 + 招商完整流程，含 KYC 上传 mock）+ `playwright.config.ts`
  - **未运行**：W0 未引 `@playwright/test` 依赖。CI 接入时 `pnpm add -D @playwright/test && pnpm exec playwright install chromium` 即可。
  - 文件作为可运行规范，e2e 路径已通过 vitest exclude 隔离。

### 1.4 验收

| 验收 | 状态 | 输出 |
|---|---|---|
| `tsc --noEmit` | ✅ 0 错 0 警 |  |
| `vite build` | ✅ |  |
| `vitest run` | ✅ 27/27 PASS | 0.7s |
| 6 路由全可达 | ✅ | / · /models · /pricing · /apply-partner · /legal/:doc · 404 |
| **bundle 预算** | ✅ | initial gz ≈ **133 KB**（react 53 + index 33 + form 22 + i18n 16 + query 9）远低于 250 KB；最大单 chunk react 53 KB < 100 KB |

---

## 2. 给 W1f / W1g 复用的模式

### 2.1 API client 模式（**重点**）

storefront 没有引 `packages/api-client`，因为该 workspace package 没有作为依赖被装到 `apps/`（`pnpm install` 未跑过 lockfile，避免 W0 锁死版本）。我在 `src/api/` **照搬**了 `packages/api-client` 的契约：

- `apiClient`（axios 实例）：interceptors 注入 `X-Oneapi-Request-Id`（trace）+ 写操作 `X-Csrf-Token`（cookie 双提交）+ `Idempotency-Key`（UUID）
- `unwrap<T>(promise)`：解包 envelope `{success, data, error}`；非 success 抛 `ApiException`
- `mapApiError(err)` → `{ i18nKey, severity }` toast 表（已枚举 15 个错误码）

**给 W1f / W1g**：
- 优先 `pnpm install` 后用 `@tnbiz/api-client`。如不能装，照抄 `apps/partner-web-storefront/src/api/{client,types,error-mapping}.ts`，保持 header / envelope 契约一致即可（**契约 = 真理**）。
- toast 错误码表已覆盖 storefront 用到的 15 个；W1f/W1g 增条目时**只 append、不修改已有 key**，避免 i18n 漂移。

### 2.2 zod schema 集中放 `src/schemas/`

每个表单一份 file；每步独立 schema + 一个总 `XxxDraftSchema` 用于持久化。zod 错误 `message` 直接写 i18n key（`validation.required` / `apply.consent.required`），由 `Field.tsx::translateError` 在渲染时翻译。**不要**在 schema 里写中文，会破坏 en-US。

### 2.3 Zustand store + immutable

`stores/applyDraft.ts` 是模板：
- `create<State>()(persist(...))`：导出 `interface State`，selector 处显式 `(s: State) => s.x` 避免 verbatim 推断陷阱
- `patchDraft` 用 `{ ...s.draft, ...patch }`，**不**在 set 内改原对象
- `partialize`：**敏感字段不入 localStorage**（身份证 / 上传 URL / consent_id）

### 2.4 ComplianceFooter 引用方式

**不**直接 import `packages/ui-kit/ComplianceFooter`（同样原因：workspace 未装依赖）。每个站点提供本地 wrapper：
- `useComplianceFooter()` 拉 `/api/public/biz_setting/footer`（TanStack Query, 5 min staleTime）
- 视觉部分内联到本地 `<ComplianceFooter />`

W1c 还没产 `/api/public/biz_setting/footer` endpoint —— 当前 query fail 会让 footer 显示全部缺省值（占位空格），不会让首屏崩。**W1c 出 endpoint 后无需改前端**。

### 2.5 i18n / SEO / CSP 模板

- **i18n**：`initI18n()` 在 `main.tsx` await 后再 render；切语言 `setLocale("en-US")` 写 localStorage
- **SEO**：每个 page 调 `useSeo({ title, description, canonical, robots })`；卸载时回滚
- **CSP**：`index.html` `<meta http-equiv="CSP">` 已 strict（`script-src 'self'`，禁 inline）；如 W1f 接 admin 站需要 inline style for chart，可在该站点单独放宽

---

## 3. 我新增到 packages/ 的共享组件

**没有**。所有新组件都放在 `apps/partner-web-storefront/`。原因：
1. `pnpm install` 未跑过，`packages/*` 未装到任何 app，跨 workspace import 失败
2. 共享代码改动需要触发 W1f/W1g 同步，串行执行的当下风险大于收益

**建议**：W1f/W1g 接手前先 `pnpm install`（以 W0 lockfile 为准），把以下三个组件**提**到 `packages/ui-kit`：
- `Stepper`（多步表单通用）
- `Field`（受控 Form Field）
- `Toast`（aria-live polite）

提的方式：把当前文件原样复制到 `packages/ui-kit/src/`，导出加到 `index.ts`，然后 storefront / partner / customer / admin 改用 `import from '@tnbiz/ui-kit'`。当前 storefront 的写法**已经是该模式 ready 的**，差一个搬运动作。

---

## 4. 担心 review 的点

1. **绕过共享 packages**：上面 §2.1 解释过 —— 契约一致，但物理上是两份代码。W2 review 时若要求合并，搬运是 PR 级别工作量。
2. **API endpoint 部分 W1c 还没产**：`/api/public/models` `/api/public/biz_setting/footer` `/api/public/legal/:doc` `/api/public/consent` `/api/public/kyc/presign` `/api/public/partner/apply` —— 我按 frontend §3.1 / W1a HANDOFF §4.2 推断了字段名，与 `internal/handler/w1a_business.go::partnerApplyBody` 不完全对齐（W1a 没有 `company_name` / `unified_social_credit_code` / `expected_monthly_calls` / `expected_use_case` / `source_channel` 字段）。W1c 出 OpenAPI 后需要：
   - 要么 service 层接受这些 optional 业务字段
   - 要么 storefront 改成只传 `type / contact_name / contact_phone / contact_email / consent_id` 五元组，业务规模信息存到独立 `partner_business_profile` 工单
   建议前者 —— 更贴 PRD §4.1。
3. **Zod resolver 自实现**：替代 `@hookform/resolvers/zod` 是为了不新增依赖；功能上只覆盖单层 path，nested object 表单要重写。当前 5 步表单都是 flat，足够。W1f/W1g 如果有复杂嵌套（如 admin saga form），改成正式依赖即可。
4. **Playwright 没有真跑**：`@playwright/test` 未装，spec 写好但没在 CI 验证 selector。W2 review 阶段我可以补一次 install + run。
5. **Semi UI 完全没用**：storefront 的设计强度不需要重组件库；移除 manualChunks 让 CSS 不被 pull。W1f/W1g 用 Semi 后会拉回 ~64 KB CSS，请把 Semi 加回 manualChunks 单独切 chunk。
6. **PII 安全**：`maskIdCard` / `maskPhone` 已实装，`apply.review` 步骤展示也用脱敏；身份证 / 上传 URL **不进 localStorage**（partialize 已剥离）。但 react-hook-form 的 `defaultValues` 在中断恢复时仍是空（KYC / consent 内存态）—— 用户中断后必须重传，这是合规设计而非 bug。

---

## 5. 联调 cheatsheet

```bash
cd apps/partner-web-storefront
./node_modules/.bin/vite --port 5173
# 浏览器访问 http://localhost:5173
# proxy: /api/* → http://localhost:8080（partner-api dev）

# 单独跑测试
./node_modules/.bin/vitest run
./node_modules/.bin/tsc --noEmit
./node_modules/.bin/vite build && ./node_modules/.bin/vite preview --port 5173
```

W1c JWT middleware 接入前，所有 cookie 操作写空字符串，`X-Csrf-Token` 不会 block 公共 endpoint。
