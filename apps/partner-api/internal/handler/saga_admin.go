// Package handler — admin saga force-resolve + dispute 终审 endpoint（暴露给 W1c admin group）.
//
// 路径（待 W1c 接入 admin router 时挂载）：
//
//	POST /admin/saga/force-resolve   dual-control 强制收口 escalated saga
//	POST /admin/dispute/:id/accept   终审通过（联动 refund saga）
//	POST /admin/dispute/:id/reject   终审驳回
//
// 本文件只暴露 handler factory；W1c 负责接 admin middleware（JWT + RBAC + step-up MFA + audit_log）.
package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/saga"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/service/dispute"
)

// SagaAdminDeps W1c 装配时注入.
type SagaAdminDeps struct {
	SagaRepo       saga.Repository
	DisputeService *dispute.Service
}

// ForceResolveBody admin 提交参数；token 由 admin 后台先签发.
type ForceResolveBody struct {
	SagaID         string `json:"saga_id" binding:"required"`
	StepName       string `json:"step_name" binding:"required"`
	Target         string `json:"target" binding:"required"` // committed / compensated / released_pessimistic
	ApproverID     int64  `json:"approver_id" binding:"required"`
	ApproverIP     string `json:"approver_ip" binding:"required"`
	Reason         string `json:"reason" binding:"required"`
	TokenIssuedAt  int64  `json:"token_issued_at" binding:"required"` // unix seconds
}

// NewSagaForceResolveHandler 构造 dual-control force-resolve endpoint.
//
// 使用方式（W1c）：
//
//	admin.POST("/saga/force-resolve",
//	    mw.AuthAdmin(), mw.RequireStepUp(), mw.RequireRole("super_admin"),
//	    handler.NewSagaForceResolveHandler(deps))
func NewSagaForceResolveHandler(deps SagaAdminDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body ForceResolveBody
		if err := c.ShouldBindJSON(&body); err != nil {
			respondError(c, http.StatusBadRequest, "BIZ_INVALID_INPUT", err.Error())
			return
		}
		actorID, ok := c.Get("staff_id")
		if !ok {
			respondError(c, http.StatusUnauthorized, "BIZ_AUTH_REQUIRED", "missing staff context")
			return
		}
		actorIDV, _ := actorID.(int64)
		actorIP := c.ClientIP()

		// 上次 force-resolve 时间从 audit_log 拿；W1c 接 audit_log query 后注入.
		req := saga.ForceResolveRequest{
			SagaID:        body.SagaID,
			StepName:      body.StepName,
			Target:        saga.Status(body.Target),
			ActorID:       actorIDV,
			ActorIP:       actorIP,
			ApproverID:    body.ApproverID,
			ApproverIP:    body.ApproverIP,
			TokenIssuedAt: time.Unix(body.TokenIssuedAt, 0),
			TokenConsumed: false, // caller 应用 Redis SETNX 标记后传 false 进入；本 handler 不直接校验 token store
			Now:           time.Now().UTC(),
			Reason:        body.Reason,
		}
		if err := saga.ValidateForceResolve(req); err != nil {
			respondError(c, http.StatusForbidden, "BIZ_DUAL_CONTROL_REJECTED", err.Error())
			return
		}
		if err := deps.SagaRepo.ForceResolve(c.Request.Context(), body.SagaID, body.StepName, saga.Status(body.Target)); err != nil {
			respondError(c, http.StatusInternalServerError, "BIZ_SAGA_FORCE_RESOLVE_FAILED", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"saga_id":   body.SagaID,
				"step_name": body.StepName,
				"target":    body.Target,
			},
		})
	}
}

// NewDisputeAcceptHandler 终审通过.
func NewDisputeAcceptHandler(deps SagaAdminDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := parseInt64(idStr)
		if err != nil {
			respondError(c, http.StatusBadRequest, "BIZ_INVALID_INPUT", "id required")
			return
		}
		actorID := c.GetInt64("staff_id")
		var body struct {
			Reason string `json:"reason"`
		}
		_ = c.ShouldBindJSON(&body)
		if err := deps.DisputeService.FinalizeAccept(c.Request.Context(), id, actorID, actorID); err != nil {
			respondError(c, http.StatusBadRequest, "BIZ_DISPUTE_FINALIZE_FAILED", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// NewDisputeRejectHandler 终审驳回.
func NewDisputeRejectHandler(deps SagaAdminDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := parseInt64(idStr)
		if err != nil {
			respondError(c, http.StatusBadRequest, "BIZ_INVALID_INPUT", "id required")
			return
		}
		actorID := c.GetInt64("staff_id")
		if err := deps.DisputeService.FinalizeReject(c.Request.Context(), id, actorID); err != nil {
			respondError(c, http.StatusBadRequest, "BIZ_DISPUTE_FINALIZE_FAILED", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

func respondError(c *gin.Context, status int, code, msg string) {
	c.JSON(status, gin.H{
		"success": false,
		"error": gin.H{
			"code":       code,
			"message_en": msg,
			"message_zh": msg,
			"trace_id":   c.GetString("trace_id"),
		},
	})
}

func parseInt64(s string) (int64, error) {
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errInvalidNumber
		}
		n = n*10 + int64(r-'0')
	}
	if n == 0 {
		return 0, errInvalidNumber
	}
	return n, nil
}

var errInvalidNumber = &handlerErr{"invalid number"}

type handlerErr struct{ s string }

func (e *handlerErr) Error() string { return e.s }
