// Package logs 子文件：traceId 调用链聚合。
//
// 给定 traceId，并发检索所有 logc 节点对应日志文件，按节点分组返回命中行，
// 便于排查跨节点调用链异常。底层复用 /logs/search 同源的 queryOneLogc 管道，
// 使用布尔查询 `traceId:<value>` 让 logc 侧识别结构化字段（若日志行未显式
// 出现 traceId 字段名，logc 会退化为全文包含匹配）。
package logs

import (
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// traceReq traceId 调用链聚合请求体。
type traceReq struct {
	// TraceID 要检索的 traceId（必填）。
	TraceID string `json:"traceId"`
	// Date 日志日期（YYYYMMDD，必填）。
	Date string `json:"date"`
	// Item 日志项 ID（0 表示使用 client 对应的全部配置，但不推荐）。
	Item int64 `json:"item"`
	// Client 客户端 ID（0 表示全部客户端并发检索）。
	Client int64 `json:"client"`
	// Regex 是否把 traceId 作为正则表达式匹配（默认 false = 精确包含）。
	Regex bool `json:"regex"`
	// MaxPerNode 单节点返回行上限（0 使用默认值 5000）。
	MaxPerNode int `json:"maxPerNode"`
}

// traceNodeResult 单个节点的命中结果。
type traceNodeResult struct {
	Node  string   `json:"node"`  // ip:port
	Count int      `json:"count"` // 命中行数
	Lines []string `json:"lines"` // 命中行（按原始行序）
}

// traceResp 聚合响应。
type traceResp struct {
	TraceID string            `json:"traceId"` // 实际检索的 traceId
	Nodes   int               `json:"nodes"`   // 参与检索的节点总数
	Total   int               `json:"total"`   // 全部节点命中行总和
	Results []traceNodeResult `json:"results"` // 各节点命中明细
}

// Trace 端点：按 traceId 跨节点检索日志，返回按节点分组的调用链片段。
// POST /logs/trace
func Trace(c *gin.Context) {
	var req traceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}
	req.TraceID = strings.TrimSpace(req.TraceID)
	if req.TraceID == "" || req.Date == "" {
		api.Fail(c, api.CodeBadRequest, "traceId 与 date 均为必填")
		return
	}
	if req.Item == 0 {
		api.Fail(c, api.CodeBadRequest, "item 未指定")
		return
	}

	const defaultMaxPerNode = 5000
	maxPerNode := req.MaxPerNode
	if maxPerNode <= 0 || maxPerNode > defaultMaxPerNode {
		maxPerNode = defaultMaxPerNode
	}

	item := models.ReadItem(req.Item)
	if item.Id == 0 {
		api.Fail(c, api.CodeNotFound, "项目不存在")
		return
	}
	path := item.LogPath + item.LogPrefix + req.Date + item.LogSuffix + ".log"

	// 构造 logc SearchReq：使用布尔查询 `traceId:<value>`。
	// logc 侧会识别 traceId 字段，未命中字段名时退化为全文包含。
	searchReq := SearchReq{
		Client:   req.Client,
		Item:     req.Item,
		Date:     req.Date,
		Key:      "",
		Regex:    req.Regex,
		Query:    "traceId:" + req.TraceID,
		PageNo:   1,
		PageSize: maxPerNode,
	}

	var targets []targetSpec
	if req.Client != 0 {
		cl := models.ReadClient(req.Client)
		if cl.Id == 0 {
			api.Fail(c, api.CodeNotFound, "客户端不存在")
			return
		}
		targets = []targetSpec{{server: cl.Ip + ":" + cl.Port, label: cl.Ip + ":" + cl.Port}}
	} else {
		clients, err := models.QueryAllClient()
		if err != nil || len(clients) == 0 {
			api.Fail(c, api.CodeNotFound, "无可用客户端")
			return
		}
		for _, cl := range clients {
			if cl.Id == 0 {
				continue
			}
			targets = append(targets, targetSpec{server: cl.Ip + ":" + cl.Port, label: cl.Ip + ":" + cl.Port})
		}
	}
	if len(targets) == 0 {
		api.Fail(c, api.CodeNotFound, "未找到有效客户端")
		return
	}

	// 并发检索所有节点
	var mu sync.Mutex
	results := make([]traceNodeResult, 0, len(targets))
	var wg sync.WaitGroup
	var total atomic.Int64
	sem := make(chan struct{}, 20)

	for _, tgt := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(t targetSpec) {
			defer wg.Done()
			defer func() { <-sem }()

			lines, err := queryTraceFromLogc(t.server, path, searchReq)
			if err != nil {
				log.Printf("trace 查询节点 %s 失败: %v", t.server, err)
				return
			}
			if len(lines) == 0 {
				return
			}
			mu.Lock()
			results = append(results, traceNodeResult{
				Node:  t.label,
				Count: len(lines),
				Lines: lines,
			})
			mu.Unlock()
			total.Add(int64(len(lines)))
		}(tgt)
	}
	wg.Wait()

	c.JSON(http.StatusOK, api.Response{
		Code:    api.CodeSuccess,
		Message: "查询成功",
		Data: traceResp{
			TraceID: req.TraceID,
			Nodes:   len(targets),
			Total:   int(total.Load()),
			Results: results,
		},
	})
}

