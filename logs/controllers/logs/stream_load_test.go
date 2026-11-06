// Package logs 子文件：Live Tail 全链路压测（P2-3）。
//
// 覆盖：
//   - buildTailLine 级别/时间戳/模板提取
//   - 令牌桶高并发压测（500+ 并发 goroutine）
//   - mergedTailSSE 多节点合并（httptest 模拟 logc SSE 源）
//   - proxyTailSSE 单节点代理
//   - 速率限制背压行为
package logs

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TestBuildTailLine_LevelExtraction 验证 buildTailLine 级别提取。
func TestBuildTailLine_LevelExtraction(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		level string
	}{
		{"ERROR", "2026-01-15 10:00:00 ERROR connection refused", "ERROR"},
		{"WARN", "2026-01-15 10:00:00 WARN slow query 3000ms", "WARN"},
		{"INFO", "2026-01-15 10:00:00 INFO user login", "INFO"},
		{"DEBUG", "2026-01-15 10:00:00 DEBUG cache hit", "DEBUG"},
		{"FATAL", "2026-01-15 10:00:00 FATAL out of memory", "FATAL"},
		{"无级别", "2026-01-15 10:00:00 plain message", ""},
		{"ERROR 在消息中", "user got ERROR code 500", "ERROR"},
		{"WARNING 识别为 WARN", "WARNING: disk almost full", "WARN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tl := buildTailLine(tc.raw, "127.0.0.1", 2020, 1, "full")
			if tl.Level != tc.level {
				t.Errorf("Level = %q, want %q", tl.Level, tc.level)
			}
		})
	}
}

// TestBuildTailLine_TimestampExtraction 验证 buildTailLine 时间戳提取。
func TestBuildTailLine_TimestampExtraction(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		timestamp string
	}{
		{"ISO 8601", "2026-01-15T10:30:45.123Z INFO msg", "2026-01-15T10:30:45.123Z"},
		{"ISO 无毫秒", "2026-01-15T10:30:45Z ERROR msg", "2026-01-15T10:30:45Z"},
		{"长格式", "2026-01-15 10:30:45.123 INFO msg", "2026-01-15 10:30:45.123"},
		{"长格式无毫秒", "2026-01-15 10:30:45 WARN msg", "2026-01-15 10:30:45"},
		{"无时间戳", "INFO msg without ts", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tl := buildTailLine(tc.raw, "127.0.0.1", 2020, 1, "full")
			if tl.Timestamp != tc.timestamp {
				t.Errorf("Timestamp = %q, want %q", tl.Timestamp, tc.timestamp)
			}
		})
	}
}

// TestBuildTailLine_TemplateExtraction 验证 buildTailLine 模板提取模式。
func TestBuildTailLine_TemplateExtraction(t *testing.T) {
	raw := "2026-01-15 10:00:00 INFO user 192.168.1.100 login from /api/v1/auth took 3000ms"
	tl := buildTailLine(raw, "127.0.0.1", 2020, 1, "template")
	if tl.Template == "" {
		t.Fatal("Template should not be empty")
	}
	if !strings.Contains(tl.Template, "<*>") {
		t.Errorf("Template should contain <*>: %s", tl.Template)
	}
	if len(tl.Params) == 0 {
		t.Error("Params should not be empty")
	}
	// 完整模式不提取模板
	tl2 := buildTailLine(raw, "127.0.0.1", 2020, 1, "full")
	if tl2.Template != "" {
		t.Errorf("full mode should not extract template: %s", tl2.Template)
	}
}

// TestTokenBucket_ConcurrentStress 令牌桶高并发压测（500 goroutine）。
func TestTokenBucket_ConcurrentStress(t *testing.T) {
	const (
		concurrency = 500
		totalTokens = 1000
	)
	tb := newTokenBucket(totalTokens, totalTokens, time.Hour) // 不自动补充
	defer tb.Stop()

	var consumed int64
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if tb.Allow(1) > 0 {
				atomic.AddInt64(&consumed, 1)
			}
		}()
	}

	close(start)
	wg.Wait()

	got := atomic.LoadInt64(&consumed)
	// 500 goroutine 各请求 1 个令牌，桶容量 1000，应全部满足
	if got != int64(concurrency) {
		t.Errorf("consumed = %d, want %d (令牌桶超发或漏发)", got, concurrency)
	}
}

// TestTokenBucket_RefillRate 验证令牌桶补充速率。
func TestTokenBucket_RefillRate(t *testing.T) {
	tb := newTokenBucket(10, 10, 50*time.Millisecond)
	defer tb.Stop()

	// 耗尽
	if got := tb.Allow(10); got != 10 {
		t.Fatalf("initial consume = %d, want 10", got)
	}
	if got := tb.Allow(1); got != 0 {
		t.Fatalf("expected 0 after exhausted, got %d", got)
	}

	// 等待补充
	time.Sleep(120 * time.Millisecond)
	got := tb.Allow(1)
	if got != 1 {
		t.Errorf("after refill, consume = %d, want 1", got)
	}
}

