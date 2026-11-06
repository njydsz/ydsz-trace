// Package logs 子文件：Agent 扩展工具（V2.3-03）。
//
// 补齐 PRD FR-903 要求的 7 类工具中的剩余 3 类：
//
//   - list_templates  模板列表（快速定位高频异常模式）
//   - get_anomalies   异常事件查询（异常检测器输出）
//   - diff_time_window 时间窗对比（判断异常是否为新发生）
//
// 加上既有的 search_logs / agg_stats / get_context / get_trace，共 7 类。
//
// 安全约束：全部为只读工具，不接受任何写操作参数。
package logs

import (
	"context"
	"fmt"
	"time"

	models "ydsz-trace/logs/models"
)

// ListTemplatesTool 列出高频模板。
type ListTemplatesTool struct{}

// Name 返回工具名。
func (t *ListTemplatesTool) Name() string { return "list_templates" }

// Description 返回工具描述。
func (t *ListTemplatesTool) Description() string {
	return "列出当前时间窗内命中次数最高的日志模板，用于快速定位异常模式。参数：timeRange（1h/6h/24h）、topN（默认 10）"
}

// listTemplatesResult 工具结果。
type listTemplatesResult struct {
	Templates []templateBrief `json:"templates"`
	Total     int             `json:"total"`
}

// templateBrief 模板摘要。
type templateBrief struct {
	TemplateHash string `json:"templateHash"`
	Sample       string `json:"sample"`
	Count        int    `json:"count"`
	Level        string `json:"level"`
}

// Execute 实现 AgentTool。
func (t *ListTemplatesTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	timeRange := toString(params["timeRange"])
	if timeRange == "" {
		timeRange = "1h"
	}
	topN := toInt(params["topN"], 10)
	if topN <= 0 || topN > 50 {
		topN = 10
	}

	if models.DB == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}

	patterns, err := models.TopPatterns(0, timeRange, topN)
	if err != nil {
		return nil, fmt.Errorf("查询模板失败: %w", err)
	}

	out := listTemplatesResult{Templates: make([]templateBrief, 0, len(patterns))}
	for _, p := range patterns {
		out.Templates = append(out.Templates, templateBrief{
			TemplateHash: p.TemplateHash,
			Sample:       truncateStr(p.Sample, 160),
			Count:        p.Count,
			Level:        extractLevel(p.Sample),
		})
	}
	out.Total = len(out.Templates)
	return out, nil
}

// GetAnomaliesTool 查询异常事件。
type GetAnomaliesTool struct{}

// Name 返回工具名。
func (t *GetAnomaliesTool) Name() string { return "get_anomalies" }

// Description 返回工具描述。
func (t *GetAnomaliesTool) Description() string {
	return "查询异常检测引擎产出的事件（突增/新增/消失）。参数：timeRange（默认 24h）、limit（默认 20）"
}

// getAnomaliesResult 工具结果。
type getAnomaliesResult struct {
	Anomalies []anomalyBrief `json:"anomalies"`
	Total     int            `json:"total"`
}

// anomalyBrief 异常摘要。
type anomalyBrief struct {
	Time         string  `json:"time"`
	TemplateHash string  `json:"templateHash"`
	Type         string  `json:"type"`
	Score        float64 `json:"score"`
	Description  string  `json:"description"`
}

// Execute 实现 AgentTool。
func (t *GetAnomaliesTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	timeRange := toString(params["timeRange"])
	if timeRange == "" {
		timeRange = "24h"
	}
	limit := toInt(params["limit"], 20)
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	if models.DB == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}

	d, err := parseDurationForAgent(timeRange)
	if err != nil {
		return nil, fmt.Errorf("timeRange 非法: %w", err)
	}

	from := time.Now().Add(-d).Format("2006-01-02 15:04:05")
	to := time.Now().Format("2006-01-02 15:04:05")

	events, err := models.QueryAnomalyEvents(0, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("查询异常事件失败: %w", err)
	}

	out := getAnomaliesResult{Anomalies: make([]anomalyBrief, 0, len(events))}
	for _, ev := range events {
		out.Anomalies = append(out.Anomalies, anomalyBrief{
			Time:         ev.CreatedTime,
			TemplateHash: ev.TemplateId,
			Type:         ev.AnomalyType,
			Score:        ev.Score,
			Description:  truncateStr(ev.Description, 160),
		})
	}
	out.Total = len(out.Anomalies)
	return out, nil
}

