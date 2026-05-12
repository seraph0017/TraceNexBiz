# TraceNexBiz 测试环境阿里云清单

> 目标：用最低可用成本跑起 TraceNexBiz partner-api + 4 个前端，并能和 Fy-api 测试环境完成 `/api/internal/*` HMAC、outbox、MNS、KYC 文件上传、审计日志联调。
>
> 推荐部署形态：单 VPC、单地域、Podman + Nginx + 阿里云托管数据面。测试环境先不上 ACK。

## 一、低成本规格

| 资源 | 最低可用 | 推荐低配 | 说明 |
|------|----------|----------|------|
| ECS | 复用 Fy-api 测试机 `2c4g` | 独立或复用 `4c8g` | 如果 Fy-api 和 TraceNexBiz 同机跑，建议 `4c8g`；`2c4g` 只适合 smoke/QA 小流量 |
| EIP | 1-5 Mbps 按量 | 5 Mbps 按量 | QA 访问够用，压测再临时升带宽 |
| RDS MySQL 8.0 | 1c2g / 40G | 1c2g 或 2c4g / 50-100G | 可与 Fy-api 测试库同实例不同 DB |
| Redis/Tair | 256MB | 512MB | partner-api 的幂等、JWT revocation、leader、webhook idempotency 都依赖 Redis |
| OSS | 标准存储，私有 Bucket | 2 个私有 Bucket | KYC/PII 热桶 + 冷归档桶 |
| MNS | 按量队列 | 3 个队列 | `in` / `out` / `dlq` |
| SLS | 可选，7 天保留 | 7-15 天保留 | 联调 trace_id 和排查 outbox/MNS 时建议开启 |
| KMS | 最小配置 | 最小配置 | 测试环境也跑真实密钥链路，避免 staging 前才暴露问题 |
| ACR | 私有仓库 | 私有仓库 | 后端和前端镜像都必须打明确版本 tag |

省钱原则：

- 前端不一定都做容器。可以 `pnpm build` 后由 Nginx 直接托管静态文件。
- Fy-api 与 TraceNexBiz 可先同 ECS 部署，`partner-api -> Fy-api` 走 `http://127.0.0.1:<fy-api-port>`。
- SLS 保留期先设 7 天；OSS 生命周期先按测试数据短周期清理。

## 二、RDS 数据库与账号

建议一个测试 RDS 实例内建 3 个库：

| DB | 用途 |
|----|------|
| `fy_api_test` | Fy-api 主库，TraceNexBiz 只读查询用户/额度等信息 |
| `fy_api_log_test` | Fy-api LOG_DB / consume_log_outbox；partner-api outbox poller 读取 |
| `partner_db_test` | TraceNexBiz partner-api 主库 |

账号建议：

| 账号 | 权限 |
|------|------|
| `fy_api_app` | `fy_api_test` 读写 |
| `tnbiz_app` | `partner_db_test` 读写；`fy_api_test` 只读 |
| `tnbiz_outbox_consumer` | `fy_api_log_test` outbox 相关表读取/更新 |
| `tnbiz_migrator` | `partner_db_test` DDL；只在迁移时使用 |

RDS 白名单只放 ECS 内网 IP 或 vSwitch CIDR。不要把 `0.0.0.0/0` 加进测试 RDS。

## 三、MNS 队列

测试环境建 3 个队列：

| 队列 | 环境变量 |
|------|----------|
| `tnbiz-test-partner-in` | `MNS_QUEUE_IN` |
| `tnbiz-test-partner-out` | `MNS_QUEUE_OUT` |
| `tnbiz-test-partner-dlq` | `MNS_QUEUE_DLQ` |

建议参数：

```env
OUTBOX_BACKEND=aliyun_mns
DATA_REGION=cn
MNS_LONG_POLL_SEC=20
MNS_VISIBILITY_TIMEOUT_SEC=30
MNS_DLQ_THRESHOLD=10
```

handler 规则：未接真实依赖的 MNS 事件必须 fail-loud，不要为了 ack 消息假成功。

## 四、OSS / KMS / SLS

OSS：

| Bucket | 用途 |
|--------|------|
| `tnbiz-test-pii` | KYC 文件、身份证、营业执照等热数据 |
| `tnbiz-test-archive` | 冷归档、审计留存、导出文件 |

Bucket 全部私有，前端上传必须走 partner-api 颁发的 presigned URL。测试环境可先不接 CDN。

KMS：

- 建一个测试 KEK，例如 `tnbiz-test-kek`。
- HMAC secret、JWT verify key、PII envelope 相关 secret 统一放测试命名空间。
- 测试环境可以用最小配置，但不要继续用 dev `noop`/stub 路径。

