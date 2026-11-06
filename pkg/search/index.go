// Package search 子文件：混合检索索引（V2.4-10，FR-402）。
//
// 对外暴露既有契约 API（BM25Index / Document / SearchResult / HybridSearch），
// 内部复用 BM25Scorer 与 RRFFuse 实现。
//
// 设计：
//   - BM25Index 线程安全（并发 Add / Search）
//   - 向量以原始 []float32 存储，相似度函数由调用方注入
//     （保持包内零依赖，不绑定具体 Embedder）
//   - HybridSearch = BM25 排名 + 向量排名 → RRF 融合
package search

import (
	"sync"
)

// Document 索引文档。
type Document struct {
	// ID 文档唯一标识
	ID string
	// Content 文档文本内容
	Content string
	// Vector 文档向量（可选，为空时仅参与 BM25 路）
	Vector []float32
}

// SearchResult 单条检索结果。
type SearchResult struct {
	// ID 文档标识
	ID string
	// BM25Score BM25 原始分数
	BM25Score float64
	// VectorScore 向量相似度（余弦）
	VectorScore float64
	// Score RRF 融合分数（HybridSearch 返回时填充）
	Score float64
}

// SimilarityFunc 向量相似度函数签名。
//
// 常见实现为余弦相似度；由调用方注入，保持本包零依赖。
type SimilarityFunc func(a, b []float32) float32

// BM25Index 混合检索索引。
type BM25Index struct {
	mu sync.RWMutex
	// scorer BM25 打分器
	scorer *BM25Scorer
	// docs 文档内容（用于 BM25 重建）
	docs map[string]*Document
	// docCount 文档总数（导出供测试断言）
	docCount int
}

// NewBM25Index 创建索引。
func NewBM25Index() *BM25Index {
	return &BM25Index{
		scorer: NewBM25Scorer(),
		docs:   make(map[string]*Document),
	}
}

// Add 添加或更新文档。
func (idx *BM25Index) Add(doc *Document) {
	if doc == nil || doc.ID == "" {
		return
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	// 已存在的文档先移除旧词频（重建）
	if _, exists := idx.docs[doc.ID]; exists {
		idx.rebuildLocked()
	} else {
		idx.docCount++
	}

	idx.docs[doc.ID] = doc
	idx.scorer.AddDocument(doc.ID, doc.Content)
}

// rebuildLocked 重建索引（在已持有锁时调用）。
//
// 注意：调用方必须持有写锁。当前实现为全量重建，
// 文档量级在万级以内时性能可接受；若后续文档量增长，
// 应改为增量更新词频表。
func (idx *BM25Index) rebuildLocked() {
	idx.scorer = NewBM25Scorer()
	for _, doc := range idx.docs {
		idx.scorer.AddDocument(doc.ID, doc.Content)
	}
}

// Search 纯 BM25 检索。
func (idx *BM25Index) Search(query string, topN int) []SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	items := idx.scorer.Rank(query, topN)
	out := make([]SearchResult, 0, len(items))
	for _, it := range items {
		out = append(out, SearchResult{ID: it.ID, BM25Score: it.Score})
	}
	return out
}

// SearchVector 纯向量检索。
func (idx *BM25Index) SearchVector(queryVec []float32, sim SimilarityFunc, topN int) []SearchResult {
	if sim == nil || len(queryVec) == 0 {
		return nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	items := make([]RankedItem, 0, len(idx.docs))
	for id, doc := range idx.docs {
		if len(doc.Vector) == 0 {
			continue
		}
		score := sim(queryVec, doc.Vector)
		if score > 0 {
			items = append(items, RankedItem{ID: id, Score: float64(score)})
		}
	}

	sortRanked(items)
	if topN > 0 && len(items) > topN {
		items = items[:topN]
	}

	out := make([]SearchResult, 0, len(items))
	for _, it := range items {
		out = append(out, SearchResult{ID: it.ID, VectorScore: it.Score})
	}
	return out
}

// HybridSearch 混合检索：BM25 + 向量 → RRF 融合。
//
// 参数：
//   - query 关键词查询（BM25 路）
//   - queryVec 查询向量（向量路，为空则退化为纯 BM25）
//   - sim 向量相似度函数（queryVec 非空时必填）
//   - topN 返回条数上限
func (idx *BM25Index) HybridSearch(query string, queryVec []float32, sim SimilarityFunc, topN int) []SearchResult {
	idx.mu.RLock()
	bm25Items := idx.scorer.Rank(query, topN*2)
	vectorItems := make([]RankedItem, 0, 8)

	if len(queryVec) > 0 && sim != nil {
		for id, doc := range idx.docs {
			if len(doc.Vector) == 0 {
				continue
			}
			score := sim(queryVec, doc.Vector)
			if score > 0 {
				vectorItems = append(vectorItems, RankedItem{ID: id, Score: float64(score)})
			}
		}
		sortRanked(vectorItems)
	}
	idx.mu.RUnlock()

	fused := RRFFuse(
		map[string][]RankedItem{"bm25": bm25Items, "vector": vectorItems},
		map[string]float64{"bm25": 0.5, "vector": 0.5},
		DefaultRRFK,
		topN,
	)

	out := make([]SearchResult, 0, len(fused))
	for _, f := range fused {
		out = append(out, SearchResult{
			ID:    f.ID,
			Score: f.Score,
		})
	}
	return out
}

// DocCount 返回索引文档总数。
func (idx *BM25Index) DocCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.docCount
}

// RRFusion 两路结果融合（契约 API）。
//
// 参数 topN 为融合后返回条数上限。
// 说明：入参的 Score 字段（BM25Score / VectorScore）不参与计算，
// 融合只依据排名，这是 RRF 的核心特性。
func RRFusion(bm25, vector []SearchResult, topN int) []SearchResult {
	toRanked := func(items []SearchResult) []RankedItem {
		out := make([]RankedItem, 0, len(items))
		for _, it := range items {
			out = append(out, RankedItem{ID: it.ID, Score: it.BM25Score})
		}
		return out
	}

	fused := RRFFuse(
		map[string][]RankedItem{"bm25": toRanked(bm25), "vector": toRanked(vector)},
		map[string]float64{"bm25": 0.5, "vector": 0.5},
		DefaultRRFK,
		topN,
	)

	out := make([]SearchResult, 0, len(fused))
	for _, f := range fused {
		out = append(out, SearchResult{ID: f.ID, Score: f.Score})
	}
	return out
}

// HybridSearch 包级便捷函数（契约 API）。
//
// 对给定索引执行一次混合检索。
func HybridSearch(idx *BM25Index, query string, queryVec []float32, sim SimilarityFunc, topN int) []SearchResult {
	if idx == nil {
		return nil
	}
	return idx.HybridSearch(query, queryVec, sim, topN)
}
