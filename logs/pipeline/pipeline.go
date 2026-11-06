// Package pipeline v2.1 后台写入管线（Background Pipeline）。
//
// 核心职责：定时扫描已纳管的日志项，将原始日志行经过 Drain 模板解析、
// 模板落库（t_log_template）、向量索引构建（t_embedding）、模式统计聚合
// （t_pattern_stat）、异常检测（t_anomaly_event）五个阶段，实现 PRD 定义的
// L0→L1→L2→L3 智能写入链路自动化。
//
// 设计约束：
//   - 管线失败不影响主检索链路（FR-801 可用性隔离）
//   - 支持优雅退出（ctx 取消 + select）
//   - 写入批量化（减少 SQLite 写压力）
//   - 向量化为可选阶段（embedder 不可用时跳过）
package pipeline

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"ydsz-trace/logs/controllers/alert"
	models "ydsz-trace/logs/models"
	"ydsz-trace/logs/storage"
	"ydsz-trace/pkg/embedding"
	"ydsz-trace/pkg/parser"
	"ydsz-trace/pkg/stats"
)

const (
	// defaultScanInterval 默认扫描间隔（每 5 分钟）
	defaultScanInterval = 5 * time.Minute
	// defaultBatchSize 每批处理的行数
	defaultBatchSize = 500
	// defaultMaxLinesPerItem 单次扫描单个 item 的最大行数
	defaultMaxLinesPerItem = 10000
	// defaultWorkerCount 并发 Drain 解析的 worker 数
	defaultWorkerCount = 4
)

// Pipeline 后台写入管线实例。
type Pipeline struct {
	interval     time.Duration
	batchSize    int
	maxLines     int
	workers      int
	parser       *parser.LogParser
	anomalyEng   *stats.AnomalyEngine
	embedder     embedding.Embedder
	modelVersion string

	// segWriter 列式段存储写入器（V2.1-02 接通）。
	// nil 时跳过段写入阶段，其余阶段不受影响。
	segWriter *storage.SegmentWriter

	// spool 背压本地缓冲（V2.2-09）：下游写入失败时入队，链路恢复后补传。
	// nil 时不启用缓冲（直接丢弃失败批次并记录日志）。
	spool *SpoolBuffer

	mu        sync.Mutex
	isRunning bool
	stopCh    chan struct{}
	doneCh    chan struct{}
}

// New 创建写入管线实例。
//
// embedder 可为 nil（向量化阶段自动跳过）。
func New(embedder embedding.Embedder, modelVersion string) *Pipeline {
	if modelVersion == "" {
		modelVersion = "bge-m3-v1"
	}
	p := &Pipeline{
		interval:     defaultScanInterval,
		batchSize:    defaultBatchSize,
		maxLines:     defaultMaxLinesPerItem,
		workers:      defaultWorkerCount,
		parser:       parser.New(),
		anomalyEng:   stats.NewAnomalyEngine(),
		embedder:     embedder,
		modelVersion: modelVersion,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}

	// 挂载列式段存储：DB 句柄可用即启用；不可用时降级（其余阶段照常）。
	if models.DB != nil {
		p.segWriter = storage.NewSegmentWriter(models.DB)
	}

	return p
}

// Start 启动后台管线 goroutine。
func (p *Pipeline) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isRunning {
		return
	}
	p.isRunning = true
	go p.run()
	log.Printf("[pipeline] 写入管线已启动，扫描间隔: %v", p.interval)

	// 启动背压补传器（V2.2-09）：仅在缓冲队列可用时生效
	if p.spool != nil {
		p.spool.StartReplayer(context.Background(), p.replayBatch)
		log.Printf("[pipeline] 背压补传已启用，待补传 %d 条", p.spool.Stats().Count)
	}
}

// Stop 停止管线 goroutine，阻塞至退出。
func (p *Pipeline) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.isRunning {
		return
	}
	p.isRunning = false
	close(p.stopCh)
	<-p.doneCh

	// 停止背压补传并落盘（V2.2-09）
	if p.spool != nil {
		p.spool.Stop()
	}
	log.Println("[pipeline] 写入管线已停止")
}

// SetSpoolBuffer 注入背压缓冲队列（V2.2-09）。
//
// 需在 Start 之前调用（Start 会启动补传器）。传 nil 表示不启用缓冲。
func (p *Pipeline) SetSpoolBuffer(buf *SpoolBuffer) {
	p.spool = buf
}

