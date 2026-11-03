// Package models 子文件：告警静默窗口持久化（V2.2 FR-806）。
//
// 静默窗口用于计划内维护期的整体告警抑制，避免发布/巡检期间的日志量异常
// 触发大量无意义告警。存储在 t_maintenance_window 表，由调度器加载到内存生效。
package models

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// MaintenanceWindow 静默窗口配置（表 t_maintenance_window）。
//
// WeekdaysJSON 对应数据库中的 JSON 文本列；Weekdays 为解析后的切片（不直接落库）。
type MaintenanceWindow struct {
	Id          int64  `json:"id" db:"id"`
	Name        string `json:"name" db:"name"`
	StartTime   string `json:"startTime" db:"start_time"` // HH:MM
	EndTime     string `json:"endTime" db:"end_time"`   // HH:MM（支持跨天）
	Recurring   string `json:"recurring" db:"recurring"`
	WeekdaysJSON string `json:"-" db:"weekdays"`
	Weekdays    []int  `json:"weekdays"`
	Enabled     bool   `json:"enabled" db:"enabled"`
	CreatedTime string `json:"createdTime" db:"created_time"`
	UpdatedTime string `json:"updatedTime" db:"updated_time"`
}

// QueryMaintenanceWindows 返回全部静默窗口（含禁用，供配置界面展示）。
func QueryMaintenanceWindows() []MaintenanceWindow {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var rows []MaintenanceWindow
	if err := DB.SelectContext(ctx, &rows, `SELECT id, name, start_time, end_time,
		recurring, weekdays, enabled, created_time, updated_time
		FROM t_maintenance_window ORDER BY id`); err != nil {
		log.Printf("[maintenance] 查询静默窗口失败: %v", err)
		return nil
	}

	for i := range rows {
		rows[i].Weekdays = ParseWeekdaysJSON(rows[i].WeekdaysJSON)
	}
	return rows
}

// ReplaceMaintenanceWindows 全量替换静默窗口配置。
//
// 采用「清空 + 批量插入」而非增量更新：配置项数量少（通常 <10），
// 全量替换能保证数据库状态与请求体完全一致，避免残留脏数据。
func ReplaceMaintenanceWindows(windows []MaintenanceWindow) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer func() {
		if rerr := tx.Rollback(); rerr != nil && rerr.Error() != "sql: transaction has already been committed or rolled back" {
			log.Printf("[maintenance] 事务回滚失败: %v", rerr)
		}
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM t_maintenance_window`); err != nil {
		return fmt.Errorf("清空静默窗口失败: %w", err)
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	for _, w := range windows {
		weekdaysJSON := "[]"
		if len(w.Weekdays) > 0 {
			if b, err := json.Marshal(w.Weekdays); err == nil {
				weekdaysJSON = string(b)
			}
		}
		enabled := 0
		if w.Enabled {
			enabled = 1
		}
		recurring := w.Recurring
		if recurring == "" {
			recurring = "daily"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO t_maintenance_window
			(name, start_time, end_time, recurring, weekdays, enabled, created_time, updated_time)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			w.Name, w.StartTime, w.EndTime, recurring, weekdaysJSON, enabled, now, now); err != nil {
			return fmt.Errorf("插入静默窗口失败: %w", err)
		}
	}

	return tx.Commit()
}

// ParseWeekdaysJSON 将数据库中的 JSON 字符串解析为星期切片。
//
// 供 sqlx 扫描辅助使用（weekdays 列以 TEXT 存储 JSON 数组）。
func ParseWeekdaysJSON(s string) []int {
	if s == "" || s == "null" {
		return nil
	}
	var out []int
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}