// TestTokenBucket_Cap 验证令牌桶容量上限。
func TestTokenBucket_Cap(t *testing.T) {
	tb := newTokenBucket(100, 10, 50*time.Millisecond)
	defer tb.Stop()

	// 等待大量补充，但不应超过 cap=10
	time.Sleep(200 * time.Millisecond)
	got := tb.Allow(10)
	if got != 10 {
		t.Errorf("capped consume = %d, want 10", got)
	}
	// 剩余应为 0（因为 cap 是 10）
	if got := tb.Allow(1); got != 0 {
		t.Errorf("after cap consume, remaining = %d, want 0", got)
	}
}

// TestMergedTailSSE_MultiNode 多节点合并压测（2 个 httptest logc 源，各推 100 行）。
func TestMergedTailSSE_MultiNode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 启动 2 个模拟 logc SSE 源
	lineCount := 100
	sources := make([]*httptest.Server, 2)
	for i := 0; i < 2; i++ {
		idx := i
		sources[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			flusher := w.(http.Flusher)
			for j := 0; j < lineCount; j++ {
				fmt.Fprintf(w, "data: 2026-01-15 10:00:%02d INFO node%d-line%d\n\n", j%60, idx, j)
				flusher.Flush()
			}
		}))
		defer sources[i].Close()
	}

	// 构造 serverSpec
	servers := make([]serverSpec, 2)
	for i, src := range sources {
		addr := strings.TrimPrefix(src.URL, "http://")
		servers[i] = serverSpec{
			address:  addr,
			ip:       "127.0.0.1",
			port:     2020 + i,
			clientID: int64(i + 1),
		}
	}

	// 临时替换 httpClient 为测试客户端（指向 httptest）
	origClient := httpClient
	httpClient = &http.Client{Timeout: 5 * time.Second}
	defer func() { httpClient = origClient }()

	// 创建 gin 测试上下文
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/logs/tail", nil)
	c.Request = c.Request.WithContext(context.Background())

	mergedTailSSE(c, servers, "/tmp/test.log", "", false, "", 0, "full")

	// 验证输出行数
	body := w.Body.String()
	events := strings.Count(body, "event: line")
	// 2 节点 × 100 行 = 200 行，但速率限制可能丢弃部分
	if events < 100 {
		t.Errorf("merged events = %d, want >= 100", events)
	}
	if !strings.Contains(body, "event: done") {
		t.Error("should contain done event")
	}
	t.Logf("mergedTailSSE: %d events from 2 nodes × %d lines", events, lineCount)
}

// TestMergedTailSSE_RateLimit 验证多节点合并时速率限制背压。
func TestMergedTailSSE_RateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 启动 1 个高速 logc 源（2000 行）
	totalLines := 2000
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for j := 0; j < totalLines; j++ {
			fmt.Fprintf(w, "data: 2026-01-15 10:00:%02d INFO line%d\n\n", j%60, j)
			flusher.Flush()
		}
	}))
	defer src.Close()

	addr := strings.TrimPrefix(src.URL, "http://")
	servers := []serverSpec{{address: addr, ip: "127.0.0.1", port: 2020, clientID: 1}}

	origClient := httpClient
	httpClient = &http.Client{Timeout: 5 * time.Second}
	defer func() { httpClient = origClient }()

	// 重置全局速率限制器（测试隔离）
	origLimiter := tailRateLimiter
	tailRateLimiter = newTokenBucket(100, 100, time.Second)
	defer func() { tailRateLimiter.Stop(); tailRateLimiter = origLimiter }()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/logs/tail", nil)
	c.Request = c.Request.WithContext(context.Background())

	mergedTailSSE(c, servers, "/tmp/test.log", "", false, "", 0, "full")

	body := w.Body.String()
	events := strings.Count(body, "event: line")
	rateLimited := strings.Count(body, "rateLimited")

	// 2000 行输入，速率限制 100/秒，应在 1 秒内完成大部分推送
	// 允许一定丢弃，但不应全部丢弃
	if events == 0 {
		t.Error("should have some events")
	}
	if rateLimited == 0 {
		t.Log("no rate limiting triggered (within 1s window)")
	}
	t.Logf("rate limit test: %d events, %d rate-limited markers (input %d)", events, rateLimited, totalLines)
}

