// Package logs 子文件：SSE 流式日志搜索与实时跟踪。
//
// 提供两个 SSE 端点：
//   - /logs/stream：多客户端并发搜索，流式返回每个节点的进度
//   - /logs/tail：代理 logc 的实时跟踪，SSE 推送新增日志行（支持多节点合并、速率限制、模板提取）
package logs

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/sanitize"

	"github.com/gin-gonic/gin"
)

// sseTailLine 推送的日志行统一结构（FR-405）。
type sseTailLine struct {
	Timestamp    string   `json:"timestamp"`
	IP           string   `json:"ip"`
	Port         int      `json:"port"`
	ClientID     int64    `json:"clientId"`
	Level        string   `json:"level"`
	Content      string   `json:"content"`
	Template     string   `json:"template,omitempty"`
	Params       []string `json:"params,omitempty"`
	RateLimited  bool     `json:"rateLimited,omitempty"`
	DroppedCount int      `json:"droppedCount,omitempty"`
}

// tokenBucket 简易令牌桶速率限制器。
type tokenBucket struct {
	mu       sync.Mutex
	cap      int
	tokens   int
	interval time.Duration
	rate     int // tokens per interval
	stopCh   chan struct{}
}

// newTokenBucket 创建一个令牌桶。
// rate: 每个 interval 补充的令牌数；cap: 桶的最大容量。
func newTokenBucket(rate, cap int, interval time.Duration) *tokenBucket {
	tb := &tokenBucket{
		cap:      cap,
		tokens:   cap,
		interval: interval,
		rate:     rate,
		stopCh:   make(chan struct{}),
	}
	go tb.refill()
	return tb
}

// refill 周期性补充令牌。
func (tb *tokenBucket) refill() {
	ticker := time.NewTicker(tb.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			tb.mu.Lock()
			tb.tokens += tb.rate
			if tb.tokens > tb.cap {
				tb.tokens = tb.cap
			}
			tb.mu.Unlock()
		case <-tb.stopCh:
			return
		}
	}
}

// Stop 停止令牌补充 goroutine。
func (tb *tokenBucket) Stop() {
	close(tb.stopCh)
}

// Allow 尝试消费 n 个令牌，返回实际消费数。
func (tb *tokenBucket) Allow(n int) int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if tb.tokens <= 0 {
		return 0
	}
	if n > tb.tokens {
		n = tb.tokens
	}
	tb.tokens -= n
	return n
}

// tailRateLimiter 全局 Tail 端点推送速率限制器：500 行/秒。
var tailRateLimiter = newTokenBucket(500, 500, time.Second)

