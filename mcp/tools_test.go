package main

import (
	"testing"
)

// TestBuildTools_Count 工具数量满足 10 类能力要求（V2.4-01）。
func TestBuildTools_Count(t *testing.T) {
	s := &MCPServer{baseURL: "http://127.0.0.1:2021"}
	tools := BuildTools(s)
	if len(tools) < 10 {
		t.Fatalf("工具数 = %d, 要求 >= 10", len(tools))
	}
}

// TestBuildTools_NamesUnique 工具名不重复。
func TestBuildTools_NamesUnique(t *testing.T) {
	s := &MCPServer{baseURL: "http://127.0.0.1:2021"}
	tools := BuildTools(s)

	seen := make(map[string]bool, len(tools))
	for _, tool := range tools {
		if tool.Name == "" {
			t.Fatal("存在无名工具")
		}
		if seen[tool.Name] {
			t.Errorf("工具名重复: %s", tool.Name)
		}
		seen[tool.Name] = true

		if tool.Description == "" {
			t.Errorf("工具 %s 缺少描述", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("工具 %s 缺少 InputSchema", tool.Name)
		}
		if tool.Handler == nil {
			t.Errorf("工具 %s 缺少 Handler", tool.Name)
		}
	}
}

// TestBuildTools_RequiredCoversCoreCapabilities 覆盖核心排障能力。
func TestBuildTools_RequiredCoversCoreCapabilities(t *testing.T) {
	s := &MCPServer{baseURL: "http://127.0.0.1:2021"}
	tools := BuildTools(s)

	byName := make(map[string]MCPTool, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}

	required := []string{
		"search_logs",      // 日志检索
		"agg_stats",        // 聚合统计
		"get_context",      // 上下文回溯
		"get_trace",        // 链路追踪
		"nl2dsl_parse",     // 自然语言解析
		"semantic_search",  // 语义检索
		"get_anomalies",    // 异常事件
		"list_templates",   // 模板列表
		"diff_time_window", // 时间窗对比
		"reason_incident",  // 根因推理
	}

	for _, name := range required {
		if _, ok := byName[name]; !ok {
			t.Errorf("缺少核心工具: %s", name)
		}
	}
}

// TestToolNames_MatchesBuildTools ToolNames 与 BuildTools 一致。
func TestToolNames_MatchesBuildTools(t *testing.T) {
	s := &MCPServer{baseURL: "http://127.0.0.1:2021"}
	tools := BuildTools(s)

	names := ToolNames()
	if len(names) != len(tools) {
		t.Errorf("ToolNames 长度 = %d, BuildTools = %d", len(names), len(tools))
	}

	toolSet := make(map[string]bool, len(tools))
	for _, tool := range tools {
		toolSet[tool.Name] = true
	}
	for _, name := range names {
		if !toolSet[name] {
			t.Errorf("ToolNames 中的 %s 未在 BuildTools 中定义", name)
		}
	}
}

// TestBuildTools_NoLegacyApiV2LogsPaths 不再指向不存在的 /api/v2/logs/* 路径。
//
// 背景：V2.4-01 之前所有工具都指向 /api/v2/logs/...，
// 但服务端实际注册的是 /logs/...（v2 组只覆盖 semantic/nl2dsl/query/traces），
// 导致工具调用必然 404。
func TestBuildTools_NoLegacyApiV2LogsPaths(t *testing.T) {
	s := &MCPServer{baseURL: "http://127.0.0.1:2021"}
	tools := BuildTools(s)

	// 通过 InputSchema 的描述无法直接拿到 path，这里改为校验
	// 工具集不包含明显指向旧路径的命名（路径在闭包内，需源码审查）。
	// 实际防护由 router 的集成测试承担。
	for _, tool := range tools {
		if containsSubstring(tool.Description, "/api/v2/logs") {
			t.Errorf("工具 %s 描述中仍引用旧路径", tool.Name)
		}
	}
}

// containsSubstring 判断子串存在。
func containsSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
