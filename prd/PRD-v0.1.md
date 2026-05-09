# TraceNex Partner — 产品需求文档（PRD）

> 版本：v0.1（initial draft，2026-05-09）
> 仓库：`/Users/nathan/Projects/apiGateway/TraceNexBiz/`
> 上游需求文档：`/Users/nathan/Projects/apiGateway/needs/大模型融合平台-渠道分销系统需求文档.docx`（v1.2，2026-04-20）
> 关联系统：`Fy-api`（AI 网关层，详见 `/Users/nathan/Projects/apiGateway/Fy-api/`）
> 维护人：Nathan
>
> **此版本为**「待 4 个 agent review 的初稿」**，会被 PM/Architect/Security/Compliance review，迭代到 v1.0。**

---

## 0. 目录

1. 产品定位
2. 关键决策（已拍板）
3. 用户角色
4. 业务场景
5. 系统架构
6. 与 Fy-api 集成边界
7. 功能需求（按模块）
8. 数据模型
9. 计费链路设计（核心难点）
10. 非功能性需求
11. 风险与对策
12. 里程碑（10-12 周）
13. 待业务进一步确认的问题

---

## 1. 产品定位

**TraceNex Partner** 是一个**渠道分销 SaaS**，与 TraceNex 现有 AI 网关（`Fy-api`）解耦，作为独立产品交付。

### 1.1 核心价值

- 让**渠道商**在 TraceNex 上发展自己的客户、自主定价加价销售、收取分润
- 让**终端客户**通过渠道商入驻，获得统一的 AI 网关访问能力
- 让**平台方**（TraceNex 运营方）通过渠道商体系扩展销售半径，节省直销成本

### 1.2 一句话定义

> "把 TraceNex 这个 toC/toB 直销 AI 网关，扩展为支持二级分销代理的 SaaS 平台。"

### 1.3 不做什么（Out of Scope）

- ❌ 不重写 AI 网关——所有计费/路由/Token 管理仍走 Fy-api
- ❌ 不做 LLM 内容审核——按现行法规由用户自负其责（参见合规章节）
- ❌ 不做多级（≥3）分销——本期只做**二级**（平台 → 渠道商 → 终端客户）
- ❌ 不做"招标式比价"或"模型聚合调度优化"——这些是 Fy-api 的核心能力，TraceNex Partner 只做其上的销售和账务层

---

## 2. 关键决策（已拍板）

| 决策项 | 选择 | 备注 |
|---|---|---|
| **仓库目录** | `/Users/nathan/Projects/apiGateway/TraceNexBiz/` | 与 Fy-api 平级 |
| **产品名** | **TraceNex Partner**（中文：TraceNex 渠道伙伴平台） | 品牌延续 |
| **后端技术栈** | **Go**（Gin + GORM v2） | 与 Fy-api 同栈，便于内部 SDK 复用 |
| **前端技术栈** | **React 18 + Vite + Semi UI** | 与 Fy-api/classic 一致 |
| **数据库边界** | **同实例不同库**（推荐方案，详见 §6.3） | 既隔离又方便跨库查询 |
| **结算周期** | **可配置**，默认月结（1 号出账，10 号前结算） | 平台管理员可在系统设置中切换 T+0 / T+7 / 月结 |
| **第一批渠道商** | **招商优先 + 自助入驻并行** | 双流程，PRD 里都设计 |
| **MVP 时间线** | **10-12 周完整商业化上线** | 三期：MVP 4 周 + 商业化 6-8 周 |

---

## 3. 用户角色

```
┌─────────────────────┐
│   平台管理员 Root    │  TraceNex 运营方，全权限
└──────────┬──────────┘
           │ 审核 / 充值 / 调控
           ▼
┌─────────────────────┐
│     渠道商 Partner   │  分销代理，可发展下级、自主加价
└──────────┬──────────┘
           │ 邀请 / 分配额度 / 私有定价
           ▼
┌─────────────────────┐
│   终端客户 Customer  │  实际调用 API 的最终用户
└─────────────────────┘
```

| 角色 | 关键能力 | 角色字段 |
|---|---|---|
| **平台管理员**（Root） | 全平台 CRUD；审核渠道商；调控订阅/充值/分润 | `User.role = ROOT(100)` 在 Fy-api 侧；本仓库自建 `staff` 表区分平台运营、客服、财务 |
| **渠道商**（Partner） | 邀请客户；分配额度；私有定价；查看分润 | TraceNexBiz 自建 `partner` 表，关联 Fy-api `User.id` |
| **终端客户**（Customer） | 调用 API；查看自己的用量、账单 | TraceNexBiz 自建 `customer` 表，关联 Fy-api `User.id`；`customer.partner_id` 标记归属 |
| **个人直销用户** | 不通过渠道商，直接和平台签约（即 TraceNex 现有用户） | 仅在 Fy-api，TraceNexBiz 不做新增 UI |

> **重要**：渠道商和终端客户在 Fy-api 侧都是普通 `User`；TraceNexBiz 在它之上加一层"业务身份"。这层映射是 §6 集成边界的核心。

---

## 4. 业务场景（关键 User Journey）

### 4.1 场景 A：渠道商招商入驻（人工招商流）

平台运营 → 沟通获客（线下渠道经理推进）→ 渠道商联系平台 → 平台 staff 在管理后台创建渠道商账号（含初始额度、分润比例、专属邀请码）→ 渠道商收到激活邮件 → 登录后台开始拉客户

### 4.2 场景 B：渠道商自助入驻（线上自助流）

注册 TraceNex 账号 → 在个人后台点击"申请成为渠道商" → 选择"企业 / 个人"类型 → 上传认证资料（营业执照 / 支付宝实名）→ 提交审核 → 平台审核（人工 24h 内）→ 审核通过/驳回 → 通过后自动获得渠道商权限 + 默认分润比例 + 邀请码

### 4.3 场景 C：终端客户被邀请入驻

