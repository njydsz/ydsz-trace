// Package agentlink 实现 logc 侧的长连接通道客户端。
//
// 职责（P1-1/P1-2/P1-3/P1-4）：
//   - 启动时向 logs 建立 WebSocket 长连接
//   - 断线后指数退避重连（借鉴 frp）
//   - 在长连接上跑控制帧（注册/心跳/配置接收）与数据帧（日志上报）
//   - 接收 logs 下发的配置并回调
package agentlink

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"ydsz-trace/pkg/agentlink"
	pkgtls "ydsz-trace/pkg/tls"
	"ydsz-trace/pkg/spool"
	"ydsz-trace/pkg/wsconn"
)

// Client 长连接通道客户端。
type Client struct {
	mu sync.Mutex

	server    string // logs 地址（ip:port）
	vKey      string
	regKey    string
	localPort string

	conn   *wsconn.Conn
	closed chan struct{}

	// onConfig 配置下发回调（P1-4）。
	onConfig func(*agentlink.ConfigPush)

	// onItems 日志项列表回调（P1-3 注册应答）。
	onItems func([]agentlink.ItemSpec)
	// onMode 模式切换回调（P1-5 灰度）。
	onMode func(string)

	// sendCh 待发送的日志批次（P1-3）。
	sendCh chan *agentlink.LogBatch

	// stopCh 停止信号。
	stopCh   chan struct{}
	stopOnce sync.Once

	// modeMu 保护 mode 与 startOnce（P1-5）。
	modeMu   sync.Mutex
	mode     string
	startOnce sync.Once

	// spool 背压队列管理器（FR-103，V2.2-09）。
	// 连接断开时把批次写入磁盘队列，恢复后补传。
	spool *SpoolManager
}

// NewClient 创建长连接客户端。
func NewClient(server, vKey, regKey, localPort string) *Client {
	return &Client{
		server:    server,
		vKey:      vKey,
		regKey:    regKey,
		localPort: localPort,
		sendCh:    make(chan *agentlink.LogBatch, 256),
		stopCh:    make(chan struct{}),
		closed:    make(chan struct{}),
	}
}

// SetSpool 设置背压队列管理器（FR-103）。
// 必须在 Start 之前调用；nil 表示不启用背压。
func (c *Client) SetSpool(m *SpoolManager) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.spool = m
}

// SpoolStats 返回背压队列统计。
func (c *Client) SpoolStats() (spool.QueueStats, bool) {
	c.mu.Lock()
	m := c.spool
	c.mu.Unlock()
	if m == nil {
		return spool.QueueStats{}, false
	}
	return m.Stats(), true
}

// SetConfigHandler 设置配置下发回调（P1-4）。
func (c *Client) SetConfigHandler(fn func(*agentlink.ConfigPush)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onConfig = fn
}

// SetItemsHandler 设置日志项列表回调（P1-3）。
func (c *Client) SetItemsHandler(fn func([]agentlink.ItemSpec)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onItems = fn
}

// SetModeHandler 设置模式切换回调（P1-5）。
func (c *Client) SetModeHandler(fn func(string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onMode = fn
}

// SubmitBatch 提交一批日志到发送队列（FR-103 背压）。
//
// 优先写入内存队列；内存队列满时写入磁盘 spool 队列（不丢弃）。
// spool 队列也满时按策略丢弃最旧数据。
func (c *Client) SubmitBatch(batch *agentlink.LogBatch) {
	select {
	case c.sendCh <- batch:
		return
	case <-c.stopCh:
		return
	default:
		// 内存队列满：尝试写入磁盘 spool。
		c.mu.Lock()
		m := c.spool
		c.mu.Unlock()
		if m != nil {
			if err := m.enqueue(batch); err != nil {
				log.Printf("[agentlink] 背压入队失败，丢弃批次 %s（%d 行）: %v", batch.BatchNo, len(batch.Lines), err)
			} else {
				log.Printf("[agentlink] 内存队列满，批次 %s（%d 行）已写入磁盘队列", batch.BatchNo, len(batch.Lines))
			}
			return
		}
		// 未启用背压：丢弃并告警。
		log.Printf("[agentlink] 发送队列已满且未启用背压，丢弃批次 %s（%d 行）", batch.BatchNo, len(batch.Lines))
	}
}

// Start 启动长连接循环（阻塞，应在 goroutine 中调用）。
//
// 重连策略：初始 1s，指数增长到上限 60s；成功后重置。
func (c *Client) Start() {
	backoff := time.Second
	const maxBackoff = 60 * time.Second

	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		if err := c.runOnce(); err != nil {
			log.Printf("[agentlink] 连接中断: %v（%v 后重连）", err, backoff)
		}

		select {
		case <-c.stopCh:
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// Stop 停止长连接循环。
func (c *Client) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn != nil {
			_ = conn.Close()
		}
	})
}

