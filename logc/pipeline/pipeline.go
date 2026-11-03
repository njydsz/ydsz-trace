// Package pipeline logc 端侧管道（PRD 6.2 L0→L1→L2→L3）。
//
// Pipeline 组合 ParsePipeline / EmbedPipeline / AnomalyPipeline / SegmentWriter，
// 提供定时扫描日志文件并执行完整管道的能力。
//
// 设计约束：
//   - 支持优雅退出（ctx 取消 + select）
//   - 向量化为可选阶段（embedder 不可用时跳过）
//   - 段存储为纯内存，不落库
package pipeline

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"ydsz-trace/pkg/embedding"
	"ydsz-trace/pkg/parser"
	"ydsz-trace/pkg/stats"
)

// 默认常量。
const (
	// defaultScanInterval 默认扫描间隔
	defaultScanInterval = 5 * time.Minute
	// defaultBatchSize 每批处理的行数
	defaultBatchSize = 500
	// defaultMaxLinesPerItem 单次扫描单个 item 的最大行数
	defaultMaxLinesPerItem = 10000
	// defaultWorkerCount 并发 Drain 解析的 worker 数
	defaultWorkerCount = 4
	// embedTimeout 向量化超时
	embedTimeout = 30 * time.Second
)

// 错误哨兵。
var (
	// ErrEmptyLine 空日志行
	ErrEmptyLine = errors.New("pipeline: 空日志行")
	// ErrEmptyTemplateHash 空模板 hash
	ErrEmptyTemplateHash = errors.New("pipeline: 空模板 hash")
)

// LogItem 日志项配置（端侧扫描目标）。
type LogItem struct {
	// ID 日志项 ID
	ID int64
	// Name 日志项名称
	Name string
	// LogPath 日志文件路径前缀
	LogPath string
	// LogPrefix 日志文件名前缀
	LogPrefix string
	// LogSuffix 日志文件名后缀
	LogSuffix string
}

// Pipeline 端侧管道主入口。
//
// 组合四个组件：
//   - parse:  L0 模板解析
//   - embed:  L2 向量化（可选）
//   - anomaly: L1 异常检测
//   - segWriter: 段存储（纯内存）
type Pipeline struct {
	parse      *ParsePipeline
	embed      *EmbedPipeline
	anomaly    *AnomalyPipeline
	segWriter  *SegmentWriter
	interval   time.Duration
	maxLines   int
	scanTargets []LogItem

	mu        sync.Mutex
	isRunning bool
	stopCh    chan struct{}
	doneCh    chan struct{}
}

// New 创建端侧管道实例。
//
// embedder 可为 nil（向量化阶段自动跳过）。
// items 为扫描目标列表，为空时需在 Start 前通过 SetScanTargets 设置。
func New(embedder embedding.Embedder, items []LogItem) *Pipeline {
	return &Pipeline{
		parse:       NewParsePipeline(),
		embed:       NewEmbedPipeline(embedder),
		anomaly:     NewAnomalyPipeline(),
		segWriter:  NewSegmentWriter(),
		interval:    defaultScanInterval,
		maxLines:    defaultMaxLinesPerItem,
		scanTargets: items,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}
}

// Start 启动端侧管道 goroutine。
//
// ctx 用于优雅退出；取消 ctx 时管道停止。
func (p *Pipeline) Start(ctx context.Context) {
	p.mu.Lock()
	if p.isRunning {
		p.mu.Unlock()
		return
	}
	p.isRunning = true
	p.mu.Unlock()

	log.Printf("[pipeline] 端侧管道已启动，扫描间隔: %v", p.interval)
	go p.run(ctx)
}

// Stop 停止管道 goroutine，阻塞至退出。
func (p *Pipeline) Stop() {
	p.mu.Lock()
	if !p.isRunning {
		p.mu.Unlock()
		return
	}
	p.isRunning = false
	close(p.stopCh)
	p.mu.Unlock()

	<-p.doneCh
	log.Println("[pipeline] 端侧管道已停止")
}

