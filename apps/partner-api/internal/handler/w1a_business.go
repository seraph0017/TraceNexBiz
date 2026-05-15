// W1a partner / customer / kyc / wallet / invitation handlers。
package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/middleware"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/customer"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/invitation"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/kyc"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/partner"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/wallet"
)

type partnerApplyBody struct {
	Type         string `json:"type" binding:"required"`
	BusinessName string `json:"business_name"`
	ContactName  string `json:"contact_name" binding:"required"`
	ContactPhone string `json:"contact_phone" binding:"required"`
	ContactEmail string `json:"contact_email" binding:"required,email"`
	ConsentID    int64  `json:"consent_id" binding:"required"`
	Note         string `json:"note"`
	FyUserID     int64  `json:"fy_user_id"`
}

func partnerApplyHandler(s *partner.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var b partnerApplyBody
		if err := c.ShouldBindJSON(&b); err != nil {
			fail(c, http.StatusBadRequest, "BIZ_VALID_BODY", "请求体无效", "invalid body")
			return
		}
		p, err := s.Apply(c.Request.Context(), partner.ApplyInput{
			FyUserID: b.FyUserID, Type: b.Type, BusinessName: b.BusinessName,
			ContactName: b.ContactName, ContactPhone: b.ContactPhone,
			ContactEmail: b.ContactEmail, ConsentID: b.ConsentID, Note: b.Note,
		})
		switch {
		case errors.Is(err, partner.ErrConsentMissing):
			fail(c, http.StatusUnprocessableEntity, "BIZ_VALID_CONSENT", "需要有效同意", "consent missing")
			return
		case errors.Is(err, partner.ErrEmailAlreadyRegistered):
			fail(c, http.StatusConflict, "BIZ_PARTNER_EMAIL_DUP", "邮箱已注册", "email already registered")
			return
		case err != nil:
			fail(c, http.StatusBadRequest, "BIZ_VALID_INPUT", "请求参数错误", err.Error())
			return
		}
		ok(c, http.StatusCreated, gin.H{"id": p.ID, "status": p.Status})
	}
}

type adminPartnerBody struct {
	ContactName  string `json:"contact_name" binding:"required"`
	ContactEmail string `json:"contact_email" binding:"required,email"`
	ContactPhone string `json:"contact_phone" binding:"required"`
}

func partnerListHandler(s *partner.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		if page < 1 {
			page = 1
		}
		if limit <= 0 || limit > 200 {
			limit = 50
		}
		rows, err := s.List(c.Request.Context(), partner.ListFilter{
			Status: partnerStatusFromAdmin(c.Query("status")),
			Search: strings.TrimSpace(c.Query("q")),
			Limit:  limit,
			Offset: (page - 1) * limit,
		})
		if err != nil {
			fail(c, http.StatusInternalServerError, "SYS_PANIC", "服务异常", err.Error())
			return
		}
		items := make([]gin.H, 0, len(rows))
		for i := range rows {
			items = append(items, partnerListDTO(rows[i]))
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    items,
			"error":   nil,
			"meta": gin.H{
				"total": len(items),
				"page":  page,
				"limit": limit,
			},
		})
	}
}

func partnerCreateHandler(s *partner.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var b adminPartnerBody
		if err := c.ShouldBindJSON(&b); err != nil {
			fail(c, http.StatusBadRequest, "BIZ_VALID_BODY", "请求体无效", "invalid body")
			return
		}
		phone := strings.TrimSpace(b.ContactPhone)
		if phone != "" && !strings.HasPrefix(phone, "+") {
			phone = "+86" + phone
		}
		p, err := s.AdminCreate(c.Request.Context(), partner.ApplyInput{
			Type:         "enterprise",
			BusinessName: strings.TrimSpace(b.ContactName),
			ContactName:  strings.TrimSpace(b.ContactName),
			ContactPhone: phone,
			ContactEmail: strings.TrimSpace(b.ContactEmail),
			Note:         "created_by_admin",
		})
		switch {
		case errors.Is(err, partner.ErrEmailAlreadyRegistered):
			fail(c, http.StatusConflict, "BIZ_PARTNER_EMAIL_DUP", "邮箱已注册", "email already registered")
			return
		case errors.Is(err, partner.ErrConsentMissing):
			fail(c, http.StatusUnprocessableEntity, "BIZ_VALID_CONSENT", "需要有效同意", "consent missing")
			return
		case err != nil:
			fail(c, http.StatusBadRequest, "BIZ_VALID_INPUT", "请求参数错误", err.Error())
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"success": true,
			"data":    partnerListDTO(*p),
			"error":   nil,
		})
	}
}

