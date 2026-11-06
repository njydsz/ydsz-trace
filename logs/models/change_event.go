// Package models 子文件：变更事件持久化（V2.3-10，FR-905）。
//
// 背景：
//   排障时「最近有没有发布/配置变更」往往是最强的根因线索。
//   PRD FR-905 要求把变更事件作为强证据加权。
//
// 数据来源：
//   - 手动录入（运维通过接口登记）
//   - 外部系统推送（CI/CD、配置中心通过 HTTP 推送）
//
// 设计：
//   - 变更事件独立于日志，不参与模板聚类
//   - 按时间窗检索，供 Agent 与事故简报使用
//   - 影响面字段记录变更涉及的服务，便于与异常服务匹配
package models

import (
	"context"
	"fmt"
	"time"
)

// ChangeEvent 变更事件（表 t_change_event）。
type TChangeEvent struct {
	Id          int64  `json:"id" db:"id"`
	EventId     string `json:"eventId" db:"event_id"`     // 幂等键（外部系统 ID）
	Title       string `json:"title" db:"title"`
	ChangeType  string `json:"changeType" db:"change_type"` // deploy / config / db / network / other
	Service     string `json:"service" db:"service"`
	Environment string `json:"environment" db:"environment"`
	Operator    string `json:"operator" db:"operator"`
	OccurredAt  string `json:"occurredAt" db:"occurred_at"`
	Source      string `json:"source" db:"source"` // manual / cicd / config-center
	Detail      string `json:"detail" db:"detail"`
	CreatedTime string `json:"createdTime" db:"created_time"`
}

// SaveChangeEvent 写入或更新变更事件（按 event_id 幂等）。
func SaveChangeEvent(ev *TChangeEvent) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if ev.EventId == "" {
		ev.EventId = generateEventID(ev.Title, ev.OccurredAt)
	}
	if ev.CreatedTime == "" {
		ev.CreatedTime = time.Now().Format("2006-01-02 15:04:05")
	}

	query := `INSERT INTO t_change_event
		(event_id, title, change_type, service, environment, operator, occurred_at, source, detail, created_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(event_id) DO UPDATE SET
			title = excluded.title,
			change_type = excluded.change_type,
			service = excluded.service,
			environment = excluded.environment,
			operator = excluded.operator,
			detail = excluded.detail`
	res, err := DB.ExecContext(ctx, query,
		ev.EventId, ev.Title, ev.ChangeType, ev.Service, ev.Environment,
		ev.Operator, ev.OccurredAt, ev.Source, ev.Detail, ev.CreatedTime)
	if err != nil {
		return 0, fmt.Errorf("保存变更事件失败: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// generateEventID 为手动录入的事件生成稳定 ID。
func generateEventID(title, occurredAt string) string {
	sum := sha1Sum(title + "|" + occurredAt)
	return fmt.Sprintf("evt_%s", sum[:16])
}

// QueryChangeEvents 按时间窗查询变更事件。
//
// service 非空时按服务过滤；changeType 非空时按类型过滤。
func QueryChangeEvents(from, to, service, changeType string, limit int) ([]TChangeEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	// DB 未初始化（测试环境或启动早期）时返回空结果，避免 panic
	if DB == nil {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := DB.QueryContext(ctx,
		`SELECT id, event_id, title, change_type, service, environment,
		        operator, occurred_at, source, detail, created_time
		 FROM t_change_event
		 WHERE occurred_at BETWEEN ? AND ?
		   AND (? = '' OR service = ?)
		   AND (? = '' OR change_type = ?)
		 ORDER BY occurred_at DESC LIMIT ?`,
		from, to, service, service, changeType, changeType, limit)
	if err != nil {
		return nil, fmt.Errorf("查询变更事件失败: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			// 关闭失败仅记录，不影响结果返回
			_ = cerr
		}
	}()

	out := make([]TChangeEvent, 0, 8)
	for rows.Next() {
		var ev TChangeEvent
		if err := rows.Scan(&ev.Id, &ev.EventId, &ev.Title, &ev.ChangeType,
			&ev.Service, &ev.Environment, &ev.Operator, &ev.OccurredAt,
			&ev.Source, &ev.Detail, &ev.CreatedTime); err != nil {
			return nil, fmt.Errorf("扫描变更事件失败: %w", err)
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// GetChangeEventsAround 查询指定时间点附近的变更事件（排障核心查询）。
//
// windowMinutes 表示向前回溯的分钟数。例如异常发生在 T，
// 查询 [T-30min, T+5min] 的变更事件，命中即为强根因线索。
func GetChangeEventsAround(t time.Time, windowMinutes int) ([]TChangeEvent, error) {
	if windowMinutes <= 0 {
		windowMinutes = 30
	}
	from := t.Add(-time.Duration(windowMinutes) * time.Minute).Format("2006-01-02 15:04:05")
	to := t.Add(5 * time.Minute).Format("2006-01-02 15:04:05")
	return QueryChangeEvents(from, to, "", "", 20)
}

// ListChangeEvents 分页列出变更事件（管理界面用）。
func ListChangeEvents(pageNo, pageSize int) ([]TChangeEvent, int, error) {
	if pageNo <= 0 {
		pageNo = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	offset := (pageNo - 1) * pageSize

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var total int
	if err := DB.GetContext(ctx, &total, "SELECT COUNT(*) FROM t_change_event"); err != nil {
		return nil, 0, fmt.Errorf("变更事件总数查询失败: %w", err)
	}

	rows, err := DB.QueryContext(ctx,
		`SELECT id, event_id, title, change_type, service, environment,
		        operator, occurred_at, source, detail, created_time
		 FROM t_change_event ORDER BY occurred_at DESC LIMIT ? OFFSET ?`, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("变更事件列表查询失败: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	out := make([]TChangeEvent, 0, 8)
	for rows.Next() {
		var ev TChangeEvent
		if err := rows.Scan(&ev.Id, &ev.EventId, &ev.Title, &ev.ChangeType,
			&ev.Service, &ev.Environment, &ev.Operator, &ev.OccurredAt,
			&ev.Source, &ev.Detail, &ev.CreatedTime); err != nil {
			return nil, 0, fmt.Errorf("扫描变更事件失败: %w", err)
		}
		out = append(out, ev)
	}
	return out, total, rows.Err()
}

// DeleteChangeEvent 删除变更事件。
func DeleteChangeEvent(id int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := DB.ExecContext(ctx, `DELETE FROM t_change_event WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除变更事件失败: %w", err)
	}
	return nil
}