// DiffTimeWindowTool 时间窗对比。
type DiffTimeWindowTool struct{}

// Name 返回工具名。
func (t *DiffTimeWindowTool) Name() string { return "diff_time_window" }

// Description 返回工具描述。
func (t *DiffTimeWindowTool) Description() string {
	return "对比两个时间窗的模板命中量，判断异常是新增还是持续存在。参数：currentWindow（默认 1h）、baselineWindow（默认 24h）、topN（默认 10）"
}

// diffTimeWindowResult 工具结果。
type diffTimeWindowResult struct {
	Entries []diffEntry `json:"entries"`
}

// diffEntry 单条对比。
type diffEntry struct {
	TemplateHash   string  `json:"templateHash"`
	CurrentCount   int     `json:"currentCount"`
	BaselinePerWin float64 `json:"baselinePerWindow"`
	DeltaPercent   float64 `json:"deltaPercent"`
	Trend          string  `json:"trend"` // surge / drop / stable / new
}

// Execute 实现 AgentTool。
func (t *DiffTimeWindowTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	currentWindow := toString(params["currentWindow"])
	if currentWindow == "" {
		currentWindow = "1h"
	}
	baselineWindow := toString(params["baselineWindow"])
	if baselineWindow == "" {
		baselineWindow = "24h"
	}
	topN := toInt(params["topN"], 10)
	if topN <= 0 || topN > 50 {
		topN = 10
	}

	if models.DB == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}

	curDur, err := parseDurationForAgent(currentWindow)
	if err != nil {
		return nil, fmt.Errorf("currentWindow 非法: %w", err)
	}
	baseDur, err := parseDurationForAgent(baselineWindow)
	if err != nil {
		return nil, fmt.Errorf("baselineWindow 非法: %w", err)
	}
	if baseDur <= 0 {
		return nil, fmt.Errorf("baselineWindow 必须为正")
	}

	// 当前窗模板
	curPatterns, err := models.TopPatterns(0, currentWindow, topN*2)
	if err != nil {
		return nil, fmt.Errorf("查询当前窗失败: %w", err)
	}
	// 基线窗模板
	basePatterns, err := models.TopPatterns(0, baselineWindow, topN*2)
	if err != nil {
		return nil, fmt.Errorf("查询基线窗失败: %w", err)
	}

	baseMap := make(map[string]int, len(basePatterns))
	for _, p := range basePatterns {
		baseMap[p.TemplateHash] = p.Count
	}

	out := diffTimeWindowResult{Entries: make([]diffEntry, 0, topN)}
	for i, p := range curPatterns {
		if i >= topN {
			break
		}

		// 基线按窗口数归一化，得到每窗平均命中
		windows := baseDur / curDur
		if windows < 1 {
			windows = 1
		}
		baseCount, exists := baseMap[p.TemplateHash]
		perWin := float64(baseCount) / float64(windows)

		entry := diffEntry{
			TemplateHash:   p.TemplateHash,
			CurrentCount:   p.Count,
			BaselinePerWin: roundTo(perWin, 1),
		}

		switch {
		case !exists || perWin == 0:
			entry.Trend = "new"
			entry.DeltaPercent = 100
		default:
			delta := (float64(p.Count) - perWin) / perWin * 100
			entry.DeltaPercent = roundTo(delta, 1)
			switch {
			case delta > 50:
				entry.Trend = "surge"
			case delta < -50:
				entry.Trend = "drop"
			default:
				entry.Trend = "stable"
			}
		}

		out.Entries = append(out.Entries, entry)
	}
	return out, nil
}

// roundTo 保留指定小数位。
func roundTo(v float64, digits int) float64 {
	factor := 1.0
	for i := 0; i < digits; i++ {
		factor *= 10
	}
	return float64(int64(v*factor+0.5)) / factor
}
