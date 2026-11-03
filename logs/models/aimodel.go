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
}

// modelCapabilityColumns 列名字段（参数化查询，防 SQL 注入）。
const modelCapabilityColumns = `id, name, provider, model_id, api_key, base_url, temperature,
max_tokens, enabled, capability, created_by, created_time, updated_time,
cost_per_1k, quota_daily, quota_used, quota_reset_at`

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
		 cost_per_1k, quota_daily, quota_used, quota_reset_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	result, err := DB.Exec(query,
		m.Name, m.Provider, m.ModelId, m.ApiKey, m.BaseUrl,
		m.Temperature, m.MaxTokens, m.Enabled, m.Capability,
		m.CreatedBy, m.CreatedTime, m.UpdatedTime,
		m.CostPer1k, m.QuotaDaily, m.QuotaUsed, m.QuotaResetAt)
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
		cost_per_1k = ?, quota_daily = ?, quota_used = ?, quota_reset_at = ?
		WHERE id = ?`
	_, err := DB.Exec(query,
		m.Name, m.Provider, m.ModelId, m.ApiKey, m.BaseUrl,
		m.Temperature, m.MaxTokens, m.Enabled, m.Capability,
		m.UpdatedTime,
		m.CostPer1k, m.QuotaDaily, m.QuotaUsed, m.QuotaResetAt,
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

// CheckQuota 检查模型的每日配额是否充足。
// 返回 true 表示可用，false 表示已超限。
func CheckQuota(modelID string, tokens int64) (bool, int64, error) {
	var quotaDaily, quotaUsed int64
	var quotaResetAt string
	query := `SELECT quota_daily, quota_used, quota_reset_at FROM t_model_capability WHERE model_id = ?`
	if err := DB.Get(&quotaDaily, query, modelID); err != nil {
		return true, 0, nil // 无配置时不限制
	}
	_ = DB.Get(&quotaDaily, `SELECT quota_daily FROM t_model_capability WHERE model_id = ?`, modelID)
	_ = DB.Get(&quotaUsed, `SELECT quota_used FROM t_model_capability WHERE model_id = ?`, modelID)
	_ = DB.Get(&quotaResetAt, `SELECT quota_reset_at FROM t_model_capability WHERE model_id = ?`, modelID)

	// 检查是否需要重置（跨天）
	if quotaResetAt != "" {
		resetTime, err := time.Parse("2006-01-02 15:04:05", quotaResetAt)
		if err == nil && time.Since(resetTime) > 24*time.Hour {
			// 重置配额
			_, _ = DB.Exec(`UPDATE t_model_capability SET quota_used = 0, quota_reset_at = ? WHERE model_id = ?`,
				time.Now().Format("2006-01-02 15:04:05"), modelID)
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

// GetCostStats 返回指定模型的成本统计。
func GetCostStats(modelID string) (map[string]interface{}, error) {
	var costPer1k float64
	var quotaDaily, quotaUsed int64
	var quotaResetAt string

	if err := DB.Get(&costPer1k, `SELECT cost_per_1k FROM t_model_capability WHERE model_id = ?`, modelID); err != nil {
		return nil, fmt.Errorf("查询模型成本配置失败: %w", err)
	}
	_ = DB.Get(&quotaDaily, `SELECT quota_daily FROM t_model_capability WHERE model_id = ?`, modelID)
	_ = DB.Get(&quotaUsed, `SELECT quota_used FROM t_model_capability WHERE model_id = ?`, modelID)
	_ = DB.Get(&quotaResetAt, `SELECT quota_reset_at FROM t_model_capability WHERE model_id = ?`, modelID)

	todayCost := float64(quotaUsed) / 1000.0 * costPer1k
	return map[string]interface{}{
		"modelId":     modelID,
		"costPer1k":    costPer1k,
		"quotaDaily":   quotaDaily,
		"quotaUsed":    quotaUsed,
		"quotaRemaining": quotaDaily - quotaUsed,
		"quotaResetAt": quotaResetAt,
		"todayCost":    fmt.Sprintf("%.4f", todayCost),
	}, nil
}