渠道商在自己的后台生成邀请码（或邀请链接） → 客户点击链接注册 → 自动绑定到该渠道商 → 客户在自己后台看到的余额/可用模型/价格全是渠道商私有配置 → 客户调 API → 计费按渠道商加价后金额扣 → 同时给渠道商记一笔成本

### 4.4 场景 D：渠道商充值客户

客户余额不足 → 渠道商在自己后台找到该客户 → 点击"充值" → 输入金额 → 从**渠道商钱包**扣 → 给**客户额度**加 → 双方都生成账单流水 → 强一致事务

### 4.5 场景 E：客户调 API 计费

客户用 API Key 调 `/v1/chat/completions` → Fy-api 路由到上游 → 计费链路触发 → **渠道商私有 group_ratio** 应用（关键：B 方案）→ 客户扣量 = 平台批发价 × 渠道商加价倍率 → 同时记录 `partner_revenue_log`（渠道商应得分润）

### 4.6 场景 F：月度结算

每月 1 号凌晨 02:00（Cron）→ 计算上月每个渠道商的：
- 客户总消费（按零售价）= 渠道商收入
- 平台批发成本 = 渠道商成本
- 分润 = 收入 − 成本

→ 生成账单 PDF + Excel → 邮件通知渠道商 → 渠道商在后台查看/下载 → 平台管理员标记"已结算" → 财务转账分润金额 → 标记"已支付"

### 4.7 场景 G：发票申请

客户/渠道商申请发票 → 选择抬头（个人/企业）→ 上传营业执照（首次企业）→ 填写税号、邮寄地址 → 提交 → 平台财务在后台审核 → 通过后线下开票（财务系统对接是 v1.2 的事，MVP 期人工开票）→ 邮寄/电子发票发给申请人

---

## 5. 系统架构

### 5.1 总体两层

```
┌──────────────────────────────────────────────────────────────┐
│                     用户/客户/渠道商浏览器                     │
└──────────┬───────────────────────────────────┬───────────────┘
           │                                   │
           │ 域名 partner.tracenex.cn          │ 域名 api.aitracenex.com
           │ （TraceNex Partner 前端）          │ （Fy-api OpenAI 兼容接口）
           ▼                                   ▼
┌──────────────────────────┐       ┌──────────────────────────┐
│    TraceNex Partner       │ HTTP │      Fy-api               │
│  - 渠道商后台 / 招商商城    │◄────►│  - AI 网关核心             │
│  - 终端客户后台           │ 内部API│  - Token / Channel /      │
│  - 平台管理后台扩展         │       │    Logs / 计费             │
│  - 财务/分润/发票          │       │                          │
└────┬─────────────────────┘       └──────┬───────────────────┘
     │                                    │
     │ 同 MySQL 实例不同库                 │
     ▼                                    ▼
┌──────────────────┐           ┌──────────────────────────────┐
│ tracenex_biz_db  │           │       transnext_db           │
│ - partner        │           │  - users / tokens / channels │
│ - customer       │           │  - logs / channels / topup   │
│ - partner_pricing│           │  - subscription / redemption │
│ - revenue_log    │           │                              │
│ - settlement     │           │                              │
│ - kyc_application│           │                              │
│ - invoice        │           │                              │
└──────────────────┘           └──────────────────────────────┘
```

### 5.2 关键设计原则

1. **TraceNex Partner 不持有 Fy-api 的 schema 写权限**——`users`、`tokens`、`logs` 这些表 TraceNexBiz **只读**，写入通过 Fy-api 的内部 API
2. **TraceNex Partner 持有的业务表自治**——`partner`、`customer`、`partner_pricing`、`revenue_log`、`settlement`、`invoice`、`kyc_application` 等业务表完全在 `tracenex_biz_db`
3. **跨库 join** 在报表场景必要时直接用 SQL `JOIN tracenex_biz_db.partner JOIN transnext_db.users`（同实例不同库支持）；事务场景禁止跨库
4. **资金事务全在 TraceNexBiz 侧**——渠道商钱包扣减 + 客户额度增加是分布式事务，用"先扣渠道商 → 调 Fy-api 给客户加 → 失败回滚渠道商"的 saga 模式（详见 §9）

---

## 6. 与 Fy-api 集成边界

这是这个系统的关键决策。我把所有交互点列出来，agent 后续可以挨个 review。

### 6.1 Fy-api 提供给 TraceNex Partner 的内部 API

| 接口 | 用途 | 调用方 | 调用频次 |
|---|---|---|---|
| `POST /api/internal/user/create` | 创建客户/渠道商账号（绕开公开注册流） | Partner backend | 每次客户入驻 |
| `POST /api/internal/user/topup` | 给指定 user 加额度 | Partner backend | 渠道商充值客户 / 平台直充 |
| `POST /api/internal/user/deduct` | 给指定 user 扣额度（异常修正） | Partner backend | 退款 / 对账修正 |
| `POST /api/internal/token/create` | 给指定 user 创建 API Token | Partner backend | 客户首次入驻 / 申请新 Token |
| `PUT /api/internal/user/group` | 修改 user 的 group | Partner backend | 渠道商私有定价生效路径（关键，§9） |
| `GET /api/internal/usage/by-user` | 查询某 user 在某时间段的消费日志 | Partner backend | 月度结算 / 实时账单 |
| `GET /api/internal/usage/by-group` | 查询某 group 的总用量 | Partner backend | 渠道商分润计算 |
| `POST /api/internal/group` | 创建/更新自定义 group_ratio | Partner backend | 渠道商私有定价生效路径 |

**鉴权**：内部 API 必须用**双因素**：
- 静态 API Key（在 Fy-api 配置里，TraceNexBiz 启动时加载）
- 时间戳 + HMAC 签名（防重放）

### 6.2 TraceNex Partner 反向通知 Fy-api

