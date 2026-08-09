package file

import (
	"os"
	"path/filepath"
	"testing"

	"ydsz-trace/pkg/source"
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
	ok, valid := sanitizeKey(`\d{3}-\w+`, true)
	if !valid {
		t.Errorf("regex mode should accept regex chars, got valid=%v", valid)
	}
	_ = ok

	_, valid = sanitizeKey("", true)
	if valid {
		t.Error("regex mode should reject empty key")
	}

	longKey := make([]byte, 300)
	for i := range longKey {
		longKey[i] = 'a'
	}
	_, valid = sanitizeKey(string(longKey), true)
	if valid {
		t.Error("regex mode should reject key > 256 chars")
	}
}

// TestFileSourceViaFactory 验证通过 Factory 创建的 FileSource 能正常读取日志。
func TestFileSourceViaFactory(t *testing.T) {
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

	s, err := source.CreateSource(source.FactoryConfig{
		Type:    source.SourceTypeFile,
		Options: map[string]string{"root_dir": dir},
	})
	if err != nil {
		t.Fatalf("创建 FileSource 失败: %v", err)
	}

	var buf []byte
	bufWriter := &bytesWriter{data: &buf}
	written, err := s.Read(t.Context(), logFile, source.ScanConfig{
		Key:          "ERROR",
		ContextLines: 2,
	}, bufWriter)
	if err != nil {
		t.Fatalf("Read 失败: %v", err)
	}
	if written == 0 {
		t.Fatal("期望写入非零字节")
	}
	if !contains(string(buf), "ERROR") {
		t.Errorf("输出应包含 ERROR，got: %s", string(buf))
	}
}

// bytesWriter 简易 io.Writer 实现，用于测试写入。
type bytesWriter struct {
	data *[]byte
}

func (w *bytesWriter) Write(p []byte) (int, error) {
	*w.data = append(*w.data, p...)
	return len(p), nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
