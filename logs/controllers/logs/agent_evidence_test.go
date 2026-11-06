package logs

import (
	"testing"
)

// newTestToolCall 构造测试用工具调用记录。
func newTestToolCall(step int, tool, summary string, success bool) ToolCallRecord {
	rec := ToolCallRecord{Step: step, Tool: tool, ResultSummary: summary, Success: success}
	if !success {
		rec.Error = "timeout"
	}
	return rec
}

// TestSourceForTool 工具名到证据来源的映射。
func TestSourceForTool(t *testing.T) {
	cases := map[string]evidenceSource{
		"search_logs":      EvidenceSourceLog,
		"get_context":      EvidenceSourceLog,
		"get_anomalies":    EvidenceSourceAnomaly,
		"list_templates":   EvidenceSourceTemplate,
		"get_trace":        EvidenceSourceTrace,
		"agg_stats":        EvidenceSourceStats,
		"diff_time_window": EvidenceSourceStats,
		"unknown_tool":     EvidenceSourceStats,
	}
	for tool, want := range cases {
		if got := sourceForTool(tool); got != want {
			t.Errorf("sourceForTool(%q) = %q want %q", tool, got, want)
		}
	}
}

// TestExtractEvidenceFromToolCall 从工具调用提取证据。
func TestExtractEvidenceFromToolCall(t *testing.T) {
	refs := extractEvidenceFromToolCall(newTestToolCall(1, "search_logs", "ERROR timeout at line 42", true))
	if len(refs) != 1 {
		t.Fatalf("len = %d want 1", len(refs))
	}
	if refs[0].Source != EvidenceSourceLog {
		t.Errorf("source = %q", refs[0].Source)
	}
	if refs[0].RefID != "step-1" {
		t.Errorf("refId = %q", refs[0].RefID)
	}
	if refs[0].Summary == "" || refs[0].Quote == "" {
		t.Error("summary/quote 不应为空")
	}
}

// TestExtractEvidenceFromToolCall_EmptySummary 空结果不产生证据。
func TestExtractEvidenceFromToolCall_EmptySummary(t *testing.T) {
	refs := extractEvidenceFromToolCall(newTestToolCall(1, "search_logs", "", true))
	if len(refs) != 0 {
		t.Errorf("len = %d want 0", len(refs))
	}
}

// TestCollectAllEvidence 汇总多条工具调用。
func TestCollectAllEvidence(t *testing.T) {
	result := &ReasoningResult{
		ToolCalls: []ToolCallRecord{
			newTestToolCall(1, "search_logs", "result one", true),
			newTestToolCall(2, "get_anomalies", "result two", true),
			newTestToolCall(3, "search_logs", "result three", false),
		},
	}

	evidence := collectAllEvidence(result)
	if len(evidence) != 3 {
		t.Fatalf("len = %d want 3", len(evidence))
	}
	if evidence[0].Source != EvidenceSourceLog {
		t.Errorf("first source = %q", evidence[0].Source)
	}
	if evidence[1].Source != EvidenceSourceAnomaly {
		t.Errorf("second source = %q", evidence[1].Source)
	}
}

// TestCollectAllEvidence_NilResult nil 安全。
func TestCollectAllEvidence_NilResult(t *testing.T) {
	if got := collectAllEvidence(nil); got != nil {
		t.Errorf("got %v want nil", got)
	}
}