// SetInterval 动态调整扫描间隔（需在 Start 前调用）。
func (p *Pipeline) SetInterval(d time.Duration) {
	if d < 10*time.Second {
		d = 10 * time.Second
	}
	p.interval = d
}

// run 主循环：立即执行一次，之后按 interval 定时执行。
func (p *Pipeline) run() {
	defer close(p.doneCh)

	p.scanAll()

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.scanAll()
		}
	}
}

// scanAll 遍历所有启用的日志项，逐个执行五阶段管线。
func (p *Pipeline) scanAll() {
	ctx, cancel := context.WithTimeout(context.Background(), p.interval-time.Second)
	defer cancel()

	items, err := models.QueryAllItem()
	if err != nil || len(items) == 0 {
		return
	}

	for _, item := range items {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if item.Status != "1" {
			continue
		}
		p.processItem(ctx, &item)
	}
}

// processItem 处理单个日志项的完整管线。
func (p *Pipeline) processItem(ctx context.Context, item *models.TItem) {
	// 构造当日日志文件路径
	date := time.Now().Format("20060102")
	filePath := item.LogPath + item.LogPrefix + date + item.LogSuffix + ".log"

	// 阶段 1：读取日志行
	lines := p.readLines(filePath)
	if len(lines) == 0 {
		return
	}

	p.processLines(ctx, item, lines, date)
}

// processLines 对一批日志行执行五阶段写入管线。
//
// 与 processItem 的区别：不读取文件，直接接收已有行参。
// 实时上报（P1-3）与定时扫描共用同一套锁入逻辑，
// 避免双份维护导致行为分歧。
func (p *Pipeline) processLines(ctx context.Context, item *models.TItem, lines []string, date string) {
	// 阶段 1b：多行合并（V2.2-07，FR-104）
	// 把 Java 堆栈 / Python traceback / Go panic 折叠为逻辑条目后再解析，
	// 避免同一异常被拆成几十条碎片模板，同时让模板聚类更贴近真实错误。
	// 合并仅影响解析与统计视图，不改动磁盘上的原始日志。
	lines = flattenMultiLineEntries(parser.MergeMultiLine(lines))

	// 阶段 2：Drain 解析
	results := p.parser.ParseBatch(lines, p.workers)

	// 阶段 3：模板落库 + 统计聚合
	templateUpserts := make([]*models.TLogTemplate, 0, len(results))
	patternStats := make(map[string]int) // templateHash -> count
	patternSamples := make(map[string]string)
	patternFirstTime := make(map[string]string)
	patternLastTime := make(map[string]string)

	for i, r := range results {
		if r.TemplateID == 0 || r.Template == "" {
			continue
		}

		hash := models.TemplateHash(r.Template)
		patternStats[hash]++
		if _, ok := patternSamples[hash]; !ok {
			patternSamples[hash] = lines[i]
			patternFirstTime[hash] = extractTimestamp(lines[i])
		}
		patternLastTime[hash] = extractTimestamp(lines[i])

		// 构造模板 upsert
		level := extractLevel(lines[i])
		tmpl := &models.TLogTemplate{
			TemplateId:   hash,
			TemplateText: r.Template,
			Service:      item.ItemName,
			Level:        level,
			FirstSeen:    patternFirstTime[hash],
			LastSeen:     patternLastTime[hash],
			TotalCount:   1,
			SampleLine:   lines[i],
		}
		templateUpserts = append(templateUpserts, tmpl)
	}

	// 阶段 3a：批量 upsert t_log_template
	writeErr := error(nil)
	if len(templateUpserts) > 0 {
		if err := models.BatchUpsertTemplates(templateUpserts); err != nil {
			log.Printf("[pipeline] 模板落库失败 item=%d: %v", item.Id, err)
			writeErr = err
		}
	}

	// 阶段 3b：批量记录 pattern_stat
	patternErr := error(nil)
	for hash := range patternStats {
		sample := patternSamples[hash]
		lastTime := patternLastTime[hash]
		if err := models.RecordPatternHit(item.Id, 0, hash, lastTime, sample); err != nil {
			log.Printf("[pipeline] pattern_stat 写入失败 item=%d hash=%s: %v", item.Id, hash, err)
			patternErr = err
		}
	}

	// 阶段 3b-2：背压缓冲（V2.2-09，FR-103）
	// 任一写入阶段失败时，把本批统计暂存到本地队列，链路恢复后由补传器重放。
	// 未启用 spool 时仅记录日志（保持既有行为）。
	if (writeErr != nil || patternErr != nil) && p.spool != nil {
		batch := p.spoolTemplateStats(item.Id, patternStats, patternSamples, patternLastTime)
		if err := p.spool.Enqueue(batch); err != nil {
			log.Printf("[pipeline] 背压入队失败 item=%d: %v", item.Id, err)
		} else {
			log.Printf("[pipeline] 已暂存批次到本地缓冲 item=%d templates=%d",
				item.Id, len(batch.TemplateStats))
		}
	}

	// 阶段 3c：异常检测（V2.2-01 多算法编排）
	// 对每个模板依次运行 Z-score / EWMA / 周期基线 / 生命周期四类检测，
	// 相比单一 Z-score 能额外覆盖"新增模板"与"稳定后消失"两类信号。
	if len(patternStats) > 0 {
		now := time.Now()
		bucketIdx := alert.BucketIndexFromTime(now)

		for hash, count := range patternStats {
			events := alert.DetectOne(p.anomalyEng, hash, item.Id, count, bucketIdx, now)

			// ── V2.2-03 自适应阈值过滤 ──
			events = alert.FilterByAdaptiveThreshold(events)
			for _, ev := range events {
				anomalyType := anomalyTypeOf(ev)
				desc := fmt.Sprintf("模板 %s %s [%s]: 实际=%d, 基线=%.1f, Z=%.1f",
					ev.TemplateHash, ev.Direction, ev.Algorithm, ev.CurrentCount, ev.BaselineMean, ev.ZScore)
				if err := models.RecordAnomalyEvent(ev.TemplateHash, item.Id, 0, anomalyType,
					ev.ZScore, ev.BaselineMean, float64(ev.CurrentCount), 300, desc); err != nil {
					log.Printf("[pipeline] 异常事件写入失败: %v", err)
				}
			}
			// 更新基线（检测与学习解耦：检测只读，学习在此统一进行）
			p.anomalyEng.UpdateBaseline(hash, item.Id, count)
		}
	}

	// 阶段 3d：列式段存储（V2.1-02）
	// 将本次扫描的行按 (template, hour) 聚合为列式段，压缩后写入 t_log_segment。
	// 该阶段为只读日志的聚合副本，写失败不影响主检索链路。
	if p.segWriter != nil && len(results) > 0 {
		p.writeSegment(ctx, item, date, lines, results)
	}

	// 阶段 4：向量化（可选）
	if p.embedder != nil && len(templateUpserts) > 0 {
		p.embedTemplates(ctx, templateUpserts)
	}

	log.Printf("[pipeline] item=%d(%s) 处理完成: 读取 %d 行, 模板 %d 个, 统计 %d 组",
		item.Id, item.ItemName, len(lines), len(templateUpserts), len(patternStats))
}

