package util

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	pkgtls "ydsz-trace/pkg/tls"
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

// NewTLSClient 创建启用 mTLS 的 HTTP 客户端。
//
// 从环境变量读取 YDSZ_TLS_* 配置；未启用 TLS 时退化为普通客户端。
// requireClientCert 控制是否必须出示客户端证书。
//
// 典型用途：
//   - logc 向 logs 发起注册/查询：NewTLSClient(false)
//   - logs 向 logc 拉取日志：NewTLSClient(true)（若 logc 也启用 mTLS）
func NewTLSClient(requireClientCert bool) (*http.Client, error) {
	cfg := pkgtls.LoadFromEnv()
	if err := cfg.ValidateClient(requireClientCert); err != nil {
		return nil, err
	}

	tlsCfg, err := cfg.ClientTLSConfig()
	if err != nil {
		return nil, err
	}
	if tlsCfg == nil {
		return NewClientWithTimeout(30 * time.Second), nil
	}

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
		TLSClientConfig:       tlsCfg,
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}, nil
}

// NewTLSClientWithTimeout 创建启用 mTLS 且带自定义超时的 HTTP 客户端。
//
// 适用于日志查询等耗时较长的场景。
func NewTLSClientWithTimeout(timeout time.Duration, requireClientCert bool) (*http.Client, error) {
	client, err := NewTLSClient(requireClientCert)
	if err != nil {
		return nil, err
	}
	client.Timeout = timeout
	return client, nil
}

// TLSVersionName 返回 TLS 版本号的可读名称，用于启动日志。
func TLSVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS1.0"
	case tls.VersionTLS11:
		return "TLS1.1"
	case tls.VersionTLS12:
		return "TLS1.2"
	case tls.VersionTLS13:
		return "TLS1.3"
	default:
		return "未知"
	}
}
