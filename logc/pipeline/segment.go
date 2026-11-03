// Package pipeline logc 端侧管道（PRD 6.2 L0→L1→L2→L3）。
//
// SegmentWriter 为纯内存段存储，不依赖数据库。
package pipeline

import (
	"sort"
	"sync"
)

// LocalSegment 本地段结构（纯内存，不落库）。
//
// 按 (ItemID, Date, Hour, TemplateHash) 聚合的日志段统计信息。
type LocalSegment struct {
	// ItemID 日志项 ID
	ItemID int64 `json:"itemId"`
	// Date 日期 YYYYMMDD
	Date string `json:"date"`
	// Hour 小时 "00"~"23"
	Hour string `json:"hour"`
	// TemplateHash 模板 hash（FNV-1a）
	TemplateHash string `json:"templateHash"`
	// LineCount 该段总行数
	LineCount int `json:"lineCount"`
	// ErrorCount ERROR/FATAL 行数
	ErrorCount int `json:"errorCount"`
	// WarnCount WARN/WARNING 行数
	WarnCount int `json:"warnCount"`
}

// SegmentWriter 纯内存段写入器（线程安全）。
//
// 职责：
//   - 接收 Add 写入的段并按 (ItemID, Date) 二级索引存储
//   - 支持按 (ItemID, Date) 查询段列表
//
// 该组件不落库，仅用于端侧内存聚合与快速查询。
type SegmentWriter struct {
	mu       sync.RWMutex
	segments []LocalSegment
}

// NewSegmentWriter 创建段写入器实例。
func NewSegmentWriter() *SegmentWriter {
	return &SegmentWriter{
		segments: make([]LocalSegment, 0, 64),
	}
}

// Add 添加一个段到内存存储。
//
// 线程安全：内部使用写锁。
func (w *SegmentWriter) Add(segment LocalSegment) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.segments = append(w.segments, segment)
}

// Query 按 (ItemID, Date) 查询段列表。
//
// 返回副本切片，避免外部修改内部状态。
// 结果按 Hour 升序、TemplateHash 升序排序。
func (w *SegmentWriter) Query(itemID int64, date string) []LocalSegment {
	w.mu.RLock()
	defer w.mu.RUnlock()

	result := make([]LocalSegment, 0)
	for _, seg := range w.segments {
		if seg.ItemID == itemID && seg.Date == date {
			result = append(result, seg)
		}
	}

	// 按 Hour 升序、TemplateHash 升序排序
	sort.Slice(result, func(i, j int) bool {
		if result[i].Hour != result[j].Hour {
			return result[i].Hour < result[j].Hour
		}
		return result[i].TemplateHash < result[j].TemplateHash
	})
	return result
}

// Clear 清理所有段（用于测试或重置）。
func (w *SegmentWriter) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.segments = w.segments[:0]
}

// Count 返回当前段总数（用于测试）。
func (w *SegmentWriter) Count() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.segments)
}
