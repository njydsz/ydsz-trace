//go:build windows

package source

import "os"

// inodeOf 在 Windows 平台返回 0（无 inode 概念）。
//
// Windows 下 logrotate 少见，ResolveResumeOffset 会跳过 inode 校验，
// 退化为按 path + size 判断，仍可覆盖文件截断场景。
func inodeOf(_ os.FileInfo) uint64 {
	return 0
}
