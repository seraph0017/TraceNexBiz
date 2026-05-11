// Package admin 暴露 W1c admin 端点（PRD §7.4 + backend §4.5）.
//
// 仅 staff 鉴权（W1a JWT middleware 由路由层挂上；本包 handler 函数体只做 service 调用 + envelope）.
// 每条 endpoint 写入 audit_log_unsealed（W1a Audit middleware）.
//
// 已实现 endpoint（≥ 5，验收阈值）：
//
//	1. POST /api/admin/invoice/:id/review              发票审核
//	2. POST /api/admin/invoice/:id/issue               发票出票
//	3. POST /api/admin/invoice/:id/red-flush           发票红冲（M8）
//	4. GET  /api/admin/tickets                         工单 drill-down
//	5. POST /api/admin/tickets/:id/assign              工单分派
//	6. GET  /api/admin/content-safety/reports          12377 上报队列列表
//	7. POST /api/admin/content-safety/reports/:id/retry 12377 重试
//	8. POST /api/admin/content-safety/reports/dispatch  12377 一键派发（dispatcher 触发）
//	9. POST /api/admin/staff                           staff 创建（super_admin only）
//	10. POST /api/admin/saga/:id/force-resolve         dual-control force_resolve
package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/content_safety"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/invoice"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/saga_admin"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/staff"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/ticket"
)

// Deps 依赖注入：W1c 服务集合（任何 nil 服务对应路由会 503）.
type Deps struct {
	Invoice       *invoice.Service
	Ticket        *ticket.Service
	ContentSafety *content_safety.Service
	Staff         *staff.Service
	SagaAdmin     *saga_admin.Service
}

// Register 在 gin.RouterGroup 上挂 admin 端点.
//
// 调用方负责在 group 上预先挂好：JWT (staff) + IP allowlist + Audit middleware.
func Register(rg *gin.RouterGroup, deps Deps) {
	rg.POST("/invoice/:id/review", invoiceReview(deps.Invoice))
	rg.POST("/invoice/:id/issue", invoiceIssue(deps.Invoice))
	rg.POST("/invoice/:id/red-flush", invoiceRedFlush(deps.Invoice))

	rg.GET("/tickets", ticketList(deps.Ticket))
	rg.POST("/tickets/:id/assign", ticketAssign(deps.Ticket))

	rg.GET("/content-safety/reports", csReportsList(deps.ContentSafety))
	rg.POST("/content-safety/reports/:id/retry", csReportRetry(deps.ContentSafety))
	rg.POST("/content-safety/reports/dispatch", csReportDispatch(deps.ContentSafety))

	rg.POST("/staff", staffCreate(deps.Staff))

	rg.POST("/saga/:id/force-resolve", sagaForceResolve(deps.SagaAdmin))
}

// Envelope per integration §8.1.
func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data, "error": nil})
}

func bad(c *gin.Context, status int, code, msg string) {
	c.JSON(status, gin.H{
		"success": false,
		"data":    nil,
		"error": gin.H{
			"code":       code,
			"message_zh": msg,
			"message_en": msg,
			"trace_id":   c.GetString("trace_id"),
		},
	})
}

func unavailable(c *gin.Context, what string) {
	bad(c, http.StatusServiceUnavailable, "SYS_DEP_UNAVAILABLE", what+" not wired")
}

func parseID(c *gin.Context) (int64, bool) {
	raw := c.Param("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		bad(c, http.StatusBadRequest, "BIZ_VALID_ID_INVALID", "invalid id: "+raw)
		return 0, false
	}
	return id, true
}

// ------------------------- invoice -------------------------

type reviewBody struct {
	Approve     bool   `json:"approve"`
	RejectCode  string `json:"reject_code"`
	RejectText  string `json:"reject_text"`
}

