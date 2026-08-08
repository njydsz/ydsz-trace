package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTempConfig 写入临时配置文件并返回路径（测试辅助）。
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.ini")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("写临时配置失败: %v", err)
	}
	return p
}

// TestLoad_Basic 验证基本键值解析和行尾注释去除。
func TestLoad_Basic(t *testing.T) {
	content := "host = 0.0.0.0\nport = 8080\nlevel = debug ; 行尾注释\n# 注释行\n"
	p := writeTempConfig(t, content)

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.String("host") != "0.0.0.0" {
		t.Fatalf("host 解析错误: got %q", cfg.String("host"))
	}
	if cfg.Int("port", 0) != 8080 {
		t.Fatalf("port 解析错误: got %d", cfg.Int("port", 0))
	}
	if cfg.String("level") != "debug" {
		t.Fatalf("行尾注释未去除: got %q", cfg.String("level"))
	}
}

// TestInt_InvalidFallsBackToDefault 验证非法整数回落默认值。
func TestInt_InvalidFallsBackToDefault(t *testing.T) {
	p := writeTempConfig(t, "port = notanumber\n")
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.Int("port", 9090) != 9090 {
		t.Fatalf("非法整数应回落默认: got %d", cfg.Int("port", 9090))
	}
}

// TestStringOr_Default 验证缺失 key 返回默认值。
func TestStringOr_Default(t *testing.T) {
	p := writeTempConfig(t, "")
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.StringOr("missing", "def") != "def" {
		t.Fatalf("缺失 key 应返回默认: got %q", cfg.StringOr("missing", "def"))
	}
}

// TestBool 验证布尔解析及默认值逻辑。
func TestBool(t *testing.T) {
	p := writeTempConfig(t, "enable = true\nflag = no\n")
	cfg, _ := Load(p)
	if !cfg.Bool("enable", false) {
		t.Fatalf("enable 应为 true")
	}
	// "no" 在 strconv.ParseBool 中解析失败，回落到默认 true
	if !cfg.Bool("flag", true) {
		t.Fatalf("flag=no 解析失败应回落默认 true")
	}
	if !cfg.Bool("missing", true) {
		t.Fatalf("缺失 key 应回落默认 true，实际 false")
	}
}

// TestLoad_MissingFile 验证文件不存在时返回错误。
func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.ini")); err == nil {
		t.Fatalf("文件不存在时应返回错误")
	}
}

// TestEnvOrConfig 验证环境变量优先，再到配置文件。
func TestEnvOrConfig(t *testing.T) {
	key := "YDSZ_TEST_ENVORC"
	os.Unsetenv(key)
	if got := EnvOrConfig(key, "cfg", "def"); got != "cfg" {
		t.Fatalf("无环境变量应使用配置值: got %q", got)
	}
	t.Setenv(key, "env")
	if got := EnvOrConfig(key, "cfg", "def"); got != "env" {
		t.Fatalf("环境变量应优先: got %q", got)
	}
}

// TestDSN 验证 DSN 拼接生成正确的连接串。
func TestDSN(t *testing.T) {
	cfg := NewDefault()
	dsn := cfg.DSN("127.0.0.1", "3306", "u", "p", "db", true)
	want := "u:p@tcp(127.0.0.1:3306)/db?charset=utf8&parseTime=true&loc=Local"
	if dsn != want {
		t.Fatalf("DSN 拼接错误:\n got %q\nwant %q", dsn, want)
	}
}
