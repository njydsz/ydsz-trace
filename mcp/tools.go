package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ToolHandler 工具执行函数签名。
type ToolHandler func(ctx context.Context, args map[string]interface{}) ([]ContentItem, error)

// ToolDefinition MCP 工具定义。
type ToolDefinition struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
	Handler     ToolHandler
}

// RegisterTools 注册 ydsz-trace 的全部工具集。
func RegisterTools(s *MCPServer) {
	globalServer = s

	// search_logs: 按条件搜索日志
	s.RegisterTool(ToolDefinition{
		Name:        "search_logs",
		Description: "按条件搜索 ydsz-trace 日志。支持关键词、正则、日志级别、时间范围过滤，返回匹配行与模板聚类。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"item":     map[string]interface{}{"type": "integer", "description": "日志项 ID（必填）"},
				"date":     map[string]interface{}{"type": "string", "description": "日志日期 YYYYMMDD（必填）"},
				"key":      map[string]interface{}{"type": "string", "description": "搜索关键词"},
				"query":    map[string]interface{}{"type": "string", "description": "布尔查询表达式（field:value AND/OR/NOT）"},
				"regex":    map[string]interface{}{"type": "boolean", "description": "是否正则匹配", "default": false},
				"level":    map[string]interface{}{"type": "string", "description": "日志级别过滤：ERROR/WARN/INFO/DEBUG/FATAL"},
				"pageNo":   map[string]interface{}{"type": "integer", "description": "页码（从 1 开始，默认 1）", "default": 1},
				"pageSize": map[string]interface{}{"type": "integer", "description": "每页条数（默认 50，最大 200）", "default": 50},
			},
			"required": []string{"item", "date"},
		},
		Handler: handleSearchLogs,
	})

	// tail_logs: 实时追踪日志
	s.RegisterTool(ToolDefinition{
		Name:        "tail_logs",
		Description: "实时追踪指定日志项的日志输出（代理 logc 的实时追踪能力）。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"client": map[string]interface{}{"type": "integer", "description": "客户端 ID（0 表示所有客户端）"},
				"item":   map[string]interface{}{"type": "integer", "description": "日志项 ID（必填）"},
				"date":   map[string]interface{}{"type": "string", "description": "日志日期 YYYYMMDD（必填）"},
				"key":    map[string]interface{}{"type": "string", "description": "追踪关键词"},
			},
			"required": []string{"item", "date"},
		},
		Handler: handleTailLogs,
	})

	// get_trace: 按 TraceID 获取调用链
	s.RegisterTool(ToolDefinition{
		Name:        "get_trace",
		Description: "按 TraceID 跨节点检索日志，返回按节点分组的调用链片段。支持常见 TraceID 格式自动识别（W3C/B3/SkyWalking/自定义）。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"traceId":    map[string]interface{}{"type": "string", "description": "链路追踪 ID（必填）"},
				"date":       map[string]interface{}{"type": "string", "description": "日志日期（必填）"},
				"item":       map[string]interface{}{"type": "integer", "description": "日志项 ID（必填）"},
				"client":     map[string]interface{}{"type": "integer", "description": "客户端 ID（0 表示全部客户端）"},
				"maxPerNode": map[string]interface{}{"type": "integer", "description": "单节点返回行上限（默认 5000）", "default": 5000},
			},
			"required": []string{"traceId", "date", "item"},
		},
		Handler: handleGetTrace,
	})

	// list_anomalies: 列出最近的异常事件
	s.RegisterTool(ToolDefinition{
		Name:        "list_anomalies",
		Description: "列出最近的异常事件（模式时序突变检测 + 告警规则触发历史）。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"item":     map[string]interface{}{"type": "integer", "description": "日志项 ID（可选，不填返回全部）"},
				"limit":    map[string]interface{}{"type": "integer", "description": "返回条数上限（默认 20）", "default": 20},
				"severity": map[string]interface{}{"type": "string", "description": "严重度过滤：warning/critical"},
			},
		},
		Handler: handleListAnomalies,
	})

	// get_stats: 获取模式统计基线
	s.RegisterTool(ToolDefinition{
		Name:        "get_stats",
		Description: "获取指定日志项的模式统计基线（模板命中时序数据，用于异常检测）。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"item":    map[string]interface{}{"type": "integer", "description": "日志项 ID（必填）"},
				"client":  map[string]interface{}{"type": "integer", "description": "客户端 ID（可选）"},
				"buckets": map[string]interface{}{"type": "integer", "description": "时间桶数量（默认 288 = 24h * 12/min）", "default": 288},
			},
			"required": []string{"item"},
		},
		Handler: handleGetStats,
	})
}

// ========== 工具 handler 实现（均通过 HTTP 调用 localhost:2021） ==========

// handleSearchLogs 调用 /logs/search 进行在线分页搜索。
func handleSearchLogs(ctx context.Context, args map[string]interface{}) ([]ContentItem, error) {
	body, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	url := globalServer.baseURL + "/logs/search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := globalServer.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != "200" {
		return []ContentItem{{Type: "text", Text: fmt.Sprintf("API error: %s", result.Message)}}, nil
	}

	// 格式化摘要返回
	var searchData struct {
		Total          int                      `json:"total"`
		PageNo         int                      `json:"pageNo"`
		PageSize       int                      `json:"pageSize"`
		Truncated      bool                     `json:"truncated"`
		TemplateGroups []map[string]interface{} `json:"templateGroups"`
	}
	_ = json.Unmarshal(result.Data, &searchData)

	summary := fmt.Sprintf("搜索命中 %d 条日志（第 %d 页，每页 %d 条）",
		searchData.Total, searchData.PageNo, searchData.PageSize)
	if searchData.Truncated {
		summary += " [已截断]"
	}

	return []ContentItem{
		{Type: "text", Text: summary + "\n\n原始数据:\n" + string(result.Data)},
	}, nil
}

// handleTailLogs 调用 /logs/tail 获取实时日志。
func handleTailLogs(ctx context.Context, args map[string]interface{}) ([]ContentItem, error) {
	body, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	url := globalServer.baseURL + "/logs/tail"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := globalServer.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return []ContentItem{{Type: "text", Text: string(respBody)}}, nil
}

// handleGetTrace 调用 /logs/trace 按 traceId 获取调用链。
func handleGetTrace(ctx context.Context, args map[string]interface{}) ([]ContentItem, error) {
	body, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	url := globalServer.baseURL + "/logs/trace"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := globalServer.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return []ContentItem{{Type: "text", Text: string(respBody)}}, nil
}

// handleListAnomalies 调用 /admin/stats/anomalies 获取异常事件列表。
func handleListAnomalies(ctx context.Context, args map[string]interface{}) ([]ContentItem, error) {
	query := ""
	if itemID, ok := args["item"].(float64); ok {
		query = fmt.Sprintf("?item=%d", int64(itemID))
	}

	url := globalServer.baseURL + "/admin/stats/anomalies" + query
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := globalServer.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return []ContentItem{{Type: "text", Text: string(respBody)}}, nil
}

// handleGetStats 调用 /admin/stats/baseline 获取模式统计基线。
func handleGetStats(ctx context.Context, args map[string]interface{}) ([]ContentItem, error) {
	query := ""
	if itemID, ok := args["item"].(float64); ok {
		query = fmt.Sprintf("?item=%d", int64(itemID))
	}

	url := globalServer.baseURL + "/admin/stats/baseline" + query
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := globalServer.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return []ContentItem{{Type: "text", Text: string(respBody)}}, nil
}
