// Package mcp 实现 v2.0 MCP Server 入口。
//
// 启动 HTTP JSON-RPC 2.0 服务（MCP Streamable HTTP）。
// 工具命名 snake_case：query_logs、search_similar、get_patterns、start_diagnosis。
// 所有工具只读，不产生副作用；响应携带 evidence_ref 以支持追溯。
package mcp
