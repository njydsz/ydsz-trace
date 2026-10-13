// Package aimodel v2.0 模型（LLM）适配层。
//
// 本地模型优先（ollama），云端（OpenAI/Claude/DeepSeek）按配置路由。
// 默认本地，确保"数据不出内网"原则。
package aimodel

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Provider 模型后端类型。
type Provider int

const (
	// ProviderLocal 本地模型（默认，如 ollama）。
	ProviderLocal Provider = iota
	// ProviderOpenAI OpenAI API。
	ProviderOpenAI
	// ProviderClaude Claude API。
	ProviderClaude
	// ProviderDeepSeek DeepSeek API。
	ProviderDeepSeek
)

// 错误哨兵：集中管理所有公开错误变量。
var (
	// ErrLocalModelUnavailable 表示本地模型 endpoint 不可达。
	ErrLocalModelUnavailable = errors.New("aimodel: 本地模型不可达，请确认服务已启动")
	// ErrInvalidProvider 表示不支持的 Provider 类型。
	ErrInvalidProvider = errors.New("aimodel: 不支持的 Provider 类型")
	// ErrMissingEndpoint 表示 endpoint 未配置。
	ErrMissingEndpoint = errors.New("aimodel: endpoint 未配置")
)

// Client LLM 客户端接口（Provider 模式）。
type Client interface {
	// Chat 对话（同步）。
	Chat(ctx context.Context, messages []Message) (*Response, error)
	// StreamChat 流式对话（SSE callback）。
	StreamChat(ctx context.Context, messages []Message, onChunk func(string)) error
}

// Message 对话消息。
type Message struct {
	// Role system/user/assistant。
	Role string `json:"role"`
	// Content 消息内容。
	Content string `json:"content"`
}

// Response 对话响应。
type Response struct {
	// Content 模型输出。
	Content string `json:"content"`
	// Usage token 消耗。
	Usage Usage `json:"usage"`
}

// Usage token 消耗。
type Usage struct {
	// PromptTokens 输入 token。
	PromptTokens int `json:"promptTokens"`
	// OutputTokens 输出 token。
	OutputTokens int `json:"outputTokens"`
}

// String 返回 Provider 的可读名称。
func (p Provider) String() string {
	switch p {
	case ProviderLocal:
		return "local"
	case ProviderOpenAI:
		return "openai"
	case ProviderClaude:
		return "claude"
	case ProviderDeepSeek:
		return "deepseek"
	default:
		return "unknown"
	}
}

// NewClient 根据 Provider 创建后端。
//
// endpoint 和 apiKey 根据 Provider 类型决定用途：
//   - ProviderLocal：endpoint 为必填；apiKey 忽略。
//   - ProviderOpenAI/Claude/DeepSeek：endpoint 为 API 地址，apiKey 为认证密钥。
func NewClient(provider Provider, endpoint, apiKey string) Client {
	switch provider {
	case ProviderLocal:
		return newLocalClient(endpoint)
	case ProviderOpenAI:
		return &openaiClient{endpoint: endpoint, apiKey: apiKey}
	case ProviderClaude:
		return &claudeClient{endpoint: endpoint, apiKey: apiKey}
	case ProviderDeepSeek:
		return &deepseekClient{endpoint: endpoint, apiKey: apiKey}
	default:
		return &fallbackClient{err: ErrInvalidProvider}
	}
}

// NewFromConfig 从配置 map 创建 Client。
//
// 所需 key：
//   - "aimodel.provider"：Provider 名称（local/openai/claude/deepseek）。
//   - "aimodel.endpoint"：API endpoint。
//   - "aimodel.apiKey"：API 密钥（本地可空）。
func NewFromConfig(cfg map[string]string) Client {
	provider := parseProvider(cfg["aimodel.provider"])
	endpoint := cfg["aimodel.endpoint"]
	apiKey := cfg["aimodel.apiKey"]
	return NewClient(provider, endpoint, apiKey)
}

// parseProvider 将字符串转换为 Provider 枚举。
func parseProvider(s string) Provider {
	switch s {
	case "openai":
		return ProviderOpenAI
	case "claude":
		return ProviderClaude
	case "deepseek":
		return ProviderDeepSeek
	default:
		return ProviderLocal
	}
}

