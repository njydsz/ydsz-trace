package agentlink

import (
	"encoding/json"
	"errors"
	"log"

	"ydsz-trace/pkg/agentlink"
	"ydsz-trace/pkg/spool"
)

// SpoolConfig 背压队列配置（FR-103）。
type SpoolConfig struct {
	// Enabled 是否启用背压队列（默认 true）。
	Enabled bool
	// Capacity 队列容量（条数）。
	Capacity int
	// Path 队列文件路径（空时用 spool 默认路径）。
	Path string
}

// defaultSpoolConfig 默认配置。
func defaultSpoolConfig() SpoolConfig {
	return SpoolConfig{
		Enabled:  true,
		Capacity: spool.DefaultCapacity,
	}
}

// SpoolManager 背压队列管理器（FR-103）。
//
// 职责：
//   - 连接断开时把日志批次写入磁盘队列
//   - 连接恢复后从磁盘队列补传
//   - 磁盘水位超限时按策略丢弃最旧数据
type SpoolManager struct {
	queue *spool.Queue
	cfg   SpoolConfig
}

// NewSpoolManager 创建背压队列管理器。
//
// cfg.Enabled=false 时返回 nil（不启用背压）。
func NewSpoolManager(cfg SpoolConfig) (*SpoolManager, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	q, err := spool.OpenQueue(cfg.Path, cfg.Capacity)
	if err != nil {
		return nil, err
	}
	return &SpoolManager{queue: q, cfg: cfg}, nil
}

// Close 关闭队列（落盘）。
func (m *SpoolManager) Close() error {
	if m == nil || m.queue == nil {
		return nil
	}
	return m.queue.Close()
}

// Stats 返回队列统计。
func (m *SpoolManager) Stats() spool.QueueStats {
	if m == nil || m.queue == nil {
		return spool.QueueStats{}
	}
	return m.queue.Stats()
}

// enqueue 写入一个批次（队列满时丢弃最旧数据）。
//
// 批次序列化为 JSON 后写入 spool 队列。
// 队列满时按 PRD 策略丢弃最旧数据：读取一条旧记录丢弃后再写入。
func (m *SpoolManager) enqueue(batch *agentlink.LogBatch) error {
	if m == nil || m.queue == nil {
		return errors.New("spool: 背压队列未启用")
	}

	payload, err := json.Marshal(batch)
	if err != nil {
		return err
	}

	// 尝试直接写入。
	if err := m.queue.Enqueue(payload); err == nil {
		return nil
	} else if !errors.Is(err, spool.ErrQueueFull) {
		return err
	}

	// 队列满：丢弃最旧数据后重试（PRD FR-103：按策略丢弃最旧数据）。
	log.Printf("[spool] 队列已满（capacity=%d），丢弃最旧数据", m.cfg.Capacity)
	if _, err := m.queue.Dequeue(); err != nil {
		return err
	}
	return m.queue.Enqueue(payload)
}

// dequeue 读取一个批次（队列为空时返回 io.EOF）。
func (m *SpoolManager) dequeue() (*agentlink.LogBatch, error) {
	if m == nil || m.queue == nil {
		return nil, errors.New("spool: 背压队列未启用")
	}

	// 直接用 Dequeue 读取并推进读指针。
	// 注意：spool.Queue 的 Peek 不推进指针，Commit 也只在 head==tail 时截断文件，
	// 因此必须用 Dequeue 才能正确消费。
	payload, err := m.queue.Dequeue()
	if err != nil {
		return nil, err
	}

	var batch agentlink.LogBatch
	if err := json.Unmarshal(payload, &batch); err != nil {
		// 损坏数据：已出队，丢弃并继续。
		return nil, err
	}

	return &batch, nil
}

// drainTo 尝试把队列中的批次全部推送到 sendFn，直到队列空或达到 maxAttempts。
//
// 用于连接恢复后补传。
func (m *SpoolManager) drainTo(sendFn func(*agentlink.LogBatch) error, maxAttempts int) int {
	if m == nil || m.queue == nil {
		return 0
	}

	drained := 0
	for i := 0; i < maxAttempts; i++ {
		batch, err := m.dequeue()
		if err != nil {
			break
		}
		if err := sendFn(batch); err != nil {
			// 发送失败：重新入队（避免丢失），但避免无限循环。
			if reErr := m.enqueue(batch); reErr != nil {
				log.Printf("[spool] 补传失败且重新入队失败: %v", reErr)
			}
			break
		}
		drained++
	}
	if drained > 0 {
		log.Printf("[spool] 补传完成: %d 个批次", drained)
	}
	return drained
}
