// Package main 启动 partner-api HTTP server。
//
// 架构层级（per backend §2）：
//   handler → service → repository → infra
//
// 关键中间件链（per backend §7.1）：
//   全局：    RequestID → CORS → SecurityHeaders → Audit
//   /partner /customer /admin（path-scoped）：
//             + JWT → CSRF → PIIScrubber → Idempotency → BOLAScope
//   /webhook： + WebhookIdempotency
//   /public：  仅全局（无鉴权）
//
// W1a 完成 7 middleware + 全部 WithScope 装配；W1c 接 KMS / 真 fyapi.Client。
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/audit"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/config"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/handler"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/db"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/kms"
	infraredis "github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/redis"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/middleware"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/outbox"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository/mysql"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/auth"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/customer"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/invitation"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/kyc"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/partner"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/wallet"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/pkg/consent"

	"gorm.io/gorm"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339Nano})

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config.Load failed")
	}

	// dev 环境 cookie Secure=false；staging/prod 强制 true。
	handler.SetCookieSecure(cfg.Env)

	// W0：infra 用 best-effort 初始化；缺依赖时不阻塞 /healthz
	bizDB, fyDB, dbErr := db.Open(cfg)
	if dbErr != nil {
		log.Warn().Err(dbErr).Msg("db.Open failed; running in degraded mode (W0 only)")
	}
	rdb, redisErr := infraredis.Open(cfg)
	if redisErr != nil {
		log.Warn().Err(redisErr).Msg("redis.Open failed; running in degraded mode (W0 only)")
	}

	// KMS 装配（per ADR-009）。
	// dev 默认 stub；staging/prod 必须 aliyun。
	kmsSvc := mustBuildKMS(cfg)
	_ = kmsSvc // W1c：服务层 inject

	_ = bizDB
	_ = fyDB

	router := buildRouter(cfg, rdb, bizDB)

	srv := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Info().Str("addr", cfg.HTTP.Addr).Msg("partner-api listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("server crashed")
		}
	}()

	// Outbox MNS SINK (Fix-B' part 3 CRIT-B5): SOURCE 由 outbox-poller cmd 单独运行；
	// SINK 在 server 进程内长轮询 MNS QueueIn 并 dispatch 到注册的 handler。
	mnsCtx, mnsCancel := context.WithCancel(context.Background())
	defer mnsCancel()
	startMNSConsumer(mnsCtx, cfg)

	// 优雅退出（per backend §13.4）
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop
	log.Info().Msg("shutdown initiated")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("graceful shutdown failed")
	}
	log.Info().Msg("partner-api stopped")
}

func mustBuildKMS(cfg *config.Config) kms.Service {
	kmsProvider := os.Getenv("KMS_PROVIDER")
	if kmsProvider == "" {
		if cfg.Env == config.EnvDev {
			kmsProvider = "stub"
		} else {
			kmsProvider = "aliyun"
		}
	}
	if cfg.Env == config.EnvDev {
		k, err := kms.New(cfg.Env, kmsProvider)
		if err != nil {
			log.Fatal().Err(err).Msg("kms init failed")
		}
		return k
	}
	// staging / prod：直接构造 AliyunKMS（factory 故意拒绝以避免误拿 stub）。
	if cfg.KMS.Endpoint == "" || cfg.KMS.KeyID == "" {
		log.Fatal().Msg("KMS_ENDPOINT / KMS_KEY_ID required in staging/prod")
	}
	return kms.NewAliyunKMS(cfg.KMS.Endpoint, cfg.KMS.KeyID, cfg.KMS.Region, cfg.KMS.AccessKey, cfg.KMS.AccessSecret)
}

