// Package perf 子文件：采样写入包装（V2.2-11，FR-105）。
//
// 背景：
//   Source 层不感知采样（它只负责读文件），而采样是 Agent 侧的策略。
//   此处提供一个 io.Writer 包装器：逐行判断是否保留，把被丢弃的行直接短路。
//
// 实现要点：
//   - 按行缓冲： Writer 可能被分片写入，需累积到换行才做决策
//   - 级别识别： 从行内提取日志级别（复用 parser 包的规则）
//   - 零分配快路径： sampler 未启用或采样率 1.0 时直接透传
package perf

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

// samplingWriter 按采样率丢弃低优先级日志行的 io.Writer。
type SamplingWriter struct {
	dst    io.Writer
	sampler *Sampler
	mu      sync.Mutex
	buf     []byte
}

// NewSamplingWriter 创建采样写入包装。
//
// sampler 为 nil 或采样率为 1.0 时直接返回原 writer（零开销快路径）。
func NewSamplingWriter(dst io.Writer, sampler *Sampler) io.Writer {
	if sampler == nil || sampler.Rate() >= 1.0 {
		return dst
	}
	return &SamplingWriter{dst: dst, sampler: sampler}
}

// Write 实现 io.Writer：累积到换行后按行采样。
func (w *SamplingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buf = append(w.buf, p...)

	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := w.buf[:idx]
		w.buf = w.buf[idx+1:]

		if w.keep(line) {
			if _, err := w.dst.Write(line); err != nil {
				return 0, err
			}
			if _, err := w.dst.Write([]byte{'\n'}); err != nil {
				return 0, err
			}
		}
	}

	return len(p), nil
}

// Flush 写出缓冲中剩余的不完整行。
func (w *SamplingWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.buf) == 0 {
		return nil
	}
	if w.keep(w.buf) {
		if _, err := w.dst.Write(w.buf); err != nil {
			return err
		}
	}
	w.buf = w.buf[:0]
	return nil
}

// keep 判断一行是否应保留。
func (w *SamplingWriter) keep(line []byte) bool {
	level := extractLevelFromLine(string(line))
	return w.sampler.ShouldKeep(level)
}

// extractLevelFromLine 从日志行中提取级别标识。
//
// 与 pkg/parser 的规则保持一致：INFO / ERROR / WARN / DEBUG / FATAL / TRACE，
// 可带方括号。提取失败返回空串（采样器会保守保留）。
//
// 支持行首时间戳：`2026-11-03 10:00:00 INFO ...` 会被正确识别。
func extractLevelFromLine(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return ""
	}

	// 跳过行首时间戳（YYYY-MM-DD 或 YYYY-MM-DDTHH:MM:SS）
	trimmed = stripLeadingTimestamp(trimmed)

	// [LEVEL] 形式
	if strings.HasPrefix(trimmed, "[") {
		if end := strings.IndexByte(trimmed, ']'); end > 1 && end <= 8 {
			candidate := strings.ToUpper(trimmed[1:end])
			if isLevelName(candidate) {
				return candidate
			}
			return ""
		}
	}

	// 行首 LEVEL 后跟分隔符
	for _, lv := range []string{"FATAL", "ERROR", "WARN", "WARNING", "INFO", "DEBUG", "TRACE"} {
		if !strings.HasPrefix(trimmed, lv) {
			continue
		}
		rest := trimmed[len(lv):]
		if rest == "" {
			return lv
		}
		switch rest[0] {
		case ' ', '\t', ':', ']', '-', ',':
			return lv
		}
	}

	// JSON 日志的 "level":"info"
	if idx := strings.Index(trimmed, `"level"`); idx >= 0 {
		if colon := strings.IndexByte(trimmed[idx:], ':'); colon > 0 {
			tail := strings.TrimSpace(trimmed[idx+colon+1:])
			tail = strings.Trim(tail, `"`)
			if end := strings.IndexAny(tail, `",}`); end > 0 {
				tail = tail[:end]
			}
			if isLevelName(strings.ToUpper(tail)) {
				return strings.ToUpper(tail)
			}
		}
	}

	return ""
}

// stripLeadingTimestamp 去除行首的时间戳前缀。
//
// 支持两种常见格式：
//   - `YYYY-MM-DD HH:MM:SS`（19 字符）
//   - `YYYY-MM-DDTHH:MM:SS`（19 字符，T 分隔）
//
// 只剥离日期后的时间部分；若日期后没有时间则只剥离日期。
func stripLeadingTimestamp(s string) string {
	if len(s) < 10 || s[4] != '-' || s[7] != '-' {
		return s
	}

	// 跳过日期（10 字符）
	rest := strings.TrimLeft(s[10:], " \t")

	// 日期后紧跟 T（ISO 格式）
	if strings.HasPrefix(rest, "T") {
		rest = strings.TrimLeft(rest[1:], " \t")
	}

	// 跳过 HH:MM:SS（8 字符，形如 10:00:00）
	if len(rest) >= 8 && rest[2] == ':' && rest[5] == ':' {
		rest = strings.TrimLeft(rest[8:], " \t")
	}

	return rest
}

// isLevelName 判断是否为合法级别名。
func isLevelName(s string) bool {
	switch s {
	case "FATAL", "ERROR", "WARN", "WARNING", "INFO", "DEBUG", "TRACE":
		return true
	}
	return false
}
