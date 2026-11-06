package search

import (
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "英文分词",
			input: "hello world! this is a test.",
			want:  []string{"hello", "world", "this", "is", "a", "test"},
		},
		{
			name:  "中文分词",
			input: "用户登录失败",
			want:  []string{"用", "户", "登", "录", "失", "败"},
		},
		{
			name:  "中英混合",
			input: "ERROR 用户登录失败 timeout",
			want:  []string{"error", "用", "户", "登", "录", "失", "败", "timeout"},
		},
		{
			name:  "空字符串",
			input: "",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Tokenize(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("Tokenize() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Tokenize()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBM25Search(t *testing.T) {
	idx := NewBM25Index()
	idx.Add(&Document{ID: "1", Content: "user login failed with timeout error"})
	idx.Add(&Document{ID: "2", Content: "database connection timeout"})
	idx.Add(&Document{ID: "3", Content: "user registration success"})
	idx.Add(&Document{ID: "4", Content: "payment gateway error"})
	idx.Add(&Document{ID: "5", Content: "user login success"})

	results := idx.Search("login timeout", 3)
	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}

	// 验证结果按相关性排序
	for i := 1; i < len(results); i++ {
		if results[i].BM25Score > results[i-1].BM25Score {
			t.Errorf("results not sorted: %v", results)
		}
	}

	// 验证 login 和 timeout 都命中的文档排在前面
	found := false
	for _, r := range results {
		if r.ID == "1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected doc 1 in results, got %v", results)
	}
}

func TestRRFusion(t *testing.T) {
	bm25 := []SearchResult{
		{ID: "a", BM25Score: 1.0},
		{ID: "b", BM25Score: 0.8},
		{ID: "c", BM25Score: 0.5},
	}
	vector := []SearchResult{
		{ID: "b", BM25Score: 0.9},
		{ID: "d", BM25Score: 0.7},
		{ID: "a", BM25Score: 0.6},
	}

	fused := RRFusion(bm25, vector, 4)
	if len(fused) != 4 {
		t.Errorf("expected 4 unique results, got %d", len(fused))
	}

	// 验证 b 在两路都命中，得分最高
	if fused[0].ID != "b" {
		t.Errorf("expected b first, got %s", fused[0].ID)
	}

	// 验证得分递减
	for i := 1; i < len(fused); i++ {
		if fused[i].Score > fused[i-1].Score {
			t.Errorf("fused results not sorted: %v", fused)
		}
	}
}

func TestHybridSearch(t *testing.T) {
	idx := NewBM25Index()
	idx.Add(&Document{ID: "1", Content: "user login failed timeout", Vector: []float32{1, 0, 0}})
	idx.Add(&Document{ID: "2", Content: "database connection error", Vector: []float32{0, 1, 0}})
	idx.Add(&Document{ID: "3", Content: "payment gateway timeout", Vector: []float32{0, 0, 1}})

	cosine := func(a, b []float32) float32 {
		var dot float32
		for i := range a {
			dot += a[i] * b[i]
		}
		return dot
	}

	results := HybridSearch(idx, "login timeout", []float32{1, 0, 0}, cosine, 2)
	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}

	// 验证 doc 1 排在第一位（BM25 和向量都命中）
	if results[0].ID != "1" {
		t.Errorf("expected doc 1 first, got %s", results[0].ID)
	}
}

func TestBM25IndexConcurrency(t *testing.T) {
	idx := NewBM25Index()
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			idx.Add(&Document{ID: string(rune('a' + n)), Content: "test document"})
			done <- true
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	if idx.docCount != 10 {
		t.Errorf("expected 10 docs, got %d", idx.docCount)
	}
}
