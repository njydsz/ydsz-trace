package parser

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// FormatMatcher 内置日志格式匹配器。
// 每种常见日志格式一个 Matcher，按优先级排序后尝试匹配。
type FormatMatcher struct {
	// Name 格式名称
	Name string

	// Regex 匹配该格式的正则表达式
	Regex *regexp.Regexp

	// Extract 从正则匹配中提取命名字段
	Extract func(match []string) map[string]string

	// Priority 优先级，数字越大越优先
	Priority int
}

// builtinFormats 返回内置格式匹配器列表。
func builtinFormats() []FormatMatcher {
	return []FormatMatcher{
		formatJSON(),
		formatSpringBoot(),
		formatK8s(),
		formatDocker(),
		formatGoSlog(),
		formatLogfmt(),
		formatNginxCombined(),
	}
}

// formatJSON 匹配 JSON 格式日志。
// 特征：以 { 开头，顶层 key→value 全部作为字段。
func formatJSON() FormatMatcher {
	return FormatMatcher{
		Name:     "json",
		Priority: 100,
		Regex:    regexp.MustCompile(`^\s*\{.*\}\s*$`),
		Extract: func(match []string) map[string]string {
			var parsed map[string]string
			if err := json.Unmarshal([]byte(match[0]), &parsed); err == nil {
				return parsed
			}
			// 尝试解析为 interface{} 再扁平化
			var raw interface{}
			if jerr := json.Unmarshal([]byte(match[0]), &raw); jerr != nil {
				return nil
			}
			return flattenJSON(raw, "")
		},
	}
}

// flattenJSON 将嵌套 JSON 扁平化为字符串 map。
func flattenJSON(v interface{}, prefix string) map[string]string {
	result := make(map[string]string)
	switch val := v.(type) {
	case map[string]interface{}:
		for k, inner := range val {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			for ik, iv := range flattenJSON(inner, key) {
				result[ik] = iv
			}
		}
	case []interface{}:
		for i, item := range val {
			key := prefix + "[" + strconv.Itoa(i) + "]"
			for ik, iv := range flattenJSON(item, key) {
				result[ik] = iv
			}
		}
	default:
		if prefix != "" {
			result[prefix] = fmt.Sprintf("%v", val)
		}
	}
	return result
}

// formatSpringBoot 匹配 Spring Boot 默认日志格式。
// yyyy-MM-dd HH:mm:ss.SSS LEVEL [thread] logger : message
func formatSpringBoot() FormatMatcher {
	return FormatMatcher{
		Name:     "springboot",
		Priority: 90,
		Regex: regexp.MustCompile(
			`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d{3})\s+(DEBUG|INFO|WARN|WARNING|ERROR|FATAL|TRACE)\s+\[([^\]]+)\]\s+(\S+)\s*:\s*(.*)$`),
		Extract: func(match []string) map[string]string {
			if len(match) < 6 {
				return nil
			}
			return map[string]string{
				"timestamp": strings.TrimSpace(match[1]),
				"level":     normalizeLevel(strings.TrimSpace(match[2])),
				"thread":    strings.TrimSpace(match[3]),
				"logger":    strings.TrimSpace(match[4]),
				"message":   strings.TrimSpace(match[5]),
			}
		},
	}
}

// formatK8s 匹配 K8s kubelet/containerd 日志格式。
// 时间戳 F/P 组件名 消息
func formatK8s() FormatMatcher {
	return FormatMatcher{
		Name:     "k8s",
		Priority: 85,
		Regex: regexp.MustCompile(
			`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z)\s+(F|P)\s+\S+\s+(.*)$`),
		Extract: func(match []string) map[string]string {
			if len(match) < 4 {
				return nil
			}
			return map[string]string{
				"timestamp": strings.TrimSpace(match[1]),
				"level":     dockerLevel(match[2]),
				"message":   strings.TrimSpace(match[3]),
			}
		},
	}
}

// formatDocker 匹配 Docker/Containerd 容器日志格式。
// 2024-01-01T00:00:00.000000000Z stderr F message
func formatDocker() FormatMatcher {
	return FormatMatcher{
		Name:     "docker",
		Priority: 80,
		Regex: regexp.MustCompile(
			`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z)\s+(stdout|stderr)\s+(F|P)\s+(.*)$`),
		Extract: func(match []string) map[string]string {
			if len(match) < 5 {
				return nil
			}
			return map[string]string{
				"timestamp": strings.TrimSpace(match[1]),
				"stream":    strings.TrimSpace(match[2]),
				"level":     dockerLevel(match[3]),
				"message":   strings.TrimSpace(match[4]),
			}
		},
	}
}