// StreamReq 流式搜索请求体（与 LogsReq 对齐）。
type StreamReq struct {
	Client    int64  `json:"client"`
	Item      int64  `json:"item"`
	Date      string `json:"date"`
	Key       string `json:"key"`
	Line      int64  `json:"line"`
	Regex     bool   `json:"regex"`
	Level     string `json:"level"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	Query     string `json:"query"`
}

// Stream 多客户端并发搜索的 SSE 进度推送端点。
//
// SSE 事件流：
//   - connected：{ "total": N } —— 搜索开始
//   - progress：{ "index": i, "server": "ip:port", "status": "ok|fail" } —— 单节点完成
//   - done：{ "success": N, "fail": N } —— 全部完成
//
// 注意：流式模式下不直接下载 zip，仅推送进度与汇总结果下载链接。
func Stream(c *gin.Context) {
	var req StreamReq
	data, err := c.GetRawData()
	if err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}
	if err := json.Unmarshal(data, &req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}

	// SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	if req.Item == 0 || (req.Key == "" && req.Query == "") || req.Date == "" {
		sseError(c, "item/query+key 不能同时为空，且 date 必填")
		return
	}

	// P0-4 修复：使用 sanitize 包白名单校验日期格式，防止路径穿越
	if _, ok := sanitize.SafeDate(req.Date); !ok {
		sseError(c, "日期格式非法，仅允许 YYYY-MM-DD")
		return
	}

	item := models.ReadItem(req.Item)
	if item.Id == 0 {
		sseError(c, "项目不存在")
		return
	}
	path := item.LogPath + item.LogPrefix + req.Date + item.LogSuffix + ".log"

	var clients []models.TClient
	if req.Client != 0 {
		clients = []models.TClient{models.ReadClient(req.Client)}
	} else {
		clients, err = models.QueryAllClient()
		if err != nil || len(clients) == 0 {
			sseError(c, "无可用客户端")
			return
		}
	}

	total := len(clients)
	sseEvent(c, "connected", map[string]interface{}{"total": total})
	c.Writer.Flush()

	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	fail := 0

	for i, cl := range clients {
		if cl.Id == 0 {
			continue
		}
		wg.Add(1)
		go func(idx int, client models.TClient) {
			defer wg.Done()
			server := client.Ip + ":" + client.Port
			sseEvent(c, "progress", map[string]interface{}{
				"index": idx, "server": server, "status": "start",
			})
			c.Writer.Flush()

			err := postToLogc(server, path, req.Key, req.Line,
				fmt.Sprintf("./temp/stream/%d.zip", client.Id),
				req.Regex, req.Level, req.StartTime, req.EndTime, req.Query)

			mu.Lock()
			status := "ok"
			if err != nil {
				status = "fail"
				fail++
				log.Printf("流式搜索节点 %s 失败: %v", server, err)
			} else {
				success++
			}
			mu.Unlock()

			sseEvent(c, "progress", map[string]interface{}{
				"index": idx, "server": server, "status": status,
			})
			c.Writer.Flush()
		}(i, cl)
	}

	wg.Wait()
	sseEvent(c, "done", map[string]interface{}{
		"success": success, "fail": fail,
	})
	c.Writer.Flush()
}

// Tail 代理 logc tail 端点：将 logc 的 SSE 流式响应转发给客户端。
//
// @Summary 实时日志追踪流
// @Description 代理 logc tail 端点的 SSE 流，将新增日志行实时推送到客户端。支持关键词、正则、日志级别过滤。
// @Tags LogSearch
// @Accept json
// @Produce text/event-stream
// @Param body body StreamReq true "追踪条件：item + date 必填"
// @Success 200 {string} string "SSE 事件流（data: ...\n\n）"
// @Failure 400 {object} api.Response "参数错误"
// @Failure 404 {object} api.Response "项目不存在或无可用客户端"
// @Router /logs/tail [post]
func Tail(c *gin.Context) {
	var req StreamReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}
	if req.Item == 0 || req.Date == "" {
		api.Fail(c, api.CodeBadRequest, "item/date 不能为空")
		return
	}

	// 可选查询参数：提取模式 extract=template|full
	extract := c.DefaultQuery("extract", "full")
	if extract != "template" && extract != "full" {
		extract = "full"
	}

	item := models.ReadItem(req.Item)
	if item.Id == 0 {
		api.Fail(c, api.CodeNotFound, "项目不存在")
		return
	}
	path := item.LogPath + item.LogPrefix + req.Date + item.LogSuffix + ".log"

	// 确定要 tail 的客户端
	var servers []serverSpec
	if req.Client != 0 {
		cl := models.ReadClient(req.Client)
		if cl.Id != 0 {
			port, _ := stringToInt(cl.Port)
			servers = []serverSpec{{
				address:  cl.Ip + ":" + cl.Port,
				ip:       cl.Ip,
				port:     port,
				clientID: cl.Id,
			}}
		}
	} else {
		clients, err := models.QueryAllClient()
		if err != nil || len(clients) == 0 {
			api.Fail(c, api.CodeNotFound, "无可用客户端")
			return
		}
		for _, cl := range clients {
			port, _ := stringToInt(cl.Port)
			servers = append(servers, serverSpec{
				address:  cl.Ip + ":" + cl.Port,
				ip:       cl.Ip,
				port:     port,
				clientID: cl.Id,
			})
		}
	}

	if len(servers) == 0 {
		api.Fail(c, api.CodeNotFound, "未找到有效客户端")
		return
	}

	// 单节点模式：直接代理，封装为统一 sseTailLine 结构
	if len(servers) == 1 {
		proxyTailSSE(c, servers[0], path, req.Key, req.Regex, req.Level, req.Line, extract)
		return
	}

	// 多节点模式：合并 SSE 流 + 速率限制 + 时间排序
	mergedTailSSE(c, servers, path, req.Key, req.Regex, req.Level, req.Line, extract)
}

// serverSpec logc 节点地址信息（含元数据）。
type serverSpec struct {
	address  string // ip:port
	ip       string
	port     int
	clientID int64
}

// proxyTailSSE 代理单个 logc 的 tail 响应，封装为 sseTailLine 结构。
func proxyTailSSE(c *gin.Context, srv serverSpec, path, key string, regex bool, level string, line int64, extract string) {
	tailReqBody, _ := json.Marshal(map[string]interface{}{
		"path":           path,
		"key":            key,
		"regex":          regex,
		"level":          level,
		"followDuration": 60,
	})

	resp, err := httpClient.Post(
		"http://"+srv.address+"/file/tail",
		"application/json",
		bytes.NewReader(tailReqBody),
	)
	if err != nil {
		api.Fail(c, api.CodeServerError, "连接 logc 代理失败")
		return
	}
	defer resp.Body.Close()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// 速率限制器（单节点也限速）
	dropped := 0

	reader := bufio.NewReader(resp.Body)
	for {
		data, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				log.Printf("代理读取 SSE 失败: %v", err)
			}
			// 推送剩余丢弃汇总
			if dropped > 0 {
				emitRateLimited(c, srv, dropped)
			}
			return
		}
		// 解析 SSE data
		if len(data) > 6 && data[:6] == "data: " {
			content := strings.TrimRight(data[6:], "\r\n")
			if len(content) == 0 {
				continue
			}

			// 速率限制
			if tailRateLimiter.Allow(1) == 0 {
				dropped++
				continue
			}
			// 如果有丢弃，先推送丢弃汇总
			if dropped > 0 {
				emitRateLimited(c, srv, dropped)
				dropped = 0
			}

			tl := buildTailLine(content, srv.ip, srv.port, srv.clientID, extract)
			emitTailLine(c, tl)
		}
	}
}

// emitRateLimited 推送速率限制丢弃汇总。
func emitRateLimited(c *gin.Context, srv serverSpec, count int) {
	tl := sseTailLine{
		IP:           srv.ip,
		Port:         srv.port,
		ClientID:     srv.clientID,
		Level:        "WARN",
		Content:      fmt.Sprintf("[rate limited] dropped %d lines", count),
		RateLimited:  true,
		DroppedCount: count,
	}
	emitTailLine(c, tl)
}

// emitTailLine 将 sseTailLine 编码为 SSE 事件推送。
func emitTailLine(c *gin.Context, tl sseTailLine) {
	b, err := json.Marshal(tl)
	if err != nil {
		return
	}
	c.Writer.WriteString("event: line\ndata: " + string(b) + "\n\n")
	c.Writer.Flush()
}

// buildTailLine 从原始日志构造 sseTailLine（含级别、时间戳提取、模板提取）。
func buildTailLine(raw, ip string, port int, clientID int64, extract string) sseTailLine {
	content := raw
	level := extractLevel(content)
	timestamp := extractTimestamp(content)

	tl := sseTailLine{
		Timestamp: timestamp,
		IP:        ip,
		Port:      port,
		ClientID:  clientID,
		Level:     level,
		Content:   content,
	}

	if extract == "template" {
		tmpl, params := extractTemplateFields(content)
		tl.Template = tmpl
		tl.Params = params
	}

	return tl
}

// extractTemplateFields 从日志行提取模板字符串与参数列表。
// 返回：<template_str, params> — 模板中动态参数用 <*> 占位，params 为提取出的具体值。
func extractTemplateFields(line string) (string, []string) {
	params := []string{}

	// 记录已匹配的位置避免重叠，按出现顺序收集参数
	type matchPos struct {
		start, end int
		val        string
	}
	var allMatches []matchPos

	patterns := []*regexp.Regexp{uuidPattern, ipv4Pattern, pathPattern, hexPattern, numberPattern}
	for _, re := range patterns {
		for _, m := range re.FindAllStringIndex(line, -1) {
			allMatches = append(allMatches, matchPos{m[0], m[1], line[m[0]:m[1]]})
		}
	}

	// 按起始位置排序（插入排序，数量通常不多）
	for i := 1; i < len(allMatches); i++ {
		for j := i; j > 0 && allMatches[j].start < allMatches[j-1].start; j-- {
			allMatches[j], allMatches[j-1] = allMatches[j-1], allMatches[j]
		}
	}

	// 去重重叠
	var filtered []matchPos
	lastEnd := -1
	for _, m := range allMatches {
		if m.start >= lastEnd {
			filtered = append(filtered, m)
			lastEnd = m.end
		}
	}

	// 替换为 <*>
	var sb strings.Builder
	prev := 0
	for _, m := range filtered {
		sb.WriteString(line[prev:m.start])
		sb.WriteString("<*>")
		params = append(params, m.val)
		prev = m.end
	}
	sb.WriteString(line[prev:])

	// 压缩空格
	template := strings.Join(strings.Fields(sb.String()), " ")
	return template, params
}

// 模板提取用正则（包级复用 search.go 的归一化语义）。
var (
	uuidPattern    = compilePattern(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	ipv4Pattern    = compilePattern(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	pathPattern    = compilePattern(`(?:/[\w\-\.]+){2,}`)
	hexPattern     = compilePattern(`\b[0-9a-fA-F]{16,}\b`)
	numberPattern  = compilePattern(`\b\d+\.\d+\b|\b\d+\b`)
)

// compilePattern 预编译正则（简化写法）。
func compilePattern(s string) *regexp.Regexp {
	return regexp.MustCompile(s)
}

// stringToInt 将字符串转 int，失败返回 0。
func stringToInt(s string) (int, error) {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid int: %s", s)
		}
		n = n*10 + int(ch-'0')
	}
	return n, nil
}

// mergedTailSSE 合并多个 logc server 的 tail 流，支持速率限制 + 模板提取。
func mergedTailSSE(c *gin.Context, servers []serverSpec, path, key string, regex bool, level string, line int64, extract string) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	sseEvent(c, "connected", map[string]interface{}{"servers": len(servers)})
	c.Writer.Flush()

	var wg sync.WaitGroup
	// 行通道（携带来源节点信息）
	type rawLine struct {
		server serverSpec
		data   string
	}
	lineCh := make(chan rawLine, 500)
	doneCh := make(chan serverSpec, len(servers))

	for _, srv := range servers {
		wg.Add(1)
		go func(s serverSpec) {
			defer wg.Done()
			tailReqBody, _ := json.Marshal(map[string]interface{}{
				"path":           path,
				"key":            key,
				"regex":          regex,
				"level":          level,
				"followDuration": 60,
			})

			resp, err := httpClient.Post(
				"http://"+s.address+"/file/tail",
				"application/json",
				bytes.NewReader(tailReqBody),
			)
			if err != nil {
				doneCh <- s
				return
			}
			defer resp.Body.Close()

			reader := bufio.NewReader(resp.Body)
			for {
				data, err := reader.ReadString('\n')
				if err != nil {
					break
				}
				if len(data) > 6 && data[:6] == "data: " {
					content := strings.TrimRight(data[6:], "\r\n")
					if len(content) == 0 {
						continue
					}
					select {
					case lineCh <- rawLine{server: s, data: content}:
					case <-c.Request.Context().Done():
						return
					}
				}
			}
			doneCh <- s
		}(srv)
	}

	// 等待所有 goroutine 结束后关闭 channel
	go func() {
		wg.Wait()
		close(lineCh)
	}()

	// 合并推送 + 速率限制
	finished := 0
	pendingDropped := 0

	for {
		select {
		case raw, ok := <-lineCh:
			if !ok {
				// 推送剩余丢弃汇总
				if pendingDropped > 0 {
					emitMergedRateLimited(c, pendingDropped)
				}
				sseEvent(c, "done", map[string]interface{}{"servers": len(servers)})
				c.Writer.Flush()
				return
			}

			// 速率限制
			if tailRateLimiter.Allow(1) == 0 {
				pendingDropped++
				continue
			}
			// 推送丢弃汇总
			if pendingDropped > 0 {
				emitMergedRateLimited(c, pendingDropped)
				pendingDropped = 0
			}

			tl := buildTailLine(raw.data, raw.server.ip, raw.server.port, raw.server.clientID, extract)
			emitTailLine(c, tl)

		case <-doneCh:
			finished++
			if finished >= len(servers) {
				// 排空 lineCh
				for raw := range lineCh {
					if tailRateLimiter.Allow(1) == 0 {
						pendingDropped++
						continue
					}
					if pendingDropped > 0 {
						emitMergedRateLimited(c, pendingDropped)
						pendingDropped = 0
					}
					tl := buildTailLine(raw.data, raw.server.ip, raw.server.port, raw.server.clientID, extract)
					emitTailLine(c, tl)
				}
				if pendingDropped > 0 {
					emitMergedRateLimited(c, pendingDropped)
				}
				sseEvent(c, "done", map[string]interface{}{"servers": len(servers)})
				c.Writer.Flush()
				return
			}
		case <-c.Request.Context().Done():
			return
		}
	}
}

// emitMergedRateLimited 多节点模式下的速率限制丢弃汇总推送。
func emitMergedRateLimited(c *gin.Context, count int) {
	tl := sseTailLine{
		Level:        "WARN",
		Content:      fmt.Sprintf("[rate limited] dropped %d lines", count),
		RateLimited:  true,
		DroppedCount: count,
	}
	emitTailLine(c, tl)
}

// sseEvent 发送一个 SSE 事件。
func sseEvent(c *gin.Context, event string, data interface{}) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	c.Writer.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(b)))
}

// sseError 发送 SSE 错误事件。
func sseError(c *gin.Context, msg string) {
	sseEvent(c, "error", map[string]interface{}{"message": msg})
	c.Writer.Flush()
}
