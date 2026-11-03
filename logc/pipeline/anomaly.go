// Package pipeline logc 端侧管道（PRD 6.2 L0→L1→L2→L3）。
//
// AnomalyPipeline 封装 pkg/stats.AnomalyEngine，负责异常检测与基线更新（L1 统计层）。
package pipeline

import (
	"ydsz-trace/pkg/stats"
)

// AnomalyPipeline 端侧异常检测管道（L1 统计层）。
//
// 封装 pkg/stats.AnomalyEngine，提供：
//   - 单模板异常检测 Detect
//   - 基线更新 UpdateBaseline
//
// 异常判定逻辑复用 AnomalyEngine.DetectAnomalies（Z-score > 3 或泊松 P < 0.01）。
type AnomalyPipeline struct {
	engine *stats.AnomalyEngine
}

// NewAnomalyPipeline 创建异常检测管道实例。
func NewAnomalyPipeline() *AnomalyPipeline {
	return &AnomalyPipeline{
		engine: stats.NewAnomalyEngine(),
	}
}

// Detect 对单个模板的当前计数做异常检测。
//
// 参数：
//   - templateHash 模板 hash（FNV-1a）
//   - count        当前时间桶的命中次数
//   - timestamp    当前时间戳（用于日志与调试）
//
// 返回异常事件；若未达异常阈值则返回 (nil, nil)。
func (p *AnomalyPipeline) Detect(templateHash string, count int, timestamp string) (*stats.AnomalyEvent, error) {
	if templateHash == "" {
		return nil, ErrEmptyTemplateHash
	}
	if count < 0 {
		count = 0
	}

	current := map[string]int{templateHash: count}
	events := p.engine.DetectAnomalies(current, timestamp)
	if len(events) == 0 {
		return nil, nil
	}

	// 取第一个匹配的事件（按 templateHash 过滤）
	for i := range events {
		if events[i].TemplateHash == templateHash {
			return &events[i], nil
		}
	}
	return nil, nil
}

// UpdateBaseline 更新指定模板的基线（滑动窗口）。
//
// 每次调用将当前计数追加到该模板的滑动窗口（最近 30 桶），
// 用于后续异常检测的均值/标准差计算。
func (p *AnomalyPipeline) UpdateBaseline(templateHash string, itemID int64, count int) {
	if templateHash == "" {
		return
	}
	p.engine.UpdateBaseline(templateHash, itemID, count)
}

// GetBaseline 读取指定模板的基线状态（用于测试与调试）。
func (p *AnomalyPipeline) GetBaseline(templateHash string, itemID int64) (*stats.Baseline, bool) {
	return p.engine.GetBaseline(templateHash, itemID)
}
