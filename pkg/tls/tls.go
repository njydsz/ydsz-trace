// Package tls 提供 logc ↔ logs 双向 TLS（mTLS）证书加载与校验能力。
//
// 设计目标（PRD FR-1308）：Agent 与服务端之间支持 mTLS。
//
// 配置项（环境变量，前缀 YDSZ_）：
//
//	YDSZ_TLS_ENABLED        是否启用 TLS（true/false，默认 false）
//	YDSZ_TLS_CERT_FILE      本端证书（PEM）；服务端必填，客户端 mTLS 时必填
//	YDSZ_TLS_KEY_FILE       本端私钥（PEM）；服务端必填，客户端 mTLS 时必填
//	YDSZ_TLS_CA_FILE        CA 证书（PEM），用于校验对端证书
//	YDSZ_TLS_SERVER_NAME    期望的对端证书 CN/SAN（客户端校验服务端身份）
//
// 安全底线：
//  1. 启用 mTLS 时，客户端必须持有 CA 才能校验服务端；服务端必须持有 CA 才能校验客户端。
//  2. 证书链校验失败时拒绝握手，不降级为明文。
//  3. 最小 TLS 版本为 1.2。
package tls

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

const (
	// MinTLSVersion 最低 TLS 版本：TLS 1.2（禁用 SSLv3/TLS1.0/TLS1.1）。
	MinTLSVersion = tls.VersionTLS12

	// DefaultServerName 未显式配置时使用的服务名（用于证书 SAN 校验）。
	DefaultServerName = "ydsz-trace"
)

// Config TLS 相关配置。
type Config struct {
	// Enabled 是否启用 TLS/mTLS。
	Enabled bool
	// CertFile 本端证书文件路径（PEM）。服务端必填；客户端在 mTLS 模式下必填。
	CertFile string
	// KeyFile 本端私钥文件路径（PEM）。服务端必填；客户端在 mTLS 模式下必填。
	KeyFile string
	// CAFile CA 证书文件路径（PEM），用于校验对端证书。
	CAFile string
	// ServerName 期望的对端证书标识（客户端校验服务端身份）。
	ServerName string
}

// LoadFromEnv 从环境变量读取 TLS 配置。
//
// 仅 YDSZ_TLS_ENABLED=true 时 Enabled 为真；其余字段可为空。
func LoadFromEnv() Config {
	return Config{
		Enabled:    envBool("YDSZ_TLS_ENABLED", false),
		CertFile:   envStr("YDSZ_TLS_CERT_FILE", ""),
		KeyFile:    envStr("YDSZ_TLS_KEY_FILE", ""),
		CAFile:     envStr("YDSZ_TLS_CA_FILE", ""),
		ServerName: envStr("YDSZ_TLS_SERVER_NAME", DefaultServerName),
	}
}

// ValidateServer 校验服务端 TLS 配置是否自洽。
//
// 返回 nil 表示配置合法；否则返回具体错误，调用方应拒绝启动。
func (c Config) ValidateServer() error {
	if !c.Enabled {
		return nil
	}
	if c.CertFile == "" || c.KeyFile == "" {
		return fmt.Errorf("TLS 已启用：必须提供 YDSZ_TLS_CERT_FILE 与 YDSZ_TLS_KEY_FILE")
	}
	if c.CAFile == "" {
		return fmt.Errorf("TLS 已启用：服务端必须提供 YDSZ_TLS_CA_FILE 以校验客户端证书（mTLS）")
	}
	return nil
}

// ValidateClient 校验客户端 TLS 配置是否自洽。
//
// 启用 TLS 时，客户端必须提供 CA 才能校验服务端；启用 mTLS 时还需客户端证书。
func (c Config) ValidateClient(requireClientCert bool) error {
	if !c.Enabled {
		return nil
	}
	if c.CAFile == "" {
		return fmt.Errorf("TLS 已启用：客户端必须提供 YDSZ_TLS_CA_FILE 以校验服务端证书")
	}
	if requireClientCert && (c.CertFile == "" || c.KeyFile == "") {
		return fmt.Errorf("mTLS 已启用：客户端必须提供 YDSZ_TLS_CERT_FILE 与 YDSZ_TLS_KEY_FILE")
	}
	return nil
}

// ServerTLSConfig 构建服务端 *tls.Config。
//
// 启用 mTLS 时要求并校验客户端证书（ClientAuth = RequireAndVerifyClientCert）。
// 未启用 TLS 时返回 nil，调用方应使用明文 HTTP。
func (c Config) ServerTLSConfig() (*tls.Config, error) {
	if !c.Enabled {
		return nil, nil
	}

	cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("加载服务端证书失败: %w", err)
	}

	caPool, err := loadCAPool(c.CAFile)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		MinVersion:   MinTLSVersion,
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}, nil
}

// ClientTLSConfig 构建客户端 *tls.Config。
//
// 客户端必须校验服务端证书（通过 CAFile + ServerName）。
// 提供证书时还会向服务端出示客户端证书（mTLS）。
func (c Config) ClientTLSConfig() (*tls.Config, error) {
	if !c.Enabled {
		return nil, nil
	}

	caPool, err := loadCAPool(c.CAFile)
	if err != nil {
		return nil, err
	}

	tlsCfg := &tls.Config{
		MinVersion: MinTLSVersion,
		RootCAs:    caPool,
		ServerName: c.ServerName,
	}

	if c.CertFile != "" && c.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("加载客户端证书失败: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return tlsCfg, nil
}

// loadCAPool 从 PEM 文件加载 CA 证书池。
func loadCAPool(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 CA 证书失败(%s): %w", path, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		// 辅助诊断：解析首个 PEM 块判断文件是否为空或非 PEM。
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("CA 证书文件不含有效 PEM 数据(%s)", path)
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return nil, fmt.Errorf("CA 证书解析失败(%s): %w", path, err)
		}
		return nil, fmt.Errorf("CA 证书池追加失败(%s)", path)
	}
	return pool, nil
}

// envStr 读取环境变量，缺省时返回 def。
func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envBool 解析布尔环境变量，缺省或非法时返回 def。
func envBool(key string, def bool) bool {
	switch v := os.Getenv(key); v {
	case "1", "true", "TRUE", "True", "yes", "on":
		return true
	case "0", "false", "FALSE", "False", "no", "off":
		return false
	default:
		return def
	}
}