无。TraceNexBiz 是消费方，Fy-api 不需要回调它。

### 6.3 数据库边界

**最终方案：同 MySQL 实例不同库**

| 候选方案 | 利 | 弊 | 评分 |
|---|---|---|---|
| **方案 1：完全独立 MySQL 实例** | 物理隔离最彻底；故障域分离；DBA 操作不互相影响 | 跨业务报表必须走 ETL；运维成本翻倍；事务无法跨库 | 🟡 |
| **方案 2：同实例不同库** | 跨库报表 SELECT 自由；运维一份配置；备份策略复用；事务跨库需 XA（不推荐） | 故障域共享（实例宕机两边都炸） | ✅ **推荐** |
| **方案 3：完全同库** | 简单 | 业务表名空间冲突；上游同步 Fy-api 时风险大；schema 演化纠缠 | ❌ |

**为什么选方案 2**：

1. **运维成本可控**：CN/SG 的 RDS 实例已经在跑，加一个 database 是几行 SQL，不用申请新实例
2. **跨库报表很常用**：「列出渠道商 X 在 4 月份所有客户的消费明细」需要 join `tracenex_biz_db.customer` 和 `transnext_db.logs`，同实例零成本
3. **故障域共享**风险可以接受：Fy-api 和 TraceNexBiz 业务上本来就是绑定的，Fy-api 挂了 TraceNexBiz 也没事可做
4. **写入边界严格**：TraceNexBiz 应用层不直接写 `transnext_db` 的表（只读），写入全走 Fy-api 内部 API。这条规则代码层面强制（GORM 用两个 connection，`transnext_db` 的连接配置成只读）

### 6.4 用户身份映射

| Fy-api `User` 视角 | TraceNexBiz 视角 |
|---|---|
| 一个 `User`（普通账号） | 在 TraceNexBiz 中要么是 `customer`，要么是 `partner`，要么都不是（直销用户） |
| `User.id` 是主键 | 在 `customer` 表里作为外键 `fy_user_id` |
| `User.group` 决定计费倍率 | 渠道商的私有定价正是通过修改 `User.group` 实现（§9） |

**身份升级规则**：
- 普通客户 → 申请成为渠道商 → 审核通过 → TraceNexBiz 给该 user 创建 `partner` 行 + 新建一个专属 `aff_code`
- 普通客户 → 通过渠道商邀请码注册 → TraceNexBiz 给该 user 创建 `customer` 行 + `customer.partner_id = X`

---

## 7. 功能需求（按模块）

> 编号沿用上游需求文档的章节号（§3.x），优先级 **P0 必做 / P1 重要 / P2 增强**。
> ★ 标记为"涉及与 Fy-api 集成"。

### 7.1 模块一：公开商城 / 招商落地页

| ID | 功能 | 优先级 | 实现要点 | ★ |
|---|---|:---:|---|:---:|
| M1-01 | 模型展示（分类浏览） | P0 | 复用 Fy-api 的 `/api/pricing` 接口拉模型列表，前端按 modality 分类 | ★ |
| M1-02 | 模型详情页 | P0 | 单独路由 `/models/:id`，显示能力描述、定价、调用示例 | |
| M1-03 | 模型对比 | P1 | 多选模型，对比表格 | |
| M1-04 | 用户注册/登录 | P0 | 复用 Fy-api 的 `/api/user/register` `/api/user/login`；TraceNexBiz 不重做账号体系 | ★ |
| M1-05 | 申请成为渠道商落地页 | P0 | 静态营销页 + CTA 跳转到"渠道商申请表单" | |
| M1-06 | 渠道商申请表单 | P0 | 分企业/个人两 tab，含必填字段、文件上传 | ★ |
| M1-07 | 在线购买（套餐/按量充值） | P0 | 复用 Fy-api `/api/subscription/plan` `/api/topup` | ★ |
| M1-08 | 试用额度 | P1 | 注册自动赠送 $0.50 quota（Fy-api 已支持 `QuotaForNewUser`） | ★ |
| M1-09 | 多语言（中/英） | P1 | i18next | |

### 7.2 模块二：个人用户后台（终端客户视角）

| ID | 功能 | 优先级 | 实现要点 | ★ |
|---|---|:---:|---|:---:|
| M2-01 | 仪表盘 | P0 | 复用 Fy-api `/api/user/dashboard` | ★ |
| M2-02 | 余额查询 | P0 | 显示从 Fy-api 拉的额度 + TraceNexBiz 计算的"由 X 渠道商提供" | ★ |
| M2-03 | 充值（线上 EPay/原生微信支付宝） | P0 | EPay 复用 Fy-api；原生支付走 TraceNexBiz 自建（§7.6） | |
| M2-04 | 充值（线下转账） | P0 | 用户上传银行回单 → 平台审核 → Fy-api 内部 API 加额度 | ★ |
| M2-05 | API Key 管理 | P0 | 复用 Fy-api `/api/token`，前端嵌套 | ★ |
| M2-06 | **席位管理**（License 模式） | P0 | TraceNexBiz 自建 `seat` 表，每席位绑定一个 Token；席位购买/扩容/续费 | ★ |
| M2-07 | 模型配置（可用模型/切换/申请开通） | P0 | 复用 Fy-api `/api/user/models`；申请开通走 TraceNexBiz 工单流 | ★ |
| M2-08 | 账单中心（消费明细 / 月度汇总 / 充值记录） | P0 | 跨库 join `transnext_db.logs` + `tracenex_biz_db.partner_pricing` | ★ |
| M2-09 | **CSV/PDF 导出** | P0 | 复用 Fy-api 的 CSV 导出（今天刚修复 `d60e0eed0`），TraceNexBiz 加 PDF 导出 | ★ |
| M2-10 | 发票申请 | P1 | 业务表 `invoice` + 工单流 + 邮件抄送财务 | |
| M2-11 | 角色切换（客户/渠道商） | P0 | 前端 Layout 顶部下拉，根据 `partner.id IS NOT NULL` 显示 | |
| M2-12 | 认证中心（企业/个人） | P0 | 业务表 `kyc_application` + OSS 上传 + OCR + 审核流（§7.7） | |

