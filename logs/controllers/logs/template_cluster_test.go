// Package logs 模板聚类分组单元测试。
package logs

import (
	"strings"
	"testing"
)

// TestGroupByTemplate 验证 100 行输入能被正确分组。
func TestGroupByTemplate(t *testing.T) {
	lines := make([]logRow, 100)
	for i := 0; i < 100; i++ {
		// 模拟 3 种模板交替出现
		switch i % 3 {
		case 0:
			lines[i] = logRow{Line: "2024-01-15 INFO user 192.168.1.100 login success from /api/v1/auth"}
		case 1:
			lines[i] = logRow{Line: "2024-01-15 ERROR user 10.0.0.1 DB timeout after 3000ms"}
		case 2:
			lines[i] = logRow{Line: "2024-10-10 DEBUG processing uuid aabbccdd-1234-5678-abcd-ef0123456789 done"}
		}
	}

	groups := groupByTemplate(lines, 2000)
	if len(groups) == 0 {
		t.Fatal("expected non-empty groups, got empty")
	}
	if len(groups) > 30 {
		t.Fatalf("expected at most 30 groups, got %d", len(groups))
	}

	// 验证总行数
	totalCount := 0
	for _, g := range groups {
		totalCount += g.Count
	}
	if totalCount != 100 {
		t.Fatalf("expected total count 100, got %d", totalCount)
	}
}

// TestGroupByTemplate_SortByCount 验证分组按 count 降序排列。
func TestGroupByTemplate_SortByCount(t *testing.T) {
	lines := []logRow{
		{Line: "INFO user 10.0.0.1 login success from /api/v1/auth"},
		{Line: "INFO user 10.0.0.2 login success from /api/v1/auth"},
		{Line: "INFO user 10.0.0.3 login success from /api/v1/auth"},
		{Line: "INFO user 10.0.0.4 login success from /api/v1/auth"},
		{Line: "INFO user 10.0.0.5 login success from /api/v1/auth"},
		{Line: "ERROR DB timeout after 3000ms from /db/pool"},
		{Line: "ERROR DB timeout after 3000ms from /db/query"},
		{Line: "DEBUG processing task abc123 done"},
	}

	groups := groupByTemplate(lines, 2000)
	if len(groups) < 2 {
		t.Fatal("expected at least 2 groups")
	}

	// 第一组应该是出现最多的 INFO 日志（count=5）
	if groups[0].Count < groups[len(groups)-1].Count {
		t.Fatalf("expected first group count (%d) >= last group count (%d)", groups[0].Count, groups[len(groups)-1].Count)
	}

	// 验证降序
	for i := 1; i < len(groups); i++ {
		if groups[i-1].Count < groups[i].Count {
			t.Errorf("groups not sorted by count desc: index %d count=%d, index %d count=%d",
				i-1, groups[i-1].Count, i, groups[i].Count)
		}
	}
}

// TestNormalizeLine 验证数字/IPv4/UUID/路径全部替换为 <*>。
func TestNormalizeLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "数字替换",
			input:    "request took 123ms with size 4567 bytes",
			expected: "request took <*>ms with size <*> bytes",
		},
		{
			name:     "IPv4 替换",
			input:    "connection from 192.168.1.100 to 10.0.0.1 established",
			expected: "connection from <*> to <*> established",
		},
		{
			name:     "UUID 替换",
			input:    "trace aabbccdd-1234-5678-abcd-ef0123456789 found in 50ms",
			expected: "trace <*> found in <*>ms",
		},
		{
			name:     "路径替换",
			input:    "reading /var/log/app/server.log file ok",
			expected: "reading <*> file ok",
		},
		{
			name:     "组合替换",
			input:    "INFO 2024-01-15T10:30:45Z user 10.0.0.1 access /api/v1/users ok with 200ms",
			expected: "INFO <*>T<*>:<*>:<*>Z user <*> access <*> ok with <*>ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeLine(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeLine(%q)\n got: %q\nwant: %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestGroupByTemplate_EmptyInput 验证空输入返回 nil。
func TestGroupByTemplate_EmptyInput(t *testing.T) {
	groups := groupByTemplate(nil, 2000)
	if groups != nil {
		t.Fatalf("expected nil for nil input, got %v", groups)
	}

	groups = groupByTemplate([]logRow{}, 2000)
	if groups != nil {
		t.Fatalf("expected nil for empty input, got %v", groups)
	}
}

// TestGroupByTemplate_MaxLinesLimit 验证 maxLines 截断工作正常。
func TestGroupByTemplate_MaxLinesLimit(t *testing.T) {
	lines := make([]logRow, 100)
	for i := 0; i < 100; i++ {
		lines[i] = logRow{Line: strings.Repeat("x", i) + " unique"}
	}

	groups := groupByTemplate(lines, 50)
	totalCount := 0
	for _, g := range groups {
		totalCount += g.Count
	}
	if totalCount != 50 {
		t.Fatalf("expected total count limited to 50, got %d", totalCount)
	}
}

// TestExtractLevel 验证各级别提取。
func TestExtractLevel(t *testing.T) {
	tests := []struct {
		line     string
		expected string
	}{
		{"2024-01-15 ERROR something failed", "ERROR"},
		{"2024-01-15 WARN something slow", "WARN"},
		{"2024-01-15 INFO all good", "INFO"},
		{"2024-01-15 DEBUG trace detail", "DEBUG"},
		{"2024-01-15 FATAL crash", "FATAL"},
		{"no level here", ""},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := extractLevel(tt.line)
			if got != tt.expected {
				t.Errorf("extractLevel(%q) = %q, want %q", tt.line, got, tt.expected)
			}
		})
	}
}

// TestDetectTraceIDFormat 验证 TraceID 格式自动识别。
func TestDetectTraceIDFormat(t *testing.T) {
	tests := []struct {
		traceID  string
		expected string
	}{
		// B3: 32 hex chars
		{"463ac35c9f6413ad48485a3953bb6124", "b3"},
		// B3: 16 hex chars
		{"48485a3953bb6124", "b3"},
		// W3C: contains dashes
		{"00-463ac35c9f6413ad48485a3953bb6124-00f067aa0ba902b7-01", "w3c"},
		// SkyWalking: long
		{"463ac35c9f6413ad48485a3953bb6124.1234567890.1234567890", "custom"},
		// Custom: short non-hex
		{"my-trace-123", "w3c"},
		// Custom: random
		{"abc123", "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.traceID, func(t *testing.T) {
			got := detectTraceIDFormat(tt.traceID)
			if got != tt.expected {
				t.Errorf("detectTraceIDFormat(%q) = %q, want %q", tt.traceID, got, tt.expected)
			}
		})
	}
}

// TestExtractTimestamp 验证时间戳提取。
func TestExtractTimestamp(t *testing.T) {
	tests := []struct {
		line     string
		expected string
	}{
		{"2024-01-15T10:30:45.123Z INFO request ok", "2024-01-15T10:30:45.123Z"},
		{"2024-01-15 10:30:45.123 ERROR fail", "2024-01-15 10:30:45.123"},
		{"2024-01-15 10:30:45 INFO no ms", "2024-01-15 10:30:45"},
		{"no timestamp here", ""},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := extractTimestamp(tt.line)
			if got != tt.expected {
				t.Errorf("extractTimestamp(%q) = %q, want %q", tt.line, got, tt.expected)
			}
		})
	}
}
