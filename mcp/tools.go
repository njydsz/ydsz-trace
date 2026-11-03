// Package main 子文件：MCP 工具集（PRD FR-1402）。
//
// 实现 6 个标准 MCP 工具，每个工具通过 HTTP 调用 logs 服务端 API：
//
//   - search_logs        代理 POST /api/v2/logs/search
//   - agg_stats          代理 GET  /api/v2/logs/dashboard/insight
//   - get_context        代理 GET  /api/v2/logs/context/:itemId
//   - get_trace          代理 POST /api/v2/logs/trace
//   - nl2dsl_parse       代理 POST /api/v2/logs/nl2dsl/parse
//   - semantic_search    代理 POST /api/v2/logs/semantic/search
//
// 约定：
//   - 后端默认 http://127.0.0.1:2021，可用 YDSZ_API_BASE 覆盖。
//   - 所有 HTTP 调用都注入调用方提供的 context，支持取消与超时。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// BuildTools 构建 MCP Server 的全部工具定义。
func BuildTools(s *MCPServer) []MCPTool {
	return []MCPTool{
		{
			Name:        "search_logs",
			Description: "按条件搜索 ydsz-trace 日志。支持关键词、布尔查询表达式、日志级别、时间范围过滤，返回分页命中行与模板聚类。",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"item":     map[string]interface{}{"type": "integer", "description": "日志项 ID（必填）"},
					"date":     map[string]interface{}{"type": "string", "description": "日志日期 YYYYMMDD（必填）"},
					"key":      map[string]interface{}{"type": "string", "description": "搜索关键词（与 query 至少提供一个）"},
					"query":    map[string]interface{}{"type": "string", "description": "布尔查询表达式（field:value AND/OR/NOT）"},
					"regex":    map[string]interface{}{"type": "boolean", "description": "是否正则匹配", "default": false},
					"level":    map[string]interface{}{"type": "string", "description": "日志级别过滤：ERROR/WARN/INFO/DEBUG/FATAL"},
					"startTime": map[string]interface{}{"type": "string", "description": "起始时间 HH:MM:SS"},
					"endTime":   map[string]interface{}{"type": "string", "description": "结束时间 HH:MM:SS"},
					"pageNo":   map[string]interface{}{"type": "integer", "description": "页码（从 1 开始）", "default": 1},
					"pageSize": map[string]interface{}{"type": "integer", "description": "每页条数（最大 200）", "default": 50},
				},
				"required": []string{"item", "date"},
			},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return callLogsAPI(ctx, s, http.MethodPost, "/api/v2/logs/search", params)
			},
		},
		{
			Name:        "agg_stats",
			Description: "获取日志项的模式统计聚合数据（模板趋势、异常事件时间线、服务健康度），用于洞察看板与基线分析。",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"timeRange": map[string]interface{}{"type": "string", "description": "时间范围（如 1h/24h/7d）", "default": "24h"},
					"itemId":    map[string]interface{}{"type": "integer", "description": "日志项 ID（0 表示全部）", "default": 0},
				},
			},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				query := buildQuery(params, "timeRange", "itemId")
				return callLogsAPI(ctx, s, http.MethodGet, "/api/v2/logs/dashboard/insight"+query, nil)
			},
		},
		{
			Name:        "get_context",
			Description: "上下文回溯：给定参考时间或参考行，在日志文件中定位命中行并向前后读取指定行数的上下文。",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"itemId": map[string]interface{}{"type": "integer", "description": "日志项 ID（必填）"},
					"refTime": map[string]interface{}{"type": "string", "description": "参考时间（ISO8601 / 'YYYY-MM-DD HH:MM:SS'）"},
					"refLine": map[string]interface{}{"type": "string", "description": "参考行原文（与 refTime 至少提供一个）"},
					"before":  map[string]interface{}{"type": "integer", "description": "命中前行数（最大 100）", "default": 10},
					"after":   map[string]interface{}{"type": "integer", "description": "命中后行数（最大 100）", "default": 10},
					"date":    map[string]interface{}{"type": "string", "description": "日志日期 YYYYMMDD（refTime 无法推断日期时必填）"},
				},
				"required": []string{"itemId"},
			},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				itemID, ok := params["itemId"].(float64)
				if !ok {
					return nil, fmt.Errorf("itemId 必填且为整数")
				}
				query := buildQuery(params, "refTime", "refLine", "before", "after", "date")
				path := fmt.Sprintf("/api/v2/logs/context/%d", int64(itemID))
				return callLogsAPI(ctx, s, http.MethodGet, path+query, nil)
			},
		},
		{
			Name:        "get_trace",
			Description: "链路追踪：按 TraceID 跨节点检索日志，返回按节点分组的调用链片段。",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"traceId":    map[string]interface{}{"type": "string", "description": "链路追踪 ID（必填）"},
					"date":       map[string]interface{}{"type": "string", "description": "日志日期 YYYYMMDD（必填）"},
					"item":       map[string]interface{}{"type": "integer", "description": "日志项 ID（必填）"},
					"client":     map[string]interface{}{"type": "integer", "description": "客户端 ID（0 表示全部客户端）"},
					"maxPerNode": map[string]interface{}{"type": "integer", "description": "单节点返回行上限（默认 5000）", "default": 5000},
				},
				"required": []string{"traceId", "date", "item"},
			},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return callLogsAPI(ctx, s, http.MethodPost, "/api/v2/logs/trace", params)
			},
		},
		{
			Name:        "nl2dsl_parse",
			Description: "自然语言转 DSL：将自然语言检索意图解析为结构化检索参数（级别、时间范围、关键词、DSL 条件）。",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{"type": "string", "description": "自然语言检索意图（必填）"},
				},
				"required": []string{"query"},
			},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return callLogsAPI(ctx, s, http.MethodPost, "/api/v2/logs/nl2dsl/parse", params)
			},
		},
		{
			Name:        "semantic_search",
			Description: "语义检索：将自然语言 query 向量化后与模板/样本向量做余弦相似度检索，召回最相似的结果。",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{"type": "string", "description": "自然语言检索内容（必填）"},
					"type":  map[string]interface{}{"type": "string", "description": "检索目标类型：template / sample", "default": "template"},
					"topK":  map[string]interface{}{"type": "integer", "description": "召回条数（最大 100）", "default": 10},
				},
				"required": []string{"query"},
			},
			Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return callLogsAPI(ctx, s, http.MethodPost, "/api/v2/logs/semantic/search", params)
			},
		},
	}
}

