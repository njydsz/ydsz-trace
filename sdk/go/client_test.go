package ydsz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestServer 返回模拟服务端与指向它的客户端。
func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, "test-token")
	c.maxRetries = 0 // 测试默认不重试，避免拖慢用例
	return srv, c
}

// okHandler 返回标准成功响应。
func okHandler(t *testing.T, wantPath, wantMethod string, checkBody func(map[string]interface{})) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if wantPath != "" && r.URL.Path != wantPath {
			t.Errorf("path = %q want %q", r.URL.Path, wantPath)
		}
		if wantMethod != "" && r.Method != wantMethod {
			t.Errorf("method = %q want %q", r.Method, wantMethod)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("缺少 Authorization 头")
		}
		if checkBody != nil {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			checkBody(body)
		}
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"lines":["a","b"],"total":2}}`))
	}
}

// TestNewClient_TrimsTrailingSlash 去除尾部斜杠。
func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	c := NewClient("http://x/", "")
	if c.BaseURL() != "http://x" {
		t.Errorf("baseURL = %q", c.BaseURL())
	}
}

// TestNewClient_TokenFromEnv 从环境变量读取 token。
func TestNewClient_TokenFromEnv(t *testing.T) {
	t.Setenv("YDSZ_TOKEN", "env-token")
	c := NewClient("http://x", "")
	if c.Token() != "env-token" {
		t.Errorf("token = %q", c.Token())
	}
}

// TestNewClient_ExplicitTokenWins 显式参数优先于环境变量。
func TestNewClient_ExplicitTokenWins(t *testing.T) {
	t.Setenv("YDSZ_TOKEN", "env-token")
	c := NewClient("http://x", "explicit")
	if c.Token() != "explicit" {
		t.Errorf("token = %q", c.Token())
	}
}

// TestSearchLogs 检索日志。
func TestSearchLogs(t *testing.T) {
	_, c := newTestServer(t, okHandler(t, "/logs/search", http.MethodPost, func(b map[string]interface{}) {
		if b["item"].(float64) != 1 {
			t.Errorf("item = %v", b["item"])
		}
		if b["key"] != "timeout" {
			t.Errorf("key = %v", b["key"])
		}
	}))

	resp, err := c.SearchLogs(context.Background(), &SearchRequest{
		Item: 1, Date: "20261106", Key: "timeout", Level: "ERROR",
	})
	if err != nil {
		t.Fatalf("SearchLogs 失败: %v", err)
	}
	if len(resp.Lines) != 2 || resp.Total != 2 {
		t.Errorf("resp = %+v", resp)
	}
}

// TestGetContext 上下文回溯。
func TestGetContext(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/logs/context/") {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"before":["a"],"match":"m","after":["b"]}}`))
	})

	resp, err := c.GetContext(context.Background(), 42, &ContextRequest{Before: 5, After: 5})
	if err != nil {
		t.Fatalf("GetContext 失败: %v", err)
	}
	if resp.Match != "m" || len(resp.Before) != 1 || len(resp.After) != 1 {
		t.Errorf("resp = %+v", resp)
	}
}

