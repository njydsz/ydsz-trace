package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ===== ParsePipeline 测试 =====

func TestParsePipeline_ProcessLine(t *testing.T) {
	p := NewParsePipeline()

	tests := []struct {
		name    string
		line    string
		wantErr bool
	}{
		{"正常日志行", "2024-01-15 10:30:45 INFO request completed", false},
		{"空行", "", true},
		{"纯空格", "   ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ProcessLine(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProcessLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result.Template == "" {
				t.Errorf("ProcessLine() 返回空模板")
			}
		})
	}
}

func TestParsePipeline_ProcessBatch(t *testing.T) {
	p := NewParsePipeline()

	lines := []string{
		"2024-01-15 10:30:45 INFO request completed",
		"2024-01-15 10:30:46 INFO request completed",
		"2024-01-15 10:30:47 ERROR connection failed",
	}

	results := p.ProcessBatch(lines)
	if len(results) != len(lines) {
		t.Errorf("ProcessBatch() 返回数量 = %d, 期望 %d", len(results), len(lines))
	}

	// 验证顺序一致
	for i, r := range results {
		if r.Template == "" {
			t.Errorf("ProcessBatch()[%d] 模板为空", i)
		}
	}
}

func TestParsePipeline_MergeContextLines(t *testing.T) {
	p := NewParsePipeline()

	lines := []string{
		"2024-01-15 10:30:45 ERROR NullPointerException occurred",
		"    at com.example.Service.handle(Service.java:42)",
		"    at com.example.Controller.process(Controller.java:15)",
	}

	template, count := p.MergeContextLines(lines)
	if template == "" {
		t.Error("MergeContextLines() 返回空模板")
	}
	if count != 3 {
		t.Errorf("MergeContextLines() 行数 = %d, 期望 3", count)
	}
}

// ===== EmbedPipeline 测试 =====

func TestEmbedPipeline_EmbedTemplate_NilEmbedder(t *testing.T) {
	p := NewEmbedPipeline(nil)

	vec, err := p.EmbedTemplate("test template")
	if vec != nil {
		t.Error("EmbedTemplate() 应返回 nil（embedder 不可用）")
	}
	if err != nil {
		t.Errorf("EmbedTemplate() 不应报错: %v", err)
	}
}

func TestEmbedPipeline_EmbedBatch_NilEmbedder(t *testing.T) {
	p := NewEmbedPipeline(nil)

	vecs, err := p.EmbedBatch([]string{"a", "b"})
	if vecs != nil {
		t.Error("EmbedBatch() 应返回 nil（embedder 不可用）")
	}
	if err != nil {
		t.Errorf("EmbedBatch() 不应报错: %v", err)
	}
}

func TestEmbedPipeline_Dimension(t *testing.T) {
	p := NewEmbedPipeline(nil)
	if p.Dimension() != 0 {
		t.Errorf("Dimension() = %d, 期望 0", p.Dimension())
	}
}

// ===== AnomalyPipeline 测试 =====

func TestAnomalyPipeline_Detect_EmptyHash(t *testing.T) {
	p := NewAnomalyPipeline()

	ev, err := p.Detect("", 10, "2024-01-15 10:30:45")
	if err == nil {
		t.Error("Detect() 空 hash 应返回错误")
	}
	if ev != nil {
		t.Error("Detect() 空 hash 不应返回事件")
	}
}

func TestAnomalyPipeline_Detect_NoBaseline(t *testing.T) {
	p := NewAnomalyPipeline()

	// 无基线数据时应返回 nil（数据不足）
	ev, err := p.Detect("abc123", 10, "2024-01-15 10:30:45")
	if err != nil {
		t.Errorf("Detect() 不应报错: %v", err)
	}
	if ev != nil {
		t.Error("Detect() 无基线时应返回 nil")
	}
}

func TestAnomalyPipeline_UpdateBaseline(t *testing.T) {
	p := NewAnomalyPipeline()

	// 更新基线不应 panic
	p.UpdateBaseline("hash1", 1, 10)
	p.UpdateBaseline("hash1", 1, 20)
	p.UpdateBaseline("hash1", 1, 30)

	bl, ok := p.GetBaseline("hash1", 1)
	if !ok {
		t.Fatal("GetBaseline() 未找到基线")
	}
	if bl.Mean == 0 {
		t.Error("GetBaseline() 均值应为非零")
	}
}

func TestAnomalyPipeline_UpdateBaseline_EmptyHash(t *testing.T) {
	p := NewAnomalyPipeline()
	// 空 hash 不应 panic
	p.UpdateBaseline("", 1, 10)
}

// ===== SegmentWriter 测试 =====

func TestSegmentWriter_AddAndQuery(t *testing.T) {
	w := NewSegmentWriter()

	segs := []LocalSegment{
		{ItemID: 1, Date: "20240115", Hour: "10", TemplateHash: "aaa", LineCount: 100, ErrorCount: 5, WarnCount: 10},
		{ItemID: 1, Date: "20240115", Hour: "11", TemplateHash: "bbb", LineCount: 200, ErrorCount: 0, WarnCount: 20},
		{ItemID: 2, Date: "20240115", Hour: "10", TemplateHash: "ccc", LineCount: 50, ErrorCount: 0, WarnCount: 0},
	}

	for _, s := range segs {
		w.Add(s)
	}

	// 查询 item=1
	result := w.Query(1, "20240115")
	if len(result) != 2 {
		t.Errorf("Query() 返回 %d 条, 期望 2", len(result))
	}

	// 验证排序（按 Hour 升序）
	if len(result) >= 2 && result[0].Hour > result[1].Hour {
		t.Error("Query() 结果未按 Hour 升序排序")
	}

	// 查询不存在的 item
	empty := w.Query(999, "20240115")
	if len(empty) != 0 {
		t.Errorf("Query() 不存在 item 返回 %d 条, 期望 0", len(empty))
	}
}