// callLogsAPI 通过 HTTP 调用 logs 服务端 API，并将响应解码为通用结构。
//
// 返回结构：{ code, message, data, traceId }（与 pkg/api.Response 对齐）。
func callLogsAPI(ctx context.Context, s *MCPServer, method, path string, body interface{}) (interface{}, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化请求失败: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	url := s.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// 标识 MCP 来源，便于服务端审计与排障。
	req.Header.Set("X-MCP-Source", MCPServerName)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 %s 失败: %w", path, err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			// 仅在调试时可见；关闭失败不影响结果。
			_ = cerr
		}
	}()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 %s 响应失败: %w", path, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s 返回非 200 状态码: %d, body: %s", path, resp.StatusCode, truncate(string(respBody), 512))
	}

	// 解码为通用响应结构；保留 data 为 json.RawMessage 避免二次转义。
	var result struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
		TraceID string          `json:"traceId"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		// 非标准响应（如纯文本）：原样返回文本。
		return map[string]interface{}{
			"path":   path,
			"status": resp.StatusCode,
			"body":   string(respBody),
		}, nil
	}

	// data 解析为 interface{}，便于 MCP content 序列化时保持结构。
	var data interface{}
	if len(result.Data) > 0 {
		if err := json.Unmarshal(result.Data, &data); err != nil {
			// data 字段解析失败时保留原始 JSON 文本。
			data = string(result.Data)
		}
	}

	return map[string]interface{}{
		"code":    result.Code,
		"message": result.Message,
		"data":    data,
		"traceId": result.TraceID,
	}, nil
}

// buildQuery 从 params 中提取指定键构造 URL query 串（跳过空值）。
func buildQuery(params map[string]interface{}, keys ...string) string {
	var sb strings.Builder
	for _, k := range keys {
		v, ok := params[k]
		if !ok || v == nil {
			continue
		}
		var sv string
		switch t := v.(type) {
		case string:
			sv = t
		case float64:
			sv = fmt.Sprintf("%d", int64(t))
		case int:
			sv = fmt.Sprintf("%d", t)
		case int64:
			sv = fmt.Sprintf("%d", t)
		case bool:
			if t {
				sv = "true"
			} else {
				sv = "false"
			}
		default:
			sv = fmt.Sprintf("%v", t)
		}
		if sv == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(sv)
	}
	if sb.Len() == 0 {
		return ""
	}
	return "?" + sb.String()
}

// truncate 截断字符串到 max 长度（按字节）。
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
