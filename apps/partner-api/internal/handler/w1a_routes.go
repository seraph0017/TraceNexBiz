// Package handler W1a — auth / partner / customer / kyc / wallet / invitation HTTP 路由。
//
// 路径分组：
//
//	POST   /public/auth/login               site-aware login
//	POST   /public/auth/logout              单设备登出
//	POST   /public/auth/refresh             refresh-token rotation
//	POST   /public/auth/password/forgot     双因子重置 阶段 1
//	POST   /public/auth/password/reset      双因子重置 阶段 2
//	POST   /public/partner/apply            场景 B 自助申请
//	POST   /public/customer/register        被邀请客户注册（防绕过）
//	GET    /partner/me                      当前 partner 详情
//	GET    /partner/wallet                  钱包快照
//	GET    /partner/wallet/logs             流水
//	POST   /partner/invitation              生成 invitation_code
//	GET    /partner/invitation              列出 invitation_code
//	POST   /partner/kyc                     提交 KYC
//	POST   /customer/kyc                    customer 提交 KYC（路径同 service）
//	POST   /customer/transfer               场景 H 切换渠道商
//	POST   /customer/erase                  场景 Q 右遗忘
//	POST   /admin/partners/:id/approve      staff 审核通过
//
// W1c 在 admin / staff 路由组上额外挂 RBAC + step-up middleware。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/middleware"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/auth"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/customer"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/invitation"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/kyc"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/partner"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/wallet"
)

// W1aDeps 主装配依赖。
type W1aDeps struct {
	Auth       *auth.Service
	Partner    *partner.Service
	Customer   *customer.Service
	KYC        *kyc.Service
	Wallet     *wallet.Service
	Invitation *invitation.Service
}

// RegisterW1aRoutes 把 W1a 全部路由挂到 gin.Engine（W1c 的 admin / customer / partner middleware 在外层挂）。
//
// 每条路由前置 middleware.WithScope(scope) — BOLA middleware (在 main.go group 上挂) 据此 enforce。
// scope 命名约定：
//   - "public"          /public/* — 不要求 JWT
//   - "partner_self"    actor=partner 自身
//   - "customer_self"   actor=customer 自身
//   - "staff_admin"     actor=staff（具体 RBAC 角色由 service 层校）
func RegisterW1aRoutes(r *gin.Engine, d W1aDeps) {
	pub := r.Group("/public")
	pub.POST("/auth/login", middleware.WithScope("public"), loginHandler(d.Auth))
	pub.POST("/auth/logout", middleware.WithScope("public"), logoutHandler(d.Auth))
	pub.POST("/auth/refresh", middleware.WithScope("public"), refreshHandler(d.Auth))
	pub.POST("/auth/password/forgot", middleware.WithScope("public"), passwordForgotHandler(d.Auth))
	pub.POST("/auth/password/reset", middleware.WithScope("public"), passwordResetHandler(d.Auth))
	pub.POST("/partner/apply", middleware.WithScope("public"), partnerApplyHandler(d.Partner))
	pub.POST("/customer/register", middleware.WithScope("public"), customerRegisterHandler(d.Customer))

	p := r.Group("/partner")
	p.GET("/me", middleware.WithScope("partner_self"), partnerMeHandler(d.Partner))
	p.GET("/wallet", middleware.WithScope("partner_self"), walletGetHandler(d.Wallet))
	p.GET("/wallet/logs", middleware.WithScope("partner_self"), walletLogsHandler(d.Wallet))
	p.POST("/invitation", middleware.WithScope("partner_self"), invitationGenerateHandler(d.Invitation))
	p.GET("/invitation", middleware.WithScope("partner_self"), invitationListHandler(d.Invitation))
	p.POST("/kyc", middleware.WithScope("partner_self"), kycSubmitHandler(d.KYC))

	c := r.Group("/customer")
	c.POST("/kyc", middleware.WithScope("customer_self"), kycSubmitHandler(d.KYC))
	c.POST("/transfer", middleware.WithScope("customer_self"), customerTransferHandler(d.Customer))
	c.POST("/erase", middleware.WithScope("customer_self"), customerEraseHandler(d.Customer))

	a := r.Group("/admin")
	a.POST("/partners/:id/approve", middleware.WithScope("staff_admin"), partnerApproveHandler(d.Partner))
	a.POST("/partners/:id/terminate", middleware.WithScope("staff_admin"), partnerTerminateHandler(d.Partner))
	a.POST("/kyc/:id/review", middleware.WithScope("staff_compliance"), kycReviewHandler(d.KYC))
}

// 封装 success / fail envelope（与 backend §11 / pkg/errors 对齐）。
func ok(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{
		"success": true, "data": data, "error": nil,
	})
}

func fail(c *gin.Context, status int, code string, msgZh, msgEn string) {
	c.JSON(status, gin.H{
		"success": false, "data": nil,
		"error": gin.H{"code": code, "message_zh": msgZh, "message_en": msgEn},
	})
}

// scopeOf 从 ctx 取 actor。
//
// 仅从 JWT middleware 注入的 claims 读取；X-Dev-Actor-* header bypass 已移除
// （Round-1 SEC-CRIT-C3：dev header bypass 不应进 prod binary）。
func scopeOf(c *gin.Context) (actorType auth.ActorType, actorID int64) {
	// 1. middleware.ClaimsFrom（推荐路径，*middleware.Claims）
	if v, ok := c.Get("jwt_claims"); ok {
		switch cl := v.(type) {
		case auth.Claims:
			return auth.ActorType(cl.ActorType), cl.ActorID
		case *auth.Claims:
			if cl != nil {
				return auth.ActorType(cl.ActorType), cl.ActorID
			}
		default:
			// middleware package 类型（避免循环依赖，仅 reflect-like 字段读取）
			type claimsLike interface {
				GetActorType() string
				GetActorID() int64
			}
			if cl, ok := v.(claimsLike); ok {
				return auth.ActorType(cl.GetActorType()), cl.GetActorID()
			}
		}
	}
	return
}
