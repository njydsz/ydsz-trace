package agentlink

import (
	"net"
	"strings"

	models "ydsz-trace/logs/models"
)

// resolveClientByAddr 由连接地址 + logc 端口 + vkey 解析服务端预注册的客户端 ID。
//
// 返回 0 表示未预注册（拒绝接入，与 HTTP 注册的安全策略一致）。
//
// 地址格式可能是 "IP:Port"（RemoteAddr）或纯 IP；这里取 host 部分。
func resolveClientByAddr(addr, port, vkey string) int64 {
	ip := extractIP(addr)
	if ip == "" {
		return 0
	}
	if port == "" {
		port = "2020"
	}
	c := models.CheckClient(ip, port, vkey)
	return c.Id
}

// extractIP 从 RemoteAddr 提取 IP。
func extractIP(addr string) string {
	if addr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	// 纯 IP 形式。
	return strings.TrimSpace(addr)
}
