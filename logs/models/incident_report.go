// Package models 子文件：事故简报持久化（V2.3-07）。
//
// 事故简报存储在 t_incident_report 表中，支持生成后落库与历史查询。
package models

import (
	"encoding/json"
	"fmt"
	"time"
)

// TIncidentReport 事故简报实体（表 t_incident_report）。
type TIncidentReport struct {
	Id         int64  `json:"id" db:"id"`
	Title      string `json:"title" db:"title"`
	Query      string `json:"query" db:"query"`
	StartedAt  string `json:"startedAt" db:"started_at"`
	Duration   string `json:"duration" db:"duration"`
	Severity   string `json:"severity" db:"severity"`
	Summary    string `json:"summary" db:"summary"`
	RootCause  string `json:"rootCause" db:"root_cause"`
	Impact     string `json:"impact" db:"impact"`
	Timeline   string `json:"timeline" db:"timeline"` // JSON array
	Evidence   string `json:"evidence" db:"evidence"` // JSON array
	Actions    string `json:"actions" db:"actions"`   // JSON array
	Markdown   string `json:"markdown" db:"markdown"`
	CreatedAt  string `json:"createdAt" db:"created_at"`
}

// incidentReportSchema 建表 DDL（幂等）。
const incidentReportSchema = `
CREATE TABLE IF NOT EXISTS t_incident_report (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    title       TEXT    NOT NULL,
    query       TEXT    DEFAULT '',
    started_at  TEXT    DEFAULT '',
    duration    TEXT    DEFAULT '',
    severity    TEXT    DEFAULT '',
    summary     TEXT    DEFAULT '',
    root_cause  TEXT    DEFAULT '',
    impact      TEXT    DEFAULT '',
    timeline    TEXT    DEFAULT '[]',
    evidence    TEXT    DEFAULT '[]',
    actions     TEXT    DEFAULT '[]',
    markdown    TEXT    DEFAULT '',
    created_at  TEXT    DEFAULT (datetime('now', 'localtime'))
);
CREATE INDEX IF NOT EXISTS idx_incident_report_created ON t_incident_report(created_at);
`

// CreateIncidentReportTable 创建事故简报表（幂等）。
func CreateIncidentReportTable() error {
	if DB == nil {
		return fmt.Errorf("database not initialized: DB is nil")
	}
	if _, err := DB.Exec(incidentReportSchema); err != nil {
		return fmt.Errorf("CreateIncidentReportTable failed: %w", err)
	}
	return nil
}

// SaveIncidentReport 保存事故简报，返回自增 ID。
//
// 参数 report 的 Timeline/Evidence/Actions 为 JSON 序列化后的字符串。
func SaveIncidentReport(report *TIncidentReport) (int64, error) {
	if report == nil {
		return 0, fmt.Errorf("report is nil")
	}
	if report.CreatedAt == "" {
		report.CreatedAt = time.Now().Format("2006-01-02 15:04:05")
	}

	res, err := DB.Exec(`INSERT INTO t_incident_report
		(title, query, started_at, duration, severity, summary, root_cause, impact,
		 timeline, evidence, actions, markdown, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		report.Title, report.Query, report.StartedAt, report.Duration, report.Severity,
		report.Summary, report.RootCause, report.Impact,
		report.Timeline, report.Evidence, report.Actions,
		report.Markdown, report.CreatedAt)
	if err != nil {
		return 0, fmt.Errorf("SaveIncidentReport failed: %w", err)
	}
	return res.LastInsertId()
}

// GetIncidentReport 按 ID 查询事故简报。
func GetIncidentReport(id int64) (*TIncidentReport, error) {
	var report TIncidentReport
	if err := DB.Get(&report,
		`SELECT id, title, query, started_at, duration, severity, summary, root_cause,
		        impact, timeline, evidence, actions, markdown, created_at
		 FROM t_incident_report WHERE id = ?`, id); err != nil {
		return nil, fmt.Errorf("GetIncidentReport failed: %w", err)
	}
	return &report, nil
}

// ListIncidentReports 分页查询事故简报（按时间倒序）。
func ListIncidentReports(pageNo, pageSize int) ([]TIncidentReport, error) {
	if pageNo <= 0 {
		pageNo = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	offset := (pageNo - 1) * pageSize

	var list []TIncidentReport
	if err := DB.Select(&list,
		`SELECT id, title, query, started_at, duration, severity, summary, root_cause,
		        impact, timeline, evidence, actions, markdown, created_at
		 FROM t_incident_report ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		pageSize, offset); err != nil {
		return nil, fmt.Errorf("ListIncidentReports failed: %w", err)
	}
	return list, nil
}

// DeleteIncidentReport 删除事故简报。
func DeleteIncidentReport(id int64) error {
	_, err := DB.Exec(`DELETE FROM t_incident_report WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("DeleteIncidentReport failed: %w", err)
	}
	return nil
}

// MarshalTimeline 将时间线条目序列化为 JSON 字符串。
func MarshalTimeline(items []IncidentTimelineItem) string {
	if len(items) == 0 {
		return "[]"
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// MarshalEvidence 将证据引用序列化为 JSON 字符串。
func MarshalEvidence(items []EvidenceRef) string {
	if len(items) == 0 {
		return "[]"
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// MarshalActions 将动作列表序列化为 JSON 字符串。
func MarshalActions(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// IncidentTimelineItem 时间线条目（与控制器层共享）。
type IncidentTimelineItem struct {
	Time        string `json:"time"`
	Description string `json:"description"`
}

// EvidenceRef 证据引用（与控制器层共享）。
type EvidenceRef struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Source  string `json:"source"`
}
