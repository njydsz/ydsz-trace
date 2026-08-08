package util

import (
	"net"
	"net/http"
	"time"
)

// defaultHTTPClient 全局共享的 HTTP 客户端，连接池复用。
//
// 默认 30 秒总超时；Transport 配置合理的空闲连接数与 KeepAlive，适合短请求 API 调用。
var defaultHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 60 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    false,
	},
}

// DefaultClient 返回全局共享的 HTTP 客户端。
//
// 适用于大多数普通短请求场景（默认 30 秒超时）。
func DefaultClient() *http.Client {
	return defaultHTTPClient
}

// NewClientWithTimeout 创建带自定义超时的独立 HTTP 客户端。
//
// 适用于耗时较长的场景（如日志查询，可能需要 120 秒以上）。
func NewClientWithTimeout(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 60 * time.Second,
		}).DialContext,
		MaxIdleConns:          50,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
