// Package models 子文件：模型能力配置的 ORM 映射与数据库操作。
//
// 安全说明：
//   - api_key 写入时明文存储（生产环境建议加密），读取回显时脱敏处理
//   - 删除为硬删除，删除后关联的 AI 功能将不可用
package models

import (
	"fmt"
	"strings"
	"time"
)

// LogModelCapability 模型能力配置实体（表 t_model_capability）。
//
// capability 字段取值：
//   - chat：对话/推理能力（LLM 聊天补全）
//   - embedding：文本嵌入能力（向量化检索）
//   - rerank：重排序能力（搜索结果精排）
type LogModelCapability struct {
	Id           int64   `json:"id" db:"id"`
	Name         string  `json:"name" db:"name"`
	Provider     string  `json:"provider" db:"provider"`
	ModelId      string  `json:"modelId" db:"model_id"`
	ApiKey       string  `json:"apiKey" db:"api_key"`
	BaseUrl      string  `json:"baseUrl" db:"base_url"`
	Temperature  float64 `json:"temperature" db:"temperature"`
	MaxTokens    int     `json:"maxTokens" db:"max_tokens"`
	Enabled      int     `json:"enabled" db:"enabled"`
	Capability   string  `json:"capability" db:"capability"`
	CreatedBy    string  `json:"createdBy" db:"created_by"`
	CreatedTime  string  `json:"createdTime" db:"created_time"`
	UpdatedTime  string  `json:"updatedTime" db:"updated_time"`
	// 成本配额（V2.1 FR-1305）
	CostPer1k    float64 `json:"costPer1k" db:"cost_per_1k"`   // 每 1k tokens 成本（元）
	QuotaDaily   int64   `json:"quotaDaily" db:"quota_daily"`   // 每日配额（tokens）
	QuotaUsed    int64   `json:"quotaUsed" db:"quota_used"`     // 今日已用（tokens）
	QuotaResetAt string  `json:"quotaResetAt" db:"quota_reset_at"` // 配额重置时间
	// 配额告警阈值（0-100，默认 80）。达到该使用率时触发告警。
	QuotaAlertThreshold int `json:"quotaAlertThreshold" db:"quota_alert_threshold"`
}

// modelCapabilityColumns 列名字段（参数化查询，防 SQL 注入）。
const modelCapabilityColumns = `id, name, provider, model_id, api_key, base_url, temperature,
max_tokens, enabled, capability, created_by, created_time, updated_time,
cost_per_1k, quota_daily, quota_used, quota_reset_at, quota_alert_threshold`

// DefaultEmbeddingModelName 默认嵌入模型名（L2 层未激活时的展示值）。
const DefaultEmbeddingModelName = "local-drain-3072"

// GetDefaultEmbeddingModelName 返回 L2 层默认嵌入模型名。
//
// 优先返回已启用的 embedding 能力配置的 model_id；未配置时返回 DefaultEmbeddingModelName。
func GetDefaultEmbeddingModelName() string {
	var modelID string
	query := fmt.Sprintf(
		"SELECT model_id FROM t_model_capability WHERE capability = 'embedding' AND enabled = 1 LIMIT 1",
	)
	if err := DB.Get(&modelID, query); err == nil && modelID != "" {
		return modelID
	}
	return DefaultEmbeddingModelName
}

