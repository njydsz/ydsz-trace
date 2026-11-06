package parser

import (
	"strings"
	"testing"
)

// TestExtractTraceInfo_Decorated pkg/trace 自身格式。
func TestExtractTraceInfo_Decorated(t *testing.T) {
	line := `[trace:abc123def456abc123def456abc123d] ERROR something failed`
	info, ok := ExtractTraceInfo(line)
	if !ok {
		t.Fatal("应识别")
	}
	if info.Format != TraceFormatDecorated {
		t.Errorf("format = %q", info.Format)
	}
	if info.TraceID != "abc123def456abc123def456abc123d" {
		t.Errorf("traceId = %q", info.TraceID)
	}
}

// TestExtractTraceInfo_DecoratedWithSpan 带 span 的装饰格式。
func TestExtractTraceInfo_DecoratedWithSpan(t *testing.T) {
	line := `[trace:abc123def456abc123def456abc123d span:00f067aa0ba902b7] INFO ok`
	info, ok := ExtractTraceInfo(line)
	if !ok {
		t.Fatal("应识别")
	}
	if info.SpanID != "00f067aa0ba902b7" {
		t.Errorf("spanId = %q", info.SpanID)
	}
}

// TestExtractTraceInfo_KV 键值对格式。
func TestExtractTraceInfo_KV(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{"2026-11-06 10:00:00 INFO traceId=abc123def456abc123def456abc123d msg", "abc123def456abc123def456abc123d"},
		{"2026-11-06 10:00:00 INFO trace_id: abc123def456abc123def456abc123d msg", "abc123def456abc123def456abc123d"},
		{`{"traceId":"abc123def456abc123def456abc123d"}`, "abc123def456abc123def456abc123d"},
	}

	for _, c := range cases {
		info, ok := ExtractTraceInfo(c.line)
		if !ok {
			t.Fatalf("未识别: %s", c.line)
		}
		if info.Format != TraceFormatKV {
			t.Errorf("format = %q want kv", info.Format)
		}
		if info.TraceID != c.want {
			t.Errorf("traceId = %q want %q", info.TraceID, c.want)
		}
	}
}

// TestExtractTraceInfo_W3C W3C traceparent。
func TestExtractTraceInfo_W3C(t *testing.T) {
	line := "traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	info, ok := ExtractTraceInfo(line)
	if !ok {
		t.Fatal("应识别")
	}
	if info.Format != TraceFormatW3C {
		t.Errorf("format = %q", info.Format)
	}
	if info.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("traceId = %q", info.TraceID)
	}
	if info.SpanID != "00f067aa0ba902b7" {
		t.Errorf("spanId = %q", info.SpanID)
	}
}

// TestExtractTraceInfo_MDC Zipkin MDC 格式。
func TestExtractTraceInfo_MDC(t *testing.T) {
	line := "2026-11-06 [abc123def456abc123def456abc123d,00f067aa0ba902b7] request done"
	info, ok := ExtractTraceInfo(line)
	if !ok {
		t.Fatal("应识别")
	}
	if info.Format != TraceFormatMDC {
		t.Errorf("format = %q", info.Format)
	}
}

// TestExtractTraceInfo_NotFound 无追踪信息的行。
func TestExtractTraceInfo_NotFound(t *testing.T) {
	cases := []string{
		"2026-11-06 10:00:00 INFO normal log line",
		"",
		"short",
	}
	for _, line := range cases {
		if _, ok := ExtractTraceInfo(line); ok {
			t.Errorf("不应识别: %q", line)
		}
	}
}

// TestExtractTraceID 便捷函数。
func TestExtractTraceID(t *testing.T) {
	id, ok := ExtractTraceID("[trace:abc123def456abc123def456abc123d] x")
	if !ok || id == "" {
		t.Errorf("id = %q ok = %v", id, ok)
	}
}

// TestNormalizeTraceID 统一小写。
func TestNormalizeTraceID(t *testing.T) {
	if got := NormalizeTraceID(" ABC123 "); got != "abc123" {
		t.Errorf("got %q", got)
	}
}

// TestLooksLikeTraceID 宽松校验。
func TestLooksLikeTraceID(t *testing.T) {
	valid := []string{
		"abc123def456abc123def456abc123d",
		"ABCDEF0123456789",
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	for _, v := range valid {
		if !LooksLikeTraceID(v) {
			t.Errorf("应视为合法: %q", v)
		}
	}

	invalid := []string{
		"",
		"abc",
		"xyz1234567890abc",
		strings.Repeat("a", 65),
	}
	for _, v := range invalid {
		if LooksLikeTraceID(v) {
			t.Errorf("不应视为合法: %q", v)
		}
	}
}

// TestGuessTraceID 兜底识别。
func TestGuessTraceID(t *testing.T) {
	line := "request id abc123def456abc123def456abc123d failed"
	got, ok := GuessTraceID(line)
	if !ok {
		t.Fatal("应识别")
	}
	if !strings.Contains(got, "abc123") {
		t.Errorf("got %q", got)
	}
}

// TestHasTraceInfo 存在性判断。
func TestHasTraceInfo(t *testing.T) {
	if !HasTraceInfo("[trace:abc123def456abc123def456abc123d] x") {
		t.Error("应返回 true")
	}
	if HasTraceInfo("no trace here") {
		t.Error("应返回 false")
	}
}

// TestExtractTraceInfo_PriorityDecoratedFirst 多种格式并存时优先 decorated。
func TestExtractTraceInfo_PriorityDecoratedFirst(t *testing.T) {
	// 同时含 decorated 与 kv 格式
	line := `[trace:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa] traceId=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb`
	info, ok := ExtractTraceInfo(line)
	if !ok {
		t.Fatal("应识别")
	}
	if info.TraceID != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Errorf("应优先 decorated: %q", info.TraceID)
	}
}
