// Package main MCP Server 入口。
//
// 实现 MCP (Model Context Protocol) 协议的 stdio 与 HTTP 两种传输模式，
// 将 ydsz-trace 的日志检索、模式统计、上下文回溯、链路追踪、NL2DSL、
// 语义检索六项能力暴露为标准 MCP 工具，供 IDE（Cursor/Claude Code）调用。
//
// 监听端口：2022（环境变量 YDSZ_MCP_PORT 可覆盖）。
// 后端 API：http://127.0.0.1:2021（环境变量 YDSZ_API_BASE 可覆盖）。
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// MCP 协议常量。
const (
	// MCPProtocolVersion MCP 协议版本。
	MCPProtocolVersion = "2024-11-05"
	// MCPServerName MCP 服务器名称。
	MCPServerName = "ydsz-trace-mcp"
	// MCPServerVersion MCP 服务器版本。
	MCPServerVersion = "2.0.0"

	// JSON-RPC 错误码。
	// CodeParseError JSON 解析错误。
	CodeParseError = -32700
	// CodeMethodNotFound 方法不存在。
	CodeMethodNotFound = -32601
	// CodeInvalidParams 参数非法。
	CodeInvalidParams = -32602
	// CodeInternalError 内部错误。
	CodeInternalError = -32603
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

// MCPTool MCP 工具定义。
//
// 与 PRD FR-1402 约定一致：Name / Description / InputSchema / Handler。
type MCPTool struct {
	// Name 工具名称。
	Name string
	// Description 工具描述（供 LLM 理解工具用途）。
	Description string
	// InputSchema JSON Schema 形式的输入参数定义。
	InputSchema map[string]interface{}
	// Handler 工具执行函数。
	Handler func(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// MCPServer MCP 服务器（JSON-RPC 2.0 over stdio / HTTP）。
type MCPServer struct {
	// client 后端 API HTTP 客户端。
	client *http.Client
	// baseURL 后端 logs 服务地址。
	baseURL string
	// tools 已注册工具列表。
	tools []MCPTool
}

// NewMCPServer 创建 MCP 服务器实例。
//
// baseURL 为后端 logs 服务地址（默认 http://127.0.0.1:2021）。
func NewMCPServer(baseURL string) *MCPServer {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:2021"
	}
	return &MCPServer{
		client: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		baseURL: strings.TrimRight(baseURL, "/"),
		tools:   []MCPTool{},
	}
}

// RegisterTools 注册工具集到服务器。
func (s *MCPServer) RegisterTools(tools []MCPTool) {
	s.tools = append(s.tools, tools...)
}

// FindTool 按名称查找工具定义。
func (s *MCPServer) FindTool(name string) *MCPTool {
	for i := range s.tools {
		if s.tools[i].Name == name {
			return &s.tools[i]
		}
	}
	return nil
}

// ListTools 返回所有已注册工具的列表（MCP tools/list 协议）。
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
func (s *MCPServer) CallTool(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	tool := s.FindTool(name)
	if tool == nil {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	if tool.Handler == nil {
		return nil, fmt.Errorf("tool %s has no handler", name)
	}
	return tool.Handler(ctx, args)
}

// main MCP Server 入口。
func main() {
	baseURL := getEnv("YDSZ_API_BASE", "http://127.0.0.1:2021")
	port := getEnv("YDSZ_MCP_PORT", "2022")
	server := NewMCPServer(baseURL)

	// 注册工具集。
	server.RegisterTools(BuildTools(server))

	log.Printf("[mcp] MCP Server 启动，后端 API: %s, 端口: %s", baseURL, port)

	// 根据运行环境选择传输模式：
	//   - MCP_FORCE_STDIO=1 强制 stdio
	//   - MCP_FORCE_HTTP=1 强制 HTTP
	//   - 默认：TTY 环境走 stdio，非 TTY 走 HTTP
	switch {
	case os.Getenv("MCP_FORCE_STDIO") != "":
		log.Println("[mcp] stdio 模式：通过 JSON-RPC 2.0 over stdio 通信")
		runStdioMode(server)
	case os.Getenv("MCP_FORCE_HTTP") != "":
		log.Println("[mcp] HTTP 模式（强制）：监听端口 " + port)
		runHTTPMode(server, port)
	case isFDATTY(os.Stdout.Fd()):
		log.Println("[mcp] stdio 模式：通过 JSON-RPC 2.0 over stdio 通信")
		runStdioMode(server)
	default:
		log.Println("[mcp] HTTP 模式：监听端口 " + port)
		runHTTPMode(server, port)
	}
}

// runStdioMode 通过 stdin/stdout 与 MCP 客户端通信（JSON-RPC 2.0）。
func runStdioMode(server *MCPServer) {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := serveStdio(ctx, server); err != nil {
			log.Printf("[mcp] stdio 服务错误: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[mcp] 收到退出信号，等待任务结束...")
	wg.Wait()
	log.Println("[mcp] MCP Server 已退出")
}

// runHTTPMode 启动 HTTP 传输模式，暴露 MCP JSON-RPC 接口。
//
// 端点：
//   - POST /mcp           JSON-RPC 2.0 单请求（initialize / tools/list / tools/call）
//   - GET  /mcp/tools     列出工具
//   - GET  /health        健康检查
func runHTTPMode(server *MCPServer, port string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", server.handleHTTPRPC)
	mux.HandleFunc("/mcp/tools", server.handleHTTPTools)
	mux.HandleFunc("/health", handleHealth)

	addr := net.JoinHostPort("", port)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[mcp] HTTP 服务启动失败: %v", err)
		}
	}()

	// 等待退出信号。
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[mcp] 收到退出信号，正在优雅关闭...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[mcp] HTTP 服务关闭错误: %v", err)
	}
	log.Println("[mcp] MCP Server 已退出")
}

// handleHTTPRPC HTTP 模式下的 JSON-RPC 2.0 端点。
func (s *MCPServer) handleHTTPRPC(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		writeRPCError(w, nil, CodeInvalidParams, "only POST is supported")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeRPCError(w, nil, CodeParseError, "read body failed: "+err.Error())
		return
	}

	var req MCPRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeRPCError(w, nil, CodeParseError, "JSON parse error: "+err.Error())
		return
	}

	resp := handleMCPRequest(r.Context(), s, req)
	_ = json.NewEncoder(w).Encode(resp)
}

