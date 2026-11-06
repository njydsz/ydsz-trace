// Package trace 提供链路追踪的采集端能力（V2.3-09，FR-1001~1003）。
//
// 对标能力：
//   TLog 通过字节码增强在 log4j/log4j2/logback 上自动追加 TraceId + SpanId，
//   性能损耗 0.01%。本项目用 Go 的 context.Context 实现等效能力，
//   并额外覆盖 goroutine / channel / HTTP client 三类传播场景
//   （TLog 明确强调的异步场景，Go 中同样常见）。
//
// 三件事：
//
//  1. 生成：生成符合 W3C Trace Context 的 TraceID（32 位 hex）
//  2. 注入：把 TraceID 写入日志行（便于日志检索时按 Trace 聚合）
//  3. 透传：跨 goroutine / channel / HTTP 请求自动携带
//
// 设计约束：
//   - 零依赖（不引入 OpenTelemetry，保持单二进制轻量）
//   - TraceID 格式与 W3C 一致，可被标准工具识别
//   - 传播失败不 panic（追踪是增强能力，不能影响业务）
package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync/atomic"
)

// 传播键常量。
const (
	// TraceIDHeader HTTP 头名称（W3C Trace Context 风格）
	TraceIDHeader = "X-Trace-Id"
	// SpanIDHeader 当前 span 标识（用于区分同一 trace 内的不同操作）
	SpanIDHeader = "X-Span-Id"
	// traceIDLen TraceID 字节长度（16 字节 = 32 hex 字符，与 W3C 一致）
	traceIDLen = 16
	// spanIDLen SpanID 字节长度（8 字节 = 16 hex 字符）
	spanIDLen = 8
)

// ─── context 键（私有类型，避免与外部键冲突） ───

type contextKey int

const (
	// traceIDKey TraceID 在 context 中的键
	traceIDKey contextKey = iota
	// spanIDKey SpanID 在 context 中的键
	spanIDKey
)

// ─── 生成 ───

// GenerateTraceID 生成新的 TraceID（32 位 hex，符合 W3C）。
func GenerateTraceID() string {
	buf := make([]byte, traceIDLen)
	if _, err := rand.Read(buf); err != nil {
		// 密码学随机不可用时退化为计数器（仍保证唯一性，仅牺牲不可预测性）
		return fallbackID(traceIDLen)
	}
	return hex.EncodeToString(buf)
}

// GenerateSpanID 生成新的 SpanID（16 位 hex）。
func GenerateSpanID() string {
	buf := make([]byte, spanIDLen)
	if _, err := rand.Read(buf); err != nil {
		return fallbackID(spanIDLen)
	}
	return hex.EncodeToString(buf)
}

// counter 退化 ID 生成器（仅 rand 失败时使用）。
var counter uint64

// fallbackID 用计数器构造兜底 ID。
func fallbackID(n int) string {
	v := atomic.AddUint64(&counter, 1)
	out := make([]byte, n)
	for i := n - 1; i >= 0; i-- {
		out[i] = byte(v)
		v >>= 8
	}
	return hex.EncodeToString(out)
}

// ─── context 注入与提取 ───

// WithTraceID 把 TraceID 注入 context（返回新 context，不改原值）。
func WithTraceID(ctx context.Context, traceID string) context.Context {
	if traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDKey, traceID)
}

// WithSpanID 把 SpanID 注入 context。
func WithSpanID(ctx context.Context, spanID string) context.Context {
	if spanID == "" {
		return ctx
	}
	return context.WithValue(ctx, spanIDKey, spanID)
}

// WithNewSpan 创建新的 span 并注入 context（spanID 变化，traceID 保持）。
func WithNewSpan(ctx context.Context) context.Context {
	return WithSpanID(ctx, GenerateSpanID())
}

// TraceIDFromContext 从 context 提取 TraceID；不存在返回 ("", false)。
func TraceIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	v, ok := ctx.Value(traceIDKey).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// SpanIDFromContext 从 context 提取 SpanID；不存在返回 ("", false)。
func SpanIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	v, ok := ctx.Value(spanIDKey).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// TraceInfoFromContext 一次性取出 TraceID 与 SpanID。
func TraceInfoFromContext(ctx context.Context) (traceID, spanID string) {
	traceID, _ = TraceIDFromContext(ctx)
	spanID, _ = SpanIDFromContext(ctx)
	return traceID, spanID
}

// ─── 日志注入 ───

// LogFields 返回可附加到日志行的追踪字段。
//
// 用法（结构化日志）：
//
//	fields := trace.LogFields(ctx)
//	log.Printf("[%s] %s", fields["traceId"], message)
//
// 未注入追踪信息时返回 nil（不污染日志）。
func LogFields(ctx context.Context) map[string]string {
	traceID, spanID := TraceInfoFromContext(ctx)
	if traceID == "" {
		return nil
	}
	fields := map[string]string{"traceId": traceID}
	if spanID != "" {
		fields["spanId"] = spanID
	}
	return fields
}