### 7.3 模块三：渠道商后台 ❗

> 这是核心增量模块，全部 P0。

| ID | 功能 | 优先级 | 实现要点 |
|---|---|:---:|---|
| M3-01 | 渠道商仪表盘（收益/客户数/用量） | P0 | 实时聚合 `revenue_log` + `customer` |
| M3-02 | 邀请客户（生成邀请码 + 链接 + 二维码） | P0 | 业务表 `invitation_code`，状态机 |
| M3-03 | 客户列表 + 客户详情 | P0 | 主列表 + drawer 详情 |
| M3-04 | 客户额度分配 | P0 | 从渠道商钱包扣 → Fy-api 给客户加，事务（§9） |
| M3-05 | 客户充值 | P0 | 同上 |
| M3-06 | 客户 License 分配（席位） | P0 | TraceNexBiz `seat.partner_id = X.id, customer_id = Y.id` |
| M3-07 | 移除/禁用客户 | P1 | 软删除，调 Fy-api 禁用 user |
| M3-08 | **加价销售（私有定价）** | P0 | **B 方案 group_ratio**，详见 §9 |
| M3-09 | 渠道商钱包（余额/充值/记录） | P0 | 业务表 `partner_wallet` + `partner_topup` |
| M3-10 | 消费账单 | P0 | 渠道商角度看：自己花了多少（成本价）+ 客户付了多少（零售价）+ 分润 |
| M3-11 | 客户额度上限设置 | P1 | 渠道商在客户详情页设置 |
| M3-12 | 渠道商成本价查看（透明化） | P1 | 给渠道商显示平台给的批发价，方便他定价 |

### 7.4 模块四：平台管理后台扩展

| ID | 功能 | 优先级 | 实现要点 |
|---|---|:---:|---|
| M4-01 | 渠道商列表/详情 | P0 | TraceNexBiz `partner` 表 |
| M4-02 | **渠道商审核**（待审核/通过/驳回/重审） | P0 | 状态机 + 邮件通知 + 操作日志 |
| M4-03 | KYC 审核（营业执照/法人身份证/支付宝） | P0 | OSS 私有桶预览 + OCR 结果展示 + 审核动作 |
| M4-04 | 渠道商额度管理（增减、冻结） | P0 | 操作 `partner_wallet` |
| M4-05 | 渠道商充值（手动/线上） | P0 | 同 M2 充值，归属到渠道商 |
| M4-06 | 渠道商旗下客户列表 | P0 | drill-down 视图 |
| M4-07 | 全部客户列表 + 客户归属查看 | P0 | filter by `customer.partner_id` |
| M4-08 | 模型管理 + 上游渠道管理 | P0 | 🟢 **复用 Fy-api 现有管理后台**，TraceNexBiz 不重做 |
| M4-09 | 套餐管理（订阅/Token/混合） | P0 | 复用 Fy-api `SubscriptionPlan`，TraceNexBiz 加 License 套餐类型 |
| M4-10 | 平台收入统计（dashboard） | P0 | 跨库聚合查询 |
| M4-11 | **分润报表 + 结算批次** | P0 | 业务表 `settlement`，Cron 月初触发，详见 §7.5 |
| M4-12 | 充值/退款管理 | P0 | 业务表 `topup` + `refund` |
| M4-13 | 发票管理（审核/开票/邮寄） | P1 | 业务表 `invoice` |
| M4-14 | 系统设置（结算周期、默认分润比例、平台参数） | P0 | 业务表 `system_setting` |
| M4-15 | 操作日志（who did what when） | P0 | 业务表 `audit_log`，重要写操作必记 |

### 7.5 模块五：分润结算系统（独立子系统）

| ID | 功能 | 优先级 | 实现要点 |
|---|---|:---:|---|
| M5-01 | 月结 Cron（默认 1 号 02:00） | P0 | Go 内置 robfig/cron，跨实例使用 distributed lock 防重复 |
| M5-02 | 周结 Cron（可选） | P1 | 同上 |
| M5-03 | T+0 实时结算（可选） | P2 | 客户每次扣费时同步触发分润记录 |
| M5-04 | 结算批次生成 | P0 | 一次结算 = 一行 `settlement`，里面是该批次所有渠道商的分润金额 |
| M5-05 | 渠道商账单生成（PDF + Excel） | P0 | 用 Go template + maroto 库生成 PDF |
| M5-06 | 邮件通知 | P0 | SMTP via Fy-api 共享配置 |
| M5-07 | 渠道商账单查看/下载 | P0 | 在渠道商后台 `/billing` |
| M5-08 | 平台审批 + 转账记录 | P0 | 平台财务在管理后台标记"已支付"，写入支付凭证 |
| M5-09 | 争议处理（客户主张账单错误） | P1 | 业务表 `billing_dispute`，状态机 |

### 7.6 模块六：支付系统

| ID | 功能 | 优先级 | 实现要点 |
|---|---|:---:|---|
| M6-01 | 微信支付（V3 API，扫码） | P0 | 商户号资质 → SDK → 回调 webhook → 加额度 |
| M6-02 | 支付宝（PC/H5/扫码） | P0 | 同上 |
| M6-03 | EPay（聚合，过渡用） | P0 | 复用 Fy-api 现有 |
| M6-04 | Stripe（海外） | P1 | 复用 Fy-api 现有 |
| M6-05 | 线下转账（用户上传回单） | P0 | OSS 上传 + 平台审核 |
| M6-06 | 退款流程 | P1 | 微信/支付宝 V3 退款 API + 工单 |
| M6-07 | 防重 + 幂等 | P0 | 订单号 idempotency key，回调去重表 |

