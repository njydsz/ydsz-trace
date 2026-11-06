// Package models 子文件：审计日志查询与统计（V2.4-06，FR-309）。
package models

import (
	"context"
	"fmt"
	"time"
)

// AuditActionStat 单类操作统计。
type AuditActionStat struct {
	// Action 操作类型
	Action string `json:"action"`
	// Count 次数
	Count int `json:"count"`
	// SuccessCount 成功次数
	SuccessCount int `json:"successCount"`
	// FailCount 失败次数
	FailCount int `json:"failCount"`
}

// AuditStats 按操作类型聚合统计。
func AuditStats(from, to time.Time) ([]AuditActionStat, error) {
	if DB == nil {
		return nil, fmt.Errorf("DB 未初始化")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := DB.QueryContext(ctx, `SELECT action,
			COUNT(*) AS count,
			SUM(CASE WHEN result = 'success' THEN 1 ELSE 0 END) AS success_count,
			SUM(CASE WHEN result = 'fail' THEN 1 ELSE 0 END) AS fail_count
		FROM t_audit_log
		WHERE created_time BETWEEN ? AND ?
		GROUP BY action
		ORDER BY count DESC`,
		from.Format("2006-01-02 15:04:05"),
		to.Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("审计统计失败: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	out := make([]AuditActionStat, 0, 8)
	for rows.Next() {
		var s AuditActionStat
		var success, fail *int
		if err := rows.Scan(&s.Action, &s.Count, &success, &fail); err != nil {
			return nil, fmt.Errorf("扫描审计统计失败: %w", err)
		}
		if success != nil {
			s.SuccessCount = *success
		}
		if fail != nil {
			s.FailCount = *fail
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// RecentAuditActions 返回最近出现过的操作类型清单（供筛选下拉使用）。
func RecentAuditActions(limit int) ([]string, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if DB == nil {
		return nil, fmt.Errorf("DB 未初始化")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var actions []string
	err := DB.SelectContext(ctx, &actions,
		`SELECT DISTINCT action FROM t_audit_log ORDER BY action LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询操作类型失败: %w", err)
	}
	return actions, nil
}
