package tls

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTempPEM 写入临时 PEM 文件并返回路径。
func writeTempPEM(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatalf("写入临时 PEM 失败: %v", err)
	}
	return p
}

// genTestCA 生成测试用自签名证书 PEM（CA + 服务端 + 客户端）。
//
// 注意：测试证书仅用于配置校验逻辑，不用于真实 TLS 握手。
func TestLoadFromEnv_Defaults(t *testing.T) {
	// 清理环境变量，确保默认行为。
	for _, k := range []string{"YDSZ_TLS_ENABLED", "YDSZ_TLS_CERT_FILE", "YDSZ_TLS_KEY_FILE", "YDSZ_TLS_CA_FILE", "YDSZ_TLS_SERVER_NAME"} {
		t.Setenv(k, "")
	}

	cfg := LoadFromEnv()
	if cfg.Enabled {
		t.Fatalf("默认应禁用 TLS")
	}
	if cfg.ServerName != DefaultServerName {
		t.Fatalf("默认 ServerName 应为 %q，实际 %q", DefaultServerName, cfg.ServerName)
	}
}

func TestValidateServer_EnabledMissingFiles(t *testing.T) {
	cfg := Config{Enabled: true}
	if err := cfg.ValidateServer(); err == nil {
		t.Fatalf("启用 TLS 但未提供证书应报错")
	}
}

func TestValidateServer_EnabledMissingCA(t *testing.T) {
	cfg := Config{Enabled: true, CertFile: "c.pem", KeyFile: "k.pem"}
	if err := cfg.ValidateServer(); err == nil {
		t.Fatalf("启用 TLS 但未提供 CA 应报错")
	}
}

func TestValidateServer_Valid(t *testing.T) {
	cfg := Config{Enabled: true, CertFile: "c.pem", KeyFile: "k.pem", CAFile: "ca.pem"}
	if err := cfg.ValidateServer(); err != nil {
		t.Fatalf("配置合法却报错: %v", err)
	}
}

func TestValidateClient_EnabledMissingCA(t *testing.T) {
	cfg := Config{Enabled: true}
	if err := cfg.ValidateClient(false); err == nil {
		t.Fatalf("客户端启用 TLS 但未提供 CA 应报错")
	}
}

func TestValidateClient_MutualRequiresCert(t *testing.T) {
	cfg := Config{Enabled: true, CAFile: "ca.pem"}
	if err := cfg.ValidateClient(true); err == nil {
		t.Fatalf("mTLS 模式下客户端未提供证书应报错")
	}
}

func TestServerTLSConfig_DisabledReturnsNil(t *testing.T) {
	cfg := Config{Enabled: false}
	c, err := cfg.ServerTLSConfig()
	if err != nil {
		t.Fatalf("未启用 TLS 不应报错: %v", err)
	}
	if c != nil {
		t.Fatalf("未启用 TLS 应返回 nil")
	}
}

func TestLoadCAPool_InvalidPEM(t *testing.T) {
	dir := t.TempDir()
	p := writeTempPEM(t, dir, "bad.pem", "not a pem")
	if _, err := loadCAPool(p); err == nil {
		t.Fatalf("非法 PEM 应报错")
	}
}

func TestLoadCAPool_MissingFile(t *testing.T) {
	if _, err := loadCAPool(filepath.Join(t.TempDir(), "nope.pem")); err == nil {
		t.Fatalf("文件不存在应报错")
	}
}

func TestEnvBool(t *testing.T) {
	cases := map[string]bool{
		"true": true, "1": true, "TRUE": true, "yes": true, "on": true,
		"false": false, "0": false, "no": false, "off": false,
		"": false, "maybe": false,
	}
	for in, want := range cases {
		t.Setenv("YDSZ_TEST_BOOL", in)
		if got := envBool("YDSZ_TEST_BOOL", false); got != want {
			t.Fatalf("envBool(%q) = %v, want %v", in, got, want)
		}
	}
}
