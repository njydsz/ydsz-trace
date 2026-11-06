//go:build !windows

package source

import (
	"os"
	"syscall"
)

// platformInode 返回 Unix 系统的真实 inode。
func platformInode(info os.FileInfo) uint64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Ino
	}
	return uint64(info.ModTime().UnixNano()) ^ uint64(info.Size())
}
