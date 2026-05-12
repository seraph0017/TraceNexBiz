package fyapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
	"testing"
)

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
