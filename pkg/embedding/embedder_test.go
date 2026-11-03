package embedding

import (
	"context"
	"reflect"
	"testing"
)

// TestCosineSimilarity_Orthogonal 正交向量相似度应为 0。
func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	got := CosineSimilarity(a, b)
	if got != 0 {
		t.Fatalf("期望正交向量相似度为 0, 得到 %f", got)
	}
}

// TestCosineSimilarity_Identical 相同向量相似度应为 1。
func TestCosineSimilarity_Identical(t *testing.T) {
	a := []float32{0.5, 0.5, 0.5}
	got := CosineSimilarity(a, a)
	want := float32(1.0)
	if got != want {
		t.Fatalf("期望相同向量相似度为 %f, 得到 %f", want, got)
	}
}

// TestCosineSimilarity_ZeroVector 零向量应返回 0（不 panic）。
func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}

	got := CosineSimilarity(a, b)
	if got != 0 {
		t.Fatalf("期望零向量相似度为 0, 得到 %f", got)
	}

	got = CosineSimilarity(b, a)
	if got != 0 {
		t.Fatalf("期望反向零向量相似度为 0, 得到 %f", got)
	}
}

// TestCosineSimilarity_LengthMismatch 不等长向量应返回 0（不 panic）。
func TestCosineSimilarity_LengthMismatch(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{1, 0, 0}
	got := CosineSimilarity(a, b)
	if got != 0 {
		t.Fatalf("期望不等长向量相似度为 0, 得到 %f", got)
	}
}

// TestNewEmbedder_InvalidProvider 无效 Provider 应返回错误。
func TestNewEmbedder_InvalidProvider(t *testing.T) {
	_, err := NewEmbedder(Provider(99))
	if err == nil {
		t.Fatal("期望无效 Provider 返回错误，但得到 nil")
	}
	if err != ErrInvalidProvider {
		t.Fatalf("期望错误 %v, 得到 %v", ErrInvalidProvider, err)
	}
}

// TestNewEmbedder_LocalMissingEndpoint Local 缺少 endpoint 应报错。
func TestNewEmbedder_LocalMissingEndpoint(t *testing.T) {
	_, err := NewEmbedder(ProviderLocal, WithEndpoint(""))
	if err != ErrMissingEndpoint {
		t.Fatalf("期望 %v, 得到 %v", ErrMissingEndpoint, err)
	}
}

// TestNewEmbedder_LocalValid 正常创建 Local embedder。
func TestNewEmbedder_LocalValid(t *testing.T) {
	emb, err := NewEmbedder(ProviderLocal, WithEndpoint("http://localhost:11434"))
	if err != nil {
		t.Fatalf("期望成功创建 Local embedder, 得到错误: %v", err)
	}
	if emb.Dimension() != DefaultDimension {
		t.Fatalf("期望维度 %d, 得到 %d", DefaultDimension, emb.Dimension())
	}
}

// TestInMemoryStore_Search 内存存储检索。
func TestInMemoryStore_Search(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()

	vecA := []float32{1, 0, 0, 0}
	vecB := []float32{0, 1, 0, 0}
	vecC := []float32{0.9, 0.1, 0, 0}

	if err := store.Store(ctx, "a", vecA, nil); err != nil {
		t.Fatalf("store a failed: %v", err)
	}
	if err := store.Store(ctx, "b", vecB, nil); err != nil {
		t.Fatalf("store b failed: %v", err)
	}
	if err := store.Store(ctx, "c", vecC, nil); err != nil {
		t.Fatalf("store c failed: %v", err)
	}

	results, err := store.Search(ctx, vecA, 2)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("期望 2 条结果，得到 %d", len(results))
	}

	if results[0].Key != "a" {
		t.Fatalf("期望第一条为 'a', 得到 '%s'", results[0].Key)
	}
}

// TestSQLStore_SerializationRoundTrip 向量序列化往返测试。
func TestSQLStore_SerializationRoundTrip(t *testing.T) {
	original := []float32{0.1, -0.2, 0.3, -0.4, 0.5}

	blob, err := vectorToBlob(original)
	if err != nil {
		t.Fatalf("vectorToBlob failed: %v", err)
	}

	if len(blob) != 4*len(original) {
		t.Fatalf("期望 blob 长度 %d, 得到 %d", 4*len(original), len(blob))
	}

	restored, err := blobToVector(blob, len(original))
	if err != nil {
		t.Fatalf("blobToVector failed: %v", err)
	}

	if !reflect.DeepEqual(original, restored) {
		t.Fatalf("序列化往返不一致:\n原始: %v\n恢复: %v", original, restored)
	}
}

// TestSQLStore_NilDB nil *sql.DB 构造应报错。
func TestSQLStore_NilDB(t *testing.T) {
	_, err := NewSQLStore(nil)
	if err == nil {
		t.Fatal("期望 nil DB 返回错误，但得到 nil")
	}
}
