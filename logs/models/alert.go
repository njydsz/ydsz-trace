// Package models 子文件：告警规则与投递记录。
//
// 告警规则按 item + 关键词/正则 + 级别匹配日志数，
// 命中数 >= threshold 且超出间隔窗口时触发 webhook 推送。
// 每次投递无论成功失败都写一条 t_alert_event，便于排查。
package models

import (
	"log"
	"time"
)

// AlertStatusEnabled / AlertStatusDisabled 规则启用状态。
const (
	AlertStatusEnabled  = 1
	AlertStatusDisabled = 0
)

// AlertRule 告警规则（表 t_alert_rule）。ItemName 由 join t_item 回填，不写库。
type AlertRule struct {
	ID          int64  `json:"id" db:"id"`
	Name        string `json:"name" db:"name"`
	ItemID      int64  `json:"itemId" db:"item_id"`
	ItemName    string `json:"itemName" db:"-"`
	ClientID    int64  `json:"clientId" db:"client_id"`
	KeyWord     string `json:"keyWord" db:"key_word"`
	Regex       bool   `json:"regex" db:"regex"`
	Level       string `json:"level" db:"level"`
	Threshold   int    `json:"threshold" db:"threshold"`
	IntervalSec int    `json:"intervalSec" db:"interval_sec"`
	WebhookURL  string `json:"webhookUrl" db:"webhook_url"`
	Enabled     int    `json:"enabled" db:"enabled"`
	LastFired   string `json:"lastFired" db:"last_fired"`
	CreatedBy   string `json:"createdBy" db:"created_by"`
	CreatedTime string `json:"createdTime" db:"created_time"`
	UpdatedTime string `json:"updatedTime" db:"updated_time"`
}

// AlertEvent 告警投递记录（表 t_alert_event）。
type AlertEvent struct {
	ID         int64  `json:"id" db:"id"`
	RuleID     int64  `json:"ruleId" db:"rule_id"`
	RuleName   string `json:"ruleName" db:"rule_name"`
	WebhookURL string `json:"webhookUrl" db:"webhook_url"`
	Status     string `json:"status" db:"status"`
	HTTPCode   int    `json:"httpCode" db:"http_code"`
	MatchCount int    `json:"matchCount" db:"match_count"`
	SampleText string `json:"sampleText" db:"sample_text"`
	ErrorMsg   string `json:"errorMsg" db:"error_msg"`
	FiredTime  string `json:"firedTime" db:"fired_time"`
}

func alertBoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ============ AlertRule CRUD ============

// InsertAlertRule 创建一条告警规则。
func InsertAlertRule(r *AlertRule) (int64, error) {
	now := time.Now().Format("2006-01-02 15:04:05")
	if r.CreatedTime == "" {
		r.CreatedTime = now
	}
	r.UpdatedTime = now
	if r.Threshold <= 0 {
		r.Threshold = 1
	}
	if r.IntervalSec <= 0 {
		r.IntervalSec = 300
	}
	res, err := DB.Exec(`INSERT INTO t_alert_rule
		(name, item_id, client_id, key_word, regex, level, threshold,
		 interval_sec, webhook_url, enabled, created_by, created_time, updated_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Name, r.ItemID, r.ClientID, r.KeyWord, alertBoolToInt(r.Regex), r.Level,
		r.Threshold, r.IntervalSec, r.WebhookURL, r.Enabled,
		r.CreatedBy, r.CreatedTime, r.UpdatedTime)
	if err != nil {
		log.Printf("insert alert rule err: %v", err)
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateAlertRule 按 id 更新规则可编辑字段。
func UpdateAlertRule(r *AlertRule) error {
	r.UpdatedTime = time.Now().Format("2006-01-02 15:04:05")
	_, err := DB.Exec(`UPDATE t_alert_rule SET
		name = ?, client_id = ?, key_word = ?, regex = ?, level = ?,
		threshold = ?, interval_sec = ?, webhook_url = ?, enabled = ?, updated_time = ?
		WHERE id = ?`,
		r.Name, r.ClientID, r.KeyWord, alertBoolToInt(r.Regex), r.Level,
		r.Threshold, r.IntervalSec, r.WebhookURL, r.Enabled,
		r.UpdatedTime, r.ID)
	if err != nil {
		log.Printf("update alert rule err: %v", err)
	}
	return err
}

// UpdateAlertRuleFired 记录 rule 的触发时间（粗粒度去重用）。
func UpdateAlertRuleFired(id int64) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := DB.Exec(`UPDATE t_alert_rule SET last_fired = ?, updated_time = ? WHERE id = ?`,
		now, now, id)
	return err
}

// DeleteAlertRule 按 id 删除规则。
func DeleteAlertRule(id int64) (int64, error) {
	res, err := DB.Exec(`DELETE FROM t_alert_rule WHERE id = ?`, id)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// GetAlertRule 按 id 查询单个规则。
func GetAlertRule(id int64) (AlertRule, error) {
	var r AlertRule
	err := DB.Get(&r, `SELECT * FROM t_alert_rule WHERE id = ?`, id)
	return r, err
}

// QueryAlertRuleCount 返回 total count（供 quota 统计）。
func QueryAlertRuleCount(enabledOnly bool) int {
	var total int
	q := `SELECT COUNT(*) FROM t_alert_rule`
	if enabledOnly {
		q += ` WHERE enabled = 1`
	}
	if err := DB.Get(&total, q); err != nil {
		log.Printf("count alert rule err: %v", err)
	}
	return total
}

// QueryAlertRules 分页查询告警规则（按 id 倒序）。
// keyword 过滤 name 或 webhook_url。
func QueryAlertRules(pageNo, pageSize int, keyword string) Page {
	if pageNo <= 0 {
		pageNo = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	where := ""
	args := []interface{}{}
	if keyword != "" {
		where = "WHERE name LIKE ? OR webhook_url LIKE ?"
		like := "%" + keyword + "%"
		args = append(args, like, like)
	}
	var rules []AlertRule
	err := DB.Select(&rules,
		`SELECT * FROM t_alert_rule `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, pageSize, (pageNo-1)*pageSize)...)
	if err != nil {
		log.Printf("query alert rules err: %v", err)
		return Page{}
	}
	var total int
	err = DB.Get(&total, `SELECT COUNT(*) FROM t_alert_rule `+where, args...)
	if err != nil {
		log.Printf("count alert rules err: %v", err)
		return Page{}
	}
	return PageUtil(total, pageNo, pageSize, rules)
}

// QueryEnabledAlertRules 返回所有启用的规则（供后台扫描循环使用）。
func QueryEnabledAlertRules() ([]AlertRule, error) {
	var rules []AlertRule
	err := DB.Select(&rules, `SELECT * FROM t_alert_rule WHERE enabled = ?`, AlertStatusEnabled)
	if err != nil {
		log.Printf("query enabled alert rules err: %v", err)
	}
	return rules, err
}

// ============ AlertEvent CRUD ============

// InsertAlertEvent 记录一次投递结果。
func InsertAlertEvent(ruleID int64, ruleName, webhookURL, status string, httpCode, matchCount int, sampleText, errMsg string) (int64, error) {
	if sampleText == "" {
		sampleText = ""
	}
	const maxSample = 2000
	if len(sampleText) > maxSample {
		sampleText = sampleText[:maxSample]
	}
	res, err := DB.Exec(`INSERT INTO t_alert_event
		(rule_id, rule_name, webhook_url, status, http_code, match_count, sample_text, error_msg)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ruleID, ruleName, webhookURL, status, httpCode, matchCount, sampleText, errMsg)
	if err != nil {
		log.Printf("insert alert event err: %v", err)
		return 0, err
	}
	return res.LastInsertId()
}