func partnerDetailHandler(s *partner.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			fail(c, http.StatusBadRequest, "BIZ_VALID_PATH", "路径参数错误", "bad id")
			return
		}
		p, err := s.Get(c.Request.Context(), id)
		if err != nil {
			fail(c, http.StatusNotFound, "BIZ_RES_NOT_FOUND", "未找到", "not found")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    partnerDetailDTO(*p),
			"error":   nil,
		})
	}
}

func partnerMeHandler(s *partner.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, actorID := scopeOf(c)
		if actorID <= 0 {
			fail(c, http.StatusUnauthorized, "BIZ_AUTH_REQUIRED", "未登录", "auth required")
			return
		}
		p, err := s.Get(c.Request.Context(), actorID)
		if err != nil {
			fail(c, http.StatusNotFound, "BIZ_RES_NOT_FOUND", "未找到", "not found")
			return
		}
		ok(c, http.StatusOK, p)
	}
}

func partnerApproveHandler(s *partner.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			fail(c, http.StatusBadRequest, "BIZ_VALID_PATH", "路径参数错误", "bad id")
			return
		}
		_, staffID := scopeOf(c)
		updated, err := s.Approve(c.Request.Context(), id, staffID)
		if err != nil {
			fail(c, http.StatusUnprocessableEntity, "BIZ_PARTNER_TRANSITION", "状态机不允许", err.Error())
			return
		}
		ok(c, http.StatusOK, gin.H{"status": updated.Status, "approved_at": updated.ApprovedAt})
	}
}

func partnerListDTO(p domain.Partner) gin.H {
	return gin.H{
		"id":                   p.ID,
		"display_name":         partnerDisplayName(p),
		"contact_email_masked": maskEmail(p.ContactEmail),
		"status":               adminPartnerStatus(p.Status),
		"kyc_status":           kycStatusText(p.KYCStatus),
		"terminated_at":        timeOrNil(p.TerminatedAt),
		"grace_period_ends_at": nil,
		"created_at":           p.CreatedAt,
	}
}

func partnerDetailDTO(p domain.Partner) gin.H {
	out := partnerListDTO(p)
	out["contact_phone_masked"] = maskPhone(p.ContactPhonePlain)
	out["bank_account_masked"] = ""
	out["monthly_gross"] = 0
	out["monthly_net"] = 0
	out["customers_count"] = 0
	return out
}

func partnerDisplayName(p domain.Partner) string {
	if strings.TrimSpace(p.ContactName) != "" {
		return p.ContactName
	}
	if strings.TrimSpace(p.ContactEmail) != "" {
		return p.ContactEmail
	}
	return "Partner #" + strconv.FormatInt(p.ID, 10)
}

func adminPartnerStatus(s domain.PartnerStatus) string {
	switch s {
	case domain.PartnerStatusApproved, domain.PartnerStatusApplied, domain.PartnerStatusReviewing:
		return "active"
	case domain.PartnerStatusFrozen, domain.PartnerStatusSuspended:
		return "suspended"
	case domain.PartnerStatusTerminated, domain.PartnerStatusRejected:
		return "terminated"
	default:
		return "active"
	}
}

func partnerStatusFromAdmin(s string) string {
	switch s {
	case "active":
		return string(domain.PartnerStatusApproved)
	case "suspended":
		return string(domain.PartnerStatusSuspended)
	case "terminated", "terminating":
		return string(domain.PartnerStatusTerminated)
	default:
		return ""
	}
}

func kycStatusText(status int8) string {
	switch status {
	case 1:
		return "submitted"
	case 2:
		return "approved"
	case 3:
		return "rejected"
	case 4:
		return "expired"
	default:
		return "pending"
	}
}

func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.Index(email, "@")
	if at <= 1 {
		return email
	}
	return email[:1] + "***" + email[at:]
}

func maskPhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if len(phone) <= 7 {
		return phone
	}
	return phone[:3] + "****" + phone[len(phone)-4:]
}

func timeOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t
}

type partnerTerminateBody struct {
	Reason    string `json:"reason"`
	GraceDays int    `json:"grace_days"`
}

func partnerTerminateHandler(s *partner.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			fail(c, http.StatusBadRequest, "BIZ_VALID_PATH", "路径参数错误", "bad id")
			return
		}
		var b partnerTerminateBody
		_ = c.ShouldBindJSON(&b)
		updated, err := s.Terminate(c.Request.Context(), id, b.Reason, b.GraceDays)
		if err != nil {
			fail(c, http.StatusUnprocessableEntity, "BIZ_PARTNER_TRANSITION", "终止失败", err.Error())
			return
		}
		ok(c, http.StatusOK, gin.H{"status": updated.Status, "terminated_at": updated.TerminatedAt})
	}
}