// writeSegment 将本批日志按 (template_hash, hour) 聚合为列式段并写入 t_log_segment。
//
// 列式布局：timestamp / level / message 三列各自连续存放，便于按需单列扫描。
// 幂等性：段表 UNIQUE(item_id, client_id, seg_date, seg_hour, template_id)，
// 重复扫描同一小时以 INSERT OR REPLACE 覆盖。
//
// p.segWriter 为 nil 时（DB 未初始化）直接跳过，不影响其余阶段。
func (p *Pipeline) writeSegment(ctx context.Context, item *models.TItem, date string,
	lines []string, results []parser.ParseResult) {

	if p.segWriter == nil || len(results) == 0 {
		return
	}

	type bucketKey struct {
		templateHash string
		hour         string
	}
	type bucketAgg struct {
		timestamps []string
		levels     []string
		messages   []string
		errorCount int
		warnCount  int
		firstTs    string
		lastTs     string
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

		hash := models.TemplateHash(r.Template)
		key := bucketKey{templateHash: hash, hour: hour}

		agg, ok := buckets[key]
		if !ok {
			agg = &bucketAgg{
				timestamps: make([]string, 0, 8),
				levels:     make([]string, 0, 8),
				messages:   make([]string, 0, 8),
				firstTs:    ts,
			}
			buckets[key] = agg
		}

		agg.timestamps = append(agg.timestamps, ts)
		agg.levels = append(agg.levels, level)
		agg.messages = append(agg.messages, line)
		agg.lastTs = ts

		switch level {
		case "ERROR", "FATAL":
			agg.errorCount++
		case "WARN", "WARNING":
			agg.warnCount++
		}
	}

	for key, agg := range buckets {
		select {
		case <-ctx.Done():
			return
		default:
		}

		payload := encodeSegmentColumns(agg.timestamps, agg.levels, agg.messages)

		seg := &models.TLogSegment{
			ItemId:     item.Id,
			ClientId:   item.ClientId,
			SegDate:    date,
			SegHour:    key.hour,
			TemplateId: key.templateHash,
			LineCount:  len(agg.messages),
			ErrorCount: agg.errorCount,
			WarnCount:  agg.warnCount,
			FirstTs:    agg.firstTs,
			LastTs:     agg.lastTs,
			Compressed: payload,
		}

		if err := p.segWriter.WriteSegment(ctx, seg); err != nil {
			log.Printf("[pipeline] 段写入失败 item=%d hash=%s hour=%s: %v",
				item.Id, key.templateHash, key.hour, err)
		}
	}
}

