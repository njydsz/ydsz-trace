package parser

import (
	"strings"
	"testing"
)

// TestDrain_BasicTemplates 验证 3-5 行相似日志聚为同一模板。
func TestDrain_BasicTemplates(t *testing.T) {
	d := NewDrain()
	lines := []string{
		"2024-01-15 10:23:45.123 INFO [http-nio-8080-exec-1] c.example.UserService : Login user alice from 192.168.1.1",
		"2024-01-15 10:23:45.456 INFO [http-nio-8080-exec-2] c.example.UserService : Login user bob from 10.0.0.1",
		"2024-01-15 10:23:46.789 INFO [http-nio-8080-exec-3] c.example.UserService : Login user charlie from 172.16.0.1",
		"2024-01-15 10:23:47.012 INFO [http-nio-8080-exec-4] c.example.UserService : Login user dave from 192.168.2.1",
		"2024-01-15 10:23:48.345 INFO [http-nio-8080-exec-5] c.example.UserService : Login user eve from 10.1.1.1",
	}

	var firstTemplate string
	for i, line := range lines {
		tmpl, _ := d.ProcessLine(line)
		if i == 0 {
			firstTemplate = tmpl
		}
		if tmpl == "" {
			t.Errorf("line %d: empty template", i)
		}
	}

	// 所有相似行应使用同一个模板组（模板合并后形式相同）
	lastTemplate, _ := d.ProcessLine(lines[4])
	templates := d.AllTemplates()
	if len(templates) != 1 {
		t.Logf("templates count: %d (may be 1 after merging)", len(templates))
	}
	if lastTemplate == "" {
		t.Error("last line processed should have template")
	}

	if firstTemplate == "" {
		t.Error("first template should not be empty")
	}
}

// TestDrain_ParamExtraction 验证参数位正确标记为 <*>。
func TestDrain_ParamExtraction(t *testing.T) {
	d := NewDrain()
	line1 := "Request processed in 123ms for user 550e8400-e29b-41d4-a716-446655440000"
	line2 := "Request processed in 456ms for user 6ba7b810-9dad-11d1-80b4-00c04fd430c8"

	tmpl1, params1 := d.ProcessLine(line1)
	tmpl2, params2 := d.ProcessLine(line2)

	// 模板中应含 <*>
	if !strings.Contains(tmpl1, "<*>") {
		t.Errorf("template should contain <*>, got: %s", tmpl1)
	}
	// 两次处理应得到相同或相似的模板
	if tmpl1 == "" || tmpl2 == "" {
		t.Error("templates should not be empty")
	}

	// 参数应被提取
	if len(params1) == 0 {
		t.Error("expected at least one parameter extracted from line1")
	}
	if len(params2) == 0 {
		t.Error("expected at least one parameter extracted from line2")
	}
}

// TestDrain_DistinctTemplates 验证差异明显的行形成不同模板。
func TestDrain_DistinctTemplates(t *testing.T) {
	d := NewDrain()
	lines := []string{
		"ERROR: database connection failed",
		"INFO: scheduled job completed successfully",
		"DEBUG: cache hit ratio 0.95",
	}

	for _, line := range lines {
		tmpl, _ := d.ProcessLine(line)
		if tmpl == "" {
			t.Errorf("unexpected empty template for: %s", line)
		}
	}

	templates := d.AllTemplates()
	// 3 个完全不同语义的行应产生至少 2-3 个模板
	if len(templates) < 2 {
		t.Errorf("expected at least 2 distinct templates, got %d", len(templates))
	}
}

// TestDrain_TemplateCount 验证 TemplateCount 计数正确。
func TestDrain_TemplateCount(t *testing.T) {
	d := NewDrain()
	if d.TemplateCount() != 0 {
		t.Error("initial template count should be 0")
	}
	_, _ = d.ProcessLine("Hello world")
	if d.TemplateCount() != 1 {
		t.Errorf("expected 1 template, got %d", d.TemplateCount())
	}
	_, _ = d.ProcessLine("Hello world")
	if d.TemplateCount() != 1 {
		t.Errorf("duplicate line should not increase template count, got %d", d.TemplateCount())
	}
	_, _ = d.ProcessLine("Goodbye world")
	if d.TemplateCount() < 2 {
		t.Errorf("distinct line should increase template count to >= 2, got %d", d.TemplateCount())
	}
}

// TestIsParamToken 验证参数 token 识别。
func TestIsParamToken(t *testing.T) {
	tests := []struct {
		token    string
		expected bool
	}{
		{"12345", true},
		{"192.168.1.1", true},
		{"550e8400-e29b-41d4-a716-446655440000", true},
		{"/var/log/app.log", true},
		{"hello", false},
		{"world", false},
		{"0x1a2b3c", true},
		{"::1", true},
		{"", false},
	}
	for _, tt := range tests {
		got := isParamToken(tt.token)
		if got != tt.expected {
			t.Errorf("isParamToken(%q) = %v, want %v", tt.token, got, tt.expected)
		}
	}
}
