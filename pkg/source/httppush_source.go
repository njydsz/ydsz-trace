package source

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HTTPPushSource HTTP 推送接收源（V2.2-10，FR-101）。
//
// 支持两类请求体：
//   1. 纯文本：每行一条日志，Content-Type: text/plain
//   2. JSON 数组：[{"message":"...","level":"INFO"}, ...]
//   3. 单条 JSON：{"message":"...","level":"INFO"}
//
// 设计目标：
//   - 让应用侧用一行 http.Post 即可接入，无需 SDK
//   - 接收后进入与 syslog 相同的缓冲，统一走 ScanAndFilter
//
// 配置项（FactoryConfig.Options）：
//   - listen : 监听地址（默认 ":5141"）
//   - path   : 接收路径（默认 "/logs"）
//
// 路由注册：
//   Source 创建后调用 RegisterHTTPHandler(mux) 挂载到指定 mux；
//   若调用方未注册，源仍可接收但无入口（便于测试直接调用 ServeHTTP）。
type HTTPPushSource struct {
	info       SourceInfo
	listenAddr string
	path       string
	maxBuffer  int

	mu     sync.Mutex
	buffer []string
	server *http.Server
	closed chan struct{}
}

// NewHTTPPushSource 创建 HTTP 推送源。
func NewHTTPPushSource(cfg FactoryConfig) *HTTPPushSource {
	hp := &HTTPPushSource{
		listenAddr: ":5141",
		path:       "/logs",
		maxBuffer:  10000,
		closed:     make(chan struct{}),
		info: SourceInfo{
			Type:        string(SourceTypeHTTPPush),
			Description: "HTTP push log receiver",
			StartedAt:   time.Now(),
		},
	}

	if v, ok := cfg.Options["listen"]; ok && v != "" {
		hp.listenAddr = v
	}
	if v, ok := cfg.Options["path"]; ok && v != "" {
		hp.path = "/" + strings.TrimPrefix(v, "/")
	}
	if v, ok := cfg.Options["max_buffer"]; ok && v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			hp.maxBuffer = n
		}
	}
	return hp
}

// Info 实现 Source 接口。
func (hp *HTTPPushSource) Info() SourceInfo { return hp.info }

// append 把日志行写入有界缓冲。
func (hp *HTTPPushSource) append(line string) {
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return
	}
	hp.mu.Lock()
	defer hp.mu.Unlock()
	hp.buffer = append(hp.buffer, line)
	if len(hp.buffer) > hp.maxBuffer {
		hp.buffer = hp.buffer[len(hp.buffer)-hp.maxBuffer:]
	}
}

