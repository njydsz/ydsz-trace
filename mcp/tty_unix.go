//go:build !windows
// +build !windows

package main

import (
	"golang.org/x/sys/unix"
)

// isUnixTTY 通过 isatty 判断 Unix-like 系统 fd 是否为 TTY。
func isUnixTTY(fd uintptr) bool {
	_, err := unix.IoctlGetTermios(int(fd), unix.TCGETS)
	return err == nil
}

// isFDATTY 是 Unix 平台的 TTY 检测入口。
func isFDATTY(fd uintptr) bool {
	return isUnixTTY(fd)
}