// QueryAlertEvents 分页查询投递记录（按时间倒序）。
// ruleID <= 0 表示不过滤规则；statusFilter 为空表示不过滤状态。
func QueryAlertEvents(pageNo, pageSize int, ruleID int64, statusFilter string) Page {
	if pageNo <= 0 {
		pageNo = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	where := "WHERE 1=1"
	args := []interface{}{}
	if ruleID > 0 {
		where += " AND rule_id = ?"
		args = append(args, ruleID)
	}
	if statusFilter != "" {
		where += " AND status = ?"
		args = append(args, statusFilter)
	}
	var events []AlertEvent
	err := DB.Select(&events,
		`SELECT * FROM t_alert_event `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, pageSize, (pageNo-1)*pageSize)...)
	if err != nil {
		log.Printf("query alert events err: %v", err)
		return Page{}
	}
	var total int
	err = DB.Get(&total, `SELECT COUNT(*) FROM t_alert_event `+where, args...)
	if err != nil {
		log.Printf("count alert events err: %v", err)
		return Page{}
	}
	return PageUtil(total, pageNo, pageSize, events)
}

// DeleteAlertEvent 按 id 删除单条投递记录。
func DeleteAlertEvent(id int64) (int64, error) {
	res, err := DB.Exec(`DELETE FROM t_alert_event WHERE id = ?`, id)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// FillRuleItemName 给单条规则填充 item_name（使用独立查询，避免 sqlx.In 批量）。
func FillRuleItemName(r *AlertRule) {
	if r.ItemName != "" {
		return
	}
	if r.ItemID == 0 {
		return
	}
	var item TItem
	if err := DB.Get(&item, `SELECT * FROM t_item WHERE id = ?`, r.ItemID); err == nil {
		r.ItemName = item.ItemName
	}
}

// AlertRulesQuota 返回近期活跃规则数 / 近 24h 投递成功数 / 失败数，便于前端展示摘要。
func AlertRulesQuota() (enabled, firedToday, failedToday int) {
	_ = DB.Get(&enabled, `SELECT COUNT(*) FROM t_alert_rule WHERE enabled = ?`, AlertStatusEnabled)
	since := time.Now().Add(-24 * time.Hour).Format("2006-01-02 15:04:05")
	_ = DB.Get(&firedToday, `SELECT COUNT(*) FROM t_alert_event WHERE fired_time >= ?`, since)
	_ = DB.Get(&failedToday, `SELECT COUNT(*) FROM t_alert_event WHERE fired_time >= ? AND status = ?`, since, "fail")
	return enabled, firedToday, failedToday
}