// ApplyMode 应用服务端下发的模式（P1-5 灰度）。
//
// "pull" 时主动断开当前连接并停止重连；
// "push" 时启动重连（若已在运行则先停后重启）。
func (c *Client) ApplyMode(mode string) {
	c.modeMu.Lock()
	defer c.modeMu.Unlock()
	if mode != "pull" {
		mode = "push"
	}
	if mode == c.mode {
		return
	}
	c.mode = mode
	if mode == "pull" {
		// 重置 startOnce，使后续切回 push 时能重新启动。
		c.startOnce = sync.Once{}
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn != nil {
			_ = conn.Close()
			log.Printf("[agentlink] 模式切换为 pull，已断开长连接")
		}
		return
	}
	// push：若未在运行则启动。
	c.startOnce.Do(func() {
		go c.Start()
		log.Printf("[agentlink] 模式切换为 push，已启动重连")
	})
}

// runOnce 建立并维持一条长连接，直到出错。
func (c *Client) runOnce() error {
	scheme := "ws"
	tlsCfg, err := c.tlsConfig()
	if err != nil {
		return err
	}
	if tlsCfg != nil {
		scheme = "wss"
	}
	url := scheme + "://" + c.server + "/agent/link"

	conn, err := wsconn.Dial(url, tlsCfg)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	log.Printf("[agentlink] 长连接已建立: %s", url)

	// 注册握手。
	if err := c.sendRegister(); err != nil {
		_ = conn.Close()
		return err
	}

	// 启动发送协程。
	sendDone := make(chan struct{})
	go c.sendLoop(conn, sendDone)

	// 主读循环。
	readErr := c.readLoop(conn)

	<-sendDone
	_ = conn.Close()

	c.mu.Lock()
	c.conn = nil
	c.mu.Unlock()

	return readErr
}

// tlsConfig 返回 TLS 配置（未启用时 nil）。
func (c *Client) tlsConfig() (*tls.Config, error) {
	cfg := pkgtls.LoadFromEnv()
	if !cfg.Enabled {
		return nil, nil
	}
	if err := cfg.ValidateClient(false); err != nil {
		return nil, err
	}
	return cfg.ClientTLSConfig()
}

// sendRegister 发送注册控制帧。
func (c *Client) sendRegister() error {
	body, _ := json.Marshal(map[string]interface{}{
		"vkey":      c.vKey,
		"localPort": c.localPort,
		"timestamp": time.Now().Unix(),
	})
	frame := &agentlink.Frame{
		Type:    agentlink.FrameTypeControl,
		Payload: agentlink.ControlPayload(agentlink.ControlRegister, body),
	}
	return c.sendFrame(frame)
}

// sendFrame 发送帧（并发安全）。
func (c *Client) sendFrame(f *agentlink.Frame) error {
	data, err := f.Encode()
	if err != nil {
		return err
	}
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return errNotConnected
	}
	return conn.WriteMessage(data)
}

