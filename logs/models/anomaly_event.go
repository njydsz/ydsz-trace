// Package models 子文件：异常事件持久化（t_anomaly_event 表）。
//
// 异常事件由 pkg/stats.AnomalyEngine 检测产生，记录日志模板的突变/新增/消失。
// 用于排障时间线展示、告警阈值自适应、案例关联。
package models

import (
	"context"
	"fmt"
	"log"
	"time"
)

// AnomalyType 异常事件类型。
type AnomalyType string

const (
	// AnomalySpike 突增（超基线 3σ）。
	AnomalySpike AnomalyType = "spike"
	// AnomalyNew 新模板（首次出现）。
	AnomalyNew AnomalyType = "new"
	// AnomalyDisappear 消失模板（静默）。
	AnomalyDisappear AnomalyType = "disappear"
)

// RecordAnomalyEvent 写入异常事件。
//
// 参数：
//   - templateHash 模板哈希（fnv/sha1 前 8 字节）
//   - itemID 关联日志项 ID
//   - clientID 关联客户端 ID
//   - anomalyType 异常类型
//   - score Z-Score 分数
//   - baseline 基线值
//   - actual 实际观测值
//   - windowSec 检测时间窗口（秒）
//   - description 可读描述
//
// 返回 error（写 DB 失败时）。
func RecordAnomalyEvent(templateHash string, itemID, clientID int64, anomalyType AnomalyType,
	score, baseline, actual float64, windowSec int, description string) error {

	now := time.Now().Format("2006-01-02 15:04:05")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `INSERT INTO t_anomaly_event
		(template_id, item_id, client_id, anomaly_type, score, baseline, actual, window_sec, status, description, created_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?)`
	_, err := DB.ExecContext(ctx, query,
		templateHash, itemID, clientID, string(anomalyType),
		score, baseline, actual, windowSec, description, now)
	if err != nil {
		return fmt.Errorf("RecordAnomalyEvent 失败: %w", err)
	}
	return nil
}

// QueryAnomalyEvents 按 item + 时间范围查询异常事件。
func QueryAnomalyEvents(itemID int64, from, to string, limit int) ([]TAnomalyEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `SELECT id, template_id, item_id, client_id, anomaly_type, score,
		baseline, actual, window_sec, status, description, created_time
		FROM t_anomaly_event
		WHERE item_id = ? AND created_time BETWEEN ? AND ?
		ORDER BY created_time DESC LIMIT ?`
	rows, err := DB.QueryContext(ctx, query, itemID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("QueryAnomalyEvents 失败: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			log.Printf("[anomaly] rows.Close 失败: %v", cerr)
		}
	}()

	events := make([]TAnomalyEvent, 0, 8)
	for rows.Next() {
		var ev TAnomalyEvent
		if err := rows.Scan(&ev.Id, &ev.TemplateId, &ev.ItemId, &ev.ClientId,
			&ev.AnomalyType, &ev.Score, &ev.Baseline, &ev.Actual, &ev.WindowSec,
			&ev.Status, &ev.Description, &ev.CreatedTime); err != nil {
			return nil, fmt.Errorf("扫描 anomaly 行失败: %w", err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 anomaly rows 失败: %w", err)
	}
	return events, nil
}

// GetAnomalyEvent 按 ID 查询单条异常事件。
//
// 查询失败（不存在或 DB 错误）时返回 error，调用方应区分处理。
func GetAnomalyEvent(id int64) (TAnomalyEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var ev TAnomalyEvent
	err := DB.GetContext(ctx, &ev, `SELECT id, template_id, item_id, client_id,
		anomaly_type, score, baseline, actual, window_sec, status, description, created_time
		FROM t_anomaly_event WHERE id = ?`, id)
	return ev, err
}

// UpdateAnomalyStatus 更新异常事件状态（active/acknowledged/closed）。
func UpdateAnomalyStatus(id int64, status string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `UPDATE t_anomaly_event SET status = ? WHERE id = ?`
	_, err := DB.ExecContext(ctx, query, status, id)
	return err
}
