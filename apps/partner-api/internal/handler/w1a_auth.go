// W1a auth handlers — login / logout / refresh / password reset。
package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/middleware"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/auth"
)

type loginBody struct {
	Site     string `json:"site" binding:"required"`
	Handle   string `json:"handle" binding:"required"`
	Password string `json:"password" binding:"required"`
	OTP      string `json:"otp"`
	DeviceFP string `json:"device_fingerprint"`
}

func loginHandler(s *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var b loginBody
		if err := c.ShouldBindJSON(&b); err != nil {
			fail(c, http.StatusBadRequest, "BIZ_VALID_BODY", "请求体无效", "invalid body")
			return
		}
		out, err := s.Login(c.Request.Context(), auth.LoginInput{
			Site: auth.Site(b.Site), Handle: b.Handle, Password: b.Password,
			MFAOTP: b.OTP, DeviceFP: b.DeviceFP,
			ClientIP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
		})
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrAccountLocked):
			fail(c, http.StatusUnauthorized, "BIZ_AUTH_INVALID", "账号或密码错误", "invalid credentials")
			return
		case errors.Is(err, auth.ErrMFARequired):
			fail(c, http.StatusUnauthorized, "BIZ_AUTH_MFA_REQUIRED", "需要二次校验", "mfa required")
			return
		case err != nil:
			fail(c, http.StatusInternalServerError, "SYS_PANIC", "服务异常", "internal error")
			return
		}
		setAuthCookies(c, out)
		ok(c, http.StatusOK, gin.H{
			"actor_type": out.ActorType, "actor_id": out.ActorID,
			"fy_user_id": out.FyUserID, "expires_at": out.ExpiresAt,
		})
	}
}

func logoutHandler(s *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		access, _ := c.Cookie(middleware.CookieAccess)
		if access == "" {
			ok(c, http.StatusOK, gin.H{"logged_out": true})
			return
		}
		cl, err := s.VerifyAccessToken(c.Request.Context(), access)
		if err != nil {
			ok(c, http.StatusOK, gin.H{"logged_out": true})
			return
		}
		_ = s.Logout(c.Request.Context(), cl.Jti, time.Until(time.Unix(cl.Exp, 0)))
		clearAuthCookies(c)
		ok(c, http.StatusOK, gin.H{"logged_out": true})
	}
}

func refreshHandler(s *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, _ := c.Cookie(middleware.CookieRefresh)
		if raw == "" {
			fail(c, http.StatusUnauthorized, "BIZ_AUTH_NO_REFRESH", "缺少刷新令牌", "no refresh token")
			return
		}
		out, err := s.Refresh(c.Request.Context(), raw)
		if err != nil {
			fail(c, http.StatusUnauthorized, "BIZ_AUTH_REFRESH_INVALID", "刷新令牌无效", "refresh invalid")
			return
		}
		setAuthCookies(c, out)
		ok(c, http.StatusOK, gin.H{"expires_at": out.ExpiresAt})
	}
}

type forgotBody struct {
	Handle         string `json:"handle" binding:"required"`
	ConsentVersion string `json:"consent_version" binding:"required"`
}

func passwordForgotHandler(s *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var b forgotBody
		if err := c.ShouldBindJSON(&b); err != nil {
			fail(c, http.StatusBadRequest, "BIZ_VALID_BODY", "请求体无效", "invalid body")
			return
		}
		// 信息恒等：任何错误都返 200。
		_ = s.PasswordResetInitiate(c.Request.Context(), auth.PasswordResetInitiateInput{
			ActorHandle: b.Handle, ConsentVersion: b.ConsentVersion,
			ClientIP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
		})
		ok(c, http.StatusOK, gin.H{"message": "If an account matches, instructions have been sent."})
	}
}

type resetBody struct {
	RawToken    string `json:"token" binding:"required"`
	OTP         string `json:"otp" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=12"`
}

func passwordResetHandler(s *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var b resetBody
		if err := c.ShouldBindJSON(&b); err != nil {
			fail(c, http.StatusBadRequest, "BIZ_VALID_BODY", "请求体无效", "invalid body")
			return
		}
		err := s.PasswordResetConfirm(c.Request.Context(), auth.PasswordResetConfirmInput{
			RawToken: b.RawToken, OTP: b.OTP, NewPassword: b.NewPassword,
			ClientIP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
		})
		switch {
		case errors.Is(err, auth.ErrTokenInvalid):
			fail(c, http.StatusBadRequest, "BIZ_AUTH_RESET_TOKEN_INVALID", "重置令牌无效", "reset_token_invalid")
			return
		case errors.Is(err, auth.ErrSecondFactorBad):
			fail(c, http.StatusBadRequest, "BIZ_AUTH_RESET_FACTOR_BAD", "二次校验失败", "second_factor_mismatch")
			return
		case err != nil:
			fail(c, http.StatusInternalServerError, "SYS_PANIC", "服务异常", "internal error")
			return
		}
		clearAuthCookies(c)
		ok(c, http.StatusOK, gin.H{"message": "reset_ok"})
	}
}

// setAuthCookies 写三 cookie（httpOnly access / refresh + non-httpOnly csrf）。
//
// 与 frontend §6.2 / backend §7.6 一致：SameSite=Lax / Path=/ / Secure=cfg。
func setAuthCookies(c *gin.Context, out auth.LoginOutput) {
	maxAge := int(time.Until(out.ExpiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(middleware.CookieAccess, out.AccessToken, maxAge, "/", "", false, true)
	c.SetCookie(middleware.CookieRefresh, out.RefreshToken, maxAge*8, "/", "", false, true)
	c.SetCookie(middleware.CookieCSRF, out.CSRFToken, maxAge*8, "/", "", false, false)
}

func clearAuthCookies(c *gin.Context) {
	c.SetCookie(middleware.CookieAccess, "", -1, "/", "", false, true)
	c.SetCookie(middleware.CookieRefresh, "", -1, "/", "", false, true)
	c.SetCookie(middleware.CookieCSRF, "", -1, "/", "", false, false)
}
