// Package models 子文件：链路追踪节点持久化（t_trace_node 表）。
//
// 一次链路查询后，将各节点聚合为持久化记录，供案例库关联、历史查询、时间线分析。
package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// TraceNodeInput 写入链路节点的输入参数。
type TraceNodeInput struct {
	TraceID    string
	SpanID     string
	ParentID   string
	Service    string
	Operation  string
	StartTs    string
	DurationMs int64
	Status     string // "ok" / "error"
	Tags       map[string]string
}

// SaveTraceNodes 批量写入链路节点（幂等：UNIQUE(trace_id, span_id) ON CONFLICT REPLACE）。
//
// 参数 nodes 可能来自多个节点的并发聚合，函数内部做批次合并。
func SaveTraceNodes(nodes []TraceNodeInput) error {
	if len(nodes) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("SaveTraceNodes 开启事务失败: %w", err)
	}
	// 事务回滚保障
	committed := false
	defer func() {
		if !committed {
			if rerr := tx.Rollback(); rerr != nil && rerr != sql.ErrTxDone {
				log.Printf("[trace] 事务回滚失败: %v", rerr)
			}
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO t_trace_node
			(trace_id, span_id, parent_id, service, operation, start_ts, duration_ms, status, tags, created_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("SaveTraceNodes prepare 失败: %w", err)
	}
	defer func() {
		if cerr := stmt.Close(); cerr != nil {
			log.Printf("[trace] stmt.Close 失败: %v", cerr)
		}
	}()

	now := time.Now().Format("2006-01-02 15:04:05")
	for _, n := range nodes {
		status := n.Status
		if status == "" {
			status = "ok"
		}
		tagsJSON, _ := json.Marshal(n.Tags)
		if _, err := stmt.ExecContext(ctx,
			n.TraceID, n.SpanID, n.ParentID, n.Service, n.Operation,
			n.StartTs, n.DurationMs, status, string(tagsJSON), now); err != nil {
			return fmt.Errorf("SaveTraceNodes exec [%s/%s] 失败: %w", n.TraceID, n.SpanID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("SaveTraceNodes commit 失败: %w", err)
	}
	committed = true
	return nil
}

// QueryTraceNodes 按 trace_id 查询链路节点列表（按 start_ts 排序）。
func QueryTraceNodes(traceID string) ([]TTraceNode, error) {
	query := `SELECT id, trace_id, span_id, parent_id, service, operation,
		start_ts, duration_ms, status, tags, created_time
		FROM t_trace_node WHERE trace_id = ? ORDER BY start_ts ASC`
	rows, err := DB.QueryContext(context.Background(), query, traceID)
	if err != nil {
		return nil, fmt.Errorf("QueryTraceNodes 失败: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			log.Printf("[trace] rows.Close 失败: %v", cerr)
		}
	}()

	nodes := make([]TTraceNode, 0, 8)
	for rows.Next() {
		var n TTraceNode
		if err := rows.Scan(&n.Id, &n.TraceId, &n.SpanId, &n.ParentId, &n.Service,
			&n.Operation, &n.StartTs, &n.DurationMs, &n.Status, &n.Tags, &n.CreatedTime); err != nil {
			return nil, fmt.Errorf("扫描 trace_node 行失败: %w", err)
		}
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 trace_node rows 失败: %w", err)
	}
	return nodes, nil
}
