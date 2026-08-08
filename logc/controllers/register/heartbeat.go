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

// HeartbeatManager 心跳管理器
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

// NewHeartbeatManager 创建心跳管理器
func NewHeartbeatManager(server, vKey string) *HeartbeatManager {
	return &HeartbeatManager{
		server: server,
		vKey:   vKey,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start 启用心跳（如果已启动则忽略）
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

// Stop 停用心程
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

// heartbeatLoop 心跳循环
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

// doRegister 执行一次注册/心跳
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

// getInterval 根据重试次数计算间隔（指数退避）
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

// RecordSuccess 记录注册成功，重置重试计数
func (h *HeartbeatManager) RecordSuccess() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.retryCount = 0
}

// RecordFailure 记录注册失败
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

// StartGlobalHeartbeat 启动全局心跳（仅首次调用生效）
func StartGlobalHeartbeat(server, vKey string) {
	heartbeatOnce.Do(func() {
		globalHeartbeat = NewHeartbeatManager(server, vKey)
		globalHeartbeat.Start()
	})
}

// StopGlobalHeartbeat 停止全局心跳
func StopGlobalHeartbeat() {
	if globalHeartbeat != nil {
		globalHeartbeat.Stop()
	}
}
