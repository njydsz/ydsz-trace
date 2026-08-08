package auth

import (
	"testing"
	"time"
)

// TestGenerateAndValidateToken 验证正常签发 - 验证往返正确性。
func TestGenerateAndValidateToken(t *testing.T) {
	j := NewJWT("unit-secret", time.Hour)
	token, err := j.GenerateToken("alice", "admin")
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}
	if token == "" {
		t.Fatalf("token 不应为空")
	}

	claims, err := j.ValidateToken(token)
	if err != nil {
		t.Fatalf("验证 token 失败: %v", err)
	}
	if claims.Username != "alice" {
		t.Fatalf("Username 不匹配: got %q", claims.Username)
	}
	if claims.Role != "admin" {
		t.Fatalf("Role 不匹配: got %q", claims.Role)
	}
	if claims.ExpiresAt <= claims.IssuedAt {
		t.Fatalf("过期时间应晚于签发时间")
	}
}

// TestValidateToken_WrongSecret 验证不同密钥签发 token 须校验失败。
func TestValidateToken_WrongSecret(t *testing.T) {
	a := NewJWT("secret-a", time.Hour)
	token, err := a.GenerateToken("u", "r")
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}
	b := NewJWT("secret-b", time.Hour)
	if _, err := b.ValidateToken(token); err == nil {
		t.Fatalf("错误密钥应验证失败")
	}
}

// TestValidateToken_Tampered 验证篡改 token 后校验失败。
func TestValidateToken_Tampered(t *testing.T) {
	j := NewJWT("secret", time.Hour)
	token, _ := j.GenerateToken("u", "r")
	if _, err := j.ValidateToken(token + "x"); err == nil {
		t.Fatalf("篡改 token 应验证失败")
	}
}

// TestValidateToken_Malformed 验证非法格式 token 返回 ErrTokenFormat。
func TestValidateToken_Malformed(t *testing.T) {
	j := NewJWT("secret", time.Hour)
	if _, err := j.ValidateToken("not.a.jwt"); err == nil {
		t.Fatalf("格式错误的 token 应验证失败")
	}
}

// TestValidateToken_Expired 验证已过期的 token 返回 ErrTokenExpired。
func TestValidateToken_Expired(t *testing.T) {
	j := NewJWT("secret", -time.Hour)
	token, err := j.GenerateToken("u", "r")
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}
	if _, err := j.ValidateToken(token); err == nil {
		t.Fatalf("过期 token 应验证失败")
	}
}

// TestRefreshToken 验证 RefreshToken 保持用户信息一致。
func TestRefreshToken(t *testing.T) {
	j := NewJWT("secret", time.Hour)
	token, _ := j.GenerateToken("bob", "user")
	newToken, err := j.RefreshToken(token)
	if err != nil {
		t.Fatalf("刷新 token 失败: %v", err)
	}
	claims, err := j.ValidateToken(newToken)
	if err != nil {
		t.Fatalf("刷新后 token 验证失败: %v", err)
	}
	if claims.Username != "bob" {
		t.Fatalf("刷新后用户名应保持: got %q", claims.Username)
	}
}

// TestInitDefaultJWT 验证全局默认 JWT 初始化后的可用性。
func TestInitDefaultJWT(t *testing.T) {
	InitDefaultJWT("secret-default", time.Hour)
	j := Default()
	if j == nil {
		t.Fatalf("Default 不应返回 nil")
	}
	token, err := j.GenerateToken("x", "y")
	if err != nil {
		t.Fatalf("默认 JWT 生成失败: %v", err)
	}
	if _, err := j.ValidateToken(token); err != nil {
		t.Fatalf("默认 JWT 验证失败: %v", err)
	}
}
