package embedding

import (
	"context"
	"errors"
)

// Provider 后端类型。
type Provider int

const (
	// ProviderLocal 本地模型（默认，如 ollama）。
	ProviderLocal Provider = iota
	// ProviderOpenAI OpenAI API。
	ProviderOpenAI
	// ProviderClaude Claude API。
	ProviderClaude
)

// 默认常量。
const (
	// DefaultLocalEndpoint ollama 默认 endpoint。
	DefaultLocalEndpoint = "http://localhost:11434"
	// DefaultLocalModel PRD 推荐模型。
	DefaultLocalModel = "bge-m3"
	// DefaultDimension bge-m3 向量维度。
	DefaultDimension = 1024
)

// 错误哨兵。
var (
	// ErrInvalidProvider 不支持的 Provider 类型。
	ErrInvalidProvider = errors.New("embedding: 不支持的 Provider 类型")
	// ErrMissingEndpoint endpoint 未配置。
	ErrMissingEndpoint = errors.New("embedding: endpoint 未配置")
	// ErrMissingAPIKey API Key 未配置。
	ErrMissingAPIKey = errors.New("embedding: API Key 未配置")
	// ErrEmptyInput 输入文本为空。
	ErrEmptyInput = errors.New("embedding: 输入文本为空")
)

// Embedder 文本向量化接口。
//
// 接口层零外部依赖；HTTP 调用只在实现层内部完成。
type Embedder interface {
	// Embed 对单条文本生成向量。
	Embed(ctx context.Context, text string) ([]float32, error)
	// EmbedBatch 批量生成向量。
	EmbedBatch(ctx context.Context, texts [][]string) ([][]float32, error)
	// Dimension 返回向量维度。
	Dimension() int
}

// EmbedderOption functional options 模式。
type EmbedderOption func(*embedderConfig)

type embedderConfig struct {
	Endpoint  string
	APIKey    string
	Model     string
	Dimension int
}

// WithEndpoint 设置 API endpoint。
func WithEndpoint(addr string) EmbedderOption {
	return func(c *embedderConfig) {
		c.Endpoint = addr
	}
}

// WithAPIKey 设置 API 密钥（仅云端 Provider 使用）。
func WithAPIKey(key string) EmbedderOption {
	return func(c *embedderConfig) {
		c.APIKey = key
	}
}

// WithModel 设置模型名称。
func WithModel(model string) EmbedderOption {
	return func(c *embedderConfig) {
		c.Model = model
	}
}

// WithDimension 设置向量维度。
func WithDimension(dim int) EmbedderOption {
	return func(c *embedderConfig) {
		c.Dimension = dim
	}
}

// NewEmbedder 根据 Provider 创建 Embedder 实例。
//
// ProviderLocal 时 opts 可选（WithEndpoint、WithModel）。
// ProviderOpenAI/Claude 时 WithAPIKey 从环境变量读取优先。
func NewEmbedder(provider Provider, opts ...EmbedderOption) (Embedder, error) {
	cfg := &embedderConfig{
		Endpoint:  DefaultLocalEndpoint,
		Model:     DefaultLocalModel,
		Dimension: DefaultDimension,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	switch provider {
	case ProviderLocal:
		if cfg.Endpoint == "" {
			return nil, ErrMissingEndpoint
		}
		return newLocalEmbedder(cfg), nil
	case ProviderOpenAI:
		return newOpenaiEmbedder(cfg), nil
	case ProviderClaude:
		return newOpenaiEmbedder(cfg), nil
	default:
		return nil, ErrInvalidProvider
	}
}
