package routers

import (
	"ydsz-trace/logs/agentlink"
)

// agentLinkServer 全局长连接服务端实例（由 logs 主程序注入）。
//
// 使用包级变量而非 Context 注入，原因：
//   - WebSocket 升级请求在中间件链之前 hijack，无法走常规 handler 参数
//   - 全局单例与 logs 的单实例部署形态一致
var agentLinkServer *agentlink.Server

// SetAgentLinkServer 注入长连接服务端（P1）。
//
// 必须在 SetupRouter 之前调用，否则 /agent/link 返回 501。
func SetAgentLinkServer(srv *agentlink.Server) {
	agentLinkServer = srv
}

// GetAgentLinkServer 返回当前长连接服务端（可能为 nil）。
func GetAgentLinkServer() *agentlink.Server {
	return agentLinkServer
}