// SetInterval 动态调整扫描间隔（需在 Start 前调用）。
func (p *Pipeline) SetInterval(d time.Duration) {
	if d < 10*time.Second {
		d = 10 * time.Second
	}
	p.interval = d
}

// SetScanTargets 设置扫描目标列表。
func (p *Pipeline) SetScanTargets(items []LogItem) {
	p.scanTargets = items
}

// run 主循环：立即执行一次，之后按 interval 定时执行。
func (p *Pipeline) run(ctx context.Context) {
	defer close(p.doneCh)

	p.scanAll(ctx)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[pipeline] ctx 取消，端侧管道退出")
			return
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.scanAll(ctx)
		}
	}
}

// scanAll 遍历所有扫描目标，逐个执行完整管道。
func (p *Pipeline) scanAll(ctx context.Context) {
	for _, item := range p.scanTargets {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		default:
		}
		p.processItem(ctx, item)
	}
}

// processItem 处理单个日志项的完整管道。
//
// 阶段：
//  1. 读取日志行（L0 输入）
//  2. Drain 解析（L0）
//  3. 异常检测（L1）
//  4. 段聚合写入（纯内存）
//  5. 向量化（L2，可选）
func (p *Pipeline) processItem(ctx context.Context, item LogItem) {
	date := time.Now().Format("20060102")
	filePath := item.LogPath + item.LogPrefix + date + item.LogSuffix + ".log"

	// 阶段 1：读取日志行
	lines := readLines(filePath, p.maxLines)
	if len(lines) == 0 {
		return
	}

	// 阶段 2：Drain 解析
	results := p.parse.ProcessBatch(lines)

	// 阶段 3：异常检测 + 段聚合
	patternStats := make(map[string]int)
	patternLastTime := make(map[string]string)

	for i, r := range results {
		if r.TemplateID == 0 || r.Template == "" {
			continue
		}
		hash := templateHash(r.Template)
		patternStats[hash]++
		patternLastTime[hash] = extractTimestamp(lines[i])
	}

	// 异常检测
	for hash, count := range patternStats {
		ev, err := p.anomaly.Detect(hash, count, patternLastTime[hash])
		if err != nil {
			log.Printf("[pipeline] 异常检测失败 hash=%s: %v", hash, err)
			continue
		}
		if ev != nil {
			log.Printf("[pipeline] 异常事件 hash=%s direction=%s severity=%s count=%d",
				ev.TemplateHash, ev.Direction, ev.Severity, ev.CurrentCount)
		}
		// 更新基线
		p.anomaly.UpdateBaseline(hash, item.ID, count)
	}

	// 阶段 4：段聚合写入（纯内存）
	p.writeSegments(item, date, lines, results)

	// 阶段 5：向量化（可选）
	if p.embed.Dimension() > 0 {
		p.embedTemplates(ctx, results)
	}

	log.Printf("[pipeline] item=%d(%s) 处理完成: 读取 %d 行, 模板 %d 个, 统计 %d 组",
		item.ID, item.Name, len(lines), len(results), len(patternStats))
}

// writeSegments 将本批日志按 (template_hash, hour) 聚合为段并写入内存。
func (p *Pipeline) writeSegments(item LogItem, date string, lines []string, results []parser.ParseResult) {
	type bucketKey struct {
		templateHash string
		hour         string
	}
	type bucketAgg struct {
		lineCount  int
		errorCount int
		warnCount  int
	}

	buckets := make(map[bucketKey]*bucketAgg)

	for i, r := range results {
		if r.TemplateID == 0 || r.Template == "" {
			continue
		}
		if i >= len(lines) {
			break
		}

		line := lines[i]
		ts := extractTimestamp(line)
		hour := segmentHour(ts)
		level := extractLevel(line)

		hash := templateHash(r.Template)
		key := bucketKey{templateHash: hash, hour: hour}

		agg, ok := buckets[key]
		if !ok {
			agg = &bucketAgg{}
			buckets[key] = agg
		}

		agg.lineCount++
		switch level {
		case "ERROR", "FATAL":
			agg.errorCount++
		case "WARN", "WARNING":
			agg.warnCount++
		}
	}

	for key, agg := range buckets {
		p.segWriter.Add(LocalSegment{
			ItemID:       item.ID,
			Date:         date,
			Hour:         key.hour,
			TemplateHash: key.templateHash,
			LineCount:    agg.lineCount,
			ErrorCount:   agg.errorCount,
			WarnCount:    agg.warnCount,
		})
	}
}

