// Package logs 子文件：根因分析 LLM 客户端（FR-901~908）。
//
// 复用 t_model_capability 表中已启用的 chat 模型配置，构造 OpenAI 兼容的
// /v1/chat/completions 请求。当无可用模型或调用失败时，由 ReasoningAgent 降级
// 为基于 AnomalyEngine + TopPatterns 的统计结论（不报错）。
package logs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/util"
)

// 常量定义。
const (
	// agentLLMTimeout 单次 LLM 调用超时（秒）。
	agentLLMTimeout = 60 * time.Second
	// agentToolTimeout 单个工具执行超时（秒）。
	agentToolTimeout = 5 * time.Second
	// agentMaxSteps ReAct 最大推理步数。
	agentMaxSteps = 8
	// agentDefaultMaxTokens 单次调用最大输出 token 数。
	agentDefaultMaxTokens = 2048
	// agentDefaultTokenBudget 单次会话累计输出 token 硬上限（FR-908）。
	//
	// ReAct 循环最多 8 步，每步 2048 token 理论峰值 16384。
	// 设为 12000 留出余量，超出后立即终止循环并基于已有历史生成结论。
	agentDefaultTokenBudget = 12000
)

// LLMClient 封装对 chat 模型的调用。
//
// 复用 util.NewClientWithTimeout 的共享 HTTP 客户端，超时由调用方通过
// context 控制，便于 ReAct 循环统一降级。
type LLMClient struct {
	// model 当前使用的模型配置（空表示不可用）。
	model *models.LogModelCapability
	// httpClient 共享 HTTP 客户端。
	httpClient *http.Client
}

// llmChatMessage OpenAI 兼容 chat message。
type llmChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// llmChatRequest OpenAI 兼容 /v1/chat/completions 请求体。
type llmChatRequest struct {
	Model       string           `json:"model"`
	Messages    []llmChatMessage `json:"messages"`
	Temperature float64          `json:"temperature"`
	MaxTokens   int              `json:"max_tokens"`
}

// llmChatResponse OpenAI 兼容响应体（仅取 choices[0].message.content）。
type llmChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	// Usage token 消耗（用于会话级预算控制，FR-908）
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// NewLLMClient 从 t_model_capability 查询已启用的 chat 模型并构造客户端。
//
// 查询失败或无可用模型时返回的 client 中 model 为 nil，调用方应据此降级。
func NewLLMClient() *LLMClient {
	m := queryDefaultChatModel()
	return &LLMClient{
		model:      m,
		httpClient: util.NewClientWithTimeout(agentLLMTimeout),
	}
}

// queryDefaultChatModel 查询默认 chat 模型配置。
func queryDefaultChatModel() *models.LogModelCapability {
	if models.DB == nil {
		return nil
	}
	list, err := models.GetEnabledModelCapabilities("chat")
	if err != nil || len(list) == 0 {
		return nil
	}
	// 返回首个启用的 chat 模型
	m := list[0]
	return &m
}

// Available 返回 LLM 是否可用。
func (c *LLMClient) Available() bool {
	return c != nil && c.model != nil
}

// ModelName 返回当前模型名称（不可用时为空字符串）。
func (c *LLMClient) ModelName() string {
	if c == nil || c.model == nil {
		return ""
	}
	return c.model.ModelId
}

// Call 发送 chat 补全请求，返回模型输出文本。
//
// 失败时返回错误，由调用方决定降级策略。
func (c *LLMClient) Call(ctx context.Context, system, user string) (string, error) {
	out, _, err := c.CallWithUsage(ctx, system, user)
	return out, err
}

// CallWithUsage 与 Call 相同，但额外返回本次调用的输出 token 数。
//
// 供 ReAct 循环做 Token 预算控制（FR-908）：
//
//	totalTokens += usage
//	if totalTokens > budget { 终止循环 }
func (c *LLMClient) CallWithUsage(ctx context.Context, system, user string) (string, int, error) {
	if !c.Available() {
		return "", 0, fmt.Errorf("LLM 不可用：未配置启用的 chat 模型")
	}

	// 构造请求体
	reqBody := llmChatRequest{
		Model: c.model.ModelId,
		Messages: []llmChatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: c.model.Temperature,
		MaxTokens:   c.model.MaxTokens,
	}
	if reqBody.MaxTokens <= 0 {
		reqBody.MaxTokens = agentDefaultMaxTokens
	}
	if reqBody.Temperature <= 0 {
		reqBody.Temperature = 0.3
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("序列化 LLM 请求失败: %w", err)
	}

	url := resolveLLMEndpoint(c.model, "/v1/chat/completions")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", 0, fmt.Errorf("构造 LLM 请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.model.ApiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.model.ApiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", 0, fmt.Errorf("调用 LLM 失败: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("LLM 返回非 200 状态码: %d", resp.StatusCode)
	}

	var parsed llmChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", 0, fmt.Errorf("解析 LLM 响应失败: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", 0, fmt.Errorf("LLM 返回错误: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", 0, fmt.Errorf("LLM 响应为空")
	}

	usage := 0
	if parsed.Usage != nil {
		usage = parsed.Usage.CompletionTokens
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), usage, nil
}

// resolveLLMEndpoint 拼接 LLM API 端点。
func resolveLLMEndpoint(m *models.LogModelCapability, path string) string {
	base := m.BaseUrl
	if base == "" {
		switch m.Provider {
		case "openai":
			base = "https://api.openai.com"
		case "anthropic":
			base = "https://api.anthropic.com"
		case "ollama":
			base = "http://localhost:11434"
		default:
			base = "http://localhost:11434"
		}
	}
	if strings.HasSuffix(base, "/") {
		base = base[:len(base)-1]
	}
	return base + path
}
