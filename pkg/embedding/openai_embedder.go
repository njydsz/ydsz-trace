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
	"os"
	"time"
)

// openaiDefaultEndpoint OpenAI API 默认地址。
const openaiDefaultEndpoint = "https://api.openai.com/v1"

// openaiRequest OpenAI embeddings 请求体。
type openaiRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// openaiData 单条 embedding 结果。
type openaiData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
	Object    string    `json:"object"`
}

// openaiResponse OpenAI embeddings 响应体。
type openaiResponse struct {
	Data   []openaiData `json:"data"`
	Model  string       `json:"model"`
	Object string       `json:"object"`
}

// openaiEmbedder 调用 OpenAI 兼容 embedding 接口。
type openaiEmbedder struct {
	endpoint  string
	model     string
	dimension int
	apiKey    string
	client    *http.Client
}

// newOpenaiEmbedder 创建 OpenAI embedding 客户端。
//
// apiKey 优先从 opts 传入；若空则从环境变量 YDSZ_OPENAI_KEY 读取。
func newOpenaiEmbedder(cfg *embedderConfig) *openaiEmbedder {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("YDSZ_OPENAI_KEY")
	}

	if cfg.Endpoint == "" {
		cfg.Endpoint = openaiDefaultEndpoint
	}
	if cfg.Model == "" {
		cfg.Model = DefaultLocalModel
	}
	if cfg.Dimension == 0 {
		cfg.Dimension = 1536 // OpenAI text-embedding-3-small 维度
	}

	return &openaiEmbedder{
		endpoint:  cfg.Endpoint,
		model:     cfg.Model,
		dimension: cfg.Dimension,
		apiKey:    apiKey,
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

// Embed 对单条文本生成向量。
func (e *openaiEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, ErrEmptyInput
	}
	if e.apiKey == "" {
		return nil, fmt.Errorf("embedding: 未配置 YDSZ_OPENAI_KEY 环境变量")
	}

	reqBody := openaiRequest{
		Model: e.model,
		Input: text,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("embedding: 序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("%s/embeddings", e.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding: 创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		log.Printf("[embedding] OpenAI 调用失败: %v", err)
		return nil, fmt.Errorf("embedding: OpenAI API 不可达: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("[embedding] 关闭响应体失败: %v", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("embedding: OpenAI 返回错误 status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result openaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("embedding: 解码 OpenAI 响应失败: %w", err)
	}

	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embedding: OpenAI 返回空向量")
	}

	return result.Data[0].Embedding, nil
}

// EmbedBatch 批量生成向量。
func (e *openaiEmbedder) EmbedBatch(ctx context.Context, texts [][]string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, group := range texts {
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
func (e *openaiEmbedder) Dimension() int {
	return e.dimension
}
