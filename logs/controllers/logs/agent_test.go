// Package logs 子文件：根因分析 Agent 单元测试（FR-901~908）。
package logs

import (
	"context"
	"encoding/json"
	"testing"
)

// TestAgentTool_Names 验证 4 个只读工具的 Name/Description 符合契约。
func TestAgentTool_Names(t *testing.T) {
	tools := []AgentTool{
		&SearchLogsTool{},
		&AggStatsTool{},
		&GetContextTool{},
		&GetTraceTool{},
	}
	expected := map[string]bool{
		"search_logs": false,
		"agg_stats":   false,
		"get_context": false,
		"get_trace":   false,
	}
	for _, tool := range tools {
		name := tool.Name()
		if _, ok := expected[name]; !ok {
			t.Errorf("unexpected tool name: %s", name)
			continue
		}
		expected[name] = true
		if tool.Description() == "" {
			t.Errorf("tool %s has empty description", name)
		}
	}
	for name, seen := range expected {
		if !seen {
			t.Errorf("missing tool: %s", name)
		}
	}
}

// TestReasoningAgent_ToolRegistration 验证 NewReasoningAgent 注册了全部 4 个工具。
func TestReasoningAgent_ToolRegistration(t *testing.T) {
	agent := NewReasoningAgent()
	if len(agent.tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(agent.tools))
	}
	for _, name := range []string{"search_logs", "agg_stats", "get_context", "get_trace"} {
		if _, ok := agent.tools[name]; !ok {
			t.Errorf("missing tool: %s", name)
		}
	}
	if agent.maxSteps != agentMaxSteps {
		t.Errorf("maxSteps = %d, want %d", agent.maxSteps, agentMaxSteps)
	}
}

// TestParseLLMOutput_ToolCall 验证从 markdown 代码块中解析工具调用 JSON。
func TestParseLLMOutput_ToolCall(t *testing.T) {
	output := "```json\n{\"thought\":\"需要检索日志\",\"action\":{\"tool\":\"search_logs\",\"params\":{\"item\":1,\"date\":\"20260101\",\"key\":\"timeout\"}}}\n```"
	step, final := parseLLMOutput(output)
	if final != "" {
		t.Fatalf("unexpected finalAnswer: %s", final)
	}
	if step.Action == nil {
		t.Fatal("expected action, got nil")
	}
	if step.Action.Tool != "search_logs" {
		t.Errorf("tool = %s, want search_logs", step.Action.Tool)
	}
	if step.Action.Params["key"] != "timeout" {
		t.Errorf("params[key] = %v, want timeout", step.Action.Params["key"])
	}
}

// TestParseLLMOutput_FinalAnswer 验证解析 final_answer 结束标记。
func TestParseLLMOutput_FinalAnswer(t *testing.T) {
	output := `{"thought":"已完成排查","finalAnswer":"{\"hypotheses\":[],\"evidence\":[],\"conclusion\":{\"rootCause\":\"test\",\"confidence\":0.5}}"}`
	step, final := parseLLMOutput(output)
	if final == "" {
		t.Fatal("expected finalAnswer, got empty")
	}
	if step.FinalAnswer != final {
		t.Errorf("step.FinalAnswer mismatch")
	}
}

// TestParseFinalAnswer_JSON 验证三段式 JSON 解析。
func TestParseFinalAnswer_JSON(t *testing.T) {
	text := `{"hypotheses":[{"index":1,"statement":"h1","confidence":0.6}],"evidence":[{"templateHash":"abc","count":10}],"conclusion":{"rootCause":"rc","suggestions":["s1"],"confidence":0.7}}`
	result := &ReasoningResult{}
	parseFinalAnswer(text, result)
	if len(result.Hypotheses) != 1 || result.Hypotheses[0].Statement != "h1" {
		t.Errorf("hypotheses mismatch: %+v", result.Hypotheses)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].TemplateHash != "abc" {
		t.Errorf("evidence mismatch: %+v", result.Evidence)
	}
	if result.Conclusion.RootCause != "rc" || result.Conclusion.Confidence != 0.7 {
		t.Errorf("conclusion mismatch: %+v", result.Conclusion)
	}
}

// TestParseFinalAnswer_TextFallback 验证非 JSON finalAnswer 降级为文本。
func TestParseFinalAnswer_TextFallback(t *testing.T) {
	text := "这是一段纯文本结论"
	result := &ReasoningResult{}
	parseFinalAnswer(text, result)
	if len(result.Hypotheses) != 1 {
		t.Fatalf("expected 1 hypothesis, got %d", len(result.Hypotheses))
	}
	if result.Conclusion.RootCause != text {
		t.Errorf("rootCause = %q, want %q", result.Conclusion.RootCause, text)
	}
}

// TestReasoningResult_JSONSerializable 验证结果可被 JSON 序列化。
func TestReasoningResult_JSONSerializable(t *testing.T) {
	result := &ReasoningResult{
		Query:    "test",
		Hypotheses: []HypothesisSection{{Index: 1, Statement: "h", Confidence: 0.5}},
		Evidence:  []EvidenceSection{{TemplateHash: "abc", Count: 3, Source: "agg_stats"}},
		Conclusion: ConclusionSection{RootCause: "rc", Suggestions: []string{"s"}, Confidence: 0.6},
		Steps:     2,
		ToolCalls: []ToolCallRecord{{Step: 1, Tool: "search_logs", Params: map[string]interface{}{"item": 1}}},
		Degraded:  true,
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var round ReasoningResult
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if round.Query != "test" || len(round.Evidence) != 1 {
		t.Errorf("round-trip mismatch: %+v", round)
	}
}

// TestAggStatsTool_TimeRangeHelper 验证时间范围辅助函数。
func TestAggStatsTool_TimeRangeHelper(t *testing.T) {
	tests := []struct {
		input string
		ok    bool
	}{
		{"1h", true},
		{"6h", true},
		{"24h", true},
		{"7d", true},
		{"invalid", false},
	}
	for _, tt := range tests {
		bucket := timeRangeToBucketForAgent(tt.input)
		if tt.ok && bucket == "" {
			t.Errorf("timeRangeToBucketForAgent(%q) = empty, want non-empty", tt.input)
		}
		if !tt.ok && bucket != "" {
			t.Errorf("timeRangeToBucketForAgent(%q) = %q, want empty", tt.input, bucket)
		}
	}
}

// TestReasoningAgent_RunDegraded 验证 LLM 不可用时降级模式不报错且返回三段式结构。
//
// 注意：该测试依赖 DB 未初始化（models.DB == nil）或 TopPatterns 返回空，
// 从而触发空数据分支；无论走哪个分支，结果都不应为 nil。
func TestReasoningAgent_RunDegraded(t *testing.T) {
	// 强制 LLM 不可用
	agent := NewReasoningAgent()
	agent.llmClient = &LLMClient{model: nil}

	ctx, cancel := context.WithTimeout(context.Background(), 10*1000*1000*1000)
	defer cancel()

	result, err := agent.Run(ctx, "今天 ERROR 日志为什么激增？", "")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if !result.Degraded {
		t.Error("expected degraded=true when LLM unavailable")
	}
	if len(result.Hypotheses) == 0 {
		t.Error("expected at least one hypothesis in degraded mode")
	}
	if result.Conclusion.RootCause == "" {
		t.Error("expected non-empty root cause in degraded mode")
	}
}
