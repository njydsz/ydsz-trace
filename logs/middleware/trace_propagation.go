// Package middleware 子文件：链路追踪传播中间件（V2.3-09，FR-1001）。
//
// 职责：
//   1. 入站请求提取 TraceID（支持 W3C traceparent / B3 / SkyWalking 三种头）
//   2. 注入 gin.Context，供业务代码与日志使用
//   3. 出站响应回写 TraceID，便于前端关联
//
// 支持的入站头：
//   - traceparent: W3C 标准（00-<32hex>-<16hex>-01）
//   - X-B3-TraceId: Zipkin B3
//   - sw8: SkyWalking
//   - X-Trace-Id: 自定义（pkg/trace 写入的格式）
//
// 设计：追踪是增强能力，任何解析失败都不影响请求处理。
package middleware

import (
	"context"
	"net/http"
	"strings"

	"ydsz-trace/pkg/trace"

	"github.com/gin-gonic/gin"
)

// contextTraceKey 在 gin.Context 中存储追踪载体的键。
const contextTraceKey = "trace_carrier"

// TraceMiddleware 提取并注入链路追踪信息。
func TraceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := buildTraceContext(c.Request)
		carrier := trace.FromContext(ctx)

		// 存 Carrier 而非 context，便于业务侧按需还原
		c.Set(contextTraceKey, carrier)

		// 出站响应回写 TraceID，便于前端与调用方关联
		if carrier.TraceID != "" {
			c.Header(trace.TraceIDHeader, carrier.TraceID)
		}

		c.Next()
	}
}

// CarrierFromGin 从 gin.Context 取出追踪载体。
//
// 未注入（中间件未启用）时返回空 Carrier，调用方应据此降级。
func CarrierFromGin(c *gin.Context) trace.Carrier {
	v, ok := c.Get(contextTraceKey)
	if !ok {
		return trace.Carrier{}
	}
	carrier, ok := v.(trace.Carrier)
	if !ok {
		return trace.Carrier{}
	}
	return carrier
}

// ContextFromGin 从 gin.Context 还原出携带追踪信息的标准 context。
func ContextFromGin(c *gin.Context) context.Context {
	if c == nil {
		return context.Background()
	}
	return CarrierFromGin(c).Context()
}

// buildTraceContext 从 HTTP 请求构造携带追踪信息的 context。
func buildTraceContext(req *http.Request) context.Context {
	if req == nil {
		return trace.WithTraceID(context.Background(), trace.GenerateTraceID())
	}

	// 按 W3C → B3 → SkyWalking → 自定义 顺序识别
	extractors := []func(*http.Request) (string, bool){
		extractW3CTraceID,
		extractB3TraceID,
		extractSkyWalkingTraceID,
		extractCustomTraceID,
	}

	for _, ex := range extractors {
		if id, ok := ex(req); ok {
			ctx := trace.WithTraceID(req.Context(), id)
			if spanID := extractSpanID(req); spanID != "" {
				ctx = trace.WithSpanID(ctx, spanID)
			}
			return ctx
		}
	}

	// 无任何追踪头 → 生成新 TraceID
	return trace.WithTraceID(req.Context(), trace.GenerateTraceID())
}

// extractW3CTraceID 从 W3C traceparent 头提取。
//
// 格式：00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
//       版本(2) - traceID(32hex) - parentSpanID(16hex) - flags(2)
func extractW3CTraceID(req *http.Request) (string, bool) {
	h := req.Header.Get("traceparent")
	if len(h) < 34 {
		return "", false
	}
	// 跳过版本前缀与第一个分隔符
	rest := strings.TrimPrefix(h, "00-")
	parts := strings.Split(rest, "-")
	if len(parts) < 2 {
		return "", false
	}
	traceID := parts[0]
	if len(traceID) != 32 || !isHex(traceID) {
		return "", false
	}
	return traceID, true
}

// extractB3TraceID 从 Zipkin B3 头提取。
func extractB3TraceID(req *http.Request) (string, bool) {
	// 单头形式
	if v := req.Header.Get("X-B3-TraceId"); v != "" {
		return normalizeTraceID(v)
	}
	// 多头形式（B3 单 header 变体）
	if v := req.Header.Get("b3"); v != "" {
		parts := strings.Split(v, "-")
		if len(parts) >= 2 {
			return normalizeTraceID(parts[0])
		}
	}
	return "", false
}

// extractSkyWalkingTraceID 从 SkyWalking sw8 头提取。
//
// sw8 格式：1-<base64(traceId)>-<base64(spanId)>-...
// 这里简化处理：取第一段并做 base64 解码。
func extractSkyWalkingTraceID(req *http.Request) (string, bool) {
	v := req.Header.Get("sw8")
	if v == "" {
		return "", false
	}
	parts := strings.Split(v, "-")
	if len(parts) < 2 || parts[0] != "1" {
		return "", false
	}
	decoded, ok := decodeBase64(parts[1])
	if !ok || decoded == "" {
		return "", false
	}
	// SkyWalking 的 traceId 形如 uid@host，取 @ 前部分
	if idx := strings.IndexByte(decoded, '@'); idx > 0 {
		decoded = decoded[:idx]
	}
	return decoded, true
}

// extractCustomTraceID 从自定义头提取。
func extractCustomTraceID(req *http.Request) (string, bool) {
	return normalizeTraceID(req.Header.Get(trace.TraceIDHeader))
}

// extractSpanID 提取 span 标识（多来源）。
func extractSpanID(req *http.Request) string {
	if v := req.Header.Get("X-B3-SpanId"); v != "" {
		return v
	}
	if v := req.Header.Get(trace.SpanIDHeader); v != "" {
		return v
	}
	return ""
}

// normalizeTraceID 校验并规整 TraceID。
//
// 接受 16/32 位 hex；不足时按原样返回（兼容短 ID 实现）。
func normalizeTraceID(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	if !isHex(v) {
		return "", false
	}
	return v, true
}

// isHex 判断字符串是否全为十六进制字符。
func isHex(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return len(s) > 0
}

// decodeBase64 解码 base64（仅支持标准与 URL 安全两种字母表）。
func decodeBase64(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	// 优先标准编码
	for _, enc := range []struct {
		pad    bool
		url    bool
	}{
		{pad: true, url: false},
		{pad: false, url: false},
		{pad: false, url: true},
	} {
		if decoded, ok := tryDecode(s, enc.pad, enc.url); ok {
			return decoded, true
		}
	}
	return "", false
}
