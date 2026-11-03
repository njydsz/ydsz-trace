// logc 子文件：读取位点定期落盘（V2.2-08，FR-102）。
//
// 设计：
//   - 位点在内存中更新（每次 Read 后回写），由本 goroutine 每 30s 落盘一次
//   - 收到 SIGTERM/SIGINT 时由 main 的退出流程触发最终 Flush
//   - FileSource 之外的类型（Docker/K8s）无位点概念，直接跳过
package main

import (
	"context"
	"log"
	"time"

	"ydsz-trace/pkg/source"
)

// offsetFlushInterval 位点落盘间隔。
//
// 30s 是权衡值：重启最多丢失 30s 的读取进度（会重复分析，不会丢失），
// 同时避免高频写盘对 IO 的影响。
const offsetFlushInterval = 30 * time.Second

// startOffsetFlusher 启动位点定期落盘 goroutine。
//
// 非 FileSource（无位点存储）时立即返回，不启动 goroutine。
func startOffsetFlusher(src source.Source) {
	fs, ok := src.(*source.FileSource)
	if !ok {
		log.Printf("[source] %T 无位点存储，跳过定期落盘", src)
		return
	}

	store := fs.OffsetStore()
	if store == nil {
		log.Printf("[source] FileSource 未注入位点存储，跳过定期落盘")
		return
	}

	go func() {
		ticker := time.NewTicker(offsetFlushInterval)
		defer ticker.Stop()

		log.Printf("[source] 位点定期落盘已启动，间隔 %s", offsetFlushInterval)
		for {
			select {
			case <-ticker.C:
				if err := store.Flush(); err != nil {
					log.Printf("[source] 位点落盘失败: %v", err)
				}
			}
		}
	}()
}

// flushOffsetsNow 立即落盘位点（供退出流程调用）。
//
// 失败仅记录日志，不影响退出。
func flushOffsetsNow(src source.Source) {
	fs, ok := src.(*source.FileSource)
	if !ok {
		return
	}
	store := fs.OffsetStore()
	if store == nil {
		return
	}
	if err := store.Flush(); err != nil {
		log.Printf("[source] 退出时位点落盘失败: %v", err)
		return
	}
	log.Printf("[source] 位点已落盘，共 %d 个文件", store.Count())
}

// offsetFlusherCtx 供 context 取消驱动落盘的辅助（预留给退出流程）。
func offsetFlusherCtx(ctx context.Context, src source.Source) {
	<-ctx.Done()
	flushOffsetsNow(src)
}
