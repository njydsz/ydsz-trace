package aimodel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// --- 测试用 mock 服务 ---

// ollamaMock 模拟 ollama /api/chat。
func ollamaMock(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// openAIMock 模拟 OpenAI 兼容 /v1/chat/completions。
func openAIMock(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// countingMock 返回可记录调用次数的 mock。
func countingMock(t *testing.T, status int, body string, calls *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// TestNewClient_LocalChat 本地模型走 ollama 协议。
func TestNewClient_LocalChat(t *testing.T) {
	srv := ollamaMock(t, http.StatusOK, `{"message":{"role":"assistant","content":"hello"},"prompt_eval_count":10,"eval_count":5}`)
	defer srv.Close()

	c := NewClient(ProviderLocal, srv.URL, "")
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Chat 错误: %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("content = %q want hello", resp.Content)
	}
	if resp.Usage.PromptTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

// TestNewClient_LocalMissingEndpoint 缺 endpoint 返回明确错误。
func TestNewClient_LocalMissingEndpoint(t *testing.T) {
	c := NewClient(ProviderLocal, "", "")
	_, err := c.Chat(context.Background(), nil)
	if err != ErrMissingEndpoint {
		t.Errorf("err = %v want ErrMissingEndpoint", err)
	}
}

// TestNewClient_OpenAIChat OpenAI 兼容协议。
func TestNewClient_OpenAIChat(t *testing.T) {
	body := `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":7,"completion_tokens":3}}`
	srv := openAIMock(t, http.StatusOK, body)
	defer srv.Close()

	c := NewClient(ProviderOpenAI, srv.URL, "test-key")
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Chat 错误: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("content = %q", resp.Content)
	}
	if resp.Usage.OutputTokens != 3 {
		t.Errorf("output tokens = %d", resp.Usage.OutputTokens)
	}
}

// TestNewClient_OpenAINoAPIKey 缺 apiKey 直接报错。
func TestNewClient_OpenAINoAPIKey(t *testing.T) {
	srv := openAIMock(t, http.StatusOK, `{}`)
	defer srv.Close()

	c := NewClient(ProviderOpenAI, srv.URL, "")
	_, err := c.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("应返回错误")
	}
}

// TestNewClient_OpenAIEndpointVariants endpoint 三种写法都能构造正确 URL。
func TestNewClient_OpenAIEndpointVariants(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://api.example.com", "https://api.example.com/v1/chat/completions"},
		{"https://api.example.com/v1", "https://api.example.com/v1/chat/completions"},
		{"https://api.example.com/v1/chat/completions", "https://api.example.com/v1/chat/completions"},
	}
	for _, c := range cases {
		cli := NewClient(ProviderOpenAI, c.in, "k").(*openaiClient)
		if got := cli.chatURL(); got != c.want {
			t.Errorf("endpoint %q → %q want %q", c.in, got, c.want)
		}
	}
}

