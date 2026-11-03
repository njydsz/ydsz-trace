// Package models 子文件：排障案例库持久化（t_case_library 表）。
//
// 案例库用于保存「已解决」的排障结论，支持相似症状检索和方案复用。
// 来源：manual（手动录入）/ conversation（对话工作台）/ feedback（用户反馈确认）。
package models

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// CaseSource 案例来源枚举。
type CaseSource string

const (
	CaseSourceManual       CaseSource = "manual"
	CaseSourceConversation CaseSource = "conversation"
	CaseSourceFeedback     CaseSource = "feedback"
)

// CaseEntry 保存案例的输入参数。
type CaseEntry struct {
	Title     string
	Symptoms  []string
	RootCause string
	Solution  string
	TagList   string // 逗号分隔
	Source    CaseSource
	ConvID    string
	CreatedBy string
}

// SaveCase 写入或更新案例库记录。
// 相同 title + source 组合视为同一条记录，命中时做更新。
func SaveCase(entry CaseEntry) (int64, error) {
	now := time.Now().Format("2006-01-02 15:04:05")
	symptomsJSON, err := json.Marshal(entry.Symptoms)
	if err != nil {
		return 0, fmt.Errorf("序列化症状失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// case_id = title 的 sha1 前 8 字节（保证同 title 幂等）
	sum := sha1.Sum([]byte(entry.Title))
	caseID := fmt.Sprintf("case_%x", sum[:8])

	query := `INSERT INTO t_case_library
		(case_id, title, symptoms, root_cause, solution, tag_list, source, conv_id, hit_count, created_by, created_time, updated_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)
		ON CONFLICT(case_id) DO UPDATE SET
			root_cause = excluded.root_cause,
			solution = excluded.solution,
			tag_list = excluded.tag_list,
			updated_time = excluded.updated_time`
	res, err := DB.ExecContext(ctx, query,
		caseID, entry.Title, string(symptomsJSON), entry.RootCause, entry.Solution,
		entry.TagList, string(entry.Source), entry.ConvID, entry.CreatedBy, now, now)
	if err != nil {
		return 0, fmt.Errorf("SaveCase [%s] 失败: %w", caseID, err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// SearchCasesBySymptoms 基于症状关键词模糊匹配案例。
func SearchCasesBySymptoms(keyword string, limit int) ([]TCaseLibrary, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `SELECT id, case_id, title, symptoms, root_cause, solution, tag_list,
		source, conv_id, hit_count, created_by, created_time, updated_time
		FROM t_case_library
		WHERE title LIKE ? OR symptoms LIKE ? OR solution LIKE ?
		ORDER BY hit_count DESC, updated_time DESC LIMIT ?`
	pat := "%" + keyword + "%"
	rows, err := DB.QueryContext(ctx, query, pat, pat, pat, limit)
	if err != nil {
		return nil, fmt.Errorf("SearchCasesBySymptoms 失败: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			log.Printf("[case] rows.Close 失败: %v", cerr)
		}
	}()

	cases := make([]TCaseLibrary, 0, 8)
	for rows.Next() {
		var c TCaseLibrary
		if err := rows.Scan(&c.Id, &c.CaseId, &c.Title, &c.Symptoms,
			&c.RootCause, &c.Solution, &c.TagList, &c.Source, &c.ConvId,
			&c.HitCount, &c.CreatedBy, &c.CreatedTime, &c.UpdatedTime); err != nil {
			return nil, fmt.Errorf("扫描 case_library 行失败: %w", err)
		}
		cases = append(cases, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 case_library rows 失败: %w", err)
	}
	return cases, nil
}

// IncrementCaseHit 增加命中次数。
func IncrementCaseHit(caseID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := DB.ExecContext(ctx, `UPDATE t_case_library SET hit_count = hit_count + 1 WHERE case_id = ?`, caseID)
	if err != nil {
		return fmt.Errorf("IncrementCaseHit [%s] 失败: %w", caseID, err)
	}
	return nil
}

// ListCases 分页获取案例列表。
func ListCases(limit, offset int) ([]TCaseLibrary, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var total int
	if err := DB.GetContext(ctx, &total, "SELECT COUNT(*) FROM t_case_library"); err != nil {
		return nil, 0, fmt.Errorf("case 总数查询失败: %w", err)
	}

	rows, err := DB.QueryContext(ctx,
		`SELECT id, case_id, title, symptoms, root_cause, solution, tag_list,
		source, conv_id, hit_count, created_by, created_time, updated_time
		FROM t_case_library ORDER BY updated_time DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("ListCases 失败: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			log.Printf("[case] rows.Close 失败: %v", cerr)
		}
	}()

	cases := make([]TCaseLibrary, 0, 8)
	for rows.Next() {
		var c TCaseLibrary
		if err := rows.Scan(&c.Id, &c.CaseId, &c.Title, &c.Symptoms,
			&c.RootCause, &c.Solution, &c.TagList, &c.Source, &c.ConvId,
			&c.HitCount, &c.CreatedBy, &c.CreatedTime, &c.UpdatedTime); err != nil {
			return nil, 0, fmt.Errorf("扫描 case_library 行失败: %w", err)
		}
		cases = append(cases, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历 case_library rows 失败: %w", err)
	}
	return cases, total, nil
}
