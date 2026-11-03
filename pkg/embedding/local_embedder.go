package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

// localHTTPTimeout 本地 ollama 调用超时。
const localHTTPTimeout = 30 * time.Second

// newLocalEmbedder 创建本地 ollama embedding 客户端。
func newLocalEmbedder(cfg *embedderConfig) *localEmbedder {
	return &localEmbedder{
		endpoint:  cfg.Endpoint,
		model:     cfg.Model,
		dimension: cfg.Dimension,
		client: &http.Client{
			Timeout: localHTTPTimeout,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 60 * time.Second,
				}).DialContext,
				MaxIdleConns:        50,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
	}
}

// localEmbedder 调用本地 ollama /api/embeddings。
type localEmbedder struct {
	endpoint  string
	model     string
	dimension int
	client    *http.Client
}

// ollamaRequest ollama /api/embeddings 请求体。
type ollamaRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// ollamaResponse ollama /api/embeddings 响应体。
type ollamaResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed 对单条文本生成向量。
func (e *localEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, ErrEmptyInput
	}

	reqBody := ollamaRequest{
		Model: e.model,
		Input: text,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("embedding: 序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("%s/api/embeddings", e.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding: 创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		log.Printf("[embedding] 本地 ollama 调用失败: %v", err)
		return nil, fmt.Errorf("embedding: 本地 embedding 服务不可达：检查 ollama 服务是否启动 (%w)", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("[embedding] 关闭响应体失败: %v", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("embedding: ollama 返回错误 status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("embedding: 解码 ollama 响应失败: %w", err)
	}

	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("embedding: ollama 返回空向量")
	}

	return result.Embeddings[0], nil
}

// EmbedBatch 批量生成向量。
func (e *localEmbedder) EmbedBatch(ctx context.Context, texts [][]string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, group := range texts {
		// 每组取首条文本 embedding（每组视为同一语义单元的多个分片）。
		if len(group) == 0 {
			return nil, fmt.Errorf("embedding: batch index %d 文本组为空", i)
		}
		vec, err := e.Embed(ctx, group[0])
		if err != nil {
			return nil, fmt.Errorf("embedding: batch index %d 失败: %w", i, err)
		}
		results[i] = vec
	}
	return results, nil
}

// Dimension 返回向量维度。
func (e *localEmbedder) Dimension() int {
	return e.dimension
}