// ServeHTTP 实现 http.Handler。
func (hp *HTTPPushSource) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != hp.path {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	contentType := r.Header.Get("Content-Type")
	switch {
	case strings.Contains(contentType, "application/json"):
		if err := hp.handleJSON(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	default:
		if err := hp.handleText(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}

// handleText 处理纯文本请求体（每行一条）。
func (hp *HTTPPushSource) handleText(r *http.Request) error {
	defer func() {
		if cerr := r.Body.Close(); cerr != nil {
			log.Printf("[source] HTTP body 关闭失败: %v", cerr)
		}
	}()

	sc := bufio.NewScanner(r.Body)
	sc.Buffer(make([]byte, 0, 64*1024), MaxLineSize)
	for sc.Scan() {
		hp.append(sc.Text())
	}
	return sc.Err()
}

// jsonLogEntry JSON 推送的日志条目。
type jsonLogEntry struct {
	Message string `json:"message"`
	Level   string `json:"level,omitempty"`
	Host    string `json:"host,omitempty"`
}

// handleJSON 处理 JSON 请求体（单条对象或数组）。
func (hp *HTTPPushSource) handleJSON(r *http.Request) error {
	defer func() {
		if cerr := r.Body.Close(); cerr != nil {
			log.Printf("[source] HTTP body 关闭失败: %v", cerr)
		}
	}()

	data, err := io.ReadAll(io.LimitReader(r.Body, MaxJSONBodySize))
	if err != nil {
		return fmt.Errorf("读取请求体失败: %w", err)
	}

	// 先尝试数组
	var list []jsonLogEntry
	if err := json.Unmarshal(data, &list); err == nil {
		for _, e := range list {
			hp.append(formatJSONEntry(e))
		}
		return nil
	}

	// 再尝试单条
	var single jsonLogEntry
	if err := json.Unmarshal(data, &single); err != nil {
		return fmt.Errorf("JSON 解析失败: %w", err)
	}
	hp.append(formatJSONEntry(single))
	return nil
}

// formatJSONEntry 把 JSON 条目格式化为统一日志行。
func formatJSONEntry(e jsonLogEntry) string {
	var b strings.Builder
	if e.Level != "" {
		b.WriteString(strings.ToUpper(e.Level))
		b.WriteByte(' ')
	}
	if e.Host != "" {
		b.WriteString(e.Host)
		b.WriteByte(' ')
	}
	b.WriteString(e.Message)
	return b.String()
}

// MaxJSONBodySize JSON 请求体大小上限（4MB）。
const MaxJSONBodySize = 4 * 1024 * 1024

// RegisterHTTPHandler 把接收端点挂载到给定 mux。
func (hp *HTTPPushSource) RegisterHTTPHandler(mux *http.ServeMux) {
	mux.Handle(hp.path, hp)
	log.Printf("[source] HTTP push 端点已注册: POST %s", hp.path)
}

// Start 启动 HTTP 服务。
func (hp *HTTPPushSource) Start() error {
	hp.mu.Lock()
	if hp.server != nil {
		hp.mu.Unlock()
		return nil
	}

	mux := http.NewServeMux()
	hp.RegisterHTTPHandler(mux)

	srv := &http.Server{
		Addr:              hp.listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	hp.server = srv
	hp.mu.Unlock()

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			select {
			case <-hp.closed:
			default:
				log.Printf("[source] HTTP push 服务异常退出: %v", err)
			}
		}
	}()

	log.Printf("[source] HTTP push 接收已启动: %s%s", hp.listenAddr, hp.path)
	return nil
}

// Read 实现 Source 接口：导出缓冲日志。
//
// 注意：不自动启动 HTTP 服务。服务端启动由 logc 主流程显式调用 Start，
// 避免多次 Read（检索请求）触发重复监听同一端口。
func (hp *HTTPPushSource) Read(_ context.Context, _ string, cfg ScanConfig, output io.Writer) (int64, error) {
	hp.mu.Lock()
	lines := make([]string, len(hp.buffer))
	copy(lines, hp.buffer)
	hp.mu.Unlock()

	reader := strings.NewReader(strings.Join(lines, "\n"))
	return ScanAndFilter(reader, cfg, output)
}

// Tail 实现 Source 接口：轮询缓冲增量实时推送。
func (hp *HTTPPushSource) Tail(ctx context.Context, _ string, cfg TailConfig, callback func(line string) error) error {
	if err := hp.Start(); err != nil {
		return err
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	hp.mu.Lock()
	cursor := len(hp.buffer)
	hp.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			hp.mu.Lock()
			if len(hp.buffer) <= cursor {
				hp.mu.Unlock()
				continue
			}
			lines := make([]string, len(hp.buffer)-cursor)
			copy(lines, hp.buffer[cursor:])
			cursor = len(hp.buffer)
			hp.mu.Unlock()

			filter, err := newLineFilter(ScanConfig{Key: cfg.Key, Regex: cfg.Regex, Level: cfg.Level})
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
		}
	}
}

// Discover 实现 Source 接口：HTTP push 无动态目标。
func (hp *HTTPPushSource) Discover(ctx context.Context) (<-chan DiscoveryEvent, error) {
	ch := make(chan DiscoveryEvent, 1)
	ch <- DiscoveryEvent{Type: "snapshot"}
	go func() {
		defer close(ch)
		<-ctx.Done()
	}()
	return ch, nil
}

// Stop 停止 HTTP 服务。
func (hp *HTTPPushSource) Stop() {
	hp.mu.Lock()
	srv := hp.server
	hp.server = nil
	select {
	case <-hp.closed:
	default:
		close(hp.closed)
	}
	hp.mu.Unlock()

	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("[source] HTTP push 关闭失败: %v", err)
		}
	}
}

func init() {
	RegisterSource(SourceTypeHTTPPush, func(cfg FactoryConfig) (Source, error) {
		return NewHTTPPushSource(cfg), nil
	})
}
