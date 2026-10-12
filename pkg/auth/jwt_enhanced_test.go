package auth

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

// TestGenerateToken_DefaultAudience 验证未指定 audience 时使用默认值。
func TestGenerateToken_DefaultAudience(t *testing.T) {
	j := NewJWT("secret", time.Hour)
	token, err := j.GenerateToken("alice", "admin", "")
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}
	claims, err := j.ValidateToken(token)
	if err != nil {
		t.Fatalf("验证 token 失败: %v", err)
	}
	if claims.Audience != defaultAudience {
		t.Fatalf("默认 audience 应为 %q, got %q", defaultAudience, claims.Audience)
	}
	if claims.NotBefore <= 0 {
		t.Fatalf("NotBefore 应已设置, got %d", claims.NotBefore)
	}
}

// TestValidateToken_WrongAudience 验证 aud 不匹配时返回 ErrTokenInvalid。
func TestValidateToken_WrongAudience(t *testing.T) {
	j := NewJWT("secret", time.Hour)
	token, err := j.GenerateToken("alice", "admin", "wrong-audience")
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}
	_, err = j.ValidateToken(token)
	if err == nil {
		t.Fatalf("aud 不匹配应返回 ErrTokenInvalid")
	}
	if err != ErrTokenInvalid {
		t.Fatalf("期望 ErrTokenInvalid, got %v", err)
	}
}

// TestValidateToken_CorrectAudience 验证 aud 匹配时通过校验。
func TestValidateToken_CorrectAudience(t *testing.T) {
	j := NewJWT("secret", time.Hour)
	token, err := j.GenerateToken("alice", "admin", defaultAudience)
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}
	claims, err := j.ValidateToken(token)
	if err != nil {
		t.Fatalf("aud 匹配时应验证通过: %v", err)
	}
	if claims.Audience != defaultAudience {
		t.Fatalf("audience 应为 %q, got %q", defaultAudience, claims.Audience)
	}
}

// TestValidateToken_EmptyAudienceBackwardCompat 验证旧 token（aud 为空）跳过 aud 校验。
func TestValidateToken_EmptyAudienceBackwardCompat(t *testing.T) {
	j := NewJWT("secret", time.Hour)
	// 手动构造一个 aud 为空字段的 token
	now := time.Now()
	claims := Claims{
		Username:  "legacy",
		Role:      "viewer",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Hour).Unix(),
		Audience:  "", // 旧 token 无 aud
		NotBefore: now.Unix(),
	}
	token := buildToken(j, claims)

	result, err := j.ValidateToken(token)
	if err != nil {
		t.Fatalf("旧 token (aud 为空) 应跳过 aud 校验: %v", err)
	}
	if result.Username != "legacy" {
		t.Fatalf("用户名不匹配: got %q", result.Username)
	}
}

// TestValidateToken_NotBeforeFuture 验证 nbf 为将来时间时返回 "token 尚未生效"。
func TestValidateToken_NotBeforeFuture(t *testing.T) {
	j := NewJWT("secret", time.Hour)
	now := time.Now()
	claims := Claims{
		Username:  "alice",
		Role:      "admin",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Hour).Unix(),
		Audience:  defaultAudience,
		NotBefore: now.Add(1 * time.Hour).Unix(), // nbf 在未来
	}
	token := buildToken(j, claims)
	_, err := j.ValidateToken(token)
	if err == nil {
		t.Fatalf("nbf 在未来应返回错误")
	}
	if err.Error() != "token 尚未生效" {
		t.Fatalf("期望 'token 尚未生效', got %q", err.Error())
	}
}

// TestValidateToken_NotBeforeZero 验证 nbf 为 0 时跳过校验。
func TestValidateToken_NotBeforeZero(t *testing.T) {
	j := NewJWT("secret", time.Hour)
	now := time.Now()
	claims := Claims{
		Username:  "alice",
		Role:      "admin",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Hour).Unix(),
		Audience:  defaultAudience,
		NotBefore: 0, // 0 表示未设置，跳过
	}
	token := buildToken(j, claims)
	_, err := j.ValidateToken(token)
	if err != nil {
		t.Fatalf("nbf 为 0 时应跳过校验: %v", err)
	}
}

// TestValidateToken_NotBeforePast 验证 nbf 为过去时间时 token 有效。
func TestValidateToken_NotBeforePast(t *testing.T) {
	j := NewJWT("secret", time.Hour)
	now := time.Now()
	claims := Claims{
		Username:  "alice",
		Role:      "admin",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Hour).Unix(),
		Audience:  defaultAudience,
		NotBefore: now.Add(-1 * time.Minute).Unix(), // nbf 在 1 分钟前
	}
	token := buildToken(j, claims)
	_, err := j.ValidateToken(token)
	if err != nil {
		t.Fatalf("nbf 已过时 token 应有效: %v", err)
	}
}

// TestRefreshToken_PreservesAudience 验证续期后 audience 保持一致。
func TestRefreshToken_PreservesAudience(t *testing.T) {
	j := NewJWT("secret", time.Hour)
	token, _ := j.GenerateToken("bob", "user", defaultAudience)
	newToken, err := j.RefreshToken(token)
	if err != nil {
		t.Fatalf("刷新 token 失败: %v", err)
	}
	claims, err := j.ValidateToken(newToken)
	if err != nil {
		t.Fatalf("刷新后 token 验证失败: %v", err)
	}
	if claims.Audience != defaultAudience {
		t.Fatalf("续期后 audience 应保持 %q, got %q", defaultAudience, claims.Audience)
	}
	if claims.Username != "bob" {
		t.Fatalf("续期后用户名应保持 bob, got %q", claims.Username)
	}
}

// TestRefreshToken_WrongAudienceExpiredToken 验证过期且 aud 不匹配的 token 续期失败。
func TestRefreshToken_WrongAudienceExpiredToken(t *testing.T) {
	// 使用极短有效期签发，使 token 迅速过期
	j := NewJWT("short-secret", 10*time.Millisecond)
	token, err := j.GenerateToken("alice", "admin", "stale-audience")
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}
	// 等待 token 过期
	time.Sleep(50 * time.Millisecond)
	_, err = j.RefreshToken(token)
	if err == nil {
		t.Fatalf("aud 不匹配的过期 token 续期应失败")
	}
	if err != ErrTokenInvalid {
		t.Fatalf("期望 ErrTokenInvalid, got %v", err)
	}
}

// buildToken 辅助函数：使用指定 claims 构建合法签名的 JWT token。
func buildToken(j *JWT, claims Claims) string {
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	claimsJSON, _ := json.Marshal(claims)
	encH := base64.RawURLEncoding.EncodeToString(header)
	encP := base64.RawURLEncoding.EncodeToString(claimsJSON)
	sig := j.sign(encH + "." + encP)
	return encH + "." + encP + "." + sig
}
