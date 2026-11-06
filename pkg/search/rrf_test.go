package search

import (
	"math"
	"testing"
)

// TestRRFFuse_BothRoutesHitRanksFirst 两路都命中的文档排最前。
func TestRRFFuse_BothRoutesHitRanksFirst(t *testing.T) {
	routes := map[string][]RankedItem{
		"bm25": {
			{ID: "a", Score: 10},
			{ID: "b", Score: 8},
			{ID: "c", Score: 5},
		},
		"vector": {
			{ID: "b", Score: 0.9},
			{ID: "d", Score: 0.7},
			{ID: "a", Score: 0.6},
		},
	}

	fused := RRFFuse(routes, nil, DefaultRRFK, 10)
	if len(fused) != 4 {
		t.Fatalf("len = %d want 4", len(fused))
	}
	if fused[0].ID != "b" {
		t.Errorf("两路命中的 b 应排第一, got %s", fused[0].ID)
	}
}

// TestRRFFuse_ScoreDecreasing 融合分数单调递减。
func TestRRFFuse_ScoreDecreasing(t *testing.T) {
	routes := map[string][]RankedItem{
		"bm25":   {{ID: "x", Score: 1}, {ID: "y", Score: 0.5}},
		"vector": {{ID: "y", Score: 0.9}, {ID: "x", Score: 0.4}},
	}
	fused := RRFFuse(routes, nil, DefaultRRFK, 10)
	for i := 1; i < len(fused); i++ {
		if fused[i].Score > fused[i-1].Score {
			t.Errorf("分数应递减: %v", fused)
		}
	}
}

// TestRRFFuse_TopNLimit 返回条数受限。
func TestRRFFuse_TopNLimit(t *testing.T) {
	routes := map[string][]RankedItem{
		"bm25": {{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}},
	}
	fused := RRFFuse(routes, nil, DefaultRRFK, 2)
	if len(fused) != 2 {
		t.Errorf("len = %d want 2", len(fused))
	}
}

// TestRRFFuse_WeightsRespected 高权重路贡献更大。
func TestRRFFuse_WeightsRespected(t *testing.T) {
	routes := map[string][]RankedItem{
		"bm25":   {{ID: "a"}, {ID: "b"}},
		"vector": {{ID: "b"}, {ID: "a"}},
	}
	weights := map[string]float64{"bm25": 1.0, "vector": 0.1}
	fused := RRFFuse(routes, weights, DefaultRRFK, 10)

	// a 在 bm25 排第 1、vector 排第 2；b 相反。
	// bm25 权重远大于 vector 时，a 应排前
	if fused[0].ID != "a" {
		t.Errorf("bm25 高权重时 a 应排前, got %s", fused[0].ID)
	}
}

// TestRRFFuse_ContributionsRecorded 各路贡献被记录。
func TestRRFFuse_ContributionsRecorded(t *testing.T) {
	routes := map[string][]RankedItem{
		"bm25":   {{ID: "a", Score: 1}},
		"vector": {{ID: "a", Score: 0.9}},
	}
	fused := RRFFuse(routes, nil, DefaultRRFK, 10)
	if len(fused) != 1 {
		t.Fatalf("len = %d", len(fused))
	}
	if len(fused[0].Contributions) != 2 {
		t.Errorf("应记录两路贡献, got %v", fused[0].Contributions)
	}
	if fused[0].Contributions["bm25"] <= 0 || fused[0].Contributions["vector"] <= 0 {
		t.Errorf("贡献值应均为正: %v", fused[0].Contributions)
	}
}

// TestRRFFuse_DuplicateInSameRoute 同一路内重复 ID 只计一次。
func TestRRFFuse_DuplicateInSameRoute(t *testing.T) {
	routes := map[string][]RankedItem{
		"bm25": {{ID: "a"}, {ID: "a"}, {ID: "a"}},
	}
	fused := RRFFuse(routes, nil, DefaultRRFK, 10)
	if len(fused) != 1 {
		t.Errorf("同一 ID 应去重, got %d", len(fused))
	}
	// 只应按首次排名计算一次；weights 为 nil 时默认权重 1.0
	want := 1.0 / (DefaultRRFK + 1)
	if math.Abs(fused[0].Score-want) > 1e-9 {
		t.Errorf("score = %f want %f", fused[0].Score, want)
	}
}

// TestRRFFuse_EmptyRoutes 空输入返回 nil。
func TestRRFFuse_EmptyRoutes(t *testing.T) {
	if got := RRFFuse(nil, nil, DefaultRRFK, 10); got != nil {
		t.Errorf("got %v want nil", got)
	}
	if got := RRFFuse(map[string][]RankedItem{}, nil, DefaultRRFK, 10); len(got) != 0 {
		t.Errorf("len = %d", len(got))
	}
}

