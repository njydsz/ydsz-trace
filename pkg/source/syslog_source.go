package source

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// SyslogSource Syslog 协议接收源（V2.2-10，FR-101）。
//
// 支持协议：
//   - UDP（RFC 3164 / RFC 5424 均按文本接收，不做严格语法校验）
//   - TCP（带换行分帧）
//
// 设计目标：
//   - 作为服务端被动接收，logc 从「拉取」变为「拉取 + 接收」双模式
//   - Read 时把已缓存的报文导出（供检索接口查询）
//   - Tail 时把报文实时推给回调（供 Live Tail 与告警使用）
//
// 配置项（FactoryConfig.Options）：
//   - listen      : 监听地址（默认 ":5140"）
//   - protocol    : "udp" | "tcp"（默认 "udp"）
//   - max_buffer  : 内存缓存最大条数（默认 10000）
//
// 注意：绑定 514 等特权端口需要 root 权限，生产建议使用 5140 + 反向代理。
type SyslogSource struct {
	info       SourceInfo
	listenAddr string
	protocol   string
	maxBuffer  int

	mu      sync.Mutex
	buffer  []string
	ln      net.Listener
	conn    net.PacketConn
	closed  chan struct{}
	started bool
}

// NewSyslogSource 创建 syslog 接收源。
func NewSyslogSource(cfg FactoryConfig) *SyslogSource {
	ss := &SyslogSource{
		listenAddr: ":5140",
		protocol:   "udp",
		maxBuffer:  10000,
		closed:     make(chan struct{}),
		info: SourceInfo{
			Type:        string(SourceTypeSyslog),
			Description: "syslog receiver (UDP/TCP)",
			StartedAt:   time.Now(),
		},
	}

	if v, ok := cfg.Options["listen"]; ok && v != "" {
		ss.listenAddr = v
	}
	if v, ok := cfg.Options["protocol"]; ok && v != "" {
		ss.protocol = strings.ToLower(v)
	}
	if v, ok := cfg.Options["max_buffer"]; ok && v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			ss.maxBuffer = n
		}
	}
	return ss
}

// parseInt 解析正整数（供配置读取，失败返回 0）。
func parseInt(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid digit %q", r)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

// Info 实现 Source 接口。
func (ss *SyslogSource) Info() SourceInfo { return ss.info }

// Start 启动接收协程。
//
// 幂等：重复调用直接返回。启动失败返回 error（调用方决定是否降级）。
func (ss *SyslogSource) Start() error {
	ss.mu.Lock()
	if ss.started {
		ss.mu.Unlock()
		return nil
	}
	ss.started = true
	ss.mu.Unlock()

	switch ss.protocol {
	case "tcp":
		ln, err := net.Listen("tcp", ss.listenAddr)
		if err != nil {
			return fmt.Errorf("syslog: 监听 TCP %s 失败: %w", ss.listenAddr, err)
		}
		ss.mu.Lock()
		ss.ln = ln
		ss.mu.Unlock()
		go ss.acceptLoop()
	case "udp":
		conn, err := net.ListenPacket("udp", ss.listenAddr)
		if err != nil {
			return fmt.Errorf("syslog: 监听 UDP %s 失败: %w", ss.listenAddr, err)
		}
		ss.mu.Lock()
		ss.conn = conn
		ss.mu.Unlock()
		go ss.readLoop(conn)
	default:
		return fmt.Errorf("syslog: 不支持的协议 %q", ss.protocol)
	}

	log.Printf("[source] syslog 接收已启动: %s %s", ss.protocol, ss.listenAddr)
	return nil
}

// acceptLoop TCP 连接接受循环。
func (ss *SyslogSource) acceptLoop() {
	for {
		conn, err := ss.ln.Accept()
		if err != nil {
			select {
			case <-ss.closed:
				return
			default:
			}
			log.Printf("[source] syslog TCP accept 失败: %v", err)
			return
		}
		go ss.handleTCPConn(conn)
	}
}

// handleTCPConn 处理单个 TCP 连接（按换行分帧）。
func (ss *SyslogSource) handleTCPConn(conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("[source] syslog TCP 连接关闭失败: %v", err)
		}
	}()

	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), MaxLineSize)
	for sc.Scan() {
		select {
		case <-ss.closed:
			return
		default:
		}
		line := strings.TrimRight(sc.Text(), "\r")
		if line != "" {
			ss.append(line)
		}
	}
}

