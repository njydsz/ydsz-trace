package parser

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 字段抽取正则表达式。
var (
	// reTimestamp 匹配常见时间戳格式
	reTimestamp = regexp.MustCompile(
		`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}(?:\.\d{3})?)`)

	// reISOTimestamp 匹配 ISO8601 时间戳
	reISOTimestamp = regexp.MustCompile(
		`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z`)

	// reLogLevel 匹配日志级别
	reLogLevel = regexp.MustCompile(
		`\b(DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL|TRACE)\b`)

	// reTraceID 匹配 32或16位十六进制 traceId
	reTraceID = regexp.MustCompile(
		`\b([0-9a-fA-F]{32}|[0-9a-fA-F]{16})\b`)

	// reSpanID 匹配16位十六进制 SpanId
	reSpanID = regexp.MustCompile(
		`\bspan[Ii][Dd][:=]\s*([0-9a-fA-F]{16})\b`)

	// reW3CTraceparent 匹配 W3C traceparent 格式
	reW3CTraceparent = regexp.MustCompile(
		`traceparent:?\s*([0-9a-f]{2})-([0-9a-f]{32})-([0-9a-f]{16})-([0-9a-f]{2})`)

	// reUserID 匹配 user_id 或 userId 模式
	reUserID = regexp.MustCompile(
		`\b(?:userId?|user[_-]?id|uid|account[_-]?id)[:=]\s*(\S+)\b`)

	// reHTTPStatus 匹配 HTTP 状态码
	reHTTPStatus = regexp.MustCompile(
		`\b([1-5]\d{2})\b`)

	// reDuration 匹配带单位的时间（ms/s/min）
	reDuration = regexp.MustCompile(
		`((?:\d+\.?\d*)\s*(?:ms|s|seconds?|minutes?|µs|ns))\b`)

	// reHTTPPath 匹配 HTTP 路径
	reHTTPPath = regexp.MustCompile(
		`\s(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\s+(\S+)`)

	// reIP 匹配 IPv4 地址
	reIPv4 = regexp.MustCompile(
		`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)

	// reServiceName 匹配服务名
	reServiceName = regexp.MustCompile(
		`\b(?:service|app|application)[:=]\s*(\S+)\b`)

	// reThreadName 匹配线程名 [xxx] 或 (xxx)
	reThreadName = regexp.MustCompile(
		`\[([^\]]{1,64})\]|\(([^)]{1,64})\)`)
)

// ExtractFields 从原始日志行中预抽取通用字段。
// 在模板匹配前调用，不影响模板匹配过程。
func ExtractFields(line string) map[string]string {
	fields := make(map[string]string)

	if ts := extractTimestamp(line); ts != "" {
		fields["timestamp"] = ts
	}
	if lvl := extractLevel(line); lvl != "" {
		fields["level"] = lvl
	}
	if tid := extractTraceID(line); tid != "" {
		fields["traceId"] = tid
	}
	if sid := extractSpanID(line); sid != "" {
		fields["spanId"] = sid
	}
	if uid := extractUserID(line); uid != "" {
		fields["userId"] = uid
	}
	if sc := extractStatusCode(line); sc != "" {
		fields["statusCode"] = sc
	}
	if d := extractDuration(line); d != "" {
		fields["duration"] = d
	}
	if u := extractURL(line); u != "" {
		fields["url"] = u
	}
	if ip := extractIP(line); ip != "" {
		fields["ip"] = ip
	}
	if svc := extractService(line); svc != "" {
		fields["service"] = svc
	}
	if thr := extractThread(line); thr != "" {
		fields["thread"] = thr
	}

	return fields
}

// extractTimestamp 从日志行提取时间戳并转为 Unix 毫秒字符串。
func extractTimestamp(line string) string {
	// ISO8601 格式
	if m := reISOTimestamp.FindString(line); m != "" {
		for _, layout := range []string{
			"2006-01-02T15:04:05.000000000Z",
			"2006-01-02T15:04:05.000Z",
			"2006-01-02T15:04:05.000000Z",
			"2006-01-02T15:04:05Z",
			time.RFC3339Nano,
			time.RFC3339,
		} {
			if t, err := time.Parse(layout, m); err == nil {
				return strconv.FormatInt(t.UnixMilli(), 10)
			}
		}
		return m
	}
	// 标准格式
	if m := reTimestamp.FindString(line); m != "" {
		layouts := []string{
			"2006-01-02 15:04:05.000",
			"2006-01-02 15:04:05",
		}
		for _, layout := range layouts {
			if t, err := time.Parse(layout, m); err == nil {
				return strconv.FormatInt(t.UnixMilli(), 10)
			}
		}
		return m
	}
	return ""
}

// extractLevel 提取日志级别。
func extractLevel(line string) string {
	m := reLogLevel.FindString(line)
	if m == "" {
		return ""
	}
	return normalizeLevel(m)
}

// extractTraceID 提取 traceId。
func extractTraceID(line string) string {
	// 优先匹配 W3C traceparent
	if m := reW3CTraceparent.FindStringSubmatch(line); len(m) > 2 {
		return m[2]
	}
	// 匹配 traceId=xxx 明确的 key=value 模式
	if m := regexp.MustCompile(`\btraceId?[=:]\s*([0-9a-fA-F]{16,32})\b`).FindStringSubmatch(line); len(m) > 1 {
		return m[1]
	}
	// 退而求其次：查找 32 或 16 位十六进制长串
	if m := reTraceID.FindString(line); m != "" {
		// 过滤掉看起来像 UUID 的（含 -）
		if !strings.Contains(m, "-") {
			return m
		}
	}
	return ""
}

// extractSpanID 提取 spanId。
func extractSpanID(line string) string {
	// W3C traceparent
	if m := reW3CTraceparent.FindStringSubmatch(line); len(m) > 3 {
		return m[3]
	}
	if m := reSpanID.FindStringSubmatch(line); len(m) > 1 {
		return m[1]
	}
	return ""
}

// extractUserID 提取用户ID。
func extractUserID(line string) string {
	if m := reUserID.FindStringSubmatch(line); len(m) > 1 {
		return strings.TrimRight(m[1], ",;")
	}
	return ""
}

// extractStatusCode 提取 HTTP 状态码。
func extractStatusCode(line string) string {
	m := reHTTPStatus.FindString(line)
	if m != "" {
		code, err := strconv.Atoi(m)
		if err == nil && code >= 100 && code <= 599 {
			return m
		}
	}
	return ""
}

// extractDuration 提取耗时。
func extractDuration(line string) string {
	m := reDuration.FindString(line)
	return strings.TrimSpace(m)
}

// extractURL 提取 HTTP 路径。
func extractURL(line string) string {
	if m := reHTTPPath.FindStringSubmatch(line); len(m) > 2 {
		return m[2]
	}
	return ""
}

// extractIP 提取 IP 地址。
func extractIP(line string) string {
	if m := reIPv4.FindString(line); m != "" {
		return m
	}
	return ""
}

// extractService 提取服务名。
func extractService(line string) string {
	if m := reServiceName.FindStringSubmatch(line); len(m) > 1 {
		return strings.TrimRight(m[1], ",")
	}
	return ""
}

// extractThread 提取线程名。
func extractThread(line string) string {
	m := reThreadName.FindStringSubmatch(line)
	if len(m) > 2 && m[2] != "" {
		return m[2]
	}
	if len(m) > 1 {
		return m[1]
	}
	return ""
}
