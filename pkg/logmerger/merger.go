// Package logmerger 提供跨节点日志聚合、过滤、排序与统计。
//
// 典型流程：
//   1. 从多个 logc 并发拉取 zip 压缩的日志文件
//   2. parseZipLogs 解析每个 zip，得到 LogEntry 列表
//   3. MergeLogs 按时间排序 + 过滤
//   4. CalcStats 输出统计摘要
//
// 时间戳识别：支持常见日志前缀格式（ISO8601、02/Jan/2006、syslog 等）。
package logmerger

import (
	"archive/zip"
	"bufio"
	"bytes"
	"io"
	"sort"
	"strings"
	"time"
)

// LogEntry 合并后的日志条目，一条对应原始日志的每一行。
type LogEntry struct {
	// Line 原始日志行内容（未修改）
	Line string `json:"line"`
	// Timestamp 从行首提取的时间戳；无法解析时为零值
	Timestamp time.Time `json:"timestamp"`
	// SourceIP 产生该日志的 logc 节点 IP
	SourceIP string `json:"sourceIp"`
	// Severity 行级别：FATAL/ERROR/WARN/INFO/DEBUG/TRACE（默认 INFO）
	Severity string `json:"severity"`
}

// MergeOptions 合并过滤与排序选项。
type MergeOptions struct {
	// MaxLines 最大返回行数，0 表示不限制
	MaxLines int
	// AfterTime 下限时间（不含），零值表示不限制
	AfterTime time.Time
	// BeforeTime 上限时间（不含），零值表示不限制
	BeforeTime time.Time
	// Severity 仅返回等于该级别的日志，空表示不过滤
	Severity string
	// SortDesc 为 true 时按时间降序（最新在前）
	SortDesc bool
}

// MergeLogs 合并多个节点的日志并按时间排序。
//
// 参数：
//   - sources: map[ip]zip文件字节流，每个 zip 由对应 logc 节点生成
//   - opts: 过滤与排序选项
//
// 单个源解析失败会被跳过（不阻塞整体合并），返回合并后的 LogEntry 切片。
func MergeLogs(sources map[string][]byte, opts MergeOptions) ([]LogEntry, error) {
	var entries []LogEntry

	for ip, data := range sources {
		zipEntries, err := parseZipLogs(data, ip)
		if err != nil {
			continue // 跳过解析失败的源
		}
		entries = append(entries, zipEntries...)
	}

	// 过滤
	entries = filterEntries(entries, opts)

	// 排序
	sort.Slice(entries, func(i, j int) bool {
		if opts.SortDesc {
			return entries[i].Timestamp.After(entries[j].Timestamp)
		}
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	// 限制行数
	if opts.MaxLines > 0 && len(entries) > opts.MaxLines {
		entries = entries[:opts.MaxLines]
	}

	return entries, nil
}

// parseZipLogs 从 zip 文件中逐行解析日志，构建 LogEntry 切片。
//
// 跳过目录条目；单行最大 1MB（通过 Scanner.Buffer 设置）。
func parseZipLogs(zipData []byte, sourceIP string) ([]LogEntry, error) {
	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, err
	}

	var entries []LogEntry

	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(rc)
		buf := make([]byte, 1024*1024) // 1MB line buffer
		scanner.Buffer(buf, 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			entries = append(entries, LogEntry{
				Line:      line,
				Timestamp: extractTimestamp(line),
				SourceIP:  sourceIP,
				Severity:  extractSeverity(line),
			})
		}
		rc.Close()
	}

	return entries, nil
}