// buildRouter 装配 gin engine + 全局 middleware + 路由组。
//
// rdb 可为 nil（dev 起步无 Redis）；nil 时 idempotency / webhook idempotency 会 fail-closed 返 503。
//
// 中间件挂载顺序（必须在 RegisterRoutes 之前）：
//
//  1. r.Use(global...)             RequestID / CORS / SecurityHeaders / Audit
//  2. r.Use(scopedChainByPath...)  对 /partner /customer /admin 走 JWT/CSRF/PII/Idem/BOLA
//  3. r.Use(webhookScopedByPath)   对 /webhook 走 WebhookIdempotency
//  4. r.GET("/healthz", ...)
//  5. handler.RegisterW1aRoutes(r, ...)
//  6. handler.RegisterTODORoutes(r)
func buildRouter(cfg *config.Config, rdb *redis.Client, bizDB *gorm.DB) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// ---- 全局 middleware（per backend §7.1） ----
	r.Use(middleware.RequestID())
	r.Use(middleware.CORS(cfg.AllowedOrigins))
	r.Use(middleware.SecurityHeaders())

	// Audit sink：drop-on-overflow，1024 buffer。
	// Fix-B' part 4 CRIT-B6: bizDB 就绪时 → EnqueueSink 落 audit_log_unsealed；
	//                       否则降级到 log-only BufferedSink（dev / W0 fallback）。
	var auditSink middleware.AuditSink
	if bizDB != nil {
		gormStore := audit.NewGormStore(bizDB)
		enqueueSink := audit.NewEnqueueSink(gormStore, 1024)
		auditSink = &auditEnqueueAdapter{sink: enqueueSink}
		log.Info().Msg("audit sink: GORM EnqueueSink → audit_log_unsealed")
	} else {
		buffered := middleware.NewBufferedSink(1024, func(e middleware.AuditEntry) {
			log.Info().
				Str("actor_type", e.ActorType).
				Int64("actor_id", e.ActorID).
				Str("method", e.Method).
				Str("path", e.Path).
				Int("status", e.Status).
				Str("trace_id", e.RequestID).
				Msg("audit")
		})
		auditSink = buffered
		log.Warn().Msg("audit sink: bizDB unavailable, falling back to log-only BufferedSink (dev/W0 only)")
	}
	r.Use(middleware.Audit(auditSink))

	// ---- 鉴权依赖构造（fail-fast） ----
	verifier := mustBuildVerifier(cfg)
	revStore := mustBuildRevocation(cfg, rdb)
	bolaLogger := bolaLoggerFunc{}
	clk := func() int64 { return time.Now().Unix() }

	// 鉴权链（path-scoped 装载）。
	//
	// 注：BOLAScope 不在此链 — WithScope 在每条路由上声明 scope 时已经 inline enforce。
	// 如果想加 fail-closed safety net（catch 漏挂 WithScope 的路由），可在 chain 末尾再挂
	// middleware.BOLAScope(bolaLogger) — 它会读取 ctx 中的 scope（无 scope → 404）。
	authedChain := []gin.HandlerFunc{
		middleware.JWT(verifier, revStore, clk),
		middleware.CSRF(),
		middleware.PIIScrubber(),
		middleware.Idempotency(rdb, cfg.IdempotencyTTL),
	}
	_ = bolaLogger

	// 把每个鉴权 middleware 包成 path-filtered，逐个 r.Use 注册。
	// 这样 gin 的 c.Next() 能正确推进 → 下一个 mw → ... → handler；
	// 而非鉴权路径（/healthz / /public/* / /webhook/*）直接 skip。
	for _, mw := range authedChain {
		r.Use(filterByPrefix(mw, "/partner", "/customer", "/admin", "/api/sdk"))
	}

	// Webhook 链：独立 idempotency；只匹配 /webhook 前缀。
	r.Use(filterByPrefix(middleware.WebhookIdempotency(rdb, cfg.InternalIdempotencyTTL), "/webhook"))

	// ---- healthz（per backend §13.3）— 必须在鉴权链之后注册（healthz path 不匹配前缀） ----
	// 这三个端点是 LB / k8s probe 调用的基础设施 endpoint，无 actor identity，无对象层访问；
	// 故豁免 BOLA scope 检查（bolascope 静态分析器仅作用于业务路由）。
	//bolascope:allow public liveness probe, no actor identity
	r.GET("/healthz", handler.Healthz)
	//bolascope:allow public liveness probe, no actor identity
	r.GET("/healthz/live", handler.HealthzLive)
	//bolascope:allow public readiness probe, no actor identity
	r.GET("/healthz/ready", handler.HealthzReady)

	// W1a：auth / partner / customer / kyc / wallet / invitation。
	handler.RegisterW1aRoutes(r, buildW1aDeps(cfg, bizDB))

	// Fix-C CRIT-C1: public footer endpoint (合规公示).
	var bizSettingRepo *mysql.BizSettingRepository
	if bizDB != nil {
		bizSettingRepo = mysql.NewBizSettingRepository(bizDB)
	}
	handler.RegisterPublicFooterRoute(r, bizSettingRepo, rdb)

	// TODO route 占位（per W0 验收）
	handler.RegisterTODORoutes(r)

	return r
}

// mustBuildVerifier 装配 JWT verifier；dev 缺 PEM 时降级到 devNullVerifier。
func mustBuildVerifier(cfg *config.Config) middleware.Verifier {
	if cfg.JWT.VerifyKeyPEM != "" {
		v, err := middleware.NewRSAVerifier([]byte(cfg.JWT.VerifyKeyPEM))
		if err != nil {
			log.Fatal().Err(err).Msg("middleware.NewRSAVerifier failed (check JWT_VERIFY_KEY_PEM)")
		}
		return v
	}
	if cfg.Env != config.EnvDev {
		log.Fatal().Msg("JWT_VERIFY_KEY_PEM required in staging/prod")
	}
	log.Warn().Msg("JWT_VERIFY_KEY_PEM not set — dev fallback verifier in use; tokens will NOT validate")
	return devNullVerifier{}
}