func TestSegmentWriter_Count(t *testing.T) {
	w := NewSegmentWriter()

	w.Add(LocalSegment{ItemID: 1, Date: "20240115", Hour: "10", TemplateHash: "aaa", LineCount: 1})
	w.Add(LocalSegment{ItemID: 1, Date: "20240115", Hour: "11", TemplateHash: "bbb", LineCount: 2})

	if w.Count() != 2 {
		t.Errorf("Count() = %d, 期望 2", w.Count())
	}
}

func TestSegmentWriter_Clear(t *testing.T) {
	w := NewSegmentWriter()

	w.Add(LocalSegment{ItemID: 1, Date: "20240115", Hour: "10", TemplateHash: "aaa", LineCount: 1})
	w.Clear()

	if w.Count() != 0 {
		t.Errorf("Clear() 后 Count() = %d, 期望 0", w.Count())
	}
}

// ===== Pipeline 主入口测试 =====

func TestNew(t *testing.T) {
	items := []LogItem{
		{ID: 1, Name: "test", LogPath: "/tmp/test"},
	}
	p := New(nil, items)

	if p.parse == nil {
		t.Error("New() parse 组件为 nil")
	}
	if p.embed == nil {
		t.Error("New() embed 组件为 nil")
	}
	if p.anomaly == nil {
		t.Error("New() anomaly 组件为 nil")
	}
	if p.segWriter == nil {
		t.Error("New() segWriter 组件为 nil")
	}
}

func TestPipeline_StartStop(t *testing.T) {
	items := []LogItem{
		{ID: 1, Name: "test", LogPath: "/nonexistent"},
	}
	p := New(nil, items)
	p.SetInterval(10 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	p.Stop()
}

func TestPipeline_ContextCancel(t *testing.T) {
	items := []LogItem{
		{ID: 1, Name: "test", LogPath: "/nonexistent"},
	}
	p := New(nil, items)
	p.SetInterval(10 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	p.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel() // 取消 ctx，管道应退出

	// 等待管道退出（doneCh 关闭）
	select {
	case <-p.doneCh:
		// 成功退出
	case <-time.After(2 * time.Second):
		t.Error("Pipeline 未在超时前退出")
	}
}

func TestPipeline_ProcessItem_NoFile(t *testing.T) {
	items := []LogItem{
		{ID: 1, Name: "test", LogPath: "/nonexistent/path"},
	}
	p := New(nil, items)

	// 不存在的文件不应导致 panic
	p.processItem(context.Background(), items[0])
}

func TestReadLines(t *testing.T) {
	// 创建临时文件
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.log")

	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}

	lines := readLines(tmpFile, 100)
	if len(lines) != 3 {
		t.Errorf("readLines() 返回 %d 行, 期望 3", len(lines))
	}
}

func TestTemplateHash(t *testing.T) {
	h := templateHash("hello world")
	if h == "" {
		t.Error("templateHash() 返回空字符串")
	}
	// 相同输入应产生相同输出
	h2 := templateHash("hello world")
	if h != h2 {
		t.Error("templateHash() 相同输入产生不同输出")
	}
}

func TestSegmentHour(t *testing.T) {
	tests := []struct {
		ts   string
		want string
	}{
		{"2024-01-15 10:30:45", "10"},
		{"2024-01-15T10:30:45", "10"},
		{"2024-01-15", "00"},
		{"", "00"},
	}

	for _, tt := range tests {
		got := segmentHour(tt.ts)
		if got != tt.want {
			t.Errorf("segmentHour(%q) = %q, want %q", tt.ts, got, tt.want)
		}
	}
}

func TestExtractLevel(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"2024-01-15 ERROR something failed", "ERROR"},
		{"2024-01-15 WARN be careful", "WARN"},
		{"2024-01-15 INFO all good", "INFO"},
		{"no level here", ""},
	}

	for _, tt := range tests {
		got := extractLevel(tt.line)
		if got != tt.want {
			t.Errorf("extractLevel(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

// ===== 集成测试 =====

func TestPipeline_FullFlow(t *testing.T) {
	// processItem 使用 time.Now().Format("20060102") 作为日期
	today := time.Now().Format("20060102")

	// 创建临时日志文件（文件名需匹配 processItem 的拼接逻辑）
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "app"+today+".log")

	content := `2024-01-15 10:30:45 INFO request completed
2024-01-15 10:30:46 INFO request completed
2024-01-15 10:30:47 ERROR connection failed
2024-01-15 10:30:48 WARN retry attempt
`
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}

	// processItem 构造路径：LogPath + LogPrefix + date + LogSuffix + ".log"
	// 文件名为 "app<today>.log"，所以 LogPath=tmpDir+"/", LogPrefix="app"
	items := []LogItem{
		{ID: 1, Name: "test-app", LogPath: tmpDir + string(filepath.Separator), LogPrefix: "app"},
	}

	p := New(nil, items)
	p.SetInterval(10 * time.Second)

	ctx := context.Background()
	p.processItem(ctx, items[0])

	// 验证段写入（使用与 processItem 相同的日期）
	segs := p.segWriter.Query(1, today)
	if len(segs) == 0 {
		t.Error("processItem() 未写入任何段")
	}
}
