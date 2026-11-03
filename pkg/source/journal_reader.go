package source

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// journalEntry journalctl -o json 的单条记录。
//
// 仅声明需要的字段，其余（__REALTIME_TIMESTAMP 等）由 json 包自动忽略。
type journalEntry struct {
	Message   string `json:"MESSAGE"`
	Priority  string `json:"PRIORITY"`
	Hostname  string `json:"_HOSTNAME"`
	Identifier string `json:"SYSLOG_IDENTIFIER"`
	PID       string `json:"_PID"`
}

// journalReader 把 journalctl 的 JSON 流转为统一日志行文本流。
//
// 设计为 io.Reader，以便直接复用 ScanAndFilter 的过滤能力
// （关键字/级别/时间范围/上下文一次到位）。
type journalReader struct {
	scanner *bufio.Scanner
	buf     []byte
	err     error
}

// newJournalReader 包装 journalctl 输出流。
func newJournalReader(r io.Reader) *journalReader {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), MaxLineSize)
	return &journalReader{scanner: sc}
}

// Read 实现 io.Reader。
func (jr *journalReader) Read(p []byte) (int, error) {
	for len(jr.buf) == 0 {
		if jr.err != nil {
			return 0, jr.err
		}
		if !jr.scanner.Scan() {
			jr.err = io.EOF
			if serr := jr.scanner.Err(); serr != nil {
				jr.err = serr
			}
			return 0, jr.err
		}
		jr.buf = []byte(journalLineToText(jr.scanner.Bytes()))
		if len(jr.buf) == 0 {
			// 跳过无法解析的记录
			continue
		}
		jr.buf = append(jr.buf, '\n')
	}

	n := copy(p, jr.buf)
	jr.buf = jr.buf[n:]
	return n, nil
}

// journalLineToText 把单条 journal JSON 转为统一日志行。
//
// 输出格式：
//
//	<PRIORITY_NAME> <HOST> <IDENTIFIER>[<PID>]: <MESSAGE>
//
// 解析失败（非 JSON / 空 MESSAGE）时返回空串，由调用方跳过。
func journalLineToText(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}

	var e journalEntry
	if err := json.Unmarshal([]byte(trimmed), &e); err != nil {
		// 非 JSON（例如 journalctl 的警告行）→ 原样返回，保留可观测性
		return trimmed
	}
	if e.Message == "" {
		return ""
	}

	return formatJournalEntry(e)
}

// formatJournalEntry 格式化单条 journal 记录。
func formatJournalEntry(e journalEntry) string {
	var b strings.Builder

	if name := priorityName(e.Priority); name != "" {
		b.WriteString(name)
		b.WriteByte(' ')
	}
	if e.Hostname != "" {
		b.WriteString(e.Hostname)
		b.WriteByte(' ')
	}
	if e.Identifier != "" {
		b.WriteString(e.Identifier)
		if e.PID != "" {
			b.WriteByte('[')
			b.WriteString(e.PID)
			b.WriteByte(']')
		}
		b.WriteString(": ")
	}
	b.WriteString(e.Message)

	return b.String()
}

// priorityName 把 journald 数字优先级转为级别名。
//
// 映射关系（RFC 5424 / syslog 标准）：
//
//	0=EMERG 1=ALERT 2=CRIT 3=ERR 4=WARNING 5=NOTICE 6=INFO 7=DEBUG
func priorityName(p string) string {
	switch p {
	case "0":
		return "FATAL"
	case "1":
		return "FATAL"
	case "2":
		return "FATAL"
	case "3":
		return "ERROR"
	case "4":
		return "WARN"
	case "5":
		return "INFO"
	case "6":
		return "INFO"
	case "7":
		return "DEBUG"
	}
	return ""
}
