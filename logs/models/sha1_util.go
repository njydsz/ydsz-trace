package models

import (
	"crypto/sha1"
	"encoding/hex"
)

// sha1Sum 计算字符串的 SHA1 十六进制摘要（小写）。
//
// 用于生成稳定的幂等 ID（变更事件、案例库等），
// 不用于安全场景（仅作去重键）。
func sha1Sum(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}