SLS：

| Logstore | 用途 |
|----------|------|
| `partner-api` | partner-api 应用日志 |
| `fy-api` | Fy-api 应用日志；可与 Fy-api 测试环境共享同一 Project |
| `nginx-access` | Nginx access log |
| `nginx-error` | Nginx error log |

建议同一个 SLS Project 内放 Fy-api 和 TraceNexBiz 的 logstore，便于按 `trace_id` 跨服务查询。

## 五、关键环境变量

partner-api 测试环境示例：

```env
ENV=prod
HTTP_ADDR=:8080

DB_BIZ_DSN=tnbiz_app:***@tcp(rm-xxx.mysql.rds.aliyuncs.com:3306)/partner_db_test?parseTime=true&charset=utf8mb4&loc=Local
DB_FY_RO_DSN=tnbiz_app:***@tcp(rm-xxx.mysql.rds.aliyuncs.com:3306)/fy_api_test?parseTime=true&charset=utf8mb4&loc=Local
DB_LOG_DSN=tnbiz_outbox_consumer:***@tcp(rm-xxx.mysql.rds.aliyuncs.com:3306)/fy_api_log_test?parseTime=true&charset=utf8mb4&loc=Local
DB_MIGRATOR_DSN=tnbiz_migrator:***@tcp(rm-xxx.mysql.rds.aliyuncs.com:3306)/partner_db_test?parseTime=true&charset=utf8mb4&loc=Local

REDIS_ADDR=r-xxx.redis.rds.aliyuncs.com:6379
REDIS_PASSWORD=***
REDIS_DB=0

FYAPI_BASE_URL=http://127.0.0.1:3001
FYAPI_HMAC_KEY_ID=tnbiz-test
FYAPI_HMAC_SECRET=***

OUTBOX_BACKEND=aliyun_mns
MNS_ENDPOINT=https://<accountId>.mns.cn-hangzhou.aliyuncs.com
MNS_ACCESS_KEY_ID=***
MNS_ACCESS_KEY_SECRET=***
MNS_QUEUE_IN=tnbiz-test-partner-in
MNS_QUEUE_OUT=tnbiz-test-partner-out
MNS_QUEUE_DLQ=tnbiz-test-partner-dlq
DATA_REGION=cn

OSS_ENDPOINT=oss-cn-hangzhou-internal.aliyuncs.com
OSS_BUCKET=tnbiz-test-pii
OSS_REGION=cn-hangzhou

KMS_ENDPOINT=***
KMS_KEY_ID=tnbiz-test-kek
KMS_REGION=cn-hangzhou
ALIBABA_ACCESS_KEY=***
ALIBABA_ACCESS_SECRET=***

SLS_ENDPOINT=cn-hangzhou.log.aliyuncs.com
SLS_PROJECT=tnbiz-test
SLS_LOGSTORE=partner-api

JWT_VERIFY_KEY_PEM=***
ALLOWED_ORIGINS=https://test-store.example.com,https://test-customer.example.com,https://test-partner.example.com,https://test-admin.example.com
```

## 六、镜像与版本 tag

镜像仓库建议：

| 仓库 | 镜像 |
|------|------|
| `tracenexbiz/partner-api` | partner-api |
| `tracenexbiz/partner-web-storefront` | 公开商城 |
| `tracenexbiz/partner-web-customer` | 客户端 |
| `tracenexbiz/partner-web-partner` | 渠道商端 |
| `tracenexbiz/partner-web-admin` | 管理端 |

所有测试/发版镜像必须打明确版本 tag，格式为 `x.x.x-tracenex`。不要只推 `latest`。

示例：

```bash
VERSION=1.2.1-tracenex
podman build -t registry-vpc.cn-hangzhou.aliyuncs.com/tracenexbiz/partner-api:${VERSION} apps/partner-api
podman push registry-vpc.cn-hangzhou.aliyuncs.com/tracenexbiz/partner-api:${VERSION}
```

## 七、验收清单

- [ ] `curl http://127.0.0.1:8080/healthz` 返回 200。
- [ ] partner-api 能连 `partner_db_test`、`fy_api_test`、`fy_api_log_test`。
- [ ] Redis `PING` 正常，JWT revocation / idempotency key 可写。
- [ ] Fy-api `/api/internal/*` HMAC parity 测试通过。
- [ ] MNS 三队列可收发，错误消息能进入 DLQ。
- [ ] OSS presigned upload 能成功 PUT，Bucket 不公开。
- [ ] SLS 能按同一 `trace_id` 查到 partner-api 与 Fy-api 日志。
- [ ] 所有镜像都有 `x.x.x-tracenex` tag。
