// Package config 加载 env + biz_setting 热加载。
//
// 引用：backend §13.1 env 表 + §3.15 biz_setting + ADR-007 v0.2 JWT pubkey 来源。
//
// 加载顺序（per ADR D-3）：
//  1. 启动期从 env / KMS Secret Manager 注入硬性 secret（DB DSN / JWT pubkey / KMS key id）
//  2. 启动期 SELECT * FROM biz_setting 装入内存 map（5-15s polling 热加载）
//  3. config.MustLoadAndValidate：缺 key / 类型不符则 panic
//
// W0：env loader + invariant validate；biz_setting 热加载在 W1c 落地。
package config

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

// Env 枚举。
const (
	EnvDev     = "dev"
	EnvStaging = "staging"
	EnvProd    = "prod"
)

// Config 是顶层配置；运行期不可 mutate（immutability rule）。
//
// W1 agent 修改时请走 update 模式：返回新 *Config 实例，不要 in-place 改。
type Config struct {
	Env        string // dev / staging / prod
	HTTP       HTTPConfig
	DB         DBConfig
	Redis      RedisConfig
	JWT        JWTConfig
	KMS        KMSConfig
	OSS        OSSConfig
	SLS        SLSConfig
	MNS        MNSConfig
	FyAPI      FyAPIConfig
	BizSetting *BizSettingConfig

	// 运行期 invariant（per ADR D-3 / SEC CRIT-7）；
	// 启动期 validate：SagaWallClock ≤ IdempotencyTTL ≤ InternalIdempotencyTTL
	IdempotencyTTL         time.Duration
	SagaWallClock          time.Duration
	InternalIdempotencyTTL time.Duration

	// AllowedOrigins CORS / CSRF Origin allowlist（W1c：从 biz_setting.cors_origins 加载）
	AllowedOrigins []string
}

// HTTPConfig 服务监听 + body limit。
type HTTPConfig struct {
	Addr           string
	TrustedProxies []string
	BodyLimitBytes int64
}

