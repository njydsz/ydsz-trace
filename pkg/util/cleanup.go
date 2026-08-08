// Package util 提供跨微服务共享的工具函数集。
package util

import (
	"log"
	"os"
	"path/filepath"
	"time"
)

// CleanupOptions 临时文件清理选项。
type CleanupOptions struct {
	// MaxAge 超过此时间的文件/目录将被删除
	MaxAge time.Duration
	// Pattern 文件名匹配模式（glob），当前版本保留字段暂不使用
	Pattern string
}

// DefaultCleanupOptions 默认清理配置：删除超过 2 小时的过期临时文件。
var DefaultCleanupOptions = CleanupOptions{
	MaxAge: 2 * time.Hour,
}

// CleanupOldFiles 删除指定目录下超过 MaxAge 的文件和空目录。
//
// 安全策略：
//   - 仅删除常规文件和空目录
//   - 跳过符号链接、设备文件等特殊文件
//   - 空目录才会被删除（非空目录因 os.Remove 失败而自然保留）
//
// 返回删除文件数、删除目录数与首个错误。
func CleanupOldFiles(dir string, opts CleanupOptions) (deletedFiles, deletedDirs int, err error) {
	if dir == "" {
		return 0, 0, nil
	}

	// 目录不存在则跳过
	if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
		return 0, 0, nil
	}

	cutoff := time.Now().Add(-opts.MaxAge)

	err = filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			log.Printf("清理时访问 %s 失败: %v", path, walkErr)
			return nil // 跳过无法访问的文件
		}

		// 跳过根目录本身
		if path == dir {
			return nil
		}

		// 跳过非常规文件（符号链接、设备文件等）
		if !info.Mode().IsRegular() && !info.IsDir() {
			return nil
		}

		// 检查修改时间
		if info.ModTime().After(cutoff) {
			if info.IsDir() {
				return filepath.SkipDir // 跳过目录内的文件
			}
			return nil
		}

		// 删除过期文件或空目录
		if info.IsDir() {
			// 尝试删除空目录（非空会失败，安全）
			if rmdirErr := os.Remove(path); rmdirErr == nil {
				deletedDirs++
				log.Printf("已清理过期目录: %s", path)
				return filepath.SkipDir
			}
			return nil
		}

		// 删除过期文件
		if removeErr := os.Remove(path); removeErr == nil {
			deletedFiles++
		} else {
			log.Printf("删除文件失败 %s: %v", path, removeErr)
		}
		return nil
	})

	if err != nil {
		log.Printf("遍历目录 %s 失败: %v", dir, err)
	}

	return deletedFiles, deletedDirs, err
}

// SafeRemove 删除文件，忽略 "not exist" 错误，其他错误仅打日志。
func SafeRemove(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("删除文件 %s 失败: %v", path, err)
	}
}

// SafeRemoveAll 递归删除目录，忽略 "not exist" 错误。
func SafeRemoveAll(path string) {
	if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
		log.Printf("删除目录 %s 失败: %v", path, err)
	}
}