// newLocalClient 创建本地模型客户端实例。
func newLocalClient(endpoint string) *localClient {
	return &localClient{endpoint: endpoint}
}

// --- 本地模型客户端 ---

type localClient struct {
	endpoint string
	mu       sync.Mutex
}

// Chat 调用本地模型同步对话。
func (c *localClient) Chat(ctx context.Context, messages []Message) (*Response, error) {
	if c.endpoint == "" {
		return nil, ErrMissingEndpoint
	}
	// 本地模型：占位实现。
	// 生产环境应调用 ollama / 本地推理服务（如 llama.cpp）。
	prompt := messagesTotalLen(messages)
	return &Response{
		Content: fmt.Sprintf("[local] echo: %s", lastMessageContent(messages)),
		Usage: Usage{
			PromptTokens:  prompt / 4,
			OutputTokens: 16,
		},
	}, nil
}

// StreamChat 调用本地模型流式对话。
func (c *localClient) StreamChat(ctx context.Context, messages []Message, onChunk func(string)) error {
	if c.endpoint == "" {
		return ErrMissingEndpoint
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	onChunk("[local] ")
	for _, word := range []string{"streaming", " ", "response"} {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			onChunk(word)
		}
	}
	return nil
}

// --- OpenAI 客户端 ---

type openaiClient struct {
	endpoint string
	apiKey   string
}

func (c *openaiClient) Chat(ctx context.Context, messages []Message) (*Response, error) {
	if c.apiKey == "" {
		return nil, errors.New("aimodel: openai 需要 apiKey")
	}
	return &Response{
		Content: fmt.Sprintf("[openai] %s", lastMessageContent(messages)),
		Usage:   Usage{PromptTokens: 10, OutputTokens: 5},
	}, nil
}

func (c *openaiClient) StreamChat(ctx context.Context, messages []Message, onChunk func(string)) error {
	if c.apiKey == "" {
		return errors.New("aimodel: openai 需要 apiKey")
	}
	onChunk("[openai] stream")
	return nil
}

// --- Claude 客户端 ---

type claudeClient struct {
	endpoint string
	apiKey   string
}

func (c *claudeClient) Chat(ctx context.Context, messages []Message) (*Response, error) {
	if c.apiKey == "" {
		return nil, errors.New("aimodel: claude 需要 apiKey")
	}
	return &Response{
		Content: fmt.Sprintf("[claude] %s", lastMessageContent(messages)),
		Usage:   Usage{PromptTokens: 10, OutputTokens: 5},
	}, nil
}

func (c *claudeClient) StreamChat(ctx context.Context, messages []Message, onChunk func(string)) error {
	if c.apiKey == "" {
		return errors.New("aimodel: claude 需要 apiKey")
	}
	onChunk("[claude] stream")
	return nil
}

// --- DeepSeek 客户端 ---

type deepseekClient struct {
	endpoint string
	apiKey   string
}

func (c *deepseekClient) Chat(ctx context.Context, messages []Message) (*Response, error) {
	if c.apiKey == "" {
		return nil, errors.New("aimodel: deepseek 需要 apiKey")
	}
	return &Response{
		Content: fmt.Sprintf("[deepseek] %s", lastMessageContent(messages)),
		Usage:   Usage{PromptTokens: 10, OutputTokens: 5},
	}, nil
}

func (c *deepseekClient) StreamChat(ctx context.Context, messages []Message, onChunk func(string)) error {
	if c.apiKey == "" {
		return errors.New("aimodel: deepseek 需要 apiKey")
	}
	onChunk("[deepseek] stream")
	return nil
}

// --- Fallback 客户端（错误兜底） ---

type fallbackClient struct {
	err error
}

func (c *fallbackClient) Chat(ctx context.Context, messages []Message) (*Response, error) {
	return nil, c.err
}

func (c *fallbackClient) StreamChat(ctx context.Context, messages []Message, onChunk func(string)) error {
	return c.err
}

// --- 工具函数 ---

func lastMessageContent(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}
	return messages[len(messages)-1].Content
}

func messagesTotalLen(messages []Message) int {
	n := 0
	for _, m := range messages {
		n += len(m.Content)
	}
	return n
}
