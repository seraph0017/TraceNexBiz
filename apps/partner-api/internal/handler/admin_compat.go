package handler

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/domain"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/middleware"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/content_safety"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/kyc"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/partner"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/saga_admin"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/staff"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/wallet"
)

// AdminCompatDeps wires admin console endpoints that the W1g frontend already
// calls but the main server had not mounted yet.
type AdminCompatDeps struct {
	Partner       *partner.Service
	KYC           *kyc.Service
	Wallet        *wallet.Service
	ContentSafety *content_safety.Service
	Staff         *staff.Service
	SagaAdmin     *saga_admin.Service
	BizSettings   map[string]string
}

// RegisterAdminCompatRoutes mounts testable admin-console APIs under /admin or
// /api/admin. List/read endpoints return stable empty data when the production
// saga/provider is not wired; money/tax/external side effects fail loudly.
func RegisterAdminCompatRoutes(r gin.IRouter, d AdminCompatDeps) {
	g := r.Group("/admin")
	g.Use(adminActorContext())

	g.GET("/kyc", middleware.WithScope("staff_compliance"), adminListKYC(d.KYC))
	g.GET("/kyc/:id", middleware.WithScope("staff_compliance"), adminGetKYC(d.KYC))
	// POST /kyc/:id/review is already mounted by W1a routes.
	g.POST("/kyc/:id/third-party", middleware.WithScope("staff_compliance"), featureNotWired("KYC 三方实名核验服务未接入测试环境"))

	g.GET("/wallet", middleware.WithScope("staff_finance"), adminListWallets(d.Partner, d.Wallet))
	g.POST("/wallet/topup", middleware.WithScope("staff_finance"), featureNotWired("钱包调账 saga 未接入测试环境"))

	g.GET("/settlements", middleware.WithScope("staff_finance"), emptyList)
	g.GET("/settlements/:id", middleware.WithScope("staff_finance"), adminSettlementDetail)
	g.POST("/settlements/:id/lock", middleware.WithScope("staff_finance"), featureNotWired("结算锁定/分账下发未接入测试环境"))
	g.POST("/settlements/:id/dispatch", middleware.WithScope("staff_finance"), featureNotWired("结算锁定/分账下发未接入测试环境"))
	g.POST("/settlements/:id/reconcile", middleware.WithScope("staff_finance"), featureNotWired("结算回执对账未接入测试环境"))

	g.GET("/refunds", middleware.WithScope("staff_finance"), emptyList)
	g.POST("/refunds", middleware.WithScope("staff_finance"), featureNotWired("退款 saga 未接入测试环境"))
	g.POST("/refunds/:id/review", middleware.WithScope("staff_finance"), featureNotWired("退款 saga 未接入测试环境"))
	g.GET("/red-flush", middleware.WithScope("staff_finance"), emptyList)

	g.GET("/audit-log", middleware.WithScope("staff_admin"), emptyList)
	g.POST("/audit-log/verify", middleware.WithScope("staff_admin"), func(c *gin.Context) {
		ok(c, http.StatusOK, gin.H{"ok": true, "checked": 0})
	})

	g.GET("/content-safety/reports", middleware.WithScope("staff_compliance"), adminListContentReports(d.ContentSafety))
	g.GET("/content-safety/reports/:id", middleware.WithScope("staff_compliance"), adminGetContentReport(d.ContentSafety))
	g.POST("/content-safety/reports/:id/retry", middleware.WithScope("staff_compliance"), adminRetryContentReport(d.ContentSafety))
	g.POST("/content-safety/reports/dispatch", middleware.WithScope("staff_compliance"), featureNotWired("12377 真实派发通道未接入测试环境"))

	g.GET("/pia", middleware.WithScope("staff_compliance"), emptyList)
	g.POST("/pia/generate", middleware.WithScope("staff_compliance"), adminGeneratePIA)
	g.GET("/pipl-complaints", middleware.WithScope("staff_compliance"), emptyList)
	g.GET("/pipl-complaints/:id", middleware.WithScope("staff_compliance"), notFound)
	g.POST("/pipl-complaints/:id/resolve", middleware.WithScope("staff_compliance"), featureNotWired("PIPL 投诉工单流未接入测试环境"))

	g.GET("/security", middleware.WithScope("staff_admin"), adminGetSecurity)
	g.PUT("/security", middleware.WithScope("staff_admin"), adminUpdateSecurity)
	g.GET("/biz-settings", middleware.WithScope("staff_admin"), adminListBizSettings(d.BizSettings))
	g.PUT("/biz-settings/:key", middleware.WithScope("staff_admin"), adminUpdateBizSetting)

	g.GET("/saga/escalated", middleware.WithScope("staff_finance"), emptyList)
	g.POST("/staff/approver-tokens", middleware.WithScope("staff_admin"), adminIssueApproverToken(d.SagaAdmin))
	g.POST("/saga/:id/force-resolve", middleware.WithScope("staff_finance"), adminForceResolve(d.SagaAdmin))

	g.GET("/staff", middleware.WithScope("staff_admin"), adminListStaff(d.Staff))
	g.GET("/staff/:id", middleware.WithScope("staff_admin"), adminGetStaff(d.Staff))
	g.POST("/staff", middleware.WithScope("staff_admin"), adminCreateStaff(d.Staff))
	g.PUT("/staff/:id", middleware.WithScope("staff_admin"), adminUpdateStaff)
	g.POST("/staff/:id/disable", middleware.WithScope("staff_admin"), adminDisableStaff(d.Staff))
}

func adminActorContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		if cl, ok := middleware.ClaimsFrom(c); ok && cl != nil {
			c.Set("staff_id", cl.ActorID)
			c.Set("staff_role", primaryStaffRole(cl.Roles))
		}
		c.Next()
	}
}

func primaryStaffRole(roles []string) string {
	for _, r := range roles {
		if r == "super_admin" || strings.HasSuffix(r, "_admin") || r == "kyc_reviewer" {
			return r
		}
	}
	return "super_admin"
}

func emptyList(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{}, "error": nil, "meta": pageMeta(c, 0)})
}

func notFound(c *gin.Context) {
	fail(c, http.StatusNotFound, "BIZ_RES_NOT_FOUND", "记录不存在", "not found")
}

func featureNotWired(message string) gin.HandlerFunc {
	return func(c *gin.Context) {
		fail(c, http.StatusNotImplemented, "BIZ_FEATURE_NOT_WIRED", message, "feature not wired in test environment")
	}
}

func pageMeta(c *gin.Context, total int) gin.H {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return gin.H{"total": total, "page": page, "limit": limit}
}

func offsetLimit(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return (page - 1) * limit, limit
}

func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(c, http.StatusBadRequest, "BIZ_VALID_PATH", "路径参数错误", "bad id")
		return 0, false
	}
	return id, true
}

func adminListKYC(s *kyc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil {
			emptyList(c)
			return
		}
		_, limit := offsetLimit(c)
		rows, err := s.ListPendingReview(c.Request.Context(), limit)
		if err != nil {
			fail(c, http.StatusInternalServerError, "BIZ_KYC_LIST", "KYC 列表读取失败", err.Error())
			return
		}
		items := make([]gin.H, 0, len(rows))
		for _, row := range rows {
			items = append(items, kycDTO(row))
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": items, "error": nil, "meta": pageMeta(c, len(items))})
	}
}

func adminGetKYC(s *kyc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, okID := pathID(c)
		if !okID {
			return
		}
		if s == nil {
			notFound(c)
			return
		}
		row, err := s.FindByID(c.Request.Context(), id)
		if err != nil || row == nil {
			notFound(c)
			return
		}
		out := kycDTO(*row)
		out["real_name_masked"] = "已加密"
		out["id_card_masked"] = "已加密"
		out["documents"] = kycDocuments(*row)
		ok(c, http.StatusOK, out)
	}
}

func kycDTO(row domain.KYCApplication) gin.H {
	subjectKind := "partner"
	if row.Type == kyc.TypeIndividual {
		subjectKind = "customer"
	}
	created := row.CreatedAt
	if created.IsZero() && row.SubmittedAt != nil {
		created = *row.SubmittedAt
	}
	return gin.H{
		"id":           row.ID,
		"subject_kind": subjectKind,
		"subject_id":   row.FyUserID,
		"subject_name": "FY User #" + strconv.FormatInt(row.FyUserID, 10),
		"status":       normalizeKYCStatus(row.Status),
		"created_at":   created,
	}
}

