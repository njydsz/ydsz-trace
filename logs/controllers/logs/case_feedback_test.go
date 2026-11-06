package logs

import (
	"strings"
	"testing"
)

// TestExtractSymptomKeywords 从查询提取症状关键词。
func TestExtractSymptomKeywords(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  []string // 期望包含的关键词（至少其一）
	}{
		{
			name:  "英文错误词",
			query: "服务出现 connection refused 错误",
			want:  []string{"connection"},
		},
		{
			name:  "引号内文本优先",
			query: "查一下 「timeout」 相关日志",
			want:  []string{"timeout"},
		},
		{
			name:  "下划线错误码",
			query: "ERR_001 异常排查",
			want:  []string{"ERR_001"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractSymptomKeywords(c.query)
			if len(got) == 0 {
				t.Fatalf("未提取到关键词")
			}
			found := false
			for _, w := range c.want {
				for _, g := range got {
					if strings.EqualFold(g, w) {
						found = true
					}
				}
			}
			if !found {
				t.Errorf("got %v 应包含 %v", got, c.want)
			}
		})
	}
}

// TestExtractSymptomKeywords_StopWords 停用词被过滤。
func TestExtractSymptomKeywords_StopWords(t *testing.T) {
	got := extractSymptomKeywords("日志错误异常")
	for _, k := range got {
		if isStopWord(k) {
			t.Errorf("停用词未被过滤: %q", k)
		}
	}
}

// TestExtractSymptomKeywords_Limit 关键词数量受限。
func TestExtractSymptomKeywords_Limit(t *testing.T) {
	query := "alpha beta gamma delta epsilon zeta eta theta"
	got := extractSymptomKeywords(query)
	if len(got) > maxSymptomKeywords {
		t.Errorf("len = %d 超过上限 %d", len(got), maxSymptomKeywords)
	}
}

// TestExtractSymptomKeywords_ShortWordsSkipped 过短词被跳过。
func TestExtractSymptomKeywords_ShortWordsSkipped(t *testing.T) {
	got := extractSymptomKeywords("ab cd ef gh")
	if len(got) != 0 {
		t.Errorf("短词应被跳过, got %v", got)
	}
}

// TestIsStopWord 停用词判断。
func TestIsStopWord(t *testing.T) {
	for _, w := range []string{"日志", "错误", "error", "failed"} {
		if !isStopWord(w) {
			t.Errorf("%q 应为停用词", w)
		}
	}
	for _, w := range []string{"timeout", "connection", "redis"} {
		if isStopWord(w) {
			t.Errorf("%q 不应为停用词", w)
		}
	}
}

// TestExtractQuotedTexts 引号文本提取。
func TestExtractQuotedTexts(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"查 「timeout」 日志", []string{"timeout"}},
		{"查 “slow query” 日志", []string{"slow query"}},
		{`查 "connection refused" 日志`, []string{"connection refused"}},
		{"无引号文本", nil},
		{"", nil},
	}

	for _, c := range cases {
		got := extractQuotedTexts(c.in)
		if len(got) != len(c.want) {
			t.Errorf("extractQuotedTexts(%q) = %v want %v", c.in, got, c.want)
			continue
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("idx %d: got %q want %q", i, got[i], c.want[i])
			}
		}
	}
}

// TestBuildCaseTitle 标题构造。
func TestBuildCaseTitle(t *testing.T) {
	got := buildCaseTitle("timeout 超时", []string{"timeout", "redis"})
	if got != "timeout/redis" {
		t.Errorf("got %q", got)
	}

	// 无关键词时回退到查询文本
	got = buildCaseTitle("服务异常排查", nil)
	if !strings.Contains(got, "服务异常排查") {
		t.Errorf("got %q", got)
	}
}

// TestBuildCaseTitle_Truncated 超长标题被截断。
func TestBuildCaseTitle_Truncated(t *testing.T) {
	long := strings.Repeat("a", 60)
	got := buildCaseTitle(long, nil)
	if len([]rune(got)) > 40 {
		t.Errorf("len = %d 应截断到 40", len([]rune(got)))
	}
}

// TestAgentPromptWithCases 案例注入。
func TestAgentPromptWithCases(t *testing.T) {
	base := "你是日志分析助手。"
	cases := []SimilarCase{
		{Title: "timeout 问题", RootCause: "连接池耗尽", Solution: "扩容连接池", HitCount: 5},
	}

	got := AgentPromptWithCases(base, cases)
	if !strings.Contains(got, "历史相似案例") {
		t.Error("应包含案例章节标题")
	}
	if !strings.Contains(got, "连接池耗尽") {
		t.Error("应包含根因")
	}
	if !strings.Contains(got, "5") {
		t.Error("应包含命中次数")
	}
}

// TestAgentPromptWithCases_Empty 无案例时原样返回。
func TestAgentPromptWithCases_Empty(t *testing.T) {
	base := "prompt"
	if got := AgentPromptWithCases(base, nil); got != base {
		t.Errorf("got %q want %q", got, base)
	}
}

// TestSearchSimilarCases_EmptyQuery 空查询返回 nil。
func TestSearchSimilarCases_EmptyQuery(t *testing.T) {
	if got := SearchSimilarCases(""); got != nil {
		t.Errorf("got %v want nil", got)
	}
}

// TestSearchSimilarCases_NoKeywords 无有效关键词返回 nil。
func TestSearchSimilarCases_NoKeywords(t *testing.T) {
	// 全停用词 → 无关键词可检索
	if got := SearchSimilarCases("日志错误异常"); got != nil {
		t.Errorf("got %v want nil", got)
	}
}

// TestIntParse 整型解析。
func TestIntParse(t *testing.T) {
	if got := parseIntParamForTest("123"); got != 123 {
		t.Errorf("got %d", got)
	}
	if got := parseIntParamForTest("abc"); got != 0 {
		t.Errorf("got %d", got)
	}
	if got := parseIntParamForTest(""); got != 0 {
		t.Errorf("got %d", got)
	}
	if got := parseIntParamForTest("-5"); got != 0 {
		t.Errorf("got %d", got)
	}
}

// parseIntParamForTest 测试辅助（避免依赖 gin.Context）。
func parseIntParamForTest(s string) int {
	if s == "" {
		return 0
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
