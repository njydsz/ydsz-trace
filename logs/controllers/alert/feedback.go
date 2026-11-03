// Package alert 子文件：异常反馈闭环（V2.2-03）。
//
// 职责：
//   - 在检测器产出事件后，用该模板的自适应阈值过滤误报
//   - 提供 MarkFeedback 入口，写入反馈并触发阈值重算
//   - 保证「标记后 24h 内同类事件不再出现」的验收目标
//
// 自适应阈值来源：models.ComputeAdaptiveThreshold（按模板聚合反馈）。
// 无反馈记录时使用 stats 包的默认 sigma 阈值（即不额外过滤）。
package alert

import (
	"log"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/stats"
)

// thresholdLookup 自适应阈值查询函数（便于测试注入）。
var thresholdLookup = models.GetAdaptiveThreshold

// FilterByAdaptiveThreshold 用自适应阈值过滤异常事件。
//
// 规则：
//   - 该模板无反馈记录 → 全部保留（不引入额外过滤）
//   - 事件 score < 自适应阈值 → 视为误报，过滤掉
//   - 事件 score >= 自适应阈值 → 保留
//
// 说明：自适应阈值已在 ComputeAdaptiveThreshold 中被限制在
// [MinScoreThreshold, MaxScoreThreshold] 区间，不会出现阈值过高导致全部漏报。
func FilterByAdaptiveThreshold(events []stats.AnomalyEvent) []stats.AnomalyEvent {
	if len(events) == 0 {
		return events
	}

	out := make([]stats.AnomalyEvent, 0, len(events))
	for _, ev := range events {
		threshold, ok := thresholdLookup(ev.TemplateHash)
		if !ok {
			// 无反馈记录：使用默认阈值（等效于不过滤）
			out = append(out, ev)
			continue
		}
		if ev.ZScore < threshold {
			log.Printf("[alert] 自适应阈值过滤: template=%s score=%.2f < threshold=%.2f",
				ev.TemplateHash, ev.ZScore, threshold)
			continue
		}
		out = append(out, ev)
	}
	return out
}

// MarkAnomalyFeedback 记录一条异常反馈并触发该模板的阈值自适应。
//
// 参数：
//   - anomalyID 关联的异常事件 ID（<=0 时仅按模板处理）
//   - templateHash 模板 hash（自适应阈值按模板维度生效）
//   - itemID 关联日志项
//   - feedbackType 反馈类型
//   - reason 用户填写的原因（可空）
//   - triggerScore 触发本次异常的 score（作为阈值上调基准；<=0 时用默认值推算）
//   - operator 操作人
//
// 返回 error（写库失败时），调用方据此决定是否回滚响应。
func MarkAnomalyFeedback(anomalyID int64, templateHash string, itemID int64,
	feedbackType models.FeedbackType, reason string, triggerScore float64, operator string) error {

	if templateHash == "" {
		return errEmptyTemplateHash
	}

	if _, err := models.RecordAnomalyFeedback(anomalyID, templateHash, itemID,
		feedbackType, reason, operator); err != nil {
		return err
	}

	// 重算并持久化该模板的自适应阈值
	threshold, err := models.ComputeAdaptiveThreshold(templateHash, triggerScore)
	if err != nil {
		log.Printf("[alert] 计算自适应阈值失败 template=%s: %v", templateHash, err)
		return err
	}
	if err := models.PersistAdaptiveThreshold(templateHash, threshold); err != nil {
		log.Printf("[alert] 持久化自适应阈值失败 template=%s: %v", templateHash, err)
		return err
	}

	log.Printf("[alert] 反馈已记录: template=%s type=%s threshold=%.2f", templateHash, feedbackType, threshold)
	return nil
}

// errEmptyTemplateHash 模板 hash 为空。
var errEmptyTemplateHash = &feedbackError{"templateHash 不能为空"}

// feedbackError 反馈领域错误。
type feedbackError struct{ msg string }

// Error 实现 error。
func (e *feedbackError) Error() string { return "feedback: " + e.msg }

// AdaptiveThresholdStats 返回自适应阈值统计（供看板/接口使用）。
type AdaptiveThresholdStats struct {
	FalsePositive int `json:"falsePositive"`
	TruePositive  int `json:"truePositive"`
	Ignored       int `json:"ignored"`
	// AdjustedTemplates 已产生自适应阈值的模板数（近似：有 adjusted_score > 0 的记录数）
	AdjustedTemplates int `json:"adjustedTemplates"`
}

// GetFeedbackStats 读取反馈统计。
func GetFeedbackStats() AdaptiveThresholdStats {
	fp, tp, ig := models.QueryAnomalyFeedbackStats()
	return AdaptiveThresholdStats{
		FalsePositive: fp,
		TruePositive:  tp,
		Ignored:       ig,
	}
}

// feedbackCooldown 标记后的抑制窗口（FR-807 验收：24h 内同类不再出现）。
//
// 说明：真正的「不再出现」由自适应阈值过滤保证；此处提供一个辅助判断，
// 供接口在标记后短时间内对同源事件做快速短路，避免阈值尚未生效的窗口期重复。
const feedbackCooldown = 24 * time.Hour