type customerRegisterBody struct {
	FyUserID       int64  `json:"fy_user_id" binding:"required"`
	InvitationCode string `json:"invitation_code" binding:"required"`
}

func customerRegisterHandler(s *customer.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var b customerRegisterBody
		if err := c.ShouldBindJSON(&b); err != nil {
			fail(c, http.StatusBadRequest, "BIZ_VALID_BODY", "请求体无效", "invalid body")
			return
		}
		got, err := s.Register(c.Request.Context(), customer.RegisterInput{
			FyUserID: b.FyUserID, InvitationCode: b.InvitationCode,
		})
		switch {
		case errors.Is(err, customer.ErrInvitationRequired):
			fail(c, http.StatusBadRequest, "BIZ_CUSTOMER_INVITATION_REQUIRED", "需要邀请码", "invitation required")
			return
		case errors.Is(err, customer.ErrAlreadyAffiliated), errors.Is(err, customer.ErrAlreadyDirect):
			fail(c, http.StatusConflict, "BIZ_CUSTOMER_DUP", "客户已存在", "already exists")
			return
		case err != nil:
			fail(c, http.StatusBadRequest, "BIZ_VALID_INPUT", "请求参数错误", err.Error())
			return
		}
		ok(c, http.StatusCreated, gin.H{
			"id": got.ID, "partner_id": got.PartnerID, "status": got.Status,
		})
	}
}

type customerTransferBody struct {
	ToPartnerID int64  `json:"to_partner_id" binding:"required"`
	Reason      string `json:"reason"`
}

func customerTransferHandler(s *customer.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, actorID := scopeOf(c)
		var b customerTransferBody
		if err := c.ShouldBindJSON(&b); err != nil {
			fail(c, http.StatusBadRequest, "BIZ_VALID_BODY", "请求体无效", "invalid body")
			return
		}
		log, err := s.RequestTransfer(c.Request.Context(), customer.TransferRequestInput{
			CustomerID: actorID, FromPartnerID: actorID, ToPartnerID: b.ToPartnerID,
			InitiatorType: "customer", InitiatorID: actorID, Reason: b.Reason,
			IdempotencyKey: c.GetHeader(middleware.HeaderIdemKey),
			TraceID:        middleware.TraceIDFrom(c),
		})
		if err != nil {
			fail(c, http.StatusBadRequest, "BIZ_CUSTOMER_TRANSFER", "切换失败", err.Error())
			return
		}
		ok(c, http.StatusAccepted, gin.H{"change_log_id": log.ID, "status": log.Status})
	}
}

func customerEraseHandler(s *customer.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, actorID := scopeOf(c)
		if err := s.SubmitErase(c.Request.Context(), customer.EraseInput{
			CustomerID: actorID, PartnerID: actorID, Reason: "pipl",
			IdempotencyKey: c.GetHeader(middleware.HeaderIdemKey),
			TraceID:        middleware.TraceIDFrom(c),
		}); err != nil {
			fail(c, http.StatusInternalServerError, "BIZ_PIPL_ERASE", "右遗忘失败", err.Error())
			return
		}
		ok(c, http.StatusOK, gin.H{"erased": true})
	}
}

type kycSubmitBody struct {
	Type                 int8   `json:"type" binding:"required"`
	BusinessLicenseURL   string `json:"business_license_url"`
	LegalPersonName      string `json:"legal_person_name" binding:"required"`
	LegalPersonID        string `json:"legal_person_id" binding:"required"`
	LegalPersonIDURL     string `json:"legal_person_id_url" binding:"required"`
	BankAccount          string `json:"bank_account"`
	AlipayOpenID         string `json:"alipay_open_id"`
	AlipayRealName       string `json:"alipay_real_name"`
	BiometricLivenessURL string `json:"biometric_liveness_url"`
	ConsentSensitivePIID int64  `json:"consent_sensitive_pi_id" binding:"required"`
	ConsentBiometricID   int64  `json:"consent_biometric_id"`
}

