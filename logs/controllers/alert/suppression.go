// Package alert 子文件：异常抑制与合并（V2.2-02）。
//
// 背景：
//   V2.1 已有两层降噪——按 ruleID 的冷却窗口（dedupeWindow）与按模板的
//   5 分钟聚合（Aggregator）。但仍有两类噪声未覆盖：
//
//   1. 维护期噪声：计划内发布/维护期间日志量天然异常，应整体静默
//   2. 跨算法重复：同一模板同一桶可能被 Z-score / EWMA / 周期基线同时命中，
//      产生多条内容相近的异常事件
//
// 本文件补齐这两类抑制能力，并提供统一的 ShouldSuppress 判定入口。
package alert

import (
	"strings"
	"sync"
	"time"
)

// MaintenanceWindow 计划内维护期（静默窗口）。
//
// 支持多个窗口并存（例如同时有发布窗口与巡检窗口）。
// 窗口时间以本地时区解释，支持跨天（例如 23:00~01:00）。
type MaintenanceWindow struct {
	Name      string    `json:"name"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Recurring string    `json:"recurring,omitempty"` // "daily" | "weekly" | ""（一次性）
	Weekdays  []int     `json:"weekdays,omitempty"`  // weekly 时生效，0=周日
	Enabled   bool      `json:"enabled"`
}

// Suppressor 静默窗口管理器（并发安全）。
//
// 内存态设计：窗口由 API 写入，重启后从数据库恢复（预留 Persist 接口）。
type Suppressor struct {
	mu      sync.RWMutex
	windows []MaintenanceWindow
}

// NewSuppressor 创建空静默窗口管理器。
func NewSuppressor() *Suppressor {
	return &Suppressor{windows: make([]MaintenanceWindow, 0, 2)}
}

// ReplaceWindows 全量替换静默窗口（配置保存时调用）。
func (s *Suppressor) ReplaceWindows(windows []MaintenanceWindow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.windows = append([]MaintenanceWindow(nil), windows...)
}

// Windows 返回当前窗口快照。
func (s *Suppressor) Windows() []MaintenanceWindow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]MaintenanceWindow(nil), s.windows...)
}

// IsSuppressed 判断给定时刻是否落在任一启用的静默窗口内。
//
// 返回 (是否静默, 命中的窗口名)。
func (s *Suppressor) IsSuppressed(now time.Time) (bool, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, w := range s.windows {
		if !w.Enabled {
			continue
		}
		if inWindow(now, w) {
			return true, w.Name
		}
	}
	return false, ""
}

// inWindow 判断时刻是否在窗口内（处理跨天与周期性规则）。
func inWindow(now time.Time, w MaintenanceWindow) bool {
	switch w.Recurring {
	case "daily":
		return timeOfDayInRange(now, w.Start, w.End)
	case "weekly":
		if len(w.Weekdays) > 0 && !containsWeekday(w.Weekdays, int(now.Weekday())) {
			return false
		}
		return timeOfDayInRange(now, w.Start, w.End)
	default:
		// 一次性窗口：比较完整时间戳
		if now.Before(w.Start) || now.After(w.End) {
			return false
		}
		return true
	}
}

// timeOfDayInRange 仅比较时分秒，忽略日期（用于周期性窗口）。
//
// 支持跨天窗口：当 end < start 时，表示跨过午夜（如 23:00~01:00）。
func timeOfDayInRange(now time.Time, start, end time.Time) bool {
	nowSec := now.Hour()*3600 + now.Minute()*60 + now.Second()
	startSec := start.Hour()*3600 + start.Minute()*60 + start.Second()
	endSec := end.Hour()*3600 + end.Minute()*60 + end.Second()

	if startSec == endSec {
		// 零长度窗口视为全天静默（常用于"全天维护"）
		return true
	}
	if endSec > startSec {
		return nowSec >= startSec && nowSec < endSec
	}
	// 跨天：23:00~01:00 → now >= 23:00 或 now < 01:00
	return nowSec >= startSec || nowSec < endSec
}

// containsWeekday 判断星期列表是否包含给定值。
func containsWeekday(days []int, d int) bool {
	for _, x := range days {
		if x == d {
			return true
		}
	}
	return false
}

// ============ 跨算法异常合并 ============

// AnomalyKey 异常事件的合并键。
//
// 同一模板 + 同一时间桶视为同源；不同算法对同一源头的多次报告合并为一条，
// 保留最高严重度与全部命中算法清单。
type AnomalyKey struct {
	TemplateHash string
	BucketIndex  int
}

// MergeAnomalies 合并同源的异常事件列表。
//
// 合并规则：
//   - 按 (templateHash, bucketIndex) 分组
//   - 每组保留 severity 最高的一条作为主事件
//   - 所有命中算法合并到 Algorithms 字段
//   - 组内取最大的 ZScore 与 CurrentCount
//
// bucketIndex 由调用方从事件时间推导（无时间字段时传 0，即全部归为一组）。
func MergeAnomalies(events []AnomalyEventLike, bucketIndex int) []AnomalyEventLike {
	if len(events) <= 1 {
		return events
	}

	type merged struct {
		primary    AnomalyEventLike
		algorithms []string
		maxScore   float64
		maxCount   int
	}

	groups := make(map[AnomalyKey]*merged)
	order := make([]AnomalyKey, 0, len(events))

	for _, ev := range events {
		key := AnomalyKey{TemplateHash: ev.GetTemplateHash(), BucketIndex: bucketIndex}
		g, ok := groups[key]
		if !ok {
			g = &merged{
				primary:    ev,
				algorithms: []string{ev.GetAlgorithm()},
				maxScore:   ev.GetScore(),
				maxCount:   ev.GetCurrentCount(),
			}
			groups[key] = g
			order = append(order, key)
			continue
		}

		// 保留更严重的主事件
		if SeverityRank(ev.GetSeverity()) > SeverityRank(g.primary.GetSeverity()) {
			g.primary = ev
		}
		if ev.GetScore() > g.maxScore {
			g.maxScore = ev.GetScore()
		}
		if ev.GetCurrentCount() > g.maxCount {
			g.maxCount = ev.GetCurrentCount()
		}
		if !containsString(g.algorithms, ev.GetAlgorithm()) {
			g.algorithms = append(g.algorithms, ev.GetAlgorithm())
		}
	}

	out := make([]AnomalyEventLike, 0, len(order))
	for _, key := range order {
		g := groups[key]
		g.primary.SetScore(g.maxScore)
		g.primary.SetCurrentCount(g.maxCount)
		g.primary.SetAlgorithms(g.algorithms)
		out = append(out, g.primary)
	}
	return out
}

// AnomalyEventLike 合并所需的异常事件视图。
//
// 抽象为接口以避免本文件依赖具体事件类型，便于 pipeline 与 scanner 各自传入。
type AnomalyEventLike interface {
	GetTemplateHash() string
	GetAlgorithm() string
	GetSeverity() string
	GetScore() float64
	GetCurrentCount() int
	SetScore(float64)
	SetCurrentCount(int)
	SetAlgorithms([]string)
}

// containsString 判断字符串切片是否包含目标值。
func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// NormalizeAlgorithms 规整算法清单：去重 + 按字母序排序，保证输出稳定。
func NormalizeAlgorithms(algorithms []string) []string {
	seen := make(map[string]struct{}, len(algorithms))
	out := make([]string, 0, len(algorithms))
	for _, a := range algorithms {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}
	// 简单选择排序（数量极少，无需引入 sort 包的额外开销）
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
