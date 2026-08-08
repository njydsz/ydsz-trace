package util

import (
	"os"
)

// GetEnv 获取环境变量，不存在时返回 fallback。
//
// 优先于 os.Getenv 的点：语义更明确（不存在 vs 空字符串统一返回 fallback）。
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
