// Package embedding 子文件：HNSW（Hierarchical Navigable Small World）向量索引（V2.1 FR-303）。
//
// 简化版 HNSW 索引，用于大规模语义检索的近似最近邻加速：
//   - 多层跳表结构：layer 0 包含全部节点，上层按指数衰减抽样
//   - 每个节点维护 M 个最近邻（启发式选择）
//   - 查询：从顶层入口点贪心下降，层 0 做 beam search（ef 参数控制精度）
//   - 插入：随机层级 + 邻居连接
//
// 设计约束：
//   - 纯内存索引，不依赖外部库
//   - 支持增量添加（Add）
//   - 查询复杂度 O(log N)，满足 100 万向量 P95 ≤ 1s（FR-303）
//   - 线程安全（RWMutex）
package embedding

import (
	"context"
	"math"
	"math/rand"
	"sync"
)

const (
	// hnswDefaultM 每个节点的最大邻居数（影响精度与内存）
	hnswDefaultM = 16
	// hnswDefaultEfConstruct 构建时的搜索宽度（影响索引质量）
	hnswDefaultEfConstruct = 100
	// hnswDefaultEfSearch 查询时的搜索宽度（影响召回率与延迟）
	hnswDefaultEfSearch = 64
	// hnswMaxLevel 最大层数（概率衰减，实际层数通常 < 10）
	hnswMaxLevel = 16
	// hnswML 层级生成概率归一化因子（1/ln(M)）
	hnswML = 0.5
)

// hnswNode HNSW 图中的节点。
type hnswNode struct {
	key      string
	vector   []float32
	metadata map[string]string
	// neighbors 每层邻居 key 列表（layer -> []neighborKey）
	neighbors map[int][]string
	// level 节点最高层
	level int
}

// HNSWIndex HNSW 向量索引。
type HNSWIndex struct {
	mu sync.RWMutex

	// nodes 全部节点（key -> node）
	nodes map[string]*hnswNode

	// entryPoint 当前入口点 key
	entryPoint string
	// maxLevel 当前最大层数
	maxLevel int

	// 参数
	m              int
	efConstruct    int
	efSearch       int
	mL             float64
	rng            *rand.Rand

	// 维度（所有向量必须一致）
	dim int
}

// NewHNSWIndex 创建空 HNSW 索引。
func NewHNSWIndex() *HNSWIndex {
	return &HNSWIndex{
		nodes:       make(map[string]*hnswNode),
		m:           hnswDefaultM,
		efConstruct: hnswDefaultEfConstruct,
		efSearch:    hnswDefaultEfSearch,
		mL:          hnswML,
		rng:         rand.New(rand.NewSource(42)),
		maxLevel:    0,
	}
}

