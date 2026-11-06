package aimodel

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ollamaChatRequest ollama /api/chat 请求体。
type ollamaChatRequest struct {
	Model    string         `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool           `json:"stream"`
	Options  ollamaOptions  `json:"options,omitempty"`
}

// ollamaMessage ollama 消息。
type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaOptions ollama 生成选项。
type ollamaOptions struct {
	NumPredict int `json:"num_predict,omitempty"`
}

// ollamaChatResponse ollama 非流式响应。
type ollamaChatResponse struct {
	Message ollamaMessage `json:"message"`
	// PromptEvalCount / EvalCount 为 ollama 自报 token 数
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount       int `json:"eval_count"`
}

// ollamaStreamChunk ollama 流式响应块。
type ollamaStreamChunk struct {
	Message ollamaMessage `json:"message"`
	Done    bool          `json:"done"`
}

// openAIChatRequest OpenAI 兼容请求体。
type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Stream      bool            `json:"stream,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
}

// openAIMessage OpenAI 消息。
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIResponse OpenAI 响应。
type openAIResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// claudeRequest Claude /v1/messages 请求体。
type claudeRequest struct {
	Model     string          `json:"model"`
	Messages  []claudeMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens"`
	Stream    bool            `json:"stream,omitempty"`
}

// claudeMessage Claude 消息。
type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeResponse Claude 响应。
type claudeResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// --- Provider 实现 ---

// localClient 本地模型（ollama）客户端。
type localClient struct {
	endpoint string
	apiKey   string
	cfg      providerConfig
}

