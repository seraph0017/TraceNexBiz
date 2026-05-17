package handler

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/middleware"
)

// RegisterPublicCompatRoutes mounts storefront public APIs that the black-box
// test exercises. They are deliberately side-effect-light except partner apply,
// which is handled by W1a.
func RegisterPublicCompatRoutes(r gin.IRouter) {
	r.GET("/api/public/models", middleware.WithScope("public"), publicModels)
	r.GET("/api/public/legal/:doc", middleware.WithScope("public"), publicLegalDoc)
	r.POST("/api/public/consent", middleware.WithScope("public"), publicConsent)
	r.POST("/api/public/kyc/presign", middleware.WithScope("public"), publicKYCPresign)
	r.PUT("/api/public/kyc/upload/:token", middleware.WithScope("public"), publicKYCUpload)
}

func publicModels(c *gin.Context) {
	ok(c, http.StatusOK, gin.H{
		"icp_license_active": false,
		"models": []gin.H{
			{
				"id": "gpt-4o-mini", "display_name": "GPT-4o mini", "vendor": "openai",
				"context_window": 128000, "price_input_per_1k": "0.0010",
				"price_output_per_1k": "0.0040", "enabled": true,
				"description": "测试环境模型展示",
			},
			{
				"id": "claude-3-5-sonnet", "display_name": "Claude 3.5 Sonnet", "vendor": "anthropic",
				"context_window": 200000, "price_input_per_1k": "0.0200",
				"price_output_per_1k": "0.0800", "enabled": true,
				"description": "测试环境模型展示",
			},
		},
	})
}

func publicLegalDoc(c *gin.Context) {
	slug := strings.TrimSpace(c.Param("doc"))
	if slug == "" {
		slug = "privacy"
	}
	title := map[string]string{
		"privacy": "隐私政策",
		"terms":   "服务条款",
		"dpo":     "个人信息保护负责人联系方式",
	}[slug]
	if title == "" {
		title = "法律文件"
	}
	ok(c, http.StatusOK, gin.H{
		"slug": slug, "title": title, "version": "2026-05-test",
		"updated_at":    time.Now().Format(time.RFC3339),
		"body_markdown": "# " + title + "\n\n测试环境用于功能联调，正式合规文本以后端配置为准。\n",
	})
}

func publicConsent(c *gin.Context) {
	var body struct {
		Scope   string `json:"scope"`
		Version string `json:"version"`
		Granted bool   `json:"granted"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || !body.Granted {
		fail(c, http.StatusBadRequest, "BIZ_VALID_CONSENT", "请先勾选同意条款", "consent required")
		return
	}
	ok(c, http.StatusOK, gin.H{
		"consent_id": time.Now().UnixNano() % 1000000000,
		"version":    body.Version,
	})
}

func publicKYCPresign(c *gin.Context) {
	var body struct {
		Kind        string `json:"kind"`
		ContentType string `json:"content_type"`
		Size        int64  `json:"size"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "BIZ_VALID_BODY", "请求体无效", err.Error())
		return
	}
	if body.Size <= 0 || body.Size > 20*1024*1024 {
		fail(c, http.StatusBadRequest, "BIZ_VALID_BODY", "文件大小超出限制", "invalid file size")
		return
	}
	token := randomHex(12)
	ok(c, http.StatusOK, gin.H{
		"upload_url":       "/api/public/kyc/upload/" + token,
		"object_url":       "/api/public/kyc/object/" + token,
		"required_headers": gin.H{"X-Tnbiz-Kyc-Kind": body.Kind},
		"expires_at":       time.Now().Add(5 * time.Minute).Format(time.RFC3339),
	})
}

func publicKYCUpload(c *gin.Context) {
	_, _ = io.Copy(io.Discard, c.Request.Body)
	c.Status(http.StatusNoContent)
}