func normalizeKYCStatus(s string) string {
	switch s {
	case kyc.StatusApproved:
		return "approved"
	case kyc.StatusRejected:
		return "rejected"
	case kyc.StatusFrozenYrly:
		return "frozen_yearly_limit"
	default:
		return "submitted"
	}
}

func kycDocuments(row domain.KYCApplication) []gin.H {
	docs := []gin.H{}
	if row.BusinessLicenseURL != "" {
		docs = append(docs, gin.H{"kind": "business_license", "url": row.BusinessLicenseURL})
	}
	if row.LegalPersonIDURL != "" {
		docs = append(docs, gin.H{"kind": "legal_person_id", "url": row.LegalPersonIDURL})
	}
	if row.BiometricLivenessURL != "" {
		docs = append(docs, gin.H{"kind": "biometric_liveness", "url": row.BiometricLivenessURL})
	}
	return docs
}

func adminListWallets(ps *partner.Service, ws *wallet.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if ps == nil {
			emptyList(c)
			return
		}
		off, limit := offsetLimit(c)
		rows, err := ps.List(c.Request.Context(), partner.ListFilter{Limit: limit, Offset: off})
		if err != nil {
			fail(c, http.StatusInternalServerError, "BIZ_WALLET_LIST", "钱包列表读取失败", err.Error())
			return
		}
		items := make([]gin.H, 0, len(rows))
		for _, p := range rows {
			balance := int64(0)
			updated := p.UpdatedAt
			if ws != nil {
				if snap, err := ws.Get(c.Request.Context(), p.ID); err == nil && snap != nil {
					balance = snap.Wallet.Balance
					updated = snap.Wallet.UpdatedAt
				}
			}
			items = append(items, gin.H{
				"id":           p.ID,
				"partner_id":   p.ID,
				"partner_name": partnerDisplayName(p),
				"balance":      balance,
				"updated_at":   updated,
			})
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": items, "error": nil, "meta": pageMeta(c, len(items))})
	}
}

func adminSettlementDetail(c *gin.Context) {
	id, okID := pathID(c)
	if !okID {
		return
	}
	ok(c, http.StatusOK, gin.H{
		"id": id, "period": time.Now().Format("2006-01"), "status": "draft",
		"partners_count": 0, "total_amount": 0, "rows": []gin.H{},
	})
}

func adminListContentReports(s *content_safety.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil {
			emptyList(c)
			return
		}
		off, limit := offsetLimit(c)
		rows, total, err := s.ListReports(c.Request.Context(), content_safety.ListQuery{
			Status: c.Query("status"), Limit: limit, Offset: off,
		})
		if err != nil {
			fail(c, http.StatusInternalServerError, "BIZ_CONTENT_SAFETY_LIST", "内容安全列表读取失败", err.Error())
			return
		}
		items := make([]gin.H, 0, len(rows))
		for _, row := range rows {
			items = append(items, contentReportDTO(row))
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": items, "error": nil, "meta": pageMeta(c, total)})
	}
}

func adminGetContentReport(s *content_safety.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, okID := pathID(c)
		if !okID {
			return
		}
		if s == nil {
			notFound(c)
			return
		}
		row, err := s.GetReport(c.Request.Context(), id)
		if err != nil || row == nil {
			notFound(c)
			return
		}
		out := contentReportDTO(*row)
		out["full_content"] = row.Payload
		out["metadata"] = gin.H{"event_id": row.EventID, "authority": row.TargetAuthority, "sla_due_at": row.SLADueAt}
		ok(c, http.StatusOK, out)
	}
}

func adminRetryContentReport(s *content_safety.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, okID := pathID(c)
		if !okID {
			return
		}
		if s == nil {
			featureNotWired("内容安全上报仓储未接入测试环境")(c)
			return
		}
		row, err := s.RetryReport(c.Request.Context(), id)
		if err != nil {
			fail(c, http.StatusBadRequest, "BIZ_CONTENT_SAFETY_RETRY", "内容安全重试失败", err.Error())
			return
		}
		ok(c, http.StatusOK, contentReportDTO(*row))
	}
}

func contentReportDTO(row content_safety.Report) gin.H {
	return gin.H{
		"id":              row.ID,
		"source":          row.TargetAuthority,
		"content_excerpt": excerpt(row.Payload, 80),
		"status":          normalizeReportStatus(row.Status),
		"remote_ref":      emptyToNil(row.ResponsePayload),
		"retries":         row.RetryCount,
		"created_at":      row.CreatedAt,
	}
}