// sendLoop 发送协程：定时 ping + 队列消费 + 磁盘补传。
func (c *Client) sendLoop(conn *wsconn.Conn, done chan struct{}) {
	defer close(done)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// 磁盘补传 ticker（每 5s 检查一次 spool 队列）。
	spoolTicker := time.NewTicker(5 * time.Second)
	defer spoolTicker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			if err := conn.WritePing(nil); err != nil {
				return
			}
		case <-spoolTicker.C:
			// 从磁盘队列补传（连接正常时才会执行）。
			c.drainSpool(conn)
		case batch := <-c.sendCh:
			if err := c.writeBatch(conn, batch); err != nil {
				log.Printf("[agentlink] 上报批次 %s 失败: %v", batch.BatchNo, err)
				// 发送失败：写入磁盘 spool，连接恢复后补传。
				c.mu.Lock()
				m := c.spool
				c.mu.Unlock()
				if m != nil {
					if err := m.enqueue(batch); err != nil {
						log.Printf("[agentlink] 发送失败且入队失败: %v", err)
					}
				}
			}
		}
	}
}

// drainSpool 从磁盘队列补传一个批次（若有）。
func (c *Client) drainSpool(conn *wsconn.Conn) {
	c.mu.Lock()
	m := c.spool
	c.mu.Unlock()
	if m == nil {
		return
	}
	m.drainTo(func(batch *agentlink.LogBatch) error {
		return c.writeBatch(conn, batch)
	}, 10)
}

// writeBatch 编码并发送一个日志批次。
func (c *Client) writeBatch(conn *wsconn.Conn, batch *agentlink.LogBatch) error {
	payload, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	frame := &agentlink.Frame{
		Type:    agentlink.FrameTypeData,
		Payload: payload,
	}
	data, err := frame.Encode()
	if err != nil {
		return err
	}
	return conn.WriteMessage(data)
}

// readLoop 读循环：处理控制帧。
func (c *Client) readLoop(conn *wsconn.Conn) error {
	for {
		msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if msg.OPCode == 0x8 {
			return errPeerClosed
		}
		if msg.OPCode != 0x2 {
			continue
		}
		if err := c.handleFrame(msg.Payload); err != nil {
			log.Printf("[agentlink] 处理帧失败: %v", err)
		}
	}
}

// handleFrame 解析并处理一帧。
func (c *Client) handleFrame(data []byte) error {
	f, _, err := agentlink.DecodeFrame(data)
	if err != nil {
		return err
	}
	if f.Type != agentlink.FrameTypeControl {
		return nil
	}
	code, body, err := agentlink.ParseControlCode(f.Payload)
	if err != nil {
		return err
	}

	switch code {
	case agentlink.ControlPong:
		// 心跳应答，无需处理。
	case agentlink.ControlRegisterOK:
		var accept agentlink.RegisterAccept
		if err := json.Unmarshal(body, &accept); err != nil {
			log.Printf("[agentlink] 注册应答解析失败: %v", err)
			return nil
		}
		log.Printf("[agentlink] 注册成功 clientId=%d items=%d", accept.ClientID, len(accept.Items))
		c.mu.Lock()
		itemsHandler := c.onItems
		modeHandler := c.onMode
		c.mu.Unlock()
		if itemsHandler != nil {
			itemsHandler(accept.Items)
		}
		if modeHandler != nil && accept.AgentMode != "" {
			modeHandler(accept.AgentMode)
		}
	case agentlink.ControlConfigPush:
		var cfg agentlink.ConfigPush
		if err := json.Unmarshal(body, &cfg); err != nil {
			return err
		}
		c.mu.Lock()
		handler := c.onConfig
		c.mu.Unlock()
		if handler != nil {
			handler(&cfg)
		}
	case agentlink.ControlLogACK:
		log.Printf("[agentlink] 服务端已确认批次: %s", string(body))
	case agentlink.ControlModeSwitch:
		var req struct {
			Mode string `json:"mode"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		log.Printf("[agentlink] 收到模式切换指令: %s", req.Mode)
		c.ApplyMode(req.Mode)
	default:
		log.Printf("[agentlink] 未知控制码: 0x%x", code)
	}
	return nil
}

var (
	errNotConnected = errors.New("未连接")
	errPeerClosed   = errors.New("对端已关闭")
)
