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

// ErrRetryable 标识可重试错误（5xx / 网络 timeout / Status==0）；
// saga 层用 errors.Is(err, ErrRetryable) 决定是否走 retry/compensate。
var ErrRetryable = errors.New("fyapi: retryable")

// envelope 是 Fy-api 统一返回结构（health.go::respondJSON / respondError）。
type envelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// doAndDecode 调用 Do 并把响应映射到 envelope + retryable/non-retryable error。
//
//   - 网络错误 / Status==0 / 5xx → wrap ErrRetryable
//   - 4xx 或 envelope.Success=false → 非 retryable
//   - 2xx 且 success=true → 解析 data 到 out（out 可为 nil 表示忽略）
func (c *Client) doAndDecode(ctx context.Context, req Request, out interface{}) error {
	resp, err := c.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("%w: transport: %v", ErrRetryable, err)
	}
	if resp.Status == 0 || resp.Status >= 500 {
		return fmt.Errorf("%w: status=%d body=%s", ErrRetryable, resp.Status, truncate(resp.Body, 256))
	}

	var env envelope
	if len(resp.Body) > 0 {
		if err := json.Unmarshal(resp.Body, &env); err != nil {
			// 服务端契约破坏：4xx 不带 envelope 也算非 retryable
			return fmt.Errorf("fyapi: status=%d malformed envelope: %v body=%s", resp.Status, err, truncate(resp.Body, 256))
		}
	}

	if resp.Status >= 400 {
		msg := env.Message
		if env.Error != nil {
			msg = env.Error.Code + ": " + env.Error.Message
		}
		return fmt.Errorf("fyapi: status=%d %s", resp.Status, msg)
	}
	if !env.Success {
		msg := env.Message
		if env.Error != nil {
			msg = env.Error.Code + ": " + env.Error.Message
		}
		return fmt.Errorf("fyapi: biz error: %s", msg)
	}

	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("fyapi: decode data: %w", err)
		}
	}
	return nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "...(truncated)"
}

// TopupResponse 与 Fy-api controller/tnbiz_internal/user.go::TopupResponse 对齐。
type TopupResponse struct {
	UserID    int64 `json:"user_id"`
	NewQuota  int64 `json:"new_quota"`
	UsedQuota int64 `json:"used_quota"`
}

// TopupCustomer POST /api/internal/user/topup（integration §2.2.2）。
//
// Fy-api 端字段名为 `quota`（绝对增量，>0），不是 spec 文案里的 `amount`；
// 我们以服务端为准（参见 controller/tnbiz_internal/user.go::TopupRequest）。
func (c *Client) TopupCustomer(ctx context.Context, fyUserID, amount int64, idemKey, traceID string) error {
	if fyUserID <= 0 {
		return errors.New("fyapi: fy_user_id required")
	}
	if amount <= 0 {
		return errors.New("fyapi: amount must be > 0")
	}
	if idemKey == "" {
		return errors.New("fyapi: idempotency key required")
	}
	body := map[string]interface{}{
		"user_id": fyUserID,
		"quota":   amount,
		"reason":  "partner_allocate",
	}
	var out TopupResponse
	return c.doAndDecode(ctx, Request{
		Method:         http.MethodPost,
		Path:           "/api/internal/user/topup",
		Body:           body,
		IdempotencyKey: idemKey,
		TraceID:        traceID,
	}, &out)
}

// RefundCustomer POST /api/internal/user/refund（integration §2.2.3 deduct 的实现：
// Fy-api 端 handler 名为 Refund，路径 /user/refund，语义对称 — quota>0 加回客户余额）。
func (c *Client) RefundCustomer(ctx context.Context, fyUserID, amount int64, idemKey, traceID string) error {
	if fyUserID <= 0 {
		return errors.New("fyapi: fy_user_id required")
	}
	if amount <= 0 {
		return errors.New("fyapi: amount must be > 0")
	}
	if idemKey == "" {
		return errors.New("fyapi: idempotency key required")
	}
	body := map[string]interface{}{
		"user_id": fyUserID,
		"quota":   amount,
		"saga_id": idemKey,
	}
	var out TopupResponse
	return c.doAndDecode(ctx, Request{
		Method:         http.MethodPost,
		Path:           "/api/internal/user/refund",
		Body:           body,
		IdempotencyKey: idemKey,
		TraceID:        traceID,
	}, &out)
}

// UpdateUserGroup PUT /api/internal/user/group（integration §2.2.5）。
//
// TODO(Fy-api): 目前 Fy-api 端尚未实现 /user/group handler（仅
// router 未挂载；controller/tnbiz_internal 无对应 func）。一旦
// Fy-api 端 PR 落地，本方法直接生效；此处保留客户端调用形态便于联调。
func (c *Client) UpdateUserGroup(ctx context.Context, fyUserID int64, group, idemKey string) error {
	if fyUserID <= 0 {
		return errors.New("fyapi: fy_user_id required")
	}
	if group == "" {
		return errors.New("fyapi: group required")
	}
	if idemKey == "" {
		return errors.New("fyapi: idempotency key required")
	}
	return errors.New("fyapi: not yet implemented on Fy-api side: PUT /api/internal/user/group")
}

