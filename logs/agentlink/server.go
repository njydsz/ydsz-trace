// Package agentlink 实现 logs 侧的长连接通道服务端。
//
// 职责（P1-1/P1-2/P1-3/P1-4）：
//   - 接受 logc 的 WebSocket 长连接（/agent/link）
//   - 鉴权（复用注册密钥的 HMAC 签名）
//   - 维护连接注册表，支持按客户端 ID 下发配置
//   - 接收日志批次并写入存储（复用 pipeline）
//   - 心跳保活与死连接清理
package agentlink

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/agentlink"
	"ydsz-trace/pkg/wsconn"
)

const (
	// handshakeTimeout 握手阶段超时。
	handshakeTimeout = 15 * time.Second
	// writeTimeout 单帧写入超时。
	writeTimeout = 10 * time.Second
	// maxSignatureDrift 签名时间戳最大偏差（秒）。
	maxSignatureDrift int64 = 300
)

// Server 长连接通道服务端。
type Server struct {
	mu sync.RWMutex

	// registerKey 注册鉴权密钥（与 /client/register 一致）。
	registerKey string

	// conns 当前活跃连接，按客户端 ID 索引。
	conns map[int64]*agentConn

	// onBatch 日志批次回调（P1-3）。
	onBatch func(*agentlink.LogBatch)

	// configVersion 配置版本号（下发时递增）。
	configVersion int64
}

// agentConn 单条 agent 连接。
type agentConn struct {
	ID       int64
	VKey     string
	Addr     string
	Conn     *wsconn.Conn
	LastPing time.Time

	mu sync.Mutex
}

// NewServer 创建长连接服务端。
func NewServer(registerKey string) *Server {
	return &Server{
		registerKey: registerKey,
		conns:       make(map[int64]*agentConn),
	}
}

// SetBatchHandler 设置日志批次回调。
func (s *Server) SetBatchHandler(fn func(*agentlink.LogBatch)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onBatch = fn
}

// SetRegisterKey 更新注册密钥（配置热更新）。
func (s *Server) SetRegisterKey(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registerKey = key
}

// ServeWS 处理一条 WebSocket 升级请求（Gin handler）。
func (s *Server) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsconn.Upgrade(w, r)
	if err != nil {
		log.Printf("[agentlink] 升级失败: %v", err)
		return
	}
	defer conn.Close()

	addr := r.RemoteAddr
	log.Printf("[agentlink] 新连接: %s", addr)

	// 握手：首帧必须是 ControlRegister。
	_ = conn.SetReadDeadline(time.Now().Add(handshakeTimeout))
	msg, err := conn.ReadMessage()
	if err != nil {
		log.Printf("[agentlink] 握手读取失败 %s: %v", addr, err)
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	ac, err := s.handleRegister(conn, msg, addr)
	if err != nil {
		log.Printf("[agentlink] 握手鉴权失败 %s: %v", addr, err)
		_ = conn.WriteClose("unauthorized")
		return
	}

	// 注册成功应答（P1-3：携带该 client 下启用的日志项）。
	items, _ := models.QueryEnabledItemsByClientId(ac.ID)
	specs := make([]agentlink.ItemSpec, 0, len(items))
	for _, it := range items {
		specs = append(specs, agentlink.ItemSpec{
			ItemID:    it.Id,
			ItemName:  it.ItemName,
			LogPath:   it.LogPath,
			LogPrefix: it.LogPrefix,
			LogSuffix: it.LogSuffix,
			Status:    it.Status,
		})
	}
	// P1-5：下发客户端级灰度模式（agent_mode）。
	agentMode := ""
	if c := models.ReadClient(ac.ID); c.Id != 0 {
		agentMode = c.AgentMode
	}
	accept := agentlink.RegisterAccept{
		ClientID:   ac.ID,
		ServerTime: time.Now().Unix(),
		Items:      specs,
		AgentMode:  agentMode,
	}
	ack, _ := json.Marshal(accept)
	if err := ac.writeControl(agentlink.ControlRegisterOK, ack); err != nil {
		log.Printf("[agentlink] 注册应答失败 %s: %v", addr, err)
		return
	}
	log.Printf("[agentlink] 注册应答已下发 %d 个日志项 id=%d", len(specs), ac.ID)

	s.register(ac)
	defer s.unregister(ac.ID)

	log.Printf("[agentlink] 客户端上线: id=%d addr=%s", ac.ID, addr)

	// 主读循环。
	for {
		m, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if m.OPCode == 0x8 {
			return
		}
		if m.OPCode != 0x2 {
			continue
		}
		if err := s.handleFrame(ac, m.Payload); err != nil {
			log.Printf("[agentlink] 处理帧失败 id=%d: %v", ac.ID, err)
		}
	}
}

