package perf

import (
	"os/exec"
	"strconv"
	"strings"
)

// runPS 执行 ps 获取当前进程 RSS（KB）。
//
// 统一由 Unix/Windows 两个文件复用，避免重复实现。
func runPS() (string, error) {
	// -o rss= 只输出 RSS 列，便于解析
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid())).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