// TestProxyTailSSE_SingleNode 单节点代理压测。
func TestProxyTailSSE_SingleNode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	totalLines := 200
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for j := 0; j < totalLines; j++ {
			fmt.Fprintf(w, "data: 2026-01-15 10:00:%02d INFO proxied-line%d\n\n", j%60, j)
			flusher.Flush()
		}
	}))
	defer src.Close()

	origClient := httpClient
	httpClient = &http.Client{Timeout: 5 * time.Second}
	defer func() { httpClient = origClient }()

	// 重置速率限制器
	origLimiter := tailRateLimiter
	tailRateLimiter = newTokenBucket(1000, 1000, time.Second)
	defer func() { tailRateLimiter.Stop(); tailRateLimiter = origLimiter }()

	addr := strings.TrimPrefix(src.URL, "http://")
	srv := serverSpec{address: addr, ip: "127.0.0.1", port: 2020, clientID: 1}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/logs/tail", nil)
	c.Request = c.Request.WithContext(context.Background())

	proxyTailSSE(c, srv, "/tmp/test.log", "", false, "", 0, "full")

	body := w.Body.String()
	events := strings.Count(body, "event: line")
	if events != totalLines {
		t.Errorf("proxied events = %d, want %d", events, totalLines)
	}
}

// TestMergedTailSSE_ClientDisconnect 验证客户端断开时 goroutine 正确退出。
func TestMergedTailSSE_ClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 启动一个长时间运行的 logc 源
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for j := 0; ; j++ {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			fmt.Fprintf(w, "data: 2026-01-15 10:00:%02d INFO line%d\n\n", j%60, j)
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer src.Close()

	addr := strings.TrimPrefix(src.URL, "http://")
	servers := []serverSpec{{address: addr, ip: "127.0.0.1", port: 2020, clientID: 1}}

	origClient := httpClient
	httpClient = &http.Client{Timeout: 5 * time.Second}
	defer func() { httpClient = origClient }()

	origLimiter := tailRateLimiter
	tailRateLimiter = newTokenBucket(1000, 1000, time.Second)
	defer func() { tailRateLimiter.Stop(); tailRateLimiter = origLimiter }()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 创建可取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	c.Request, _ = http.NewRequest(http.MethodPost, "/logs/tail", nil)
	c.Request = c.Request.WithContext(ctx)

	// 在 goroutine 中运行 mergedTailSSE
	done := make(chan struct{})
	go func() {
		mergedTailSSE(c, servers, "/tmp/test.log", "", false, "", 0, "full")
		close(done)
	}()

	// 等待一段时间后取消
	time.Sleep(50 * time.Millisecond)
	cancel()

	// 验证 goroutine 退出
	select {
	case <-done:
		// 正常退出
	case <-time.After(2 * time.Second):
		t.Fatal("mergedTailSSE did not exit after client disconnect")
	}
}

// TestMergedTailSSE_HTTPEndToEnd 端到端压测：2 节点 × 500 行，验证完整 SSE 输出格式。
func TestMergedTailSSE_HTTPEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 启动 2 个模拟 logc SSE 源
	lineCount := 500
	sources := make([]*httptest.Server, 2)
	for i := 0; i < 2; i++ {
		idx := i
		sources[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher := w.(http.Flusher)
			for j := 0; j < lineCount; j++ {
				fmt.Fprintf(w, "data: 2026-01-15 10:00:%02d INFO node%d-line%d\n\n", j%60, idx, j)
				flusher.Flush()
			}
		}))
		defer sources[i].Close()
	}

	servers := make([]serverSpec, 2)
	for i, src := range sources {
		addr := strings.TrimPrefix(src.URL, "http://")
		servers[i] = serverSpec{
			address:  addr,
			ip:       "127.0.0.1",
			port:     2020 + i,
			clientID: int64(i + 1),
		}
	}

	origClient := httpClient
	httpClient = &http.Client{Timeout: 10 * time.Second}
	defer func() { httpClient = origClient }()

	origLimiter := tailRateLimiter
	tailRateLimiter = newTokenBucket(2000, 2000, time.Second)
	defer func() { tailRateLimiter.Stop(); tailRateLimiter = origLimiter }()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/logs/tail", nil)
	c.Request = c.Request.WithContext(context.Background())

	mergedTailSSE(c, servers, "/tmp/test.log", "", false, "", 0, "full")

	body := w.Body.String()

	// 验证 SSE 格式
	if !strings.Contains(body, "event: connected") {
		t.Error("missing connected event")
	}
	if !strings.Contains(body, "event: done") {
		t.Error("missing done event")
	}

	// 解析所有 line 事件
	scanner := bufio.NewScanner(strings.NewReader(body))
	lineCount2 := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: line") {
			lineCount2++
		}
	}

	// 2 节点 × 500 行 = 1000 行，速率限制 2000/秒应全部通过
	if lineCount2 != 1000 {
		t.Errorf("total line events = %d, want 1000", lineCount2)
	}
	t.Logf("E2E: %d line events from 2 nodes × %d lines", lineCount2, lineCount)
}