func invoiceReview(svc *invoice.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			unavailable(c, "invoice")
			return
		}
		id, okID := parseID(c)
		if !okID {
			return
		}
		var body reviewBody
		if err := c.ShouldBindJSON(&body); err != nil {
			bad(c, http.StatusBadRequest, "BIZ_VALID_BODY", err.Error())
			return
		}
		app, err := svc.Review(c.Request.Context(), id, body.Approve, body.RejectCode, body.RejectText)
		if err != nil {
			bad(c, http.StatusBadRequest, "BIZ_INVOICE_REVIEW", err.Error())
			return
		}
		ok(c, app)
	}
}

func invoiceIssue(svc *invoice.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			unavailable(c, "invoice")
			return
		}
		id, okID := parseID(c)
		if !okID {
			return
		}
		app, err := svc.Issue(c.Request.Context(), id)
		if err != nil {
			bad(c, http.StatusBadRequest, "BIZ_INVOICE_ISSUE", err.Error())
			return
		}
		ok(c, app)
	}
}

type redFlushBody struct {
	ReasonCode string `json:"reason_code" binding:"required"`
	ReasonText string `json:"reason_text"`
}

func invoiceRedFlush(svc *invoice.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			unavailable(c, "invoice")
			return
		}
		id, okID := parseID(c)
		if !okID {
			return
		}
		var body redFlushBody
		if err := c.ShouldBindJSON(&body); err != nil {
			bad(c, http.StatusBadRequest, "BIZ_VALID_BODY", err.Error())
			return
		}
		actorID, _ := c.Get("staff_id")
		actorIDInt, _ := actorID.(int64)
		app, rf, err := svc.RedFlush(c.Request.Context(), id, body.ReasonCode, body.ReasonText, actorIDInt)
		if err != nil {
			bad(c, http.StatusBadRequest, "BIZ_INVOICE_RED_FLUSH", err.Error())
			return
		}
		ok(c, gin.H{"application": app, "red_flush": rf})
	}
}

// ------------------------- ticket -------------------------

func ticketList(svc *ticket.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			unavailable(c, "ticket")
			return
		}
		q := ticket.ListQuery{
			OpenerType: c.Query("opener_type"),
			Status:     c.Query("status"),
			Category:   c.Query("category"),
			Limit:      atoiOrDefault(c.Query("limit"), 50),
			Offset:     atoiOrDefault(c.Query("offset"), 0),
		}
		list, total, err := svc.AdminList(c.Request.Context(), q)
		if err != nil {
			bad(c, http.StatusInternalServerError, "BIZ_TICKET_LIST", err.Error())
			return
		}
		ok(c, gin.H{"items": list, "total": total})
	}
}

type assignBody struct {
	StaffID int64 `json:"staff_id" binding:"required"`
}

func ticketAssign(svc *ticket.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			unavailable(c, "ticket")
			return
		}
		id, okID := parseID(c)
		if !okID {
			return
		}
		var body assignBody
		if err := c.ShouldBindJSON(&body); err != nil {
			bad(c, http.StatusBadRequest, "BIZ_VALID_BODY", err.Error())
			return
		}
		t, err := svc.Assign(c.Request.Context(), id, body.StaffID)
		if err != nil {
			bad(c, http.StatusBadRequest, "BIZ_TICKET_ASSIGN", err.Error())
			return
		}
		ok(c, t)
	}
}

// ------------------------- content_safety -------------------------

func csReportsList(svc *content_safety.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			unavailable(c, "content_safety")
			return
		}
		list, total, err := svc.ListReports(c.Request.Context(), content_safety.ListQuery{
			Status: c.Query("status"),
			Limit:  atoiOrDefault(c.Query("limit"), 50),
			Offset: atoiOrDefault(c.Query("offset"), 0),
		})
		if err != nil {
			bad(c, http.StatusInternalServerError, "BIZ_CS_LIST", err.Error())
			return
		}
		ok(c, gin.H{"items": list, "total": total})
	}
}