func kycSubmitHandler(s *kyc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, actorID := scopeOf(c)
		var b kycSubmitBody
		if err := c.ShouldBindJSON(&b); err != nil {
			fail(c, http.StatusBadRequest, "BIZ_VALID_BODY", "请求体无效", "invalid body")
			return
		}
		// fy_user_id 取自 session 反查；W1c JWT middleware 会把 sub 注进 ctx，本 handler 复用 actorID 作为 fy_user_id 占位
		got, err := s.Submit(c.Request.Context(), kyc.SubmitInput{
			FyUserID: actorID, Type: b.Type,
			BusinessLicenseURL: b.BusinessLicenseURL,
			LegalPersonName:    b.LegalPersonName, LegalPersonID: b.LegalPersonID,
			LegalPersonIDURL: b.LegalPersonIDURL,
			BankAccount:      b.BankAccount,
			AlipayOpenID:     b.AlipayOpenID, AlipayRealName: b.AlipayRealName,
			BiometricLivenessURL: b.BiometricLivenessURL,
			ConsentSensitivePIID: b.ConsentSensitivePIID,
			ConsentBiometricID:   b.ConsentBiometricID,
		})
		switch {
		case errors.Is(err, kyc.ErrConsentRequired):
			fail(c, http.StatusUnprocessableEntity, "BIZ_VALID_CONSENT", "需要敏感个人信息授权", "consent required")
			return
		case errors.Is(err, kyc.ErrFileNotVerified):
			fail(c, http.StatusBadRequest, "BIZ_KYC_FILE_INVALID", "上传校验失败", err.Error())
			return
		case err != nil:
			fail(c, http.StatusBadRequest, "BIZ_VALID_INPUT", "请求参数错误", err.Error())
			return
		}
		ok(c, http.StatusAccepted, gin.H{"id": got.ID, "status": got.Status})
	}
}

type kycReviewBody struct {
	Approve          bool   `json:"approve"`
	RejectReasonCode string `json:"reject_reason_code"`
	RejectReasonText string `json:"reject_reason_text"`
}

func kycReviewHandler(s *kyc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			fail(c, http.StatusBadRequest, "BIZ_VALID_PATH", "路径参数错误", "bad id")
			return
		}
		_, staffID := scopeOf(c)
		var b kycReviewBody
		_ = c.ShouldBindJSON(&b)
		got, err := s.Review(c.Request.Context(), id, kyc.ApprovalInput{
			StaffID: staffID, Approve: b.Approve,
			RejectReasonCode: b.RejectReasonCode, RejectReasonText: b.RejectReasonText,
		})
		if err != nil {
			fail(c, http.StatusUnprocessableEntity, "BIZ_KYC_REVIEW", "审核失败", err.Error())
			return
		}
		ok(c, http.StatusOK, gin.H{"status": got.Status})
	}
}

func walletGetHandler(s *wallet.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, partnerID := scopeOf(c)
		snap, err := s.Get(c.Request.Context(), partnerID)
		if err != nil {
			fail(c, http.StatusNotFound, "BIZ_RES_NOT_FOUND", "未找到", "not found")
			return
		}
		ok(c, http.StatusOK, snap)
	}
}

func walletLogsHandler(s *wallet.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, partnerID := scopeOf(c)
		limit, _ := strconv.Atoi(c.Query("limit"))
		offset, _ := strconv.Atoi(c.Query("offset"))
		rows, err := s.ListLogs(c.Request.Context(), partnerID, wallet.LogFilter{
			Limit: limit, Offset: offset,
		})
		if err != nil {
			fail(c, http.StatusInternalServerError, "SYS_PANIC", "服务异常", err.Error())
			return
		}
		ok(c, http.StatusOK, rows)
	}
}

type invitationGenBody struct {
	Type       string `json:"type"`
	UsageLimit int32  `json:"usage_limit"`
}

func invitationGenerateHandler(s *invitation.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, partnerID := scopeOf(c)
		var b invitationGenBody
		_ = c.ShouldBindJSON(&b)
		if b.Type == "" {
			b.Type = invitation.TypePermanent
		}
		code, err := s.GenerateWith(c.Request.Context(), invitation.GenerateInput{
			PartnerID: partnerID, Type: b.Type, UsageLimit: b.UsageLimit,
		})
		if err != nil {
			fail(c, http.StatusBadRequest, "BIZ_VALID_INPUT", "生成邀请码失败", err.Error())
			return
		}
		ok(c, http.StatusCreated, gin.H{"code": code})
	}
}

func invitationListHandler(s *invitation.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, partnerID := scopeOf(c)
		rows, err := s.ListByPartner(c.Request.Context(), partnerID)
		if err != nil {
			fail(c, http.StatusInternalServerError, "SYS_PANIC", "服务异常", err.Error())
			return
		}
		ok(c, http.StatusOK, rows)
	}
}