// handleRegister 解析注册帧并鉴权。
func (s *Server) handleRegister(conn *wsconn.Conn, msg *wsconn.Message, addr string) (*agentConn, error) {
	f, _, err := agentlink.DecodeFrame(msg.Payload)
	if err != nil {
		return nil, err
	}
	if f.Type != agentlink.FrameTypeControl {
		return nil, errors.New("首帧必须为控制帧")
	}
	code, _, err := agentlink.ParseControlCode(f.Payload)
	if err != nil {
		return nil, err
	}
	if code != agentlink.ControlRegister {
		return nil, errors.New("首帧必须为注册帧")
	}

	var req struct {
		VKey      string `json:"vkey"`
		LocalPort string `json:"localPort"`
		Timestamp int64  `json:"timestamp"`
		Signature string `json:"signature"`
	}
	_, body, err := agentlink.ParseControlCode(f.Payload)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	// 签名校验（与 HTTP 注册一致）。
	if err := s.verifySignature(req.VKey, req.Timestamp, req.Signature); err != nil {
		return nil, err
	}

	// 客户端 ID 由服务端根据 IP+Port+VKey 解析（防止伪造）。
	// 未预注册的客户端拒绝接入（与 HTTP 注册策略一致）。
	id := resolveClientByAddr(addr, req.LocalPort, req.VKey)
	if id == 0 {
		return nil, errors.New("客户端未预注册")
	}

	return &agentConn{
		ID:       id,
		VKey:     req.VKey,
		Addr:     addr,
		Conn:     conn,
		LastPing: time.Now(),
	}, nil
}

// verifySignature 校验 HMAC-SHA1 签名。
func (s *Server) verifySignature(vkey string, timestamp int64, signature string) error {
	s.mu.RLock()
	secret := s.registerKey
	s.mu.RUnlock()

	drift := time.Now().Unix() - timestamp
	if drift < -maxSignatureDrift || drift > maxSignatureDrift {
		return errors.New("签名时间戳超偏")
	}
	tsStr := strconv.FormatInt(timestamp, 10)
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(tsStr + "." + vkey))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return errors.New("签名不匹配")
	}
	return nil
}

// register 注册连接。
func (s *Server) register(ac *agentConn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 踢掉同 ID 的旧连接（新连接优先）。
	if old, ok := s.conns[ac.ID]; ok {
		_ = old.Conn.Close()
	}
	s.conns[ac.ID] = ac
}

// unregister 注销连接。
func (s *Server) unregister(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.conns[id]; ok {
		delete(s.conns, id)
		_ = cur.Conn.Close()
	}
}

