package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Claims struct {
	UserID    int64  `json:"user_id"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"exp"`
}

type TokenManager struct {
	secret []byte
	now    func() time.Time
}

type tokenHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

func NewTokenManager(secret string) TokenManager {
	return TokenManager{secret: []byte(secret), now: time.Now}
}

func (m TokenManager) Generate(claims Claims) (string, error) {
	header, err := json.Marshal(tokenHeader{Algorithm: "HS256", Type: "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + m.sign(body), nil
}

func (m TokenManager) Parse(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, fmt.Errorf("invalid token")
	}
	signedBody := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(m.sign(signedBody)), []byte(parts[2])) {
		return Claims{}, fmt.Errorf("invalid token signature")
	}
	rawHeader, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, err
	}
	var header tokenHeader
	if err := json.Unmarshal(rawHeader, &header); err != nil {
		return Claims{}, err
	}
	if header.Algorithm != "HS256" || header.Type != "JWT" {
		return Claims{}, fmt.Errorf("unsupported token header")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, err
	}
	var claims Claims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return Claims{}, err
	}
	if claims.ExpiresAt > 0 && claims.ExpiresAt < m.now().Unix() {
		return Claims{}, fmt.Errorf("token expired")
	}
	return claims, nil
}

func (m TokenManager) sign(body string) string {
	h := hmac.New(sha256.New, m.secret)
	_, _ = h.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
