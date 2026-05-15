package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// RSASigner signs and verifies compact RS256 JWTs.
type RSASigner struct {
	priv  *rsa.PrivateKey
	pub   *rsa.PublicKey
	keyID string
}

// NewRSASigner loads an RSA private key from PKCS#1 or PKCS#8 PEM.
func NewRSASigner(pemBytes []byte, keyID string) (*RSASigner, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("auth: empty or invalid RSA private key PEM")
	}
	var priv *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		priv = k
	} else if k, err2 := x509.ParsePKCS8PrivateKey(block.Bytes); err2 == nil {
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("auth: PKCS8 key is not RSA")
		}
		priv = rk
	} else {
		return nil, fmt.Errorf("auth: parse private key: PKCS1=%v PKCS8=%v", err, err2)
	}
	return &RSASigner{priv: priv, pub: &priv.PublicKey, keyID: keyID}, nil
}

// Sign creates a compact JWT with alg=RS256.
func (s *RSASigner) Sign(c Claims) (string, error) {
	hdr := struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
		Kid string `json:"kid,omitempty"`
	}{Alg: "RS256", Typ: "JWT", Kid: s.keyID}
	header, err := json.Marshal(hdr)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.priv, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Verify verifies an RS256 compact JWT and returns claims. Expiry is checked by callers.
func (s *RSASigner) Verify(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, errors.New("auth: malformed token")
	}
	headerB, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, err
	}
	var hdr struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerB, &hdr); err != nil {
		return Claims{}, err
	}
	if hdr.Alg != "RS256" {
		return Claims{}, errors.New("auth: unsupported jwt alg")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, err
	}
	signingInput := parts[0] + "." + parts[1]
	sum := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(s.pub, crypto.SHA256, sum[:], sig); err != nil {
		return Claims{}, err
	}
	payloadB, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, err
	}
	var c Claims
	if err := json.Unmarshal(payloadB, &c); err != nil {
		return Claims{}, err
	}
	return c, nil
}
