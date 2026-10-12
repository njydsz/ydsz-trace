package parser

import (
	"strings"
	"sync"
	"testing"
)

// TestNew_ZeroState 验证初始状态 TemplateCount=0。
func TestNew_ZeroState(t *testing.T) {
	p := New()
	if p == nil {
		t.Fatal("New() returned nil")
	}
	if count := p.TemplateCount(); count != 0 {
		t.Errorf("expected TemplateCount=0, got %d", count)
	}
	tmpls := p.AllTemplates()
	if len(tmpls) != 0 {
		t.Errorf("expected 0 templates, got %d", len(tmpls))
	}
}

// TestParseLine_SimpleText 验证普通文本能产出模板。
func TestParseLine_SimpleText(t *testing.T) {
	p := New()
	line := "User alice logged in from 192.168.1.100"
	result := p.ParseLine(line)

	if result.Template == "" {
		t.Error("expected non-empty template")
	}
	if result.TemplateID == 0 {
		t.Error("expected non-zero TemplateID")
	}
	// 第二次解析相似行应匹配到同一模板
	line2 := "User bob logged in from 10.0.0.1"
	result2 := p.ParseLine(line2)
	if result2.TemplateID == 0 {
		t.Error("expected non-zero TemplateID for second line")
	}
	// 模板应含参数位
	if !strings.Contains(result.Template, "<*>") {
		t.Errorf("template should contain parameter placeholder <*>, got: %s", result.Template)
	}
}

// TestParseBatch_Concurrent 验证并发安全无竞态。
func TestParseBatch_Concurrent(t *testing.T) {
	p := New()
	lines := []string{
		"2024-01-15 10:23:45.123 INFO [http-nio-8080-exec-1] c.example.UserService : Login user alice from 192.168.1.1",
		"2024-01-15 10:23:45.456 INFO [http-nio-8080-exec-2] c.example.UserService : Login user bob from 10.0.0.1",
		"2024-01-15 10:23:46.789 INFO [http-nio-8080-exec-3] c.example.UserService : Login user charlie from 172.16.0.1",
		"2024-01-15 10:23:47.012 INFO [http-nio-8080-exec-4] c.example.UserService : Login user dave from 192.168.2.1",
		"2024-01-15 10:23:48.345 INFO [http-nio-8080-exec-5] c.example.UserService : Login user eve from 10.1.1.1",
	}

	results := p.ParseBatch(lines, 4)
	if len(results) != len(lines) {
		t.Fatalf("expected %d results, got %d", len(lines), len(results))
	}

	// 验证每条结果都有有效模板
	for i, r := range results {
		if r.Template == "" {
			t.Errorf("result[%d] has empty template", i)
		}
	}

	// 并发安全：多个 goroutine 同时调用 ParseLine
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, line := range lines {
				_ = p.ParseLine(line)
			}
		}()
	}
	wg.Wait()

	// 解析后应有模板
	if count := p.TemplateCount(); count == 0 {
		t.Error("expected at least one template after concurrent parsing")
	}
}

// TestGetTemplate 验证通过行查找已知模板。
func TestGetTemplate(t *testing.T) {
	p := New()
	line := "Connection established from 192.168.1.100:8080"
	_ = p.ParseLine(line)

	tmpl, found := p.GetTemplate(line)
	if !found {
		t.Error("expected to find template for processed line")
	}
	if tmpl == nil {
		t.Fatal("GetTemplate returned nil template")
	}
	if tmpl.Template == "" {
		t.Error("template string should not be empty")
	}
	if tmpl.Count < 1 {
		t.Errorf("expected template count >= 1, got %d", tmpl.Count)
	}
}