// TestNewClient_ClaudeChat Claude 协议。
func TestNewClient_ClaudeChat(t *testing.T) {
	body := `{"content":[{"type":"text","text":"claude says"}],"usage":{"input_tokens":4,"output_tokens":2}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") == "" {
			t.Error("缺少 x-api-key 头")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient(ProviderClaude, srv.URL, "claude-key")
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Chat 错误: %v", err)
	}
	if resp.Content != "claude says" {
		t.Errorf("content = %q", resp.Content)
	}
}

// TestNewClient_DeepSeekChat DeepSeek 复用 OpenAI 协议。
func TestNewClient_DeepSeekChat(t *testing.T) {
	body := `{"choices":[{"message":{"role":"assistant","content":"deepseek ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`
	srv := openAIMock(t, http.StatusOK, body)
	defer srv.Close()

	c := NewClient(ProviderDeepSeek, srv.URL, "dk-key")
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Chat 错误: %v", err)
	}
	if resp.Content != "deepseek ok" {
		t.Errorf("content = %q", resp.Content)
	}
}

// TestNewClient_InvalidProvider 非法 Provider 返回 fallback。
func TestNewClient_InvalidProvider(t *testing.T) {
	c := NewClient(Provider(99), "x", "y")
	_, err := c.Chat(context.Background(), nil)
	if err != ErrInvalidProvider {
		t.Errorf("err = %v want ErrInvalidProvider", err)
	}
}

// --- 重试机制 ---

// TestRetry_SuccessAfterFailure 首次失败后重试成功。
func TestRetry_SuccessAfterFailure(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"recovered"}}],"usage":{}}`))
	}))
	defer srv.Close()

	c := NewClientWithOptions(ProviderOpenAI, srv.URL, "k", Options{
		MaxRetries:     2,
		RetryBaseDelay: time.Millisecond,
	})
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("应重试成功: %v", err)
	}
	if resp.Content != "recovered" {
		t.Errorf("content = %q", resp.Content)
	}
	if calls != 2 {
		t.Errorf("calls = %d want 2", calls)
	}
}

// TestRetry_4xxNotRetried 4xx 客户端错误不重试。
func TestRetry_4xxNotRetried(t *testing.T) {
	var calls int32
	srv := countingMock(t, http.StatusBadRequest, `{"error":"bad"}`, &calls)
	defer srv.Close()

	c := NewClientWithOptions(ProviderOpenAI, srv.URL, "k", Options{
		MaxRetries:     3,
		RetryBaseDelay: time.Millisecond,
	})
	_, err := c.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("应返回错误")
	}
	if calls != 1 {
		t.Errorf("4xx 不应重试, calls = %d want 1", calls)
	}
}

// TestRetry_Exhausted 重试耗尽后报错。
func TestRetry_Exhausted(t *testing.T) {
	var calls int32
	srv := countingMock(t, http.StatusServiceUnavailable, `{}`, &calls)
	defer srv.Close()

	c := NewClientWithOptions(ProviderOpenAI, srv.URL, "k", Options{
		MaxRetries:     2,
		RetryBaseDelay: time.Millisecond,
	})
	_, err := c.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("应返回错误")
	}
	if calls != 3 { // 首次 + 2 次重试
		t.Errorf("calls = %d want 3", calls)
	}
}

// --- Token 预算 ---

// TestBudget_Exceeded 超出预算返回错误。
func TestBudget_Exceeded(t *testing.T) {
	SetBudget(10)
	defer SetBudget(0)

	body := `{"choices":[{"message":{"content":"long"}}],"usage":{"completion_tokens":20}}`
	srv := openAIMock(t, http.StatusOK, body)
	defer srv.Close()

	c := NewClient(ProviderOpenAI, srv.URL, "k")
	_, err := c.Chat(context.Background(), nil)
	if err != ErrBudgetExceeded {
		t.Errorf("err = %v want ErrBudgetExceeded", err)
	}
}

// TestBudget_Accumulates 预算累计消耗。
func TestBudget_Accumulates(t *testing.T) {
	SetBudget(100)
	defer SetBudget(0)

	body := `{"choices":[{"message":{"content":"x"}}],"usage":{"completion_tokens":30}}`
	srv := openAIMock(t, http.StatusOK, body)
	defer srv.Close()

	c := NewClient(ProviderOpenAI, srv.URL, "k")
	if _, err := c.Chat(context.Background(), nil); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := c.Chat(context.Background(), nil); err != nil {
		t.Fatalf("second call: %v", err)
	}

	used, limit, enabled := BudgetUsage()
	if !enabled || limit != 100 || used != 60 {
		t.Errorf("usage = used:%d limit:%d enabled:%v", used, limit, enabled)
	}
}

// TestBudget_Disabled 预算为 0 时不限制。
func TestBudget_Disabled(t *testing.T) {
	SetBudget(0)

	body := `{"choices":[{"message":{"content":"x"}}],"usage":{"completion_tokens":99999}}`
	srv := openAIMock(t, http.StatusOK, body)
	defer srv.Close()

	c := NewClient(ProviderOpenAI, srv.URL, "k")
	if _, err := c.Chat(context.Background(), nil); err != nil {
		t.Fatalf("不应受预算限制: %v", err)
	}
}

// --- 超时 ---

// TestTimeout_ContextCancelled 调用方取消立即返回。
func TestTimeout_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	c := NewClientWithOptions(ProviderOpenAI, srv.URL, "k", Options{NoRetry: true})
	_, err := c.Chat(ctx, nil)
	if err == nil {
		t.Fatal("应返回超时错误")
	}
}

// --- 流式 ---

// TestStreamChat_OpenAI OpenAI SSE 流式解析。
func TestStreamChat_OpenAI(t *testing.T) {
	sse := "data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n" +
		"data: [DONE]\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sse))
	}))
	defer srv.Close()

	c := NewClient(ProviderOpenAI, srv.URL, "k")
	var got string
	err := c.StreamChat(context.Background(), []Message{{Role: "user", Content: "hi"}}, func(chunk string) {
		got += chunk
	})
	if err != nil {
		t.Fatalf("StreamChat 错误: %v", err)
	}
	if got != "Hello" {
		t.Errorf("got %q want Hello", got)
	}
}

// TestStreamChat_Ollama ollama 流式解析。
func TestStreamChat_Ollama(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"message\":{\"content\":\"a\"},\"done\":false}\n" +
			"{\"message\":{\"content\":\"b\"},\"done\":true}\n"))
	}))
	defer srv.Close()

	c := NewClient(ProviderLocal, srv.URL, "")
	var got string
	if err := c.StreamChat(context.Background(), nil, func(chunk string) { got += chunk }); err != nil {
		t.Fatalf("StreamChat 错误: %v", err)
	}
	if got != "ab" {
		t.Errorf("got %q want ab", got)
	}
}

// --- 消息格式转换 ---

// TestToClaudeMessages_SystemMerged system 消息被合并进首条 user 消息。
func TestToClaudeMessages_SystemMerged(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "be helpful"},
		{Role: "user", Content: "hi"},
	}
	out := toClaudeMessages(msgs)
	if len(out) != 1 {
		t.Fatalf("len = %d want 1", len(out))
	}
	if out[0].Role != "user" {
		t.Errorf("role = %q want user", out[0].Role)
	}
	if !contains(out[0].Content, "be helpful") || !contains(out[0].Content, "hi") {
		t.Errorf("content = %q", out[0].Content)
	}
}

// TestToOpenAIMessages 逐条转换。
func TestToOpenAIMessages(t *testing.T) {
	msgs := []Message{{Role: "user", Content: "a"}, {Role: "assistant", Content: "b"}}
	out := toOpenAIMessages(msgs)
	if len(out) != 2 || out[0].Content != "a" || out[1].Content != "b" {
		t.Errorf("out = %+v", out)
	}
}

// TestNewFromConfig 从配置 map 创建。
func TestNewFromConfig(t *testing.T) {
	c := NewFromConfig(map[string]string{
		"aimodel.provider": "openai",
		"aimodel.endpoint": "https://x",
		"aimodel.apiKey":   "k",
	})
	if _, ok := c.(*openaiClient); !ok {
		t.Errorf("type = %T want *openaiClient", c)
	}
}

// --- 辅助 ---

// contains 判断子串是否存在（避免引入 strings 到部分测试）。
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// TestParseProvider 字符串解析。
func TestParseProvider(t *testing.T) {
	cases := map[string]Provider{
		"local":    ProviderLocal,
		"openai":   ProviderOpenAI,
		"claude":   ProviderClaude,
		"deepseek": ProviderDeepSeek,
		"":         ProviderLocal,
		"unknown":  ProviderLocal,
	}
	for in, want := range cases {
		if got := parseProvider(in); got != want {
			t.Errorf("parseProvider(%q) = %v want %v", in, got, want)
		}
	}
}

// TestProviderString Provider 可读名。
func TestProviderString(t *testing.T) {
	if ProviderLocal.String() != "local" {
		t.Errorf("local = %q", ProviderLocal.String())
	}
	if ProviderOpenAI.String() != "openai" {
		t.Errorf("openai = %q", ProviderOpenAI.String())
	}
	if Provider(99).String() != "unknown" {
		t.Errorf("unknown = %q", Provider(99).String())
	}
}

// TestNoRetry_Explicit 显式禁用重试。
func TestNoRetry_Explicit(t *testing.T) {
	var calls int32
	srv := countingMock(t, http.StatusInternalServerError, `{}`, &calls)
	defer srv.Close()

	c := NewClientWithOptions(ProviderOpenAI, srv.URL, "k", Options{
		NoRetry:       true,
		RetryBaseDelay: time.Millisecond,
	})
	_, err := c.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("应返回错误")
	}
	if calls != 1 {
		t.Errorf("NoRetry 时不应重试, calls = %d want 1", calls)
	}
}

// TestNormalizeOptions 默认值填充。
func TestNormalizeOptions(t *testing.T) {
	cfg := normalizeOptions(Options{})
	if cfg.timeout != DefaultTimeout {
		t.Errorf("timeout = %v", cfg.timeout)
	}
	if cfg.maxRetries != DefaultMaxRetries {
		t.Errorf("maxRetries = %d", cfg.maxRetries)
	}
	if cfg.maxOutputTokens != DefaultMaxOutputTokens {
		t.Errorf("maxOutputTokens = %d", cfg.maxOutputTokens)
	}
}

// TestTruncateBody 长响应体截断。
func TestTruncateBody(t *testing.T) {
	long := string(make([]byte, 500))
	got := truncateBody([]byte(long))
	if len(got) > 250 {
		t.Errorf("len = %d", len(got))
	}
}

// TestChatWithRetry_RequestErrorNotRetried 请求构造错误不重试。
func TestChatWithRetry_RequestErrorNotRetried(t *testing.T) {
	calls := 0
	cfg := normalizeOptions(Options{MaxRetries: 3})
	_, err := chatWithRetry(context.Background(), cfg, func() ([]byte, int, error) {
		calls++
		return nil, 0, &requestError{err: &json.SyntaxError{}}
	})
	if err == nil {
		t.Fatal("应返回错误")
	}
	if calls != 1 {
		t.Errorf("requestError 不应重试, calls = %d", calls)
	}
}
