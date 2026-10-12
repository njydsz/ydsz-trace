// Package parser 日志模板解析引擎。
// 基于 Drain3 算法实现增量式在线模板挖掘，内置常见格式库加速首次解析。
package parser

import "errors"

// 错误哨兵：集中管理所有公开错误变量。
var (
	// ErrInvalidLine 表示输入日志行为空或仅包含空白字符
	ErrInvalidLine = errors.New("parser: invalid empty log line")

	// ErrInvalidWorkers 表示并发 workers 参数非法
	ErrInvalidWorkers = errors.New("parser: workers count must be >= 1")

	// ErrTemplateNotFound 表示未找到对应模板
	ErrTemplateNotFound = errors.New("parser: template not found")
)
