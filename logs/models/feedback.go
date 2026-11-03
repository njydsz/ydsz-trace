// Package models 子文件：异常反馈与阈值自适应（V2.2 FR-807）。
//
// 闭环设计：
//
//	用户在异常时间线上标记「误报」→ 写入 t_anomaly_feedback →
//	按模板聚合最近 N 条反馈 → 计算该模板的自适应 score 阈值 →
//	检测器在判定时读取该阈值，从而自动减少同类误报。
//
// 阈值自适应算法（指数回退）：
//
//   - 误报（false_positive）：阈值上调，上调幅度与该次 score 成正比，
//     确保新的阈值高于触发误报的 score，避免同类再次触发。
//   - 确认（true_positive）：阈值下调（但不低于默认值），让检测更灵敏。
//   - 忽略（ignored）：仅记录，不调整阈值。
//
// 约束：
//   - 自适应阈值被限制在 [DefaultScoreThreshold, MaxScoreThreshold] 区间内
//   - 单模板最近 FeedbackWindow 条反馈参与计算，超出窗口的旧反馈不再影响
//   - 阈值持久化在 t_anomaly_feedback.adjusted_score（最新一条）
package models

import (
	"context"
	"fmt"
	"log"
	"time"
)

// FeedbackType 反馈类型。
type FeedbackType string

const (
	// FeedbackFalsePositive 误报：该异常实际不是问题。
	FeedbackFalsePositive FeedbackType = "false_positive"
	// FeedbackTruePositive 确认：该异常是真实问题。
	FeedbackTruePositive FeedbackType = "true_positive"
	// FeedbackIgnored 忽略：用户主动忽略，不参与阈值调整。
	FeedbackIgnored FeedbackType = "ignored"
)

// 阈值自适应参数。
const (
	// DefaultScoreThreshold 默认 score 阈值（与 stats 包默认 sigma 对齐）。
	DefaultScoreThreshold = 3.0
	// MinScoreThreshold 自适应下限（不允许低于默认值，避免过度灵敏）。
	MinScoreThreshold = 1.5
	// MaxScoreThreshold 自适应上限（防止阈值无限抬高导致漏报）。
	MaxScoreThreshold = 10.0
	// FeedbackWindow 参与自适应计算的反馈条数。
	FeedbackWindow = 20
	// FalsePositiveBumpRatio 误报时的阈值上调比例（相对触发 score）。
	FalsePositiveBumpRatio = 1.15
	// TruePositiveDropRatio 确认时的阈值下调比例。
	TruePositiveDropRatio = 0.95
)

// TAnomalyFeedback 异常反馈记录（表 t_anomaly_feedback）。
type TAnomalyFeedback struct {
	Id           int64       `json:"id" db:"id"`
	AnomalyID    int64       `json:"anomalyId" db:"anomaly_id"`
	TemplateId   string      `json:"templateId" db:"template_id"`
	ItemId       int64       `json:"itemId" db:"item_id"`
	FeedbackType FeedbackType `json:"feedbackType" db:"feedback_type"`
	Reason       string      `json:"reason" db:"reason"`
	AdjustedScore float64    `json:"adjustedScore" db:"adjusted_score"`
	CreatedBy    string      `json:"createdBy" db:"created_by"`
	CreatedTime  string      `json:"createdTime" db:"created_time"`
}

