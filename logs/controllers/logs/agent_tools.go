// Package logs 子文件：根因分析 Agent 工具集（FR-902）。
//
// 4 个只读工具，封装现有能力：
//   - SearchLogsTool：日志检索（复用 queryOneLogc 管道）
//   - AggStatsTool：模式统计（复用 models.QueryPatternTrend + TopPatterns）
//   - GetContextTool：上下文回溯（复用 readContext）
//   - GetTraceTool：链路追踪（复用 trace 聚合逻辑）
//
// 所有工具返回 JSON 可序列化的结果，执行超时由 ReasoningAgent 统一控制
// （agentToolTimeout = 5s）。
package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/sanitize"
)

// AgentTool 根因分析只读工具接口。
type AgentTool interface {
	// Name 工具名（LLM 调用时使用）。
	Name() string
	// Description 工具描述（注入 prompt，供 LLM 理解工具用途）。
	Description() string
	// Execute 执行工具，返回 JSON 可序列化的结果。
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// ============================================================================
// SearchLogsTool — 封装现有日志检索
// ============================================================================

// SearchLogsTool 按关键词检索日志行（只读）。
type SearchLogsTool struct{}

// Name 返回工具名。
func (t *SearchLogsTool) Name() string { return "search_logs" }

// Description 返回工具描述。
func (t *SearchLogsTool) Description() string {
	return `按关键词检索日志行。参数：item(int,必填)、date(string,YYYYMMDD,必填)、
key(string,关键词)、level(string,级别)、startTime/endTime(string,可选)、
client(int64,默认0=全部节点)、maxLines(int,默认100,最大500)。
返回命中行列表与总数。`
}

// searchLogsResult 工具返回结构。
type searchLogsResult struct {
	Total int      `json:"total"`
	Lines []string `json:"lines"`
}

// Execute 执行日志检索。
func (t *SearchLogsTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	itemID := toInt64(params["item"])
	if itemID <= 0 {
		return nil, fmt.Errorf("item 为必填正整数")
	}
	date := toString(params["date"])
	if date == "" {
		return nil, fmt.Errorf("date 为必填（YYYYMMDD）")
	}
	if _, ok := sanitize.SafeDate(date); !ok {
		return nil, fmt.Errorf("date 格式非法，仅允许 YYYYMMDD")
	}
	key := toString(params["key"])
	level := toString(params["level"])
	startTime := toString(params["startTime"])
	endTime := toString(params["endTime"])
	clientID := toInt64(params["client"])
	maxLines := toInt(params["maxLines"], 100)
	if maxLines > 500 {
		maxLines = 500
	}

	item := models.ReadItem(itemID)
	if item.Id == 0 {
		return nil, fmt.Errorf("项目不存在: item=%d", itemID)
	}
	path := item.LogPath + item.LogPrefix + date + item.LogSuffix + ".log"

	// 构造检索请求（复用 queryOneLogc 管道）
	req := SearchReq{
		Client:    clientID,
		Item:      itemID,
		Date:      date,
		Key:       key,
		Level:     level,
		StartTime: startTime,
		EndTime:   endTime,
		Query:     toString(params["query"]),
		PageNo:    1,
		PageSize:  maxLines,
	}

	// 单节点或多节点并发
	var targets []targetSpec
	if clientID != 0 {
		cl := models.ReadClient(clientID)
		if cl.Id == 0 {
			return nil, fmt.Errorf("客户端不存在: client=%d", clientID)
		}
		targets = []targetSpec{{server: cl.Ip + ":" + cl.Port, label: cl.Ip + ":" + cl.Port}}
	} else {
		clients, err := models.QueryAllClient()
		if err != nil || len(clients) == 0 {
			return nil, fmt.Errorf("无可用客户端")
		}
		for _, cl := range clients {
			if cl.Id == 0 {
				continue
			}
			targets = append(targets, targetSpec{server: cl.Ip + ":" + cl.Port, label: cl.Ip + ":" + cl.Port})
		}
	}

	// 并发检索（带 ctx 取消）
	type nodeResult struct {
		lines []string
		err   error
	}
	resultCh := make(chan nodeResult, len(targets))
	for _, tgt := range targets {
		go func(server string) {
			lines, err := queryOneLogc(server, path, req, maxLines)
			resultCh <- nodeResult{lines: lines, err: err}
		}(tgt.server)
	}

	var all []string
	deadline := time.After(agentToolTimeout)
	for i := 0; i < len(targets); i++ {
		select {
		case r := <-resultCh:
			if r.err == nil {
				all = append(all, r.lines...)
			}
		case <-deadline:
			// 超时：返回已收集到的部分结果
			return searchLogsResult{Total: len(all), Lines: all}, nil
		case <-ctx.Done():
			return searchLogsResult{Total: len(all), Lines: all}, nil
		}
	}

	return searchLogsResult{Total: len(all), Lines: all}, nil
}

// ============================================================================
// AggStatsTool — 封装模式统计查询
// ============================================================================

// AggStatsTool 查询模式统计与 TopN 模板（只读）。
type AggStatsTool struct{}

// Name 返回工具名。
func (t *AggStatsTool) Name() string { return "agg_stats" }

// Description 返回工具描述。
func (t *AggStatsTool) Description() string {
	return `查询模式统计与 TopN 模板趋势。参数：item(int,必填)、timeRange(string,如1h/6h/24h/7d,默认1h)、
topN(int,默认10)、templateHash(string,可选,指定则查询该模板的趋势)。
返回 TopPatterns 列表；若指定 templateHash，额外返回其时间序列 trend。`
}

// aggStatsResult 工具返回结构。
type aggStatsResult struct {
	TopPatterns []models.PatternStat      `json:"topPatterns"`
	Trend       []models.PatternStat      `json:"trend,omitempty"`
	TimeRange   string                   `json:"timeRange"`
}

// Execute 执行模式统计查询。
func (t *AggStatsTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	itemID := toInt64(params["item"])
	if itemID <= 0 {
		return nil, fmt.Errorf("item 为必填正整数")
	}
	timeRange := toString(params["timeRange"])
	if timeRange == "" {
		timeRange = "1h"
	}
	topN := toInt(params["topN"], 10)
	templateHash := toString(params["templateHash"])

	// TopPatterns
	patterns, err := models.TopPatterns(itemID, timeRange, topN)
	if err != nil {
		return nil, fmt.Errorf("查询 TopPatterns 失败: %w", err)
	}

	result := aggStatsResult{
		TopPatterns: patterns,
		TimeRange:   timeRange,
	}

	// 若指定 templateHash，查询其趋势
	if templateHash != "" {
		bucketStart := timeRangeToBucketForAgent(timeRange)
		bucketEnd := time.Now().Format("200601021504")
		trend, terr := models.QueryPatternTrend(templateHash, bucketStart, bucketEnd)
		if terr == nil {
			result.Trend = trend
		}
	}

	return result, nil
}

// ============================================================================
// GetContextTool — 封装上下文回溯
// ============================================================================

// GetContextTool 按参考行/时间读取上下文（只读）。
type GetContextTool struct{}

// Name 返回工具名。
func (t *GetContextTool) Name() string { return "get_context" }

// Description 返回工具描述。
func (t *GetContextTool) Description() string {
	return `按参考行或时间戳读取日志上下文。参数：item(int,必填)、date(string,YYYYMMDD,必填)、
refTime(string,可选,ISO8601)、refLine(string,可选,原文片段)、before(int,默认10,最大100)、
after(int,默认10,最大100)。refTime 与 refLine 至少提供一个。
返回命中前行、命中行、命中后行。`
}

// Execute 执行上下文回溯。
func (t *GetContextTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	itemID := toInt64(params["item"])
	if itemID <= 0 {
		return nil, fmt.Errorf("item 为必填正整数")
	}
	date := toString(params["date"])
	if date == "" {
		return nil, fmt.Errorf("date 为必填（YYYYMMDD）")
	}
	refTime := toString(params["refTime"])
	refLine := toString(params["refLine"])
	if refTime == "" && refLine == "" {
		return nil, fmt.Errorf("refTime 与 refLine 至少提供一个")
	}
	before := toInt(params["before"], contextDefaultLines)
	if before > contextMaxLines {
		before = contextMaxLines
	}
	after := toInt(params["after"], contextDefaultLines)
	if after > contextMaxLines {
		after = contextMaxLines
	}

	item := models.ReadItem(itemID)
	if item.Id == 0 {
		return nil, fmt.Errorf("项目不存在: item=%d", itemID)
	}
	logPath := item.LogPath + item.LogPrefix + date + item.LogSuffix + ".log"

	result, err := readContext(logPath, refTime, refLine, before, after)
	if err != nil {
		return nil, fmt.Errorf("读取上下文失败: %w", err)
	}
	return result, nil
}

// ============================================================================
// GetTraceTool — 封装链路追踪
// ============================================================================

// GetTraceTool 按 traceId 跨节点聚合调用链（只读）。
type GetTraceTool struct{}

// Name 返回工具名。
func (t *GetTraceTool) Name() string { return "get_trace" }

// Description 返回工具描述。
func (t *GetTraceTool) Description() string {
	return `按 traceId 跨节点检索调用链。参数：traceId(string,必填)、item(int,必填)、
date(string,YYYYMMDD,必填)、client(int64,默认0=全部节点)、maxPerNode(int,默认500)。
返回各节点命中行数与明细。`
}

// Execute 执行链路追踪检索。
func (t *GetTraceTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	traceID := strings.TrimSpace(toString(params["traceId"]))
	if traceID == "" {
		return nil, fmt.Errorf("traceId 为必填")
	}
	itemID := toInt64(params["item"])
	if itemID <= 0 {
		return nil, fmt.Errorf("item 为必填正整数")
	}
	date := toString(params["date"])
	if date == "" {
		return nil, fmt.Errorf("date 为必填（YYYYMMDD）")
	}
	clientID := toInt64(params["client"])
	maxPerNode := toInt(params["maxPerNode"], 500)

	item := models.ReadItem(itemID)
	if item.Id == 0 {
		return nil, fmt.Errorf("项目不存在: item=%d", itemID)
	}
	path := item.LogPath + item.LogPrefix + date + item.LogSuffix + ".log"

	searchReq := SearchReq{
		Client:   clientID,
		Item:     itemID,
		Date:     date,
		Query:    "traceId:" + traceID,
		PageNo:   1,
		PageSize: maxPerNode,
	}

	var targets []targetSpec
	if clientID != 0 {
		cl := models.ReadClient(clientID)
		if cl.Id == 0 {
			return nil, fmt.Errorf("客户端不存在: client=%d", clientID)
		}
		targets = []targetSpec{{server: cl.Ip + ":" + cl.Port, label: cl.Ip + ":" + cl.Port}}
	} else {
		clients, err := models.QueryAllClient()
		if err != nil || len(clients) == 0 {
			return nil, fmt.Errorf("无可用客户端")
		}
		for _, cl := range clients {
			if cl.Id == 0 {
				continue
			}
			targets = append(targets, targetSpec{server: cl.Ip + ":" + cl.Port, label: cl.Ip + ":" + cl.Port})
		}
	}

	type nodeResult struct {
		node  string
		lines []string
		err   error
	}
	resultCh := make(chan nodeResult, len(targets))
	for _, tgt := range targets {
		go func(server, label string) {
			lines, err := queryTraceFromLogc(server, path, searchReq)
			resultCh <- nodeResult{node: label, lines: lines, err: err}
		}(tgt.server, tgt.label)
	}

	results := make([]traceNodeResult, 0, len(targets))
	total := 0
	deadline := time.After(agentToolTimeout)
	for i := 0; i < len(targets); i++ {
		select {
		case r := <-resultCh:
			if r.err == nil && len(r.lines) > 0 {
				results = append(results, traceNodeResult{
					Node:  r.node,
					Count: len(r.lines),
					Lines: r.lines,
				})
				total += len(r.lines)
			}
		case <-deadline:
			return traceResp{TraceID: traceID, Nodes: len(targets), Total: total, Results: results}, nil
		case <-ctx.Done():
			return traceResp{TraceID: traceID, Nodes: len(targets), Total: total, Results: results}, nil
		}
	}

	return traceResp{TraceID: traceID, Nodes: len(targets), Total: total, Results: results}, nil
}

// ============================================================================
// 参数解析辅助函数
// ============================================================================

// toString 从 interface{} 安全提取字符串。
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return fmt.Sprintf("%v", t)
	}
}

// toInt64 从 interface{} 安全提取 int64。
func toInt64(v interface{}) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case json.Number:
		n, _ := t.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(t, 10, 64)
		return n
	default:
		return 0
	}
}

// toInt 从 interface{} 安全提取 int，失败返回 def。
func toInt(v interface{}, def int) int {
	n := toInt64(v)
	if n == 0 {
		return def
	}
	return int(n)
}

// timeRangeToBucketForAgent 将时间范围转为 5min bucket 起始字符串（供 Agent 工具使用）。
func timeRangeToBucketForAgent(timeRange string) string {
	if timeRange == "" {
		timeRange = "1h"
	}
	d, err := parseDurationForAgent(timeRange)
	if err != nil {
		return ""
	}
	ts := time.Now().Add(-d)
	minute := (ts.Minute() / 5) * 5
	return fmt.Sprintf("%04d%02d%02d%02d%02d", ts.Year(), ts.Month(), ts.Day(), ts.Hour(), minute)
}

// parseDurationForAgent 解析人类友好时长（复用 models 层逻辑）。
func parseDurationForAgent(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err == nil {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	return 0, fmt.Errorf("非法时长: %s", s)
}