// Chat 实现 Client。
func (c *localClient) Chat(ctx context.Context, messages []Message) (*Response, error) {
	if c.endpoint == "" {
		return nil, ErrMissingEndpoint
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	payload := ollamaChatRequest{
		Model:    c.modelName(),
		Messages: toOllamaMessages(messages),
		Stream:   false,
		Options:  ollamaOptions{NumPredict: c.cfg.maxOutputTokens},
	}

	body, err := chatWithRetry(reqCtx, c.cfg, func() ([]byte, int, error) {
		return doJSONRequest(reqCtx, normalizeEndpoint(c.endpoint)+"/api/chat",
			buildHeaders(c.apiKey), payload)
	})
	if err != nil {
		return nil, err
	}

	var resp ollamaChatResponse
	if uerr := json.Unmarshal(body, &resp); uerr != nil {
		return nil, fmt.Errorf("aimodel: 解析 ollama 响应失败: %w", uerr)
	}
	if resp.Message.Content == "" {
		return nil, fmt.Errorf("aimodel: ollama 返回空内容")
	}

	usage := Usage{PromptTokens: resp.PromptEvalCount, OutputTokens: resp.EvalCount}
	if err := consumeBudget(resp.EvalCount); err != nil {
		return nil, err
	}
	return &Response{Content: resp.Message.Content, Usage: usage}, nil
}

// StreamChat 实现 Client：调用 ollama 流式接口并逐块回调。
func (c *localClient) StreamChat(ctx context.Context, messages []Message, onChunk func(string)) error {
	if c.endpoint == "" {
		return ErrMissingEndpoint
	}

	payload := ollamaChatRequest{
		Model:    c.modelName(),
		Messages: toOllamaMessages(messages),
		Stream:   true,
		Options:  ollamaOptions{NumPredict: c.cfg.maxOutputTokens},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		normalizeEndpoint(c.endpoint)+"/api/chat", strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
		return fmt.Errorf("aimodel: HTTP %d: %s", resp.StatusCode, truncateBody(body))
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var chunk ollamaStreamChunk
		if uerr := json.Unmarshal([]byte(line), &chunk); uerr != nil {
			continue
		}
		if chunk.Message.Content != "" {
			onChunk(chunk.Message.Content)
		}
		if chunk.Done {
			return nil
		}
	}
	return sc.Err()
}

// modelName 返回配置的模型名。
func (c *localClient) modelName() string {
	if c.cfg.model != "" {
		return c.cfg.model
	}
	return "qwen2.5"
}

// openaiClient OpenAI 兼容客户端（同时服务 DeepSeek）。
type openaiClient struct {
	endpoint string
	apiKey   string
	cfg      providerConfig
}

// Chat 实现 Client。
func (c *openaiClient) Chat(ctx context.Context, messages []Message) (*Response, error) {
	if c.endpoint == "" {
		return nil, ErrMissingEndpoint
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("aimodel: openai 需要 apiKey")
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	payload := openAIChatRequest{
		Model:       c.modelName(),
		Messages:    toOpenAIMessages(messages),
		Stream:      false,
		MaxTokens:   c.cfg.maxOutputTokens,
		Temperature: 0.3,
	}

	body, err := chatWithRetry(reqCtx, c.cfg, func() ([]byte, int, error) {
		return doJSONRequest(reqCtx, c.chatURL(), buildHeaders(c.apiKey), payload)
	})
	if err != nil {
		return nil, err
	}

	var resp openAIResponse
	if uerr := json.Unmarshal(body, &resp); uerr != nil {
		return nil, fmt.Errorf("aimodel: 解析响应失败: %w", uerr)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("aimodel: 响应无 choices")
	}

	usage := Usage{PromptTokens: resp.Usage.PromptTokens, OutputTokens: resp.Usage.CompletionTokens}
	if err := consumeBudget(resp.Usage.CompletionTokens); err != nil {
		return nil, err
	}
	return &Response{Content: resp.Choices[0].Message.Content, Usage: usage}, nil
}

// StreamChat 实现 Client。
func (c *openaiClient) StreamChat(ctx context.Context, messages []Message, onChunk func(string)) error {
	if c.endpoint == "" {
		return ErrMissingEndpoint
	}
	if c.apiKey == "" {
		return fmt.Errorf("aimodel: openai 需要 apiKey")
	}

	payload := openAIChatRequest{
		Model:       c.modelName(),
		Messages:    toOpenAIMessages(messages),
		Stream:      true,
		MaxTokens:   c.cfg.maxOutputTokens,
		Temperature: 0.3,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.chatURL(), strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
		return fmt.Errorf("aimodel: HTTP %d: %s", resp.StatusCode, truncateBody(body))
	}

	return scanOpenAIStream(resp.Body, onChunk)
}

// scanOpenAIStream 解析 OpenAI SSE 流并逐块回调。
func scanOpenAIStream(body io.Reader, onChunk func(string)) error {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		raw := strings.TrimPrefix(line, "data: ")
		if raw == "[DONE]" {
			return nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if uerr := json.Unmarshal([]byte(raw), &chunk); uerr != nil {
			continue
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				onChunk(ch.Delta.Content)
			}
		}
	}
	return sc.Err()
}

// chatURL 构造 chat completions URL。
func (c *openaiClient) chatURL() string {
	ep := normalizeEndpoint(c.endpoint)
	if strings.HasSuffix(ep, "/chat/completions") {
		return ep
	}
	if strings.HasSuffix(ep, "/v1") {
		return ep + "/chat/completions"
	}
	return ep + "/v1/chat/completions"
}

// modelName 返回配置的模型名。
func (c *openaiClient) modelName() string {
	if c.cfg.model != "" {
		return c.cfg.model
	}
	return "gpt-4o-mini"
}

// claudeClient Claude 客户端。
type claudeClient struct {
	endpoint string
	apiKey   string
	cfg      providerConfig
}

// Chat 实现 Client。
func (c *claudeClient) Chat(ctx context.Context, messages []Message) (*Response, error) {
	if c.endpoint == "" {
		return nil, ErrMissingEndpoint
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("aimodel: claude 需要 apiKey")
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	payload := claudeRequest{
		Model:     c.modelName(),
		Messages:  toClaudeMessages(messages),
		MaxTokens: c.cfg.maxOutputTokens,
	}

	body, err := chatWithRetry(reqCtx, c.cfg, func() ([]byte, int, error) {
		headers := buildHeaders(c.apiKey)
		headers["x-api-key"] = c.apiKey
		headers["anthropic-version"] = "2023-06-01"
		return doJSONRequest(reqCtx, normalizeEndpoint(c.endpoint)+"/v1/messages", headers, payload)
	})
	if err != nil {
		return nil, err
	}

	var resp claudeResponse
	if uerr := json.Unmarshal(body, &resp); uerr != nil {
		return nil, fmt.Errorf("aimodel: 解析 claude 响应失败: %w", uerr)
	}
	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("aimodel: claude 返回空内容")
	}

	usage := Usage{PromptTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens}
	if err := consumeBudget(resp.Usage.OutputTokens); err != nil {
		return nil, err
	}
	return &Response{Content: resp.Content[0].Text, Usage: usage}, nil
}

// StreamChat 实现 Client（Claude SSE）。
func (c *claudeClient) StreamChat(ctx context.Context, messages []Message, onChunk func(string)) error {
	if c.endpoint == "" {
		return ErrMissingEndpoint
	}
	if c.apiKey == "" {
		return fmt.Errorf("aimodel: claude 需要 apiKey")
	}

	payload := claudeRequest{
		Model:     c.modelName(),
		Messages:  toClaudeMessages(messages),
		MaxTokens: c.cfg.maxOutputTokens,
		Stream:    true,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		normalizeEndpoint(c.endpoint)+"/v1/messages", strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
		return fmt.Errorf("aimodel: HTTP %d: %s", resp.StatusCode, truncateBody(body))
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		raw := strings.TrimPrefix(line, "data: ")
		var chunk struct {
			Type  string `json:"type"`
			Delta struct {
				Text string `json:"text"`
			} `json:"delta"`
		}
		if uerr := json.Unmarshal([]byte(raw), &chunk); uerr != nil {
			continue
		}
		if chunk.Type == "content_block_delta" && chunk.Delta.Text != "" {
			onChunk(chunk.Delta.Text)
		}
	}
	return sc.Err()
}

// modelName 返回配置的模型名。
func (c *claudeClient) modelName() string {
	if c.cfg.model != "" {
		return c.cfg.model
	}
	return "claude-sonnet-4-20250514"
}

// deepseekClient DeepSeek 客户端（OpenAI 兼容协议）。
type deepseekClient struct {
	inner *openaiClient
}

// Chat 实现 Client。
func (c *deepseekClient) Chat(ctx context.Context, messages []Message) (*Response, error) {
	return c.inner.Chat(ctx, messages)
}

// StreamChat 实现 Client。
func (c *deepseekClient) StreamChat(ctx context.Context, messages []Message, onChunk func(string)) error {
	return c.inner.StreamChat(ctx, messages, onChunk)
}

// fallbackClient 错误兜底客户端。
type fallbackClient struct {
	err error
}

// Chat 实现 Client。
func (c *fallbackClient) Chat(_ context.Context, _ []Message) (*Response, error) {
	return nil, c.err
}

// StreamChat 实现 Client。
func (c *fallbackClient) StreamChat(_ context.Context, _ []Message, _ func(string)) error {
	return c.err
}

// --- 消息格式转换 ---

// toOllamaMessages 转换为 ollama 消息格式。
func toOllamaMessages(messages []Message) []ollamaMessage {
	out := make([]ollamaMessage, 0, len(messages))
	for _, m := range messages {
		out = append(out, ollamaMessage{Role: m.Role, Content: m.Content})
	}
	return out
}

// toOpenAIMessages 转换为 OpenAI 消息格式。
func toOpenAIMessages(messages []Message) []openAIMessage {
	out := make([]openAIMessage, 0, len(messages))
	for _, m := range messages {
		out = append(out, openAIMessage{Role: m.Role, Content: m.Content})
	}
	return out
}

// toClaudeMessages 转换为 Claude 消息格式。
//
// Claude 的 system 角色需单独传，此处将 system 消息并入 user 消息内容，
// 保证单消息调用仍能携带系统提示。
func toClaudeMessages(messages []Message) []claudeMessage {
	out := make([]claudeMessage, 0, len(messages))
	systemParts := make([]string, 0, 2)

	for _, m := range messages {
		if m.Role == "system" {
			systemParts = append(systemParts, m.Content)
			continue
		}
		role := m.Role
		if role != "assistant" {
			role = "user"
		}
		out = append(out, claudeMessage{Role: role, Content: m.Content})
	}

	if len(systemParts) > 0 {
		prefix := strings.Join(systemParts, "\n\n")
		if len(out) > 0 {
			out[0].Content = prefix + "\n\n" + out[0].Content
		} else {
			out = append(out, claudeMessage{Role: "user", Content: prefix})
		}
	}
	return out
}
