// Package middleware - jwt_rsa_verifier.go：RS256 JWT 验签（无第三方 jwt 库依赖）。
//
// 解析 header.payload.signature，公钥来自 cfg.JWT.VerifyKeyPEM（PEM 编码 RSA pubkey）。
// 仅支持 RS256（per ADR-007 v0.2）；HS* / none 一律拒绝。
package middleware

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrTokenInvalid 解析或验签失败的通用错误（不向 client 暴露细节）。
var ErrTokenInvalid = errors.New("middleware: jwt invalid")

// RSAVerifier RS256 JWT verifier；线程安全，可单例。
type RSAVerifier struct {
	pub    *rsa.PublicKey
	clock  func() time.Time
	leeway time.Duration
}

// NewRSAVerifier 从 PEM 加载公钥。pem 可以是 PKCS1 RSA 公钥或 PKIX SubjectPublicKeyInfo。
func NewRSAVerifier(pemBytes []byte) (*RSAVerifier, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("middleware: empty or invalid PEM block")
	}
	var pub *rsa.PublicKey
	if k, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		rk, ok := k.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("middleware: PKIX key is not RSA")
		}
		pub = rk
	} else if k, err2 := x509.ParsePKCS1PublicKey(block.Bytes); err2 == nil {
		pub = k
	} else {
		return nil, fmt.Errorf("middleware: parse pub key: PKIX=%v PKCS1=%v", err, err2)
	}
	return &RSAVerifier{pub: pub, clock: time.Now, leeway: 30 * time.Second}, nil
}

// WithClock 测试钩子（不修改原 verifier）。
func (v *RSAVerifier) WithClock(clk func() time.Time) *RSAVerifier {
	cp := *v
	cp.clock = clk
	return &cp
}

// Verify 解析 + 验签 + exp/iat 校验。
func (v *RSAVerifier) Verify(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrTokenInvalid
	}
	headerB, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	var hdr struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerB, &hdr); err != nil {
		return nil, ErrTokenInvalid
	}
	if hdr.Alg != "RS256" {
		return nil, ErrTokenInvalid
	}

	payloadB, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	var cl Claims
	if err := json.Unmarshal(payloadB, &cl); err != nil {
		return nil, ErrTokenInvalid
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrTokenInvalid
	}

	signingInput := parts[0] + "." + parts[1]
	h := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(v.pub, crypto.SHA256, h[:], sig); err != nil {
		return nil, ErrTokenInvalid
	}

	now := v.clock()
	if cl.Exp > 0 && time.Unix(cl.Exp, 0).Add(v.leeway).Before(now) {
		return nil, ErrTokenInvalid
	}
	if cl.Iat > 0 && time.Unix(cl.Iat, 0).After(now.Add(v.leeway)) {
		return nil, ErrTokenInvalid
	}
	return &cl, nil
}

// nowUnix 提供给 JWT() 的默认 clock。
func nowUnix() int64 { return time.Now().Unix() }