// TestNl2DslParse 自然语言解析。
func TestNl2DslParse(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{
			"userIntent":"近3小时 ERROR","level":"ERROR","strategy":"rule","confidence":0.8}}`))
	})

	resp, err := c.Nl2DslParse(context.Background(), &Nl2DslRequest{Query: "近3小时 ERROR"})
	if err != nil {
		t.Fatalf("Nl2DslParse 失败: %v", err)
	}
	if resp.Level != "ERROR" || resp.Strategy != "rule" {
		t.Errorf("resp = %+v", resp)
	}
}

// TestSemanticSearch 语义检索。
func TestSemanticSearch(t *testing.T) {
	_, c := newTestServer(t, okHandler(t, "/api/v2/semantic/search", http.MethodPost, nil))

	resp, err := c.SemanticSearch(context.Background(), &SemanticSearchRequest{Query: "timeout", TopK: 5})
	if err != nil {
		t.Fatalf("SemanticSearch 失败: %v", err)
	}
	if resp.Results != nil {
		t.Error("mock 未返回 results，应为 nil")
	}
}

// TestListTemplates 模板列表。
func TestListTemplates(t *testing.T) {
	_, c := newTestServer(t, okHandler(t, "/logs/templates/search", http.MethodGet, nil))

	resp, err := c.ListTemplates(context.Background(), "1h", 10)
	if err != nil {
		t.Fatalf("ListTemplates 失败: %v", err)
	}
	_ = resp
}

// TestGetAnomalies 异常事件。
func TestGetAnomalies(t *testing.T) {
	_, c := newTestServer(t, okHandler(t, "/logs/anomalies/timeline", http.MethodGet, nil))

	if _, err := c.GetAnomalies(context.Background(), "24h", 20); err != nil {
		t.Fatalf("GetAnomalies 失败: %v", err)
	}
}

// TestReasonIncident 根因推理。
func TestReasonIncident(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{
			"hypothesis":"连接池耗尽","degraded":false,"totalTokens":120,"maxSteps":8}}`))
	})

	resp, err := c.ReasonIncident(context.Background(), &ReasonRequest{Query: "服务超时"})
	if err != nil {
		t.Fatalf("ReasonIncident 失败: %v", err)
	}
	if resp.Hypothesis != "连接池耗尽" || resp.TotalTokens != 120 {
		t.Errorf("resp = %+v", resp)
	}
}

// TestGetTrace 链路查询。
func TestGetTrace(t *testing.T) {
	_, c := newTestServer(t, okHandler(t, "/logs/trace", http.MethodPost, nil))

	if _, err := c.GetTrace(context.Background(), &TraceRequest{
		Item: 1, Date: "20261106", TraceID: "abc123",
	}); err != nil {
		t.Fatalf("GetTrace 失败: %v", err)
	}
}

// TestAPIError_BusinessCode 业务码非 0 返回 APIError。
func TestAPIError_BusinessCode(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":403,"message":"无权限","data":null,"traceId":"t1"}`))
	})

	_, err := c.SearchLogs(context.Background(), &SearchRequest{})
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("应为 APIError, got %T: %v", err, err)
	}
	if apiErr.Code != 403 || apiErr.TraceID != "t1" {
		t.Errorf("apiErr = %+v", apiErr)
	}
	if !strings.Contains(apiErr.Error(), "403") {
		t.Errorf("错误信息应包含 code: %q", apiErr.Error())
	}
}

// TestAPIError_HTTPStatus 非 200 状态码。
func TestAPIError_HTTPStatus(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":403,"message":"forbidden","data":null}`))
	})

	_, err := c.SearchLogs(context.Background(), &SearchRequest{})
	if _, ok := err.(*APIError); !ok {
		t.Fatalf("应为 APIError, got %T: %v", err, err)
	}
}

// TestRetry_SuccessAfterFailure 重试后成功。
func TestRetry_SuccessAfterFailure(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "t", WithMaxRetries(2), WithRetryBaseDelay(time.Millisecond))
	if _, err := c.SearchLogs(context.Background(), &SearchRequest{}); err != nil {
		t.Fatalf("应重试成功: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d want 2", calls)
	}
}

// TestRetry_4xxNotRetried 4xx 不重试。
func TestRetry_4xxNotRetried(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "t", WithMaxRetries(3), WithRetryBaseDelay(time.Millisecond))
	_, _ = c.SearchLogs(context.Background(), &SearchRequest{})
	if calls != 1 {
		t.Errorf("4xx 不应重试, calls = %d", calls)
	}
}

// TestContextCancelled context 取消立即返回。
func TestContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "t")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err := c.SearchLogs(ctx, &SearchRequest{})
	if err == nil {
		t.Fatal("应返回错误")
	}
}

// TestTrimSlash 辅助函数。
func TestTrimSlash(t *testing.T) {
	cases := map[string]string{
		"http://x":       "http://x",
		"http://x/":      "http://x",
		"http://x////":   "http://x",
		"":               "",
	}
	for in, want := range cases {
		if got := trimSlash(in); got != want {
			t.Errorf("trimSlash(%q) = %q want %q", in, got, want)
		}
	}
}
