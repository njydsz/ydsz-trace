// Package auth 提供 JWT 认证（使用标准库实现，无外部依赖）
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

var (
	ErrTokenExpired = errors.New("token 已过期")
	ErrTokenInvalid = errors.New("token 无效")
	ErrTokenFormat  = errors.New("token 格式错误")
)

// Claims JWT 声明
type Claims struct {
	Username  string `json:"username"`
	Role      string `json:"role"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// JWT JWT 认证器
type JWT struct {
	secret []byte
	expiry time.Duration
	issuer string
}

// NewJWT 创建 JWT 实例
func NewJWT(secret string, expiry time.Duration) *JWT {
	if secret == "" {
		secret = "ydsz-trace-jwt-secret-change-in-production"
		log.Printf("[WARN] JWT 使用默认密钥，生产环境请通过环境变量 YDSZ_JWT_SECRET 设置")
	}
	if expiry == 0 {
		expiry = 24 * time.Hour
	}
	return &JWT{
		secret: []byte(secret),
		expiry: expiry,
		issuer: "ydsz-trace",
	}
}

// GenerateToken 生成 JWT token
func (j *JWT) GenerateToken(username, role string) (string, error) {
	now := time.Now()
	claims := Claims{
		Username:  username,
		Role:      role,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(j.expiry).Unix(),
	}

	// Header
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)

	// Payload
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(claimsJSON)

	// Signature
	signature := j.sign(encodedHeader + "." + encodedPayload)

	return encodedHeader + "." + encodedPayload + "." + signature, nil
}

// ValidateToken 验证 JWT token
func (j *JWT) ValidateToken(tokenString string) (*Claims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, ErrTokenFormat
	}

	// 验证签名
	expectedSig := j.sign(parts[0] + "." + parts[1])
	if !hmac.Equal([]byte(expectedSig), []byte(parts[2])) {
		return nil, ErrTokenInvalid
	}

	// 解码 Payload
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTokenInvalid, err)
	}

	var claims Claims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTokenInvalid, err)
	}

	// 检查过期
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, ErrTokenExpired
	}

	return &claims, nil
}

// sign 使用 HMAC-SHA256 签名
func (j *JWT) sign(data string) string {
	h := hmac.New(sha256.New, j.secret)
	h.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// RefreshToken 刷新 token（在快过期时续期）
func (j *JWT) RefreshToken(tokenString string) (string, error) {
	claims, err := j.ValidateToken(tokenString)
	if err != nil && !errors.Is(err, ErrTokenExpired) {
		return "", err
	}

	// 允许过期 7 天内的 token 续期
	if time.Now().Unix()-claims.ExpiresAt > 7*24*60*60 {
		return "", ErrTokenExpired
	}

	return j.GenerateToken(claims.Username, claims.Role)
}

// ============ 全局默认 JWT 实例 ============

var defaultJWT *JWT

// InitDefaultJWT 初始化全局默认 JWT
func InitDefaultJWT(secret string, expiry time.Duration) {
	defaultJWT = NewJWT(secret, expiry)
}

// Default 获取全局默认 JWT
func Default() *JWT {
	if defaultJWT == nil {
		defaultJWT = NewJWT("", 0)
	}
	return defaultJWT
}
