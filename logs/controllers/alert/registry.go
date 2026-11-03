// Package alert 子文件：调度器注册表（V2.2）。
//
// 背景：
//   配置类接口（静默窗口保存）需要刷新运行中调度器的内存状态。
//   若把 Scheduler 做成全局单例，会造成包级隐藏依赖、测试难以替换。
//   此处采用「显式注册」模式：main.go 在创建 Scheduler 后调用 RegisterScheduler，
//   配置接口通过 GetRunningScheduler 获取。
//
// 并发安全：注册与读取均持锁；同一时刻仅允许一个调度器（重复注册覆盖并记录日志）。
package alert

import (
	"log"
	"sync"
)

var (
	schedulerMu    sync.RWMutex
	runningScheduler *Scheduler
)

// RegisterScheduler 注册运行中的调度器实例。
//
// 通常在 main.go 创建 Scheduler 后立即调用；传 nil 表示清空注册。
func RegisterScheduler(s *Scheduler) {
	schedulerMu.Lock()
	defer schedulerMu.Unlock()
	runningScheduler = s
	if s == nil {
		log.Printf("[alert] 调度器注册已清空")
	}
}

// GetRunningScheduler 返回当前注册的调度器实例（可能为 nil）。
func GetRunningScheduler() *Scheduler {
	schedulerMu.RLock()
	defer schedulerMu.RUnlock()
	return runningScheduler
}
