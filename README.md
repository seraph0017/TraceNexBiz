# TraceNexBiz / TraceNex Partner

> 渠道分销 SaaS — 在 TraceNex 现有 AI 网关（Fy-api）之上提供二级分销代理能力的独立产品。

**当前状态**：v1.0 PRD 已定稿（2026-05-09），Phase 1 工程开工窗口已开。

## 目录结构

```
TraceNexBiz/
├── README.md           本文件
├── prd/
│   ├── PRD-v0.1.md     初稿
│   ├── PRD-v0.2.md     基于 Round-1 review 重写
│   └── PRD-v1.0.md     ★ 当前定稿
├── reviews/
│   ├── round-1/        v0.1 的四方 review（PM / Architect / Security / Compliance）
│   └── round-2/        v0.2 的四方 review（最终通过）
└── docs/               (Phase 1 期间产出的工程文档)
```

## 关键事实

| 项 | 值 |
|---|---|
| 产品代号 | **TraceNex Partner**（仓库名 TraceNexBiz） |
| 后端 | Go + Gin + GORM v2 |
| 前端 | React 18 + Vite + Semi UI |
| 与 Fy-api 关系 | 独立部署；通过 `/api/internal/*`（覆盖层）+ 同实例不同 DB 集成 |
| 资金清算 | 持牌分账方托管（去二清） |
| 时间线 | 10-12 周完整商业化（Phase 1 / 2A / 2B / 3）|

## Phase 1 启动前必须答的 BLOCK 问题

参见 `prd/PRD-v1.0.md` §13：
- Q11.1-4 ICP 经营许可证四项前置条件
- Q12 持牌分账方选哪家
- Q13 DPO 由谁担任
- Q14 算法备案文本（详见附录 E）
- Q16 渠道商合作协议模板

## Phase 2 hard-gate（合规）

参见 §22.3：ICP 证、生成式 AI 备案、持牌分账上线、个税代扣、全电发票、PIA 报告、等保 2.0 二级、DPO 公示、内容安全闭环——**任意一项不达标，Phase 2 商业化不能上线**。
