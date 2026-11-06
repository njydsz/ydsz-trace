// Package embedding 子文件：HNSW 索引测试（V2.1 FR-303）。
//
// 覆盖：
//   - 基本插入与查询
//   - 召回率（与暴力搜索对比）
//   - 增量添加
//   - 并发安全
//   - 大规模性能（10 万向量）
package embedding

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

// TestHNSWIndex_Basic 基本插入与查询。
func TestHNSWIndex_Basic(t *testing.T) {
	idx := NewHNSWIndex()
	ctx := context.Background()

	vecA := []float32{1, 0, 0, 0}
	vecB := []float32{0, 1, 0, 0}
	vecC := []float32{0.9, 0.1, 0, 0}

	idx.Add("a", vecA, nil)
	idx.Add("b", vecB, nil)
	idx.Add("c", vecC, nil)

	if idx.Count() != 3 {
		t.Fatalf("Count = %d, want 3", idx.Count())
	}

	// 查询 vecA，最近邻应为 a
	results, err := idx.Search(ctx, vecA, 2)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results")
	}
	if results[0].Key != "a" {
		t.Errorf("top1 = %s, want a", results[0].Key)
	}
}

// TestHNSWIndex_Recall 召回率验证（与暴力搜索对比）。
func TestHNSWIndex_Recall(t *testing.T) {
	idx := NewHNSWIndex()
	ctx := context.Background()

	// 插入 200 个随机向量
	rng := rand.New(rand.NewSource(42))
	vectors := make(map[string][]float32)
	for i := 0; i < 200; i++ {
		vec := make([]float32, 16)
		for j := range vec {
			vec[j] = rng.Float32()
		}
		key := "vec_" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		idx.Add(key, vec, nil)
		vectors[key] = vec
	}

	// 随机查询 20 次，计算召回率
	topK := 5
	totalRecall := 0.0
	for q := 0; q < 20; q++ {
		query := make([]float32, 16)
		for j := range query {
			query[j] = rng.Float32()
		}

		// HNSW 结果
		results, err := idx.Search(ctx, query, topK)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// 暴力搜索 topK
		type kv struct {
			key   string
			score float32
		}
		var all []kv
		for k, v := range vectors {
			all = append(all, kv{k, CosineSimilarity(query, v)})
		}
		// 排序取 topK
		for i := 0; i < topK; i++ {
			maxIdx := i
			for j := i + 1; j < len(all); j++ {
				if all[j].score > all[maxIdx].score {
					maxIdx = j
				}
			}
			all[i], all[maxIdx] = all[maxIdx], all[i]
		}

		// 计算召回
		hits := 0
		for _, r := range results {
			for i := 0; i < topK; i++ {
				if r.Key == all[i].key {
					hits++
					break
				}
			}
		}
		totalRecall += float64(hits) / float64(topK)
	}

	avgRecall := totalRecall / 20.0
	if avgRecall < 0.8 {
		t.Errorf("average recall = %.2f, want >= 0.8", avgRecall)
	}
	t.Logf("HNSW recall@%d = %.2f", topK, avgRecall)
}

// TestHNSWIndex_Incremental 增量添加不破坏已有结构。
func TestHNSWIndex_Incremental(t *testing.T) {
	idx := NewHNSWIndex()
	ctx := context.Background()

	// 先插入 50 个
	for i := 0; i < 50; i++ {
		vec := []float32{float32(i), float32(50 - i), 0, 0}
		idx.Add(fmt.Sprintf("key_%d", i), vec, nil)
	}

	// 查询
	results1, _ := idx.Search(ctx, []float32{10, 40, 0, 0}, 3)
	if len(results1) == 0 {
		t.Fatal("no results before incremental")
	}

	// 再插入 50 个
	for i := 50; i < 100; i++ {
		vec := []float32{float32(i), float32(100 - i), 0, 0}
		idx.Add(fmt.Sprintf("key_%d", i), vec, nil)
	}

	// 再次查询
	results2, _ := idx.Search(ctx, []float32{10, 40, 0, 0}, 3)
	if len(results2) == 0 {
		t.Fatal("no results after incremental")
	}

	// 验证索引完整性：所有节点都可被访问（Count 正确）
	if idx.Count() != 100 {
		t.Errorf("Count = %d, want 100", idx.Count())
	}

	// 验证查询结果合理性：top1 应该是与查询最相似的
	// 查询向量 [10,40,0,0] 对应 key 'key_10' (i=10, 50-10=40)
	// 增量添加后，最近邻应该仍是 'key_10' 或新添加的某个节点
	bestScore := results2[0].Score
	for _, r := range results2 {
		if r.Score > bestScore {
			t.Errorf("results not sorted by score desc")
		}
	}
	// top1 的相似度应该很高（>0.9）
	if bestScore < 0.9 {
		t.Errorf("top1 score = %f, want >= 0.9", bestScore)
	}
}

