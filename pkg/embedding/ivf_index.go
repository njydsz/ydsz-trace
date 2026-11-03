// Package embedding 子文件：IVF（倒排文件）向量索引（V2.1）。
//
// 简化版 IVF 索引，用于语义检索的近似最近邻加速：
//   - 训练阶段：对已入库向量做 k-means 聚类（k = sqrt(n)，上限 256）
//   - 查询阶段：先找最近的 nprobe 个簇，再在簇内精确 cosine 搜索
//
// 设计约束：
//   - 纯内存索引，不依赖外部库
//   - 支持增量添加（Add）
//   - 训练为懒触发（当样本数 >= minSamplesForTrain 时）
//   - 未训练时退化为全表扫描
package embedding

import (
	"context"
	"math"
	"math/rand"
	"sync"
)

const (
	// ivfDefaultNProbe 默认查询探针簇数
	ivfDefaultNProbe = 4
	// ivfMaxClusters 最大簇数
	ivfMaxClusters = 256
	// ivfMinSamplesForTrain 触发训练的最小样本数
	ivfMinSamplesForTrain = 100
	// ivfMaxIter k-means 最大迭代次数
	ivfMaxIter = 10
)

// IVFIndex IVF 向量索引。
type IVFIndex struct {
	mu sync.RWMutex

	// vectors 全部向量（key -> vector）
	vectors map[string][]float32
	// metadata 元数据（key -> metadata）
	metadata map[string]map[string]string

	// centroids 簇中心（训练后生成）
	centroids [][]float32
	// clusters 簇分配（clusterID -> []key）
	clusters map[int][]string

	// trained 是否已训练
	trained bool
	// nProbe 查询探针簇数
	nProbe int
}

// NewIVFIndex 创建空 IVF 索引。
func NewIVFIndex() *IVFIndex {
	return &IVFIndex{
		vectors:  make(map[string][]float32),
		metadata: make(map[string]map[string]string),
		clusters: make(map[int][]string),
		nProbe:   ivfDefaultNProbe,
	}
}

// Add 增量添加向量（线程安全）。
//
// 添加后若样本数达到阈值则自动触发训练。
func (idx *IVFIndex) Add(key string, vector []float32, metadata map[string]string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	vecCopy := make([]float32, len(vector))
	copy(vecCopy, vector)
	idx.vectors[key] = vecCopy

	if metadata != nil {
		metaCopy := make(map[string]string, len(metadata))
		for k, v := range metadata {
			metaCopy[k] = v
		}
		idx.metadata[key] = metaCopy
	} else {
		idx.metadata[key] = make(map[string]string)
	}

	// 标记需要重新训练
	idx.trained = false

	// 达到阈值时自动训练
	if len(idx.vectors) >= ivfMinSamplesForTrain {
		idx.trainLocked()
	}
}

// Search 检索 topK 相似向量（线程安全）。
//
// 未训练时退化为全表扫描；训练后走 IVF 路径。
func (idx *IVFIndex) Search(ctx context.Context, query []float32, topK int) ([]SearchResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// 未训练或样本太少：全表扫描
	if !idx.trained || len(idx.centroids) == 0 {
		return idx.bruteForceSearch(ctx, query, topK), nil
	}

	// IVF 路径：先找最近簇，再在簇内搜索
	clusterIDs := idx.findNearestClusters(query, idx.nProbe)
	results := make([]SearchResult, 0, topK*2)

	for _, cid := range clusterIDs {
		for _, key := range idx.clusters[cid] {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			vec := idx.vectors[key]
			score := CosineSimilarity(query, vec)
			results = append(results, SearchResult{
				Key:      key,
				Score:    score,
				Metadata: idx.metadata[key],
			})
		}
	}

	return topKResults(results, topK), nil
}

// Count 返回索引中的向量总数。
func (idx *IVFIndex) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.vectors)
}

// IsTrained 返回是否已训练。
func (idx *IVFIndex) IsTrained() bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.trained
}