// mustBuildRevocation 装配 jti revocation store；Redis 缺失时 dev 走 allow-all 占位。
func mustBuildRevocation(cfg *config.Config, rdb *redis.Client) middleware.RevocationStore {
	if rdb != nil {
		return middleware.NewRedisRevocationStore(rdb, 1*time.Second)
	}
	if cfg.Env != config.EnvDev {
		log.Fatal().Msg("Redis required in staging/prod for jti revocation (fail-closed)")
	}
	log.Warn().Msg("revocation store: Redis unavailable, using allow-all stub (dev only)")
	return devAllowAllRevStore{}
}

// filterByPrefix 把一个 middleware 包成 path-prefix 触发：path 命中任一 prefix 才执行；否则直接 c.Next()。
//
// 这种包装让 gin 的中间件链按正常顺序执行：mw 自己调用 c.Next() 时会推进到下一个 mw，
// 实现 group-scoped middleware 的等价语义，而不需要 r.Group。
func filterByPrefix(mw gin.HandlerFunc, prefixes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		for _, pre := range prefixes {
			if strings.HasPrefix(p, pre) {
				mw(c)
				return
			}
		}
		c.Next()
	}
}

// devNullVerifier dev 阶段无 PEM 时的占位 — 始终拒绝。
type devNullVerifier struct{}

func (devNullVerifier) Verify(string) (*middleware.Claims, error) {
	return nil, middleware.ErrTokenInvalid
}

// devAllowAllRevStore dev 阶段无 Redis 时的占位 — 始终放行。
type devAllowAllRevStore struct{}

func (devAllowAllRevStore) IsRevoked(string) (bool, error) { return false, nil }

// bolaLoggerFunc BOLA 拒绝时记录 — W1c 接 SLS / Prometheus counter。
type bolaLoggerFunc struct{}

func (bolaLoggerFunc) LogAttempt(actorType string, actorID int64, scope, path string) {
	log.Warn().
		Str("actor_type", actorType).
		Int64("actor_id", actorID).
		Str("scope", scope).
		Str("path", path).
		Msg("bola_attempt")
}

// buildW1aDeps 装配 W1a service。
//
// W1b：partner / customer / kyc / wallet / invitation 走 GORM repository（bizDB 必须非 nil；
// nil 时 fail-fast 退回 memory 以避免 dev 启动失败）。auth 暂仍走 memory（W1c 再迁）。
func buildW1aDeps(cfg *config.Config, bizDB *gorm.DB) handler.W1aDeps {
	_ = cfg
	authRepo := auth.NewMemoryRepo()
	hasher := auth.SimpleHasher{Salt: "tnbiz-dev-salt"}
	signer := auth.HMACSigner{Secret: []byte("tnbiz-dev-jwt-secret")}
	authSvc := auth.NewService(authRepo, auth.NewMemoryRevocation(), hasher,
		signer, auth.NoopNotifier{}, auth.AlwaysConsented{}, auth.Options{})

	// W1b：5 个 GORM repo（仅当 bizDB 已就绪；否则 fallback 到 memory 让 dev 起得来）。
	var (
		invRepo     invitation.Repository
		custRepo    customer.Repository
		partnerRepo partner.Repository
		walletRepo  wallet.Repository
		kycRepo     kyc.Repository
	)
	if bizDB != nil {
		invRepo = mysql.NewInvitationRepository(bizDB)
		custRepo = mysql.NewCustomerRepository(bizDB)
		partnerRepo = mysql.NewPartnerRepository(bizDB)
		walletRepo = mysql.NewWalletRepository(bizDB)
		kycRepo = mysql.NewKYCRepository(bizDB)
	} else {
		log.Warn().Msg("bizDB unavailable; falling back to in-memory repos (dev/W0 only)")
		invRepo = invitation.NewMemoryRepo()
		custRepo = customer.NewMemoryRepo()
		partnerRepo = partner.NewMemoryRepo()
		walletRepo = wallet.NewMemoryRepo()
		kycRepo = kyc.NewMemoryRepo()
	}

	invSvc := invitation.NewService(invRepo)

	custFy := customer.NewStubFyAPI()
	custSvc := customer.NewService(custRepo, &invitationAdapter{svc: invSvc}, custFy)
	// Fix-C P1-7：consent_text_version guard.
	cv := consent.NewVersionGuard(cfg.BizSetting)
	cv.SetDevMode(cfg.Env == config.EnvDev)
	custSvc = custSvc.WithConsentVerifier(cv)

	partnerSvc := partner.NewService(
		partnerRepo,
		partner.NewStubCrypto(),
		partner.NewAlwaysFreshConsent(time.Now()),
		invSvc,
		custSvc,
	)

	walletSvc := wallet.NewService(walletRepo)

	kycSvc := kyc.NewService(
		kycRepo,
		kyc.NewStubCrypto(),
		kyc.NewStubOCR(),
		kyc.NewStubOSS(),
		kyc.NewStubConsent(),
		kyc.NewStubLinker(),
	)

	return handler.W1aDeps{
		Auth: authSvc, Partner: partnerSvc, Customer: custSvc,
		KYC: kycSvc, Wallet: walletSvc, Invitation: invSvc,
	}
}

