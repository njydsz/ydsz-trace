//go:build !windows

package perf

import (
	"os"
	"strconv"
	"strings"
)

// currentMemMB 返回当前进程 RSS 内存占用（MB）。
//
// 读取 /proc/self/statm（Linux）或通过 sysctl（macOS）实现。
// 读取失败时返回 0，调用方据此跳过资源检查（不误降级）。
func currentMemMB() float64 {
	if vm := readStatm(); vm > 0 {
		return vm
	}
	return readSysctlRSS()
}

// readStatm 从 /proc/self/statm 读取常驻内存（Linux）。
//
// statm 第二列为 resident pages，乘以页面大小得到字节数。
func readStatm() float64 {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	pageSize := float64(os.Getpagesize())
	return float64(pages) * pageSize / 1024 / 1024
}

// readSysctlRSS macOS 兜底：通过 ps 获取 RSS。
func readSysctlRSS() float64 {
	out, err := runPS()
	if err != nil {
		return 0
	}
	// ps 输出最后一列为 RSS（KB）
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return 0
	}
	kb, err := strconv.ParseFloat(fields[len(fields)-1], 64)
	if err != nil {
		return 0
	}
	return kb / 1024
}
