package agentlink

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"ydsz-trace/pkg/agentlink"
	"ydsz-trace/pkg/source"
)

// TailingItem 正在被跟踪的日志项。
type TailingItem struct {
	// Spec 服务端下发的配置。
	Spec agentlink.ItemSpec
	// FilePath 当日日志文件完整路径。
	FilePath string
	// cancel 停止该 item 的跟踪。
	cancel context.CancelFunc
}

// Tailer 按服务端下发的 item 配置在本地跟踪日志文件并上报（P1-3）。
//
// 生命周期：
//   - Start() 启动后台 goroutine
//   - ApplyItems() 全量替换跟踪列表（注册应答 / 配置下发时调用）
//   - Stop() 停止全部跟踪
//
// 上报语义：从文件当前末尾开始跟踪新增行（实时增量），
// 不重复上报历史日志；agent 重启后从重启时刻的末尾继续。
type Tailer struct {
	src source.Source

	mu      sync.Mutex
	items   map[int64]*TailingItem
	running bool

	// submit 提交一个批次（由 Client.SubmitBatch 注入）。
	submit func(batch *agentlink.LogBatch)

	// batchInterval 攒批上报的时间窗口。
	batchInterval time.Duration
	// maxBatchLines 单批最大行数。
	maxBatchLines int
}

// NewTailer 创建 Tailer。
//
// submit 为批次提交函数（通常传 Client.SubmitBatch）。
func NewTailer(src source.Source, submit func(batch *agentlink.LogBatch)) *Tailer {
	return &Tailer{
		src:            src,
		items:          make(map[int64]*TailingItem),
		submit:         submit,
		batchInterval:  2 * time.Second,
		maxBatchLines:  500,
	}
}

// Start 启动 Tailer。
func (t *Tailer) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.running = true
}

// Stop 停止全部跟踪。
func (t *Tailer) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.running = false
	for id, it := range t.items {
		it.cancel()
		delete(t.items, id)
	}
}

// ApplyItems 全量替换跟踪列表。
//
// 服务端在注册应答或配置下发时调用：新增的 item 开始跟踪，
// 已移除的 item 停止跟踪，配置变化的 item 以最新配置重启。
func (t *Tailer) ApplyItems(specs []agentlink.ItemSpec) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return
	}

	next := make(map[int64]*TailingItem, len(specs))
	for _, spec := range specs {
		if spec.Status != "1" {
			continue
		}
		if spec.ItemID == 0 {
			continue
		}
		next[spec.ItemID] = &TailingItem{Spec: spec}
	}

	// 停止已移除或配置变化的 item。
	for id, cur := range t.items {
		nw, ok := next[id]
		if !ok || itemFilePath(nw.Spec) != itemFilePath(cur.Spec) {
			cur.cancel()
			delete(t.items, id)
			log.Printf("[tailer] 停止跟踪 item=%d", id)
		}
	}

	// 启动新增的 item。
	for id, nw := range next {
		if _, ok := t.items[id]; ok {
			continue
		}
		nw.FilePath = itemFilePath(nw.Spec)
		ctx, cancel := context.WithCancel(context.Background())
		nw.cancel = cancel
		t.items[id] = nw
		go t.tailOne(ctx, nw)
		log.Printf("[tailer] 开始跟踪 item=%d file=%s", id, nw.FilePath)
	}
}

// tailOne 跟踪单个文件并把新增行批量上报。
func (t *Tailer) tailOne(ctx context.Context, item *TailingItem) {
	cfg := source.TailConfig{}

	// 攒批缓冲：把 callback 收到的行先攒起来，按时间窗口批量提交。
	buf := make([]string, 0, t.maxBatchLines)
	mu := sync.Mutex{}

	flush := func() {
		mu.Lock()
		if len(buf) == 0 {
			mu.Unlock()
			return
		}
		lines := make([]string, len(buf))
		copy(lines, buf)
		buf = buf[:0]
		mu.Unlock()

		batch := &agentlink.LogBatch{
			BatchNo:   fmt.Sprintf("%d-%d", item.Spec.ItemID, time.Now().UnixNano()),
			ClientID:  0, // 服务端会强制覆写为连接解析的 ID
			ItemID:    item.Spec.ItemID,
			Lines:     lines,
			Timestamp: time.Now().Unix(),
		}
		t.submit(batch)
	}

	// 定时 flush 协程。
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(t.batchInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				flush()
			}
		}
	}()

	callback := func(line string) error {
		mu.Lock()
		buf = append(buf, line)
		shouldFlush := len(buf) >= t.maxBatchLines
		mu.Unlock()
		if shouldFlush {
			flush()
		}
		return nil
	}

	if err := t.src.Tail(ctx, item.FilePath, cfg, callback); err != nil {
		log.Printf("[tailer] item=%d 跟踪结束: %v", item.Spec.ItemID, err)
	}
	flush()
}

// TrackedCount 返回当前跟踪的 item 数（用于日志/监控）。
func (t *Tailer) TrackedCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.items)
}

// itemFilePath 返回 item 当日日志文件完整路径。
//
// 与 processItem 的拼接规则保持一致：LogPath + LogPrefix + yyyyMMdd + LogSuffix + ".log"。
func itemFilePath(s agentlink.ItemSpec) string {
	date := time.Now().Format("20060102")
	return s.LogPath + s.LogPrefix + date + s.LogSuffix + ".log"
}