// AddModelCapability 新增模型能力配置。
func AddModelCapability(m *LogModelCapability) (int64, error) {
	now := time.Now().Format("2006-01-02 15:04:05")
	m.CreatedTime = now
	m.UpdatedTime = now
	if m.Capability == "" {
		m.Capability = "chat"
	}
	query := `INSERT INTO t_model_capability
		(name, provider, model_id, api_key, base_url, temperature, max_tokens, enabled, capability, created_by, created_time, updated_time,
		 cost_per_1k, quota_daily, quota_used, quota_reset_at, quota_alert_threshold)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	result, err := DB.Exec(query,
		m.Name, m.Provider, m.ModelId, m.ApiKey, m.BaseUrl,
		m.Temperature, m.MaxTokens, m.Enabled, m.Capability,
		m.CreatedBy, m.CreatedTime, m.UpdatedTime,
		m.CostPer1k, m.QuotaDaily, m.QuotaUsed, m.QuotaResetAt, m.QuotaAlertThreshold)
	if err != nil {
		return 0, fmt.Errorf("新增模型能力配置失败: %w", err)
	}
	return result.LastInsertId()
}

// GetModelCapabilityByID 根据 ID 查询模型能力配置。
func GetModelCapabilityByID(id int64) (*LogModelCapability, error) {
	var m LogModelCapability
	query := fmt.Sprintf("SELECT %s FROM t_model_capability WHERE id = ?", modelCapabilityColumns)
	if err := DB.Get(&m, query, id); err != nil {
		return nil, fmt.Errorf("查询模型能力配置失败: %w", err)
	}
	return &m, nil
}

// ListModelCapabilities 查询模型能力配置列表（支持按能力类型和关键词过滤）。
func ListModelCapabilities(keyword string, capability string, pageNum, pageSize int) (Page, error) {
	var conditions []string
	var args []interface{}

	if keyword != "" {
		conditions = append(conditions, "(name LIKE ? OR model_id LIKE ? OR provider LIKE ?)")
		kw := "%" + keyword + "%"
		args = append(args, kw, kw, kw)
	}
	if capability != "" {
		conditions = append(conditions, "capability = ?")
		args = append(args, capability)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM t_model_capability " + where
	if err := DB.Get(&total, countQuery, args...); err != nil {
		return Page{}, fmt.Errorf("统计模型能力配置失败: %w", err)
	}

	listQuery := fmt.Sprintf("SELECT %s FROM t_model_capability %s ORDER BY id DESC LIMIT ? OFFSET ?",
		modelCapabilityColumns, where)
	args = append(args, pageSize, (pageNum-1)*pageSize)
	var list []LogModelCapability
	if err := DB.Select(&list, listQuery, args...); err != nil {
		return Page{}, fmt.Errorf("查询模型能力配置列表失败: %w", err)
	}

	return PageUtil(total, pageNum, pageSize, list), nil
}

// UpdateModelCapability 更新模型能力配置（全字段更新）。
func UpdateModelCapability(m *LogModelCapability) error {
	m.UpdatedTime = time.Now().Format("2006-01-02 15:04:05")
	query := `UPDATE t_model_capability SET
		name = ?, provider = ?, model_id = ?, api_key = ?, base_url = ?,
		temperature = ?, max_tokens = ?, enabled = ?, capability = ?, updated_time = ?,
		cost_per_1k = ?, quota_daily = ?, quota_used = ?, quota_reset_at = ?, quota_alert_threshold = ?
		WHERE id = ?`
	_, err := DB.Exec(query,
		m.Name, m.Provider, m.ModelId, m.ApiKey, m.BaseUrl,
		m.Temperature, m.MaxTokens, m.Enabled, m.Capability,
		m.UpdatedTime,
		m.CostPer1k, m.QuotaDaily, m.QuotaUsed, m.QuotaResetAt, m.QuotaAlertThreshold,
		m.Id)
	if err != nil {
		return fmt.Errorf("更新模型能力配置失败: %w", err)
	}
	return nil
}

// DeleteModelCapability 删除模型能力配置（硬删除）。
func DeleteModelCapability(id int64) error {
	_, err := DB.Exec("DELETE FROM t_model_capability WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("删除模型能力配置失败: %w", err)
	}
	return nil
}

// ModelCapabilityExists 检查名称是否已存在（用于唯一性校验，排除自身）。
func ModelCapabilityExists(name string, excludeID int64) bool {
	var count int
	DB.Get(&count, "SELECT COUNT(*) FROM t_model_capability WHERE name = ? AND id != ?", name, excludeID)
	return count > 0
}

// GetEnabledModelCapabilities 查询所有已启用的模型能力配置（按能力类型过滤）。
func GetEnabledModelCapabilities(capability string) ([]LogModelCapability, error) {
	var list []LogModelCapability
	query := fmt.Sprintf("SELECT %s FROM t_model_capability WHERE enabled = 1 AND capability = ? ORDER BY id ASC",
		modelCapabilityColumns)
	if err := DB.Select(&list, query, capability); err != nil {
		return nil, fmt.Errorf("查询可用模型能力配置失败: %w", err)
	}
	return list, nil
}

// ---- 成本配额管理（V2.1 FR-1305）----

// quotaRow 配额查询行（具名结构体，供 sqlx 映射）。
type quotaRow struct {
	QuotaDaily   int64  `db:"quota_daily"`
	QuotaUsed    int64  `db:"quota_used"`
	QuotaResetAt string `db:"quota_reset_at"`
	Threshold    int    `db:"quota_alert_threshold"`
}

// CheckQuota 检查模型的每日配额是否充足。
// 返回 true 表示可用，false 表示已超限。
func CheckQuota(modelID string, tokens int64) (bool, int64, error) {
	var row quotaRow
	query := `SELECT quota_daily, quota_used, quota_reset_at, quota_alert_threshold FROM t_model_capability WHERE model_id = ?`
	if err := DB.Get(&row, query, modelID); err != nil {
		// 无配置时不限制（modelID 不存在或未配置配额）。
		return true, 0, nil
	}

	quotaDaily, quotaUsed, quotaResetAt := row.QuotaDaily, row.QuotaUsed, row.QuotaResetAt

	// 检查是否需要重置（跨天）。
	if quotaResetAt != "" {
		resetTime, err := time.Parse("2006-01-02 15:04:05", quotaResetAt)
		if err == nil && time.Since(resetTime) > 24*time.Hour {
			// 重置配额。
			if _, uerr := DB.Exec(`UPDATE t_model_capability SET quota_used = 0, quota_reset_at = ? WHERE model_id = ?`,
				time.Now().Format("2006-01-02 15:04:05"), modelID); uerr != nil {
				return true, 0, fmt.Errorf("重置配额失败: %w", uerr)
			}
			quotaUsed = 0
		}
	}

	if quotaDaily > 0 && quotaUsed+tokens > quotaDaily {
		return false, quotaDaily - quotaUsed, nil
	}
	return true, quotaDaily - quotaUsed, nil
}

// ConsumeQuota 消费模型的每日配额。
func ConsumeQuota(modelID string, tokens int64) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := DB.Exec(`UPDATE t_model_capability
		SET quota_used = quota_used + ?, quota_reset_at = ?
		WHERE model_id = ?`,
		tokens, now, modelID)
	if err != nil {
		return fmt.Errorf("消费配额失败: %w", err)
	}
	return nil
}

// costStatsRow 成本统计查询行。
type costStatsRow struct {
	CostPer1k    float64 `db:"cost_per_1k"`
	QuotaDaily   int64   `db:"quota_daily"`
	QuotaUsed    int64   `db:"quota_used"`
	QuotaResetAt string  `db:"quota_reset_at"`
}

// GetCostStats 返回指定模型的成本统计。
func GetCostStats(modelID string) (map[string]interface{}, error) {
	var row costStatsRow
	query := `SELECT cost_per_1k, quota_daily, quota_used, quota_reset_at FROM t_model_capability WHERE model_id = ?`
	if err := DB.Get(&row, query, modelID); err != nil {
		return nil, fmt.Errorf("查询模型成本配置失败: %w", err)
	}

	costPer1k, quotaDaily, quotaUsed, quotaResetAt := row.CostPer1k, row.QuotaDaily, row.QuotaUsed, row.QuotaResetAt
	todayCost := float64(quotaUsed) / 1000.0 * costPer1k
	return map[string]interface{}{
		"modelId":       modelID,
		"costPer1k":      costPer1k,
		"quotaDaily":     quotaDaily,
		"quotaUsed":      quotaUsed,
		"quotaRemaining": quotaDaily - quotaUsed,
		"quotaResetAt":   quotaResetAt,
		"todayCost":      fmt.Sprintf("%.4f", todayCost),
	}, nil
}

// ---- 成本告警（FR-1305）----

// DefaultQuotaAlertThreshold 默认配额告警阈值（80%）。
const DefaultQuotaAlertThreshold = 80

// CheckQuotaAndAlert 检查配额并在使用率达到阈值时触发告警（FR-1305）。
//
// 逻辑：
//  1. 查询模型配额配置
//  2. 计算当前使用率
//  3. 达到阈值且未告警过（24h 内）→ 写入 t_alert_event
//
// 返回 (是否可用, 剩余 tokens, 使用率, error)。
func CheckQuotaAndAlert(modelID string, tokens int64) (bool, int64, int, error) {
	var row quotaRow
	query := `SELECT quota_daily, quota_used, quota_reset_at, quota_alert_threshold FROM t_model_capability WHERE model_id = ?`
	if err := DB.Get(&row, query, modelID); err != nil {
		// 无配置时不限制。
		return true, 0, 0, nil
	}

	quotaDaily, quotaUsed, quotaResetAt, threshold := row.QuotaDaily, row.QuotaUsed, row.QuotaResetAt, row.Threshold

	// 跨天重置。
	if quotaResetAt != "" {
		resetTime, err := time.Parse("2006-01-02 15:04:05", quotaResetAt)
		if err == nil && time.Since(resetTime) > 24*time.Hour {
			if _, uerr := DB.Exec(`UPDATE t_model_capability SET quota_used = 0, quota_reset_at = ? WHERE model_id = ?`,
				time.Now().Format("2006-01-02 15:04:05"), modelID); uerr != nil {
				return true, 0, 0, fmt.Errorf("重置配额失败: %w", uerr)
			}
			quotaUsed = 0
		}
	}

	// 计算使用率。
	usagePercent := 0
	if quotaDaily > 0 {
		usagePercent = int(float64(quotaUsed) / float64(quotaDaily) * 100)
	}

	// 检查阈值：达到阈值且未告警过。
	if quotaDaily > 0 && usagePercent >= threshold {
		alreadyAlerted, err := hasQuotaAlertToday(modelID)
		if err != nil {
			return false, quotaDaily - quotaUsed, usagePercent, err
		}
		if !alreadyAlerted {
			if aerr := writeQuotaAlertEvent(modelID, quotaDaily, quotaUsed, usagePercent); aerr != nil {
				return false, quotaDaily - quotaUsed, usagePercent, aerr
			}
		}
	}

	if quotaDaily > 0 && quotaUsed+tokens > quotaDaily {
		return false, quotaDaily - quotaUsed, usagePercent, nil
	}
	return true, quotaDaily - quotaUsed, usagePercent, nil
}

// hasQuotaAlertToday 检查模型今日是否已触发过配额告警。
func hasQuotaAlertToday(modelID string) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM t_alert_event 
		WHERE rule_name = ? AND fired_time >= datetime('now', 'localtime', '-1 day')`
	if err := DB.Get(&count, query, "配额告警-"+modelID); err != nil {
		return false, fmt.Errorf("查询配额告警记录失败: %w", err)
	}
	return count > 0, nil
}

