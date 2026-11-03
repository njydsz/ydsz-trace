// Package index logc 端侧本地索引。
//
// 提供基于内存 map 的二级索引（itemID + date），支持：
//   - 按模板 hash 检索
//   - 按时间范围检索
//   - 按日期清理
//
// 所有操作线程安全（sync.RWMutex）。
package index

import (
	"sort"
	"sync"
	"time"
)

// IndexEntry 单条索引项。
type IndexEntry struct {
	// LineNumber 行号（1-based）
	LineNumber int `json:"lineNumber"`
	// Timestamp 时间戳字符串
	Timestamp string `json:"timestamp"`
	// TemplateHash 模板 hash（FNV-1a）
	TemplateHash string `json:"templateHash"`
	// Level 日志级别
	Level string `json:"level"`
	// Content 日志原文
	Content string `json:"content"`
}

// LocalIndex 本地内存索引（线程安全）。
//
// 数据结构：
//   - 第一级 key: itemID
//   - 第二级 key: date (YYYYMMDD)
//   - 值: 按 InsertionOrder 排序的 IndexEntry 切片
type LocalIndex struct {
	mu      sync.RWMutex
	entries map[int64]map[string][]IndexEntry
}

// NewLocalIndex 创建本地索引实例。
func NewLocalIndex() *LocalIndex {
	return &LocalIndex{
		entries: make(map[int64]map[string][]IndexEntry),
	}
}

// Add 添加一条索引项。
//
// 按 (itemID, date) 二级索引存储；date 为 YYYYMMDD 格式。
func (idx *LocalIndex) Add(itemID int64, date string, entry IndexEntry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// 初始化二级 map
	if idx.entries[itemID] == nil {
		idx.entries[itemID] = make(map[string][]IndexEntry)
	}
	idx.entries[itemID][date] = append(idx.entries[itemID][date], entry)
}

// SearchByTemplate 按模板 hash 检索索引项。
//
// 返回指定 itemID 下所有日期中匹配 templateHash 的条目，
// 按 Timestamp 升序排列。
func (idx *LocalIndex) SearchByTemplate(itemID int64, hash string) []IndexEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	dateMap, ok := idx.entries[itemID]
	if !ok {
		return nil
	}

	var result []IndexEntry
	for _, entries := range dateMap {
		for _, e := range entries {
			if e.TemplateHash == hash {
				result = append(result, e)
			}
		}
	}

	// 按时间戳升序排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp < result[j].Timestamp
	})
	return result
}

// SearchByTimeRange 按时间范围检索索引项。
//
// start/end 为时间范围（包含边界）。返回指定 itemID 下所有日期中
// 时间戳在 [start, end] 范围内的条目，按 Timestamp 升序排列。
//
// 时间戳格式支持：
//   - "2006-01-02 15:04:05"
//   - "2006-01-02T15:04:05"
//   - RFC3339
func (idx *LocalIndex) SearchByTimeRange(itemID int64, start, end time.Time) []IndexEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	dateMap, ok := idx.entries[itemID]
	if !ok {
		return nil
	}

	var result []IndexEntry
	for _, entries := range dateMap {
		for _, e := range entries {
			ts := parseTimestamp(e.Timestamp)
			if ts.IsZero() {
				continue
			}
			// 包含边界
			if (ts.Equal(start) || ts.After(start)) && (ts.Equal(end) || ts.Before(end)) {
				result = append(result, e)
			}
		}
	}

	// 按时间戳升序排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp < result[j].Timestamp
	})
	return result
}

// Clear 清理指定 (itemID, date) 的索引。
//
// date 为 YYYYMMDD 格式。
func (idx *LocalIndex) Clear(itemID int64, date string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if dateMap, ok := idx.entries[itemID]; ok {
		delete(dateMap, date)
		// 若该 itemID 下已无日期，清理一级 map
		if len(dateMap) == 0 {
			delete(idx.entries, itemID)
		}
	}
}

// Count 返回指定 (itemID, date) 的索引条目数（用于测试）。
func (idx *LocalIndex) Count(itemID int64, date string) int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if dateMap, ok := idx.entries[itemID]; ok {
		return len(dateMap[date])
	}
	return 0
}

// TotalCount 返回全部索引条目总数（用于测试）。
func (idx *LocalIndex) TotalCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	total := 0
	for _, dateMap := range idx.entries {
		for _, entries := range dateMap {
			total += len(entries)
		}
	}
	return total
}

// parseTimestamp 解析时间戳字符串，支持多种格式。
func parseTimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}

	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05.000",
	}

	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