// segmentHour 从时间戳提取小时（"HH"）。
//
// 支持 "YYYY-MM-DD HH:MM:SS" 与 "YYYY-MM-DDTHH:MM:SS" 两种常见格式；
// 分隔符（空格或 T）必须位于第 11 位，否则视为无法解析。
// 无法解析时返回 "00"（归到零点桶，保证段仍可写入）。
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

// flattenMultiLineEntries 将多行合并条目还原为行切片。
//
// 单行条目直接输出原文；多行条目输出合并后的完整消息（含从属行），
// 使 Drain 把整个堆栈视为一条日志来提取模板。
func flattenMultiLineEntries(entries []parser.MultiLineEntry) []string {
	if len(entries) == 0 {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Kind == parser.KindSingle || e.MergedMessage == "" {
			out = append(out, e.Primary)
			continue
		}
		out = append(out, e.MergedMessage)
	}
	return out
}

// anomalyTypeOf 将检测事件映射为持久化的异常类型。
//
// 优先使用检测器给出的 AnomalyType（新增/消失有明确语义），
// 缺失时按方向推断：下降归为 disappear，其余归为 spike。
func anomalyTypeOf(ev stats.AnomalyEvent) models.AnomalyType {
	// stats.AnomalyType 与 models.AnomalyType 是两套独立定义（值相同），
	// 按字符串值映射，避免 pkg 依赖 logs 造成架构倒置。
	switch string(ev.AnomalyType) {
	case string(models.AnomalyNew):
		return models.AnomalyNew
	case string(models.AnomalyDisappear):
		return models.AnomalyDisappear
	case string(models.AnomalySpike):
		return models.AnomalySpike
	}

	if ev.Direction == stats.DirectionDrop {
		return models.AnomalyDisappear
	}
	return models.AnomalySpike
}

// segmentFormatVersion 列式段格式版本号。
const segmentFormatVersion = byte(1)

// encodeSegmentColumns 将三列数据编码为列式字节流。
//
// 布局：[magic(4B)][version(1B)][rowCount(4B)]
//
//	[tsCount(4B)][tsOff(4B)][lvCount(4B)][lvOff(4B)][msgCount(4B)][msgOff(4B)]
//	[tsBlob][lvBlob][msgBlob]
//
// 每个 blob 内部为重复的 [len(4B)][bytes] 记录。
func encodeSegmentColumns(timestamps, levels, messages []string) []byte {
	tsBlob := encodeStringColumn(timestamps)
	lvBlob := encodeStringColumn(levels)
	msgBlob := encodeStringColumn(messages)

	const headerSize = 33 // magic(4) + version(1) + rowCount(4) + 6*uint32(24)

	header := make([]byte, 0, headerSize+len(tsBlob)+len(lvBlob)+len(msgBlob))
	header = append(header, 'Y', 'D', 'S', 'G')
	header = append(header, segmentFormatVersion)
	header = append(header, u32(uint32(len(messages)))...)
	header = append(header, make([]byte, 24)...) // 占位 3 组 (count, offset)

	blob := header
	blob = append(blob, tsBlob...)
	blob = append(blob, lvBlob...)
	blob = append(blob, msgBlob...)

	putU32(blob[9:], uint32(len(timestamps)))
	putU32(blob[13:], uint32(headerSize))
	putU32(blob[17:], uint32(len(levels)))
	putU32(blob[21:], uint32(headerSize+len(tsBlob)))
	putU32(blob[25:], uint32(len(messages)))
	putU32(blob[29:], uint32(headerSize+len(tsBlob)+len(lvBlob)))

	return blob
}