// handleHTTPTools HTTP 模式下的工具列表端点。
func (s *MCPServer) handleHTTPTools(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"tools": s.ListTools()})
}

// handleHealth 健康检查端点。
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"name":   MCPServerName,
		"v":      MCPServerVersion,
	})
}

// writeRPCError 写入 JSON-RPC 错误响应。
func writeRPCError(w http.ResponseWriter, id interface{}, code int, msg string) {
	_ = json.NewEncoder(w).Encode(MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &MCPError{Code: code, Message: msg},
	})
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

		line, err := readLineWithCtx(ctx, reader)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if err == io.EOF {
				return nil
			}
			_ = encoder.Encode(MCPResponse{
				JSONRPC: "2.0",
				Error:   &MCPError{Code: CodeParseError, Message: "Parse error: " + err.Error()},
			})
			continue
		}

		if len(line) == 0 {
			continue
		}

		var req MCPRequest
		if err := json.Unmarshal(line, &req); err != nil {
			_ = encoder.Encode(MCPResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &MCPError{Code: CodeParseError, Message: "JSON parse error: " + err.Error()},
			})
			continue
		}

		resp := handleMCPRequest(ctx, server, req)
		_ = encoder.Encode(resp)
	}
}

// handleMCPRequest 处理单个 MCP JSON-RPC 请求。
//
// 支持方法：
//   - initialize   握手，返回协议版本、服务器信息与能力
//   - tools/list   列出全部工具
//   - tools/call   调用指定工具
func handleMCPRequest(ctx context.Context, server *MCPServer, req MCPRequest) MCPResponse {
	switch req.Method {
	case "initialize":
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"protocolVersion": MCPProtocolVersion,
				"serverInfo": map[string]interface{}{
					"name":    MCPServerName,
					"version": MCPServerVersion,
				},
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{
						"listChanged": false,
					},
				},
			},
		}

	case "notifications/initialized", "initialized":
		// 客户端完成握手后的通知，无需响应。
		return MCPResponse{JSONRPC: "2.0", ID: req.ID}

	case "ping":
		return MCPResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]interface{}{}}

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
			Error:   &MCPError{Code: CodeMethodNotFound, Message: "Method not found: " + req.Method},
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
			Error:   &MCPError{Code: CodeInvalidParams, Message: "Invalid params: " + err.Error()},
		}
	}
	if params.Arguments == nil {
		params.Arguments = map[string]interface{}{}
	}

	result, err := server.CallTool(ctx, params.Name, params.Arguments)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPError{Code: CodeInternalError, Message: err.Error()},
		}
	}

	// 工具返回值统一包装为 MCP content 结构。
	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"content": []ContentItem{{Type: "text", Text: toJSONText(result)}},
		},
	}
}

// toJSONText 将任意返回值序列化为 JSON 文本；失败时降级为 fmt.Sprint。
func toJSONText(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
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

// getEnv 获取环境变量，未设置时返回 fallback。
func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