// bruteForceSearch 全表扫描（退化路径）。
func (idx *IVFIndex) bruteForceSearch(ctx context.Context, query []float32, topK int) []SearchResult {
	results := make([]SearchResult, 0, len(idx.vectors))
	for key, vec := range idx.vectors {
		select {
		case <-ctx.Done():
			return results
		default:
		}
		score := CosineSimilarity(query, vec)
		results = append(results, SearchResult{
			Key:      key,
			Score:    score,
			Metadata: idx.metadata[key],
		})
	}
	return topKResults(results, topK)
}

// findNearestClusters 找到离 query 最近的 n 个簇。
func (idx *IVFIndex) findNearestClusters(query []float32, n int) []int {
	if n <= 0 || len(idx.centroids) == 0 {
		return nil
	}
	if n > len(idx.centroids) {
		n = len(idx.centroids)
	}

	type clusterDist struct {
		id   int
		dist float32
	}
	dists := make([]clusterDist, len(idx.centroids))
	for i, c := range idx.centroids {
		dists[i] = clusterDist{id: i, dist: CosineSimilarity(query, c)}
	}

	// 简单选择排序取前 n（n 通常很小）
	for i := 0; i < n; i++ {
		maxIdx := i
		for j := i + 1; j < len(dists); j++ {
			if dists[j].dist > dists[maxIdx].dist {
				maxIdx = j
			}
		}
		dists[i], dists[maxIdx] = dists[maxIdx], dists[i]
	}

	result := make([]int, n)
	for i := 0; i < n; i++ {
		result[i] = dists[i].id
	}
	return result
}

// trainLocked 训练 k-means 聚类（调用方需持有写锁）。
func (idx *IVFIndex) trainLocked() {
	n := len(idx.vectors)
	if n < ivfMinSamplesForTrain {
		return
	}

	// 确定簇数 k = sqrt(n)，上限 ivfMaxClusters
	k := int(math.Sqrt(float64(n)))
	if k > ivfMaxClusters {
		k = ivfMaxClusters
	}
	if k < 1 {
		k = 1
	}

	// 提取所有向量
	keys := make([]string, 0, n)
	vecs := make([][]float32, 0, n)
	for key, vec := range idx.vectors {
		keys = append(keys, key)
		vecs = append(vecs, vec)
	}

	// 随机初始化中心点
	rng := rand.New(rand.NewSource(42))
	centroids := make([][]float32, k)
	used := make(map[int]bool)
	for i := 0; i < k; i++ {
		var ri int
		for {
			ri = rng.Intn(n)
			if !used[ri] {
				used[ri] = true
				break
			}
		}
		c := make([]float32, len(vecs[ri]))
		copy(c, vecs[ri])
		centroids[i] = c
	}

	// k-means 迭代
	dim := len(vecs[0])
	for iter := 0; iter < ivfMaxIter; iter++ {
		// 分配
		assignments := make([]int, n)
		changed := false
		for i, vec := range vecs {
			bestCluster := 0
			bestScore := float32(-2)
			for ci, centroid := range centroids {
				score := CosineSimilarity(vec, centroid)
				if score > bestScore {
					bestScore = score
					bestCluster = ci
				}
			}
			if assignments[i] != bestCluster {
				changed = true
			}
			assignments[i] = bestCluster
		}

		// 更新中心
		newCentroids := make([][]float32, k)
		counts := make([]int, k)
		for ci := range newCentroids {
			newCentroids[ci] = make([]float32, dim)
		}
		for i, vec := range vecs {
			ci := assignments[i]
			counts[ci]++
			for d := 0; d < dim; d++ {
				newCentroids[ci][d] += vec[d]
			}
		}
		for ci := range newCentroids {
			if counts[ci] > 0 {
				for d := 0; d < dim; d++ {
					newCentroids[ci][d] /= float32(counts[ci])
				}
			}
		}
		centroids = newCentroids

		if !changed {
			break
		}
	}

	// 重建簇分配
	idx.centroids = centroids
	idx.clusters = make(map[int][]string)
	for i, key := range keys {
		ci := 0
		bestScore := float32(-2)
		for cj, centroid := range centroids {
			score := CosineSimilarity(vecs[i], centroid)
			if score > bestScore {
				bestScore = score
				ci = cj
			}
		}
		idx.clusters[ci] = append(idx.clusters[ci], key)
	}
	idx.trained = true
}
