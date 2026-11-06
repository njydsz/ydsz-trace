package agentlink

import (
	"os"
	"path/filepath"
	"testing"

	"ydsz-trace/pkg/agentlink"
)

// TestNewSpoolManager_Disabled 禁用时返回 nil。
func TestNewSpoolManager_Disabled(t *testing.T) {
	m, err := NewSpoolManager(SpoolConfig{Enabled: false})
	if err != nil {
		t.Fatalf("禁用时不应返回错误: %v", err)
	}
	if m != nil {
		t.Error("禁用时应返回 nil")
	}
}

// TestNewSpoolManager_Enabled 启用时创建成功。
func TestNewSpoolManager_Enabled(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSpoolManager(SpoolConfig{
		Enabled:  true,
		Capacity: 100,
		Path:     filepath.Join(dir, "spool.bin"),
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	defer func() {
		if cerr := m.Close(); cerr != nil {
			t.Errorf("关闭失败: %v", cerr)
		}
	}()

	stats := m.Stats()
	if stats.Capacity != 100 {
		t.Errorf("capacity = %d want 100", stats.Capacity)
	}
	if stats.Count != 0 {
		t.Errorf("初始 count = %d want 0", stats.Count)
	}
}

// TestSpoolManager_EnqueueDequeue 入队出队。
func TestSpoolManager_EnqueueDequeue(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSpoolManager(SpoolConfig{
		Enabled:  true,
		Capacity: 100,
		Path:     filepath.Join(dir, "spool.bin"),
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	defer func() {
		if cerr := m.Close(); cerr != nil {
			t.Errorf("关闭失败: %v", cerr)
		}
	}()

	batch := &agentlink.LogBatch{
		BatchNo:  "test-1",
		ItemID:   1,
		Lines:    []string{"line1", "line2"},
		ClientID: 42,
	}

	if err := m.enqueue(batch); err != nil {
		t.Fatalf("入队失败: %v", err)
	}

	got, err := m.dequeue()
	if err != nil {
		t.Fatalf("出队失败: %v", err)
	}
	if got.BatchNo != "test-1" {
		t.Errorf("BatchNo = %q want test-1", got.BatchNo)
	}
	if len(got.Lines) != 2 {
		t.Errorf("lines = %d want 2", len(got.Lines))
	}
}

// TestSpoolManager_FullEvictsOldest 队列满时丢弃最旧数据。
func TestSpoolManager_FullEvictsOldest(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSpoolManager(SpoolConfig{
		Enabled:  true,
		Capacity: 3,
		Path:     filepath.Join(dir, "spool.bin"),
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	defer func() {
		if cerr := m.Close(); cerr != nil {
			t.Errorf("关闭失败: %v", cerr)
		}
	}()

	// 写入 3 条（满）。
	for i := 0; i < 3; i++ {
		b := &agentlink.LogBatch{BatchNo: "batch-" + string(rune('a'+i))}
		if err := m.enqueue(b); err != nil {
			t.Fatalf("入队 %d 失败: %v", i, err)
		}
	}

	// 再写入 1 条，应丢弃最旧的 batch-a。
	b4 := &agentlink.LogBatch{BatchNo: "batch-d"}
	if err := m.enqueue(b4); err != nil {
		t.Fatalf("入队 batch-d 失败: %v", err)
	}

	// 出队应得到 batch-b（batch-a 被丢弃）。
	got, err := m.dequeue()
	if err != nil {
		t.Fatalf("出队失败: %v", err)
	}
	if got.BatchNo != "batch-b" {
		t.Errorf("首条 = %q want batch-b（batch-a 应被丢弃）", got.BatchNo)
	}
}

// TestSpoolManager_DrainTo 补传全部批次。
func TestSpoolManager_DrainTo(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSpoolManager(SpoolConfig{
		Enabled:  true,
		Capacity: 100,
		Path:     filepath.Join(dir, "spool.bin"),
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	defer func() {
		if cerr := m.Close(); cerr != nil {
			t.Errorf("关闭失败: %v", cerr)
		}
	}()

	for i := 0; i < 5; i++ {
		b := &agentlink.LogBatch{BatchNo: "batch-" + string(rune('a'+i))}
		if err := m.enqueue(b); err != nil {
			t.Fatalf("入队 %d 失败: %v", i, err)
		}
	}

	var sent []*agentlink.LogBatch
	drained := m.drainTo(func(b *agentlink.LogBatch) error {
		sent = append(sent, b)
		return nil
	}, 10)

	if drained != 5 {
		t.Errorf("drained = %d want 5", drained)
	}
	if len(sent) != 5 {
		t.Errorf("sent = %d want 5", len(sent))
	}
}

// TestSpoolManager_Persistence 重启后数据不丢失。
func TestSpoolManager_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spool.bin")

	// 第一轮：写入 3 条。
	m1, err := NewSpoolManager(SpoolConfig{Enabled: true, Capacity: 100, Path: path})
	if err != nil {
		t.Fatalf("第一轮创建失败: %v", err)
	}
	for i := 0; i < 3; i++ {
		b := &agentlink.LogBatch{BatchNo: "persist-" + string(rune('a'+i))}
		if err := m1.enqueue(b); err != nil {
			t.Fatalf("入队 %d 失败: %v", i, err)
		}
	}
	if err := m1.Close(); err != nil {
		t.Fatalf("第一轮关闭失败: %v", err)
	}

	// 第二轮：重新打开，应能读出 3 条。
	m2, err := NewSpoolManager(SpoolConfig{Enabled: true, Capacity: 100, Path: path})
	if err != nil {
		t.Fatalf("第二轮创建失败: %v", err)
	}
	defer func() {
		if cerr := m2.Close(); cerr != nil {
			t.Errorf("第二轮关闭失败: %v", cerr)
		}
	}()

	stats := m2.Stats()
	if stats.Count != 3 {
		t.Fatalf("重启后 count = %d want 3", stats.Count)
	}
}

// TestClient_SubmitBatch_WithSpool 内存队列满时写入磁盘。
func TestClient_SubmitBatch_WithSpool(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSpoolManager(SpoolConfig{
		Enabled:  true,
		Capacity: 10,
		Path:     filepath.Join(dir, "spool.bin"),
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	defer func() {
		if cerr := m.Close(); cerr != nil {
			t.Errorf("关闭失败: %v", cerr)
		}
	}()

	c := NewClient("127.0.0.1:2021", "vkey", "regkey", "2020")
	c.SetSpool(m)

	// 提交超过内存队列容量的批次（sendCh 容量 256，直接提交 300 个）。
	for i := 0; i < 300; i++ {
		b := &agentlink.LogBatch{BatchNo: "batch-" + string(rune('a'+i%26))}
		c.SubmitBatch(b)
	}

	// 磁盘队列应有数据（300 - 256 = 44 条）。
	stats := m.Stats()
	if stats.Count == 0 {
		t.Error("内存队列满后磁盘队列应有数据")
	}
}

// TestClient_SpoolStats 统计接口。
func TestClient_SpoolStats(t *testing.T) {
	c := NewClient("127.0.0.1:2021", "vkey", "regkey", "2020")
	_, ok := c.SpoolStats()
	if ok {
		t.Error("未设置 spool 时应返回 false")
	}

	dir := t.TempDir()
	m, err := NewSpoolManager(SpoolConfig{Enabled: true, Capacity: 10, Path: filepath.Join(dir, "spool.bin")})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	defer func() {
		if cerr := m.Close(); cerr != nil {
			t.Errorf("关闭失败: %v", cerr)
		}
	}()

	c.SetSpool(m)
	stats, ok := c.SpoolStats()
	if !ok {
		t.Fatal("设置 spool 后应返回 true")
	}
	if stats.Capacity != 10 {
		t.Errorf("capacity = %d want 10", stats.Capacity)
	}
}

// TestSpoolManager_NilSafe nil 接收器安全。
func TestSpoolManager_NilSafe(t *testing.T) {
	var m *SpoolManager
	if err := m.Close(); err != nil {
		t.Errorf("nil Close 应返回 nil: %v", err)
	}
	stats := m.Stats()
	if stats.Capacity != 0 {
		t.Errorf("nil Stats capacity = %d want 0", stats.Capacity)
	}
	if m.drainTo(func(*agentlink.LogBatch) error { return nil }, 10) != 0 {
		t.Error("nil drainTo 应返回 0")
	}
}

// TestSpoolManager_CorruptedData 损坏数据应被丢弃。
func TestSpoolManager_CorruptedData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spool.bin")

	// 手动写入损坏数据。
	m, err := NewSpoolManager(SpoolConfig{Enabled: true, Capacity: 10, Path: path})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	// 写入合法批次。
	if err := m.enqueue(&agentlink.LogBatch{BatchNo: "good"}); err != nil {
		t.Fatalf("入队失败: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	// 重新打开，读取应成功（数据合法）。
	m2, err := NewSpoolManager(SpoolConfig{Enabled: true, Capacity: 10, Path: path})
	if err != nil {
		t.Fatalf("重新打开失败: %v", err)
	}
	defer func() {
		if cerr := m2.Close(); cerr != nil {
			t.Errorf("关闭失败: %v", cerr)
		}
	}()

	got, err := m2.dequeue()
	if err != nil {
		t.Fatalf("出队失败: %v", err)
	}
	if got.BatchNo != "good" {
		t.Errorf("BatchNo = %q want good", got.BatchNo)
	}
}

// TestSpoolManager_EmptyDequeue 空队列出队返回 EOF。
func TestSpoolManager_EmptyDequeue(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSpoolManager(SpoolConfig{
		Enabled:  true,
		Capacity: 10,
		Path:     filepath.Join(dir, "spool.bin"),
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	defer func() {
		if cerr := m.Close(); cerr != nil {
			t.Errorf("关闭失败: %v", cerr)
		}
	}()

	_, err = m.dequeue()
	if err == nil {
		t.Error("空队列出队应返回错误")
	}
}

// TestSpoolManager_DefaultPath 默认路径（环境变量）。
func TestSpoolManager_DefaultPath(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("YDSZ_SPOOL_DIR", dir)
	defer func() {
		if uerr := os.Unsetenv("YDSZ_SPOOL_DIR"); uerr != nil {
			t.Logf("unsetenv 失败: %v", uerr)
		}
	}()

	m, err := NewSpoolManager(SpoolConfig{Enabled: true, Capacity: 10})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	defer func() {
		if cerr := m.Close(); cerr != nil {
			t.Errorf("关闭失败: %v", cerr)
		}
	}()

	stats := m.Stats()
	if stats.Path == "" {
		t.Error("默认路径不应为空")
	}
	if filepath.Dir(stats.Path) != dir {
		t.Errorf("路径目录 = %q want %q", filepath.Dir(stats.Path), dir)
	}
}
