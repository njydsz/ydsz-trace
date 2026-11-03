// Package alert 子文件：stats.AnomalyEvent 与 AnomalyEventLike 的适配。
//
// 目的：让 pkg/stats 的事件类型（不可变结构体）参与 alert 包的合并逻辑，
// 而不需要把合并算法下沉到 pkg（pkg 不应依赖 logs 的告警语义）。
//
// 实现方式：定义适配器包装 *stats.AnomalyEvent，把 Setter 的修改收集到适配器内部，
// 合并后再由 ToStatsEvent 回写为新的 stats 事件。
package alert

import (
	"strings"

	"ydsz-trace/pkg/stats"
)

// statsEventAdapter 将 *stats.AnomalyEvent 适配为 AnomalyEventLike。
//
// 合并过程中只修改 Score / CurrentCount / Algorithms 三个字段，
// 其余字段（模板、方向、严重度）保持主事件不变。
type statsEventAdapter struct {
	event      *stats.AnomalyEvent
	algorithms []string
}

// GetTemplateHash 实现 AnomalyEventLike。
func (a *statsEventAdapter) GetTemplateHash() string { return a.event.TemplateHash }

// GetAlgorithm 实现 AnomalyEventLike。
func (a *statsEventAdapter) GetAlgorithm() string { return a.event.Algorithm }

// GetSeverity 实现 AnomalyEventLike。
func (a *statsEventAdapter) GetSeverity() string { return a.event.Severity }

// GetScore 实现 AnomalyEventLike。
func (a *statsEventAdapter) GetScore() float64 { return a.event.ZScore }

// GetCurrentCount 实现 AnomalyEventLike。
func (a *statsEventAdapter) GetCurrentCount() int { return a.event.CurrentCount }

// SetScore 实现 AnomalyEventLike。
func (a *statsEventAdapter) SetScore(v float64) { a.event.ZScore = v }

// SetCurrentCount 实现 AnomalyEventLike。
func (a *statsEventAdapter) SetCurrentCount(v int) { a.event.CurrentCount = v }

// SetAlgorithms 实现 AnomalyEventLike（算法清单写入适配器，不写回事件）。
func (a *statsEventAdapter) SetAlgorithms(list []string) { a.algorithms = list }

// Algorithms 返回合并后的算法清单。
func (a *statsEventAdapter) Algorithms() []string { return a.algorithms }

// mergeStatsEvents 合并 stats.AnomalyEvent 列表（同源去重 + 保留最高严重度）。
//
// 合并后通过 Description 追加算法清单标记（stats.AnomalyEvent 无 Algorithms 字段，
// 用 Description 承载合并信息，避免修改 pkg 结构体签名）。
func mergeStatsEvents(events []stats.AnomalyEvent, bucketIndex int) []stats.AnomalyEvent {
	if len(events) <= 1 {
		return events
	}

	adapters := make([]AnomalyEventLike, len(events))
	for i := range events {
		ev := events[i]
		adapters[i] = &statsEventAdapter{event: &ev}
	}

	merged := MergeAnomalies(adapters, bucketIndex)

	out := make([]stats.AnomalyEvent, 0, len(merged))
	for _, m := range merged {
		ad, ok := m.(*statsEventAdapter)
		if !ok {
			continue
		}
		ev := *ad.event
		if algs := NormalizeAlgorithms(ad.Algorithms()); len(algs) > 1 {
			ev.Direction = ev.Direction + " [" + strings.Join(algs, "+") + "]"
		}
		out = append(out, ev)
	}
	return out
}
