// Package auth 提供密码哈希与校验能力。
//
// 当前实现采用 Argon2id（golang.org/x/crypto/argon2），抵抗 GPU/ASIC 暴力破解。
// 为兼容存量 SHA-256 哈希（旧版本迁移），VerifyPassword 同时支持两种格式：
//   - $argon2id$<base64(salt)>$<base64(hash)>           （新格式）
//   - $sha256$<base64(salt)>$<base64(hash)>             （旧格式，仅用于验证）
//
// 推荐配置（OWASP 建议）：time=3, memory=64MB, threads=4。
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

	"golang.org/x/crypto/argon2"
)

// saltSize 盐值字节长度
const saltSize = 32

// hashFormat 哈希字符串格式前缀
const hashArgon2Prefix = "$argon2id$"
const hashSHA256Prefix = "$sha256$"

// argon2Params Argon2id 参数配置（OWASP 推荐：抵抗侧信道 + 暴力破解）
const (
	argon2Time    = 3         // 迭代次数
	argon2Memory  = 64 * 1024 // 内存 64 MB
	argon2Threads = 4         // 并行度
	argon2KeyLen  = 32        // 输出密钥长度
)

// HashPassword 对明文密码进行 Argon2id 加盐哈希。
//
// 返回格式：$argon2id$<base64_salt>$<base64_hash>
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("生成盐值失败: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	return hashArgon2Prefix +
		base64.StdEncoding.EncodeToString(salt) + "$" +
		base64.StdEncoding.EncodeToString(hash), nil
}

// VerifyPassword 校验明文密码是否匹配已存储的哈希。
//
// 自动识别$argon2id$与$sha256$两种格式，分别使用对应算法验证。
// 使用恒定时间比较（subtle.ConstantTimeCompare）防御时序攻击。
func VerifyPassword(password, encodedHash string) (bool, error) {
	if strings.HasPrefix(encodedHash, hashArgon2Prefix) {
		return verifyArgon2id(password, encodedHash)
	}
	if strings.HasPrefix(encodedHash, hashSHA256Prefix) {
		return verifySHA256(password, encodedHash)
	}
	return false, fmt.Errorf("不支持的哈希格式")
}

// verifyArgon2id 验证 Argon2id 格式的哈希。
func verifyArgon2id(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 4 {
		return false, fmt.Errorf("argon2id 哈希格式损坏")
	}

	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false, fmt.Errorf("argon2id 盐值解码失败: %w", err)
	}

	expectedHash, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, fmt.Errorf("argon2id 哈希解码失败: %w", err)
	}

	actualHash := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, uint32(len(expectedHash)))

	if subtle.ConstantTimeCompare(actualHash, expectedHash) == 1 {
		return true, nil
	}
	return false, nil
}

// verifySHA256 验证旧版 SHA-256 格式哈希（仅用于平滑迁移，新哈希不再生成此格式）。
func verifySHA256(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 4 {
		return false, fmt.Errorf("sha256 哈希格式损坏")
	}

	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false, fmt.Errorf("sha256 盐值解码失败: %w", err)
	}

	expectedHash, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, fmt.Errorf("sha256 哈希解码失败: %w", err)
	}

	actualHash := hashSHA256WithSalt(password, salt)

	if subtle.ConstantTimeCompare(actualHash, expectedHash) == 1 {
		return true, nil
	}
	return false, nil
}

// IsHashedPassword 判断字符串是否已经是加盐哈希格式（支持识别新旧两种格式）。
//
// 用于区分明文密码与已哈希密码，支持平滑迁移。
func IsHashedPassword(s string) bool {
	return strings.HasPrefix(s, hashArgon2Prefix) || strings.HasPrefix(s, hashSHA256Prefix)
}

// IsArgon2Hash 判断字符串是否为 Argon2id 哈希格式。
func IsArgon2Hash(s string) bool {
	return strings.HasPrefix(s, hashArgon2Prefix)
}

// NeedsRehash 判断已存储的哈希是否需要升级为 Argon2id（当前为 SHA-256 格式）。
//
// 可在登录成功后调用，若返回 true，应生成新哈希并更新存储。
func NeedsRehash(encodedHash string) bool {
	return strings.HasPrefix(encodedHash, hashSHA256Prefix)
}

// hashSHA256WithSalt 计算 password + salt 的 SHA-256 哈希（兼容旧版）。
//
// 算法：HMAC-like 结构 = SHA256(salt || SHA256(password) || salt)，
// 既防御彩虹表也防御长度扩展攻击。
func hashSHA256WithSalt(password string, salt []byte) []byte {
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