func csReportRetry(svc *content_safety.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			unavailable(c, "content_safety")
			return
		}
		id, okID := parseID(c)
		if !okID {
			return
		}
		r, err := svc.RetryReport(c.Request.Context(), id)
		if err != nil {
			bad(c, http.StatusBadRequest, "BIZ_CS_RETRY", err.Error())
			return
		}
		ok(c, r)
	}
}

type dispatchBody struct {
	Batch int `json:"batch"`
}

func csReportDispatch(svc *content_safety.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			unavailable(c, "content_safety")
			return
		}
		var body dispatchBody
		_ = c.ShouldBindJSON(&body)
		if body.Batch <= 0 {
			body.Batch = 50
		}
		sub, fail, err := svc.DispatchOnce(c.Request.Context(), body.Batch)
		if err != nil {
			bad(c, http.StatusInternalServerError, "BIZ_CS_DISPATCH", err.Error())
			return
		}
		ok(c, gin.H{"submitted": sub, "failed": fail})
	}
}

// ------------------------- staff -------------------------

type staffCreateBody struct {
	Username     string `json:"username" binding:"required"`
	PasswordHash string `json:"password_hash" binding:"required"`
	Role         string `json:"role" binding:"required"`
	Email        string `json:"email"`
}

func staffCreate(svc *staff.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			unavailable(c, "staff")
			return
		}
		var body staffCreateBody
		if err := c.ShouldBindJSON(&body); err != nil {
			bad(c, http.StatusBadRequest, "BIZ_VALID_BODY", err.Error())
			return
		}
		actorID, _ := c.Get("staff_id")
		actorRole, _ := c.Get("staff_role")
		actorIDInt, _ := actorID.(int64)
		actorRoleStr, _ := actorRole.(string)
		st, err := svc.Create(c.Request.Context(), staff.CreateInput{
			ActorID:      actorIDInt,
			ActorRole:    staff.Role(actorRoleStr),
			ActorIP:      clientIP(c),
			Username:     body.Username,
			PasswordHash: body.PasswordHash,
			Role:         staff.Role(body.Role),
			Email:        body.Email,
		})
		if err != nil {
			bad(c, http.StatusForbidden, "BIZ_STAFF_CREATE", err.Error())
			return
		}
		ok(c, st)
	}
}

// ------------------------- saga force_resolve -------------------------

type forceResolveBody struct {
	ApproverToken string `json:"approver_token" binding:"required"`
	ApproverIP    string `json:"approver_ip" binding:"required"`
	Outcome       string `json:"outcome" binding:"required"`
	Reason        string `json:"reason"`
}

func sagaForceResolve(svc *saga_admin.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			unavailable(c, "saga_admin")
			return
		}
		sagaID := c.Param("id")
		if sagaID == "" {
			bad(c, http.StatusBadRequest, "BIZ_VALID_ID_INVALID", "saga_id required")
			return
		}
		var body forceResolveBody
		if err := c.ShouldBindJSON(&body); err != nil {
			bad(c, http.StatusBadRequest, "BIZ_VALID_BODY", err.Error())
			return
		}
		actorID, _ := c.Get("staff_id")
		actorIDInt, _ := actorID.(int64)
		err := svc.ForceResolve(c.Request.Context(), saga_admin.ForceResolveInput{
			SagaID:           sagaID,
			InitiatorStaffID: actorIDInt,
			InitiatorIP:      clientIP(c),
			ApproverIP:       body.ApproverIP,
			ApproverToken:    body.ApproverToken,
			Outcome:          body.Outcome,
			Reason:           body.Reason,
		})
		if err != nil {
			bad(c, http.StatusForbidden, "BIZ_SAGA_FORCE_RESOLVE", err.Error())
			return
		}
		ok(c, gin.H{"saga_id": sagaID, "outcome": body.Outcome})
	}
}

// ------------------------- helpers -------------------------

func atoiOrDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func clientIP(c *gin.Context) string {
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