// filterEntries 按 MergeOptions 中的时间范围与级别过滤日志。
//
// 未设置的过滤条件（零值/空字符串）视为跳过对应维度。
func filterEntries(entries []LogEntry, opts MergeOptions) []LogEntry {
	var filtered []LogEntry
	for _, e := range entries {
		if !opts.AfterTime.IsZero() && e.Timestamp.Before(opts.AfterTime) {
			continue
		}
		if !opts.BeforeTime.IsZero() && e.Timestamp.After(opts.BeforeTime) {
			continue
		}
		if opts.Severity != "" && e.Severity != opts.Severity {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

// extractTimestamp 从行首尝试解析时间戳。
//
// 支持的格式：
//   - 2006-01-02 15:04:05
//   - 2006-01-02T15:04:05
//   - 2006/01/02 15:04:05
//   - 2006/01/02T15:04:05
//   - Jan _2 15:04:05（syslog）
//   - 02/Jan/2006:15:04:05（Apache）
//
// 仅取前 35 字符作前缀匹配；全部格式失败返回零值。
func extractTimestamp(line string) time.Time {
	// 常见日志前缀长度不超过 35 字符
	maxLen := 35
	if len(line) < maxLen {
		maxLen = len(line)
	}
	prefix := line[:maxLen]

	// 尝试解析常见时间格式
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006/01/02 15:04:05",
		"2006/01/02T15:04:05",
		"Jan _2 15:04:05",
		"02/Jan/2006:15:04:05",
	}

	for _, format := range formats {
		// 尝试不同长度
		for _, length := range []int{19, 20, 23, 24, 25, 26, 28, 31, 32} {
			if length > len(prefix) {
				break
			}
			if t, err := time.Parse(format, prefix[:length]); err == nil {
				return t
			}
		}
	}

	return time.Time{} // 无法解析则为零值
}

// extractSeverity 通过字符串包含判断日志级别。
//
// 优先级从高到低：FATAL > ERROR > WARN > INFO > DEBUG > TRACE。
// 无法识别默认返回 "INFO"。
func extractSeverity(line string) string {
	lineLower := strings.ToLower(line)
	switch {
	case strings.Contains(lineLower, "fatal"):
		return "FATAL"
	case strings.Contains(lineLower, "panic"):
		return "FATAL"
	case strings.Contains(lineLower, "error"):
		return "ERROR"
	case strings.Contains(lineLower, "warn"):
		return "WARN"
	case strings.Contains(lineLower, "info"):
		return "INFO"
	case strings.Contains(lineLower, "debug"):
		return "DEBUG"
	case strings.Contains(lineLower, "trace"):
		return "TRACE"
	default:
		return "INFO"
	}
}

// Stats 合并结果统计信息摘要。
type Stats struct {
	// TotalCount 合并后总条数
	TotalCount int `json:"totalCount"`
	// SourceCount 涉及的不同节点（IP）数量
	SourceCount int `json:"sourceCount"`
	// BySeverity 各级别条数，key 为 FATAL/ERROR/WARN/INFO/DEBUG/TRACE
	BySeverity map[string]int `json:"bySeverity"`
	// TimeRange 时间跨度
	TimeRange struct {
		// Earliest 最早时间戳（可能为零值）
		Earliest time.Time `json:"earliest"`
		// Latest 最新时间戳（可能为零值）
		Latest time.Time `json:"latest"`
	} `json:"timeRange"`
}

// CalcStats 计算合并结果的统计摘要（总数、级别分布、时间跨度）。
//
// 输入为空时返回 TotalCount=0 的空 Stats。
func CalcStats(entries []LogEntry) Stats {
	stats := Stats{
		TotalCount: len(entries),
		BySeverity: make(map[string]int),
	}

	if len(entries) == 0 {
		return stats
	}

	sources := make(map[string]bool)
	stats.TimeRange.Earliest = entries[0].Timestamp
	stats.TimeRange.Latest = entries[0].Timestamp

	for _, e := range entries {
		sources[e.SourceIP] = true
		stats.BySeverity[e.Severity]++

		if !e.Timestamp.IsZero() {
			if e.Timestamp.Before(stats.TimeRange.Earliest) {
				stats.TimeRange.Earliest = e.Timestamp
			}
			if e.Timestamp.After(stats.TimeRange.Latest) {
				stats.TimeRange.Latest = e.Timestamp
			}
		}
	}

	stats.SourceCount = len(sources)
	return stats
}

// closeReader 安全关闭 ReadCloser，忽略关闭错误（兼容接口占位）。
func closeReader(c io.Closer) {
	_ = c.Close()
}
