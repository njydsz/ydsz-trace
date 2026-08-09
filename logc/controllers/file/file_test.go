package file

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeKey_ValidPlain(t *testing.T) {
	tests := []struct {
		key    string
		regex  bool
		expect bool
	}{
		{"simple", false, true},
		{"with-dash", false, true},
		{"with_underscore", false, true},
		{"MixedCase123", false, true},
		{"", false, false},
		{"with space", false, false},
		{"with!special", false, false},
		{"../../../etc/passwd", false, false},
	}

	for _, tt := range tests {
		_, ok := sanitizeKey(tt.key, tt.regex)
		if ok != tt.expect {
			t.Errorf("sanitizeKey(%q, %v) = %v, want %v", tt.key, tt.regex, ok, tt.expect)
		}
	}
}

func TestSanitizeKey_RegexMode(t *testing.T) {
	// 正则模式下允许正则元字符
	ok, valid := sanitizeKey(`\d{3}-\w+`, true)
	if !valid {
		t.Errorf("regex mode should accept regex chars, got valid=%v", valid)
	}
	_ = ok

	// 正则模式下仍拒绝空字符串
	_, valid = sanitizeKey("", true)
	if valid {
		t.Error("regex mode should reject empty key")
	}

	// 正则模式下仍拒绝过长输入
	longKey := make([]byte, 300)
	for i := range longKey {
		longKey[i] = 'a'
	}
	_, valid = sanitizeKey(string(longKey), true)
	if valid {
		t.Error("regex mode should reject key > 256 chars")
	}
}

func TestReadConfig_Defaults(t *testing.T) {
	cfg := ReadConfig{
		Filename: "/var/log/test.log",
		Key:      "error",
		Line:     10,
		TempPath: "/tmp",
	}
	if cfg.Regex {
		t.Error("Regex should default to false")
	}
	if cfg.Level != "" {
		t.Error("Level should default to empty")
	}
}

func TestReadString_SearchInFile(t *testing.T) {
	// 创建临时日志文件
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")
	content := `2024-01-02 10:00:00 INFO application started
2024-01-02 10:00:01 DEBUG initializing cache
2024-01-02 10:00:02 ERROR connection failed
2024-01-02 10:00:03 INFO retrying...
2024-01-02 10:00:04 DEBUG cache ready
`
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("写入日志文件失败: %v", err)
	}

	tempPath := filepath.Join(dir, "output")
	result := ReadString(ReadConfig{
		Filename: logFile,
		Key:      "ERROR",
		Line:     2,
		TempPath: tempPath,
	})

	// 结果应为 zip 文件
	if result == "" {
		t.Fatal("期望返回 zip 路径，但为空")
	}
	defer os.Remove(result)

	if !filepath.IsAbs(result) && !strings.HasPrefix(result, tempPath) && !strings.HasPrefix(result, "."+string(filepath.Separator)+tempPath) {
		t.Errorf("结果路径应在 tempPath 下: got %q", result)
	}
}