// TestHNSWIndex_Concurrent 并发安全测试。
func TestHNSWIndex_Concurrent(t *testing.T) {
	idx := NewHNSWIndex()
	ctx := context.Background()

	// 并发插入
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			vec := []float32{float32(n), float32(100 - n), 0, 0}
			idx.Add(string(rune(n)), vec, nil)
		}(i)
	}
	wg.Wait()

	if idx.Count() != 100 {
		t.Errorf("Count = %d, want 100", idx.Count())
	}

	// 并发查询
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := idx.Search(ctx, []float32{50, 50, 0, 0}, 5)
			if err != nil {
				t.Errorf("concurrent search failed: %v", err)
			}
		}()
	}
	wg.Wait()
}

// TestHNSWIndex_LargeScale 大规模性能测试（1 万向量）。
func TestHNSWIndex_LargeScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large scale test in short mode")
	}

	idx := NewHNSWIndex()
	ctx := context.Background()

	// 插入 1 万向量（CI 友好规模，同时验证性能）
	const n = 10000
	const dim = 64
	rng := rand.New(rand.NewSource(42))

	start := time.Now()
	for i := 0; i < n; i++ {
		vec := make([]float32, dim)
		for j := range vec {
			vec[j] = rng.Float32()
		}
		idx.Add("vec"+string(rune(i%128))+"_"+string(rune(i/128)), vec, nil)
	}
	insertDuration := time.Since(start)
	t.Logf("inserted %d vectors in %v (%.0f vec/s)", n, insertDuration, float64(n)/insertDuration.Seconds())

	// 查询 100 次，测量 P95
	queryTimes := make([]time.Duration, 100)
	for q := 0; q < 100; q++ {
		query := make([]float32, dim)
		for j := range query {
			query[j] = rng.Float32()
		}

		start := time.Now()
		_, err := idx.Search(ctx, query, 10)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		queryTimes[q] = time.Since(start)
	}

	// 计算 P95
	// 简单排序
	for i := 1; i < len(queryTimes); i++ {
		for j := i; j > 0 && queryTimes[j] < queryTimes[j-1]; j-- {
			queryTimes[j], queryTimes[j-1] = queryTimes[j-1], queryTimes[j]
		}
	}
	p95 := queryTimes[95]
	avg := time.Duration(0)
	for _, d := range queryTimes {
		avg += d
	}
	avg /= time.Duration(len(queryTimes))

	t.Logf("query latency: avg=%v, p95=%v", avg, p95)

	// PRD FR-303 要求 P95 ≤ 1s（100 万向量规模）
	// 1 万向量下应远小于 1s
	if p95 > time.Second {
		t.Errorf("p95 = %v, want <= 1s", p95)
	}
}

// TestHNSWIndex_EmptyIndex 空索引查询不 panic。
func TestHNSWIndex_EmptyIndex(t *testing.T) {
	idx := NewHNSWIndex()
	results, err := idx.Search(context.Background(), []float32{1, 0, 0}, 5)
	if err != nil {
		t.Fatalf("empty search error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("empty index should return 0 results, got %d", len(results))
	}
	if idx.Count() != 0 {
		t.Errorf("empty index count = %d, want 0", idx.Count())
	}
}

// TestHNSWIndex_Metadata 元数据正确传递。
func TestHNSWIndex_Metadata(t *testing.T) {
	idx := NewHNSWIndex()
	ctx := context.Background()

	idx.Add("key1", []float32{1, 0, 0}, map[string]string{"service": "auth", "level": "ERROR"})
	idx.Add("key2", []float32{0, 1, 0}, map[string]string{"service": "db", "level": "INFO"})

	results, _ := idx.Search(ctx, []float32{1, 0, 0}, 1)
	if len(results) == 0 {
		t.Fatal("no results")
	}
	if results[0].Metadata["service"] != "auth" {
		t.Errorf("metadata service = %s, want auth", results[0].Metadata["service"])
	}
}