// encodeStringColumn 将字符串列编码为 [len(4B)][bytes] 重复序列。
func encodeStringColumn(values []string) []byte {
	out := make([]byte, 0, len(values)*8)
	for _, v := range values {
		out = append(out, u32(uint32(len(v)))...)
		out = append(out, v...)
	}
	return out
}

// u32 将 uint32 编码为大端 4 字节。
func u32(v uint32) []byte {
	return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
}

// putU32 以大端写入 uint32 到 dst（dst 长度须 >= 4）。
func putU32(dst []byte, v uint32) {
	dst[0] = byte(v >> 24)
	dst[1] = byte(v >> 16)
	dst[2] = byte(v >> 8)
	dst[3] = byte(v)
}
func (p *Pipeline) embedTemplates(ctx context.Context, templates []*models.TLogTemplate) {
	items := make([]models.EmbeddingWriteItem, 0, len(templates))
	for _, tmpl := range templates {
		select {
		case <-ctx.Done():
			return
		default:
		}

		vec, err := p.embedder.Embed(ctx, tmpl.TemplateText)
		if err != nil {
			log.Printf("[pipeline] embed 模板失败 %s: %v", tmpl.TemplateId, err)
			continue
		}
		items = append(items, models.EmbeddingWriteItem{
			TargetType:   "template",
			TargetID:     tmpl.TemplateId,
			ModelVersion: p.modelVersion,
			Vector:       vec,
		})
	}

	if len(items) > 0 {
		if err := models.BatchStoreEmbeddings(items); err != nil {
			log.Printf("[pipeline] 向量批量写入失败: %v", err)
		}
	}
}

// readLines 从文件中读取最多 maxLines 行。
func (p *Pipeline) readLines(filePath string) []string {
	f, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Printf("[pipeline] close file failed: %v", cerr)
		}
	}()

	lines := make([]string, 0, p.batchSize)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 支持 1MB 行长

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
			if len(lines) >= p.maxLines {
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("[pipeline] scan file failed: %v", err)
	}
	return lines
}

// extractTimestamp 从日志行提取时间戳。
func extractTimestamp(line string) string {
	re := regexp.MustCompile(`\d{4}-\d{2}-\d{2}(?:[T ]\d{2}:\d{2}:\d{2})?`)
	m := re.FindString(line)
	return m
}

// extractLevel 从日志行提取级别。
func extractLevel(line string) string {
	re := regexp.MustCompile(`(?i)\b(ERROR|WARN|INFO|DEBUG|FATAL)\b`)
	m := re.FindString(line)
	if m == "" {
		return ""
	}
	return strings.ToUpper(m)
}

// lastTimestamp 从多个时间戳中取最后一个非空值。
func lastTimestamp(m map[string]string) string {
	var last string
	for _, v := range m {
		if v != "" {
			last = v
		}
	}
	return last
}

// IngestBatch 实时入口（P1-3）：处理 agentlink 长连接通道上报的日志批次。
//
// 与 processItem 的区别：行数据由 agent 上报而非本地读取，
// 因此真正实现“日志主动上报”，减少本地扫描与 agent 端的重复读取。
//
// 安全校验：根据 clientID + itemID 回查日志项，校验归属关系；
// 不匹配或不存在时拒绝上报，防止 agent 伪造任意 item。
//
// 行数超过 maxLines 时截断（与定时扫描的保护线一致）。
func (p *Pipeline) IngestBatch(ctx context.Context, clientID, itemID int64, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	if len(lines) > p.maxLines {
		lines = lines[:p.maxLines]
	}

	// 安全校验：日志项必须归属于该 agent，且状态为启用。
	item := models.ReadItem(itemID)
	if item.Id == 0 {
		return fmt.Errorf("item %d not found", itemID)
	}
	if item.ClientId != clientID {
		return fmt.Errorf("item %d not owned by client %d (owner=%d)", itemID, clientID, item.ClientId)
	}
	if item.Status != "1" {
		return fmt.Errorf("item %d is disabled", itemID)
	}

	date := time.Now().Format("20060102")
	p.processLines(ctx, &item, lines, date)
	return nil
}