### 7.7 模块七：实名认证子系统（KYC）

| ID | 功能 | 优先级 | 实现要点 |
|---|---|:---:|---|
| M7-01 | 企业认证（营业执照 OCR） | P0 | 阿里云 OCR API；图片只能上传到私有 OSS bucket；7 天后自动删原图保留 OCR 结果 |
| M7-02 | 个人实名（支付宝芝麻认证） | P0 | 支付宝开放平台「人脸认证」H5 流；前端跳过去 → 回调 |
| M7-03 | 法人身份证（企业必填） | P0 | OCR + 校验姓名一致 |
| M7-04 | 审核流（待审/通过/驳回） | P0 | 业务表 `kyc_application` + 审核动作 |
| M7-05 | 重审 | P0 | 驳回后允许重新上传 |
| M7-06 | 信息加密存储 | P0 | 身份证号 / 支付宝实名等 PII 字段 AES-GCM 加密；密钥放 KMS |
| M7-07 | 30 天后自动清原图 | P0 | OSS lifecycle rule + Cron |

### 7.8 模块八：发票子系统

| ID | 功能 | 优先级 | 实现要点 |
|---|---|:---:|---|
| M8-01 | 抬头管理（个人/企业） | P1 | 业务表 `invoice_title`（一个用户可有多个） |
| M8-02 | 申请发票 | P1 | 业务表 `invoice_application` |
| M8-03 | 平台财务审核 | P1 | 后台动作 |
| M8-04 | 开票（v1.x 人工，v2 对接税控） | P1 | MVP 期人工 |
| M8-05 | 邮寄/电子发票发送 | P1 | 发邮件附件 |

---

## 8. 数据模型

> 表结构按 GORM 风格描述。所有表加 `created_at` `updated_at` `deleted_at`（软删除）。

### 8.1 渠道商表 `partner`

```go
type Partner struct {
    ID                  int64
    FyUserId            int64     // ★ 关联 Fy-api users.id
    InvitationCode      string    // 唯一邀请码（aff_code 的 partner 版）
    Status              int8      // 0=待审核 1=正常 2=禁用 3=驳回
    KycType             int8      // 0=未认证 1=企业 2=个人
    KycStatus           int8      // 0=未提交 1=待审核 2=通过 3=驳回
    DefaultRevenueShare float64   // 默认分润比例 0.0-1.0（平台给渠道商的分润比例，低于"加价销售"模式时使用）
    Tier                int8      // 渠道商等级（v1.1+）
    AppliedAt           time.Time
    ApprovedAt          *time.Time
    ApprovedBy          *int64    // staff.id
    ContactName         string
    ContactPhone        string
    ContactEmail        string
    Notes               string    // 内部备注
}
```

### 8.2 终端客户表 `customer`

```go
type Customer struct {
    ID                  int64
    FyUserId            int64     // ★ 关联 Fy-api users.id
    PartnerId           int64     // ← 归属渠道商
    JoinedVia           string    // 'invitation' / 'manual_create' / 'self_register_with_code'
    InvitationCodeUsed  string    // 注册时使用的邀请码
    Status              int8      // 0=正常 1=禁用 2=已移除
    GroupNameInFyApi    string    // ★ 这个客户在 Fy-api 中所属的 group（B 方案的关键）
    QuotaLimit          int64     // 渠道商对该客户的额度上限（0=不限）
}
```

### 8.3 渠道商钱包 `partner_wallet`

```go
type PartnerWallet struct {
    ID         int64
    PartnerId  int64    UNIQUE
    Balance    int64    // ★ 单位：quota（与 Fy-api 一致），不要用 float
    LockedBalance int64 // 冻结余额（结算中、争议中）
    Version    int64    // 乐观锁
}
```

### 8.4 钱包流水 `partner_wallet_log`

```go
type PartnerWalletLog struct {
    ID            int64
    PartnerId     int64
    Type          string  // 'topup' / 'allocate_to_customer' / 'settlement_payout' / 'refund' / 'adjustment'
    Amount        int64   // 正数为收入，负数为支出
    BalanceAfter  int64   // 操作后余额
    RefId         string  // 关联订单 / 客户 ID / 结算批次 ID
    Note          string
    OperatorType  string  // 'partner' / 'system' / 'platform_staff'
    OperatorId    int64
}
```

### 8.5 渠道商私有定价 `partner_pricing`

```go
type PartnerPricing struct {
    ID         int64
    PartnerId  int64
    GroupName  string   // 这个 group 对应 Fy-api 的 group_ratio 配置
    Markup     float64  // 加价倍率（1.20 = 加 20%）
    Note       string
    EffectiveAt time.Time  // 生效时间
}
```

### 8.6 收益记录 `revenue_log`

```go
type RevenueLog struct {
    ID            int64
    PartnerId     int64
    CustomerId    int64
    FyApiLogId    int64    // ★ 关联 Fy-api logs.id（计费链路触发时写入）
    GrossAmount   int64    // 客户付出的总额（按渠道商零售价）
    CostAmount    int64    // 平台成本（批发价）
    NetRevenue    int64    // 渠道商分润 = Gross - Cost
    OccurredAt    time.Time
    SettlementId  *int64   // 已结算批次 ID（NULL = 未结算）
}
```

### 8.7 结算批次 `settlement`

```go
type Settlement struct {
    ID              int64
    Period          string   // 'monthly_2026_05'
    PeriodStart     time.Time
    PeriodEnd       time.Time
    TotalRevenue    int64
    TotalCost       int64
    TotalPayout     int64
    Status          string   // 'generating' / 'pending_payment' / 'paid' / 'partially_disputed'
    GeneratedAt     time.Time
    PaidAt          *time.Time
    PaidBy          *int64
    PaymentEvidence string
}
```

### 8.8 结算明细 `settlement_item`

