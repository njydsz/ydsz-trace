package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ydsz-trace/pkg/trace"

	"github.com/gin-gonic/gin"
)

// setupGin 创建带追踪中间件的 gin 测试引擎。
func setupGin() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TraceMiddleware())
	r.GET("/probe", func(c *gin.Context) {
		carrier := CarrierFromGin(c)
		c.JSON(http.StatusOK, gin.H{
			"traceId": carrier.TraceID,
			"spanId":  carrier.SpanID,
		})
	})
	return r
}

// TestTraceMiddleware_GeneratesWhenMissing 无头时生成 TraceID。
func TestTraceMiddleware_GeneratesWhenMissing(t *testing.T) {
	r := setupGin()
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if got := w.Header().Get(trace.TraceIDHeader); got == "" {
		t.Error("响应头应回写 TraceID")
	}
	if !containsStr(w.Body.String(), `"traceId"`) {
		t.Errorf("响应体应含 traceId: %s", w.Body.String())
	}
}

// TestTraceMiddleware_W3CExtracted W3C 头被识别。
func TestTraceMiddleware_W3CExtracted(t *testing.T) {
	r := setupGin()
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if got := w.Header().Get(trace.TraceIDHeader); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("traceID = %q", got)
	}
}

// TestTraceMiddleware_B3Extracted B3 头被识别。
func TestTraceMiddleware_B3Extracted(t *testing.T) {
	r := setupGin()
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("X-B3-TraceId", "80f198ee56343ba864fe8b2a57d3eff7")
	req.Header.Set("X-B3-SpanId", "e457b5a2e4d86bd1")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if got := w.Header().Get(trace.TraceIDHeader); got != "80f198ee56343ba864fe8b2a57d3eff7" {
		t.Errorf("traceID = %q", got)
	}
}

// TestTraceMiddleware_CustomHeader 自定义头被识别。
func TestTraceMiddleware_CustomHeader(t *testing.T) {
	r := setupGin()
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	// 必须是合法 hex（非 hex 会被判定为非法并重新生成）
	req.Header.Set(trace.TraceIDHeader, "a1b2c3d4e5f60718")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if got := w.Header().Get(trace.TraceIDHeader); got != "a1b2c3d4e5f60718" {
		t.Errorf("traceID = %q", got)
	}
}

// TestTraceMiddleware_InvalidCustomHeader 非法自定义头被重新生成。
func TestTraceMiddleware_InvalidCustomHeader(t *testing.T) {
	r := setupGin()
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set(trace.TraceIDHeader, "not-a-hex-id")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if got := w.Header().Get(trace.TraceIDHeader); got == "not-a-hex-id" || got == "" {
		t.Errorf("非法头应被替换为新生成的 ID, got %q", got)
	}
}

// TestExtractW3CTraceID 直接测试提取函数。
func TestExtractW3CTraceID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("traceparent", "00-abcdefabcdefabcdefabcdefabcdef12-00f067aa0ba902b7-01")

	id, ok := extractW3CTraceID(req)
	if !ok {
		t.Fatal("应识别")
	}
	if id != "abcdefabcdefabcdefabcdefabcdef12" {
		t.Errorf("id = %q", id)
	}
}

// TestExtractW3CTraceID_Invalid 非法格式不误判。
func TestExtractW3CTraceID_Invalid(t *testing.T) {
	cases := []string{
		"",
		"00-short-00f067aa0ba902b7-01",
		"00-zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz-00f067aa0ba902b7-01",
		"not-a-traceparent",
	}
	for _, h := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("traceparent", h)
		if _, ok := extractW3CTraceID(req); ok {
			t.Errorf("不应识别: %q", h)
		}
	}
}

// TestExtractB3TraceID B3 单头与 b3 变体。
func TestExtractB3TraceID(t *testing.T) {
	// 单头
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-B3-TraceId", "abc123def456abc1")
	if id, ok := extractB3TraceID(req); !ok || id != "abc123def456abc1" {
		t.Errorf("got %q %v", id, ok)
	}

	// b3 组合头
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("b3", "abc123def456abc1-00f067aa0ba902b7")
	if id, ok := extractB3TraceID(req2); !ok || id != "abc123def456abc1" {
		t.Errorf("got %q %v", id, ok)
	}
}

// TestNormalizeTraceID 校验。
func TestNormalizeTraceID(t *testing.T) {
	if _, ok := normalizeTraceID("xyz"); ok {
		t.Error("非 hex 不应通过")
	}
	if _, ok := normalizeTraceID(""); ok {
		t.Error("空值不应通过")
	}
}

// TestIsHex 十六进制判断。
func TestIsHex(t *testing.T) {
	valid := []string{"abc123", "ABCDEF", "0123456789abcdef"}
	for _, v := range valid {
		if !isHex(v) {
			t.Errorf("%q 应为 hex", v)
		}
	}
	invalid := []string{"", "xyz", "abc 123"}
	for _, v := range invalid {
		if isHex(v) {
			t.Errorf("%q 不应为 hex", v)
		}
	}
}

// TestDecodeBase64 多种编码变体。
func TestDecodeBase64(t *testing.T) {
	// "hello" 的 base64 = aGVsbG8=
	if got, ok := decodeBase64("aGVsbG8="); !ok || got != "hello" {
		t.Errorf("got %q %v", got, ok)
	}
	// 无填充
	if got, ok := decodeBase64("aGVsbG8"); !ok || got != "hello" {
		t.Errorf("got %q %v", got, ok)
	}
	// 非法
	if _, ok := decodeBase64("!!!"); ok {
		t.Error("非法输入不应成功")
	}
}

// TestCarrierFromGin_NotPresent 未注入时返回空。
func TestCarrierFromGin_NotPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	carrier := CarrierFromGin(c)
	if !carrier.Empty() {
		t.Errorf("未注入时应为空: %+v", carrier)
	}
}

// TestContextFromGin_Nil nil 安全。
func TestContextFromGin_Nil(t *testing.T) {
	// 不应 panic
	_ = ContextFromGin(nil)
}

// containsStr 子串判断。
func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
