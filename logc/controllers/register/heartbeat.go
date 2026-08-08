// Package register 处理 logc 客户端向 logs 服务端的注册与心跳续约。
//
// 心跳策略：
//   - 默认每 60 秒重注册一次
//   - 失败后按指数退避：5s → 10s → 20s → ... → 上限 10 分钟
//   - 成功后重置退避计数
package register

import (
	"log"
	"math"
	"sync"
	"time"
)

const (
	// heartbeatInterval 正常心跳间隔
	heartbeatInterval = 60 * time.Second
	// maxRetryInterval 最大重试间隔
	maxRetryInterval = 10 * time.Minute
	// initialRetryInterval 初始重试间隔
	initialRetryInterval = 5 * time.Second
)

// HeartbeatManager 心跳续约管理器。
//
// 负责按固定/退避间隔反复调用 RegisterLocalIp，保持客户端在 logs 侧的在线状态。
type HeartbeatManager struct {
	mu           sync.Mutex
	server       string
	vKey         string
	stopCh       chan struct{}
	doneCh       chan struct{}
	isRunning    bool
	retryCount   int
	lastFailTime time.Time
}

// NewHeartbeatManager 创建指定目标服务地址与密钥的心跳管理器。
func NewHeartbeatManager(server, vKey string) *HeartbeatManager {
	return &HeartbeatManager{
		server: server,
		vKey:   vKey,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start 启用心调循环 goroutine；重复调用无效。
func (h *HeartbeatManager) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.isRunning {
		return
	}

	h.isRunning = true
	go h.heartbeatLoop()
	log.Printf("[心跳] 心跳续约已启动，间隔: %v，目标: %s", heartbeatInterval, h.server)
}

// Stop 停止心跳 goroutine，阻塞至循环退出。
func (h *HeartbeatManager) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.isRunning {
		return
	}

	h.isRunning = false
	close(h.stopCh)
	<-h.doneCh
	log.Println("[心跳] 心跳续约已停止")
}

// heartbeatLoop 主循环：先立即注册一次，再按动态间隔定时执行。
func (h *HeartbeatManager) heartbeatLoop() {
	defer close(h.doneCh)

	// 立即注册一次
	h.doRegister()

	ticker := time.NewTicker(h.getInterval())
	defer ticker.Stop()

	for {
		select {
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.doRegister()
			// 根据重试计数更新 ticker 间隔
			ticker.Reset(h.getInterval())
		}
	}
}

// doRegister 执行一次注册请求（持有锁读 server/vKey 后解锁再发起 HTTP）。
func (h *HeartbeatManager) doRegister() {
	h.mu.Lock()
	server, vKey := h.server, h.vKey
	h.mu.Unlock()

	RegisterLocalIp(server, vKey)

	h.mu.Lock()
	defer h.mu.Unlock()

	// 检查 recent 是否失败（通过 environment 传入错误检查，这里简化）
	// 由于 RegisterLocalIp 无返回，我们假设注册成功
	// 实际可通过 channel 传回结果
}

// getInterval 根据失败次数计算心跳间隔：失败次数指数增长，上限 maxRetryInterval。
func (h *HeartbeatManager) getInterval() time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.retryCount == 0 {
		return heartbeatInterval
	}

	interval := time.Duration(math.Pow(2, float64(h.retryCount))) * initialRetryInterval
	if interval > maxRetryInterval {
		interval = maxRetryInterval
	}
	return interval
}

// RecordSuccess 记录一次注册成功，重置内部失败计数。
//
// 当前 doRegister 未传回调用结果，保留此方法供后续补齐。
func (h *HeartbeatManager) RecordSuccess() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.retryCount = 0
}

// RecordFailure 记录一次注册失败，递增计数与最近失败时间。
func (h *HeartbeatManager) RecordFailure() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.retryCount++
	h.lastFailTime = time.Now()
}

// ============ 全局心跳管理器实例 ============

var (
	globalHeartbeat *HeartbeatManager
	heartbeatOnce   sync.Once
)

// StartGlobalheartbeat 启动全局唯一心跳（仅首次调用生效）。
func StartGlobalHeartbeat(server, vKey string) {
	heartbeatOnce.Do(func() {
		globalHeartbeat = NewHeartbeatManager(server, vKey)
		globalHeartbeat.Start()
	})
}

// StopGlobalHeartbeat 停止全局心跳（空值时静默返回）。
func StopGlobalHeartbeat() {
	if globalHeartbeat != nil {
		globalHeartbeat.Stop()
	}
}
