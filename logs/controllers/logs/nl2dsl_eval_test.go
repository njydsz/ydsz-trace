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

	// === 级别变体（大小写/别名） ===
	{Name: "level_lowercase", Input: "今天的 error 日志", ExpectedLevel: "ERROR", ExpectToday: true},
	{Name: "level_warning_alias", Input: "近 1 小时 WARNING 日志", ExpectedLevel: "WARN"},
	{Name: "level_info_upper", Input: "最近 2 小时 INFO", ExpectedLevel: "INFO"},
	{Name: "level_fatal_p0", Input: "昨天 FATAL 告警", ExpectedLevel: "FATAL"},
	{Name: "level_debug_today", Input: "今天 DEBUG 级别日志", ExpectedLevel: "DEBUG", ExpectToday: true},
	{Name: "level_error_2h", Input: "过去 2 小时 ERROR", ExpectedLevel: "ERROR"},
	{Name: "level_warn_15m", Input: "最近 15 分钟 WARN", ExpectedLevel: "WARN"},
	{Name: "level_info_1d", Input: "近 1 天 INFO 日志", ExpectedLevel: "INFO"},

	// === 时间表达变体 ===
	{Name: "time_yesterday_error", Input: "昨天的 ERROR 日志", ExpectedLevel: "ERROR"},
	{Name: "time_last_2_days", Input: "近 2 天的日志", ExpectToday: false},
	{Name: "time_this_week", Input: "本周 WARN 日志", ExpectedLevel: "WARN"},
	{Name: "time_last_hour", Input: "过去一小时 ERROR", ExpectedLevel: "ERROR"},
	{Name: "time_now", Input: "现在有什么错误"},
	{Name: "time_recent_10min", Input: "最近 10 分钟的日志"},
	{Name: "time_3h_ago", Input: "3 小时前的 ERROR", ExpectedLevel: "ERROR"},

	// === 关键词变体 ===
	{Name: "key_exception", Input: "查一下 Exception 日志", ExpectedKey: "Exception"},
	{Name: "key_refused", Input: "connection refused 的报错", ExpectedKey: "connection refused"},
	{Name: "key_null_ptr", Input: "NullPointerException 出现了吗", ExpectedKey: "NullPointerException"},
	{Name: "key_timeout_case", Input: "TIMEOUT 相关日志", ExpectedKey: "TIMEOUT"},
	{Name: "key_500_error", Input: "500 状态码错误", ExpectedKey: "500"},
	{Name: "key_slow_query", Input: "慢查询 slow query 日志", ExpectedKey: "slow query"},
	{Name: "key_oom", Input: "OOM 内存溢出", ExpectedKey: "OOM"},
	{Name: "key_stack_overflow", Input: "StackOverflowError 日志", ExpectedKey: "StackOverflowError"},
	{Name: "key_full_gc", Input: "Full GC 频繁", ExpectedKey: "Full GC"},

	// === 组合查询 ===
	{Name: "combo_today_error_timeout", Input: "今天 ERROR 里包含 timeout 的", ExpectedLevel: "ERROR", ExpectedKey: "timeout", ExpectToday: true},
	{Name: "combo_2h_warn_slow", Input: "近 2 小时 WARN 中的 slow query", ExpectedLevel: "WARN", ExpectedKey: "slow query"},
	{Name: "combo_yesterday_fatal_oom", Input: "昨天 FATAL 中的 OOM", ExpectedLevel: "FATAL", ExpectedKey: "OOM"},
	{Name: "combo_keyword_only_time", Input: "近 30 分钟的 NullPointerException", ExpectedKey: "NullPointerException"},
	{Name: "combo_level_time_keyword", Input: "今天 10:00-12:00 的 ERROR timeout", ExpectedLevel: "ERROR", ExpectedKey: "timeout"},

	// === 意图/通用查询 ===
	{Name: "intent_general_today", Input: "今天的日志", ExpectToday: true},
	{Name: "intent_general_all", Input: "查看所有日志"},
	{Name: "intent_anomaly", Input: "有什么异常"},
	{Name: "intent_error_loose", Input: "报错了"},
	{Name: "intent_dashboard", Input: "看看仪表盘"},
	{Name: "intent_help", Input: "怎么用"},

	// === 边缘/噪声 ===
	{Name: "noise_single_char", Input: "a"},
	{Name: "noise_punctuation", Input: "！！！"},
	{Name: "noise_log_word", Input: "日志"},
	{Name: "noise_hello", Input: "你好"},
	{Name: "noise_number_only", Input: "12345"},
	{Name: "noise_mixed", Input: "查一下 x"},

	// === 长句/口语化 ===
	{Name: "long_colloquial_1", Input: "帮我看看今天有没有什么报错，特别是 timeout 相关的", ExpectedLevel: "ERROR", ExpectToday: true},
	{Name: "long_colloquial_2", Input: "最近一小时有没有 WARN 以上的日志", ExpectedLevel: "WARN"},
	{Name: "long_colloquial_3", Input: "昨天下午 3 点到 5 点之间有没有 FATAL", ExpectedLevel: "FATAL"},
	{Name: "long_colloquial_4", Input: "我想知道近 3 天 INFO 级别的日志有多少", ExpectedLevel: "INFO"},
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
