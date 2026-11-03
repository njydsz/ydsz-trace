// Pipeline 子文件：背压本地缓冲（V2.2-09，FR-103）。
//
// 背景：
//   写入管线的下游（SQLite 写入、向量化调用）在负载高或依赖不可用时会变慢。
//   若同步等待，会阻塞日志读取；若直接丢弃，会丢失故障期日志。
//
// 设计：
//   - 管线的「模板/统计批次」在下游写入前，先尝试正常路径
//   - 正常路径失败时，将批次序列化后写入本地 spool 队列
//   - 后台补传 goroutine 在链路恢复后按 FIFO 顺序重放
//
// 与采集端的区别：
//   采集端（logc）无 spool 必要——拉取模式下服务端不可达时不产生数据。
//   管线端需要 spool，因为本地日志仍在增长，分析不能停。
package pipeline

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"sync"
	"time"

	"ydsz-trace/pkg/spool"
)

// spoolBatch 写入管线的待补传批次。
//
// 只序列化「可重建的统计信息」，不保存原始日志行：
// 原始行仍留在磁盘日志文件中，可通过位点重新读取；
// spool 仅保证统计与模板更新不丢。
type spoolBatch struct {
	// ItemID 关联日志项
	ItemID int64 `json:"itemId"`
	// TemplateStats 模板 hash -> 计数
	TemplateStats map[string]int `json:"templateStats"`
	// PatternHits 模式命中样本（供 pattern_stat 重放）
	PatternHits []spoolPatternHit `json:"patternHits"`
	// EnqueuedAt 入队时间
	EnqueuedAt time.Time `json:"enqueuedAt"`
}

// spoolPatternHit 模式命中记录（重放用）。
type spoolPatternHit struct {
	TemplateHash string `json:"templateHash"`
	Timestamp    string `json:"timestamp"`
	Sample       string `json:"sample"`
}

// SpoolBuffer 管线背压缓冲。
type SpoolBuffer struct {
	queue *spool.Queue
	// replayInterval 补传重试间隔
	replayInterval time.Duration
	// maxReplayBatch 单次补传最多处理条数（避免长时间占用）
	maxReplayBatch int

	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSpoolBuffer 创建背压缓冲。
//
// path 为空时使用 spool 包默认路径；capacity <= 0 时使用默认容量。
func NewSpoolBuffer(path string, capacity int, replayInterval time.Duration) (*SpoolBuffer, error) {
	if replayInterval <= 0 {
		replayInterval = defaultReplayInterval
	}

	q, err := spool.OpenQueue(path, capacity)
	if err != nil {
		return nil, err
	}

	return &SpoolBuffer{
		queue:          q,
		replayInterval: replayInterval,
		maxReplayBatch: defaultMaxReplayBatch,
	}, nil
}

// 默认参数。
const (
	// defaultReplayInterval 补传重试间隔
	defaultReplayInterval = 30 * time.Second
	// defaultMaxReplayBatch 单次补传最大条数
	defaultMaxReplayBatch = 100
)

// Enqueue 将失败批次写入缓冲队列。
func (b *SpoolBuffer) Enqueue(batch *spoolBatch) error {
	if batch == nil {
		return nil
	}
	data, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	return b.queue.Enqueue(data)
}

// StartReplayer 启动后台补传 goroutine。
//
// replayFn 为实际的重放逻辑（由调用方注入，通常写回 models 层）。
// 返回的 context 取消时退出。
func (b *SpoolBuffer) StartReplayer(ctx context.Context, replayFn func(batch *spoolBatch) error) {
	ctx, cancel := context.WithCancel(ctx)
	b.mu.Lock()
	b.cancel = cancel
	b.mu.Unlock()

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		ticker := time.NewTicker(b.replayInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				b.replayOnce(ctx, replayFn)
			}
		}
	}()
}

// replayOnce 执行一轮补传。
func (b *SpoolBuffer) replayOnce(ctx context.Context, replayFn func(batch *spoolBatch) error) {
	processed := 0
	for processed < b.maxReplayBatch {
		select {
		case <-ctx.Done():
			return
		default:
		}

		data, err := b.queue.Peek()
		if err == io.EOF {
			return
		}
		if err != nil {
			log.Printf("[pipeline] spool peek 失败: %v", err)
			return
		}

		var batch spoolBatch
		if err := json.Unmarshal(data, &batch); err != nil {
			log.Printf("[pipeline] spool 反序列化失败，跳过: %v", err)
			// 损坏条目直接消费掉，避免卡住队列
			if _, derr := b.queue.Dequeue(); derr != nil {
				return
			}
			continue
		}

		if err := replayFn(&batch); err != nil {
			// 重放失败：保留条目等待下轮（链路可能仍不可用）
			log.Printf("[pipeline] spool 重放失败，将在 %s 后重试: %v", b.replayInterval, err)
			return
		}

		// 重放成功：消费该条目
		if _, err := b.queue.Dequeue(); err != nil {
			log.Printf("[pipeline] spool dequeue 失败: %v", err)
			return
		}
		processed++
	}

	if processed > 0 {
		log.Printf("[pipeline] spool 补传完成 %d 条", processed)
	}
}

// Stop 停止补传并关闭队列。
func (b *SpoolBuffer) Stop() {
	b.mu.Lock()
	cancel := b.cancel
	b.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	b.wg.Wait()

	if err := b.queue.Close(); err != nil {
		log.Printf("[pipeline] spool 关闭失败: %v", err)
	}
}

// Stats 返回缓冲队列统计。
func (b *SpoolBuffer) Stats() spool.QueueStats {
	return b.queue.Stats()
}
