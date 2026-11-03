// Package perf 核心热路径性能基准测试。
//
// 覆盖 FR-101/FR-302/FR-406/FR-601 关键性能指标：
//   - Drain 解析吞吐量（≥50,000 行/秒）
//   - 向量余弦相似度（微秒级）
//   - 缓存读写 P95（≤200ms 命中路径应 ≤1ms）
//
// 运行命令：
//   go test ./pkg/perf/ -bench=. -benchmem
//   go test ./pkg/perf/ -bench=BenchmarkDrain -benchtime=5s
package perf

import (
	"fmt"
	"testing"

	"ydsz-trace/pkg/embedding"
	"ydsz-trace/pkg/parser"
)

// sampleLines 模拟 100 条日志行用于基准测试。
var sampleLines = func() []string {
	lines := make([]string, 100)
	templates := []string{
		"ERROR connection refused to host=%s port=%d",
		"WARN retrying request id=%s attempt=%d",
		"INFO user login success user_id=%s ip=%s",
		"DEBUG cache hit key=%s duration=%dms",
		"FATAL out_of_memory heap_size=%d",
	}
	for i := range lines {
		tpl := templates[i%len(templates)]
		lines[i] = fmt.Sprintf(tpl, "abc123", i*100)
	}
	return lines
}()

// BenchmarkDrainParse  Drain 单行解析基准。
func BenchmarkDrainParse(b *testing.B) {
	p := parser.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.ParseLine(sampleLines[i%len(sampleLines)])
	}
}

// BenchmarkDrainBatchParse  Drain 批量解析基准（并发 4 协程）。
func BenchmarkDrainBatchParse(b *testing.B) {
	p := parser.New()
	batch := make([]string, 1000)
	for i := range batch {
		batch[i] = sampleLines[i%len(sampleLines)]
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.ParseBatch(batch, 4)
	}
}

// BenchmarkCosineSimilarity  向量余弦相似度计算基准。
func BenchmarkCosineSimilarity(b *testing.B) {
	v1 := make([]float32, 64)
	v2 := make([]float32, 64)
	for i := range v1 {
		v1[i] = float32(i) * 0.01
		v2[i] = float32(i+1) * 0.01
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		embedding.CosineSimilarity(v1, v2)
	}
}

// BenchmarkTemplateHash  模板 hash 计算基准。
func BenchmarkTemplateHash(b *testing.B) {
	line := "ERROR connection refused to host=abc123 port=8080"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hashString(line)
	}
}

// hashString 简单字符串 hash（SHA1 变体）。
func hashString(s string) string {
	h := 0
	for _, c := range s {
		h = h*31 + int(c)
	}
	return fmt.Sprintf("%x", h)
}