// DBConfig 三类 GORM DSN（per ADR-005 / backend §14.2）。
type DBConfig struct {
	BizDSN          string // tnbiz_app to partner_db (RW)
	FyReadOnlyDSN   string // tnbiz_app to fy_api_db (read-only via GRANT)
	LogDSN          string // tnbiz_outbox_consumer to fy_api_db (LOG_DB)
	MigratorDSN     string // tnbiz_migrator (DDL only)
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// RedisConfig go-redis/v8 配置。
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// JWTConfig JWT 校验配置（per ADR-007 v0.2：公钥从 KMS Secret Manager 注入）。
type JWTConfig struct {
	VerifyKeyPEM string
	SignKeyPEM   string
	KeyID        string
	Issuer       string
	Audience     string
}

// KMSConfig 阿里云 KMS（per ADR-009）。
type KMSConfig struct {
	Endpoint     string
	KeyID        string
	Region       string
	AccessKey    string
	AccessSecret string
}

// OSSConfig 阿里云 OSS（per ADR-017 v0.2 强 PresignPut 约束）。
type OSSConfig struct {
	Endpoint     string
	Bucket       string
	Region       string
	AccessKey    string
	AccessSecret string
	PresignTTL   time.Duration
}

// SLSConfig 阿里云 SLS。
type SLSConfig struct {
	Endpoint string
	Project  string
	Logstore string
}

// MNSConfig 阿里云 MNS（Fix-B' part 3 CRIT-B5）.
//
// SOURCE: partner-api 把本地 outbox 行发到 QueueOut（事件由其它服务消费）.
// SINK:   partner-api 长轮询 QueueIn（消费其它服务投递的事件，如 Fy-api 流向 partner-api）.
//
// 默认 backend：APP_ENV=prod 时必须 aliyun_mns；缺 Endpoint/AccessKey → fail-fast.
type MNSConfig struct {
	Backend              string // "aliyun_mns" | "memstub"；默认 prod=aliyun_mns / dev=memstub
	Endpoint             string // https://{accountId}.mns.{region}.aliyuncs.com
	AccessKeyID          string
	AccessKeySecret      string
	QueueIn              string // SINK 长轮询队列
	QueueOut             string // SOURCE 发布队列
	QueueDLQ             string // DLQ 队列名
	DataRegion           string // cn / sg, stamped on each MNS message
	VisibilityTimeoutSec int    // ChangeVisibility 默认；MNS console 也可配
	LongPollSec          int    // ReceiveMessage waitseconds；MNS 最大 30
	DLQThreshold         int    // dequeue_count 达此值 → MoveToDLQ
}

// FyAPIConfig Fy-api 内部客户端配置。
type FyAPIConfig struct {
	BaseURL    string
	HMACKeyID  string
	HMACSecret string
	Timeout    time.Duration
}

// BizSettingConfig 内存视图（W1c：5-15s polling 刷新）。
type BizSettingConfig struct {
	mu      sync.RWMutex
	values  map[string]string
	updated time.Time
}

// NewBizSettingConfig 默认空 map。
func NewBizSettingConfig() *BizSettingConfig {
	return &BizSettingConfig{values: map[string]string{}}
}

// Load 从 env 加载启动期配置。
//
// 返回错误而不是 panic，由 caller 决定降级策略。
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Env: getenv("ENV", EnvDev),
		HTTP: HTTPConfig{
			Addr:           getenv("HTTP_ADDR", ":8080"),
			BodyLimitBytes: getenvInt64("HTTP_BODY_LIMIT", 10*1024*1024),
		},
		DB: DBConfig{
			BizDSN:          getenv("DB_BIZ_DSN", "tnbiz_app:tnbiz_app@tcp(127.0.0.1:3306)/partner_db?parseTime=true&charset=utf8mb4&loc=Local"),
			FyReadOnlyDSN:   getenv("DB_FY_RO_DSN", ""),
			LogDSN:          getenv("DB_LOG_DSN", ""),
			MigratorDSN:     getenv("DB_MIGRATOR_DSN", ""),
			MaxOpenConns:    int(getenvInt64("DB_MAX_OPEN", 100)),
			MaxIdleConns:    int(getenvInt64("DB_MAX_IDLE", 10)),
			ConnMaxLifetime: time.Duration(getenvInt64("DB_CONN_MAX_LIFE_SEC", 600)) * time.Second,
		},
		Redis: RedisConfig{
			Addr:     getenv("REDIS_ADDR", "127.0.0.1:6379"),
			Password: getenv("REDIS_PASSWORD", ""),
			DB:       int(getenvInt64("REDIS_DB", 0)),
		},
		JWT: JWTConfig{
			VerifyKeyPEM: getenvFileOrValue("JWT_VERIFY_KEY_FILE", "JWT_VERIFY_KEY_PEM", ""),
			SignKeyPEM:   getenvFileOrValue("JWT_SIGN_KEY_FILE", "JWT_SIGN_KEY_PEM", ""),
			KeyID:        getenv("JWT_KEY_ID", ""),
			Issuer:       getenv("JWT_ISSUER", "tracenex-biz"),
			Audience:     getenv("JWT_AUDIENCE", "tracenex-biz"),
		},
		KMS: KMSConfig{
			Endpoint:     getenv("KMS_ENDPOINT", ""),
			KeyID:        getenv("KMS_KEY_ID", ""),
			Region:       getenv("KMS_REGION", "cn-hangzhou"),
			AccessKey:    getenv("ALIBABA_ACCESS_KEY", ""),
			AccessSecret: getenv("ALIBABA_ACCESS_SECRET", ""),
		},
		OSS: OSSConfig{
			Endpoint:     getenv("OSS_ENDPOINT", "http://127.0.0.1:4566"),
			Bucket:       getenv("OSS_BUCKET", "tnbiz-dev"),
			Region:       getenv("OSS_REGION", "cn-hangzhou"),
			AccessKey:    getenv("ALIBABA_ACCESS_KEY", ""),
			AccessSecret: getenv("ALIBABA_ACCESS_SECRET", ""),
			PresignTTL:   time.Duration(getenvInt64("OSS_PRESIGN_TTL_SEC", 300)) * time.Second,
		},
		SLS: SLSConfig{
			Endpoint: getenv("SLS_ENDPOINT", ""),
			Project:  getenv("SLS_PROJECT", "tnbiz-dev"),
			Logstore: getenv("SLS_LOGSTORE", "partner-api"),
		},
		MNS: MNSConfig{
			Backend:              getenv("OUTBOX_BACKEND", ""), // 由 validate 补默认
			Endpoint:             getenv("MNS_ENDPOINT", ""),
			AccessKeyID:          getenv("MNS_ACCESS_KEY_ID", ""),
			AccessKeySecret:      getenv("MNS_ACCESS_KEY_SECRET", ""),
			QueueIn:              getenv("MNS_QUEUE_IN", "tnbiz-partner-in"),
			QueueOut:             getenv("MNS_QUEUE_OUT", "tnbiz-partner-out"),
			QueueDLQ:             getenv("MNS_QUEUE_DLQ", "tnbiz-partner-dlq"),
			DataRegion:           getenv("DATA_REGION", "cn"),
			VisibilityTimeoutSec: int(getenvInt64("MNS_VISIBILITY_TIMEOUT_SEC", 30)),
			LongPollSec:          int(getenvInt64("MNS_LONG_POLL_SEC", 20)),
			DLQThreshold:         int(getenvInt64("MNS_DLQ_THRESHOLD", 10)),
		},
		FyAPI: FyAPIConfig{
			BaseURL:    getenv("FYAPI_BASE_URL", "http://127.0.0.1:3000"),
			HMACKeyID:  getenv("FYAPI_HMAC_KEY_ID", ""),
			HMACSecret: getenv("FYAPI_HMAC_SECRET", ""),
			Timeout:    time.Duration(getenvInt64("FYAPI_TIMEOUT_SEC", 10)) * time.Second,
		},
		BizSetting:             NewBizSettingConfig(),
		IdempotencyTTL:         time.Duration(getenvInt64("IDEMPOTENCY_TTL_HOURS", 24)) * time.Hour,
		SagaWallClock:          time.Duration(getenvInt64("SAGA_WALL_CLOCK_HOURS", 1)) * time.Hour,
		InternalIdempotencyTTL: time.Duration(getenvInt64("INTERNAL_IDEMPOTENCY_TTL_DAYS", 7)) * 24 * time.Hour,
		AllowedOrigins:         splitCSV(getenv("ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:5174,http://localhost:5175,http://localhost:5176")),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// validate 启动期 invariant（per ADR D-3 / SEC CRIT-7）。
//
// 调用方约定：Load() 总会给 HTTP.Addr 默认值；裸构造 Config 用于单元测试时
// 仅 SagaWallClock / IdempotencyTTL / InternalIdempotencyTTL 上下界是关键 invariant。
func (c *Config) validate() error {
	if c.Env == "" {
		return fmt.Errorf("ENV not set")
	}
	if c.SagaWallClock > c.IdempotencyTTL {
		return fmt.Errorf("invariant violation: saga_wall_clock_hours (%s) must be <= idempotency_ttl_hours (%s)",
			c.SagaWallClock, c.IdempotencyTTL)
	}
	if c.IdempotencyTTL > c.InternalIdempotencyTTL {
		return fmt.Errorf("invariant violation: idempotency_ttl_hours (%s) must be <= internal_idempotency_ttl_days × 24 (%s)",
			c.IdempotencyTTL, c.InternalIdempotencyTTL)
	}
	// MNS backend default + prod hardening (Fix-B' part 3 CRIT-B5).
	if c.MNS.Backend == "" {
		if c.Env == EnvProd {
			c.MNS.Backend = "aliyun_mns"
		} else {
			c.MNS.Backend = "memstub"
		}
	}
	switch c.MNS.Backend {
	case "aliyun_mns", "memstub":
	default:
		return fmt.Errorf("OUTBOX_BACKEND must be aliyun_mns|memstub, got %q", c.MNS.Backend)
	}
	if c.Env == EnvProd && c.MNS.Backend == "memstub" {
		return fmt.Errorf("OUTBOX_BACKEND=memstub refused in prod (Fix-B' CRIT-B5 fail-closed)")
	}
	if c.MNS.Backend == "aliyun_mns" && (c.MNS.Endpoint == "" || c.MNS.AccessKeyID == "" || c.MNS.AccessKeySecret == "") {
		return fmt.Errorf("OUTBOX_BACKEND=aliyun_mns requires MNS_ENDPOINT / MNS_ACCESS_KEY_ID / MNS_ACCESS_KEY_SECRET")
	}
	if c.MNS.DataRegion == "" {
		c.MNS.DataRegion = "cn"
	}
	if c.MNS.DataRegion != "cn" && c.MNS.DataRegion != "sg" {
		return fmt.Errorf("DATA_REGION must be cn|sg, got %q", c.MNS.DataRegion)
	}
	return nil
}

// Get 读取 biz_setting 值；不存在返回空串。线程安全。
func (b *BizSettingConfig) Get(key string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.values[key]
}

// Replace 原子替换全部 biz_setting；W1c 由 polling 调用。
//
// immutability：调用方传入的 map 不被持有；内部 copy。
func (b *BizSettingConfig) Replace(next map[string]string) {
	cp := make(map[string]string, len(next))
	for k, v := range next {
		cp[k] = v
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.values = cp
	b.updated = time.Now()
}

// LastUpdated 上次成功 polling 时间；W1c 用于健康检查暴露。
func (b *BizSettingConfig) LastUpdated() time.Time {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.updated
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvFileOrValue(fileKey, valueKey, def string) string {
	if path := os.Getenv(fileKey); path != "" {
		b, err := os.ReadFile(path)
		if err == nil {
			return string(b)
		}
	}
	return getenv(valueKey, def)
}

func getenvInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	out := []string{}
	cur := ""
	for _, r := range s {
		if r == ',' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