// handleFrame 处理一帧。
func (s *Server) handleFrame(ac *agentConn, data []byte) error {
	f, _, err := agentlink.DecodeFrame(data)
	if err != nil {
		return err
	}

	switch f.Type {
	case agentlink.FrameTypeControl:
		code, _, err := agentlink.ParseControlCode(f.Payload)
		if err != nil {
			return err
		}
		switch code {
		case agentlink.ControlPing:
			ac.LastPing = time.Now()
			return ac.writeControl(agentlink.ControlPong, nil)
		case agentlink.ControlLogACK:
			// 客户端确认（一般不出现），忽略。
			return nil
		default:
			log.Printf("[agentlink] 未知控制码 id=%d: 0x%x", ac.ID, code)
			return nil
		}
	case agentlink.FrameTypeData:
		var batch agentlink.LogBatch
		if err := json.Unmarshal(f.Payload, &batch); err != nil {
			return err
		}
		batch.ClientID = ac.ID // 强制以服务端解析为准，防伪造
		s.mu.RLock()
		handler := s.onBatch
		s.mu.RUnlock()
		if handler != nil {
			handler(&batch)
		}
		// ACK。
		return ac.writeControl(agentlink.ControlLogACK, []byte(batch.BatchNo))
	default:
		return errors.New("未知帧类型")
	}
}

// writeControl 发送控制帧。
func (ac *agentConn) writeControl(code agentlink.ControlCode, body []byte) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.Conn == nil {
		return errors.New("连接已关闭")
	}
	_ = ac.Conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	defer func() { _ = ac.Conn.SetWriteDeadline(time.Time{}) }()

	frame := &agentlink.Frame{
		Type:    agentlink.FrameTypeControl,
		Payload: agentlink.ControlPayload(code, body),
	}
	data, err := frame.Encode()
	if err != nil {
		return err
	}
	return ac.Conn.WriteMessage(data)
}

// PushConfig 向指定客户端下发配置（P1-4）。
//
// 返回 error 表示客户端不在线；调用方应决定是否回落到数据库配置。
func (s *Server) PushConfig(clientID int64, cfg *agentlink.ConfigPush) error {
	s.mu.RLock()
	ac, ok := s.conns[clientID]
	s.mu.RUnlock()
	if !ok {
		return errors.New("客户端不在线")
	}

	s.mu.Lock()
	s.configVersion++
	cfg.Version = s.configVersion
	s.mu.Unlock()

	body, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return ac.writeControl(agentlink.ControlConfigPush, body)
}

// PushMode 向指定客户端下发长连接模式切换（P1-5 灰度）。
//
// mode 取值为 "push" / "pull"；客户端收到后调用 ApplyMode 动态切换。
func (s *Server) PushMode(clientID int64, mode string) error {
	s.mu.RLock()
	ac, ok := s.conns[clientID]
	s.mu.RUnlock()
	if !ok {
		return errors.New("客户端不在线")
	}
	body, err := json.Marshal(map[string]string{"mode": mode})
	if err != nil {
		return err
	}
	return ac.writeControl(agentlink.ControlModeSwitch, body)
}

// PushConfigAll 向所有在线客户端广播配置（P1-4）。
func (s *Server) PushConfigAll(cfg *agentlink.ConfigPush) {
	s.mu.RLock()
	ids := make([]int64, 0, len(s.conns))
	for id := range s.conns {
		ids = append(ids, id)
	}
	s.mu.RUnlock()

	for _, id := range ids {
		if err := s.PushConfig(id, cfg); err != nil {
			log.Printf("[agentlink] 下发配置失败 id=%d: %v", id, err)
		}
	}
}

// OnlineCount 返回当前在线连接数。
func (s *Server) OnlineCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.conns)
}

// StartReaper 启动死连接清理循环（超过 2 分钟无心跳则断开）。
func (s *Server) StartReaper(interval time.Duration) func() {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				s.reapStale(2 * time.Minute)
			}
		}
	}()
	return func() { close(stop) }
}

// reapStale 清理超过 threshold 未心跳的连接。
func (s *Server) reapStale(threshold time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, ac := range s.conns {
		if now.Sub(ac.LastPing) > threshold {
			log.Printf("[agentlink] 清理死连接 id=%d（%v 无心跳）", id, now.Sub(ac.LastPing))
			delete(s.conns, id)
			_ = ac.Conn.Close()
		}
	}
}