```go
type SettlementItem struct {
    ID             int64
    SettlementId   int64
    PartnerId      int64
    Revenue        int64
    Cost           int64
    Payout         int64
    Status         string  // 'pending' / 'paid' / 'disputed'
    InvoiceId      *int64
}
```

### 8.9 KYC 申请 `kyc_application`

```go
type KycApplication struct {
    ID                  int64
    FyUserId            int64
    Type                int8    // 1=企业 2=个人
    Status              int8    // 0=待审核 1=通过 2=驳回
    BusinessLicenseUrl  string  // 营业执照 OSS 私有 URL
    BusinessLicenseOcr  string  // OCR 提取结果（JSON）
    LegalPersonName     string  // AES-GCM 加密
    LegalPersonIdNo     string  // AES-GCM 加密
    LegalPersonIdUrl    string  // 身份证 OSS 私有 URL，30 天后自动删
    AlipayOpenId        string  // 加密
    AlipayRealName      string  // 加密
    SubmittedAt         time.Time
    ReviewedAt          *time.Time
    ReviewedBy          *int64
    RejectReason        string
    PiiPurgedAt         *time.Time  // PII 清理时间
}
```

### 8.10 邀请码 `invitation_code`

```go
type InvitationCode struct {
    ID           int64
    PartnerId    int64
    Code         string  UNIQUE
    Type         string  // 'permanent' / 'one_time' / 'limited'
    UsageLimit   int     // 0=不限
    UsedCount    int
    ExpiresAt    *time.Time
    Status       string  // 'active' / 'expired' / 'revoked'
}
```

### 8.11 席位 `seat`

```go
type Seat struct {
    ID                int64
    OwnerType         string   // 'partner' / 'customer'（席位归属类型）
    OwnerId           int64
    Name              string   // 席位名（如 "张三"）
    FyTokenId         int64    // ★ 关联 Fy-api tokens.id
    PurchasedAt       time.Time
    ExpiresAt         time.Time
    Status            string   // 'active' / 'expired' / 'disabled'
}
```

### 8.12 发票 `invoice_application`

```go
type InvoiceApplication struct {
    ID            int64
    ApplicantType string   // 'partner' / 'customer'
    ApplicantId   int64
    TitleType     int8     // 1=个人 2=企业
    Title         string
    TaxNumber     string   // 企业税号
    Amount        int64
    Period        string   // 月份字符串
    Status        string   // 'pending' / 'approved' / 'issued' / 'mailed' / 'rejected'
    InvoiceUrl    string   // 电子发票 PDF
    MailAddress   string
    AppliedAt     time.Time
    IssuedAt      *time.Time
    Notes         string
}
```

### 8.13 操作日志 `audit_log`

```go
type AuditLog struct {
    ID         int64
    ActorType  string   // 'staff' / 'partner' / 'customer' / 'system'
    ActorId    int64
    Action     string   // 'partner.approve' / 'kyc.reject' / 'wallet.adjust' / ...
    TargetType string
    TargetId   int64
    DiffJson   string   // 修改前后差异（重要写操作必记）
    IpAddress  string
    UserAgent  string
    OccurredAt time.Time
}
```

### 8.14 平台 staff `staff`

```go
type Staff struct {
    ID          int64
    Username    string
    PasswordHash string
    Role        string   // 'super_admin' / 'operations' / 'finance' / 'support'
    Email       string
    Status      string
    LastLogin   *time.Time
    Mfa         string   // TOTP secret，加密
}
```

### 8.15 系统配置 `biz_setting`

```go
type BizSetting struct {
    Key         string PRIMARY KEY
    Value       string
    UpdatedAt   time.Time
    UpdatedBy   int64
}
// 关键 keys:
//   settlement.period         = 'monthly' | 'weekly' | 't+0' | 't+7'
//   settlement.day_of_month   = '1'
//   settlement.cutoff_hour    = '02'
//   default_revenue_share     = '0.20'
//   pii_purge_days            = '30'
```

---

## 9. 计费链路设计（核心难点）

> 这是整个系统**风险最高**的一段。下面给详细方案，agent 重点 review。

### 9.1 问题陈述

需求：渠道商 X 给客户 Y 设置加价倍率 1.30（加 30%）。客户 Y 调用 GPT-4o，平台批发价 $0.005/1k tokens。
- 实际给客户 Y 扣的钱：$0.005 × 1.30 = $0.0065
- 平台从 Y 收的钱：$0.0065
- 渠道商 X 的成本：$0.005
- 渠道商 X 的收益：$0.0015

### 9.2 方案 B：用 Fy-api 现有的 group_ratio 实现

**Fy-api 现有逻辑**：
```
最终倍率 = model_ratio × group_ratio × completion_ratio
```

**TraceNex Partner 的做法**：
1. 渠道商 X 创建时，在 Fy-api 自动建一个名为 `partner_X` 的 group，配置 group_ratio = 1.0
2. 渠道商 X 给客户 Y 创建时，在 Fy-api 自动建一个名为 `partner_X_customer_Y` 的 group，配置 group_ratio = 1.30
3. 把客户 Y 的 `User.group` 设为 `partner_X_customer_Y`
4. 客户 Y 调 API 时，Fy-api 自然按这个 group_ratio 计费 = 平台批发价 × 1.30

**优点**：
- ✅ 不动 Fy-api 一行代码
- ✅ 计费精度沿用 Fy-api 的（已验证）
- ✅ 调价立刻生效（改 group_ratio 即可）

**风险与缓解**：

| 风险 | 缓解 |
|---|---|
| group 数量爆炸（1 渠道商 1000 客户 = 1000 group） | Fy-api 的 group 是字符串，只在请求路径上做 hash 比较，不影响性能；MySQL 字符串索引能扛 10 万级 |
| 渠道商批量改价格要遍历客户 | 接受这个限制，提供"按渠道商批量改"按钮 |
| 客户跨渠道商迁移时 group 要改 | 可以接受，迁移本来就是低频操作 |

