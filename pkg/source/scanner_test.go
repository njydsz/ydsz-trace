package source

import (
	"strings"
	"testing"
)

func TestScanConfig_Defaults(t *testing.T) {
	cfg := ScanConfig{Key: "error"}
	if cfg.Regex {
		t.Error("Regex should default to false")
	}
	if cfg.ContextLines != 0 {
		t.Error("ContextLines should default to 0")
	}
	if cfg.Level != "" {
		t.Error("Level should default to empty")
	}
}

// TestLineFilter_KeywordMatch 验证普通关键字匹配 + 上下文窗口。
func TestLineFilter_KeywordMatch(t *testing.T) {
	f, err := newLineFilter(ScanConfig{Key: "error", ContextLines: 2})
	if err != nil {
		t.Fatalf("newLineFilter: %v", err)
	}

	input := strings.Join([]string{
		"line 1: hello",
		"line 2: an error occurred", // hit
		"line 3: context A",         // in window
		"line 4: context B",         // in window
		"line 5: out of window",
		"line 6: another error",     // new hit
		"line 7: ctx after 6",
	}, "\n") + "\n"

	var buf strings.Builder
	written, err := scanAndFilter(strings.NewReader(input), f, &buf)
	if err != nil {
		t.Fatalf("scanAndFilter: %v", err)
	}
	if written == 0 {
		t.Fatal("expected non-zero bytes written")
	}
	out := buf.String()
	// hit + context 共 5 行 (2,3,4,6,7)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines, got %d: %q", len(lines), out)
	}
}

// TestLineFilter_RegexMatch 验证正则匹配。
func TestLineFilter_RegexMatch(t *testing.T) {
	f, err := newLineFilter(ScanConfig{Key: `ERR\d+`, Regex: true})
	if err != nil {
		t.Fatalf("newLineFilter regex: %v", err)
	}
	input := "INFO hello\nERR404 not found\nWARN minor\nERR500 oops\n"
	var buf strings.Builder
	scanAndFilter(strings.NewReader(input), f, &buf)
	out := buf.String()
	if !strings.Contains(out, "ERR404") && !strings.Contains(out, "ERR500") {
		t.Errorf("应包含 ERR404/ERR500, got %q", out)
	}
	if strings.Contains(out, "WARN") {
		t.Errorf("不应包含 WARN, got %q", out)
	}
}

// TestLineFilter_LevelFilter 验证按级别过滤。
func TestLineFilter_LevelFilter(t *testing.T) {
	f, err := newLineFilter(ScanConfig{Key: "", Level: "ERROR"})
	if err != nil {
		t.Fatalf("newLineFilter: %v", err)
	}
	input := "INFO ok\nDEBUG detailed\nERROR boom\nWARN minor\n"
	var buf strings.Builder
	scanAndFilter(strings.NewReader(input), f, &buf)
	out := buf.String()
	if !strings.Contains(out, "ERROR boom") {
		t.Errorf("应包含 ERROR 行, got %q", out)
	}
	if strings.Contains(out, "INFO") || strings.Contains(out, "WARN") {
		t.Errorf("不应包含非 ERROR 行, got %q", out)
	}
}

// TestLineFilter_TimeRange 验证时间范围过滤。
func TestLineFilter_TimeRange(t *testing.T) {
	f, err := newLineFilter(ScanConfig{
		StartTime: "10:00",
		EndTime:   "10:02",
	})
	if err != nil {
		t.Fatalf("newLineFilter: %v", err)
	}
	input := "09:59 early\n10:00 on time\n10:01 still valid\n10:02 last ok\n10:03 late\n"
	var buf strings.Builder
	scanAndFilter(strings.NewReader(input), f, &buf)
	out := buf.String()
	if strings.Contains(out, "09:59") || strings.Contains(out, "10:03") {
		t.Errorf("应排除范围外的时间, got %q", out)
	}
	if !strings.Contains(out, "10:00") || !strings.Contains(out, "10:02") {
		t.Errorf("应包含范围内行, got %q", out)
	}
}

