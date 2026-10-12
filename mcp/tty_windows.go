//go:build windows
// +build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
)

// isWindowsTTY 通过 GetConsoleMode 判断 Windows 控制台是否为 TTY。
func isWindowsTTY(fd uintptr) bool {
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(fd, uintptr(unsafe.Pointer(&mode)))
	// GetConsoleMode 返回 0 表示失败（非控制台句柄，如管道）
	return r != 0
}

// isFDATTY 是 Windows 平台的 TTY 检测入口。
func isFDATTY(fd uintptr) bool {
	return isWindowsTTY(fd)
}