// Add 增量添加向量（线程安全）。
//
// 空索引时该节点成为入口点；否则从顶层贪心下降到插入层，
// 在每一层做启发式邻居选择并建立双向连接。
func (idx *HNSWIndex) Add(key string, vector []float32, metadata map[string]string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if len(vector) == 0 {
		return
	}

	// 维度一致性检查
	if idx.dim == 0 {
		idx.dim = len(vector)
	} else if len(vector) != idx.dim {
		return
	}

	vecCopy := make([]float32, len(vector))
	copy(vecCopy, vector)

	metaCopy := make(map[string]string, len(metadata))
	for k, v := range metadata {
		metaCopy[k] = v
	}

	// 生成随机层级
	level := idx.randomLevel()

	node := &hnswNode{
		key:      key,
		vector:   vecCopy,
		metadata: metaCopy,
		neighbors: make(map[int][]string),
		level:    level,
	}
	idx.nodes[key] = node

	// 空索引：设为入口点
	if idx.entryPoint == "" {
		idx.entryPoint = key
		idx.maxLevel = level
		return
	}

	// 从顶层贪心下降
	currEP := idx.entryPoint
	currDist := CosineSimilarity(vector, idx.nodes[currEP].vector)

	// 从 maxLevel 下降到 level+1：只移动入口点
	for l := idx.maxLevel; l > level; l-- {
		changed := true
		for changed {
			changed = false
			for _, nb := range idx.nodes[currEP].neighbors[l] {
				nbNode := idx.nodes[nb]
				if nbNode == nil {
					continue
				}
				d := CosineSimilarity(vector, nbNode.vector)
				if d > currDist {
					currDist = d
					currEP = nb
					changed = true
				}
			}
		}
	}

	// 从 min(level, maxLevel) 下降到 0：建立邻居连接
	for l := min(level, idx.maxLevel); l >= 0; l-- {
		// beam search 找 efConstruct 个最近邻
		candidates := idx.searchLayer(vector, currEP, l, idx.efConstruct)

		// 选择 M 个邻居（启发式：简单取最近的 M 个）
		selected := idx.selectNeighbors(candidates, idx.m)

		// 建立双向连接
		node.neighbors[l] = make([]string, len(selected))
		copy(node.neighbors[l], selected)

		for _, nb := range selected {
			nbNode := idx.nodes[nb]
			if nbNode == nil {
				continue
			}
			// 追加邻居
			nbNode.neighbors[l] = append(nbNode.neighbors[l], key)

			// 邻居数超过 M：裁剪最远的
			if len(nbNode.neighbors[l]) > idx.m {
				nbNode.neighbors[l] = idx.trimNeighbors(nbNode.vector, nbNode.neighbors[l], idx.m)
			}
		}

		// 更新入口点（如果新节点更近）
		if len(candidates) > 0 && candidates[0] != "" {
			currEP = candidates[0]
			currDist = CosineSimilarity(vector, idx.nodes[currEP].vector)
		}
	}

	// 更新全局入口点
	if level > idx.maxLevel {
		idx.maxLevel = level
		idx.entryPoint = key
	}
}

// Search 检索 topK 相似向量（线程安全）。
//
// 从顶层入口点贪心下降，层 0 做 beam search（ef 参数）。
// 返回结果按相似度降序排列。
func (idx *HNSWIndex) Search(ctx context.Context, query []float32, topK int) ([]SearchResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.entryPoint == "" || len(idx.nodes) == 0 {
		return nil, nil
	}

	// 从顶层贪心下降
	currEP := idx.entryPoint
	currDist := CosineSimilarity(query, idx.nodes[currEP].vector)

	for l := idx.maxLevel; l > 0; l-- {
		changed := true
		for changed {
			changed = false
			for _, nb := range idx.nodes[currEP].neighbors[l] {
				nbNode := idx.nodes[nb]
				if nbNode == nil {
					continue
				}
				d := CosineSimilarity(query, nbNode.vector)
				if d > currDist {
					currDist = d
					currEP = nb
					changed = true
				}
			}
		}
	}

	// 层 0 beam search
	candidates := idx.searchLayer(query, currEP, 0, idx.efSearch)

	// 构造结果
	results := make([]SearchResult, 0, len(candidates))
	for _, key := range candidates {
		node := idx.nodes[key]
		if node == nil {
			continue
		}
		score := CosineSimilarity(query, node.vector)
		results = append(results, SearchResult{
			Key:      key,
			Score:    score,
			Metadata: node.metadata,
		})
	}

	return topKResults(results, topK), nil
}

// Count 返回索引中的向量总数。
func (idx *HNSWIndex) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.nodes)
}

// IsReady 返回索引是否可用（至少有一个节点）。
func (idx *HNSWIndex) IsReady() bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.entryPoint != ""
}

// randomLevel 生成随机层级（指数衰减）。
func (idx *HNSWIndex) randomLevel() int {
	level := 0
	for idx.rng.Float64() < math.Exp(-1.0/idx.mL) && level < hnswMaxLevel {
		level++
	}
	return level
}

