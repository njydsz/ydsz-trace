package logs

import (
	"testing"
)

// TestNL2DSLConfidence 置信度评估表驱动测试。
func TestNL2DSLConfidence(t *testing.T) {
	cases := []struct {
		name string
		resp Nl2DslParseResp
		want float64
	}{
		{
			name: "级别+关键词+时间 → 满分",
			resp: Nl2DslParseResp{Level: "ERROR", Key: "timeout", StartTime: "10:00:00", EndTime: "11:00:00"},
			want: 1.0,
		},
		{
			name: "仅关键词",
			resp: Nl2DslParseResp{Key: "timeout"},
			want: 0.35,
		},
		{
			name: "仅级别",
			resp: Nl2DslParseResp{Level: "ERROR"},
			want: 0.25,
		},
		{
			name: "general 意图不额外扣分",
			resp: Nl2DslParseResp{IntentType: INTENT_GENERAL, Level: "ERROR"},
			want: 0.25,
		},
		{
			name: "仅 Date（默认填充）→ 0 分",
			resp: Nl2DslParseResp{Date: "20261106"},
			want: 0,
		},
		{
			name: "关键词过短 → 不算有效提取",
			resp: Nl2DslParseResp{Key: "x"},
			want: 0,
		},
		{
			name: "纯数字关键词 → 不算有效提取",
			resp: Nl2DslParseResp{Key: "12345"},
			want: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := nl2dslConfidence(c.resp)
			if got < c.want-0.001 || got > c.want+0.001 {
				t.Errorf("confidence = %.3f want %.3f", got, c.want)
			}
		})
	}
}

// TestParseNL2DSLJSON 从 LLM 输出解析 JSON。
func TestParseNL2DSLJSON(t *testing.T) {
	cases := []struct {
		name  string
		input string
		ok    bool
		level string
		key   string
	}{
		{
			name:  "纯 JSON",
			input: `{"level":"ERROR","key":"timeout","intentType":"error_search"}`,
			ok:    true,
			level: "ERROR",
			key:   "timeout",
		},
		{
			name:  "带 markdown 代码块",
			input: "```json\n{\"level\":\"WARN\",\"key\":\"x\"}\n```",
			ok:    true,
			level: "WARN",
			key:   "x",
		},
		{
			name:  "前后有解释文字",
			input: "这是解析结果：{\"level\":\"INFO\",\"key\":\"a\"} 以上。",
			ok:    true,
			level: "INFO",
			key:   "a",
		},
		{
			name:  "非 JSON",
			input: "无法解析",
			ok:    false,
		},
		{
			name:  "空输入",
			input: "",
			ok:    false,
		},
		{
			name:  "JSON 语法错误",
			input: `{"level":}`,
			ok:    false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseNL2DSLJSON(c.input)
			if ok != c.ok {
				t.Fatalf("ok = %v want %v", ok, c.ok)
			}
			if !ok {
				return
			}
			if got.Level != c.level {
				t.Errorf("level = %q want %q", got.Level, c.level)
			}
			if got.Key != c.key {
				t.Errorf("key = %q want %q", got.Key, c.key)
			}
		})
	}
}

// TestNL2DSLMerge LLM 结果优先、规则结果补空。
func TestNL2DSLMerge(t *testing.T) {
	rule := Nl2DslParseResp{
		UserIntent: "近3小时ERROR",
		Level:      "ERROR",
		StartTime:  "10:00:00",
		EndTime:    "13:00:00",
		Key:        "",
		IntentType: INTENT_ERROR_SEARCH,
	}
	llm := nl2dslLLMResponse{
		Level:     "ERROR",
		Key:       "timeout",
		TimeRange: "近3小时",
		Intent:    INTENT_ERROR_SEARCH,
	}

	merged := nl2dslMerge(rule, llm)

	if merged.Level != "ERROR" {
		t.Errorf("level = %q", merged.Level)
	}
	if merged.Key != "timeout" {
		t.Errorf("key = %q want timeout", merged.Key)
	}
	// 规则有、LLM 无的字段应保留
	if merged.StartTime != "10:00:00" {
		t.Errorf("startTime = %q want 规则值", merged.StartTime)
	}
	if merged.TimeRangeDetail != "近3小时" {
		t.Errorf("timeRangeDetail = %q", merged.TimeRangeDetail)
	}
	// DSL 条件应重新生成
	if merged.DslConditions == "" || merged.DslConditions == rule.DslConditions {
		t.Errorf("dslConditions 应重新生成: %q", merged.DslConditions)
	}
}

// TestNL2DSLMerge_EmptyLLMFields LLM 字段为空时保留规则值。
func TestNL2DSLMerge_EmptyLLMFields(t *testing.T) {
	rule := Nl2DslParseResp{Level: "WARN", Key: "slow query", StartTime: "09:00:00"}
	merged := nl2dslMerge(rule, nl2dslLLMResponse{})

	if merged.Level != "WARN" {
		t.Errorf("level = %q want WARN", merged.Level)
	}
	if merged.Key != "slow query" {
		t.Errorf("key = %q want slow query", merged.Key)
	}
	if merged.StartTime != "09:00:00" {
		t.Errorf("startTime = %q", merged.StartTime)
	}
}

// TestNL2DSLParseWithFallback_RulePath 规则置信度足够时走规则路径。
func TestNL2DSLParseWithFallback_RulePath(t *testing.T) {
	resp, strategy := nl2dslParseWithFallback("近3小时 ERROR 日志")

	if strategy != "rule" {
		t.Errorf("strategy = %q want rule", strategy)
	}
	if resp.Level != "ERROR" {
		t.Errorf("level = %q want ERROR", resp.Level)
	}
	if resp.Confidence < nl2dslMinRuleConfidence {
		t.Errorf("confidence = %.2f want >= %.2f", resp.Confidence, nl2dslMinRuleConfidence)
	}
}

// TestNL2DSLParseWithFallback_DegradedPath 无有效信息时降级（LLM 不可用场景）。
func TestNL2DSLParseWithFallback_DegradedPath(t *testing.T) {
	// "abcdefg" 无任何可提取信息，规则置信度为 0，LLM 不可用时降级
	resp, strategy := nl2dslParseWithFallback("abcdefg")

	if strategy != "degraded" {
		t.Errorf("strategy = %q want degraded", strategy)
	}
	if resp.Level != "" {
		t.Errorf("level = %q want empty", resp.Level)
	}
}

// TestBuildNL2DSLSystemPrompt 提示词包含必要约束。
func TestBuildNL2DSLSystemPrompt(t *testing.T) {
	prompt := buildNL2DSLSystemPrompt()
	for _, want := range []string{"level", "intentType", "JSON"} {
		if !containsStr(prompt, want) {
			t.Errorf("prompt 缺少 %q", want)
		}
	}
}

// containsStr 判断子串存在。
func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