// embedTemplates 对模板结果批量向量化。
func (p *Pipeline) embedTemplates(ctx context.Context, results []parser.ParseResult) {
	seen := make(map[string]bool)
	templates := make([]string, 0)

	for _, r := range results {
		if r.Template == "" || seen[r.Template] {
			continue
		}
		seen[r.Template] = true
		templates = append(templates, r.Template)
	}

	if len(templates) == 0 {
		return
	}

	vecs, err := p.embed.EmbedBatch(templates)
	if err != nil {
		log.Printf("[pipeline] 批量向量化失败: %v", err)
		return
	}
	log.Printf("[pipeline] 向量化完成: %d 个模板, 维度 %d", len(templates), p.embed.Dimension())
	_ = vecs // 向量结果可由后续 L3 层使用
}

// readLines 从文件中读取最多 maxLines 行。
func readLines(filePath string, maxLines int) []string {
	f, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Printf("[pipeline] close file failed: %v", cerr)
		}
	}()

	lines := make([]string, 0, defaultBatchSize)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 支持 1MB 行长

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
			if len(lines) >= maxLines {
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("[pipeline] scan file failed: %v", err)
	}
	return lines
}

// templateHash 计算模板串的 FNV-1a hash（32 位，16 进制）。
func templateHash(s string) string {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return fmt.Sprintf("%08x", h)
}

// segmentHour 从时间戳提取小时（"HH"）。
//
// 支持 "YYYY-MM-DD HH:MM:SS" 与 "YYYY-MM-DDTHH:MM:SS" 两种常见格式；
// 无法解析时返回 "00"。
func segmentHour(ts string) string {
	if len(ts) < 13 {
		return "00"
	}
	sep := ts[10]
	if sep != ' ' && sep != 'T' && sep != 't' {
		return "00"
	}
	h := ts[11:13]
	for _, r := range h {
		if r < '0' || r > '9' {
			return "00"
		}
	}
	return h
}

// extractTimestamp 从日志行提取时间戳。
func extractTimestamp(line string) string {
	// 简单实现：查找 "YYYY-MM-DD" 模式
	if len(line) < 10 {
		return ""
	}
	for i := 0; i <= len(line)-10; i++ {
		if line[i] >= '0' && line[i] <= '9' &&
			line[i+1] >= '0' && line[i+1] <= '9' &&
			line[i+2] >= '0' && line[i+2] <= '9' &&
			line[i+3] >= '0' && line[i+3] <= '9' &&
			line[i+4] == '-' &&
			line[i+5] >= '0' && line[i+5] <= '9' &&
			line[i+6] >= '0' && line[i+6] <= '9' &&
			line[i+7] == '-' &&
			line[i+8] >= '0' && line[i+8] <= '9' &&
			line[i+9] >= '0' && line[i+9] <= '9' {
			// 取到秒（如果存在）
			end := i + 10
			if len(line) >= i+19 {
				end = i + 19
			}
			return line[i:end]
		}
	}
	return ""
}

// extractLevel 从日志行提取级别。
func extractLevel(line string) string {
	upper := strings.ToUpper(line)
	for _, lvl := range []string{"FATAL", "ERROR", "WARN", "INFO", "DEBUG"} {
		if strings.Contains(upper, lvl) {
			return lvl
		}
	}
	return ""
}

// 确保 stats 包被引用（AnomalyPipeline 内部使用）。
var _ = stats.DirectionSurge
