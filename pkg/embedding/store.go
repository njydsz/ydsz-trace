package embedding

import (
	"context"
	"sort"
	"sync"
)

// SearchResult 向量检索结果。
type SearchResult struct {
	// Key 向量标识。
	Key string
	// Score 相似度得分（越大越相似）。
	Score float32
	// Metadata 附加元数据。
	Metadata map[string]string
}

// VectorStore 向量存储接口。
//
// 两种实现：InMemoryStore（测试用）、SQLStore（生产用）。
type VectorStore interface {
	// Store 存入向量。
	Store(ctx context.Context, key string, vector []float32, metadata map[string]string) error
	// Search 检索 topK 相似向量。
	Search(ctx context.Context, query []float32, topK int) ([]SearchResult, error)
}

// --- InMemoryStore ---

// NewInMemoryStore 创建内存向量存储（适合测试）。
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		vectors:  make(map[string][]float32),
		metadata: make(map[string]map[string]string),
	}
}

// InMemoryStore 内存向量存储（并发安全）。
type InMemoryStore struct {
	mu       sync.RWMutex
	vectors  map[string][]float32
	metadata map[string]map[string]string
}

// Store 存入向量。
func (s *InMemoryStore) Store(ctx context.Context, key string, vector []float32, metadata map[string]string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	vecCopy := make([]float32, len(vector))
	copy(vecCopy, vector)
	s.vectors[key] = vecCopy

	if metadata != nil {
		metaCopy := make(map[string]string, len(metadata))
		for k, v := range metadata {
			metaCopy[k] = v
		}
		s.metadata[key] = metaCopy
	} else {
		s.metadata[key] = make(map[string]string)
	}

	return nil
}

// Search 在内存中做 cosine similarity 检索。
func (s *InMemoryStore) Search(ctx context.Context, query []float32, topK int) ([]SearchResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]SearchResult, 0, len(s.vectors))
	for key, vec := range s.vectors {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		score := CosineSimilarity(query, vec)
		results = append(results, SearchResult{
			Key:      key,
			Score:    score,
			Metadata: s.metadata[key],
		})
	}

	return topKResults(results, topK), nil
}

// topKResults 按 Score 降序排列并返回前 topK 项。
//
// V2.1 优化：使用 sort.Slice（底层为 pdqsort，比插入排序快 10-100 倍）。
func topKResults(results []SearchResult, topK int) []SearchResult {
	if len(results) == 0 {
		return results
	}

	// 如果 topK 大于等于结果数，直接全排序
	if topK <= 0 || topK >= len(results) {
		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})
		return results
	}

	// topK 小于结果数：先全排序（数据量在万级时仍可接受）
	// 未来可优化为堆选择算法（维护大小为 topK 的小顶堆）
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results[:topK]
}
