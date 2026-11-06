// Package search 子文件：BM25 打分（V2.4-10，FR-402）。
//
// BM25 是关键词检索的标准排序函数：
//
//	score(q,d) = Σ IDF(t) · (tf(t,d) · (k1+1)) / (tf(t,d) + k1 · (1 - b + b · |d|/avgdl))
//
// 参数：
//   - k1 控制词频饱和速度（默认 1.2）
//   - b  控制文档长度归一化强度（默认 0.75）
//
// 实现说明：
//   - 文档集合由调用方提供（日志行或模板），本包不负责存储
//   - 分词采用「英文按非字母数字切分 + 中文按单字」的轻量策略，
//     与排障场景匹配（错误码、类名、异常名为主）
package search

import (
	"math"
	"strings"
)

// BM25 默认参数。
const (
	// DefaultBM25K1 词频饱和参数
	DefaultBM25K1 = 1.2
	// DefaultBM25B 长度归一化参数
	DefaultBM25B = 0.75
)

// BM25Scorer BM25 打分器。
type BM25Scorer struct {
	// K1 词频饱和参数
	K1 float64
	// B 长度归一化参数
	B float64

	// docs 文档词频表：docID -> term -> tf
	docs map[string]map[string]int
	// docLengths 文档长度
	docLengths map[string]int
	// avgdl 平均文档长度
	avgdl float64
	// N 文档总数
	N int
	// idf 缓存：term -> IDF
	idf map[string]float64
}

// NewBM25Scorer 创建打分器。
func NewBM25Scorer() *BM25Scorer {
	return &BM25Scorer{
		K1:         DefaultBM25K1,
		B:          DefaultBM25B,
		docs:       make(map[string]map[string]int),
		docLengths: make(map[string]int),
		idf:         make(map[string]float64),
	}
}

// AddDocument 添加文档到索引。
//
// id 为文档标识；text 为文档内容。
func (s *BM25Scorer) AddDocument(id, text string) {
	if id == "" {
		return
	}
	tf := make(map[string]int)
	for _, term := range Tokenize(text) {
		tf[term]++
	}
	s.docs[id] = tf
	s.docLengths[id] = len(tf)
	s.N++
	s.recalcAvgdl()
	s.idf = make(map[string]float64) // 清空 IDF 缓存
}

// recalcAvgdl 重算平均文档长度。
func (s *BM25Scorer) recalcAvgdl() {
	if s.N == 0 {
		s.avgdl = 0
		return
	}
	total := 0
	for _, l := range s.docLengths {
		total += l
	}
	s.avgdl = float64(total) / float64(s.N)
}

// idfOf 计算词条 IDF（带缓存）。
//
// 使用平滑形式 ln(1 + (N - df + 0.5)/(df + 0.5))，避免负值。
func (s *BM25Scorer) idfOf(term string) float64 {
	if v, ok := s.idf[term]; ok {
		return v
	}
	df := 0
	for _, tf := range s.docs {
		if _, exists := tf[term]; exists {
			df++
		}
	}
	v := math.Log(1 + (float64(s.N)-float64(df)+0.5)/(float64(df)+0.5))
	s.idf[term] = v
	return v
}

// Score 计算查询对指定文档的 BM25 分数。
func (s *BM25Scorer) Score(query, docID string) float64 {
	tf, ok := s.docs[docID]
	if !ok || s.avgdl == 0 {
		return 0
	}

	terms := Tokenize(query)
	if len(terms) == 0 {
		return 0
	}

	dl := float64(s.docLengths[docID])
	score := 0.0
	for _, term := range terms {
		freq, exists := tf[term]
		if !exists {
			continue
		}
		tfVal := float64(freq)
		numerator := tfVal * (s.K1 + 1)
		denominator := tfVal + s.K1*(1-s.B+s.B*dl/s.avgdl)
		if denominator == 0 {
			continue
		}
		score += s.idfOf(term) * (numerator / denominator)
	}
	return score
}

// Rank 对全部文档按查询打分，返回降序结果。
func (s *BM25Scorer) Rank(query string, topN int) []RankedItem {
	if topN <= 0 {
		topN = DefaultRRFTopN
	}

	out := make([]RankedItem, 0, len(s.docs))
	for id := range s.docs {
		score := s.Score(query, id)
		if score > 0 {
			out = append(out, RankedItem{ID: id, Score: score})
		}
	}

	// 按分数降序
	sortRanked(out)

	if len(out) > topN {
		out = out[:topN]
	}
	return out
}

// sortRanked 按分数降序排序。
func sortRanked(items []RankedItem) {
	// 简单插入排序（文档量级通常不大，且避免引入 sort 包的额外开销）
	for i := 1; i < len(items); i++ {
		cur := items[i]
		j := i - 1
		for j >= 0 && items[j].Score < cur.Score {
			items[j+1] = items[j]
			j--
		}
		items[j+1] = cur
	}
}

// Tokenize 分词。
//
// 策略：
//   - 英文/数字：连续字母数字片段（长度 ≥2）
//   - 中文：逐字切分
//   - 统一转小写
//
// 该策略适配排障场景：错误码（ERR_001）、类名（NullPointerException）、
// 英文单词（timeout）均能被正确切分。
func Tokenize(text string) []string {
	text = strings.ToLower(text)
	out := make([]string, 0, 16)
	var current strings.Builder

	flush := func() {
		w := current.String()
		current.Reset()
		// 保留单字母单词（a/I）与错误码片段，仅过滤纯单数字
		if len(w) == 1 && (w[0] < 'a' || w[0] > 'z') {
			return
		}
		if w == "" {
			return
		}
		out = append(out, w)
	}

	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			current.WriteRune(r)
		case r >= 0x4E00 && r <= 0x9FFF:
			// 中文：先冲刷累积的英文片段，再单独成词
			flush()
			out = append(out, string(r))
		default:
			flush()
		}
	}
	flush()
	return out
}
