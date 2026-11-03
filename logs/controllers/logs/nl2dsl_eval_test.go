package logs

import (
	"strings"
	"testing"
	"time"
)

// evalCase NL2DSL 评测用例（后端 Go 实现）。
type evalCase struct {
	Name          string
	Input         string
	ExpectedLevel string
	ExpectedKey   string
	ExpectToday   bool // true: 验证日期为今天
}

// evalCases NL2DSL 评测集（目标 200 条，首批覆盖核心场景）。
//
// 评测命令: go test ./logs/controllers/logs/ -run TestNl2DslEval -v
// CI 要求: accuracy >= 0.95。
var evalCases = []evalCase{
	// === 级别提取 ===
	{Name: "error_today", Input: "今天有哪些 ERROR？", ExpectedLevel: "ERROR", ExpectToday: true},
	{Name: "warn_1h", Input: "近 1 小时 WARN 日志", ExpectedLevel: "WARN"},
	{Name: "info_3d", Input: "近 3 天的 INFO 日志", ExpectedLevel: "INFO"},
	{Name: "fatal_yesterday", Input: "昨天的 FATAL 错误", ExpectedLevel: "FATAL"},
	{Name: "debug_today", Input: "今天的 DEBUG 日志", ExpectedLevel: "DEBUG", ExpectToday: true},

	// === 关键词搜索 ===
	{Name: "keyword_quoted", Input: "查一下 \"NullPointerException\"", ExpectedKey: "NullPointerException"},
	{Name: "keyword_500", Input: "查一下 500 错误", ExpectedKey: "500"},
	{Name: "keyword_timeout", Input: "搜索包含 timeout 的日志", ExpectedKey: "timeout"},

	// === 组合查询 ===
	{Name: "combo_error_keyword", Input: "今天包含 connection refused 的 ERROR", ExpectedLevel: "ERROR", ExpectedKey: "connection refused"},
	{Name: "combo_level_time", Input: "过去 1 小时内所有 WARN", ExpectedLevel: "WARN"},

	// === 边缘场景 ===
	{Name: "general_today", Input: "查看今天的日志", ExpectToday: true},
	{Name: "no_level", Input: "最近有什么异常"},
	{Name: "empty_after_clean", Input: "日志"},

	// === 时间精确 ===
	{Name: "exact_time", Input: "14:00-15:00 的 ERROR 日志", ExpectedLevel: "ERROR"},
	{Name: "recent_30min", Input: "最近 30 分钟"},
}

// TestNl2DslEval NL2DSL 后端评测集驱动测试。
//
// 覆盖 FR-701/FR-702 核心解析能力。
// CI 集成: make test → go test ./logs/controllers/logs/ -run TestNl2DslEval。
// 阈值: 通过率 >= 0.95。
func TestNl2DslEval(t *testing.T) {
	passed := 0
	for _, tc := range evalCases {
		t.Run(tc.Name, func(t *testing.T) {
			result := nl2dslBuildResponse(tc.Input)

			ok := true
			if tc.ExpectedLevel != "" && result.Level != tc.ExpectedLevel {
				t.Errorf("Level mismatch: got %q, want %q", result.Level, tc.ExpectedLevel)
				ok = false
			}
			if tc.ExpectedKey != "" && !strings.Contains(strings.ToLower(result.Key), strings.ToLower(tc.ExpectedKey)) {
				t.Errorf("Key mismatch: got %q, want contains %q", result.Key, tc.ExpectedKey)
				ok = false
			}
			if tc.ExpectToday {
				expected := time.Now().Format("20060102")
				if result.Date != expected {
					t.Errorf("Date mismatch: got %q, want today %q", result.Date, expected)
					ok = false
				}
			}
			if ok {
				passed++
			}
		})
	}

	accuracy := float64(passed) / float64(len(evalCases))
	t.Logf("NL2DSL Eval: %d/%d passed (accuracy: %.2f)", passed, len(evalCases), accuracy)

	if accuracy < 0.95 {
		t.Errorf("NL2DSL 评测通过率 %.2f 低于阈值 0.95", accuracy)
	}
}
