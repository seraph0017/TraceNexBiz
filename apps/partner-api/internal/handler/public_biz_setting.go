// Package handler — public biz_setting footer endpoint（Fix-C CRIT-C1）.
//
// 路由：GET /api/public/biz_setting/footer
// 响应：固定 8 字段（icp_record / ai_record / company_name / support_email /
//   tos_url / privacy_url / consent_text_version / 12377_link）。
//
// 缓存：5 min Redis；key="public:biz_setting:footer"；miss → DB GetMany → 序列化回填。
// fallback：DB 缺失字段 → 空串（前端 ComplianceFooter 自行处理空白条件）。
//
// scope 标记 "public"（无 actor 身份）。
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/middleware"
	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/repository/mysql"
)

// FooterPayload 前端 ComplianceFooter 消费体。
type FooterPayload struct {
	ICPRecord          string `json:"icp_record"`
	AIRecord           string `json:"ai_record"`
	CompanyName        string `json:"company_name"`
	SupportEmail       string `json:"support_email"`
	TOSURL             string `json:"tos_url"`
	PrivacyURL         string `json:"privacy_url"`
	ConsentTextVersion string `json:"consent_text_version"`
	Report12377Link    string `json:"report_12377_link"`
}

// FooterCacheKey Redis 缓存 key.
const FooterCacheKey = "public:biz_setting:footer"

// FooterCacheTTL 5 min.
const FooterCacheTTL = 5 * time.Minute

// PublicFooterHandler 装配。
//
// repo == nil（dev / W0）→ 全空 fallback（仍 200 OK）；前端不会渲染缺失项。
func PublicFooterHandler(repo *mysql.BizSettingRepository, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		// 1. cache hit?
		if rdb != nil {
			if cached, err := rdb.Get(ctx, FooterCacheKey).Bytes(); err == nil && len(cached) > 0 {
				var p FooterPayload
				if json.Unmarshal(cached, &p) == nil {
					ok(c, http.StatusOK, p)
					return
				}
			}
		}
		// 2. DB miss → build
		p := buildFooter(ctx, repo)
		// 3. cache write-through
		if rdb != nil {
			if b, err := json.Marshal(p); err == nil {
				_ = rdb.Set(ctx, FooterCacheKey, b, FooterCacheTTL).Err()
			}
		}
		ok(c, http.StatusOK, p)
	}
}

func buildFooter(ctx context.Context, repo *mysql.BizSettingRepository) FooterPayload {
	keys := []string{
		"compliance.icp_record_no",
		"compliance.gen_ai_filing_no",
		"compliance.algorithm_filing_no",
		"compliance.dpo_contact_email",
		"compliance.report_phone_12377_link",
		"footer.company_name",
		"footer.tos_url",
		"footer.privacy_url",
		"compliance.consent_text_version",
	}
	var m map[string]string
	if repo != nil {
		var err error
		m, err = repo.GetMany(ctx, keys)
		if err != nil {
			m = map[string]string{}
		}
	} else {
		m = map[string]string{}
	}
	aiRecord := m["compliance.gen_ai_filing_no"]
	if aiRecord == "" {
		aiRecord = m["compliance.algorithm_filing_no"]
	}
	tos := m["footer.tos_url"]
	if tos == "" {
		tos = "/legal/tos"
	}
	priv := m["footer.privacy_url"]
	if priv == "" {
		priv = "/legal/privacy"
	}
	cv := m["compliance.consent_text_version"]
	if cv == "" {
		cv = "v2.0"
	}
	return FooterPayload{
		ICPRecord:          m["compliance.icp_record_no"],
		AIRecord:           aiRecord,
		CompanyName:        m["footer.company_name"],
		SupportEmail:       m["compliance.dpo_contact_email"],
		TOSURL:             tos,
		PrivacyURL:         priv,
		ConsentTextVersion: cv,
		Report12377Link:    m["compliance.report_phone_12377_link"],
	}
}

// RegisterPublicFooterRoute 挂 /api/public/biz_setting/footer.
func RegisterPublicFooterRoute(r gin.IRouter, repo *mysql.BizSettingRepository, rdb *redis.Client) {
	r.GET("/api/public/biz_setting/footer",
		middleware.WithScope("public"),
		PublicFooterHandler(repo, rdb))
}
