package trace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGenerateTraceID 长度与格式。
func TestGenerateTraceID(t *testing.T) {
	id := GenerateTraceID()
	if len(id) != 32 {
		t.Errorf("len = %d want 32", len(id))
	}
	for _, r := range id {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Errorf("非 hex 字符: %q", r)
		}
	}
}

// TestGenerateTraceID_Unique 多次生成不重复。
func TestGenerateTraceID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := GenerateTraceID()
		if seen[id] {
			t.Fatalf("重复 ID: %s", id)
		}
		seen[id] = true
	}
}

// TestGenerateSpanID 长度。
func TestGenerateSpanID(t *testing.T) {
	if len(GenerateSpanID()) != 16 {
		t.Error("spanID 长度应为 16")
	}
}

// TestWithTraceID_RoundTrip 注入与提取。
func TestWithTraceID_RoundTrip(t *testing.T) {
	ctx := context.Background()
	ctx = WithTraceID(ctx, "abc123")

	got, ok := TraceIDFromContext(ctx)
	if !ok || got != "abc123" {
		t.Errorf("got %q ok=%v", got, ok)
	}
}

// TestWithTraceID_EmptyIgnored 空值不注入。
func TestWithTraceID_EmptyIgnored(t *testing.T) {
	ctx := WithTraceID(context.Background(), "")
	if _, ok := TraceIDFromContext(ctx); ok {
		t.Error("空 TraceID 不应被注入")
	}
}

// TestTraceIDFromContext_NilContext nil 安全。
func TestTraceIDFromContext_NilContext(t *testing.T) {
	if _, ok := TraceIDFromContext(nil); ok {
		t.Error("nil context 应返回 false")
	}
}

// TestWithNewSpan_SpanChangesTraceKeeps span 变化、trace 不变。
func TestWithNewSpan_SpanChangesTraceKeeps(t *testing.T) {
	ctx := WithTraceID(context.Background(), "trace-abc")
	ctx = WithNewSpan(ctx)

	traceID, _ := TraceIDFromContext(ctx)
	if traceID != "trace-abc" {
		t.Errorf("traceID 应保持不变: %q", traceID)
	}
	if _, ok := SpanIDFromContext(ctx); !ok {
		t.Error("spanID 应存在")
	}
}

// TestTraceInfoFromContext 同时取两个字段。
func TestTraceInfoFromContext(t *testing.T) {
	ctx := WithTraceID(context.Background(), "t1")
	ctx = WithSpanID(ctx, "s1")

	traceID, spanID := TraceInfoFromContext(ctx)
	if traceID != "t1" || spanID != "s1" {
		t.Errorf("got %q %q", traceID, spanID)
	}
}

// TestLogFields 字段提取。
func TestLogFields(t *testing.T) {
	ctx := WithTraceID(context.Background(), "t1")
	ctx = WithSpanID(ctx, "s1")

	fields := LogFields(ctx)
	if fields["traceId"] != "t1" || fields["spanId"] != "s1" {
		t.Errorf("fields = %v", fields)
	}
}

// TestLogFields_NoTrace nil 返回。
func TestLogFields_NoTrace(t *testing.T) {
	if got := LogFields(context.Background()); got != nil {
		t.Errorf("got %v want nil", got)
	}
}

// TestDecorateLine 日志行装饰。
func TestDecorateLine(t *testing.T) {
	ctx := WithTraceID(context.Background(), "abc123")
	got := DecorateLine(ctx, "original message")
	if !strings.Contains(got, "[trace:abc123]") {
		t.Errorf("got %q", got)
	}
	if !strings.Contains(got, "original message") {
		t.Errorf("原文应保留: %q", got)
	}
}

// TestDecorateLine_WithSpan 带 span。
func TestDecorateLine_WithSpan(t *testing.T) {
	ctx := WithTraceID(context.Background(), "abc123")
	ctx = WithSpanID(ctx, "def456")

	got := DecorateLine(ctx, "msg")
	if !strings.Contains(got, "span:def456") {
		t.Errorf("got %q", got)
	}
}

// TestDecorateLine_NoTrace 无追踪时原样返回。
func TestDecorateLine_NoTrace(t *testing.T) {
	line := "plain"
	if got := DecorateLine(context.Background(), line); got != line {
		t.Errorf("got %q want %q", got, line)
	}
}

