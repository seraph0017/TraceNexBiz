package fyapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"
)

// newTestClient builds a Client pointed at the given httptest server.
func newTestClient(srv *httptest.Server) *Client {
	return &Client{
		baseURL:    srv.URL,
		keyID:      "kid-test",
		hmacSecret: []byte("test-secret-32-bytes-min-len-CSPRNG-x"),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// envelopeOK returns the success envelope shape Fy-api emits.
func envelopeOK(data string) string {
	if data == "" {
		data = "null"
	}
	return `{"success":true,"data":` + data + `}`
}

func envelopeErr(code, msg string) string {
	return `{"success":false,"error":{"code":"` + code + `","message":"` + msg + `"}}`
}

// TestTopupCustomer_Success 验证 200 + envelope.success=true 解码为 nil error，
// 且 4 个 X-Auth header + Idempotency-Key + X-Oneapi-Request-Id 实际上线。
func TestTopupCustomer_Success(t *testing.T) {
	var gotPath, gotMethod, gotIdem, gotTrace, gotKid, gotSig string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotIdem = r.Header.Get("Idempotency-Key")
		gotTrace = r.Header.Get("X-Oneapi-Request-Id")
		gotKid = r.Header.Get("X-Auth-KeyId")
		gotSig = r.Header.Get("X-Signature")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(envelopeOK(`{"user_id":42,"new_quota":1100,"used_quota":50}`)))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	if err := c.TopupCustomer(context.Background(), 42, 1000, "idem-1", "trace-1"); err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if gotMethod != "POST" || gotPath != "/api/internal/user/topup" {
		t.Fatalf("wrong route: %s %s", gotMethod, gotPath)
	}
	if gotIdem != "idem-1" {
		t.Fatalf("Idempotency-Key not propagated: %q", gotIdem)
	}
	if gotTrace != "trace-1" {
		t.Fatalf("X-Oneapi-Request-Id not propagated: %q", gotTrace)
	}
	if gotKid != "kid-test" || gotSig == "" {
		t.Fatalf("HMAC headers missing: kid=%q sig=%q", gotKid, gotSig)
	}
	if !strings.Contains(string(gotBody), `"quota":1000`) {
		t.Fatalf("body schema wrong (must use 'quota' field): %s", gotBody)
	}
}

func TestTopupCustomer_Unauthorized_NonRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(envelopeErr("unauthorized", "bad signature")))
	}))
	defer srv.Close()
	c := newTestClient(srv)
	err := c.TopupCustomer(context.Background(), 42, 1000, "idem-1", "trace-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrRetryable) {
		t.Fatalf("401 must be non-retryable, got %v", err)
	}
}

