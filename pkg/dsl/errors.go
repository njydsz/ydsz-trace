// Package dsl 统一查询 DSL 语法解析器。
package dsl

import "errors"

// 解析错误哨兵。
var (
	// ErrParseEOF 输入在解析完成前结束。
	ErrParseEOF = errors.New("DSL 解析错误: 意外的输入结束")
	// ErrParseSyntax 语法异常（如括号不匹配、运算符不完整）。
	ErrParseSyntax = errors.New("DSL 解析错误: 语法异常")
	// ErrInvalidField 遇到的字段名未知。
	ErrInvalidField = errors.New("DSL 解析错误: 未知字段")
)
