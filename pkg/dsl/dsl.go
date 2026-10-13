// Package dsl 统一查询 DSL 语法解析器。
//
// 支持管道式与类 SQL WHERE 子句两种语法，可将自然 DSL 文本转换为
// logs.controllers.SearchReq 兼容的查询计划结构。
package dsl

import "strings"

// QueryPlan 从 DSL 编译得到的查询计划。
// 与 logs.controllers.logs.SearchReq 字段对齐，可直接映射到 HTTP 请求体。
type QueryPlan struct {
	// Level 日志级别（如 ERROR/WARN/INFO）。
	Level string
	// Key 全文关键词（多个关键词空格拼接）。
	Key string
	// IsRegex 是否启用正则模式。
	IsRegex bool
	// ClientID 客户端过滤（0 表示全部）。
	ClientID int64
	// StartTime 起始时间（毫秒时间戳字符串或自然语言）。
	StartTime string
	// EndTime 结束时间。
	EndTime string
	// Filters 结构化字段过滤器（key → raw value；扩展用）。
	Filters map[string]string
	// SortBy 排序字段（count/level/time）。
	SortBy string
	// SortAsc 是否升序。
	SortAsc bool
	// GroupBy 分组字段。
	GroupBy string
	// PageNo 页码（从 1 开始）。
	PageNo int
	// PageSize 每页条数（默认 50，最大 1000）。
	PageSize int
	// NaturalLanguage 若 DSL 全文为裸词（无任何字段限定），标记为自然语言问句。
	NaturalLanguage string
}

// Parse 解析 DSL 字符串为查询计划。
// 返回 ErrParse* 哨兵或 nil。
func Parse(dsl string) (*QueryPlan, error) {
	plan, err := ParseToPlan(dsl)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

// MustParse 同 Parse，失败时 panic（仅用于测试或已知正确的常量 DSL）。
func MustParse(dsl string) *QueryPlan {
	plan, err := Parse(dsl)
	if err != nil {
		panic(err)
	}
	return plan
}

// ParseToPlan 完整解析流水线：tokenize → parse → compile → annotate。
func ParseToPlan(dsl string) (*QueryPlan, error) {
	tokens := tokenize(dsl)
	ast, err := parse(tokens)
	if err != nil {
		return nil, err
	}
	plan := compile(ast)
	annotatePlan(plan, ast)
	return plan, nil
}

// annotatePlan 基于 AST 结构添加补充语义标记（如 NaturalLanguage）。
func annotatePlan(plan *QueryPlan, expr Expr) {
	if expr == nil {
		return
	}
	if isAllBarewords(expr) {
		plan.NaturalLanguage = strings.TrimSpace(plan.Key)
	}
}

// isAllBarewords 检查 AST 是否全部由 Keyword 节点构成。
// 注意：NotExpr 使 NOT 语义生效，不被视为纯裸词。
func isAllBarewords(expr Expr) bool {
	switch n := expr.(type) {
	case *AndExpr:
		return isAllBarewords(n.Left) && isAllBarewords(n.Right)
	case *OrExpr:
		return isAllBarewords(n.Left) && isAllBarewords(n.Right)
	case *Group:
		return isAllBarewords(n.Inner)
	case *Keyword:
		return true
	default:
		return false
	}
}