// writeQuotaAlertEvent 写入配额告警事件。
//
// t_alert_event.rule_id 有外键约束（引用 t_alert_rule.id），
// 因此先确保存在一个「系统配额告警」规则，再写入事件。
func writeQuotaAlertEvent(modelID string, quotaDaily, quotaUsed int64, usagePercent int) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	sample := fmt.Sprintf("模型 %s 配额使用率已达 %d%%（已用 %d / 上限 %d tokens）",
		modelID, usagePercent, quotaUsed, quotaDaily)

	// 确保系统规则存在。
	ruleID, err := ensureQuotaAlertRule()
	if err != nil {
		return fmt.Errorf("创建系统告警规则失败: %w", err)
	}

	query := `INSERT INTO t_alert_event 
		(rule_id, rule_name, webhook_url, status, http_code, match_count, sample_text, error_msg, fired_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = DB.Exec(query, ruleID, "配额告警-"+modelID, "", "ok", 0, 1, sample, "", now)
	if err != nil {
		return fmt.Errorf("写入配额告警事件失败: %w", err)
	}
	return nil
}

// ensureQuotaAlertRule 确保存在「系统配额告警」规则，返回其 ID。
//
// t_alert_rule 有 item_id 外键约束（引用 t_item.id），
// 因此先确保存在一个系统日志项，再创建规则。
func ensureQuotaAlertRule() (int64, error) {
	var id int64
	if err := DB.Get(&id, `SELECT id FROM t_alert_rule WHERE name = '系统配额告警' LIMIT 1`); err == nil && id > 0 {
		return id, nil
	}

	// 确保系统日志项存在。
	itemID, err := ensureSystemItem()
	if err != nil {
		return 0, fmt.Errorf("创建系统日志项失败: %w", err)
	}

	// 创建系统规则。
	now := time.Now().Format("2006-01-02 15:04:05")
	result, err := DB.Exec(`
		INSERT INTO t_alert_rule (name, item_id, client_id, key_word, regex, level, threshold, interval_sec, webhook_url, channel, enabled, created_time, updated_time)
		VALUES (?, ?, 0, '', 0, '', 1, 3600, '', 'webhook', 0, ?, ?)
	`, "系统配额告警", itemID, now, now)
	if err != nil {
		return 0, fmt.Errorf("插入系统告警规则失败: %w", err)
	}
	return result.LastInsertId()
}

// ensureSystemItem 确保存在系统日志项，返回其 ID。
//
// t_item 有 client_id 外键约束（引用 t_client.id），
// 因此先确保存在一个系统客户端，再创建系统日志项。
func ensureSystemItem() (int64, error) {
	var id int64
	if err := DB.Get(&id, `SELECT id FROM t_item WHERE item_name = '系统日志项' LIMIT 1`); err == nil && id > 0 {
		return id, nil
	}

	// 确保系统客户端存在。
	clientID, err := ensureSystemClient()
	if err != nil {
		return 0, fmt.Errorf("创建系统客户端失败: %w", err)
	}

	// 插入系统日志项。
	now := time.Now().Format("2006-01-02 15:04:05")
	result, err := DB.Exec(`
		INSERT INTO t_item (client_id, item_name, item_desc, log_path, log_prefix, log_suffix, status, created_by, created_time, updated_by, updated_time)
		VALUES (?, '系统日志项', '系统级日志项（用于配额告警等系统事件）', '', '', '', '1', 'system', ?, 'system', ?)
	`, clientID, now, now)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// ensureSystemClient 确保存在系统客户端，返回其 ID。
func ensureSystemClient() (int64, error) {
	var id int64
	if err := DB.Get(&id, `SELECT id FROM t_client WHERE info = '系统客户端' LIMIT 1`); err == nil && id > 0 {
		return id, nil
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	result, err := DB.Exec(`
		INSERT INTO t_client (ip, port, info, status, online, source_type, created_by, created_time, updated_by, updated_time)
		VALUES ('127.0.0.1', '0', '系统客户端', '1', '0', 'file', 'system', ?, 'system', ?)
	`, now, now)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// ConsumeQuotaAndAlert 消费配额并在使用率达到阈值时触发告警（FR-1305）。
//
// 先消费，再检查使用率；达到阈值时触发告警。
func ConsumeQuotaAndAlert(modelID string, tokens int64) error {
	if err := ConsumeQuota(modelID, tokens); err != nil {
		return err
	}
	if _, _, _, err := CheckQuotaAndAlert(modelID, 0); err != nil {
		return err
	}
	return nil
}

