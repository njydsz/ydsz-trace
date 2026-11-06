// Package source 子文件：多行日志合并（V2.1 FR-104）。
//
// 将堆栈日志（stack trace）等多行记录合并为一个逻辑日志事件：
//   - 识别"起始行"（含时间戳/级别标识）与"续行"（缩进、异常堆栈）
//   - 续行合并到上一个起始行，用换行符连接
//   - 支持最大合并行数限制（避免内存爆炸）
package source

import (
	"regexp"
	"strings"
)

const (
	// maxMergedLines 单个逻辑事件最多合并的行数
	maxMergedLines = 100
	// maxMergedBytes 单个逻辑事件最大字节数
	maxMergedBytes = 64 * 1024
)

// multilineStartPattern 起始行匹配模式（含时间戳或级别标识）。
var multilineStartPattern = regexp.MustCompile(
	`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}|^\[(?:ERROR|WARN|INFO|DEBUG|FATAL)\]|^\s*(?:ERROR|WARN|INFO|DEBUG|FATAL)\s`,
)

// MultilineMerger 多行日志合并器。
type MultilineMerger struct {
	// buffer 当前累积的逻辑事件
	buffer []string
	// byteSize 当前累积的字节数
	byteSize int
}

// NewMultilineMerger 创建多行日志合并器。
func NewMultilineMerger() *MultilineMerger {
	return &MultilineMerger{}
}

// Feed 输入一行日志，返回合并后的完整事件（如果当前事件结束）。
//
// 返回值：
//   - event: 合并后的完整日志事件（可能包含多行）
//   - done: 是否已形成一个完整事件
func (m *MultilineMerger) Feed(line string) (event string, done bool) {
	trimmed := strings.TrimSpace(line)

	// 空行：作为续行处理（堆栈中的空行）
	if trimmed == "" {
		if len(m.buffer) > 0 {
			m.appendLine(line)
		}
		return "", false
	}

	// 判断是否为新的起始行
	isStart := multilineStartPattern.MatchString(trimmed)

	if isStart {
		// 新起始行：先输出上一个事件（如果有）
		prev := m.flush()

		// 开始新事件
		m.buffer = []string{line}
		m.byteSize = len(line)

		if prev != "" {
			return prev, true
		}
		return "", false
	}

	// 续行：追加到当前事件
	if len(m.buffer) > 0 {
		m.appendLine(line)
		return "", false
	}

	// 没有起始行的续行（如文件首行就是续行）：作为独立事件
	return line, true
}

// Flush 强制输出当前累积的事件（用于文件末尾）。
func (m *MultilineMerger) Flush() string {
	return m.flush()
}

// appendLine 追加一行到当前事件。
func (m *MultilineMerger) appendLine(line string) {
	// 检查限制
	if len(m.buffer) >= maxMergedLines || m.byteSize >= maxMergedBytes {
		// 超限：先输出当前事件，再开始新事件
		m.flush()
	}
	m.buffer = append(m.buffer, line)
	m.byteSize += len(line)
}

// flush 输出并清空当前事件。
func (m *MultilineMerger) flush() string {
	if len(m.buffer) == 0 {
		return ""
	}
	event := strings.Join(m.buffer, "\n")
	m.buffer = nil
	m.byteSize = 0
	return event
}

// MergeLines 批量合并多行日志（静态工具方法）。
//
// 输入原始日志行列表，输出合并后的逻辑事件列表。
func MergeLines(lines []string) []string {
	merger := NewMultilineMerger()
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		event, done := merger.Feed(line)
		if done && event != "" {
			result = append(result, event)
		}
	}

	// 处理末尾残留
	if tail := merger.Flush(); tail != "" {
		result = append(result, tail)
	}

	return result
}
