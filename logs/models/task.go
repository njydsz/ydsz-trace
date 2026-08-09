// Package models 子文件：检索任务持久化。
//
// 每次检索请求（/logs/query / /logs/search）都落库到 t_task，
// 前端可通过 task_no 轮询任务进度，避免大结果集长时间占用 HTTP 连接。
package models

import (
	"fmt"
	"log"
	"time"
)

const (
	TaskStatusPending = "pending"
	TaskStatusRunning = "running"
	TaskStatusSuccess = "success"
	TaskStatusFailed  = "failed"
	TaskStatusPurged  = "purged"
)

// TTask 检索任务记录（表 t_task）。
type TTask struct {
	ID           int64  `json:"id" db:"id"`
	TaskNo       string `json:"taskNo" db:"task_no"`
	UserName     string `json:"userName" db:"user_name"`
	ClientID     int64  `json:"clientId" db:"client_id"`
	ItemID       int64  `json:"itemId" db:"item_id"`
	ItemName     string `json:"itemName" db:"item_name"`
	LogDate      string `json:"logDate" db:"log_date"`
	KeyWord      string `json:"keyWord" db:"key_word"`
	Regex        bool   `json:"regex" db:"regex"`
	Level        string `json:"level" db:"level"`
	StartTime    string `json:"startTime" db:"start_time"`
	EndTime      string `json:"endTime" db:"end_time"`
	QueryExpr    string `json:"queryExpr" db:"query_expr"`
	LineCount    int64  `json:"lineCount" db:"line_count"`
	MaxLines     int    `json:"maxLines" db:"max_lines"`
	Status       string `json:"status" db:"status"`
	NodeTotal    int    `json:"nodeTotal" db:"node_total"`
	NodeDone     int    `json:"nodeDone" db:"node_done"`
	MatchCount   int64  `json:"matchCount" db:"match_count"`
	ErrorMsg     string `json:"errorMsg" db:"error_msg"`
	ZipPath      string `json:"zipPath" db:"zip_path"`
	CreatedTime  string `json:"createdTime" db:"created_time"`
	StartedTime  string `json:"startedTime" db:"started_time"`
	FinishedTime string `json:"finishedTime" db:"finished_time"`
}

// NewTaskNo 生成唯一 taskNo：T + 年月日时分秒 + 6位随机。
func NewTaskNo() string {
	return "T" + time.Now().Format("20060102150405") + fmt.Sprintf("%06d", time.Now().Nanosecond()%1000000)
}

// InsertTask 创建一条待执行的检索任务记录。
func InsertTask(t *TTask) (int64, error) {
	if t.TaskNo == "" {
		t.TaskNo = NewTaskNo()
	}
	if t.Status == "" {
		t.Status = TaskStatusPending
	}
	res, err := DB.Exec(`INSERT INTO t_task
		(task_no, user_name, client_id, item_id, item_name, log_date, key_word, regex,
			level, start_time, end_time, query_expr, line_count, max_lines, status,
			node_total, node_done, match_count, error_msg, zip_path, started_time, finished_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.TaskNo, t.UserName, t.ClientID, t.ItemID, t.ItemName, t.LogDate,
		t.KeyWord, boolToInt(t.Regex), t.Level, t.StartTime, t.EndTime, t.QueryExpr,
		t.LineCount, t.MaxLines, t.Status,
		t.NodeTotal, t.NodeDone, t.MatchCount, t.ErrorMsg, t.ZipPath,
		t.StartedTime, t.FinishedTime)
	if err != nil {
		log.Printf("insert task err: %v", err)
		return 0, err
	}
	return res.LastInsertId()
}

// GetTaskByNo 按 taskNo 查询单个任务。
func GetTaskByNo(taskNo string) (TTask, error) {
	var t TTask
	err := DB.Get(&t, `SELECT * FROM t_task WHERE task_no = ?`, taskNo)
	return t, err
}

// GetTaskByID 按 id 查询单个任务。
func GetTaskByID(id int64) (TTask, error) {
	var t TTask
	err := DB.Get(&t, `SELECT * FROM t_task WHERE id = ?`, id)
	return t, err
}

// UpdateTaskStatus 更新任务 progress / status。
func UpdateTaskStatus(taskNo, status string, nodeTotal, nodeDone int, matchCount int64, errMsg, zipPath string) error {
	_, err := DB.Exec(`UPDATE t_task SET
		status = ?, node_total = ?, node_done = ?, match_count = ?, error_msg = ?, zip_path = ?
		WHERE task_no = ?`,
		status, nodeTotal, nodeDone, matchCount, errMsg, zipPath, taskNo)
	return err
}

// TouchTaskStarted 标记任务开始执行。
func TouchTaskStarted(taskNo string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := DB.Exec(`UPDATE t_task SET status = ?, started_time = ? WHERE task_no = ?`,
		TaskStatusRunning, now, taskNo)
	return err
}

// TouchTaskFinished 标记任务完成。
func TouchTaskFinished(taskNo, status, errMsg, zipPath string, matchCount int64) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := DB.Exec(`UPDATE t_task SET status = ?, finished_time = ?, error_msg = ?, zip_path = ?, match_count = ? WHERE task_no = ?`,
		status, now, errMsg, zipPath, matchCount, taskNo)
	return err
}

// QueryTasks 分页查询任务列表（按创建时间倒序）。
func QueryTasks(pageNo, pageSize int, statusFilter string) Page {
	if pageNo <= 0 {
		pageNo = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	where := ""
	args := []interface{}{}
	if statusFilter != "" {
		where = "WHERE status = ?"
		args = append(args, statusFilter)
	}
	var tasks []TTask
	err := DB.Select(&tasks,
		`SELECT * FROM t_task `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, pageSize, (pageNo-1)*pageSize)...)
	if err != nil {
		log.Printf("query tasks err: %v", err)
		return Page{}
	}
	var total int
	err = DB.Get(&total, `SELECT COUNT(*) FROM t_task `+where, args...)
	if err != nil {
		log.Printf("count tasks err: %v", err)
		return Page{}
	}
	return PageUtil(total, pageNo, pageSize, tasks)
}

// DeleteTaskByNo 删除任务记录（用于管理/清理）。
func DeleteTaskByNo(taskNo string) int64 {
	res, err := DB.Exec(`DELETE FROM t_task WHERE task_no = ?`, taskNo)
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return n
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
