// Package logs 子文件：Agent 工具调用指标收集（V2.3-03）。
//
// 目的：
//   PRD FR-903 要求「工具成功率 ≥95%」。没有度量就无法验收，
//   因此需要记录每次工具调用的成败，并提供聚合查询。
//
// 设计：
//   - 内存计数器（按工具名分组），进程重启归零
//   - 记录最近 N 条调用明细，供排查失败原因
//   - 并发安全（Agent 可能被多个会话并发调用）
//
// 为什么用内存而非落库：
//   工具调用是高频低价值数据，落库会让每次 ReAct 都写一次磁盘，
//   与「工具调用 5s 超时」的性能目标冲突。需要长期统计时可后续补表。
package logs

import (
	"sync"
	"time"

	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// toolCallEntry 单条工具调用明细。
type toolCallEntry struct {
	Tool      string    `json:"tool"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	DurationMs int64    `json:"durationMs"`
	CalledAt  time.Time `json:"calledAt"`
}

// toolMetric 单个工具的聚合指标。
type toolMetric struct {
	total   int
	success int
	failure int
}

// toolMetrics 全局工具指标收集器。
type toolMetrics struct {
	mu sync.RWMutex
	// byTool 按工具名聚合
	byTool map[string]*toolMetric
	// recent 最近调用明细（环形，最多 maxRecentEntries 条）
	recent []toolCallEntry
	// recentIdx 环形写指针
	recentIdx int
}

// maxRecentEntries 保留的最近调用明细条数。
const maxRecentEntries = 200

// globalToolMetrics 全局指标实例。
var globalToolMetrics = newToolMetrics()

// newToolMetrics 创建指标收集器。
func newToolMetrics() *toolMetrics {
	return &toolMetrics{
		byTool: make(map[string]*toolMetric),
		recent: make([]toolCallEntry, maxRecentEntries),
	}
}

// Record 记录一次工具调用结果。
func (m *toolMetrics) Record(tool string, success bool, errMsg string, durationMs int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metric, ok := m.byTool[tool]
	if !ok {
		metric = &toolMetric{}
		m.byTool[tool] = metric
	}
	metric.total++
	if success {
		metric.success++
	} else {
		metric.failure++
	}

	// 写入环形明细
	m.recent[m.recentIdx] = toolCallEntry{
		Tool:       tool,
		Success:    success,
		Error:      errMsg,
		DurationMs: durationMs,
		CalledAt:   time.Now(),
	}
	m.recentIdx = (m.recentIdx + 1) % maxRecentEntries
}

// Snapshot 返回当前聚合指标快照。
func (m *toolMetrics) Snapshot() []AgentToolStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]AgentToolStats, 0, len(m.byTool))
	for name, metric := range m.byTool {
		rate := 0.0
		if metric.total > 0 {
			rate = float64(metric.success) / float64(metric.total)
		}
		out = append(out, AgentToolStats{
			Tool:         name,
			TotalCalls:   metric.total,
			SuccessCalls: metric.success,
			FailureCalls: metric.failure,
			SuccessRate:  rate,
		})
	}
	return out
}

// RecentCalls 返回最近调用明细（按时间倒序）。
func (m *toolMetrics) RecentCalls(limit int) []toolCallEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > maxRecentEntries {
		limit = maxRecentEntries
	}

	out := make([]toolCallEntry, 0, limit)
	// 从最新往回取
	for i := 0; i < maxRecentEntries && len(out) < limit; i++ {
		idx := (m.recentIdx - 1 - i + maxRecentEntries) % maxRecentEntries
		entry := m.recent[idx]
		// 未写入的槽位（CalledAt 为零值）跳过
		if entry.CalledAt.IsZero() {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// collectToolStats 汇总工具成功率（供 HTTP handler 调用）。
func collectToolStats() []AgentToolStats {
	return globalToolMetrics.Snapshot()
}

// recordToolCall 记录工具调用（供 ReAct 循环调用）。
func recordToolCall(tool string, success bool, errMsg string, durationMs int64) {
	globalToolMetrics.Record(tool, success, errMsg, durationMs)
}

// AgentRecentCallsHandler GET /logs/agent/calls/recent
//
// 返回最近的工具调用明细，用于排查失败原因。
func AgentRecentCallsHandler(c *gin.Context) {
	// 简单解析 limit 参数（上限 200）
	limit := 20
	if v := c.Query("limit"); v != "" {
		n := 0
		for _, r := range v {
			if r < '0' || r > '9' {
				n = 0
				break
			}
			n = n*10 + int(r-'0')
		}
		if n > 0 {
			limit = n
		}
	}
	if limit > maxRecentEntries {
		limit = maxRecentEntries
	}

	api.Success(c, "查询成功", gin.H{
		"calls": globalToolMetrics.RecentCalls(limit),
	})
}
