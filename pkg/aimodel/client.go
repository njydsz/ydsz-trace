// Package aimodel 子文件：真实 HTTP Provider 实现（V2.3-01）。
//
// 覆盖四类后端，统一为 OpenAI 兼容的 /chat/completions 协议：
//
//   - ProviderLocal（ollama）：POST {endpoint}/api/chat
//   - ProviderOpenAI：POST {endpoint}/v1/chat/completions
//   - ProviderClaude：POST {endpoint}/v1/messages
//   - ProviderDeepSeek：OpenAI 兼容，endpoint 需含 /v1
//
// 三项硬约束（FR-908）：
//  1. 超时：每次调用受 context 控制，默认 DefaultTimeout
//  2. 重试：可重试错误（429/5xx/网络错误）按指数退避重试 DefaultMaxRetries 次
//  3. Token 预算：单次输出受 MaxOutputTokens 限制，累计受 Budget 限制
//
// 安全约束：
//   - 云端 Provider 必须显式配置 endpoint，禁止硬编码官方地址（便于私有化部署）
//   - API Key 仅通过构造参数注入，不落盘、不写日志
package aimodel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// 默认参数。
const (
	// DefaultTimeout 单次调用默认超时
	DefaultTimeout = 60 * time.Second
	// DefaultMaxRetries 默认重试次数
	DefaultMaxRetries = 2
	// DefaultMaxOutputTokens 默认最大输出 token
	DefaultMaxOutputTokens = 2048
	// DefaultRetryBaseDelay 重试基准退避（指数增长基数）
	DefaultRetryBaseDelay = 500 * time.Millisecond
	// DefaultStreamChunkTimeout 流式响应单块超时（防止连接挂死）
	DefaultStreamChunkTimeout = 30 * time.Second
)

// Options Client 行为选项。
type Options struct {
	// Timeout 单次调用超时（<=0 用 DefaultTimeout）
	Timeout time.Duration
	// MaxRetries 重试次数（0 表示使用 DefaultMaxRetries）。
	//
	// 需要「显式不重试」时置 NoRetry = true。
	MaxRetries int
	// NoRetry 显式禁用重试（优先级高于 MaxRetries）。
	NoRetry bool
	// MaxOutputTokens 最大输出 token（<=0 用 DefaultMaxOutputTokens）
	MaxOutputTokens int
	// RetryBaseDelay 重试退避基准（<=0 用 DefaultRetryBaseDelay）
	RetryBaseDelay time.Duration
	// Model 模型名（覆盖 endpoint 默认模型）
	Model string
}

// httpClient 共享 HTTP 客户端（连接复用 + 超时由 context 控制）。
var httpClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	},
}

// budgetTracker Token 预算追踪器（进程级，跨会话累计）。
type budgetTracker struct {
	mu      sync.Mutex
	limit   int
	used    int
	enabled bool
}

// globalBudget 进程级 Token 预算（0 表示不限制）。
var globalBudget = &budgetTracker{}

// SetBudget 设置进程级 Token 预算（累计输出 token 上限）。
//
// limit <= 0 表示不限制。超过预算后所有调用返回 ErrBudgetExceeded。
func SetBudget(limit int) {
	globalBudget.mu.Lock()
	defer globalBudget.mu.Unlock()
	globalBudget.limit = limit
	globalBudget.enabled = limit > 0
}

// BudgetUsage 返回当前预算使用情况。
func BudgetUsage() (used, limit int, enabled bool) {
	globalBudget.mu.Lock()
	defer globalBudget.mu.Unlock()
	return globalBudget.used, globalBudget.limit, globalBudget.enabled
}

// ErrBudgetExceeded Token 预算耗尽。
var ErrBudgetExceeded = fmt.Errorf("aimodel: token 预算已耗尽")

// consumeBudget 尝试消费指定 token 数；超出预算返回错误。
func consumeBudget(n int) error {
	globalBudget.mu.Lock()
	defer globalBudget.mu.Unlock()

	if !globalBudget.enabled {
		return nil
	}
	if globalBudget.used+n > globalBudget.limit {
		return ErrBudgetExceeded
	}
	globalBudget.used += n
	return nil
}

