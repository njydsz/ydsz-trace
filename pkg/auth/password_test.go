package auth

import (
	"strings"
	"testing"
)

func TestHashPassword_ReturnsValidFormat(t *testing.T) {
	hash, err := HashPassword("testPassword123")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if !strings.HasPrefix(hash, hashPrefix) {
		t.Errorf("expected prefix %q, got %q", hashPrefix, hash[:len(hashPrefix)])
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 4 {
		t.Errorf("expected 4 parts separated by $, got %d", len(parts))
	}
}

func TestHashPassword_IsDeterministicInFormatButNotValue(t *testing.T) {
	// 两次哈希同一密码应产生不同结果（因为盐是随机的）
	hash1, _ := HashPassword("samepassword")
	hash2, _ := HashPassword("samepassword")
	if hash1 == hash2 {
		t.Error("expected different hashes due to random salt")
	}
}

func TestVerifyPassword_CorrectPassword(t *testing.T) {
	password := "mySecureP@ss"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	ok, err := VerifyPassword(password, hash)
	if err != nil {
		t.Fatalf("VerifyPassword failed: %v", err)
	}
	if !ok {
		t.Error("expected correct password to verify")
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	hash, _ := HashPassword("original")
	ok, err := VerifyPassword("wrong", hash)
	if err != nil {
		t.Fatalf("VerifyPassword failed: %v", err)
	}
	if ok {
		t.Error("expected wrong password to fail verification")
	}
}

func TestVerifyPassword_InvalidHashFormat(t *testing.T) {
	_, err := VerifyPassword("test", "not-a-hash")
	if err == nil {
		t.Error("expected error for invalid hash format")
	}
}

func TestVerifyPassword_CorruptHash(t *testing.T) {
	_, err := VerifyPassword("test", "$sha256$invalid$hash")
	if err == nil {
		t.Error("expected error for corrupt hash")
	}
}

func TestIsHashedPassword(t *testing.T) {
	hash, _ := HashPassword("test")
	if !IsHashedPassword(hash) {
		t.Error("expected hashed password to be detected")
	}
	if IsHashedPassword("plainpassword") {
		t.Error("plain password should not be detected as hashed")
	}
}

func TestHashPassword_EmptyPassword(t *testing.T) {
	hash, err := HashPassword("")
	if err != nil {
		t.Fatalf("empty password should still hash: %v", err)
	}
	ok, _ := VerifyPassword("", hash)
	if !ok {
		t.Error("empty password should verify against its hash")
	}
}