// queryTraceFromLogc 调用单个 logc 节点检索 traceId 匹配行。
// 复用 SearchReq 结构；maxLines 通过 PageSize 透传。
func queryTraceFromLogc(server string, path string, req SearchReq) ([]string, error) {
	return queryOneLogc(server, path, req, req.PageSize)
}

// traceWaterfallReq TraceID 瀑布视图请求体。
type traceWaterfallReq struct {
	// TraceID 要检索的 traceId（必填）。
	TraceID string `json:"traceId"`
	// Date 日志日期（YYYYMMDD，必填）。
	Date string `json:"date"`
	// Item 日志项 ID（必填）。
	Item int64 `json:"item"`
	// Client 客户端 ID（0 表示全部客户端并发检索）。
	Client int64 `json:"client"`
	// MaxPerNode 单节点返回行上限（0 使用默认值 5000）。
	MaxPerNode int `json:"maxPerNode"`
}

// traceSpan 瀑布视图中的单个时间线段。
type traceSpan struct {
	Node      string `json:"node"`      // ip:port
	Timestamp string `json:"timestamp"`  // 日志中的时间戳
	Level     string `json:"level"`     // 级别
	Content   string `json:"content"`   // 摘要（前 200 字符）
	Service   string `json:"service"`   // 尝试识别的服务名
	Thread    string `json:"thread"`    // 线程名
	IsError   bool   `json:"isError"`   // 是否 ERROR
}

// traceWaterfallResp TraceID 瀑布视图响应体。
type traceWaterfallResp struct {
	TraceID        string       `json:"traceId"`
	TraceFormat    string       `json:"traceFormat"`    // w3c/b3/skywalking/custom
	TotalNodes     int          `json:"totalNodes"`
	Spans          []traceSpan  `json:"spans"`          // 按时间排序
	ErrorCount     int          `json:"errorCount"`
	FirstError     *traceSpan   `json:"firstError"`     // 首个错误 span
	LongestGapMs   int64        `json:"longestGapMs"`   // 相邻 span 间最大时间差（毫秒）
	LongestSpanIdx int          `json:"longestSpanIdx"` // 最长耗时 span 在 spans 中的索引
	DurationMs     int64        `json:"durationMs"`     // 总跨度（首尾 span 时间差）
}

// TraceWaterfall 端点增强：自动识别 TraceID 格式（W3C/B3/SkyWalking/自定义），
// 并发检索所有节点后按时间排序输出瀑布视图。
// POST /logs/trace/waterfall
func TraceWaterfall(c *gin.Context) {
	var req traceWaterfallReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}
	req.TraceID = strings.TrimSpace(req.TraceID)
	if req.TraceID == "" || req.Date == "" || req.Item == 0 {
		api.Fail(c, api.CodeBadRequest, "traceId、date 与 item 均为必填")
		return
	}

	const defaultMaxPerNode = 5000
	maxPerNode := req.MaxPerNode
	if maxPerNode <= 0 || maxPerNode > defaultMaxPerNode {
		maxPerNode = defaultMaxPerNode
	}

	item := models.ReadItem(req.Item)
	if item.Id == 0 {
		api.Fail(c, api.CodeNotFound, "项目不存在")
		return
	}
	path := item.LogPath + item.LogPrefix + req.Date + item.LogSuffix + ".log"

	// 计算 TraceID 格式
	traceFormat := detectTraceIDFormat(req.TraceID)

	// 构造 logc SearchReq
	searchReq := SearchReq{
		Client:   req.Client,
		Item:     req.Item,
		Date:     req.Date,
		Key:      "",
		Regex:    false,
		Query:    "traceId:" + req.TraceID,
		PageNo:   1,
		PageSize: maxPerNode,
	}

	var targets []targetSpec
	if req.Client != 0 {
		cl := models.ReadClient(req.Client)
		if cl.Id == 0 {
			api.Fail(c, api.CodeNotFound, "客户端不存在")
			return
		}
		targets = []targetSpec{{server: cl.Ip + ":" + cl.Port, label: cl.Ip + ":" + cl.Port}}
	} else {
		clients, err := models.QueryAllClient()
		if err != nil || len(clients) == 0 {
			api.Fail(c, api.CodeNotFound, "无可用客户端")
			return
		}
		for _, cl := range clients {
			if cl.Id == 0 {
				continue
			}
			targets = append(targets, targetSpec{server: cl.Ip + ":" + cl.Port, label: cl.Ip + ":" + cl.Port})
		}
	}
	if len(targets) == 0 {
		api.Fail(c, api.CodeNotFound, "未找到有效客户端")
		return
	}

	// 并发检索所有节点
	type nodeLines struct {
		node  string
		lines []string
	}
	var mu sync.Mutex
	var allSpans []traceSpan
	var wg sync.WaitGroup
	var totalNodes int64
	sem := make(chan struct{}, 20)

	for _, tgt := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(t targetSpec) {
			defer wg.Done()
			defer func() { <-sem }()

			lines, err := queryTraceFromLogc(t.server, path, searchReq)
			if err != nil {
				log.Printf("waterfall 查询节点 %s 失败: %v", t.server, err)
				return
			}
			if len(lines) == 0 {
				return
			}

			totalNodes++
			spans := make([]traceSpan, 0, len(lines))
			for _, line := range lines {
				spans = append(spans, parseTraceSpan(line, t.label))
			}

			mu.Lock()
			allSpans = append(allSpans, spans...)
			mu.Unlock()
		}(tgt)
	}
	wg.Wait()

	// 按时间排序
	sort.SliceStable(allSpans, func(i, j int) bool {
		return allSpans[i].Timestamp < allSpans[j].Timestamp
	})

	// 统计错误、首个错误、最长 gap
	var errorCount int
	var firstError *traceSpan
	var longestGapMs int64
	var longestSpanIdx int
	var durationMs int64
	var firstTs, lastTs int64
	for i := range allSpans {
		if allSpans[i].IsError {
			errorCount++
			if firstError == nil {
				copy := allSpans[i]
				firstError = &copy
			}
		}
		ts := spanTimeMillis(allSpans[i].Timestamp)
		if i == 0 {
			firstTs = ts
		}
		if ts > 0 {
			lastTs = ts
		}
		if i > 0 {
			prev := spanTimeMillis(allSpans[i-1].Timestamp)
			if ts > 0 && prev > 0 {
				gap := ts - prev
				if gap > longestGapMs {
					longestGapMs = gap
					longestSpanIdx = i
				}
			}
		}
	}
	durationMs = lastTs - firstTs
	if durationMs < 0 {
		durationMs = 0
	}

	c.JSON(http.StatusOK, api.Response{
		Code:    api.CodeSuccess,
		Message: "查询成功",
		Data: traceWaterfallResp{
			TraceID:        req.TraceID,
			TraceFormat:    traceFormat,
			TotalNodes:     int(totalNodes),
			Spans:          allSpans,
			ErrorCount:     errorCount,
			FirstError:     firstError,
			LongestGapMs:   longestGapMs,
			LongestSpanIdx: longestSpanIdx,
			DurationMs:     durationMs,
		},
	})
}

