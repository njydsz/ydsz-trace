// Package auth 提供基于 HMAC-SHA256 的轻量级 JWT 认证。
//
// 设计目标：
//   - 零外部依赖，仅使用 Go 标准库
//   - 兼容 beego session 的使用习惯
//   - 支持 token 签发、验证、刷新（含过期续期）
//
// token 结构：header.payload.signature（标准 JWT 三段式，Base64URL 编码）。
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
	// ErrTokenExpired 表示 token 已过期（不含宽限期）
	ErrTokenExpired = errors.New("token 已过期")
	// ErrTokenInvalid 表示 token 签名或载荷无效
	ErrTokenInvalid = errors.New("token 无效")
	// ErrTokenFormat 表示 token 格式不符合三段式 JWT
	ErrTokenFormat = errors.New("token 格式错误")
)

// Claims JWT 载荷声明。
//
// 字段精简为业务所需：用户名与角色。如需扩展自定义字段，
// 建议新建结构体嵌入 Claims，避免破坏现有签名兼容性。
type Claims struct {
	// Username 用户名，唯一业务标识
	Username string `json:"username"`
	// Role 用户角色（如 admin、viewer）
	Role string `json:"role"`
	// IssuedAt 签发时间（Unix 秒）
	IssuedAt int64 `json:"iat"`
	// ExpiresAt 过期时间（Unix 秒）
	ExpiresAt int64 `json:"exp"`
}

// JWT JWT 认证器实例。
//
// 线程安全：所有方法只读 secret/expiry，可在多 goroutine 间共享。
type JWT struct {
	secret []byte
	expiry time.Duration
	issuer string
}

// NewJWT 创建 JWT 实例。
//
// 参数：
//   - secret: 签名密钥，为空则使用内置默认（生产环境务必自定义）
//   - expiry: token 有效期，为零默认 24 小时
//
// 安全提示：生产环境请通过环境变量 YDSZ_JWT_SECRET 注入强密钥。
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

// GenerateToken 签发新的 JWT token。
//
// 参数：
//   - username: 用户名
//   - role: 用户角色
//
// 返回完整 JWT 字符串（header.payload.signature）。
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

// ValidateToken 验证 JWT token 的签名、格式、有效期。
//
// 可能返回的错误：ErrTokenFormat / ErrTokenInvalid / ErrTokenExpired
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

// sign 使用 HMAC-SHA256 对数据进行签名，返回 Base64URL 编码的签名串。
func (j *JWT) sign(data string) string {
	h := hmac.New(sha256.New, j.secret)
	h.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// RefreshToken 在允许宽限期内续签 token。
//
// 仍有效的 token 可直接续期；过期 7 天内的 token 也可续期，
// 超出宽限期返回 ErrTokenExpired。
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

// InitDefaultJWT 初始化全局默认 JWT 实例。
//
// 应在应用启动时调用一次，后续通过 Default() 获取共享实例。
func InitDefaultJWT(secret string, expiry time.Duration) {
	defaultJWT = NewJWT(secret, expiry)
}

// Default 获取全局默认 JWT 实例。
//
// 若未初始化则自动创建一个生产不安全的默认实例（会打 WARNING 日志）。
func Default() *JWT {
	if defaultJWT == nil {
		defaultJWT = NewJWT("", 0)
	}
	return defaultJWT
}
