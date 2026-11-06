//go:build windows

package source

import (
	"os"
)

// platformInode 返回 Windows 系统的伪 inode（基于 ModTime + Size）。
func platformInode(info os.FileInfo) uint64 {
	return uint64(info.ModTime().UnixNano()) ^ uint64(info.Size())
}