// TestRRFFuse_InvalidK 非法 K 用默认值。
func TestRRFFuse_InvalidK(t *testing.T) {
	routes := map[string][]RankedItem{"bm25": {{ID: "a"}}}
	fused := RRFFuse(routes, nil, -1, 10)
	if len(fused) != 1 {
		t.Fatal("应有结果")
	}
	// k 取默认 60
	want := 1.0 / (DefaultRRFK + 1)
	if math.Abs(fused[0].Score-want) > 1e-9 {
		t.Errorf("score = %f want %f", fused[0].Score, want)
	}
}

// TestFuseTwoRoutes 便捷函数。
func TestFuseTwoRoutes(t *testing.T) {
	bm25 := []RankedItem{{ID: "a"}, {ID: "b"}}
	vector := []RankedItem{{ID: "b"}, {ID: "c"}}

	fused := FuseTwoRoutes(bm25, vector, 0.5, 0.5)
	if len(fused) != 3 {
		t.Fatalf("len = %d want 3", len(fused))
	}
	if fused[0].ID != "b" {
		t.Errorf("b 两路命中应排前, got %s", fused[0].ID)
	}
}

// TestNormalizeScores 归一化到 [0,1]。
func TestNormalizeScores(t *testing.T) {
	items := []RankedItem{{ID: "a", Score: 5}, {ID: "b", Score: 10}, {ID: "c", Score: 15}}
	NormalizeScores(items)

	if math.Abs(items[0].Score-0) > 1e-9 {
		t.Errorf("min 应为 0, got %f", items[0].Score)
	}
	if math.Abs(items[2].Score-1) > 1e-9 {
		t.Errorf("max 应为 1, got %f", items[2].Score)
	}
}

// TestNormalizeScores_SameValue 全部相同不除零。
func TestNormalizeScores_SameValue(t *testing.T) {
	items := []RankedItem{{ID: "a", Score: 5}, {ID: "b", Score: 5}}
	NormalizeScores(items)
	if items[0].Score != 5 || items[1].Score != 5 {
		t.Error("相同分数不应被修改")
	}
}

// TestBM25Index_HybridSearch 混合检索。
func TestBM25Index_HybridSearch(t *testing.T) {
	idx := NewBM25Index()
	idx.Add(&Document{ID: "1", Content: "user login failed timeout", Vector: []float32{1, 0, 0}})
	idx.Add(&Document{ID: "2", Content: "database connection error", Vector: []float32{0, 1, 0}})

	cosine := func(a, b []float32) float32 {
		var dot, na, nb float32
		for i := range a {
			dot += a[i] * b[i]
			na += a[i] * a[i]
			nb += b[i] * b[i]
		}
		if na == 0 || nb == 0 {
			return 0
		}
		return dot / (sqrt(na) * sqrt(nb))
	}

	results := idx.HybridSearch("login timeout", []float32{1, 0, 0}, cosine, 5)
	if len(results) == 0 {
		t.Fatal("应有结果")
	}
	if results[0].ID != "1" {
		t.Errorf("doc1 两路命中应排前, got %s", results[0].ID)
	}
}

// TestBM25Index_HybridWithoutVector 无向量时退化为 BM25。
func TestBM25Index_HybridWithoutVector(t *testing.T) {
	idx := NewBM25Index()
	idx.Add(&Document{ID: "1", Content: "timeout error"})
	idx.Add(&Document{ID: "2", Content: "success info"})

	results := idx.HybridSearch("timeout", nil, nil, 5)
	if len(results) == 0 {
		t.Fatal("应有结果")
	}
	if results[0].ID != "1" {
		t.Errorf("got %s", results[0].ID)
	}
}

// TestBM25Index_Concurrency 并发 Add 安全。
func TestBM25Index_Concurrency(t *testing.T) {
	idx := NewBM25Index()
	done := make(chan struct{}, 20)
	for i := 0; i < 20; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			idx.Add(&Document{ID: string(rune('a' + n)), Content: "test document"})
		}(i)
	}
	for i := 0; i < 20; i++ {
		<-done
	}
	if idx.DocCount() != 20 {
		t.Errorf("DocCount = %d want 20", idx.DocCount())
	}
}

// TestRRFusion_ContractAPI 契约 API。
func TestRRFusion_ContractAPI(t *testing.T) {
	bm25 := []SearchResult{{ID: "a", BM25Score: 1.0}}
	vector := []SearchResult{{ID: "a", BM25Score: 0.9}, {ID: "b", BM25Score: 0.5}}

	fused := RRFusion(bm25, vector, 5)
	if len(fused) != 2 {
		t.Fatalf("len = %d", len(fused))
	}
	if fused[0].ID != "a" {
		t.Errorf("got %s", fused[0].ID)
	}
}

// TestHybridSearch_PackageFunc 包级函数。
func TestHybridSearch_PackageFunc(t *testing.T) {
	if got := HybridSearch(nil, "q", nil, nil, 5); got != nil {
		t.Errorf("nil index 应返回 nil")
	}
}

// sqrt 平方根（避免在测试中引入 math 包之外的依赖）。
func sqrt(v float32) float32 {
	if v <= 0 {
		return 0
	}
	z := v
	for i := 0; i < 20; i++ {
		z = (z + v/z) / 2
	}
	return z
}
