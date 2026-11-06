// Package ydsz 提供云顶追踪 HTTP API 的 Go 客户端（V2.4-02）。
//
// 设计：
//   - 统一错误模型：非 200 或业务码非 0 返回 *APIError
//   - 统一重试：网络错误 / 429 / 5xx 指数退避重试
//   - 鉴权：优先 YDSZ_TOKEN 环境变量，其次构造参数
//   - 全部方法接受 context.Context，支持取消与超时
package ydsz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

// 默认配置。
const (
	// DefaultTimeout 单次请求默认超时
	DefaultTimeout = 30 * time.Second
	// DefaultMaxRetries 默认重试次数
	DefaultMaxRetries = 2
	// DefaultRetryBaseDelay 重试退避基准
	DefaultRetryBaseDelay = 300 * time.Millisecond
	// maxResponseBodySize 响应体读取上限
	maxResponseBodySize = 8 * 1024 * 1024
)

// Client 云顶追踪 API 客户端。
type Client struct {
	baseURL string
	token   string
	http    *http.Client

	maxRetries     int
	retryBaseDelay time.Duration
}

// ClientOption 客户端配置选项。
type ClientOption func(*Client)

// WithHTTPClient 注入自定义 HTTP 客户端。
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) { c.http = hc }
}

// WithTimeout 设置请求超时。
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) { c.http.Timeout = d }
}

// WithMaxRetries 设置重试次数（0 表示不重试）。
func WithMaxRetries(n int) ClientOption {
	return func(c *Client) { c.maxRetries = n }
}

// WithRetryBaseDelay 设置重试退避基准。
func WithRetryBaseDelay(d time.Duration) ClientOption {
	return func(c *Client) { c.retryBaseDelay = d }
}

// NewClient 创建客户端。
//
// token 为空时回退读取 YDSZ_TOKEN 环境变量。
func NewClient(baseURL, token string, opts ...ClientOption) *Client {
	if token == "" {
		token = os.Getenv("YDSZ_TOKEN")
	}

	c := &Client{
		baseURL:        trimSlash(baseURL),
		token:          token,
		http:           &http.Client{Timeout: DefaultTimeout},
		maxRetries:     DefaultMaxRetries,
		retryBaseDelay: DefaultRetryBaseDelay,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Token 返回当前鉴权令牌（便于测试与调试）。
func (c *Client) Token() string { return c.token }

// BaseURL 返回服务地址。
func (c *Client) BaseURL() string { return c.baseURL }

// APIError 统一错误模型。
type APIError struct {
	// Code 业务码
	Code int
	// Message 错误信息
	Message string
	// TraceID 链路 ID（用于排查）
	TraceID string
	// StatusCode HTTP 状态码
	StatusCode int
}

// Error 实现 error 接口。
func (e *APIError) Error() string {
	return fmt.Sprintf("ydsz: code=%d message=%q traceId=%s", e.Code, e.Message, e.TraceID)
}

// IsRetryable 判断 HTTP 状态码是否值得重试。
func isRetryable(status int) bool {
	return status == http.StatusTooManyRequests ||
		status == http.StatusRequestTimeout ||
		status >= http.StatusInternalServerError
}

// do 执行 HTTP 请求并解码统一响应结构。
func (c *Client) do(ctx context.Context, method, path string, body, out interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			delay := c.retryBaseDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		status, resp, err := c.once(ctx, method, path, body)
		if err != nil {
			lastErr = err
			// context 取消不再重试
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}

		if status != http.StatusOK {
			apiErr := &APIError{
				StatusCode: status,
				Message:    resp.Message,
				TraceID:    resp.TraceID,
				Code:       resp.Code,
			}
			if !isRetryable(status) {
				return apiErr
			}
			lastErr = apiErr
			continue
		}

		// 业务码非 0 视为失败（不重试）
		if resp.Code != 0 {
			return &APIError{
				Code:     resp.Code,
				Message:  resp.Message,
				TraceID:  resp.TraceID,
				StatusCode: status,
			}
		}

		if out != nil && resp.Data != nil {
			// 重新序列化再反序列化，避免直接持有内部引用
			raw, merr := json.Marshal(resp.Data)
			if merr != nil {
				return fmt.Errorf("ydsz: 序列化响应数据失败: %w", merr)
			}
			if uerr := json.Unmarshal(raw, out); uerr != nil {
				return fmt.Errorf("ydsz: 解析响应数据失败: %w", uerr)
			}
		}
		return nil
	}

	return fmt.Errorf("ydsz: 重试 %d 次后仍失败: %w", c.maxRetries, lastErr)
}

// once 执行单次请求。
func (c *Client) once(ctx context.Context, method, path string, body interface{}) (int, *Response, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("ydsz: 序列化请求失败: %w", err)
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("ydsz: 构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("ydsz: 读取响应失败: %w", err)
	}

	// 空响应体（部分网关对 4xx/5xx 不返回 body）按状态码处理，不做 JSON 解析
	if len(bytes.TrimSpace(raw)) == 0 {
		return resp.StatusCode, &Response{StatusCode: resp.StatusCode}, nil
	}

	var parsed Response
	if uerr := json.Unmarshal(raw, &parsed); uerr != nil {
		// 非 200 时解析失败无需重试（错误已确定）；200 时才视为可重试的协议异常
		if resp.StatusCode != http.StatusOK {
			return resp.StatusCode, &Response{StatusCode: resp.StatusCode}, nil
		}
		return resp.StatusCode, &parsed, fmt.Errorf("ydsz: 解析响应失败: %w", uerr)
	}
	parsed.StatusCode = resp.StatusCode
	return resp.StatusCode, &parsed, nil
}

// Response 统一响应结构（与 pkg/api.Response 对齐）。
type Response struct {
	Code       int         `json:"code"`
	Message    string      `json:"message"`
	Data       interface{} `json:"data"`
	TraceID    string      `json:"traceId"`
	StatusCode int         `json:"-"`
}

// trimSlash 去除尾部斜杠。
func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

// itoa 整数转字符串（避免调用方引入 strconv）。
func itoa(n int) string {
	return strconv.Itoa(n)
}
