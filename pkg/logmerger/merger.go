// Package logmerger 提供跨节点日志聚合排序功能
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

// LogEntry 合并后的日志条目
type LogEntry struct {
	Line      string    `json:"line"`
	Timestamp time.Time `json:"timestamp"`
	SourceIP  string    `json:"sourceIp"`
	Severity  string    `json:"severity"` // ERROR/WARN/INFO/DEBUG
}

// MergeOptions 合并选项
type MergeOptions struct {
	MaxLines   int       // 最大返回行数（0=不限制）
	AfterTime  time.Time // 仅返回此时间之后的日志
	BeforeTime time.Time // 仅返回此时间之前的日志
	Severity   string    // 按级别过滤
	SortDesc   bool      // 降序（最新在前）
}

// MergeLogs 合并多个源的日志并按时间排序
// sources: map[ip]zip文件字节流
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

// parseZipLogs 从 zip 文件中解析日志行
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

// filterEntries 过滤日志条目
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

// extractTimestamp 尝试从日志行提取时间戳
// 支持常见格式：2006-01-02 15:04:05、2006/01/02 15:04:05、RFC3339 等
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

// extractSeverity 从日志行提取日志级别
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

// Stats 合并结果统计
type Stats struct {
	TotalCount  int            `json:"totalCount"`
	SourceCount int            `json:"sourceCount"`
	BySeverity  map[string]int `json:"bySeverity"`
	TimeRange   struct {
		Earliest time.Time `json:"earliest"`
		Latest   time.Time `json:"latest"`
	} `json:"timeRange"`
}

// CalcStats 计算合并结果统计信息
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

// Close 关闭 reader（兼容接口）
func closeReader(c io.Closer) {
	_ = c.Close()
}
