// Package search 提供混合检索能力（V2.1 FR-402）。
//
// 核心：BM25 关键词相关性 + 向量语义相似度，通过 RRF（Reciprocal Rank Fusion）
// 融合两路排序结果，兼顾"精确匹配"与"语义理解"。
//
// RRF 公式：score(d) = Σ 1/(k + rank_i(d))，k 默认 60。
package search

import (
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"
)

const (
	// rrfK RRF 平滑常数（原始论文推荐 60）
	rrfK = 60.0
	// bm25K1 BM25 词频饱和参数
	bm25K1 = 1.2
	// bm25B BM25 文档长度归一化参数
	bm25B = 0.75
)

// Document 待检索文档。
type Document struct {
	ID      string
	Content string
	// Vector 可选：语义向量（为空则不参与向量路）
	Vector []float32
}

// SearchResult 单条检索结果。
type SearchResult struct {
	ID         string
	Score      float64
	BM25Score  float64
	RRFScore   float64
	VectorRank int
	TextRank   int
}

// BM25Index BM25 倒排索引。
type BM25Index struct {
	sync.RWMutex

	docs      map[string]*Document
	docCount  int
	avgDocLen float64
	// termFreqs 词项 -> 文档 -> 词频
	termFreqs map[string]map[string]int
	// docFreqs 词项 -> 文档频率
	docFreqs map[string]int
	// docLens 文档 -> 长度（词数）
	docLens map[string]int
}

// NewBM25Index 创建空 BM25 索引。
func NewBM25Index() *BM25Index {
	return &BM25Index{
		docs:      make(map[string]*Document),
		termFreqs: make(map[string]map[string]int),
		docFreqs:  make(map[string]int),
		docLens:   make(map[string]int),
	}
}

// Add 添加文档到索引。
func (idx *BM25Index) Add(doc *Document) {
	if doc == nil || doc.ID == "" {
		return
	}

	idx.Lock()
	defer idx.Unlock()

	idx.docs[doc.ID] = doc

	tokens := Tokenize(doc.Content)
	idx.docLens[doc.ID] = len(tokens)

	// 统计词频
	for _, tok := range tokens {
		if idx.termFreqs[tok] == nil {
			idx.termFreqs[tok] = make(map[string]int)
		}
		idx.termFreqs[tok][doc.ID]++
	}

	// 更新文档频率
	seen := make(map[string]bool)
	for _, tok := range tokens {
		if !seen[tok] {
			seen[tok] = true
			idx.docFreqs[tok]++
		}
	}

	idx.docCount++
	idx.recalcAvgLen()
}

// recalcAvgLen 重新计算平均文档长度。
func (idx *BM25Index) recalcAvgLen() {
	if idx.docCount == 0 {
		idx.avgDocLen = 0
		return
	}
	total := 0
	for _, l := range idx.docLens {
		total += l
	}
	idx.avgDocLen = float64(total) / float64(idx.docCount)
}

// Search 对查询词执行 BM25 检索，返回 topK 结果。
func (idx *BM25Index) Search(query string, topK int) []SearchResult {
	idx.RLock()
	defer idx.RUnlock()

	queryTokens := Tokenize(query)
	if len(queryTokens) == 0 || idx.docCount == 0 {
		return nil
	}

	scores := make(map[string]float64)
	for _, tok := range queryTokens {
		tfMap, ok := idx.termFreqs[tok]
		if !ok {
			continue
		}
		df := float64(idx.docFreqs[tok])
		idf := math.Log(1 + (float64(idx.docCount)-df+0.5)/(df+0.5))

		for docID, tf := range tfMap {
			docLen := float64(idx.docLens[docID])
			tfScore := float64(tf) * (bm25K1 + 1)
			denom := float64(tf) + bm25K1*(1-bm25B+bm25B*docLen/idx.avgDocLen)
			if denom == 0 {
				continue
			}
			scores[docID] += idf * tfScore / denom
		}
	}

	results := make([]SearchResult, 0, len(scores))
	for id, s := range scores {
		results = append(results, SearchResult{
			ID:        id,
			BM25Score: s,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].BM25Score > results[j].BM25Score
	})

	if topK > 0 && topK < len(results) {
		results = results[:topK]
	}
	return results
}

// Tokenize 分词（支持中英文混合）。
//
// 中文按字切分，英文按空格+标点切分。
func Tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var current strings.Builder

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	for _, r := range text {
		switch {
		case unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r):
			flush()
		case r >= '\u4e00' && r <= '\u9fff': // CJK 统一表意文字
			flush()
			tokens = append(tokens, string(r))
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return tokens
}

// RRFusion 使用 RRF 融合 BM25 和向量两路排序结果。
//
// bm25Results: BM25 检索结果（已排序）
// vectorResults: 向量检索结果（已排序）
// topK: 返回条数
func RRFusion(bm25Results, vectorResults []SearchResult, topK int) []SearchResult {
	fused := make(map[string]*SearchResult)

	// 合并 BM25 结果
	for rank, r := range bm25Results {
		s := fused[r.ID]
		if s == nil {
			s = &SearchResult{ID: r.ID, BM25Score: r.BM25Score}
			fused[r.ID] = s
		}
		s.TextRank = rank + 1
		s.RRFScore += 1.0 / (rrfK + float64(rank+1))
	}

	// 合并向量结果
	for rank, r := range vectorResults {
		s := fused[r.ID]
		if s == nil {
			s = &SearchResult{ID: r.ID}
			fused[r.ID] = s
		}
		s.VectorRank = rank + 1
		s.RRFScore += 1.0 / (rrfK + float64(rank+1))
	}

	// 转换为切片并排序
	results := make([]SearchResult, 0, len(fused))
	for _, s := range fused {
		s.Score = s.RRFScore
		results = append(results, *s)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if topK > 0 && topK < len(results) {
		results = results[:topK]
	}
	return results
}

// HybridSearch 执行混合检索（BM25 + 向量 RRF 融合）。
//
// 参数：
//   - index BM25 索引
//   - query 查询文本
//   - vector 查询向量（为空则仅 BM25）
//   - cosineFn 余弦相似度函数（用于向量路排序）
//   - topK 返回条数
func HybridSearch(index *BM25Index, query string, vector []float32,
	cosineFn func(a, b []float32) float32, topK int) []SearchResult {

	// BM25 路
	bm25Results := index.Search(query, topK*2)

	// 向量路
	var vectorResults []SearchResult
	if vector != nil && cosineFn != nil && len(index.docs) > 0 {
		type vecScore struct {
			id    string
			score float64
		}
		scores := make([]vecScore, 0, len(index.docs))
		for id, doc := range index.docs {
			if len(doc.Vector) == 0 {
				continue
			}
			s := cosineFn(vector, doc.Vector)
			scores = append(scores, vecScore{id: id, score: float64(s)})
		}
		sort.Slice(scores, func(i, j int) bool {
			return scores[i].score > scores[j].score
		})
		if len(scores) > topK*2 {
			scores = scores[:topK*2]
		}
		for _, vs := range scores {
			vectorResults = append(vectorResults, SearchResult{
				ID:        vs.id,
				BM25Score: vs.score,
			})
		}
	}

	// RRF 融合
	return RRFusion(bm25Results, vectorResults, topK)
}
