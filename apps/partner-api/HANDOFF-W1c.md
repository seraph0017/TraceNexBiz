# HANDOFF-W1c — partner-api 业务 2

**日期**：2026-05-12 **scope**：支付 / 发票 / 工单 / 通知 / 内容安全+12377 / staff / saga force-resolve / 审计 sealer。

## 交付

- `internal/service/{payment,invoice,ticket,notification,content_safety,staff,saga_admin}/` — service + memory repo + ≥5 单测，TDD。
- `internal/audit/sealer.go` 重写：`Store`/`LeaderLock` 接口 + `SealOnce` + `Verify`（增量、抓篡改）+ `MemoryStore`。
- `cmd/audit-sealer` / `cmd/audit-verify` 入口；GORM Store 待 W1a。
- `internal/handler/admin/` 10 个 endpoint：invoice review/issue/red-flush、ticket list/assign、12377 list/retry/dispatch、staff create、saga force-resolve。
- `openapi/admin.yaml` admin 契约（W1g orval 入口）。

## 不变量

immutable `Update(updater)` 模式；BOLA scope 强制；销售方主体 service 注入；红冲仅 issued；12377 24h SLA + retry + dead_letter；force-resolve 五约束（不同人 / 不同 /24 / 一次性 token / 30min cooldown / audit）；hash chain `prev || sha256(canonicalize)`。

## 验收

`go test -race` W1c 9 包全绿；`go vet` 无警；admin endpoints 10 个集成测试通过。pre-existing 失败：W1b 的 `service/partner` `service/saga_allocate`（与 W1c 无关）。

## 待 W1a

JWT/CSRF/IP allowlist middleware；GORM Repo/Store；biz_setting 模板 + 9 备案号；cron 入口（12377.retry / audit.sealer Lease）；Audit middleware unsealed enqueue。
