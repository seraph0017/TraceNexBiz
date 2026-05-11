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
	"errors"

	"github.com/gin-gonic/gin"

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
func RegisterW1aRoutes(r *gin.Engine, d W1aDeps) {
	pub := r.Group("/public")
	pub.POST("/auth/login", loginHandler(d.Auth))
	pub.POST("/auth/logout", logoutHandler(d.Auth))
	pub.POST("/auth/refresh", refreshHandler(d.Auth))
	pub.POST("/auth/password/forgot", passwordForgotHandler(d.Auth))
	pub.POST("/auth/password/reset", passwordResetHandler(d.Auth))
	pub.POST("/partner/apply", partnerApplyHandler(d.Partner))
	pub.POST("/customer/register", customerRegisterHandler(d.Customer))

	p := r.Group("/partner")
	p.GET("/me", partnerMeHandler(d.Partner))
	p.GET("/wallet", walletGetHandler(d.Wallet))
	p.GET("/wallet/logs", walletLogsHandler(d.Wallet))
	p.POST("/invitation", invitationGenerateHandler(d.Invitation))
	p.GET("/invitation", invitationListHandler(d.Invitation))
	p.POST("/kyc", kycSubmitHandler(d.KYC))

	c := r.Group("/customer")
	c.POST("/kyc", kycSubmitHandler(d.KYC))
	c.POST("/transfer", customerTransferHandler(d.Customer))
	c.POST("/erase", customerEraseHandler(d.Customer))

	a := r.Group("/admin")
	a.POST("/partners/:id/approve", partnerApproveHandler(d.Partner))
	a.POST("/partners/:id/terminate", partnerTerminateHandler(d.Partner))
	a.POST("/kyc/:id/review", kycReviewHandler(d.KYC))
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

// scopeOf 从 ctx 取 actor（middleware 注入；W1c 实现 JWT middleware 后会填）。
//
// W1a 阶段允许 dev / curl 通过 query/header 直接 override 以方便 happy-path 联调。
func scopeOf(c *gin.Context) (actorType auth.ActorType, actorID int64) {
	if claims, ok := c.Get("jwt_claims"); ok {
		if cl, ok := claims.(auth.Claims); ok {
			return auth.ActorType(cl.ActorType), cl.ActorID
		}
	}
	if v := c.GetHeader("X-Dev-Actor-Type"); v != "" {
		actorType = auth.ActorType(v)
	}
	if v := c.GetHeader("X-Dev-Actor-Id"); v != "" {
		var n int64
		_, _ = fmtSscan(v, &n)
		actorID = n
	}
	return
}

// fmtSscan 简单 atoi（避免引入 strconv 依赖到 handler 顶层）。
func fmtSscan(s string, out *int64) (int, error) {
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("not a number")
		}
		n = n*10 + int64(r-'0')
	}
	*out = n
	return len(s), nil
}
