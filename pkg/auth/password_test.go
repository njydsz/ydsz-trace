package auth

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestHashPassword_ReturnsArgon2Format(t *testing.T) {
	hash, err := HashPassword("testPassword123")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if !strings.HasPrefix(hash, hashArgon2Prefix) {
		t.Errorf("expected prefix %q, got %q", hashArgon2Prefix, hash[:len(hashArgon2Prefix)])
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

func TestVerifyPassword_CorruptArgon2Hash(t *testing.T) {
	_, err := VerifyPassword("test", "$argon2id$invalid$hash")
	if err == nil {
		t.Error("expected error for corrupt argon2id hash")
	}
}

func TestVerifyPassword_BackwardCompatSHA256(t *testing.T) {
	// 模拟旧版 SHA-256 哈希格式：构造一个确认兼容的旧格式哈希
	salt := make([]byte, 32)
	for i := range salt {
		salt[i] = byte(i)
	}
	password := "legacyPassword"
	hash := hashSHA256WithSalt(password, salt)
	encoded := hashSHA256Prefix + base64.StdEncoding.EncodeToString(salt) + "$" + base64.StdEncoding.EncodeToString(hash)

	ok, err := VerifyPassword(password, encoded)
	if err != nil {
		t.Fatalf("VerifyPassword backward compat failed: %v", err)
	}
	if !ok {
		t.Error("expected SHA256-format legacy password to verify")
	}
}

func TestVerifyPassword_SHA256WrongPassword(t *testing.T) {
	salt := make([]byte, 32)
	for i := range salt {
		salt[i] = byte(i)
	}
	hash := hashSHA256WithSalt("correct", salt)
	encoded := hashSHA256Prefix + base64.StdEncoding.EncodeToString(salt) + "$" + base64.StdEncoding.EncodeToString(hash)

	ok, _ := VerifyPassword("wrong", encoded)
	if ok {
		t.Error("expected wrong password to fail SHA256 verification")
	}
}

func TestIsHashedPassword(t *testing.T) {
	hash, _ := HashPassword("test")
	if !IsHashedPassword(hash) {
		t.Error("expected argon2id hashed password to be detected")
	}
	if IsHashedPassword("plainpassword") {
		t.Error("plain password should not be detected as hashed")
	}
}

func TestIsHashedPassword_DetectsSHA256(t *testing.T) {
	if !IsHashedPassword("$sha256$abc$def") {
		t.Error("expected SHA256 format to be detected as hashed")
	}
}

func TestNeedsRehash(t *testing.T) {
	argonHash, _ := HashPassword("test")
	if NeedsRehash(argonHash) {
		t.Error("fresh argon2id hash should not need rehash")
	}
	if !NeedsRehash("$sha256$abc$def") {
		t.Error("SHA256 hash should need rehash to argon2id")
	}
}

func TestIsArgon2Hash(t *testing.T) {
	argonHash, _ := HashPassword("test")
	if !IsArgon2Hash(argonHash) {
		t.Error("argon2id hash should be detected")
	}
	if IsArgon2Hash("$sha256$abc$def") {
		t.Error("SHA256 hash should not be detected as argon2id")
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
