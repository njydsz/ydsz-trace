package dsl

import (
	"strconv"
	"strings"
)

// knownLevels 内置日志级别集合（大小写不敏感匹配）。
var knownLevels = map[string]bool{
	"fatal": true, "error": true, "warn": true, "warning": true,
	"info": true, "debug": true, "trace": true,
}

// compiler 将 AST 编译为 QueryPlan。
type compiler struct {
	plan  *QueryPlan
	mode  compileMode // 当前子表达式所在的逻辑上下文
	levelSet bool     // 是否已设置 Level 字段（防止 OR 后项覆盖）
}

// compileMode 表示子表达式所在的逻辑父上下文。
type compileMode int

const (
	// modeAnd 默认 AND 上下文，裸词可映射为该级别的 Level。
	modeAnd compileMode = iota
	// modeOr OR 上下文，裸词统一归入 Key。
	modeOr
)

// compile 从 AST 根节点生成 QueryPlan。
func compile(expr Expr) *QueryPlan {
	c := &compiler{plan: &QueryPlan{
		Filters:  make(map[string]string),
		PageNo:   1,
		PageSize: 50,
	}}
	c.visit(expr)
	return c.plan
}

// visit 递归遍历 AST 节点并填充 QueryPlan。
func (c *compiler) visit(expr Expr) {
	switch n := expr.(type) {
	case *AndExpr:
		c.visit(n.Left)
		c.visit(n.Right)
	case *OrExpr:
		prev := c.mode
		c.mode = modeOr
		c.visit(n.Left)
		c.visit(n.Right)
		c.mode = prev
	case *NotExpr:
		c.visit(n.Inner)
	case *FieldEq:
		c.applyFieldEq(n.Field, n.Value)
	case *FieldLike:
		if n.Field == "*" {
			c.appendKey(n.Value)
		} else {
			c.plan.Filters[n.Field] = n.Value
		}
	case *FieldCmp:
		c.plan.Filters[n.Field+"_op"] = n.Op
		c.plan.Filters[n.Field] = strconv.FormatFloat(n.Value, 'f', -1, 64)
	case *Keyword:
		c.applyKeyword(n.Value)
	case *Group:
		c.visit(n.Inner)
	}
}

// applyKeyword 根据上下文将裸词关键词分配到 Level 或 Key。
func (c *compiler) applyKeyword(value string) {
	if c.mode == modeAnd && !c.levelSet && isLogLevel(value) {
		c.plan.Level = strings.ToUpper(value)
		c.levelSet = true
		return
	}
	c.appendKey(value)
}

// applyFieldEq 应用字段等值匹配到 QueryPlan。
func (c *compiler) applyFieldEq(field, value string) {
	switch strings.ToLower(field) {
	case "level":
		c.plan.Level = strings.ToUpper(value)
		c.levelSet = true
	case "key", "keyword":
		c.appendKey(value)
	case "client", "client_id", "clientid":
		if id, err := strconv.ParseInt(value, 10, 64); err == nil {
			c.plan.ClientID = id
		}
	case "start", "start_time", "starttime":
		c.plan.StartTime = value
	case "end", "end_time", "endtime":
		c.plan.EndTime = value
	case "isregex", "regex", "is_regex":
		c.plan.IsRegex = strings.EqualFold(value, "true") || value == "1"
	case "pageno", "page":
		if n, err := strconv.Atoi(value); err == nil {
			c.plan.PageNo = n
		}
	case "pagesize", "size":
		if n, err := strconv.Atoi(value); err == nil {
			c.plan.PageSize = clamp(n, 1, 1000)
		}
	case "sort", "sortby":
		c.plan.SortBy = value
	case "sortdir":
		c.plan.SortAsc = strings.EqualFold(value, "asc")
	case "groupby":
		c.plan.GroupBy = value
	default:
		c.plan.Filters[field] = value
	}
}

// appendKey 就地追加全文关键词。
func (c *compiler) appendKey(value string) {
	if c.plan.Key == "" {
		c.plan.Key = value
		return
	}
	c.plan.Key += " " + value
}

// isLogLevel 判断字符串是否为已知日志级别名称。
func isLogLevel(s string) bool {
	return knownLevels[strings.ToLower(s)]
}

// clamp 将数值限制在 [min, max] 范围内。
func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
