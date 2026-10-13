package dsl

import (
	"strings"
	"testing"
)

// TestParse_OnlyBareword 验证单个裸词解析为 Keyword。
func TestParse_OnlyBareword(t *testing.T) {
	plan, err := Parse("login")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Key != "login" {
		t.Errorf("Key = %q, want %q", plan.Key, "login")
	}
	if plan.NaturalLanguage != "login" {
		t.Errorf("NaturalLanguage = %q, want %q", plan.NaturalLanguage, "login")
	}
	if plan.Level != "" {
		t.Errorf("Level = %q, want empty", plan.Level)
	}
}

// TestParse_FieldEq 验证 level:ERROR 解析到 Level 字段。
func TestParse_FieldEq(t *testing.T) {
	plan, err := Parse("level:ERROR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Level != "ERROR" {
		t.Errorf("Level = %q, want %q", plan.Level, "ERROR")
	}
}

// TestParse_AndOr 验证 AND/OR 和括号优先级。
func TestParse_AndOr(t *testing.T) {
	cases := []struct {
		name  string
		input string
		check func(t *testing.T, plan *QueryPlan)
	}{
		{
			name:  "AND chain with log level",
			input: "ERROR AND login",
			check: func(t *testing.T, plan *QueryPlan) {
				if plan.Level != "ERROR" {
					t.Errorf("Level = %q, want ERROR", plan.Level)
				}
				if !strings.Contains(plan.Key, "login") {
					t.Errorf("Key = %q, want containing login", plan.Key)
				}
			},
		},
		{
			name:  "OR stays as Key",
			input: "ERROR OR WARN",
			check: func(t *testing.T, plan *QueryPlan) {
				if plan.Level != "" {
					t.Errorf("Level = %q, want empty", plan.Level)
				}
				if !strings.Contains(plan.Key, "ERROR") || !strings.Contains(plan.Key, "WARN") {
					t.Errorf("Key = %q, want containing ERROR and WARN", plan.Key)
				}
			},
		},
		{
			name:  "grouped OR with AND uses field",
			input: "(ERROR OR WARN) AND host:web-01",
			check: func(t *testing.T, plan *QueryPlan) {
				if plan.Filters["host"] != "web-01" {
					t.Errorf("Filter[host] = %q, want web-01", plan.Filters["host"])
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, plan)
		})
	}
}

// TestParse_Not 验证 NOT 否定不产生自然语言标记。
func TestParse_Not(t *testing.T) {
	plan, err := Parse("NOT debug")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.NaturalLanguage != "" {
		t.Errorf("NaturalLanguage = %q, want empty (NOT implies structure)", plan.NaturalLanguage)
	}
}

// TestParse_Comparison 验证数值比较运算符。
func TestParse_Comparison(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantField string
		wantOp    string
	}{
		{"gt", "duration:>100", "duration", ">"},
		{"gte", "status:>=500", "status", ">="},
		{"lt", "size:<1024", "size", "<"},
		{"lte", "latency:<=50", "latency", "<="},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := plan.Filters[tc.wantField+"_op"]; got != tc.wantOp {
				t.Errorf("Filter[%q] = %q, want %q", tc.wantField+"_op", got, tc.wantOp)
			}
		})
	}
}

// TestParse_FullChain 验证完整查询链解析。
func TestParse_FullChain(t *testing.T) {
	plan, err := Parse("level:ERROR AND path:/api/* AND duration:>100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Level != "ERROR" {
		t.Errorf("Level = %q, want %q", plan.Level, "ERROR")
	}
	if plan.Filters["path"] != "/api/*" {
		t.Errorf("Filter[path] = %q, want %q", plan.Filters["path"], "/api/*")
	}
	if plan.Filters["duration_op"] != ">" {
		t.Errorf("Filter[duration_op] = %q, want %q", plan.Filters["duration_op"], ">")
	}
}

// TestParse_PlainNumberBareword 验证纯数字裸词。
func TestParse_PlainNumberBareword(t *testing.T) {
	plan, err := Parse("500")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.NaturalLanguage != "500" {
		t.Errorf("NaturalLanguage = %q, want %q", plan.NaturalLanguage, "500")
	}
}

// TestParse_DefaultPagination 验证默认分页参数。
func TestParse_DefaultPagination(t *testing.T) {
	plan, err := Parse("level:INFO")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.PageNo != 1 {
		t.Errorf("PageNo = %d, want 1", plan.PageNo)
	}
	if plan.PageSize != 50 {
		t.Errorf("PageSize = %d, want 50", plan.PageSize)
	}
}

// TestParse_EmptyInput 验证空输入返回错误。
func TestParse_EmptyInput(t *testing.T) {
	_, err := Parse("")
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}

// TestMustParse 验证 MustParse 行为。
func TestMustParse(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on invalid DSL")
		}
	}()
	MustParse("")
}

// TestParse_PathWildcard 验证路径通配符解析。
func TestParse_PathWildcard(t *testing.T) {
	plan, err := Parse("path:/api/* AND duration:>100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Filters["path"] != "/api/*" {
		t.Errorf("Filter[path] = %q, want %q", plan.Filters["path"], "/api/*")
	}
}