### 9.3 计费链路完整流程

```
客户 Y 调 /v1/chat/completions
       │
       ▼
Fy-api distributor 中间件 → 选 channel
       │
       ▼
relay 到上游 → 拿响应
       │
       ▼
text_quota.go 计费
  group_ratio = lookup("partner_X_customer_Y") = 1.30
  amount = base × model_ratio × 1.30 × ...
       │
       ▼
扣 user.quota → 写 logs (user_id=Y, quota=amount, group="partner_X_customer_Y")
       │
       ▼  ← 这里是关键 hook 点
       │
       │  方案 B-1：实时（侵入 Fy-api，不推荐）
       │    在 RecordConsumeLog 之后调一次 TraceNexBiz 内部 webhook
       │
       │  方案 B-2：离线（推荐）
       │    Fy-api 不改一行；TraceNexBiz 用 binlog CDC（canal/maxwell）订阅 logs 表
       │    新增日志 → 解析 group 字段 → 反推 partner_id + customer_id
       │    → 写入 revenue_log（cost=base价, gross=按 1.30 价, net=差额）
       ▼
TraceNexBiz revenue_log 落库
```

**推荐方案 B-2（离线 CDC）**：

| 维度 | B-1 实时 webhook | B-2 离线 CDC |
|---|---|---|
| 侵入 Fy-api | 是 | 否 |
| 延迟 | 实时 | 1-3 秒（binlog 重放速度） |
| 失败重试 | 复杂 | binlog 自然有 |
| 性能影响 | 每次 API 调用多一次 HTTP | 0 |
| 上游同步压力 | 每月 sync 都要解决冲突 | 0 |

延迟 1-3 秒对"渠道商仪表盘实时性"足够（用户感知不到差别）。

### 9.4 退款 / 调整链路

退款逻辑：
1. 渠道商或平台发起退款
2. TraceNexBiz 在 `revenue_log` 写一条负数补偿日志
3. 调 Fy-api `/api/internal/user/topup` 给客户加回额度
4. 调用 partner_wallet 减回渠道商应分润

**关键**：所有调整必须有审计日志（who、when、why、how much），且必须能从 `revenue_log` 倒推出最终的渠道商钱包余额。这是对账的根本。

---

## 10. 非功能性需求

| 类别 | 指标 | 备注 |
|---|---|---|
| **性能** | 后台 API P95 < 500ms（不含 Fy-api 调用时间） | |
| | 列表页首屏 < 2s | |
| | 月结 Cron 跑完时间 < 30 min（10 渠道商 × 1000 客户级别） | |
| **并发** | TraceNexBiz 自身后台 ≥ 200 QPS | API 计费走 Fy-api，不算它 |
| **可用性** | 99.5%（MVP）→ 99.9%（v1.2） | 业务系统而非热路径，可弱于 Fy-api |
| **数据持久性** | RDS 自动备份每日 + 月结后单独 dump | |
| **事务一致性** | 钱包扣减必须 ACID；跨服务用 saga | |
| **安全** | OWASP Top 10 全部缓解 | 见 §11 |
| | 所有 PII（身份证号、手机号、银行卡）字段必须加密存储 | AES-GCM, KMS 管 key |
| | 越权检测：每个查询接口必须有自动化测试覆盖"渠道商只能看到自己客户" | |
| | 操作日志保留 ≥ 6 月 | |
| **可观测性** | Prometheus 指标：QPS / 错误率 / 钱包余额一致性 / 月结成功率 | |
| | 结构化日志：JSON 格式，关联 trace_id | |
| **国际化** | 中文 + 英文界面 | 渠道商可能涉及海外 |
| **可配置性** | 结算周期、默认分润比例、试用额度等运营参数 全部走 `biz_setting` | |
| **可维护性** | Go 后端单元测试覆盖率 ≥ 60%；前端核心流程 ≥ 50% | |

---

## 11. 风险与对策

| 风险 | 等级 | 对策 |
|---|---|---|
| **数据归属越权（渠道商越权查别家客户）** | 🔴 极高 | 所有查询走 `RepositoryWithPartnerScope` 中间件；每个 endpoint 必须有越权测试用例 |
| **钱包资金事务一致性** | 🔴 极高 | 严格事务 + 乐观锁；跨服务用 saga + 补偿；定时对账 Job 跑差异告警 |
| **渠道商 group_ratio 改价时计费瞬间不一致** | 🟡 中 | 接受秒级窗口不一致（计费日志会落到调价后的 group），需在产品上明确告诉渠道商"调价 1 分钟内仍可能按旧价计算" |
| **PII 数据泄露（KYC 资料）** | 🔴 极高 | 私有 OSS bucket + 上传/下载需鉴权 + 加密存储 + 30 天自动清原图；KMS 管密钥；审核员账号 MFA |
| **微信/支付宝资质周期不可控** | 🟡 中 | 早 2-3 周启动资质申请，与开发并行 |
| **结算 Cron 重复触发（多实例）** | 🟡 中 | distributed lock（Redis SETNX 或 MySQL `GET_LOCK`） |
| **CDC 链路丢消息** | 🟡 中 | binlog 持久化 7 天；TraceNexBiz 维护 ack offset；月底对账 Job 校验 `revenue_log.amount SUM` 与 Fy-api `logs.quota SUM` 偏差 |
| **争议处理流程缺失导致客户投诉** | 🟡 中 | M5-09 设计争议工单流；客服后台 |
| **渠道商私有 group 数量过多影响 Fy-api 性能** | 🟢 低 | 监控 group 表大小，超过 10 万行时考虑迁移到独立 group_ratio 缓存层 |
| **税务合规（个人渠道商劳务报酬税）** | 🔴 极高 | 详见 §13 待业务确认 |
| **未成年人/无完全民事行为能力人开渠道商** | 🟡 中 | KYC 校验：身份证号读取出生年月，必须 ≥18 岁 |
| **跨境数据流动（SG 站点）** | 🟡 中 | KYC 数据按 user 国别分库；中国用户的 KYC 在 CN 库；其他 SG 库 |