// TestNormalizeLevel 验证 normalizeLevel。
func TestNormalizeLevel(t *testing.T) {
	cases := map[string]string{
		"warning": "WARN",
		"WARN":    "WARN",
		"warn":    "WARN",
		"Error":   "ERROR",
		"DEBUG":   "DEBUG",
		"info":    "INFO",
		"FATAL":   "FATAL",
		"unknown": "UNKNOWN",
	}
	for in, want := range cases {
		if got := normalizeLevel(in); got != want {
			t.Errorf("normalizeLevel(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestParseTimeToSeconds 验证 parseTimeToSeconds。
func TestParseTimeToSeconds(t *testing.T) {
	cases := map[string]int{
		"":         -1,
		"00:00":    0,
		"01:00":    3600,
		"12:30":    45000,
		"12:30:45": 45045,
		"23:59:59": 86399,
		"24:00":    -1,   // invalid
		"12:60":    -1,   // invalid
		"12":       -1,   // malformed
		"abc":      -1,   // non-numeric
	}
	for in, want := range cases {
		if got := parseTimeToSeconds(in); got != want {
			t.Errorf("parseTimeToSeconds(%q) = %d, want %d", in, got, want)
		}
	}
}

// TestExtractTimeFromLogLine 验证时间戳提取。
func TestExtractTimeFromLogLine(t *testing.T) {
	cases := map[string]int{
		"12:30:45 some log":          12*3600 + 30*60 + 45,
		"12:30 log without seconds":  12*3600 + 30*60,
		"2006-01-02 15:04:05 event": 15*3600 + 4*60 + 5,
		"short":                      -1,
	}
	for in, want := range cases {
		if got := extractTimeFromLogLine(in); got != want {
			t.Errorf("extractTimeFromLogLine(%q) = %d, want %d", in, got, want)
		}
	}
}

// TestExtractLogLevel 验证级别提取（大小写不敏感）。
func TestExtractLogLevel(t *testing.T) {
	cases := map[string]string{
		"INFO msg":     "INFO",
		"WARN warning": "WARN",
		"ERROR x":      "ERROR",
		"fatal panic":  "FATAL",
		"DEBUG detail": "DEBUG",
		"no level":     "", // no match
	}
	for in, want := range cases {
		if got := extractLogLevel(in); got != want {
			t.Errorf("extractLogLevel(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestNewTailFilter 验证 TailFilter 匹配行为。
func TestNewTailFilter(t *testing.T) {
	// plain 模式
	tf, err := NewTailFilter(TailConfig{Key: "error"})
	if err != nil {
		t.Fatalf("NewTailFilter plain: %v", err)
	}
	if !tf.Match("an error occurred") {
		t.Error("plain match should pass")
	}
	if tf.Match("no problem here") {
		t.Error("plain match should not pass")
	}

	// regex 模式
	tf2, err := NewTailFilter(TailConfig{Key: `ERR\d+`, Regex: true})
	if err != nil {
		t.Fatalf("NewTailFilter regex: %v", err)
	}
	if !tf2.Match("ERR404 not found") {
		t.Error("regex should match ERR404")
	}
	if tf2.Match("debug info") {
		t.Error("regex should not match unrelated")
	}

	// 级别过滤
	tf3, err := NewTailFilter(TailConfig{Level: "ERROR"})
	if err != nil {
		t.Fatalf("NewTailFilter level: %v", err)
	}
	if !tf3.Match("2026-01-01 ERROR boom") {
		t.Error("should match ERROR level")
	}
	if tf3.Match("2026-01-01 WARN minor") {
		t.Error("should not match WARN when filter is ERROR")
	}
}

// TestNewTailFilter_BadRegex 验证正则错误时返回 error。
func TestNewTailFilter_BadRegex(t *testing.T) {
	_, err := NewTailFilter(TailConfig{Key: `[unclosed`, Regex: true})
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

// TestNewLineFilter_InvalidRegex 验证正则错误。
func TestNewLineFilter_InvalidRegex(t *testing.T) {
	_, err := newLineFilter(ScanConfig{Key: `[bad`, Regex: true})
	if err == nil {
		t.Error("expected error")
	}
}