// readLoop UDP 读取循环。
func (ss *SyslogSource) readLoop(conn net.PacketConn) {
	buf := make([]byte, 65536)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			select {
			case <-ss.closed:
				return
			default:
			}
			log.Printf("[source] syslog UDP 读取失败: %v", err)
			return
		}
		line := strings.TrimRight(string(buf[:n]), "\r\n")
		if line != "" {
			ss.append(line)
		}
	}
}

// append 把一条报文写入有界缓存。
func (ss *SyslogSource) append(line string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	ss.buffer = append(ss.buffer, line)
	// 超出容量时丢弃最旧数据（FIFO）
	if len(ss.buffer) > ss.maxBuffer {
		ss.buffer = ss.buffer[len(ss.buffer)-ss.maxBuffer:]
	}
}

// Read 实现 Source 接口：导出缓存报文并应用过滤。
//
// 说明：syslog 源不主动消费缓存（Tail 仍可读到历史），
// 保证多次检索不会互相影响。
// 注意：不自动启动接收服务（避免多次检索请求重复监听端口）。
func (ss *SyslogSource) Read(_ context.Context, _ string, cfg ScanConfig, output io.Writer) (int64, error) {
	ss.mu.Lock()
	lines := make([]string, len(ss.buffer))
	copy(lines, ss.buffer)
	ss.mu.Unlock()

	reader := strings.NewReader(strings.Join(lines, "\n"))
	return ScanAndFilter(reader, cfg, output)
}

// Tail 实现 Source 接口：把新到的报文实时推给回调。
//
// 实现方式：轮询缓存增量（简单可靠，避免为每个订阅者维护 channel）。
// 轮询间隔 200ms，对 Live Tail 足够。
func (ss *SyslogSource) Tail(ctx context.Context, _ string, cfg TailConfig, callback func(line string) error) error {
	if err := ss.Start(); err != nil {
		return err
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	ss.mu.Lock()
	cursor := len(ss.buffer)
	ss.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			ss.mu.Lock()
			if len(ss.buffer) > cursor {
				lines := make([]string, len(ss.buffer)-cursor)
				copy(lines, ss.buffer[cursor:])
				cursor = len(ss.buffer)
				ss.mu.Unlock()

				filter, err := newLineFilter(ScanConfig{
					Key: cfg.Key, Regex: cfg.Regex, Level: cfg.Level,
				})
				if err != nil {
					return err
				}
				for _, line := range lines {
					if filter.shouldKeep(line) {
						if err := callback(line); err != nil {
							return err
						}
					}
				}
			} else {
				ss.mu.Unlock()
			}
		}
	}
}

// Discover 实现 Source 接口：syslog 无动态目标，返回空快照。
func (ss *SyslogSource) Discover(ctx context.Context) (<-chan DiscoveryEvent, error) {
	ch := make(chan DiscoveryEvent, 1)
	ch <- DiscoveryEvent{Type: "snapshot"}
	go func() {
		defer close(ch)
		<-ctx.Done()
	}()
	return ch, nil
}

// Stop 停止接收并释放端口。
func (ss *SyslogSource) Stop() {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	select {
	case <-ss.closed:
		return
	default:
		close(ss.closed)
	}

	if ss.conn != nil {
		_ = ss.conn.Close()
		ss.conn = nil
	}
	if ss.ln != nil {
		_ = ss.ln.Close()
		ss.ln = nil
	}
	ss.started = false
}

func init() {
	RegisterSource(SourceTypeSyslog, func(cfg FactoryConfig) (Source, error) {
		return NewSyslogSource(cfg), nil
	})
}