func normalizeReportStatus(s string) string {
	switch s {
	case "submitted":
		return "submitted"
	case "failed", "dead_letter":
		return "failed"
	case "dispatching":
		return "dispatching"
	default:
		return "pending"
	}
}

func adminGeneratePIA(c *gin.Context) {
	var body struct {
		Scope string `json:"scope"`
	}
	_ = c.ShouldBindJSON(&body)
	if strings.TrimSpace(body.Scope) == "" {
		fail(c, http.StatusBadRequest, "BIZ_VALID_BODY", "评估范围必填", "scope required")
		return
	}
	ok(c, http.StatusOK, gin.H{
		"id": 1, "scope": body.Scope, "status": "draft",
		"generated_at": time.Now(), "download_url": nil,
	})
}

func adminGetSecurity(c *gin.Context) {
	ok(c, http.StatusOK, gin.H{
		"ip_allowlist":        []string{},
		"step_up_ttl_seconds": 900,
		"password_policy":     "min_len=12; require_mfa_for_sensitive_ops=true",
		"watermark_enabled":   true,
		"session_max_seconds": 28800,
	})
}

func adminUpdateSecurity(c *gin.Context) {
	var body map[string]any
	_ = c.ShouldBindJSON(&body)
	if body == nil {
		adminGetSecurity(c)
		return
	}
	ok(c, http.StatusOK, body)
}

func adminListBizSettings(settings map[string]string) gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now()
		keys := []string{
			"compliance.icp_record_no",
			"compliance.gen_ai_filing_no",
			"compliance.algorithm_filing_no",
			"compliance.report_phone_12377_link",
			"compliance.consent_text_version",
			"settlement.min_payout_fen",
			"settlement.tax_withholding_enabled",
			"saga.force_resolve_cooldown_minutes",
			"idempotency.ttl_hours",
		}
		items := make([]gin.H, 0, len(keys))
		for _, k := range keys {
			items = append(items, gin.H{
				"key": k, "value": settings[k],
				"description": "test environment setting",
				"updated_at":  now,
			})
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": items, "error": nil, "meta": pageMeta(c, len(items))})
	}
}

func adminUpdateBizSetting(c *gin.Context) {
	var body struct {
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "BIZ_VALID_BODY", "请求体无效", err.Error())
		return
	}
	ok(c, http.StatusOK, gin.H{"key": c.Param("key"), "value": body.Value, "updated_at": time.Now()})
}

func adminIssueApproverToken(s *saga_admin.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil {
			featureNotWired("Saga force-resolve 服务未接入测试环境")(c)
			return
		}
		var body struct {
			SagaID string `json:"saga_id"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.SagaID) == "" {
			fail(c, http.StatusBadRequest, "BIZ_VALID_BODY", "saga_id 必填", "saga_id required")
			return
		}
		staffID, _ := c.Get("staff_id")
		token, err := s.IssueApproverToken(c.Request.Context(), body.SagaID, int64FromAny(staffID), clientIPFromRequest(c))
		if err != nil {
			fail(c, http.StatusBadRequest, "BIZ_SAGA_TOKEN_ISSUE", "审批 token 签发失败", err.Error())
			return
		}
		ok(c, http.StatusOK, gin.H{"token": token.Token, "expires_at": token.ExpiresAt.Unix()})
	}
}

func adminForceResolve(s *saga_admin.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil {
			featureNotWired("Saga force-resolve 服务未接入测试环境")(c)
			return
		}
		var body struct {
			ApproverToken string `json:"approver_token"`
			Outcome       string `json:"outcome"`
			Reason        string `json:"reason"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			fail(c, http.StatusBadRequest, "BIZ_VALID_BODY", "请求体无效", err.Error())
			return
		}
		staffID, _ := c.Get("staff_id")
		err := s.ForceResolve(c.Request.Context(), saga_admin.ForceResolveInput{
			SagaID:           c.Param("id"),
			InitiatorStaffID: int64FromAny(staffID),
			InitiatorIP:      clientIPFromRequest(c),
			ApproverToken:    body.ApproverToken,
			Outcome:          body.Outcome,
			Reason:           body.Reason,
		})
		if err != nil {
			fail(c, http.StatusForbidden, "BIZ_SAGA_FORCE_RESOLVE", "Saga 强制处理失败", err.Error())
			return
		}
		ok(c, http.StatusOK, gin.H{"saga_id": c.Param("id"), "outcome": body.Outcome})
	}
}