---

## 12. 里程碑（10-12 周完整商业化上线）

### Phase 1 — MVP 内测可用（4 周）

**目标**：几家种子渠道商可以**端到端走完一遍**——招进来、邀请客户、客户调 API、看到账单。**不含**自助审核、原生支付、加价销售。

**范围**：
- M1-04（注册登录）、M1-06（渠道商申请表单）
- M2-05（API Key）、M2-08（账单中心，无加价）
- M3-01（仪表盘）、M3-02（邀请客户）、M3-03（客户列表）、M3-04（额度分配 - **从平台直拨给客户，绕过钱包**）
- M4-01/M4-02（人工审核渠道商）、M4-04/M4-05（人工充值）、M4-06/M4-07（视图）
- 数据模型：`partner` `customer` `invitation_code` `audit_log` `staff`
- 集成：Fy-api 内部 API 鉴权 + create user/token + topup
- **暂不做**：钱包系统、加价销售、CDC、月结、原生支付、KYC、发票

**验收**：
- 平台 staff 可创建渠道商账号
- 渠道商可登录后台并生成邀请码
- 客户用邀请码注册成功，自动绑定到渠道商
- 客户调 API 计费成功（按平台默认价）
- 渠道商在自己后台看到旗下客户列表 + 客户用量

### Phase 2 — 商业化框架（4 周）

**目标**：**真正的"分销系统"** —— 钱包、加价销售、KYC、原生支付、CDC、月结。

**范围**：
- M2-12（KYC 认证）、M7 全部（OCR + 审核 + 加密存储）
- M3-04/M3-05/M3-09（钱包扣减客户充值）
- M3-08（加价销售）+ §9 计费链路：
  - 自动给每个 partner+customer 创建 group
  - 客户 user.group 设置生效路径
  - **CDC 链路**（binlog → revenue_log）
- M5-01/M5-04/M5-05/M5-06/M5-07（月结 Cron + 账单生成 + 邮件通知）
- M6-01/M6-02（原生微信/支付宝）
- M4-11（分润报表）

**外部并行**：微信支付/支付宝商户号资质审核（2-3 周，与开发并行）

**验收**：
- 渠道商完成 KYC 后自动激活
- 渠道商设置加价 30%，客户调 API 计费按 30% 加成
- 月初 Cron 自动跑出 4 月结算批次，渠道商收到邮件
- 渠道商可用微信/支付宝原生支付给自己钱包充值

### Phase 3 — 企业级补完（2-4 周）

**目标**：发票、席位、客户额度上限等。

**范围**：
- M2-06（席位）+ M3-06、M8 全部（发票）、M4-13（发票管理）、M5-08/M5-09（结算审批 + 争议）、M2-10（个人发票申请）

**验收**：
- 客户可申请发票，财务可审核 + 开票 + 邮寄
- 渠道商可购买/分配席位

### 关键时间锚点

| 时间 | 里程碑 |
|---|---|
| Week 0（今天） | PRD v1.0 定稿，仓库 init，技术选型确认 |
| Week 1-4 | Phase 1 开发 |
| Week 4 末 | Phase 1 内测，3-5 家种子渠道商灰度 |
| Week 5-8 | Phase 2 开发（与微信支付资质并行） |
| Week 8 末 | Phase 2 上线，正式商业化 |
| Week 9-12 | Phase 3 开发 + 上线 |

---

## 13. 待业务进一步确认的问题

下面这些问题**会影响产品设计**，需要业务/产品/法务/财务在 PRD v1.0 之前给出明确答案：

| # | 问题 | 影响范围 |
|---|---|---|
| Q1 | 默认渠道商分润比例多少？是否所有渠道商一致？ | 产品定价、§7.4 M4-14 |
| Q2 | 第一批种子渠道商的目标行业？（决定 KYC 严格度和合同模板） | §4.1 招商话术 |
| Q3 | 个人渠道商是否要扣**劳务报酬个税**？平台代扣还是渠道商自报？ | 重大合规问题，§11 风险 |
| Q4 | 客户调 API 的内容是否要做敏感信息检测/审核？由谁负责违规内容？ | 合规、§1.3 不做什么 |
| Q5 | 海外客户（SG 站点）的支付如何处理？是否走 Stripe？分润是否要换汇？ | §6.4 国际化 |
| Q6 | 退款规则：客户充值后多久内可全额退款？渠道商分润是否要回收？ | §9.4 退款链路 |
| Q7 | 发票：是否对接航天信息/百望等税控系统？v1.x 人工开票上限多少？ | §7.8 |
| Q8 | 若渠道商跑路（账户被冻结），其旗下客户如何处置？ | §11 风险 |
| Q9 | 渠道商之间能否相互"挖墙脚"（一个渠道商接受另一个渠道商的客户）？ | §3 角色规则 |
| Q10 | 平台收取技术服务费的方式：从客户支付里抽成 vs 从渠道商分润里抽成？ | 计费模型，§9 |

---

## 14. 附：与现有 Fy-api 的接口契约（待 v1.0 详化）

> 这是给 Fy-api 团队的修改清单。MVP 阶段需要 Fy-api 至少新增以下内部 API：

```http
POST /api/internal/user
POST /api/internal/user/:id/topup
POST /api/internal/user/:id/group
POST /api/internal/token
GET  /api/internal/user/:id/usage?from=&to=
GET  /api/internal/group/:name/usage?from=&to=
POST /api/internal/group           # 创建/更新自定义 group_ratio
```

鉴权方案、错误码、响应 schema 在 v1.0 详细化。

---

> 本文档为 v0.1 初稿。下一步：4 个 agent（PM / Architect / Security / Compliance）独立 review，反馈 → v0.2 → 二轮 review → v1.0。