// searchLayer 在指定层做 beam search，返回 ef 个最近邻 key（按相似度降序）。
//
// 调用方需持有读锁。
//
// 算法：维护候选扩展集（小顶堆）和结果集（大顶堆）。
// 每次从候选集取最相似的节点扩展，将其邻居加入候选集；
// 当候选集中最不相似的节点也比结果集中最相似的节点更相似时，停止。
func (idx *HNSWIndex) searchLayer(query []float32, entry string, layer, ef int) []string {
	if entry == "" {
		return nil
	}

	entryNode := idx.nodes[entry]
	if entryNode == nil {
		return nil
	}

	// 已访问集合
	visited := make(map[string]bool)
	visited[entry] = true

	// 候选扩展集（candidates）：维护 ef 个最相似的待扩展节点
	// 使用切片 + 每次线性查找最小值（ef 通常 ≤ 200，O(ef) 可接受）
	type candidate struct {
		key  string
		dist float32
	}
	candidates := []candidate{{key: entry, dist: CosineSimilarity(query, entryNode.vector)}}

	// 结果集（results）：维护 ef 个最相似的节点（最终返回）
	results := []candidate{{key: entry, dist: CosineSimilarity(query, entryNode.vector)}}

	for len(candidates) > 0 {
		// 找候选集中最相似的（最大 dist）
		bestIdx := 0
		for i := 1; i < len(candidates); i++ {
			if candidates[i].dist > candidates[bestIdx].dist {
				bestIdx = i
			}
		}
		best := candidates[bestIdx]

		// 终止条件：候选集中最相似的也不如结果集中最差的
		// （结果集已满 ef 且 best.dist < results 中最小 dist）
		if len(results) >= ef {
			worstResultIdx := 0
			for i := 1; i < len(results); i++ {
				if results[i].dist < results[worstResultIdx].dist {
					worstResultIdx = i
				}
			}
			if best.dist < results[worstResultIdx].dist {
				break
			}
		}

		// 从候选集移除 best
		candidates = append(candidates[:bestIdx], candidates[bestIdx+1:]...)

		// 扩展 best 的邻居
		bestNode := idx.nodes[best.key]
		if bestNode == nil {
			continue
		}
		for _, nb := range bestNode.neighbors[layer] {
			if visited[nb] {
				continue
			}
			nbNode := idx.nodes[nb]
			if nbNode == nil {
				continue
			}
			visited[nb] = true

			d := CosineSimilarity(query, nbNode.vector)

			// 加入候选集（如果候选集未满或比候选集中最差的更相似）
			if len(candidates) < ef {
				candidates = append(candidates, candidate{key: nb, dist: d})
			} else {
				worstCandIdx := 0
				for i := 1; i < len(candidates); i++ {
					if candidates[i].dist < candidates[worstCandIdx].dist {
						worstCandIdx = i
					}
				}
				if d > candidates[worstCandIdx].dist {
					candidates[worstCandIdx] = candidate{key: nb, dist: d}
				}
			}

			// 加入结果集（如果结果集未满或比结果集中最差的更相似）
			if len(results) < ef {
				results = append(results, candidate{key: nb, dist: d})
			} else {
				worstResIdx := 0
				for i := 1; i < len(results); i++ {
					if results[i].dist < results[worstResIdx].dist {
						worstResIdx = i
					}
				}
				if d > results[worstResIdx].dist {
					results[worstResIdx] = candidate{key: nb, dist: d}
				}
			}
		}
	}

	// 按相似度降序排序结果
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].dist > results[i].dist {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	keys := make([]string, len(results))
	for i, r := range results {
		keys[i] = r.key
	}
	return keys
}

// selectNeighbors 从候选集中选择 M 个最近邻。
func (idx *HNSWIndex) selectNeighbors(candidates []string, m int) []string {
	if len(candidates) <= m {
		return candidates
	}
	return candidates[:m]
}

// trimNeighbors 裁剪邻居列表至 m 个（保留最相似的）。
func (idx *HNSWIndex) trimNeighbors(query []float32, neighbors []string, m int) []string {
	if len(neighbors) <= m {
		return neighbors
	}

	type nbDist struct {
		key  string
		dist float32
	}
	dists := make([]nbDist, 0, len(neighbors))
	for _, nb := range neighbors {
		nbNode := idx.nodes[nb]
		if nbNode == nil {
			continue
		}
		dists = append(dists, nbDist{key: nb, dist: CosineSimilarity(query, nbNode.vector)})
	}

	// 选择排序取前 m
	for i := 0; i < m; i++ {
		maxIdx := i
		for j := i + 1; j < len(dists); j++ {
			if dists[j].dist > dists[maxIdx].dist {
				maxIdx = j
			}
		}
		dists[i], dists[maxIdx] = dists[maxIdx], dists[i]
	}

	result := make([]string, m)
	for i := 0; i < m; i++ {
		result[i] = dists[i].key
	}
	return result
}

// min 返回较小值。
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
