package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeLogFile 写入测试日志文件。
func writeLogFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}
	return path
}

// TestLocalIndex_BuildAndQuery 索引构建与查询。
func TestLocalIndex_BuildAndQuery(t *testing.T) {
	path := writeLogFile(t, strings.Join([]string{
		"2026-11-06 10:00:00 ERROR connection timeout after 30s",
		"2026-11-06 10:00:01 INFO request completed",
		"2026-11-06 10:00:02 ERROR database connection refused",
		"",
	}, "\n"))

	m := NewLocalIndexManager()
	idx, err := m.LoadOrBuild(path)
	if err != nil {
		t.Fatalf("构建索引失败: %v", err)
	}

	// 文件中确实存在的关键词应返回 true
	for _, kw := range []string{"connection", "timeout", "database", "refused", "error"} {
		if !idx.MayContain(kw) {
			t.Errorf("MayContain(%q) = false，应为 true", kw)
		}
	}

	// 文件中不存在的关键词应返回 false（Bloom 无假阴性）
	for _, kw := range []string{"kubernetes", "zookeeper", "elasticsearch"} {
		if idx.MayContain(kw) {
			t.Errorf("MayContain(%q) = true，应为 false", kw)
		}
	}
}

// TestLocalIndex_CacheHit 第二次加载走缓存。
func TestLocalIndex_CacheHit(t *testing.T) {
	path := writeLogFile(t, "ERROR something failed\n")

	m := NewLocalIndexManager()
	first, err := m.LoadOrBuild(path)
	if err != nil {
		t.Fatalf("首次加载失败: %v", err)
	}
	second, err := m.LoadOrBuild(path)
	if err != nil {
		t.Fatalf("二次加载失败: %v", err)
	}

	// 文件未变化时应返回同一实例（缓存命中）
	if first != second {
		t.Error("文件未变化时应命中缓存")
	}
}

// TestLocalIndex_RebuildWhenChanged 文件变化后重建。
func TestLocalIndex_RebuildWhenChanged(t *testing.T) {
	path := writeLogFile(t, "ERROR alpha\n")

	m := NewLocalIndexManager()
	idx, err := m.LoadOrBuild(path)
	if err != nil {
		t.Fatalf("首次加载失败: %v", err)
	}

	// 修改文件
	if err := os.WriteFile(path, []byte("ERROR beta gamma delta\n"), 0o644); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	rebuilt, err := m.LoadOrBuild(path)
	if err != nil {
		t.Fatalf("重建失败: %v", err)
	}
	if rebuilt == idx {
		t.Error("文件变化后应重建索引")
	}
}

// TestLocalIndex_SaveAndLoadFromDisk 索引持久化。
func TestLocalIndex_SaveAndLoadFromDisk(t *testing.T) {
	path := writeLogFile(t, "ERROR persist me please\n")

	m := NewLocalIndexManager()
	idx, err := m.LoadOrBuild(path)
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}
	if err := idx.SaveToDisk(); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 新管理器从磁盘加载
	m2 := NewLocalIndexManager()
	loaded, err := m2.LoadOrBuild(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if !loaded.MayContain("persist") {
		t.Error("从磁盘加载后应能查询到关键词")
	}
}

// TestLocalIndex_MayMatch OR 语义。
func TestLocalIndex_MayMatch(t *testing.T) {
	path := writeLogFile(t, "ERROR connection timeout\n")
	m := NewLocalIndexManager()
	idx, err := m.LoadOrBuild(path)
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}

	if !idx.MayMatch([]string{"connection", "kubernetes"}) {
		t.Error("任一命中即应返回 true")
	}
	if idx.MayMatch([]string{"kubernetes", "zookeeper"}) {
		t.Error("全部不命中应返回 false")
	}
}

// TestLocalIndex_MustSkipAll AND 语义。
func TestLocalIndex_MustSkipAll(t *testing.T) {
	path := writeLogFile(t, "ERROR connection timeout\n")
	m := NewLocalIndexManager()
	idx, err := m.LoadOrBuild(path)
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}

	// 全部命中 → 不可跳过
	if idx.MustSkipAll([]string{"connection", "timeout"}) {
		t.Error("全部命中时不应跳过")
	}
	// 含不可能命中词 → 不可跳过（AND 语义要求全部可能命中才需保留）
	if idx.MustSkipAll([]string{"connection", "kubernetes"}) {
		t.Error("部分命中时不应跳过")
	}
	// 全部不可能命中 → 可跳过
	if !idx.MustSkipAll([]string{"kubernetes", "zookeeper"}) {
		t.Error("全部不可能命中时应跳过")
	}
}

// TestLocalIndex_NoFalseNegative 关键保证：Bloom 无假阴性。
func TestLocalIndex_NoFalseNegative(t *testing.T) {
	var lines []string
	for i := 0; i < 500; i++ {
		lines = append(lines, "ERROR request failed with code ABC123XYZ")
	}
	path := writeLogFile(t, strings.Join(lines, "\n"))

	m := NewLocalIndexManager()
	idx, err := m.LoadOrBuild(path)
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}

	if !idx.MayContain("abc123xyz") {
		t.Error("必须保证无假阴性：文件中确实存在的词不应被判为不存在")
	}
}