// TestBuildThreeSectionReport 三段式报告字段完整。
func TestBuildThreeSectionReport(t *testing.T) {
	result := &ReasoningResult{
		ToolCalls: []ToolCallRecord{
			newTestToolCall(1, "get_anomalies", "异常突增", true),
			newTestToolCall(2, "search_logs", "ERROR 日志若干", true),
		},
		Conclusion: ConclusionSection{
			RootCause:  "数据库连接池耗尽导致超时",
			Suggestions: []string{"扩容连接池"},
			Confidence: 0.7,
		},
	}

	evidence := collectAllEvidence(result)
	report := buildThreeSectionReport(result, evidence)

	if report == nil {
		t.Fatal("report 不应为 nil")
	}
	if report.Hypothesis == "" {
		t.Error("hypothesis 为空")
	}
	if report.Verification == "" {
		t.Error("verification 为空")
	}
	if report.Conclusion != "数据库连接池耗尽导致超时" {
		t.Errorf("conclusion = %q", report.Conclusion)
	}
	if len(report.Suggestions) == 0 {
		t.Error("suggestions 为空")
	}
	// 假设段应优先引用异常/统计证据
	if len(report.HypothesisRefs) == 0 {
		t.Error("hypothesisRefs 应包含异常证据")
	}
}

// TestBuildThreeSectionReport_NoToolCalls 无工具调用时给出兜底文案。
func TestBuildThreeSectionReport_NoToolCalls(t *testing.T) {
	report := buildThreeSectionReport(&ReasoningResult{}, nil)
	if report.Hypothesis == "" {
		t.Error("hypothesis 应为兜底文案")
	}
	if report.Verification == "" {
		t.Error("verification 应为兜底文案")
	}
}

// TestBuildVerification 验证段包含证据统计。
func TestBuildVerification(t *testing.T) {
	result := &ReasoningResult{
		ToolCalls: []ToolCallRecord{
			newTestToolCall(1, "search_logs", "a", true),
			newTestToolCall(2, "get_anomalies", "b", true),
		},
	}
	evidence := []EvidenceRef{
		{Source: EvidenceSourceLog},
		{Source: EvidenceSourceAnomaly},
	}

	got := buildVerification(result, evidence)
	if !containsStr(got, "2") {
		t.Errorf("应包含证据数量, got %q", got)
	}
}

// TestPickRefs 按来源筛选并限量。
func TestPickRefs(t *testing.T) {
	evidence := []EvidenceRef{
		{Source: EvidenceSourceLog},
		{Source: EvidenceSourceLog},
		{Source: EvidenceSourceLog},
		{Source: EvidenceSourceLog},
		{Source: EvidenceSourceLog},
		{Source: EvidenceSourceLog},
		{Source: EvidenceSourceAnomaly},
	}

	got := pickRefs(evidence, EvidenceSourceLog)
	if len(got) != maxEvidencePerSection {
		t.Errorf("len = %d want %d", len(got), maxEvidencePerSection)
	}
	for _, r := range got {
		if r.Source != EvidenceSourceLog {
			t.Errorf("source = %q", r.Source)
		}
	}
}

// TestPickRefs_Empty 空证据返回 nil。
func TestPickRefs_Empty(t *testing.T) {
	if got := pickRefs(nil, EvidenceSourceLog); got != nil {
		t.Errorf("got %v want nil", got)
	}
}

// TestAttachEvidenceAndReport 统一入口正确附加。
func TestAttachEvidenceAndReport(t *testing.T) {
	result := &ReasoningResult{
		ToolCalls: []ToolCallRecord{
			newTestToolCall(1, "search_logs", "ERROR boom", true),
		},
	}
	attachEvidenceAndReport(result)

	if len(result.TraceableRefs) != 1 {
		t.Errorf("traceableRefs = %d", len(result.TraceableRefs))
	}
	if result.ThreeSection == nil {
		t.Error("threeSection 未附加")
	}
}

// TestEvidenceQuoteTruncated 证据原文被截断。
func TestEvidenceQuoteTruncated(t *testing.T) {
	long := make([]byte, maxQuoteLength*3)
	for i := range long {
		long[i] = 'a'
	}

	refs := extractEvidenceFromToolCall(newTestToolCall(1, "search_logs", string(long), true))
	if len(refs) != 1 {
		t.Fatal("len != 1")
	}
	// truncateStr 会在 max 后追加 "..."，因此总长度是 max+3
	if len(refs[0].Quote) > maxQuoteLength+3 {
		t.Errorf("quote len = %d 应被截断到 %d+3", len(refs[0].Quote), maxQuoteLength)
	}
}