// EraseUser POST /api/internal/user/erase（integration §2.2.12）。
//
// TODO(Fy-api): handler 尚未在 Fy-api 仓库实现。
func (c *Client) EraseUser(ctx context.Context, fyUserID int64, idemKey string) error {
	if fyUserID <= 0 {
		return errors.New("fyapi: fy_user_id required")
	}
	if idemKey == "" {
		return errors.New("fyapi: idempotency key required")
	}
	return errors.New("fyapi: not yet implemented on Fy-api side: POST /api/internal/user/erase")
}

// QuotaResponse 与 Fy-api controller/tnbiz_internal/user.go::QuotaResponse 对齐。
type QuotaResponse struct {
	UserID    int64 `json:"user_id"`
	Quota     int64 `json:"quota"`
	UsedQuota int64 `json:"used_quota"`
	AffQuota  int64 `json:"aff_quota"`
}

// QuotaInfo is the full quota snapshot returned by Fy-api.
type QuotaInfo = QuotaResponse

// GetUserQuota GET /api/internal/user/quota?user_id=...
//
// 返回客户完整 quota snapshot；GET 不需 idempotency key。
func (c *Client) GetUserQuota(ctx context.Context, fyUserID int64) (*QuotaInfo, error) {
	if fyUserID <= 0 {
		return nil, errors.New("fyapi: fy_user_id required")
	}
	q := url.Values{}
	q.Set("user_id", strconv.FormatInt(fyUserID, 10))
	var out QuotaResponse
	if err := c.doAndDecode(ctx, Request{
		Method: http.MethodGet,
		Path:   "/api/internal/user/quota",
		Query:  q,
	}, &out); err != nil {
		return nil, err
	}
	return (*QuotaInfo)(&out), nil
}

// GetUserQuotaBalance returns only the quota balance for legacy callers.
func (c *Client) GetUserQuotaBalance(ctx context.Context, fyUserID int64) (int64, error) {
	info, err := c.GetUserQuota(ctx, fyUserID)
	if err != nil {
		return 0, err
	}
	return info.Quota, nil
}

// CreateTokenRequest 透传到 Fy-api/controller/tnbiz_internal/token.go::CreateTokenRequest。
type CreateTokenRequest struct {
	UserID         int64    `json:"user_id"`
	Name           string   `json:"name"`
	Group          string   `json:"group,omitempty"`
	UnlimitedQuota bool     `json:"unlimited_quota,omitempty"`
	RemainQuota    int64    `json:"remain_quota,omitempty"`
	ExpiredAt      int64    `json:"expired_at,omitempty"`
	ModelLimits    []string `json:"model_limits,omitempty"`
}

// CreateTokenResponse partner 永远拿不到明文 sk-key，只有 masked + DeliveryHandle。
type CreateTokenResponse struct {
	TokenID        int64  `json:"token_id"`
	MaskedKey      string `json:"masked_key"`
	DeliveryHandle string `json:"delivery_handle"`
}

// TokenCreate POST /api/internal/token/create（integration §2.2.4）。
func (c *Client) TokenCreate(ctx context.Context, req CreateTokenRequest, idemKey, traceID string) (*CreateTokenResponse, error) {
	if req.UserID <= 0 {
		return nil, errors.New("fyapi: user_id required")
	}
	if req.Name == "" {
		return nil, errors.New("fyapi: name required")
	}
	if idemKey == "" {
		return nil, errors.New("fyapi: idempotency key required")
	}
	var out CreateTokenResponse
	if err := c.doAndDecode(ctx, Request{
		Method:         http.MethodPost,
		Path:           "/api/internal/token/create",
		Body:           req,
		IdempotencyKey: idemKey,
		TraceID:        traceID,
	}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GroupRatioOverrideUpsert POST /api/internal/group_ratio_override/upsert
// （integration §2.2.6 — Fy-api router 实际用 POST，与 OpenAPI 文案的 PUT 不同；
// 以 router/api-internal-router.go 为准）。
func (c *Client) GroupRatioOverrideUpsert(ctx context.Context, fyUserID int64, group string, ratio float64, idemKey, traceID string) error {
	if fyUserID <= 0 {
		return errors.New("fyapi: fy_user_id required")
	}
	if group == "" {
		return errors.New("fyapi: group required")
	}
	if ratio < 0 {
		return errors.New("fyapi: ratio must be >= 0")
	}
	if idemKey == "" {
		return errors.New("fyapi: idempotency key required")
	}
	body := map[string]interface{}{
		"user_id": fyUserID,
		"group":   group,
		"ratio":   ratio,
	}
	return c.doAndDecode(ctx, Request{
		Method:         http.MethodPost,
		Path:           "/api/internal/group_ratio_override/upsert",
		Body:           body,
		IdempotencyKey: idemKey,
		TraceID:        traceID,
	}, nil)
}