// TestExtractQueryTokens 查询关键词提取。
func TestExtractQueryTokens(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"connection timeout", []string{"connection", "timeout"}},
		{"ERROR", []string{"error"}},
		{"ab", nil},                      // 过短
		{"", nil},                        // 空
		{"level:ERROR message", []string{"level", "error", "message"}},
	}

	for _, c := range cases {
		got := ExtractQueryTokens(c.in)
		if len(got) != len(c.want) {
			t.Errorf("ExtractQueryTokens(%q) = %v want %v", c.in, got, c.want)
			continue
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("idx %d: got %q want %q", i, got[i], c.want[i])
			}
		}
	}
}

// TestPushdownFilter_ShouldSkipFile 下推过滤器。
func TestPushdownFilter_ShouldSkipFile(t *testing.T) {
	path := writeLogFile(t, "ERROR connection timeout\n")
	m := NewLocalIndexManager()

	filter := NewPushdownFilter(m, "kubernetes zookeeper", false)
	if !filter.ShouldSkipFile(path) {
		t.Error("文件不含查询词时应跳过")
	}

	filter2 := NewPushdownFilter(m, "connection", false)
	if filter2.ShouldSkipFile(path) {
		t.Error("文件含查询词时不应跳过")
	}
}

// TestPushdownFilter_NoKeyNotSkipped 无关键词时不跳过。
func TestPushdownFilter_NoKeyNotSkipped(t *testing.T) {
	path := writeLogFile(t, "ERROR anything\n")
	m := NewLocalIndexManager()

	filter := NewPushdownFilter(m, "", false)
	if filter.ShouldSkipFile(path) {
		t.Error("无关键词时无法下推，不应跳过")
	}
}

// TestPushdownFilter_NilManager 未启用索引时不跳过。
func TestPushdownFilter_NilManager(t *testing.T) {
	filter := NewPushdownFilter(nil, "timeout", false)
	if filter.ShouldSkipFile("/any") {
		t.Error("未启用索引时不应跳过")
	}
}

// TestPushdownFilter_MissingFile 文件不存在时不跳过（保守）。
func TestPushdownFilter_MissingFile(t *testing.T) {
	m := NewLocalIndexManager()
	filter := NewPushdownFilter(m, "timeout", false)
	if filter.ShouldSkipFile("/nonexistent/file.log") {
		t.Error("文件不存在时应保守处理（不跳过）")
	}
}

// TestIsIndexFile 索引文件判定。
func TestIsIndexFile(t *testing.T) {
	if !IsIndexFile("/x/app.log.ydsz.idx") {
		t.Error("应识别为索引文件")
	}
	if IsIndexFile("/x/app.log") {
		t.Error("普通日志不应识别为索引文件")
	}
}

// TestIndexFileFor 索引文件路径。
func TestIndexFileFor(t *testing.T) {
	if got := IndexFileFor("/x/app.log"); got != "/x/app.log"+IndexFileSuffix {
		t.Errorf("got %q", got)
	}
}

// TestExtractIndexTokens 行内关键词提取。
func TestExtractIndexTokens(t *testing.T) {
	tokens := extractIndexTokens("ERROR connection refused to redis-server")
	joined := strings.Join(tokens, ",")

	for _, want := range []string{"error", "connection", "refused", "redis-server"} {
		if !strings.Contains(joined, want) {
			t.Errorf("tokens = %q 缺少 %q", joined, want)
		}
	}
}

// TestExtractIndexTokens_Dedup 同一行内重复词去重。
func TestExtractIndexTokens_Dedup(t *testing.T) {
	tokens := extractIndexTokens("timeout timeout timeout")
	count := 0
	for _, t := range tokens {
		if t == "timeout" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("同一行内重复词应去重, count = %d", count)
	}
}

// TestOptimalHashFuncs 哈希函数个数合理。
func TestOptimalHashFuncs(t *testing.T) {
	if got := optimalHashFuncs(10); got < 1 || got > 8 {
		t.Errorf("got %d", got)
	}
	if got := optimalHashFuncs(0); got != 1 {
		t.Errorf("got %d want 1", got)
	}
}

// TestLocalIndex_MaxKeysLimit 超出上限后停止新增。
func TestLocalIndex_MaxKeysLimit(t *testing.T) {
	path := writeLogFile(t, strings.Repeat("unique_token_001\n", 200))

	m := NewLocalIndexManager()
	m.maxKeys = 5 // 强制小上限
	idx, err := m.LoadOrBuild(path)
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}
	if idx.KeyCount > idx.MaxKeys {
		t.Errorf("KeyCount = %d 超过 MaxKeys = %d", idx.KeyCount, idx.MaxKeys)
	}
}
