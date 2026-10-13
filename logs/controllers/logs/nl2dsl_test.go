// Package logs_test 包含 NL2DSL 服务单元测试。
//
// 验证自然语言解析能力：级别提取、时间范围、关键词提取、意图分类。
package logs

import (
	"testing"
)

// TestNl2Dsl_LevelExtraction 验证日志级别提取。
func TestNl2Dsl_LevelExtraction(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ERROR", "今天的 ERROR 日志", "ERROR"},
		{"WARN", "近1小时 WARN", "WARN"},
		{"INFO", "查 INFO 级别", "INFO"},
		{"DEBUG", "DEBUG 日志", "DEBUG"},
		{"FATAL", "FATAL 崩溃", "FATAL"},
		{"无级别", "今天的日志", ""},
		{"大小写混合", "error 日志", "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nl2dslExtractLevel(tt.input)
			if got != tt.want {
				t.Errorf("输入=%q, 得到=%q, 期望=%q", tt.input, got, tt.want)
			}
		})
	}
}

// TestNl2Dsl_TimeRange 验证时间范围解析。
func TestNl2Dsl_TimeRange(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantDetail   string
		wantHasStart bool
	}{
		{"近1小时", "近 1 小时 ERROR", " - ", true},
		{"今天", "今天的 WARN", "今天", true},
		{"近3天", "近 3 天日志", " - ", false},
		{"无时间", "ERROR 日志", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, startTime, endTime, detail := nl2dslExtractTimeRange(tt.input)
			if tt.wantHasStart && startTime == "" {
				t.Errorf("输入=%q: 期望有起始时间, 未得到", tt.input)
			}
			if tt.wantDetail != "" && detail == "" {
				t.Errorf("输入=%q: 期望时间描述包含 %q, 未得到", tt.input, tt.wantDetail)
			}
			if !tt.wantHasStart && detail != "" && tt.wantDetail == "" {
				t.Errorf("输入=%q: 不期望有时间描述, 得到了=%q", tt.input, detail)
			}
			_ = endTime
		})
	}
}

// TestNl2Dsl_KeyExtraction 验证关键词提取。
func TestNl2Dsl_KeyExtraction(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"引号内文本", `查找包含 "NullPointerException" 的日志`, "NullPointerException"},
		{"中文引号", `查找 "超时" 日志`, "超时"},
		{"无引号清理残留", "搜索 login failed 日志", "搜索 login failed"},
		{"带级别清理", "今天 ERROR", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := nl2dslBuildResponse(tt.input)
			if resp.Key != tt.want {
				t.Errorf("输入=%q: Key 得到=%q, 期望=%q", tt.input, resp.Key, tt.want)
			}
		})
	}
}

// TestNl2Dsl_IntentDetection 验证意图分类。
func TestNl2Dsl_IntentDetection(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{"ERROR+keyword", `查找包含 "timeout" 的 ERROR 日志`, "error_search"},
		{"仅ERROR", "ERROR", "error_search"},
		{"仅keyword", `查找 "login"`, "keyword_search"},
		{"today+timerange", "今天的日志", "time_range"},
		{"general", "日志", "general"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := nl2dslBuildResponse(tt.key)
			if resp.IntentType != tt.want {
				t.Errorf("输入=%q: IntentType 得到=%q, 期望=%q", tt.key, resp.IntentType, tt.want)
			}
		})
	}
}

// TestNl2Dsl_ResponseFormat 验证 DSL 条件表达式格式。
func TestNl2Dsl_ResponseFormat(t *testing.T) {
	resp := nl2dslBuildResponse("近 1 小时 ERROR 日志")

	// 基本字段验证
	if resp.Level != "ERROR" {
		t.Errorf("Level 应=ERROR, 得到=%q", resp.Level)
	}
	if resp.IntentType == "" {
		t.Error("IntentType 不应为空")
	}
	if resp.DslConditions == "" {
		t.Error("DslConditions 不应为空")
	}
	if resp.UserIntent == "" {
		t.Error("UserIntent 不应为空")
	}
}

// TestNl2Dsl_EmptyInput 验证空输入处理。
func TestNl2Dsl_EmptyInput(t *testing.T) {
	// nl2dslBuildResponse 不校验空输入（由 HTTP handler 负责），
	// 但构建不应 panic。
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("空输入导致 panic: %v", r)
		}
	}()
	nl2dslBuildResponse("")
}
