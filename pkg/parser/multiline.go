// Package parser 子文件：多行日志合并（V2.2-07，FR-104）。
//
// 背景：
//	Java 堆栈、Python traceback、Go panic 等异常日志天然跨多行，
//	若按单行处理会产生大量碎片条目，既影响检索也干扰模板聚类。
//	本模块把「主行 + 连续从属行」折叠为一个逻辑条目。
//
// 判定规则（任一命中即视为从属行）：
//  1. Java：`^\s+at\s+...`（堆栈帧）、`^\s+\.\.\.\s*\d+\s+more`（省略帧）、
//     `^\s*Caused by:`（根因链）、`^\s*Suppressed:`（抑制异常）
//  2. Python：`^\s+File "...", line \d+`、`^\s+\w+Error:` 续行、
//     `^\s+\^+`（指示符）
//  3. Go：`^\s+goroutine\s+\d+`、`^\s+[^\s]+\.go:\d+`、`^\s+created by`
//  4. 通用：以 tab 或 2+ 空格开头且上一行以 `\` 结尾（续行符）
//  5. 通用：以 tab 或 4+ 空格开头且不含时间戳（缩进续行）
//
// 设计要点：
//   - 纯函数实现，不持有状态，可安全并发调用
//   - 主行必须满足「行首有时间戳或日志级别」——否则整段都不做合并，
//     避免把无结构的缩进文本误折叠
//   - 合并后的条目保留主行时间戳，从属行追加到 Message 字段
package parser

import (
	"regexp"
	"strings"
)

// 从属行识别正则（预编译，包初始化时构建）。
var (
	// reJavaFrame Java 堆栈帧：    at com.example.Foo.bar(Foo.java:42)
	reJavaFrame = regexp.MustCompile(`^\s+at\s+[\w.$/]+\.[\w$]+\([\w.]+\.java:\d+\)`)
	// reJavaEllipsis Java 省略帧：    ... 36 more
	reJavaEllipsis = regexp.MustCompile(`^\s*\.{3}\s*\d+\s+more`)
	// reJavaCausedBy Java 根因链：    Caused by: java.lang.NullPointerException
	reJavaCausedBy = regexp.MustCompile(`^\s*Caused by:\s*\S`)
	// reJavaSuppressed Java 抑制异常：    Suppressed: ...
	reJavaSuppressed = regexp.MustCompile(`^\s*Suppressed:\s*\S`)
	// rePythonFrame Python 堆栈帧：    File "/x/y.py", line 12, in func
	rePythonFrame = regexp.MustCompile(`^\s+File\s+"[^"]+",\s*line\s+\d+`)
	// rePythonCaret Python 指示符：    ^
	rePythonCaret = regexp.MustCompile(`^\s*\^+`)
	// rePythonTracebackHeader traceback 起始行（属于 Python 调用栈的一部分）
	rePythonTracebackHeader = regexp.MustCompile(`^Traceback \(most recent call last\):`)
	// rePythonErrorLine Python 异常类型行（末尾无时间戳的续行）
	rePythonErrorLine = regexp.MustCompile(`^(?:[A-Za-z_][\w.]*Error|Exception|SystemExit|KeyboardInterrupt|StopIteration|GeneratorExit)\b.*:.*$`)
	// reGoFrame Go 堆栈帧：    main.main()\t/x/main.go:10 +0x1a
	// 同时匹配 tab 缩进的源文件行（Go 栈帧标准形式）
	reGoFrame = regexp.MustCompile(`^\s*\S.*\.go:\d+`)
	// reGoFuncLine Go 函数调用行：    main.main()  /  net/http.(*Server).Serve()
	// 紧跟在 goroutine 头之后的调用栈行没有 .go: 后缀，需单独识别
	reGoFuncLine = regexp.MustCompile(`^\s*[\w.$/()]+\.[\w$]+\(`)
	// reGoCreatedBy Go 协程创建点：    created by ...
	reGoCreatedBy = regexp.MustCompile(`^\s*created by\s+\S`)
	// reGoGoroutine Go 协程头：    goroutine 1 [running]:
	reGoGoroutine = regexp.MustCompile(`^\s*goroutine\s+\d+`)
)

// MultiLineEntry 合并后的多行日志条目。
type MultiLineEntry struct {
	// Primary 主行原文（含时间戳/级别的那一行）
	Primary string
	// Continuation 从属行原文列表（不含主行）
	Continuation []string
	// Kind 合并类型：java-stack / python-traceback / go-panic / generic-indent / single
	Kind string
	// MergedMessage 合并后的完整消息（主行 + 从属行，换行连接）
	MergedMessage string
}

// LineCount 返回条目占用的原始行数。
func (e MultiLineEntry) LineCount() int {
	return 1 + len(e.Continuation)
}

// 合并类型常量。
const (
	// KindSingle 单行（未合并）
	KindSingle = "single"
	// KindJavaStack Java 堆栈
	KindJavaStack = "java-stack"
	// KindPythonTraceback Python 调用栈
	KindPythonTraceback = "python-traceback"
	// KindGoPanic Go panic/协程栈
	KindGoPanic = "go-panic"
	// KindGenericIndent 通用缩进续行
	KindGenericIndent = "generic-indent"
)

