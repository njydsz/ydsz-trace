// Package search 实现混合检索融合（V2.4-10，FR-402）。
//
// 背景：
//   单一检索手段各有盲区：
//   - 关键词检索（BM25）：精确匹配强，但无法理解同义表达
//   - 向量检索（语义）：能理解语义，但对专有名词/错误码不敏感
//
//   RRF（Reciprocal Rank Fusion）把两路结果的**排名**融合为统一分数，
//   不依赖各自分数的量纲，是业界最稳妥的混合排序方案。
//
// RRF 公式：
//
//	score(d) = Σ  weight_i / (k + rank_i(d))
//
// 其中：
//   - rank_i(d) 为文档 d 在第 i 路结果中的排名（从 1 开始）
//   - k 为平滑常数，取 60（Cormack 等人论文推荐值）
//   - weight_i 为该路权重，默认各 0.5
//
// 为什么用排名而非原始分数：
//   BM25 分数与余弦相似度量纲完全不同，直接相加无意义；
//   排名天然归一化，且对离群值不敏感。
package search

import (
	"sort"
)

// RRF 默认参数。
const (
	// DefaultRRFK 平滑常数（论文推荐 60）
	DefaultRRFK = 60
	// DefaultRRFTopN 融合后返回的最大条数
	DefaultRRFTopN = 50
)

// RankedItem 单路检索结果项。
type RankedItem struct {
	// ID 文档标识（模板 hash / 日志行号等）
	ID string
	// Score 原始分数（仅用于同路内排序，不参与融合）
	Score float64
	// Metadata 附加信息（原样透传到融合结果）
	Metadata map[string]interface{}
}

// FusedItem 融合后的结果项。
type FusedItem struct {
	// ID 文档标识
	ID string
	// Score RRF 融合分数
	Score float64
	// Contributions 各路贡献明细（便于解释与调试）
	Contributions map[string]float64
	// Metadata 来自原始结果的元数据（取首次出现者）
	Metadata map[string]interface{}
}

// RRFFuse 对多路检索结果做 RRF 融合。
//
// 参数：
//   - routes 各路结果，key 为路名称（如 "bm25" / "vector"）
//   - weights 各路权重，缺省的路使用 1.0
//   - k 平滑常数，<=0 时用 DefaultRRFK
//   - topN 返回条数上限，<=0 时用 DefaultRRFTopN
//
// 返回按融合分数降序排列的结果。
func RRFFuse(routes map[string][]RankedItem, weights map[string]float64, k float64, topN int) []FusedItem {
	if k <= 0 {
		k = DefaultRRFK
	}
	if topN <= 0 {
		topN = DefaultRRFTopN
	}
	if len(routes) == 0 {
		return nil
	}

	// 累加每篇文档的 RRF 分数
	type acc struct {
		score     float64
		contribs  map[string]float64
		metadata  map[string]interface{}
		firstSeen int
	}
	scores := make(map[string]*acc)
	order := make([]string, 0, 16)

	seq := 0
	for routeName, items := range routes {
		weight := 1.0
		if w, ok := weights[routeName]; ok && w > 0 {
			weight = w
		}

		for rank, item := range items {
			if item.ID == "" {
				continue
			}
			seq++
			a, exists := scores[item.ID]
			if !exists {
				a = &acc{
					contribs:  make(map[string]float64, len(routes)),
					metadata:  item.Metadata,
					firstSeen: seq,
				}
				scores[item.ID] = a
				order = append(order, item.ID)
			}
			// 同一路内同一 ID 只计一次（取最佳排名）
			if _, dup := a.contribs[routeName]; dup {
				continue
			}
			contribution := weight / (k + float64(rank+1))
			a.contribs[routeName] = contribution
			a.score += contribution
		}
	}

	// 组装结果
	out := make([]FusedItem, 0, len(order))
	for _, id := range order {
		a := scores[id]
		out = append(out, FusedItem{
			ID:            id,
			Score:         a.score,
			Contributions: a.contribs,
			Metadata:      a.metadata,
		})
	}

	// 按融合分数降序；同分按首次出现顺序稳定排序
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return scores[out[i].ID].firstSeen < scores[out[j].ID].firstSeen
	})

	if len(out) > topN {
		out = out[:topN]
	}
	return out
}

// FuseTwoRoutes 便捷函数：融合两路结果（常见的 BM25 + 向量场景）。
//
// bm25Weight / vectorWeight 分别指定权重，<=0 时各取 0.5。
func FuseTwoRoutes(bm25, vector []RankedItem, bm25Weight, vectorWeight float64) []FusedItem {
	if bm25Weight <= 0 {
		bm25Weight = 0.5
	}
	if vectorWeight <= 0 {
		vectorWeight = 0.5
	}
	return RRFFuse(
		map[string][]RankedItem{"bm25": bm25, "vector": vector},
		map[string]float64{"bm25": bm25Weight, "vector": vectorWeight},
		DefaultRRFK,
		DefaultRRFTopN,
	)
}

// NormalizeScores 把一路分数归一化到 [0,1]（用于需要展示原始分数的场景）。
//
// 注意：RRF 本身不需要归一化（它只用排名），此函数供其他排序策略使用。
func NormalizeScores(items []RankedItem) {
	if len(items) == 0 {
		return
	}
	maxScore := items[0].Score
	minScore := items[0].Score
	for _, it := range items {
		if it.Score > maxScore {
			maxScore = it.Score
		}
		if it.Score < minScore {
			minScore = it.Score
		}
	}
	span := maxScore - minScore
	if span == 0 {
		return
	}
	for i := range items {
		items[i].Score = (items[i].Score - minScore) / span
	}
}
