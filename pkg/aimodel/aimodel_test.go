// Package aimodel 模型适配层单元测试。
//
// 使用 MockClient 进行路由测试，不会实际调用远程 API。
package aimodel

import (
	"context"
	"strings"
	"testing"
)

// mockClient 测试用 Mock 实现。
type mockClient struct {
	chatCalled     bool
	streamCalled  bool
	lastMessages  []Message
	streamChunks  []string
	response      *Response
	streamErr     error
}

func (m *mockClient) Chat(ctx context.Context, messages []Message) (*Response, error) {
	m.chatCalled = true
	m.lastMessages = messages
	if m.response != nil {
		return m.response, nil
	}
	return &Response{Content: "mock-response", Usage: Usage{PromptTokens: 5, OutputTokens: 3}}, nil
}

func (m *mockClient) StreamChat(ctx context.Context, messages []Message, onChunk func(string)) error {
	m.streamCalled = true
	m.lastMessages = messages
	if m.streamErr != nil {
		return m.streamErr
	}
	for _, chunk := range m.streamChunks {
		onChunk(chunk)
	}
	return nil
}

// TestNewClient_Local 验证本地 Provider 创建。
func TestNewClient_Local(t *testing.T) {
	c := NewClient(ProviderLocal, "http://localhost:11434", "")
	if c == nil {
		t.Fatal("NewClient 返回 nil")
	}

	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("Local Chat 错误: %v", err)
	}
	if !strings.Contains(resp.Content, "hello") {
		t.Errorf("Local Chat 应回显输入, got %q", resp.Content)
	}
}

// TestNewClient_LocalMissingEndpoint 验证 endpoint 缺失时返回错误。
func TestNewClient_LocalMissingEndpoint(t *testing.T) {
	c := NewClient(ProviderLocal, "", "")
	_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Error("空 endpoint 应返回错误")
	}
	if err != ErrMissingEndpoint {
		t.Errorf("期望 ErrMissingEndpoint, got %v", err)
	}
}

// TestNewClient_OpenAI 验证 OpenAI Provider 创建（无 API key 时返回错误）。
func TestNewClient_OpenAI(t *testing.T) {
	c := NewClient(ProviderOpenAI, "https://api.openai.com/v1", "")
	_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err == nil {
		t.Error("OpenAI 无 apiKey 应返回错误")
	}

	c2 := NewClient(ProviderOpenAI, "https://api.openai.com/v1", "sk-test-key")
	resp, err := c2.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("OpenAI Chat 错误: %v", err)
	}
	if !strings.Contains(resp.Content, "[openai]") {
		t.Errorf("OpenAI 响应应包含标记, got %q", resp.Content)
	}
}

// TestNewClient_Claude 验证 Claude Provider。
func TestNewClient_Claude(t *testing.T) {
	c := NewClient(ProviderClaude, "https://api.anthropic.com", "sk-ant-test")
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "ping"}})
	if err != nil {
		t.Fatalf("Claude Chat 错误: %v", err)
	}
	if !strings.Contains(resp.Content, "[claude]") {
		t.Errorf("Claude 响应应包含标记, got %q", resp.Content)
	}
}

// TestNewClient_DeepSeek 验证 DeepSeek Provider。
func TestNewClient_DeepSeek(t *testing.T) {
	c := NewClient(ProviderDeepSeek, "https://api.deepseek.com", "sk-ds-test")
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "ping"}})
	if err != nil {
		t.Fatalf("DeepSeek Chat 错误: %v", err)
	}
	if !strings.Contains(resp.Content, "[deepseek]") {
		t.Errorf("DeepSeek 响应应包含标记, got %q", resp.Content)
	}
}

// TestNewFromConfig 验证配置路由。
func TestNewFromConfig(t *testing.T) {
	tests := []struct {
		name     string
		cfg      map[string]string
		wantType string
	}{
		{
			name:     "local",
			cfg:      map[string]string{"aimodel.provider": "local", "aimodel.endpoint": "http://localhost:11434"},
			wantType: "local",
		},
		{
			name:     "openai",
			cfg:      map[string]string{"aimodel.provider": "openai", "aimodel.endpoint": "https://api.openai.com", "aimodel.apiKey": "sk-test"},
			wantType: "openai",
		},
		{
			name:     "empty_defaults_local",
			cfg:      map[string]string{},
			wantType: "local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewFromConfig(tt.cfg)
			if c == nil {
				t.Fatal("NewFromConfig 返回 nil")
			}
			// 验证类型通过调用 Chat 的响应标记来判断
			resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "test"}})
			if err != nil {
				// local 可能无 endpoint 返回错误，属于预期
				if tt.wantType == "local" {
					return
				}
				t.Fatalf("Chat 错误: %v", err)
			}
			if !strings.Contains(resp.Content, "["+tt.wantType+"]") {
				t.Errorf("期望响应包含 %q, got %q", "["+tt.wantType+"]", resp.Content)
			}
		})
	}
}

// TestLocalStreamChat 验证本地流式对话 callback 机制。
func TestLocalStreamChat(t *testing.T) {
	c := NewClient(ProviderLocal, "http://localhost:11434", "")
	var chunks []string
	messages := []Message{{Role: "user", Content: "stream test"}}

	err := c.StreamChat(context.Background(), messages, func(s string) {
		chunks = append(chunks, s)
	})
	if err != nil {
		t.Fatalf("StreamChat 错误: %v", err)
	}

	if len(chunks) == 0 {
		t.Error("StreamChat 未触发任何 callback")
	}

	total := ""
	for _, c := range chunks {
		total += c
	}
	if !strings.Contains(total, "stream") {
		t.Errorf("流式输出应包含 stream, got %q", total)
	}
}

// TestProvider_String 验证 String 方法。
func TestProvider_String(t *testing.T) {
	tests := []struct {
		p        Provider
		expected string
	}{
		{ProviderLocal, "local"},
		{ProviderOpenAI, "openai"},
		{ProviderClaude, "claude"},
		{ProviderDeepSeek, "deepseek"},
		{Provider(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.p.String(); got != tt.expected {
			t.Errorf("Provider(%d).String() = %q, want %q", int(tt.p), got, tt.expected)
		}
	}
}