func TestTopupCustomer_5xx_Retryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"success":false,"error":{"code":"upstream","message":"bad"}}`))
	}))
	defer srv.Close()
	c := newTestClient(srv)
	err := c.TopupCustomer(context.Background(), 42, 1000, "idem-1", "trace-1")
	if !errors.Is(err, ErrRetryable) {
		t.Fatalf("502 must be retryable, got %v", err)
	}
}

func TestTopupCustomer_Validation(t *testing.T) {
	c := newTestClient(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	cases := []struct {
		name     string
		uid, amt int64
		idem     string
	}{
		{"bad uid", 0, 1000, "k"},
		{"bad amount", 1, 0, "k"},
		{"missing idem", 1, 1000, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := c.TopupCustomer(context.Background(), tc.uid, tc.amt, tc.idem, "tr")
			if err == nil {
				t.Fatal("expected validation error")
			}
			if errors.Is(err, ErrRetryable) {
				t.Fatalf("validation error must be non-retryable, got %v", err)
			}
		})
	}
}

func TestRefundCustomer_Success(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(envelopeOK(`{"user_id":42,"new_quota":1100,"used_quota":50}`)))
	}))
	defer srv.Close()
	c := newTestClient(srv)
	if err := c.RefundCustomer(context.Background(), 42, 500, "idem-r", "tr"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotPath != "/api/internal/user/refund" {
		t.Fatalf("wrong path: %s", gotPath)
	}
	if gotBody["saga_id"] != "idem-r" {
		t.Fatalf("saga_id=%v", gotBody["saga_id"])
	}
	if _, ok := gotBody["order_ref"]; ok {
		t.Fatalf("order_ref must not be populated from trace id: %v", gotBody)
	}
}

func TestGetUserQuota_Success(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(envelopeOK(`{"user_id":42,"quota":777,"used_quota":3,"aff_quota":0}`)))
	}))
	defer srv.Close()
	c := newTestClient(srv)
	q, err := c.GetUserQuota(context.Background(), 42)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if q.Quota != 777 || q.UsedQuota != 3 || q.AffQuota != 0 {
		t.Fatalf("quota info=%+v", q)
	}
	if !strings.Contains(gotQuery, "user_id=42") {
		t.Fatalf("query wrong: %s", gotQuery)
	}
}

func TestGetUserQuota_Validation(t *testing.T) {
	c := &Client{baseURL: "http://x", hmacSecret: []byte("k"), httpClient: http.DefaultClient}
	if _, err := c.GetUserQuota(context.Background(), 0); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestTokenCreate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(envelopeOK(`{"token_id":99,"masked_key":"sk-***","delivery_handle":"h-abc"}`)))
	}))
	defer srv.Close()
	c := newTestClient(srv)
	out, err := c.TokenCreate(context.Background(), CreateTokenRequest{UserID: 42, Name: "default"}, "idem-t", "tr")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.TokenID != 99 || out.DeliveryHandle != "h-abc" {
		t.Fatalf("wrong response: %+v", out)
	}
}

func TestGroupRatioOverrideUpsert_Success(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(envelopeOK(`null`)))
	}))
	defer srv.Close()
	c := newTestClient(srv)
	if err := c.GroupRatioOverrideUpsert(context.Background(), 42, "partner_1_tier_a", 1.5, "idem-g", "tr"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotPath != "/api/internal/group_ratio_override/upsert" {
		t.Fatalf("wrong path: %s", gotPath)
	}
}

// TestUpdateUserGroup_NotImplemented Fy-api 端尚未实现 → 直接返回非 retryable
// "not yet implemented" 错误（不打到网络）。
func TestUpdateUserGroup_NotImplemented(t *testing.T) {
	c := &Client{baseURL: "http://x", hmacSecret: []byte("k"), httpClient: http.DefaultClient}
	err := c.UpdateUserGroup(context.Background(), 42, "g", "idem-u")
	if err == nil || !strings.Contains(err.Error(), "not yet implemented") {
		t.Fatalf("expected not-yet-implemented, got %v", err)
	}
	if errors.Is(err, ErrRetryable) {
		t.Fatal("not-yet-implemented must not be retryable")
	}
}

func TestEraseUser_NotImplemented(t *testing.T) {
	c := &Client{baseURL: "http://x", hmacSecret: []byte("k"), httpClient: http.DefaultClient}
	err := c.EraseUser(context.Background(), 42, "idem-e")
	if err == nil || !strings.Contains(err.Error(), "not yet implemented") {
		t.Fatalf("expected not-yet-implemented, got %v", err)
	}
}

// fyApiBuildCanonical 复刻 Fy-api/middleware/internal_auth.go 的 BuildCanonical，
// 用于 parity 测试：partner-api 客户端签名与 Fy-api 服务端验签必须字节级一致。
func fyApiBuildCanonical(method, path, rawQuery, ts, nonce, bodyHashHex string) string {
	return strings.Join([]string{
		strings.ToUpper(method),
		path,
		fyApiCanonicalQuery(rawQuery),
		ts,
		nonce,
		bodyHashHex,
	}, "\n")
}

func fyApiCanonicalQuery(raw string) string {
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

// TestSign_FyApiParity 证明 Client.sign 与 Fy-api 服务端验签的 canonical
// 串字节级一致；secret 相同 → 签名相同；这是 partner-api ↔ Fy-api 联调的前提。
func TestSign_FyApiParity(t *testing.T) {
	secret := []byte("test-secret-32-bytes-min-len-CSPRNG-x")
	c := &Client{hmacSecret: secret}

	cases := []struct {
		name              string
		method, path, raw string
		body              []byte
	}{
		{"GET no body no query", "GET", "/api/internal/usage/by-user", "", nil},
		{"POST body", "POST", "/api/internal/user/topup", "", []byte(`{"user_id":42,"amount":1000}`)},
		{"GET sorted query", "GET", "/api/internal/usage/by-user", "user_id=42&from=1700000000&to=1700100000", nil},
		{"GET reversed query (must sort)", "GET", "/api/internal/usage/by-user", "to=1700100000&from=1700000000&user_id=42", nil},
		{"POST + query + body", "POST", "/api/internal/user/group_ratio_override", "trace=abc", []byte(`{"user_id":42,"override":1.5}`)},
		{"lowercase method must uppercase", "post", "/api/internal/user/topup", "", []byte(`{}`)},
	}

	const ts = "1700000000"
	const nonce = "11111111-2222-3333-4444-555555555555"

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotSig := c.sign(tc.method, tc.path, tc.raw, tc.body, ts, nonce)

			bodyHash := sha256.Sum256(tc.body)
			expectCanonical := fyApiBuildCanonical(tc.method, tc.path, tc.raw, ts, nonce, hex.EncodeToString(bodyHash[:]))
			mac := hmac.New(sha256.New, secret)
			mac.Write([]byte(expectCanonical))
			expectSig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

			if gotSig != expectSig {
				t.Fatalf("HMAC parity broken:\n  client got: %s\n  Fy-api exp: %s\n  canonical:\n%s",
					gotSig, expectSig, expectCanonical)
			}
		})
	}
}

// TestSign_OutputIsBase64 验证签名输出是 base64（与 Fy-api hmac.Equal 比较的形式一致）。
func TestSign_OutputIsBase64(t *testing.T) {
	c := &Client{hmacSecret: []byte("k")}
	sig := c.sign("GET", "/api/internal/x", "", nil, "1", "n")
	if _, err := base64.StdEncoding.DecodeString(sig); err != nil {
		t.Fatalf("signature must be base64-encoded, got %q: %v", sig, err)
	}
}
