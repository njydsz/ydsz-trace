package perf

import "os"

// pid 返回当前进程 ID（供 ps 命令使用）。
func pid() int {
	return os.Getpid()
}