// parseTraceSpan 从日志行中提取瀑布视图所需的 span 信息。
func parseTraceSpan(line, node string) traceSpan {
	ts := extractTimestamp(line)
	level := extractLevel(line)
	content := line
	if len(content) > 200 {
		content = content[:200]
	}
	service := extractService(line)
	thread := extractThread(line)
	isError := level == "ERROR" || level == "FATAL"

	return traceSpan{
		Node:      node,
		Timestamp: ts,
		Level:     level,
		Content:   content,
		Service:   service,
		Thread:    thread,
		IsError:   isError,
	}
}

// spanTimeMillis 解析 span 时间戳为毫秒数（失败返回 0）。
func spanTimeMillis(ts string) int64 {
	if ts == "" {
		return 0
	}
	// 尝试 ISO 8601
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.UnixMilli()
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.UnixMilli()
	}
	// 尝试 "2006-01-02 15:04:05.000"
	if t, err := time.Parse("2006-01-02 15:04:05.000", ts); err == nil {
		return t.UnixMilli()
	}
	if t, err := time.Parse("2006-01-02 15:04:05", ts); err == nil {
		return t.UnixMilli()
	}
	return 0
}

// detectTraceIDFormat 自动识别 TraceID 格式。
func detectTraceIDFormat(traceID string) string {
	if len(traceID) == 32 || len(traceID) == 16 {
		allHex := true
		for _, c := range traceID {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				allHex = false
				break
			}
		}
		if allHex {
			return "b3"
		}
	}
	if strings.Contains(traceID, "-") {
		return "w3c"
	}
	if len(traceID) > 50 {
		return "skywalking"
	}
	return "custom"
}

// extractTimestamp 从日志行中提取时间戳。
// 支持 ISO 8601 (2024-01-15T10:30:45.123Z) 和常见格式 (2024-01-15 10:30:45.123)。
var (
	tsISO  = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z?`)
	tsLong = regexp.MustCompile(`\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}(?:\.\d+)?`)
)

func extractTimestamp(line string) string {
	if m := tsISO.FindString(line); m != "" {
		return m
	}
	if m := tsLong.FindString(line); m != "" {
		return m
	}
	return ""
}

// extractService 从日志行尝试识别服务名。
var svcPatterns = regexp.MustCompile(`service[=:](\w+)|app[=:](\w+)|appName[=:](\w+)`)

func extractService(line string) string {
	m := svcPatterns.FindStringSubmatch(line)
	if m != nil {
		for _, s := range m[1:] {
			if s != "" {
				return s
			}
		}
	}
	return ""
}

// extractThread 从日志行尝试提取线程名。
var threadPattern = regexp.MustCompile(`\[([\w\-/]+)\]|thread[=:](\w+)`)

func extractThread(line string) string {
	m := threadPattern.FindStringSubmatch(line)
	if m != nil {
		for _, s := range m[1:] {
			if s != "" {
				return s
			}
		}
	}
	return ""
}