// RecordAnomalyFeedback 写入一条异常反馈。
func RecordAnomalyFeedback(anomalyID int64, templateID string, itemID int64,
	feedbackType FeedbackType, reason, createdBy string) (int64, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().Format("2006-01-02 15:04:05")
	res, err := DB.ExecContext(ctx, `INSERT INTO t_anomaly_feedback
		(anomaly_id, template_id, item_id, feedback_type, reason, created_by, created_time)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		anomalyID, templateID, itemID, string(feedbackType), reason, createdBy, now)
	if err != nil {
		return 0, fmt.Errorf("RecordAnomalyFeedback 失败: %w", err)
	}
	return res.LastInsertId()
}

// GetAnomalyFeedbackWindow 读取指定模板最近 N 条反馈（按时间倒序）。
func GetAnomalyFeedbackWindow(templateID string, limit int) ([]TAnomalyFeedback, error) {
	if limit <= 0 || limit > FeedbackWindow {
		limit = FeedbackWindow
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var rows []TAnomalyFeedback
	err := DB.SelectContext(ctx, &rows, `SELECT id, anomaly_id, template_id, item_id,
		feedback_type, reason, adjusted_score, created_by, created_time
		FROM t_anomaly_feedback
		WHERE template_id = ?
		ORDER BY id DESC LIMIT ?`, templateID, limit)
	if err != nil {
		return nil, fmt.Errorf("GetAnomalyFeedbackWindow 失败: %w", err)
	}
	return rows, nil
}

// ComputeAdaptiveThreshold 计算指定模板的自适应 score 阈值。
//
// 算法：
//  1. 取该模板最近 FeedbackWindow 条反馈
//  2. 从默认阈值出发，逐条应用调整：
//     - 误报：threshold = max(threshold, triggerScore * bumpRatio)
//     - 确认：threshold = threshold * dropRatio
//  3. 结果限制在 [MinScoreThreshold, MaxScoreThreshold]
//
// 参数 triggerScore 为该模板最近一次异常事件的 score（作为误报上调的基准）。
// 返回计算后的阈值；无反馈时返回 DefaultScoreThreshold。
func ComputeAdaptiveThreshold(templateID string, triggerScore float64) (float64, error) {
	feedbacks, err := GetAnomalyFeedbackWindow(templateID, FeedbackWindow)
	if err != nil {
		return DefaultScoreThreshold, err
	}
	if len(feedbacks) == 0 {
		return DefaultScoreThreshold, nil
	}

	threshold := DefaultScoreThreshold
	if triggerScore > 0 {
		// 以最近一次触发 score 作为基准，避免误报上调幅度不足
		threshold = maxFloat(threshold, triggerScore*FalsePositiveBumpRatio)
	}

	for _, fb := range feedbacks {
		switch fb.FeedbackType {
		case FeedbackFalsePositive:
			threshold = maxFloat(threshold, triggerScore*FalsePositiveBumpRatio)
			if triggerScore <= 0 {
				threshold *= FalsePositiveBumpRatio
			}
		case FeedbackTruePositive:
			threshold *= TruePositiveDropRatio
		case FeedbackIgnored:
			// 不调整
		}
	}

	if threshold < MinScoreThreshold {
		threshold = MinScoreThreshold
	}
	if threshold > MaxScoreThreshold {
		threshold = MaxScoreThreshold
	}
	return threshold, nil
}

// PersistAdaptiveThreshold 将自适应阈值写回该模板最近一条反馈记录。
//
// 采用「写回最新一条」而非新增配置表的设计：
//   - 复用现有表，避免为单值配置引入新表
//   - adjusted_score 作为该模板当前生效阈值的快照，供查询接口直接读取
func PersistAdaptiveThreshold(templateID string, threshold float64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := DB.ExecContext(ctx, `UPDATE t_anomaly_feedback
		SET adjusted_score = ?
		WHERE id = (SELECT id FROM t_anomaly_feedback
		            WHERE template_id = ? ORDER BY id DESC LIMIT 1)`,
		threshold, templateID)
	if err != nil {
		return fmt.Errorf("PersistAdaptiveThreshold 失败: %w", err)
	}
	return nil
}

// GetAdaptiveThreshold 读取指定模板当前生效的自适应阈值。
//
// 返回 (阈值, 是否有反馈记录)。无记录时返回默认阈值与 false。
func GetAdaptiveThreshold(templateID string) (float64, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var score float64
	err := DB.GetContext(ctx, &score, `SELECT adjusted_score FROM t_anomaly_feedback
		WHERE template_id = ? AND adjusted_score > 0 ORDER BY id DESC LIMIT 1`, templateID)
	if err != nil {
		return DefaultScoreThreshold, false
	}
	return score, true
}

// QueryAnomalyFeedbackStats 返回反馈统计（按类型分组），供看板使用。
func QueryAnomalyFeedbackStats() (falsePositive, truePositive, ignored int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := DB.QueryContext(ctx, `SELECT feedback_type, COUNT(*)
		FROM t_anomaly_feedback GROUP BY feedback_type`)
	if err != nil {
		log.Printf("[feedback] 统计查询失败: %v", err)
		return 0, 0, 0
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			log.Printf("[feedback] rows.Close 失败: %v", cerr)
		}
	}()

	for rows.Next() {
		var t string
		var n int
		if err := rows.Scan(&t, &n); err != nil {
			continue
		}
		switch FeedbackType(t) {
		case FeedbackFalsePositive:
			falsePositive = n
		case FeedbackTruePositive:
			truePositive = n
		case FeedbackIgnored:
			ignored = n
		}
	}
	return falsePositive, truePositive, ignored
}

// maxFloat 返回较大值。
func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
