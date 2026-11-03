//go:build !windows

package source

import (
	"os"
	"syscall"
)

// inodeOf 从 FileInfo 中提取 Unix 平台的 inode。
//
// 通过类型断言 *syscall.Stat_t 获取；非 Unix 平台由 inode_windows.go 提供零实现。
func inodeOf(info os.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Ino
	}
	return 0
}