// DecorateLine 给日志行追加追踪标记。
//
// 输出格式：`[trace:abc123 span:def456] 原始日志`
// 该格式可被 pkg/source 的 trace 提取规则识别。
//
// 无追踪信息时原样返回。
func DecorateLine(ctx context.Context, line string) string {
	traceID, spanID := TraceInfoFromContext(ctx)
	if traceID == "" {
		return line
	}
	if spanID == "" {
		return fmt.Sprintf("[trace:%s] %s", traceID, line)
	}
	return fmt.Sprintf("[trace:%s span:%s] %s", traceID, spanID, line)
}

// ─── HTTP 传播 ───

// InjectHeader 把追踪信息写入 HTTP 请求头（出站）。
//
// 用于 HTTP client 调用下游服务时自动透传。
func InjectHeader(ctx context.Context, req *http.Request) {
	if req == nil {
		return
	}
	traceID, spanID := TraceInfoFromContext(ctx)
	if traceID != "" {
		req.Header.Set(TraceIDHeader, traceID)
	}
	if spanID != "" {
		req.Header.Set(SpanIDHeader, spanID)
	}
}

// ExtractHeader 从 HTTP 请求头提取追踪信息（入站）。
//
// 请求头无追踪信息时生成新的 TraceID。
// 返回携带追踪信息的 context，供业务处理使用。
func ExtractHeader(req *http.Request) context.Context {
	ctx := context.Background()
	if req == nil {
		return WithTraceID(ctx, GenerateTraceID())
	}

	traceID := req.Header.Get(TraceIDHeader)
	if traceID == "" {
		traceID = GenerateTraceID()
	}
	ctx = WithTraceID(ctx, traceID)

	if spanID := req.Header.Get(SpanIDHeader); spanID != "" {
		ctx = WithSpanID(ctx, spanID)
	}
	return ctx
}

// RoundTripper 包装 http.RoundTripper，自动注入追踪头。
type RoundTripper struct {
	// Base 底层 RoundTripper（为 nil 时用 http.DefaultTransport）
	Base http.RoundTripper
}

// RoundTrip 实现 http.RoundTripper。
func (rt *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("trace: nil request")
	}

	base := rt.Base
	if base == nil {
		base = http.DefaultTransport
	}

	// 从请求 context 提取追踪信息注入头
	if req.Context() != nil {
		InjectHeader(req.Context(), req)
	}

	return base.RoundTrip(req)
}

// Transport 返回可直接用于 http.Client 的传输层。
func Transport(base http.RoundTripper) http.RoundTripper {
	return &RoundTripper{Base: base}
}

// ─── goroutine 传播 ───

// Go 启动 goroutine 时透传当前 context。
//
// 用法：
//
//	trace.Go(ctx, func(ctx context.Context) {
//	    log.Println(trace.DecorateLine(ctx, "async work"))
//	})
//
// 注意：Go 的 goroutine 必须通过 context 传递追踪信息，
// 直接引用外部变量会导致竞态。本函数保证 ctx 被正确传入。
func Go(ctx context.Context, fn func(context.Context)) {
	if fn == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	go fn(ctx)
}

// Detach 从当前 context 中剥离取消/超时信号，但保留追踪信息。
//
// 用途：后台任务（如日志上报）不应随请求结束而被取消，
// 但仍需保留 TraceID 以便串联日志。
func Detach(ctx context.Context) context.Context {
	traceID, spanID := TraceInfoFromContext(ctx)
	base := context.Background()
	base = WithTraceID(base, traceID)
	base = WithSpanID(base, spanID)
	return base
}

// ─── channel 传播辅助 ───

// Carrier 可在 channel 中传递追踪信息的最小载体。
//
// 用法：
//
//	carrier := trace.FromContext(ctx)
//	ch <- carrier.WithPayload(data)
//	// 接收侧
//	ctx := trace.ContextFrom(carrier)
type Carrier struct {
	TraceID string
	SpanID  string
}

// FromContext 从 context 构造 Carrier。
func FromContext(ctx context.Context) Carrier {
	traceID, spanID := TraceInfoFromContext(ctx)
	return Carrier{TraceID: traceID, SpanID: spanID}
}

// Context 把 Carrier 还原为 context。
func (c Carrier) Context() context.Context {
	ctx := context.Background()
	ctx = WithTraceID(ctx, c.TraceID)
	ctx = WithSpanID(ctx, c.SpanID)
	return ctx
}

// Empty 判断 Carrier 是否不含追踪信息。
func (c Carrier) Empty() bool {
	return c.TraceID == "" && c.SpanID == ""
}

// CarrierWithPayload 追踪载体 + 业务负载（泛型）。
type CarrierWithPayload[T any] struct {
	Carrier Carrier
	Payload T
}

// WithPayload 把任意负载与追踪信息打包。
func WithPayload[T any](ctx context.Context, payload T) CarrierWithPayload[T] {
	return CarrierWithPayload[T]{Carrier: FromContext(ctx), Payload: payload}
}

// Context 还原出携带追踪信息的 context。
func (c CarrierWithPayload[T]) Context() context.Context {
	return c.Carrier.Context()
}