// MergeMultiLine 将连续多行日志折叠为逻辑条目。
//
// 参数 lines 为按时间顺序的日志行切片；返回合并后的条目列表。
// 非从属行各自成为独立的 KindSingle 条目。
//
// 注意：本函数只做「结构折叠」，不做模板聚类（那是 Drain 的职责）。
// 折叠结果可直接交给 ParseBatch，也可单独用于展示。
func MergeMultiLine(lines []string) []MultiLineEntry {
	if len(lines) == 0 {
		return nil
	}

	entries := make([]MultiLineEntry, 0, len(lines))
	i := 0

	for i < len(lines) {
		line := lines[i]

		// 空行不参与合并，直接产出单行条目
		if strings.TrimSpace(line) == "" {
			entries = append(entries, MultiLineEntry{
				Primary:       line,
				Kind:          KindSingle,
				MergedMessage: line,
			})
			i++
			continue
		}

		// 主行必须像一条「真正的日志」：行首有时间戳或以级别标识开头。
		// 否则整段不合并，避免把无结构文本误折叠。
		if !looksLikePrimary(line) {
			entries = append(entries, MultiLineEntry{
				Primary:       line,
				Kind:          KindSingle,
				MergedMessage: line,
			})
			i++
			continue
		}

		kind := KindSingle
		cont := make([]string, 0, 4)
		j := i + 1

		for j < len(lines) {
			next := lines[j]

			// 遇到下一条主行 → 结束当前条目
			if looksLikePrimary(next) {
				break
			}

			k := classifyContinuation(next)
			if k == KindSingle {
				break
			}
			// 首条从属行决定条目类型；后续同类或通用缩进都可追加
			if kind == KindSingle {
				kind = k
			} else if k != kind && k != KindGenericIndent {
				// 类型切换（如 Java 栈中出现 Python 帧）视为新条目
				break
			}

			cont = append(cont, next)
			j++
		}

		entry := MultiLineEntry{
			Primary:       line,
			Continuation:  cont,
			Kind:          kind,
			MergedMessage: joinLines(append([]string{line}, cont...)),
		}
		entries = append(entries, entry)
		i = j
	}

	return entries
}

// looksLikePrimary 判断一行是否像「主行」（而非从属行）。
//
// 策略：行首（忽略前导空白后）必须匹配以下任一模式：
//   - 时间戳：2026-11-03 10:00:00 / 2026-11-03T10:00:00 / 1700000000
//   - 日志级别：INFO / ERROR / WARN / DEBUG / FATAL / TRACE（可带方括号）
//   - JSON 起始：{
func looksLikePrimary(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return false
	}
	return hasLeadingTimestamp(trimmed) || hasLeadingLevel(trimmed) || strings.HasPrefix(trimmed, "{")
}

// hasLeadingTimestamp 判断字符串是否以时间戳开头。
func hasLeadingTimestamp(s string) bool {
	// 宽松匹配：前 10 个字符形如 YYYY-MM-DD 或数字串长度 >= 10
	if len(s) < 10 {
		return false
	}
	// YYYY-MM-DD
	if s[4] == '-' && s[7] == '-' && isAllDigits(s[:4]) {
		return true
	}
	// ISO: YYYY-MM-DDTHH...
	if s[4] == '-' && s[7] == '-' && s[10] == 'T' {
		return true
	}
	// Unix 时间戳（10 位）或毫秒（13 位）
	if len(s) >= 10 && isAllDigits(s[:10]) {
		return true
	}
	return false
}

// hasLeadingLevel 判断字符串是否以日志级别开头。
func hasLeadingLevel(s string) bool {
	levels := []string{"INFO", "ERROR", "WARN", "DEBUG", "FATAL", "TRACE", "WARNING"}
	for _, lv := range levels {
		if !strings.HasPrefix(s, lv) {
			continue
		}
		// 必须紧跟分隔符，避免把 "INFORMATION" 误判为 INFO
		rest := s[len(lv):]
		if rest == "" {
			return true
		}
		switch rest[0] {
		case ' ', '\t', ':', ']', '[', '-', ',':
			return true
		}
	}
	// [INFO] / [ERROR]
	if strings.HasPrefix(s, "[") {
		for _, lv := range levels {
			if strings.HasPrefix(s, "["+lv+"]") {
				return true
			}
		}
	}
	return false
}

// classifyContinuation 判定一行的从属类型；非从属行返回 KindSingle。
func classifyContinuation(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return KindSingle
	}

	switch {
	case rePythonTracebackHeader.MatchString(line):
		return KindPythonTraceback
	case rePythonErrorLine.MatchString(line):
		// 异常类型行：仅在上一行是 Python 栈时才算从属；此处保守识别为 Python 续行
		return KindPythonTraceback
	case reJavaFrame.MatchString(line):
		return KindJavaStack
	case reJavaEllipsis.MatchString(line):
		return KindJavaStack
	case reJavaCausedBy.MatchString(line):
		return KindJavaStack
	case reJavaSuppressed.MatchString(line):
		return KindJavaStack
	case rePythonFrame.MatchString(line):
		return KindPythonTraceback
	case rePythonCaret.MatchString(line):
		return KindPythonTraceback
	case reGoGoroutine.MatchString(line):
		return KindGoPanic
	case reGoFrame.MatchString(line):
		return KindGoPanic
	case reGoFuncLine.MatchString(line):
		return KindGoPanic
	case reGoCreatedBy.MatchString(line):
		return KindGoPanic
	}

	// 通用缩进续行：以 tab 或 2+ 空格开头，且不含时间戳/级别
	if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "  ") {
		if !hasLeadingTimestamp(trimmed) && !hasLeadingLevel(trimmed) {
			return KindGenericIndent
		}
	}

	return KindSingle
}

// joinLines 用换行符连接多行。
func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}

// isAllDigits 判断字符串是否全为数字。
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