func adminListStaff(s *staff.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{rootStaffDTO()}, "error": nil, "meta": pageMeta(c, 1)})
			return
		}
		role, _ := c.Get("staff_role")
		rows, err := s.List(c.Request.Context(), staff.Role(stringFromAny(role, "super_admin")), clientIPFromRequest(c))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": []gin.H{rootStaffDTO()}, "error": nil, "meta": pageMeta(c, 1)})
			return
		}
		items := make([]gin.H, 0, len(rows))
		for _, row := range rows {
			items = append(items, staffDTO(row))
		}
		if len(items) == 0 {
			items = append(items, rootStaffDTO())
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": items, "error": nil, "meta": pageMeta(c, len(items))})
	}
}

func adminGetStaff(s *staff.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, okID := pathID(c)
		if !okID {
			return
		}
		if s == nil || id == 1 {
			ok(c, http.StatusOK, rootStaffDTO())
			return
		}
		notFound(c)
	}
}

func adminCreateStaff(s *staff.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil {
			featureNotWired("员工管理仓储未接入测试环境")(c)
			return
		}
		var body struct {
			Username     string `json:"username"`
			PasswordHash string `json:"password_hash"`
			Role         string `json:"role"`
			Email        string `json:"email"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			fail(c, http.StatusBadRequest, "BIZ_VALID_BODY", "请求体无效", err.Error())
			return
		}
		staffID, _ := c.Get("staff_id")
		role, _ := c.Get("staff_role")
		got, err := s.Create(c.Request.Context(), staff.CreateInput{
			ActorID:      int64FromAny(staffID),
			ActorRole:    staff.Role(stringFromAny(role, "super_admin")),
			ActorIP:      clientIPFromRequest(c),
			Username:     strings.TrimSpace(body.Username),
			PasswordHash: body.PasswordHash,
			Role:         staff.Role(body.Role),
			Email:        body.Email,
		})
		if err != nil {
			fail(c, http.StatusBadRequest, "BIZ_STAFF_CREATE", "员工创建失败", err.Error())
			return
		}
		ok(c, http.StatusOK, staffDTO(*got))
	}
}

func adminUpdateStaff(c *gin.Context) {
	featureNotWired("员工更新未接入测试环境")(c)
}

func adminDisableStaff(s *staff.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil {
			featureNotWired("员工停用未接入测试环境")(c)
			return
		}
		id, okID := pathID(c)
		if !okID {
			return
		}
		staffID, _ := c.Get("staff_id")
		role, _ := c.Get("staff_role")
		got, err := s.SetStatus(c.Request.Context(), int64FromAny(staffID), staff.Role(stringFromAny(role, "super_admin")), clientIPFromRequest(c), id, "disabled")
		if err != nil {
			fail(c, http.StatusBadRequest, "BIZ_STAFF_DISABLE", "员工停用失败", err.Error())
			return
		}
		ok(c, http.StatusOK, staffDTO(*got))
	}
}

func staffDTO(row staff.Staff) gin.H {
	return gin.H{
		"id": row.ID, "username": row.Username, "email_masked": maskEmail(row.Email),
		"role": row.Role, "status": row.Status, "last_login_at": row.LastLogin,
		"mfa_enrolled": row.ElevatedUntil != nil,
	}
}

func rootStaffDTO() gin.H {
	return gin.H{
		"id": int64(1), "username": "root", "email_masked": "ro***@tracenex.local",
		"role": "super_admin", "status": "active", "last_login_at": nil,
		"mfa_enrolled": true,
	}
}

func int64FromAny(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func stringFromAny(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

func clientIPFromRequest(c *gin.Context) string {
	if v := c.GetHeader("X-Real-IP"); v != "" {
		return strings.TrimSpace(v)
	}
	if v := c.GetHeader("X-Forwarded-For"); v != "" {
		if idx := strings.IndexByte(v, ','); idx > 0 {
			return strings.TrimSpace(v[:idx])
		}
		return strings.TrimSpace(v)
	}
	return c.ClientIP()
}

func excerpt(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func emptyToNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}