// TestInjectHeader 出站头注入。
func TestInjectHeader(t *testing.T) {
	ctx := WithTraceID(context.Background(), "t1")
	ctx = WithSpanID(ctx, "s1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	InjectHeader(ctx, req)

	if req.Header.Get(TraceIDHeader) != "t1" {
		t.Errorf("trace header = %q", req.Header.Get(TraceIDHeader))
	}
	if req.Header.Get(SpanIDHeader) != "s1" {
		t.Errorf("span header = %q", req.Header.Get(SpanIDHeader))
	}
}

// TestInjectHeader_NilRequest nil 安全。
func TestInjectHeader_NilRequest(t *testing.T) {
	InjectHeader(context.Background(), nil) // 不应 panic
}

// TestExtractHeader 入站提取。
func TestExtractHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(TraceIDHeader, "incoming-trace")
	req.Header.Set(SpanIDHeader, "incoming-span")

	ctx := ExtractHeader(req)
	traceID, spanID := TraceInfoFromContext(ctx)
	if traceID != "incoming-trace" || spanID != "incoming-span" {
		t.Errorf("got %q %q", traceID, spanID)
	}
}

// TestExtractHeader_GeneratesWhenMissing 无头时生成新 ID。
func TestExtractHeader_GeneratesWhenMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := ExtractHeader(req)

	traceID, ok := TraceIDFromContext(ctx)
	if !ok || traceID == "" {
		t.Error("应生成 TraceID")
	}
}

// TestExtractHeader_NilRequest nil 安全。
func TestExtractHeader_NilRequest(t *testing.T) {
	ctx := ExtractHeader(nil)
	if _, ok := TraceIDFromContext(ctx); !ok {
		t.Error("nil 请求应生成 TraceID")
	}
}

// TestRoundTripper 端到端：context 注入到请求头。
func TestRoundTripper(t *testing.T) {
	var gotTrace string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTrace = r.Header.Get(TraceIDHeader)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := WithTraceID(context.Background(), "propagated")
	// 注意：客户端请求必须用 http.NewRequest；
	// httptest.NewRequest 会设置 RequestURI，导致 client.Do 报错。
	req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if rerr != nil {
		t.Fatalf("构造请求失败: %v", rerr)
	}

	client := &http.Client{Transport: Transport(nil)}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()

	if gotTrace != "propagated" {
		t.Errorf("下游收到的 trace = %q want propagated", gotTrace)
	}
}

// TestRoundTripper_NilRequest nil 请求报错。
func TestRoundTripper_NilRequest(t *testing.T) {
	_, err := (&RoundTripper{}).RoundTrip(nil)
	if err == nil {
		t.Error("nil 请求应返回错误")
	}
}

// TestGo goroutine 传播。
func TestGo(t *testing.T) {
	ctx := WithTraceID(context.Background(), "async-trace")

	got := make(chan string, 1)
	Go(ctx, func(c context.Context) {
		id, _ := TraceIDFromContext(c)
		got <- id
	})

	if id := <-got; id != "async-trace" {
		t.Errorf("goroutine 内 traceID = %q", id)
	}
}

// TestGo_NilFunc nil 安全。
func TestGo_NilFunc(t *testing.T) {
	Go(context.Background(), nil) // 不应 panic
}

// TestDetach 剥离取消信号但保留追踪。
func TestDetach(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ctx = WithTraceID(ctx, "keep-me")
	cancel()

	// 原 context 已取消
	if ctx.Err() == nil {
		t.Fatal("前提：context 应已取消")
	}

	detached := Detach(ctx)
	if detached.Err() != nil {
		t.Error("Detach 后不应被取消")
	}
	if id, _ := TraceIDFromContext(detached); id != "keep-me" {
		t.Errorf("traceID 应保留: %q", id)
	}
}

// TestCarrier Channel 载体往返。
func TestCarrier(t *testing.T) {
	ctx := WithTraceID(context.Background(), "c1")
	ctx = WithSpanID(ctx, "s1")

	c := FromContext(ctx)
	if c.TraceID != "c1" || c.SpanID != "s1" {
		t.Errorf("carrier = %+v", c)
	}

	restored := c.Context()
	if id, _ := TraceIDFromContext(restored); id != "c1" {
		t.Errorf("restored traceID = %q", id)
	}
}

// TestCarrier_Empty 空判断。
func TestCarrier_Empty(t *testing.T) {
	if !(Carrier{}).Empty() {
		t.Error("空 Carrier 应返回 true")
	}
	if (Carrier{TraceID: "x"}).Empty() {
		t.Error("非空 Carrier 应返回 false")
	}
}

// TestCarrierWithPayload 泛型载荷打包。
func TestCarrierWithPayload(t *testing.T) {
	ctx := WithTraceID(context.Background(), "ctx-trace")
	c := WithPayload(ctx, "payload-data")

	if c.Payload != "payload-data" {
		t.Errorf("payload = %q", c.Payload)
	}
	if c.Carrier.TraceID != "ctx-trace" {
		t.Errorf("carrier trace = %q", c.Carrier.TraceID)
	}
}

// TestTraceIDHeader 常量值。
func TestTraceIDHeader(t *testing.T) {
	if TraceIDHeader != "X-Trace-Id" {
		t.Errorf("header = %q", TraceIDHeader)
	}
}