// dockerLevel 将 Docker 日志段级别转为标准级别。
func dockerLevel(s string) string {
	switch strings.ToUpper(s) {
	case "F":
		return "FATAL"
	case "P":
		return "INFO"
	default:
		return "UNKNOWN"
	}
}

// formatGoSlog 匹配 Go log/slog / Zap 日志格式。
// LEVEL message key=value key=value...
func formatGoSlog() FormatMatcher {
	return FormatMatcher{
		Name:     "goslog",
		Priority: 70,
		Regex: regexp.MustCompile(
			`^(DEBUG|INFO|WARN|WARNING|ERROR|FATAL|TRACE)\s+(.+)$`),
		Extract: func(match []string) map[string]string {
			if len(match) < 3 {
				return nil
			}
			level := normalizeLevel(strings.TrimSpace(match[1]))
			fields := map[string]string{
				"level":   level,
				"message": strings.TrimSpace(match[2]),
			}
			// 从 message 末尾提取 key=value 对
			msg := strings.TrimSpace(match[2])
			kvPairs := parseLogfmtInline(msg)
			for k, v := range kvPairs {
				fields[k] = v
			}
			return fields
		},
	}
}

// formatLogfmt 匹配 Logfmt 格式日志。
// key=value key2=value2...
func formatLogfmt() FormatMatcher {
	return FormatMatcher{
		Name:     "logfmt",
		Priority: 60,
		Regex:    regexp.MustCompile(`^\s*\w+=[^\s]+(\s+\w+=[^\s]+)+\s*$`),
		Extract: func(match []string) map[string]string {
			fields := make(map[string]string)
			pairs := strings.Fields(match[0])
			for _, pair := range pairs {
				if idx := strings.Index(pair, "="); idx > 0 {
					key := pair[:idx]
					value := pair[idx+1:]
					fields[key] = value
				}
			}
			return fields
		},
	}
}

// formatNginxCombined 匹配 Nginx/Apache Combined Log Format。
func formatNginxCombined() FormatMatcher {
	return FormatMatcher{
		Name:     "nginx",
		Priority: 50,
		Regex: regexp.MustCompile(
			`^([\d\.a-fA-F:]+)\s+-\s+(\S+)\s+\[([^\]]+)\]\s+"([^"]+)"\s+(\d{3})\s+(\d+)\s+"([^"]*)"\s+"([^"]*)"`),
		Extract: func(match []string) map[string]string {
			if len(match) < 9 {
				return nil
			}
			fields := map[string]string{
				"ip":         strings.TrimSpace(match[1]),
				"user":       strings.TrimSpace(match[2]),
				"timestamp":  strings.TrimSpace(match[3]),
				"request":    strings.TrimSpace(match[4]),
				"statusCode": strings.TrimSpace(match[5]),
				"bytes":      strings.TrimSpace(match[6]),
				"referer":    strings.TrimSpace(match[7]),
				"userAgent":  strings.TrimSpace(match[8]),
			}
			// 从 request 提取 method/path
			parts := strings.SplitN(match[4], " ", 3)
			if len(parts) >= 2 {
				fields["method"] = parts[0]
				fields["path"] = parts[1]
			}
			return fields
		},
	}
}

// normalizeLevel 将日志级别统一为大写标准形式。
func normalizeLevel(level string) string {
	level = strings.ToUpper(level)
	switch level {
	case "WARNING":
		return "WARN"
	default:
		return level
	}
}

// parseLogfmtInline 从字符串中提取内联的 key=value 对。
func parseLogfmtInline(s string) map[string]string {
	fields := make(map[string]string)
	pairs := kvRegex.FindAllString(s, -1)
	for _, p := range pairs {
		if idx := strings.Index(p, "="); idx > 0 {
			fields[p[:idx]] = strings.Trim(p[idx+1:], "\"")
		}
	}
	return fields
}

// kvRegex 匹配内联的 key=value 对。
var kvRegex = regexp.MustCompile(`\b\w+=\"?[^"\s]+\"?`)

// MatchFormat 按优先级尝试所有内置格式，返回第一个匹配结果。
// 未匹配时 matched=false。
func MatchFormat(line string) (fields map[string]string, formatName string, matched bool) {
	formats := builtinFormats()
	sortFormatsByPriority(formats)
	for _, fm := range formats {
		m := fm.Regex.FindStringSubmatch(line)
		if m != nil {
			f := fm.Extract(m)
			return f, fm.Name, true
		}
	}
	return nil, "", false
}

// sortFormatsByPriority 按优先级降序排序（简单插入排序，格式数量少）。
func sortFormatsByPriority(formats []FormatMatcher) {
	for i := 1; i < len(formats); i++ {
		j := i
		for j > 0 && formats[j].Priority > formats[j-1].Priority {
			formats[j], formats[j-1] = formats[j-1], formats[j]
			j--
		}
	}
}
