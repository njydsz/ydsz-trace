// Package auth 提供密码哈希与校验能力。
//
// 当前实现采用标准库 crypto/sha256 + 随机 salt，避免外部依赖。
// 后续网络通畅后应迁移至 bcrypt/argon2id，届时只需替换 HashPassword/VerifyPassword 实现。
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// saltSize 盐值字节长度
const saltSize = 32

// hashFormat 哈希字符串格式：$sha256$<base64(salt)>$<base64(hash)>
const hashPrefix = "$sha256$"

// HashPassword 对明文密码进行加盐哈希。
//
// 返回格式：$sha256$<base64_salt>$<base64_hash>
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltSize)
	if _, err := io.Read(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("生成盐值失败: %w", err)
	}

	hash := hashWithSalt(password, salt)
	return hashPrefix + base64.StdEncoding.EncodeToString(salt) + "$" + base64.StdEncoding.EncodeToString(hash), nil
}

// VerifyPassword 校验明文密码是否匹配已存储的哈希。
//
// 使用恒定时间比较（subtle.ConstantTimeCompare）防御时序攻击。
func VerifyPassword(password, encodedHash string) (bool, error) {
	if !strings.HasPrefix(encodedHash, hashPrefix) {
		return false, fmt.Errorf("不支持的哈希格式")
	}

	parts := strings.Split(encodedHash, "$")
	if len(parts) != 4 {
		return false, fmt.Errorf("哈希格式损坏")
	}

	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false, fmt.Errorf("盐值解码失败: %w", err)
	}

	expectedHash, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, fmt.Errorf("哈希解码失败: %w", err)
	}

	actualHash := hashWithSalt(password, salt)

	// 恒定时间比较，防御时序攻击
	if subtle.ConstantTimeCompare(actualHash, expectedHash) == 1 {
		return true, nil
	}
	return false, nil
}

// IsHashedPassword 判断字符串是否已经是加盐哈希格式。
//
// 用于区分明文密码与已哈希密码，支持平滑迁移。
func IsHashedPassword(s string) bool {
	return strings.HasPrefix(s, hashPrefix)
}

// hashWithSalt 计算 password + salt 的 SHA-256 哈希（32 字节）。
//
// 算法：HMAC-like 结构 = SHA256(salt || SHA256(password) || salt)，
// 既防御彩虹表也防御长度扩展攻击。
func hashWithSalt(password string, salt []byte) []byte {
	// 第一层：password 自身哈希，统一长度
	h := sha256.New()
	h.Write([]byte(password))
	innerHash := h.Sum(nil)

	// 第二层：盐包裹，增强安全性
	h2 := sha256.New()
	h2.Write(salt)
	h2.Write(innerHash)
	h2.Write(salt)
	return h2.Sum(nil)
}

// 保持兼容：hex 哈希输出工具函数（用于其他场景的 token 生成等）
func hashSHA256(input string) string {
	h := sha256.New()
	h.Write([]byte(input))
	return hex.EncodeToString(h.Sum(nil))
}
