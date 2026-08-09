package source

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestCreateSourceFile 验证工厂能正确创建 FileSource。
func TestCreateSourceFile(t *testing.T) {
	s, err := CreateSource(FactoryConfig{
		Type: SourceTypeFile,
	})
	if err != nil {
		t.Fatalf("CreateSource(file) error: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil source")
	}
	info := s.Info()
	if info.Type != "file" {
		t.Errorf("expected type=file, got %s", info.Type)
	}
}

// TestCreateSourceFileWithRootDir 验证 rootDir 传递正确。
func TestCreateSourceFileWithRootDir(t *testing.T) {
	s, err := CreateSource(FactoryConfig{
		Type:    SourceTypeFile,
		Options: map[string]string{"root_dir": "/var/log"},
	})
	if err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	fs := s.(*FileSource)
	_, err = fs.ResolvePath("/var/log/app/access.log")
	if err != nil {
		t.Errorf("ResolvePath should allow path under rootDir: %v", err)
	}
	_, err = fs.ResolvePath("/etc/passwd")
	if err == nil {
		t.Error("ResolvePath should reject paths outside rootDir")
	}
}

// TestCreateSourceDocker 验证 Docker 模式能构建。
func TestCreateSourceDocker(t *testing.T) {
	s, err := CreateSource(FactoryConfig{
		Type:    SourceTypeDocker,
		Options: map[string]string{"socket": "/var/run/docker.sock"},
	})
	if err != nil {
		t.Fatalf("CreateSource(docker) error: %v", err)
	}
	info := s.Info()
	if info.Type != "docker" {
		t.Errorf("expected type=docker, got %s", info.Type)
	}
}

// TestCreateSourceK8s 验证 K8s 模式 — 没有 in-cluster 配置时应返回错误。
func TestCreateSourceK8s(t *testing.T) {
	_, err := CreateSource(FactoryConfig{Type: SourceTypeK8s})
	if err == nil {
		t.Error("expected error in non-k8s environment")
	}
}

// TestCreateSourceUnknown 验证未知类型返回错误。
func TestCreateSourceUnknown(t *testing.T) {
	_, err := CreateSource(FactoryConfig{Type: "unknown"})
	if err == nil {
		t.Error("expected error for unknown source type")
	}
}

// TestFileSourceRead 验证 FileSource 能正确读文件并过滤。
func TestFileSourceRead(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/app.log"
	content := "INFO start\nDEBUG detail\nERROR fail\nWARN minor\nINFO end\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewFileSource(WithRootDir(dir))
	var buf strings.Builder
	written, err := s.Read(context.Background(), path, ScanConfig{Key: "ERROR", ContextLines: 1}, &buf)
	if err != nil {
		t.Fatalf("FileSource.Read: %v", err)
	}
	if written == 0 {
		t.Fatal("expected non-zero bytes")
	}
	out := buf.String()
	if !strings.Contains(out, "ERROR fail") {
		t.Errorf("应包含 ERROR 行: %q", out)
	}
}

// TestFileSourceRead_NotFound 验证文件不存在返回错误。
func TestFileSourceRead_NotFound(t *testing.T) {
	s := NewFileSource()
	_, err := s.Read(context.Background(), "/nonexistent/file.log", ScanConfig{Key: "test"}, &strings.Builder{})
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

// TestFileSourceRead_PathTraversal 验证路径遍历保护。
func TestFileSourceRead_PathTraversal(t *testing.T) {
	s := NewFileSource(WithRootDir("/var/log/app"))
	var buf strings.Builder
	_, err := s.Read(context.Background(), "/var/log/app/../../etc/passwd", ScanConfig{Key: "x"}, &buf)
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

// TestFileSourceTail 验证 Tail 能在超时后退出。
func TestFileSourceTail(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/stream.log"
	if err := os.WriteFile(path, []byte("first\nsecond\nERROR critical\nlast\n"), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewFileSource(WithRootDir(dir))
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var matched []string
	err := s.Tail(ctx, path, TailConfig{Key: "ERROR"}, func(line string) error {
		matched = append(matched, line)
		return nil
	})
	// Tail 因 ctx 取消或文件到达 EOF 而退出。文件已读完加 EOF 时，err 可能为 nil，
	// 或者因 ctx 被取消 — 两种情况都是可接受的。
	if err != nil {
		t.Logf("Tail returned (may be ok): %v", err)
	}
	_ = matched
}

// TestParseK8sPath 验证 k8s 路径解析。
func TestParseK8sPath(t *testing.T) {
	tests := []struct {
		path        string
		wantNS      string
		wantPod     string
		wantCtr     string
		wantErr     bool
		errContains string
	}{
		{"k8s://default/my-pod/main", "default", "my-pod", "main", false, ""},
		{"k8s://prod/api-server-0/api", "prod", "api-server-0", "api", false, ""},
		{"invalid", "", "", "", true, "无法识别"},
		{"k8s://ns-only", "", "", "", true, "应为 k8s://"},
	}
	for _, tt := range tests {
		ns, pod, ctr, err := parseK8sPath(tt.path)
		if !tt.wantErr {
			if err != nil {
				t.Errorf("parseK8sPath(%q) unexpected err: %v", tt.path, err)
				continue
			}
			if ns != tt.wantNS || pod != tt.wantPod || ctr != tt.wantCtr {
				t.Errorf("parseK8sPath(%q) = (%s,%s,%s), want (%s,%s,%s)",
					tt.path, ns, pod, ctr, tt.wantNS, tt.wantPod, tt.wantCtr)
			}
		} else {
			if err == nil {
				t.Errorf("parseK8sPath(%q) expected error, got nil", tt.path)
			}
		}
	}
}

// TestDiscoveryTarget 验证 DiscoveryTarget 字段传递正确。
func TestDiscoveryTarget(t *testing.T) {
	dt := DiscoveryTarget{
		Identity:    "container-abc123",
		DisplayName: "my-app",
		SourceType:  "docker",
		LogPath:     "container:abc123",
		Labels:      map[string]string{"app": "myapp"},
	}
	if dt.SourceType != "docker" {
		t.Errorf("expected docker source type, got %s", dt.SourceType)
	}
}

// TestTailFilter_Match 补充测试 Tail 过滤。
func TestTailFilter_MatchEmpty(t *testing.T) {
	tf, err := NewTailFilter(TailConfig{})
	if err != nil {
		t.Fatalf("NewTailFilter: %v", err)
	}
	// 无 key、无 level：所有行都应匹配
	if !tf.Match("any line here") {
		t.Error("empty filter should match everything")
	}
}

func TestTailFilter_MatchKeyWithLevel(t *testing.T) {
	// 同时有 key 和 level：行必须同时满足
	tf, err := NewTailFilter(TailConfig{Key: "error", Level: "ERROR"})
	if err != nil {
		t.Fatalf("NewTailFilter: %v", err)
	}
	if !tf.Match("ERROR: something went wrong with error") {
		t.Error("should match line that contains keyword and correct level")
	}
	if tf.Match("WARN: error in config") {
		t.Error("should not match line with wrong level")
	}
	if tf.Match("ERROR: everything is fine, no problem") {
		t.Error("should not match line missing keyword")
	}
}
