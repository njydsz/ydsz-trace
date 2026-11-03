// Package embedding 子文件：Embedder 配置与工厂（V2.1 接通）。
//
// 职责：
//   - 从环境变量（YDSZ_EMBED_*）与数据库能力配置（t_model_capability）构建 Embedder
//   - 提供可达性探测（Probe），让调用方在启动期判断 L2 是否可用
//   - 未配置或不可达时返回 nil + 明确的 ErrUnavailable 语义，由上层降级
//
// 环境变量约定（前缀统一 YDSZ_）：
//   - YDSZ_EMBED_PROVIDER：local | openai | claude（默认 local）
//   - YDSZ_EMBED_ENDPOINT：服务地址（默认 http://localhost:11434）
//   - YDSZ_EMBED_MODEL：模型名（默认 bge-m3）
//   - YDSZ_EMBED_API_KEY：云端 Provider 的 API Key（缺省回退 YDSZ_OPENAI_KEY）
//   - YDSZ_EMBED_DIMENSION：向量维度（缺省按 Provider 内置默认）
//
// 说明：
//   - 当 YDSZ_EMBED_PROVIDER=local 且探测失败时，返回 nil（调用方降级为 L0+L1）。
//   - 探测使用极短超时（probeTimeout），避免启动阻塞。
package embedding

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// ErrUnavailable 表示 Embedder 不可用（未配置或探测失败）。
//
// 调用方应据此降级为规则 + 统计检索，而非直接报错给用户。
var ErrUnavailable = errors.New("embedding: L2 嵌入层不可用（未配置或后端不可达）")

// 环境变量名。
const (
	// EnvEmbedProvider Provider 类型。
	EnvEmbedProvider = "YDSZ_EMBED_PROVIDER"
	// EnvEmbedEndpoint 服务地址。
	EnvEmbedEndpoint = "YDSZ_EMBED_ENDPOINT"
	// EnvEmbedModel 模型名。
	EnvEmbedModel = "YDSZ_EMBED_MODEL"
	// EnvEmbedAPIKey 云端 API Key。
	EnvEmbedAPIKey = "YDSZ_EMBED_API_KEY"
	// EnvEmbedDimension 向量维度。
	EnvEmbedDimension = "YDSZ_EMBED_DIMENSION"
)

// 默认值。
const (
	// DefaultProviderName 默认 Provider 名称。
	DefaultProviderName = "local"
	// probeTimeout 探测超时（避免启动阻塞）。
	probeTimeout = 2 * time.Second
)

// ProviderFromName 将环境变量中的 Provider 名称解析为 Provider 常量。
//
// 无法识别时返回 ProviderLocal（保守降级到本地）。
func ProviderFromName(name string) Provider {
	switch name {
	case "openai":
		return ProviderOpenAI
	case "claude":
		return ProviderClaude
	case "local", "":
		return ProviderLocal
	default:
		return ProviderLocal
	}
}

// Probe 探测 Embedder 是否可用。
//
// 通过向量化一个固定短文本判断后端可达性；返回 nil 表示可用。
func Probe(ctx context.Context, e Embedder) error {
	if e == nil {
		return ErrUnavailable
	}
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	_, err := e.Embed(probeCtx, "connectivity probe")
	if err != nil {
		return fmt.Errorf("embedding: 探测失败: %w", err)
	}
	return nil
}

// NewFromEnv 从环境变量构建 Embedder。
//
// 返回值：
//   - embedder：构建成功时返回实例；未配置（Provider 为空且无 endpoint）时返回 nil
//   - provider：实际使用的 Provider
//   - err：构建失败（如云端 Provider 缺 API Key）
//
// 该函数不做网络探测，探测请调用 Probe。
func NewFromEnv() (Embedder, Provider, error) {
	provider := ProviderFromName(os.Getenv(EnvEmbedProvider))
	endpoint := os.Getenv(EnvEmbedEndpoint)
	model := os.Getenv(EnvEmbedModel)
	apiKey := os.Getenv(EnvEmbedAPIKey)
	if apiKey == "" {
		apiKey = os.Getenv("YDSZ_OPENAI_KEY")
	}
	dimStr := os.Getenv(EnvEmbedDimension)

	// 本地 Provider 未显式配置时，仍使用默认 endpoint（ollama 本地常驻）。
	opts := make([]EmbedderOption, 0, 4)
	if endpoint != "" {
		opts = append(opts, WithEndpoint(endpoint))
	}
	if model != "" {
		opts = append(opts, WithModel(model))
	}
	if apiKey != "" {
		opts = append(opts, WithAPIKey(apiKey))
	}
	if dimStr != "" {
		dim, err := strconv.Atoi(dimStr)
		if err != nil || dim <= 0 {
			return nil, provider, fmt.Errorf("embedding: %s 非法: %q", EnvEmbedDimension, dimStr)
		}
		opts = append(opts, WithDimension(dim))
	}

	e, err := NewEmbedder(provider, opts...)
	if err != nil {
		return nil, provider, fmt.Errorf("embedding: 构建失败: %w", err)
	}
	return e, provider, nil
}

// MustNewFromEnv 与 NewFromEnv 相同，但构建失败时返回 nil 而非 error。
//
// 供启动期调用方使用：构建失败等价于 L2 不可用，由降级路径处理。
func MustNewFromEnv() (Embedder, Provider) {
	e, provider, err := NewFromEnv()
	if err != nil {
		return nil, provider
	}
	return e, provider
}
