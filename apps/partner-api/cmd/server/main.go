// Package main 启动 partner-api HTTP server。
//
// 架构层级（per backend §2）：
//   handler → service → repository → infra
//
// 关键中间件链（per backend §7.1）：
//   RequestID → CORS → SecurityHeaders → BodyLimit → Tracing
//   → JWT (cookie tnbiz_access) → CSRF (double-submit) → Scope → Permission
//   → Idempotency → Handler
//
// W0 scaffold：仅完成 wiring + healthz + TODO route 占位；业务 handler / service body 留 TODO。
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/config"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/handler"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/db"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/infra/redis"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/middleware"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/auth"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/customer"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/invitation"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/kyc"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/partner"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/wallet"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339Nano})

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config.Load failed")
	}

	// W0：infra 用 best-effort 初始化；缺依赖时不阻塞 /healthz
	bizDB, fyDB, dbErr := db.Open(cfg)
	if dbErr != nil {
		log.Warn().Err(dbErr).Msg("db.Open failed; running in degraded mode (W0 only)")
	}
	rdb, redisErr := redis.Open(cfg)
	if redisErr != nil {
		log.Warn().Err(redisErr).Msg("redis.Open failed; running in degraded mode (W0 only)")
	}
	// W1c agent 在此处构造 service / repository / fyapi client 并 wire 进 handler；
	// W0 仅装载基础 router + healthz。
	_ = bizDB
	_ = fyDB
	_ = rdb

	router := buildRouter(cfg)

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

// buildRouter 装配 gin engine + 全局 middleware + 路由组。
//
// 详细路由表见 internal/handler。
func buildRouter(cfg *config.Config) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// 全局 middleware（per backend §7.1）；
	// W1a 在每个 group 内补 JWT / CSRF / Scope / Permission。
	r.Use(middleware.RequestID())
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORS(cfg.AllowedOrigins))

	// healthz（per backend §13.3）
	r.GET("/healthz", handler.Healthz)
	r.GET("/healthz/live", handler.HealthzLive)
	r.GET("/healthz/ready", handler.HealthzReady)

	// W1a：auth / partner / customer / kyc / wallet / invitation。
	//
	// dev 模式用全内存 repo + stub crypto/fyapi/notify；
	// W1c JWT middleware 接入后由 staff / partner / customer site cookie 互斥。
	handler.RegisterW1aRoutes(r, buildW1aDeps(cfg))

	// TODO route 占位（per W0 验收）
	handler.RegisterTODORoutes(r)

	return r
}

// buildW1aDeps 装配 W1a service。
//
// dev / W1a 阶段：所有依赖走 memory + stub；W1c 改为接 GORM repository / Redis revocation /
// KMS / fyapi.Client。
func buildW1aDeps(cfg *config.Config) handler.W1aDeps {
	_ = cfg
	authRepo := auth.NewMemoryRepo()
	hasher := auth.SimpleHasher{Salt: "tnbiz-dev-salt"}
	signer := auth.HMACSigner{Secret: []byte("tnbiz-dev-jwt-secret")}
	authSvc := auth.NewService(authRepo, auth.NewMemoryRevocation(), hasher,
		signer, auth.NoopNotifier{}, auth.AlwaysConsented{}, auth.Options{})

	invRepo := invitation.NewMemoryRepo()
	invSvc := invitation.NewService(invRepo)

	custRepo := customer.NewMemoryRepo()
	custFy := customer.NewStubFyAPI()
	// invitation port 适配：直接用 invitation.Service 暴露 Resolve / Consume 方法。
	custSvc := customer.NewService(custRepo, &invitationAdapter{svc: invSvc}, custFy)

	partnerSvc := partner.NewService(
		partner.NewMemoryRepo(),
		partner.NewStubCrypto(),
		partner.NewAlwaysFreshConsent(time.Now()),
		invSvc,
		custSvc,
	)

	walletSvc := wallet.NewService(wallet.NewMemoryRepo())

	kycSvc := kyc.NewService(
		kyc.NewMemoryRepo(),
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
