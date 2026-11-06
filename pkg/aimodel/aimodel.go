// Package aimodel v2.0 模型（LLM）适配层。
//
// 本地模型优先（ollama），云端（OpenAI/Claude/DeepSeek）按配置路由。
// 默认本地，确保"数据不出内网"原则。
//
	// V2.3-01 状态：四类 Provider 均已接入真实 HTTP 推理接口，
	// 并落实超时 / 重试 / Token 预算三项硬约束（FR-908）。
	// 详见 client.go 与 providers.go。
package aimodel

import (
	"context"
	"errors"
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
	return NewClientWithOptions(provider, endpoint, apiKey, Options{})
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

// newLocalClient 创建本地模型客户端（真实 HTTP 实现）。
func newLocalClient(endpoint string, opts ...Options) *localClient {
	cfg := normalizeOptions(Options{})
	if len(opts) > 0 {
		cfg = normalizeOptions(opts[0])
	}
	return &localClient{endpoint: endpoint, cfg: cfg}
}

// NewClientWithOptions 带选项创建客户端（V2.3-01）。
//
// 与 NewClient 的区别在于可注入超时/重试/Token 预算等约束（FR-908）。
func NewClientWithOptions(provider Provider, endpoint, apiKey string, opts Options) Client {
	cfg := normalizeOptions(opts)
	switch provider {
	case ProviderLocal:
		return &localClient{endpoint: endpoint, apiKey: apiKey, cfg: cfg}
	case ProviderOpenAI:
		return &openaiClient{endpoint: endpoint, apiKey: apiKey, cfg: cfg}
	case ProviderClaude:
		return &claudeClient{endpoint: endpoint, apiKey: apiKey, cfg: cfg}
	case ProviderDeepSeek:
		return &deepseekClient{inner: &openaiClient{
			endpoint: endpoint, apiKey: apiKey,
			cfg: providerConfig{
				timeout:         cfg.timeout,
				maxRetries:      cfg.maxRetries,
				maxOutputTokens: cfg.maxOutputTokens,
				retryBaseDelay:  cfg.retryBaseDelay,
				model:           cfg.model,
			},
		}}
	default:
		return &fallbackClient{err: ErrInvalidProvider}
	}
}

// lastMessageContent 返回最后一条消息内容（供日志与调试使用）。
func lastMessageContent(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}
	return messages[len(messages)-1].Content
}

// messagesTotalLen 返回全部消息字符总数（用于粗估输入 token）。
func messagesTotalLen(messages []Message) int {
	n := 0
	for _, m := range messages {
		n += len(m.Content)
	}
	return n
}
