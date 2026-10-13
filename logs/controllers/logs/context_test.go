// Package logs 上下文回溯接口单元测试。
package logs

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TestContext_MissingItem 验证 itemId 非法时返回 400（校验阶段即被拦截）。
func TestContext_MissingItem(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// itemId=0 => 400
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/logs/context/0", nil)
	c.Params = gin.Params{{Key: "itemId", Value: "0"}}
	Context(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for itemId=0, got %d", w.Code)
	}

	// itemId="-1" => 400（负数）
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request, _ = http.NewRequest(http.MethodGet, "/logs/context/-1", nil)
	c2.Params = gin.Params{{Key: "itemId", Value: "-1"}}
	Context(c2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for itemId=-1, got %d", w2.Code)
	}

	// itemId=abc => 400（非数字）
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Request, _ = http.NewRequest(http.MethodGet, "/logs/context/abc", nil)
	c3.Params = gin.Params{{Key: "itemId", Value: "abc"}}
	Context(c3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid itemId, got %d", w3.Code)
	}
}

// TestContext_BeforeAfterExplicit 验证显式 before=5/after=8 读取正确数量。
func TestContext_BeforeAfterExplicit(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "explicit.log")
	lines := make([]string, 30)
	for i := 0; i < 30; i++ {
		lines[i] = fmt.Sprintf("2026-01-15 10:00:%02d INFO msg-%d", i, i)
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// 第 20 行作为参考
	refLine := lines[20]

	// before=5, after=8 => before 取到 lines[15..19], after 取到 lines[21..28]
	result, err := readContext(logPath, "", refLine, 5, 8)
	if err != nil {
		t.Fatalf("readContext: %v", err)
	}
	if len(result.BeforeLines) != 5 {
		t.Fatalf("expected 5 before lines, got %d", len(result.BeforeLines))
	}
	if len(result.AfterLines) != 8 {
		t.Fatalf("expected 8 after lines, got %d", len(result.AfterLines))
	}
	// 验证 before 行顺序：lines[15], lines[16], ..., lines[19]
	for i := 0; i < 5; i++ {
		if result.BeforeLines[i] != lines[15+i] {
			t.Errorf("beforeLines[%d] = %q, want %q", i, result.BeforeLines[i], lines[15+i])
		}
	}
	// 验证 after 行顺序：lines[21], lines[22], ..., lines[28]
	for i := 0; i < 8; i++ {
		if result.AfterLines[i] != lines[21+i] {
			t.Errorf("afterLines[%d] = %q, want %q", i, result.AfterLines[i], lines[21+i])
		}
	}
}

// TestContext_BeforeAfter 写入临时日志文件并验证上下文读取行数正确。
func TestContext_BeforeAfter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 建一个临时目录和日志文件
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	lines := make([]string, 50)
	for i := 0; i < 50; i++ {
		lines[i] = fmt.Sprintf("2026-01-15 10:00:%02d INFO msg-%d addr=192.168.1.%d", i%60, i, i%256)
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatalf("write temp log: %v", err)
	}

	// 使用第 25 行作为参考行
	refLine := lines[25] // "2026-01-15 10:00:25 INFO msg-25 addr=192.168.1.25"

	// 直接调用 readContext
	before, after := 3, 4
	result, err := readContext(logPath, "", refLine, before, after)
	if err != nil {
		t.Fatalf("readContext failed: %v", err)
	}

	// 命中前 3 行：lines[22], lines[23], lines[24]
	if len(result.BeforeLines) != 3 {
		t.Fatalf("expected 3 before lines, got %d: %v", len(result.BeforeLines), result.BeforeLines)
	}
	// 命中后 4 行：lines[26], lines[27], lines[28], lines[29]
	if len(result.AfterLines) != 4 {
		t.Fatalf("expected 4 after lines, got %d: %v", len(result.AfterLines), result.AfterLines)
	}
	// 参考行匹配
	if result.RefLine != refLine {
		t.Fatalf("expected refLine=%q, got %q", refLine, result.RefLine)
	}

	// 验证 before 行顺序
	if result.BeforeLines[0] != lines[22] || result.BeforeLines[1] != lines[23] || result.BeforeLines[2] != lines[24] {
		t.Errorf("before lines content mismatch: %v", result.BeforeLines)
	}
	// 验证 after 行顺序
	if result.AfterLines[0] != lines[26] || result.AfterLines[3] != lines[29] {
		t.Errorf("after lines content mismatch: %v", result.AfterLines)
	}
}

// TestContext_BoundaryBefore 验证文件头部边界不越界。
func TestContext_BoundaryBefore(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "boundary.log")
	lines := []string{
		"2026-01-15 10:00:00 INFO first line",
		"2026-01-15 10:00:01 INFO second line",
		"2026-01-15 10:00:02 INFO third line",
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// 取第 0 行作为参考，要求 before=3（实际只有 2 行在它前面）
	result, err := readContext(logPath, "", lines[0], 3, 0)
	if err != nil {
		t.Fatalf("readContext: %v", err)
	}
	if len(result.BeforeLines) != 0 {
		t.Fatalf("expected 0 before lines at file head, got %d", len(result.BeforeLines))
	}
	if len(result.AfterLines) != 0 {
		t.Fatalf("expected 0 after lines, got %d", len(result.AfterLines))
	}
}

// TestExtractDateFromRefTime 验证日期提取。
func TestExtractDateFromRefTime(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2026-01-01T10:00:00Z", "20260101"},
		{"2026-01-01 10:00:00", "20260101"},
		{"2026-12-31 23:59:59", "20261231"},
		{"", ""},
		{"abc", ""},
	}
	for _, tt := range tests {
		got := extractDateFromRefTime(tt.input)
		if got != tt.expected {
			t.Errorf("extractDateFromRefTime(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// TestNormalizeDate 验证日期规范化。
func TestNormalizeDate(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"20260101", "20260101"},
		{"2026-01-01", "20260101"},
		{"", ""},
		{"invalid", ""},
	}
	for _, tt := range tests {
		got := normalizeDate(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeDate(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// TestExtractTemplateFields 验证模板提取。
func TestExtractTemplateFields(t *testing.T) {
	line := "2026-01-15 INFO user 192.168.1.100 login from /api/v1/auth took 3000ms"
	tmpl, params := extractTemplateFields(line)
	if tmpl == "" {
		t.Fatal("expected non-empty template")
	}
	// 检查占位符存在
	if !strings.Contains(tmpl, "<*>") {
		t.Errorf("template should contain <*>: %s", tmpl)
	}
	// 参数提取出了 IP、路径、数字
	if len(params) == 0 {
		t.Error("expected non-empty params")
	}
}

// TestTokenBucket 验证令牌桶基本行为。
func TestTokenBucket(t *testing.T) {
	tb := newTokenBucket(10, 10, 100*time.Millisecond)
	defer tb.Stop()

	// 应该消耗 10
	got := tb.Allow(10)
	if got != 10 {
		t.Fatalf("expected consume 10, got %d", got)
	}
	// 不能再消耗
	got = tb.Allow(1)
	if got != 0 {
		t.Fatalf("expected consume 0, got %d", got)
	}
}
