package dsl

import (
	"testing"
)

// TestCompiler_LevelEq 验证 level:ERROR 编译到 QueryPlan.Level。
func TestCompiler_LevelEq(t *testing.T) {
	tokens := tokenize("level:ERROR")
	ast, err := parse(tokens)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	plan := compile(ast)
	if plan.Level != "ERROR" {
		t.Errorf("Level = %q, want %q", plan.Level, "ERROR")
	}
}

// TestCompiler_MultipleKeywords 验证多个裸词合并为 Key。
func TestCompiler_MultipleKeywords(t *testing.T) {
	tokens := tokenize("login AND auth")
	ast, err := parse(tokens)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	plan := compile(ast)
	if plan.Key != "login auth" {
		t.Errorf("Key = %q, want %q", plan.Key, "login auth")
	}
}

// TestCompiler_ComparisonFilter 验证数值比较写入 Filters。
func TestCompiler_ComparisonFilter(t *testing.T) {
	tokens := tokenize("duration:>100")
	ast, err := parse(tokens)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	plan := compile(ast)
	if plan.Filters["duration_op"] != ">" {
		t.Errorf("Filter[duration_op] = %q, want %q", plan.Filters["duration_op"], ">")
	}
	if plan.Filters["duration"] != "100" {
		t.Errorf("Filter[duration] = %q, want %q", plan.Filters["duration"], "100")
	}
}

// TestCompiler_PaginationMeta 验证分页元数据默认值。
func TestCompiler_PaginationMeta(t *testing.T) {
	tokens := tokenize("level:WARN")
	ast, err := parse(tokens)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	plan := compile(ast)
	if plan.PageNo != 1 {
		t.Errorf("PageNo = %d, want 1", plan.PageNo)
	}
	if plan.PageSize != 50 {
		t.Errorf("PageSize = %d, want 50", plan.PageSize)
	}
}

// TestCompiler_OrExpr 验证 OR 表达式中的字段都被提取到 Filters。
func TestCompiler_OrExpr(t *testing.T) {
	tokens := tokenize("host:web-01 OR host:web-02")
	ast, err := parse(tokens)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	plan := compile(ast)
	if plan.Filters["host"] == "" {
		t.Errorf("Filters[host] should not be empty, got %v", plan.Filters)
	}
}

// TestCompiler_ComplexAnd 验证复杂 AND 表达式正确拆解所有字段。
func TestCompiler_ComplexAnd(t *testing.T) {
	tokens := tokenize("level:ERROR AND host:web-1 AND duration:>500")
	ast, err := parse(tokens)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	plan := compile(ast)
	if plan.Level != "ERROR" {
		t.Errorf("Level = %q, want %q", plan.Level, "ERROR")
	}
	if plan.Filters["host"] != "web-1" {
		t.Errorf("Filter[host] = %q, want %q", plan.Filters["host"], "web-1")
	}
	if plan.Filters["duration_op"] != ">" {
		t.Errorf("Filter[duration_op] = %q, want %q", plan.Filters["duration_op"], ">")
	}
}

// TestCompiler_NotExpr 验证 NOT 表达式内部关键词仍被提取。
func TestCompiler_NotExpr(t *testing.T) {
	tokens := tokenize("level:ERROR AND NOT debug")
	ast, err := parse(tokens)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	plan := compile(ast)
	if plan.Level != "ERROR" {
		t.Errorf("Level = %q, want %q", plan.Level, "ERROR")
	}
	if plan.Key != "debug" {
		t.Errorf("Key = %q, want %q", plan.Key, "debug")
	}
}
