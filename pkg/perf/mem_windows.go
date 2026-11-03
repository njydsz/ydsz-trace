//go:build windows

package perf

import (
	"os"
	"syscall"
	"unsafe"
)

// currentMemMB Windows 平台通过 GetProcessMemoryInfo 读取工作集大小。
func currentMemMB() float64 {
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(os.Getpid()))
	if err != nil {
		return 0
	}
	defer func() {
		_ = syscall.CloseHandle(h)
	}()

	// 加载 psapi.dll
	psapi := syscall.NewLazyDLL("psapi.dll")
	proc := psapi.NewProc("GetProcessMemoryInfo")

	var mem counters
	mem.cb = uint32(unsafe.Sizeof(mem))

	_, _, callErr := proc.Call(uintptr(h), uintptr(unsafe.Pointer(&mem)), uintptr(mem.cb))
	if callErr != nil && callErr.Error() != "The operation completed successfully." {
		return 0
	}
	// WorkingSetSize 即当前常驻内存（字节）
	return float64(mem.WorkingSetSize) / 1024 / 1024
}

// counters PROCESS_MEMORY_COUNTERS 结构（Windows API）。
type counters struct {
	cb                      uint32
	PageFaultCount          uint32
	PeakWorkingSetSize      uintptr
	WorkingSetSize          uintptr
	QuotaPeakPagedPoolUsage uintptr
	QuotaPagedPoolUsage     uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage  uintptr
	PagefileUsage           uintptr
	PeakPagefileUsage       uintptr
}