// invitationAdapter 把 invitation.Service 暴露为 customer.InvitationResolver。
type invitationAdapter struct{ svc *invitation.Service }

// Resolve .
func (a *invitationAdapter) Resolve(ctx context.Context, code string) (*domain.InvitationCode, error) {
	return a.svc.Resolve(ctx, code)
}

// Consume .
func (a *invitationAdapter) Consume(ctx context.Context, code string) (*domain.InvitationCode, error) {
	return a.svc.Consume(ctx, code)
}

// auditEnqueueAdapter 桥 middleware.AuditEntry → audit.UnsealedRow（Fix-B' part 4 CRIT-B6）.
//
// payload_json 用 BodyRedacted 透传（middleware 已经 piiscrubber.Redact 过）；
// request_hash = SHA-256(BodyRedacted)；route/method/status/trace_id/client_ip 直接映射。
//
// 不阻塞 HTTP 链路：EnqueueSink.Send 内部是非阻塞 channel send。
type auditEnqueueAdapter struct {
	sink *audit.EnqueueSink
}

func (a *auditEnqueueAdapter) Send(e middleware.AuditEntry) {
	var payload *string
	if len(e.BodyRedacted) > 0 {
		s := string(e.BodyRedacted)
		payload = &s
	}
	a.sink.Send(audit.UnsealedRow{
		ActorType:   e.ActorType,
		ActorID:     e.ActorID,
		Action:      e.Method + " " + e.Path,
		TargetType:  "http_request",
		Route:       e.Path,
		Method:      e.Method,
		Status:      e.Status,
		RequestHash: audit.HashRequestBody(e.BodyRedacted),
		PayloadJSON: payload,
		TraceID:     e.RequestID,
		IPAddress:   e.IP,
		UserAgent:   e.UserAgent,
		OccurredAt:  time.Now().UTC(),
	})
}

// startMNSConsumer 启动 outbox SINK（Fix-B' part 3 CRIT-B5）.
//
// backend=memstub → 跳过（dev 默认）；
// backend=aliyun_mns → 构造 HTTPMNSClient + MNSConsumer，goroutine 长轮询 QueueIn.
//
// 未注册 event_type 时 NoopOnUnknown=true → log warning + ack（不阻塞队列）.
// Phase-1 不注册任何 handler；后续 PR 在此处按 event_type 注册业务回调.
func startMNSConsumer(ctx context.Context, cfg *config.Config) {
	if cfg.MNS.Backend != "aliyun_mns" {
		log.Info().Str("backend", cfg.MNS.Backend).Msg("outbox sink: skipped (non-MNS backend)")
		return
	}
	client, err := outbox.NewHTTPMNSClient(outbox.MNSConfig{
		Endpoint:        cfg.MNS.Endpoint,
		AccessKeyID:     cfg.MNS.AccessKeyID,
		AccessKeySecret: cfg.MNS.AccessKeySecret,
		Timeout:         time.Duration(cfg.MNS.LongPollSec+15) * time.Second,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("outbox sink: NewHTTPMNSClient failed")
	}
	consumer, err := outbox.NewMNSConsumer(client, outbox.MNSConsumerOptions{
		QueueName:     cfg.MNS.QueueIn,
		DLQName:       cfg.MNS.QueueDLQ,
		WaitSeconds:   cfg.MNS.LongPollSec,
		DLQThreshold:  cfg.MNS.DLQThreshold,
		NoopOnUnknown: true,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("outbox sink: NewMNSConsumer failed")
	}
	// TODO(post-B5): consumer.Register("<event_type>", handler) 按业务领域注册.
	go func() {
		log.Info().
			Str("queue", cfg.MNS.QueueIn).
			Str("dlq", cfg.MNS.QueueDLQ).
			Int("wait_sec", cfg.MNS.LongPollSec).
			Msg("outbox sink: starting MNS consumer")
		if err := consumer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Error().Err(err).Msg("outbox sink: consumer.Run exited with error")
		}
	}()
}
