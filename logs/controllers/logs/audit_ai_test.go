package logs

import (
	"testing"
)

// TestBuildAIReasonDetail AI 推理审计详情字段完整。
func TestBuildAIReasonDetail(t *testing.T) {
	d := BuildAIReasonDetail("服务超时怎么办", "qwen2.5", 1200, 5, false, 3400)

	if d["model"] != "qwen2.5" {
		t.Errorf("model = %v", d["model"])
	}
	if d["tokens"].(int) != 1200 {
		t.Errorf("tokens = %v", d["tokens"])
	}
	if d["steps"].(int) != 5 {
		t.Errorf("steps = %v", d["steps"])
	}
	if d["degraded"].(bool) != false {
		t.Errorf("degraded = %v", d["degraded"])
	}
	if d["costMs"].(int64) != 3400 {
		t.Errorf("costMs = %v", d["costMs"])
	}
	query, _ := d["query"].(string)
	if query == "" {
		t.Error("query 不应为空")
	}
}

// TestBuildAIReasonDetail_TruncatedQuery 超长 query 被截断。
func TestBuildAIReasonDetail_TruncatedQuery(t *testing.T) {
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'a'
	}
	d := BuildAIReasonDetail(string(long), "", 0, 0, true, 0)

	query := d["query"].(string)
	if len(query) > 250 {
		t.Errorf("query 应被截断到 200+3, got %d", len(query))
	}
}

// TestBuildNl2DslDetail NL2DSL 审计详情。
func TestBuildNl2DslDetail(t *testing.T) {
	d := BuildNl2DslDetail("近3小时 ERROR", "rule", 0.8, 45)

	if d["strategy"] != "rule" {
		t.Errorf("strategy = %v", d["strategy"])
	}
	if d["confidence"].(float64) != 0.8 {
		t.Errorf("confidence = %v", d["confidence"])
	}
	if d["costMs"].(int64) != 45 {
		t.Errorf("costMs = %v", d["costMs"])
	}
}

// TestBuildReportDetail 简报审计详情。
func TestBuildReportDetail(t *testing.T) {
	d := BuildReportDetail("timeout 分析", "24h", "P1", 1200)

	if d["timeRange"] != "24h" {
		t.Errorf("timeRange = %v", d["timeRange"])
	}
	if d["severity"] != "P1" {
		t.Errorf("severity = %v", d["severity"])
	}
}

// TestAuditActionConstants 审计动作常量。
func TestAuditActionConstants(t *testing.T) {
	actions := map[string]string{
		"AI 推理":     AuditActionAIReason,
		"NL2DSL":    AuditActionAINl2Dsl,
		"简报":       AuditActionAIReport,
		"语义检索":    AuditActionAISemantic,
	}
	for name, val := range actions {
		if val == "" {
			t.Errorf("%s 动作常量不应为空", name)
		}
	}
}

// TestAtoiSafe 安全整数解析。
func TestAtoiSafe(t *testing.T) {
	if n, _ := atoiSafe("123"); n != 123 {
		t.Errorf("atoiSafe(123) = %d", n)
	}
	if n, _ := atoiSafe(""); n != 0 {
		t.Errorf("atoiSafe(empty) = %d", n)
	}
	if n, _ := atoiSafe("abc"); n != 0 {
		t.Errorf("atoiSafe(abc) = %d", n)
	}
	if n, _ := atoiSafe("-5"); n != 0 {
		t.Errorf("atoiSafe(-5) = %d", n)
	}
}

// TestAuditDetail_DegradedFlag 降级标记正确传递。
func TestAuditDetail_DegradedFlag(t *testing.T) {
	d := BuildAIReasonDetail("q", "model", 0, 0, true, 0)
	if !d["degraded"].(bool) {
		t.Error("degraded 应为 true")
	}
}
