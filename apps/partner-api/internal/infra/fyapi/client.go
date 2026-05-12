// Package fyapi 是 Fy-api `/api/internal/*` 客户端。
//
// 引用：integration §1 / §2 / §17（HMAC 签名 4 元组）。
//
// 关键约束：
//   - 唯一允许调用 /api/internal/* 的 package（per overview I-3.3）
//   - 4 个 X-Auth header：X-Auth-KeyId / X-Auth-Timestamp / X-Auth-Nonce / X-Signature
//   - HMAC-SHA256 签名 over canonical(method, path, query, body, ts, nonce)
//   - 5xx / timeout → caller 决定走 saga retry
//   - 透传 Idempotency-Key 给 Fy-api（24h vs 7d TTL，取较长，per overview §4.4）
//   - 透传 X-Oneapi-Request-Id（trace_id）
//
// W0：定义接口 + HMAC 签名工具；具体 endpoint 方法由 W1b 落地。
package fyapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/internal/config"
)

// Client 是 Fy-api 内部 SDK；线程安全。
type Client struct {
	baseURL    string
	keyID      string
	hmacSecret []byte
	httpClient *http.Client
}

// NewClient 构造 client；HMAC secret 应从 KMS Secret Manager 注入（per ADR-010）。
func NewClient(cfg *config.Config) (*Client, error) {
	if cfg.FyAPI.BaseURL == "" {
		return nil, errors.New("FYAPI_BASE_URL is empty")
	}
	return &Client{
		baseURL:    cfg.FyAPI.BaseURL,
		keyID:      cfg.FyAPI.HMACKeyID,
		hmacSecret: []byte(cfg.FyAPI.HMACSecret),
		httpClient: &http.Client{
			Timeout: cfg.FyAPI.Timeout,
		},
	}, nil
}

// Request 描述一次内部 API 调用。调用方不应直接构造 *http.Request。
type Request struct {
	Method         string // GET / POST / PUT / DELETE
	Path           string // 必须以 /api/internal/ 开头
	Query          url.Values
	Body           interface{} // 自动 JSON marshal
	IdempotencyKey string      // state-changing 必传
	TraceID        string      // 由 caller 透传
}

// Response 解 envelope 后的结果。
type Response struct {
	Status int
	Body   []byte
}

// Do 执行单次调用；不做重试（重试由 saga 层决定）。
func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	if req.Method == "" || req.Path == "" {
		return nil, errors.New("fyapi: method/path required")
	}
	if !isInternalPath(req.Path) {
		return nil, fmt.Errorf("fyapi: path must start with /api/internal/, got %q", req.Path)
	}

	var body []byte
	if req.Body != nil {
		var err error
		body, err = json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("fyapi: marshal body: %w", err)
		}
	}

	rawQuery := req.Query.Encode()
	urlStr := c.baseURL + req.Path
	if rawQuery != "" {
		urlStr += "?" + rawQuery
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, urlStr, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// HMAC 4 元组（integration-design v1.2 §1.1.3 权威）
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := uuid.NewString()
	sig := c.sign(req.Method, req.Path, rawQuery, body, ts, nonce)

	httpReq.Header.Set("X-Auth-KeyId", c.keyID)
	httpReq.Header.Set("X-Auth-Timestamp", ts)
	httpReq.Header.Set("X-Auth-Nonce", nonce)
	httpReq.Header.Set("X-Signature", sig)

	if req.IdempotencyKey != "" {
		httpReq.Header.Set("Idempotency-Key", req.IdempotencyKey)
	}
	if req.TraceID != "" {
		httpReq.Header.Set("X-Oneapi-Request-Id", req.TraceID)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &Response{Status: resp.StatusCode, Body: respBody}, nil
}

// sign 计算 HMAC-SHA256 签名（integration-design v1.2 §1.1.3 权威 canonical）。
//
//	canonical = METHOD\nPATH\ncanonical_query\nTS\nNONCE\nSHA256_HEX(body)
//
// METHOD uppercase；canonical_query 按 key 字典序 RFC3986 编码；签名输出 base64。
// 与 Fy-api middleware/internal_auth.go::BuildCanonical 字节级一致。
func (c *Client) sign(method, path, rawQuery string, body []byte, ts, nonce string) string {
	bodyHash := sha256.Sum256(body)
	canonical := strings.Join([]string{
		strings.ToUpper(method),
		path,
		canonicalQuery(rawQuery),
		ts,
		nonce,
		hex.EncodeToString(bodyHash[:]),
	}, "\n")

	mac := hmac.New(sha256.New, c.hmacSecret)
	mac.Write([]byte(canonical))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// canonicalQuery 与 Fy-api 端 canonicalQuery 完全一致：按 key 字典序，每对 RFC3986 编码。
func canonicalQuery(raw string) string {
	if raw == "" {
		return ""
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return ""
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(values))
	for _, k := range keys {
		vs := values[k]
		sort.Strings(vs)
		ek := url.QueryEscape(k)
		for _, v := range vs {
			parts = append(parts, ek+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

func isInternalPath(p string) bool {
	const prefix = "/api/internal/"
	return len(p) >= len(prefix) && p[:len(prefix)] == prefix
}

// 以下为 W1b 必须实现的 endpoint 方法占位。
//
// 全部按 integration §2 字段级 schema 实现，每个方法都必须：
//   1) 透传 ctx、Idempotency-Key、TraceID
//   2) 5xx / timeout 包成 retryable error；非 retryable 不进 saga retry
//   3) 解 envelope；返回 typed result + biz error code

// CreateUser TODO(W1b): per integration §2 - POST /api/internal/user/create
func (c *Client) CreateUser(_ context.Context, _ interface{}) (interface{}, error) {
	return nil, errors.New("fyapi: CreateUser not implemented; W1b to wire per integration §2")
}

// SetGroupRatioOverride TODO(W1b): per integration §2 - POST /api/internal/user/group-ratio-override
func (c *Client) SetGroupRatioOverride(_ context.Context, _ interface{}) (interface{}, error) {
	return nil, errors.New("fyapi: SetGroupRatioOverride not implemented; W1b to wire per integration §2")
}

// AdjustQuota TODO(W1b): per integration §2 - POST /api/internal/user/quota/adjust
func (c *Client) AdjustQuota(_ context.Context, _ interface{}) (interface{}, error) {
	return nil, errors.New("fyapi: AdjustQuota not implemented; W1b to wire per integration §2")
}

// CreateToken TODO(W1b): per integration §2 - POST /api/internal/token/create
func (c *Client) CreateToken(_ context.Context, _ interface{}) (interface{}, error) {
	return nil, errors.New("fyapi: CreateToken not implemented; W1b to wire per integration §2")
}

// GetUsage TODO(W1b): per integration §2 - GET /api/internal/usage/*
func (c *Client) GetUsage(_ context.Context, _ interface{}) (interface{}, error) {
	return nil, errors.New("fyapi: GetUsage not implemented; W1b to wire per integration §2")
}
