// Package parser 子文件：日志行 TraceID 提取（V2.3-09，FR-1002）。
//
// 职责：
//   从日志文本中识别 TraceID，供链路检索与聚合使用。
//   与 pkg/trace 配合：trace 负责生成与透传，本文件负责从文本识别。
//
// 支持的格式：
//
//	1. pkg/trace 自身格式：[trace:<id>] / [trace:<id> span:<id>]
//	2. 键值对：traceId=abc123 / trace_id: abc123 / traceId abc123
//	3. W3C：traceparent: 00-<32hex>-<16hex>-01
//	4. MDC 风格：[abc123,def456]（Zipkin 传统格式）
//
// 设计约束：
//   - 纯函数，无状态，可并发调用
//   - 只提取「看起来像 ID」的串，避免误报（长度 + hex 校验）
package parser

import (
	"regexp"
	"strings"
)

// TraceInfo 从日志行提取的追踪信息。
type TraceInfo struct {
	// TraceID 链路 ID
	TraceID string
	// SpanID 当前 span 标识（可能为空）
	SpanID string
	// Format 识别到的格式：trace_decorated / kv / w3c / mdc / unknown
	Format string
}

// 识别格式常量。
const (
	// TraceFormatDecorated pkg/trace 的 [trace:xxx] 格式
	TraceFormatDecorated = "trace_decorated"
	// TraceFormatKV 键值对格式（traceId=xxx）
	TraceFormatKV = "kv"
	// TraceFormatW3C W3C traceparent
	TraceFormatW3C = "w3c"
	// TraceFormatMDC Zipkin MDC 风格
	TraceFormatMDC = "mdc"
	// TraceFormatUnknown 未识别
	TraceFormatUnknown = "unknown"
)

// 预编译正则。
var (
	// reDecorated pkg/trace 写入的标记：[trace:abc span:def]
	reDecorated = regexp.MustCompile(`\[trace:([0-9a-fA-F]{8,64})(?:\s+span:([0-9a-fA-F]{8,64}))?\]`)
	// reTraceKV 键值对：traceId=xxx / trace_id: xxx / "traceId":"xxx"
	reTraceKV = regexp.MustCompile(`(?i)trace[_-]?id["']?\s*[:=]\s*["']?([0-9a-fA-F]{8,64})["']?`)
	// reW3C W3C traceparent
	reW3C = regexp.MustCompile(`00-([0-9a-fA-F]{32})-([0-9a-fA-F]{16})-[0-9a-fA-F]{2}`)
	// reMDC Zipkin MDC：[traceId,spanId]
	reMDC = regexp.MustCompile(`\[([0-9a-fA-F]{8,64}),\s*([0-9a-fA-F]{8,64})\]`)
	// reAnyHex 通用 hex 片段（用于兜底识别）
	reAnyHex = regexp.MustCompile(`[0-9a-fA-F]{16,64}`)
)

// ExtractTraceInfo 从日志行提取追踪信息。
//
// 未识别到 TraceID 时返回 (TraceInfo{}, false)。
// 识别到多种格式时按优先级返回第一个：decorated > kv > w3c > mdc。
func ExtractTraceInfo(line string) (TraceInfo, bool) {
	if line == "" {
		return TraceInfo{}, false
	}

	// 1. pkg/trace 自身格式（优先级最高：格式明确无歧义）
	if m := reDecorated.FindStringSubmatch(line); len(m) >= 2 {
		info := TraceInfo{TraceID: m[1], Format: TraceFormatDecorated}
		if len(m) >= 3 && m[2] != "" {
			info.SpanID = m[2]
		}
		return info, true
	}

	// 2. 键值对格式
	if m := reTraceKV.FindStringSubmatch(line); len(m) >= 2 {
		info := TraceInfo{TraceID: m[1], Format: TraceFormatKV}
		// 同一行可能还有 spanId
		if sm := regexp.MustCompile(`(?i)span[_-]?id\s*[:=]\s*["']?([0-9a-fA-F]{8,64})`).FindStringSubmatch(line); len(sm) >= 2 {
			info.SpanID = sm[1]
		}
		return info, true
	}

	// 3. W3C traceparent
	if m := reW3C.FindStringSubmatch(line); len(m) >= 3 {
		return TraceInfo{TraceID: m[1], SpanID: m[2], Format: TraceFormatW3C}, true
	}

	// 4. Zipkin MDC 风格
	if m := reMDC.FindStringSubmatch(line); len(m) >= 3 {
		return TraceInfo{TraceID: m[1], SpanID: m[2], Format: TraceFormatMDC}, true
	}

	return TraceInfo{}, false
}

// ExtractTraceID 便捷函数：只取 TraceID，忽略 SpanID 与格式。
func ExtractTraceID(line string) (string, bool) {
	info, ok := ExtractTraceInfo(line)
	if !ok {
		return "", false
	}
	return info.TraceID, true
}

// HasTraceInfo 判断日志行是否包含追踪信息。
func HasTraceInfo(line string) bool {
	_, ok := ExtractTraceInfo(line)
	return ok
}

// NormalizeTraceID 规整 TraceID（统一小写、去除空白）。
//
// 不同框架可能输出大写或带连字符的 ID，统一后便于聚合比较。
func NormalizeTraceID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

// LooksLikeTraceID 判断字符串是否形如 TraceID。
//
// 宽松校验：8~64 位 hex。用于从任意文本中猜测 ID。
func LooksLikeTraceID(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 8 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// GuessTraceID 从日志行猜测 TraceID（兜底策略）。
//
// 当前三种格式都未命中时，取行内第一个「看起来像 ID」的 hex 片段。
// 误报率较高，仅作为最后手段。
func GuessTraceID(line string) (string, bool) {
	m := reAnyHex.FindString(line)
	if m == "" {
		return "", false
	}
	if !LooksLikeTraceID(m) {
		return "", false
	}
	return m, true
}
