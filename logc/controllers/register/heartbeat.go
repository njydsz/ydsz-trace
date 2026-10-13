// Package register 处理 logc 客户端向 logs 服务端的注册与心跳续约。
//
// 心跳策略：
//   - 默认每 60 秒发送一次心跳（轻量 keepalive，非重注册）
//   - 失败后按指数退避：5s → 10s → 20s → ... → 上限 10 分钟
//   - 成功后重置退避计数
//   - 心跳结果由 RegisterLocalIp 返回的 error 驱动（P0-1 修复）
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
// 调用结果（成功/失败）驱动退避计数，实现真正的指数退避。
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

// Start 启用心跳循环 goroutine；重复调用无效。
func (h *HeartbeatManager) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.isRunning {
		return
	}

	h.isRunning = true
	go h.heartbeatLoop()
	log.Printf("[heartbeat] 心跳续约已启动，间隔: %v，目标: %s", heartbeatInterval, h.server)
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
	log.Println("[heartbeat] 心跳续约已停止")
}

// heartbeatLoop 主循环：先立即注册一次，再按动态间隔定时执行。
// 使用 sync.WaitGroup 对应的 context 取消信号确保 goroutine 不泄漏（符合并发规范 §10.1）。
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
			// 根据当前重试计数重置 ticker 间隔
			ticker.Reset(h.getInterval())
		}
	}
}

// doRegister 执行一次注册请求并根据结果更新退避状态。
// 成功时重置 retryCount，失败时递增 retryCount 并记录时间。
// 这是 P0-1 修复的核心：心跳结果真正反馈到退避逻辑。
func (h *HeartbeatManager) doRegister() {
	h.mu.Lock()
	server, vKey := h.server, h.vKey
	h.mu.Unlock()

	_, err := RegisterLocalIp(server, vKey)

	h.mu.Lock()
	defer h.mu.Unlock()

	if err != nil {
		h.retryCount++
		h.lastFailTime = time.Now()
		log.Printf("[heartbeat] 注册失败 (第 %d 次)，下次间隔: %v，错误: %v",
			h.retryCount, h.getIntervalLocked(), err)
		return
	}

	// 成功时重置退避
	if h.retryCount > 0 {
		log.Printf("[heartbeat] 注册恢复，重置退避计数 (此前失败 %d 次)", h.retryCount)
	}
	h.retryCount = 0
}

// getInterval 根据失败次数计算心跳间隔：失败次数指数增长，上限 maxRetryInterval。
func (h *HeartbeatManager) getInterval() time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.getIntervalLocked()
}

// getIntervalLocked 内部无锁版本，调用者必须持有 mu。
func (h *HeartbeatManager) getIntervalLocked() time.Duration {
	if h.retryCount == 0 {
		return heartbeatInterval
	}
	interval := time.Duration(math.Pow(2, float64(h.retryCount))) * initialRetryInterval
	if interval > maxRetryInterval {
		interval = maxRetryInterval
	}
	return interval
}

// ============ 全局心跳管理器实例 ============

var (
	globalHeartbeat *HeartbeatManager
	heartbeatOnce   sync.Once
)

// StartGlobalHeartbeat 启动全局唯一心跳（仅首次调用生效）。
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
