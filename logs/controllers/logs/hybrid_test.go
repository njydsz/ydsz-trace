package logs

import (
	"testing"

	"ydsz-trace/pkg/search"
)

// TestHybridResult_Fields 混合结果字段完整。
func TestHybridResult_Fields(t *testing.T) {
	r := HybridResult{
		TargetID:     "tpl-1",
		RRFScore:     0.032,
		BM25Score:    4.5,
		VectorScore:  0.87,
		FromKeywords: true,
		FromVector:   true,
	}
	if !r.FromKeywords || !r.FromVector {
		t.Error("标记位错误")
	}
	if r.RRFScore <= 0 {
		t.Error("RRF 分数应为正")
	}
}

// TestBuildHybridResults_EmptyVector 向量结果为空时不执行融合。
func TestBuildHybridResults_EmptyVector(t *testing.T) {
	req := SemanticSearchReq{Query: "timeout", Type: "template", TopK: 10}
	got := buildHybridResults(nil, req, nil)
	if got != nil {
		t.Errorf("向量结果为空应返回 nil, got %v", got)
	}
}

// TestSearchRRFIntegration 与 pkg/search 的端到端集成。
func TestSearchRRFIntegration(t *testing.T) {
	idx := search.NewBM25Index()
	idx.Add(&search.Document{ID: "a", Content: "ERROR connection timeout"})
	idx.Add(&search.Document{ID: "b", Content: "INFO request completed"})
	idx.Add(&search.Document{ID: "c", Content: "ERROR database connection refused"})

	// BM25 路
	bm25Results := idx.Search("connection error", 10)

	// 模拟向量路（只召回 a、c）
	vectorItems := []search.SearchResult{
		{ID: "a", BM25Score: 0.9, VectorScore: 0.9},
		{ID: "c", BM25Score: 0.7, VectorScore: 0.7},
	}

	fused := search.RRFusion(bm25Results, vectorItems, 10)
	if len(fused) == 0 {
		t.Fatal("应有融合结果")
	}

	// 分数递减
	for i := 1; i < len(fused); i++ {
		if fused[i].Score > fused[i-1].Score {
			t.Error("分数应递减")
		}
	}
}

// TestHybridScoreMonotonic 混合分数单调递减。
func TestHybridScoreMonotonic(t *testing.T) {
	bm25 := []search.SearchResult{{ID: "a", BM25Score: 5}, {ID: "b", BM25Score: 3}, {ID: "c", BM25Score: 1}}
	vector := []search.SearchResult{{ID: "b", BM25Score: 0.9}, {ID: "a", BM25Score: 0.8}}

	fused := search.RRFusion(bm25, vector, 10)
	if len(fused) != 3 {
		t.Fatalf("len = %d want 3", len(fused))
	}
	for i := 1; i < len(fused); i++ {
		if fused[i].Score > fused[i-1].Score {
			t.Errorf("第 %d 项分数上升: %v", i, fused)
		}
	}
}
