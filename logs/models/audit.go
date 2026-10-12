// Package models 子文件：审计日志持久化。
//
// 记录所有敏感操作（搜索、导出、登录、配置变更、删除）到 t_audit_log，
// 满足合规审计要求：谁在什么时间通过什么 IP 做了什么操作，结果如何。
package models

import (
	"encoding/json"
	"log"
)

// TAuditLog 审计日志实体（表 t_audit_log）。
type TAuditLog struct {
	ID          int64  `json:"id" db:"id"`
	Username    string `json:"username" db:"username"`
	Action      string `json:"action" db:"action"`
	Resource    string `json:"resource" db:"resource"`
	Detail      string `json:"detail" db:"detail"` // JSON 字符串
	ClientIP    string `json:"clientIp" db:"client_ip"`
	Result      string `json:"result" db:"result"`
	CreatedTime string `json:"createdTime" db:"created_time"`
}

// AppendAuditLog 追加一条审计日志记录。
//
// detailMap 会被序列化为 JSON 字符串存储。
// 该函数仅在 RecordAudit 包级辅助函数内调用，写入失败仅打日志不返回错误。
func AppendAuditLog(username, action, resource string, detail map[string]interface{}, clientIP, result string) error {
	detailJSON := "{}"
	if len(detail) > 0 {
		if b, err := json.Marshal(detail); err == nil {
			detailJSON = string(b)
		} else {
			log.Printf("marshal audit detail failed: %v", err)
		}
	}

	_, err := DB.Exec(`INSERT INTO t_audit_log (username, action, resource, detail, client_ip, result)
		VALUES (?, ?, ?, ?, ?, ?)`,
		username, action, resource, detailJSON, clientIP, result)
	if err != nil {
		log.Printf("insert audit log err: %v", err)
	}
	return err
}

// QueryAuditLogs 分页查询审计日志列表（按 id 倒序）。
//
// username / action 为空字符串表示不过滤该字段。
func QueryAuditLogs(pageNo, pageSize int, username, action string) ([]TAuditLog, int, error) {
	if pageNo <= 0 {
		pageNo = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}

	where := ""
	args := []interface{}{}
	if username != "" {
		where += " WHERE username = ?"
		args = append(args, username)
	}
	if action != "" {
		if where == "" {
			where += " WHERE action = ?"
		} else {
			where += " AND action = ?"
		}
		args = append(args, action)
	}

	var logs []TAuditLog
	query := `SELECT * FROM t_audit_log ` + where + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	err := DB.Select(&logs, query, append(args, pageSize, (pageNo-1)*pageSize)...)
	if err != nil {
		log.Printf("query audit logs err: %v", err)
		return nil, 0, err
	}

	var total int
	countQuery := `SELECT COUNT(*) FROM t_audit_log ` + where
	err = DB.Get(&total, countQuery, args...)
	if err != nil {
		log.Printf("count audit logs err: %v", err)
		return nil, 0, err
	}

	return logs, total, nil
}

// AuditStatByAction 按 action 分组的统计结果。
type AuditStatByAction struct {
	Action string `json:"action" db:"action"`
	Count  int    `json:"count" db:"count"`
}

// AuditStatByDate 按日期分组的统计结果。
type AuditStatByDate struct {
	Date  string `json:"date" db:"date"`
	Count int    `json:"count" db:"count"`
}

// QueryAuditStats 返回审计统计：按 action 分组计数 + 近 7 天每日操作量。
func QueryAuditStats() (byAction []AuditStatByAction, byDate []AuditStatByDate, err error) {
	// 按 action 分组
	byAction = []AuditStatByAction{}
	err = DB.Select(&byAction, `SELECT action, COUNT(*) as count FROM t_audit_log GROUP BY action ORDER BY count DESC`)
	if err != nil {
		log.Printf("query audit stats by action err: %v", err)
		return nil, nil, err
	}

	// 近 7 天每日操作量
	byDate = []AuditStatByDate{}
	err = DB.Select(&byDate,
		`SELECT substr(created_time, 1, 10) AS date, COUNT(*) AS count
		 FROM t_audit_log
		 WHERE created_time >= datetime('now', 'localtime', '-7 days')
		 GROUP BY substr(created_time, 1, 10)
		 ORDER BY date ASC`)
	if err != nil {
		log.Printf("query audit stats by date err: %v", err)
		return nil, nil, err
	}

	return byAction, byDate, nil
}