// providerConfig 从 Options 归一化出的运行时配置。
type providerConfig struct {
	timeout        time.Duration
	maxRetries     int
	maxOutputTokens int
	retryBaseDelay time.Duration
	model          string
}

// normalizeOptions 归一化选项默认值。
//
// MaxRetries 为 0 时使用默认值；需显式禁用重试时置 NoRetry = true。
func normalizeOptions(opts Options) providerConfig {
	cfg := providerConfig{
		timeout:         opts.Timeout,
		maxRetries:      opts.MaxRetries,
		maxOutputTokens: opts.MaxOutputTokens,
		retryBaseDelay:  opts.RetryBaseDelay,
		model:           opts.Model,
	}
	if cfg.timeout <= 0 {
		cfg.timeout = DefaultTimeout
	}
	if opts.NoRetry {
		cfg.maxRetries = 0
	} else if cfg.maxRetries <= 0 {
		cfg.maxRetries = DefaultMaxRetries
	}
	if cfg.maxOutputTokens <= 0 {
		cfg.maxOutputTokens = DefaultMaxOutputTokens
	}
	if cfg.retryBaseDelay <= 0 {
		cfg.retryBaseDelay = DefaultRetryBaseDelay
	}
	return cfg
}

// isRetryable 判断 HTTP 状态码是否值得重试。
func isRetryable(status int) bool {
	return status == http.StatusTooManyRequests ||
		status == http.StatusRequestTimeout ||
		status >= http.StatusInternalServerError
}

// chatWithRetry 执行一次 chat 请求，按指数退避重试。
//
// fn 返回 (响应体字节, 状态码, 错误)；状态码 0 表示网络层失败。
func chatWithRetry(ctx context.Context, cfg providerConfig, fn func() ([]byte, int, error)) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt <= cfg.maxRetries; attempt++ {
		if attempt > 0 {
			delay := cfg.retryBaseDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		body, status, err := fn()
		if err == nil && status >= 200 && status < 300 {
			return body, nil
		}

		if err != nil {
			lastErr = err
			// context 取消/超时不再重试
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// 非网络错误（如请求体构造失败）不重试
			if _, ok := err.(*requestError); ok {
				return nil, err
			}
		} else if !isRetryable(status) {
			// 4xx 客户端错误不重试（重试只会重复失败）
			return nil, fmt.Errorf("aimodel: HTTP %d: %s", status, truncateBody(body))
		} else {
			lastErr = fmt.Errorf("aimodel: HTTP %d", status)
		}
	}

	return nil, fmt.Errorf("aimodel: 重试 %d 次后仍失败: %w", cfg.maxRetries, lastErr)
}

// requestError 请求构造期错误（不可重试）。
type requestError struct{ err error }

// Error 实现 error。
func (e *requestError) Error() string { return e.err.Error() }

// truncateBody 截断响应体用于错误信息。
func truncateBody(b []byte) string {
	const max = 200
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "..."
}

// doJSONRequest 发送 JSON POST 请求并返回响应体。
func doJSONRequest(ctx context.Context, url string, headers map[string]string, payload interface{}) ([]byte, int, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, &requestError{err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, 0, &requestError{err}
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			// 关闭响应体失败仅记录，不影响主流程
			_ = cerr
		}
	}()

	body, rerr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if rerr != nil {
		return nil, resp.StatusCode, rerr
	}
	return body, resp.StatusCode, nil
}

// maxResponseBodySize 响应体读取上限（8MB，防止异常响应撑爆内存）。
const maxResponseBodySize = 8 * 1024 * 1024

// buildHeaders 构造认证头。
func buildHeaders(apiKey string) map[string]string {
	headers := make(map[string]string, 2)
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}
	return headers
}

// normalizeEndpoint 规整 endpoint（去尾斜杠）。
func normalizeEndpoint(endpoint string) string {
	return strings.TrimRight(endpoint, "/")
}
