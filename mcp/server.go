package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// MCPRequest MCP JSON-RPC 2.0 请求结构。
type MCPRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// MCPResponse MCP JSON-RPC 2.0 响应结构。
type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

// MCPError MCP 错误结构。
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ContentItem MCP 内容项（文本结果）。
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MCPServer MCP 服务器（基于 JSON-RPC 2.0 over stdio/HTTP bridge）。
type MCPServer struct {
	client  *http.Client
	baseURL string
	tools   []ToolDefinition
}

// NewMCPServer 创建 MCP 服务器实例。
func NewMCPServer(baseURL string) *MCPServer {
	return &MCPServer{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     30 * time.Second,
				DisableCompression:  false,
			},
		},
		baseURL: baseURL,
		tools:   []ToolDefinition{},
	}
}

// RegisterTools 注册工具集到服务器。
func (s *MCPServer) RegisterTool(t ToolDefinition) {
	s.tools = append(s.tools, t)
}

// FindTool 按名称查找工具定义。
func (s *MCPServer) FindTool(name string) *ToolDefinition {
	for i := range s.tools {
		if s.tools[i].Name == name {
			return &s.tools[i]
		}
	}
	return nil
}

// ListTools 返回所有已注册工具的列表（MCP list 协议）。
func (s *MCPServer) ListTools() []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(s.tools))
	for _, t := range s.tools {
		result = append(result, map[string]interface{}{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
		})
	}
	return result
}

// CallTool 调用指定工具并返回结果。
func (s *MCPServer) CallTool(ctx context.Context, name string, args map[string]interface{}) ([]ContentItem, error) {
	tool := s.FindTool(name)
	if tool == nil {
		return nil, fmt.Errorf("未知工具: %s", name)
	}

	if tool.Handler == nil {
		return nil, fmt.Errorf("工具 %s 未实现 handler", name)
	}

	return tool.Handler(ctx, args)
}

// serveStdio 从 stdin 读取 MCP JSON-RPC 请求，处理后将结果写入 stdout。
func serveStdio(ctx context.Context, server *MCPServer) error {
	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// 设置读取超时，支持优雅退出
		line, err := readLineWithCtx(ctx, reader)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if err == io.EOF {
				return nil
			}
			// 发送 parse 错误响应
			resp := MCPResponse{
				JSONRPC: "2.0",
				Error:   &MCPError{Code: -32700, Message: "Parse error: " + err.Error()},
			}
			_ = encoder.Encode(resp)
			continue
		}

		if len(line) == 0 {
			continue
		}

		var req MCPRequest
		if err := json.Unmarshal(line, &req); err != nil {
			resp := MCPResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &MCPError{Code: -32700, Message: "JSON parse error: " + err.Error()},
			}
			_ = encoder.Encode(resp)
			continue
		}

		resp := handleMCPRequest(ctx, server, req)
		_ = encoder.Encode(resp)
	}
}

// handleMCPRequest 处理单个 MCP JSON-RPC 请求。
func handleMCPRequest(ctx context.Context, server *MCPServer, req MCPRequest) MCPResponse {
	switch req.Method {
	case "initialize":
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"serverInfo": map[string]interface{}{
					"name":    "ydsz-trace-mcp",
					"version": "0.1.0",
				},
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{},
				},
			},
		}

	case "tools/list":
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"tools": server.ListTools(),
			},
		}

	case "tools/call":
		return handleToolCall(ctx, server, req)

	default:
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPError{Code: -32601, Message: "Method not found: " + req.Method},
		}
	}
}

// handleToolCall 解析 tools/call 请求并执行工具。
func handleToolCall(ctx context.Context, server *MCPServer, req MCPRequest) MCPResponse {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPError{Code: -32602, Message: "Invalid params: " + err.Error()},
		}
	}

	contents, err := server.CallTool(ctx, params.Name, params.Arguments)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPError{Code: -32603, Message: err.Error()},
		}
	}

	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"content": contents,
		},
	}
}

// handleListTools HTTP 模式：返回所有工具列表。
func handleListTools(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// 简单返回工具名列表（需要 globalServer 实例引用）
	if globalServer == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"tools": []interface{}{}})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"tools": globalServer.ListTools()})
}

// handleCallTool HTTP 模式：调用指定工具。
func (s *MCPServer) handleCallTool(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}

	contents, err := s.CallTool(r.Context(), req.Name, req.Arguments)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"content": contents})
}

// globalServer HTTP 模式所需的全局引用。
var globalServer *MCPServer

// isTTY 判断当前 fd 是否为 TTY（用于选择 stdio/HTTP 模式）。
func isTTY(fd uintptr) bool {
	// Windows: 通过系统调用判断；其他平台使用 syscall.Stat
	// 简化处理：检测环境变量 MCP_FORCE_HTTP
	if os.Getenv("MCP_FORCE_HTTP") != "" {
		return false
	}
	if os.Getenv("MCP_FORCE_STDIO") != "" {
		return true
	}
	// 默认：非 TTY 环境（如管道/重定向）走 HTTP，否则走 stdio
	return isFDATTY(fd)
}

// getEnv 获取环境变量，未设置时返回 fallback。
func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

// readLineWithCtx 带上下文的行读取。
func readLineWithCtx(ctx context.Context, reader *bufio.Reader) ([]byte, error) {
	type result struct {
		line []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := reader.ReadBytes('\n')
		ch <- result{line: line, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.line, r.err
	}
}

