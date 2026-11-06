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
//
// 轻量化策略（P0-3）：
//   - 首次调用完整注册（RegisterLocalIp），携带 source_type 等元数据。
//   - 后续调用轻量 Ping，仅刷新在线状态，显著降低心跳带宽。
type HeartbeatManager struct {
	mu              sync.Mutex
	server          string
	vKey            string
	regKey          string
	localLogcPort   string
	stopCh          chan struct{}
	doneCh          chan struct{}
	isRunning       bool
	retryCount      int
	lastFailTime    time.Time
	firstRegisterDone bool
}

// NewHeartbeatManager 创建指定目标服务地址与密钥的心跳管理器。
// regKey 为注册鉴权密钥，与服务端 getRegisterKey 一致。
// localLogcPort 用于轻量心跳中携带 logc 端口（服务端可据此更新回调地址）。
func NewHeartbeatManager(server, vKey, regKey, localLogcPort string) *HeartbeatManager {
	return &HeartbeatManager{
		server:        server,
		vKey:          vKey,
		regKey:        regKey,
		localLogcPort: localLogcPort,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
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

// doRegister 执行一次心跳并根据结果更新退避状态。
//
// 首次调用完整注册（RegisterLocalIp），后续调用轻量 Ping（P0-3）。
// 成功时重置 retryCount，失败时递增 retryCount 并记录时间。
func (h *HeartbeatManager) doRegister() {
	h.mu.Lock()
	server, vKey, regKey, port := h.server, h.vKey, h.regKey, h.localLogcPort
	first := !h.firstRegisterDone
	h.mu.Unlock()

	var err error
	if first {
		_, err = RegisterLocalIp(server, vKey, regKey)
	} else {
		err = Ping(server, vKey, regKey, port)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if err != nil {
		h.retryCount++
		h.lastFailTime = time.Now()
		log.Printf("[heartbeat] 心跳失败 (第 %d 次)，下次间隔: %v，错误: %v",
			h.retryCount, h.getIntervalLocked(), err)
		return
	}

	// 首次注册成功后标记，后续走轻量 Ping。
	if first {
		h.firstRegisterDone = true
	}

	// 成功时重置退避
	if h.retryCount > 0 {
		log.Printf("[heartbeat] 心跳恢复，重置退避计数 (此前失败 %d 次)", h.retryCount)
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
// regKey 为注册鉴权密钥，与服务端 getRegisterKey 一致。
// localLogcPort 用于轻量心跳携带端口。
func StartGlobalHeartbeat(server, vKey, regKey, localLogcPort string) {
	heartbeatOnce.Do(func() {
		globalHeartbeat = NewHeartbeatManager(server, vKey, regKey, localLogcPort)
		globalHeartbeat.Start()
	})
}

// StopGlobalHeartbeat 停止全局心跳（空值时静默返回）。
func StopGlobalHeartbeat() {
	if globalHeartbeat != nil {
		globalHeartbeat.Stop()
	}
}
