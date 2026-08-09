package source

import (
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractLogrotateSuffix(t *testing.T) {
	cases := []struct {
		path string
		want int
	}{
		{"app.log", 0},
		{"app.log.1", 1},
		{"app.log.2.gz", 2},
		{"app.log.10.bz2", 10},
		{"/var/log/foo.20260809.log.3.zst", 3},
		{"app.log.bak", 0},
		{"app.log.", 0},
		{"noext", 0},
	}
	for _, c := range cases {
		got := extractLogrotateSuffix(c.path)
		if got != c.want {
			t.Errorf("extractLogrotateSuffix(%q) = %d, want %d", c.path, got, c.want)
		}
	}
}

func TestSortLogrotateDesc(t *testing.T) {
	in := []string{"app.log.1", "app.log", "app.log.10.gz", "app.log.2"}
	sortLogrotateDesc(in)
	want := []string{"app.log.10.gz", "app.log.2", "app.log.1", "app.log"}
	for i := range want {
		if in[i] != want[i] {
			t.Fatalf("sortLogrotateDesc = %v, want %v", in, want)
		}
	}
}

func TestIsGlobPattern(t *testing.T) {
	cases := map[string]bool{
		"/var/log/app.log":       false,
		"/var/log/app*.log":      true,
		"/var/log/app?.log":      true,
		"/var/log/app[0-9].log":  true,
		"/var/log/{a,b}/app.log": true,
		"/var/log/app.log{,.1}":  true,
	}
	for p, want := range cases {
		if got := isGlobPattern(p); got != want {
			t.Errorf("isGlobPattern(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestOpenLogFile_Gzip(t *testing.T) {
	dir := t.TempDir()
	gzPath := filepath.Join(dir, "app.log.1.gz")
	var raw bytes.Buffer
	gw := gzip.NewWriter(&raw)
	_, _ = gw.Write([]byte("line-one\nlinetwo\n"))
	_ = gw.Close()
	if err := os.WriteFile(gzPath, raw.Bytes(), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	rc, err := openLogFile(gzPath)
	if err != nil {
		t.Fatalf("openLogFile: %v", err)
	}
	defer rc.Close()

	// 直接 rc.Read 测 gzip 解压
	out := make([]byte, 1024)
	n, _ := rc.Read(out)
	if got := string(out[:n]); !strings.Contains(got, "line-one") {
		t.Errorf("gzip read = %q, want contains 'line-one'", got)
	}
}

// TestFileSourceRead_Glob 验证 glob 模式下多文件按 logrotate 逆序聚合读取。
func TestFileSourceRead_Glob(t *testing.T) {
	dir := t.TempDir()
	// 创建 fixtures：app.log (最新) → app.log.1 → app.log.2.gz (最旧)
	contents := map[string]string{
		"app.log":      "newest line",
		"app.log.1":    "middle line",
		"app.log.2.gz": "oldest line",
	}
	for name, body := range contents {
		p := filepath.Join(dir, name)
		if strings.HasSuffix(name, ".gz") {
			var buf bytes.Buffer
			gw := gzip.NewWriter(&buf)
			_, _ = gw.Write([]byte(body + "\n"))
			_ = gw.Close()
			_ = os.WriteFile(p, buf.Bytes(), 0644)
		} else {
			_ = os.WriteFile(p, []byte(body+"\n"), 0644)
		}
	}

	fs := NewFileSource()
	var out bytes.Buffer
	// 故意使用不存在的精确路径触发 glob 分支
	glob := filepath.Join(dir, "app.log*")
	n, err := fs.Read(context.Background(), glob, ScanConfig{}, &out)
	if err != nil {
		t.Fatalf("Read glob: %v", err)
	}
	if n == 0 {
		t.Fatal("Read glob returned 0 bytes")
	}

	got := out.String()
	// 预期顺序：oldest → middle → newest
	idxOldest := strings.Index(got, "oldest line")
	idxMiddle := strings.Index(got, "middle line")
	idxNewest := strings.Index(got, "newest line")
	if idxOldest < 0 || idxMiddle < 0 || idxNewest < 0 {
		t.Fatalf("missing expected lines in output:\n%s", got)
	}
	if !(idxOldest < idxMiddle && idxMiddle < idxNewest) {
		t.Errorf("wrong order in output (want oldest→middle→newest):\n%s", got)
	}
}

// TestFileSourceRead_SingleFile 验证精确单文件路径仍走单文件逻辑。
func TestFileSourceRead_SingleFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "app.log")
	_ = os.WriteFile(p, []byte("single file content\n"), 0644)

	fs := NewFileSource()
	var out bytes.Buffer
	n, err := fs.Read(context.Background(), p, ScanConfig{}, &out)
	if err != nil {
		t.Fatalf("Read single: %v", err)
	}
	if n == 0 {
		t.Fatal("Read single returned 0 bytes")
	}
	if got := out.String(); !strings.Contains(got, "single file content") {
		t.Errorf("single file read = %q, want contains 'single file content'", got)
	}
}

// TestFileSourceRead_NoMatch 验证无文件可匹配时返回明确错误。
func TestFileSourceRead_NoMatch(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileSource()
	var out bytes.Buffer
	_, err := fs.Read(context.Background(), filepath.Join(dir, "nonexistent*.log"), ScanConfig{}, &out)
	if err == nil {
		t.Fatal("expected error when no files match glob")
	}
	if !strings.Contains(err.Error(), "无匹配日志文件") {
		t.Errorf("error = %v, want contain '无匹配日志文件'", err)
	}
}

// TestResolvePath_RejectGlob 验证 ResolvePath 拒绝 glob 路径。
func TestResolvePath_RejectGlob(t *testing.T) {
	fs := NewFileSource()
	_, err := fs.ResolvePath("/var/log/app*.log")
	if err == nil {
		t.Fatal("expected error when ResolvePath gets glob")
	}
}
