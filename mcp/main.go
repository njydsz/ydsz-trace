// Package main MCP Server 入口。
//
// 通过 stdio 与 IDE（Cursor/Claude Code）通信，暴露 ydsz-trace 检索、聚合、诊断能力。
// 当 stdout 不是 TTY（即开发/调试模式）时，退化为 HTTP 模式，在 9527 端口暴露相同接口。
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	baseURL := getEnv("YDSZ_API_BASE", "http://localhost:2021")
	server := NewMCPServer(baseURL)

	log.Printf("[mcp] MCP Server 启动，后端 API: %s", baseURL)

	// 注册工具集
	RegisterTools(server)

	// 根据运行环境选择模式
	if isTTY(os.Stdout.Fd()) {
		log.Println("[mcp] stdio 模式：通过 JSON-RPC 2.0 over stdio 通信")
		runStdioMode(server)
	} else {
		log.Println("[mcp] HTTP 模式：HTTP-to-MCP bridge 端口 9527")
		runHTTPMode(server)
	}
}

// runStdioMode 通过 stdin/stdout 与 MCP 客户端通信（JSON-RPC 2.0）。
func runStdioMode(server *MCPServer) {
	// 监听退出信号
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

// runHTTPMode 启动 HTTP-to-MCP bridge，在 9527 端口暴露工具调用接口。
// 开发模式下便于 Postman/curl 直接测试。
func runHTTPMode(server *MCPServer) {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp/list", handleListTools)
	mux.HandleFunc("/mcp/call", server.handleCallTool)

	port := getEnv("MCP_HTTP_PORT", "9527")
	addr := net.JoinHostPort("", port)
	httpSrv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[mcp] HTTP 服务启动失败: %v", err)
		}
	}()

	// 等待退出信号
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
