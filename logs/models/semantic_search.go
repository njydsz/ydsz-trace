// Package models 子文件：语义检索支撑查询与写入（L2 embedding 层）。
//
// 提供模板与 embedding 元数据的查询与写入，供语义检索控制器及写入管线调用。
package models

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"time"
)

// templateColumns 查询列。
const templateColumns = `id, template_id, template_text, service, level, first_seen, last_seen, total_count, sample_line`

// GetTemplateByID 按 template_id 查询单条日志模板。
func GetTemplateByID(templateID string) (*TLogTemplate, error) {
	var t TLogTemplate
	query := fmt.Sprintf("SELECT %s FROM t_log_template WHERE template_id = ?", templateColumns)
	if err := DB.Get(&t, query, templateID); err != nil {
		return nil, fmt.Errorf("查询模板失败: %w", err)
	}
	return &t, nil
}

// SearchTemplates 按关键字模糊搜索日志模板（自动补全）。
//
// keyword 匹配 template_text 或 sample_line，返回最多 limit 条（默认 10，最大 20）。
// 结果按 total_count 降序排列（高频模板优先）。
func SearchTemplates(keyword string, limit int) ([]TLogTemplate, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	query := fmt.Sprintf(
		"SELECT %s FROM t_log_template WHERE template_text LIKE ? OR sample_line LIKE ? ORDER BY total_count DESC LIMIT ?",
		templateColumns,
	)
	pat := "%" + keyword + "%"
	rows, err := DB.Query(query, pat, pat, limit)
	if err != nil {
		return nil, fmt.Errorf("搜索模板失败: %w", err)
	}
	defer rows.Close()

	templates := make([]TLogTemplate, 0, limit)
	for rows.Next() {
		var t TLogTemplate
		if err := rows.Scan(&t.Id, &t.TemplateId, &t.TemplateText, &t.Service,
			&t.Level, &t.FirstSeen, &t.LastSeen, &t.TotalCount, &t.SampleLine); err != nil {
			return nil, fmt.Errorf("扫描模板行失败: %w", err)
		}
		templates = append(templates, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历模板 rows 失败: %w", err)
	}
	return templates, nil
}

// embeddingColumns 查询列。
const embeddingColumns = `id, target_type, target_id, model_version, vector_dim, created_time`

// GetEmbeddingByKey 按 target_id 查询单条 embedding 元数据（key 即 target_id）。
func GetEmbeddingByKey(targetID string) (*TEmbedding, error) {
	var e TEmbedding
	query := fmt.Sprintf("SELECT %s FROM t_embedding WHERE target_id = ?", embeddingColumns)
	if err := DB.Get(&e, query, targetID); err != nil {
		return nil, fmt.Errorf("查询 embedding 失败: %w", err)
	}
	return &e, nil
}

// UpsertTemplate 写入或更新一条日志模板（幂等）。
//
// 若 template_id 已存在则更新 last_seen / total_count / sample_line / updated_time；
// 否则插入新记录。用于 Drain 解析后的自动落库。
func UpsertTemplate(tmpl *TLogTemplate) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	if tmpl.CreatedTime == "" {
		tmpl.CreatedTime = now
	}
	tmpl.UpdatedTime = now

	_, err := DB.Exec(`INSERT INTO t_log_template
		(template_id, template_text, service, level, first_seen, last_seen, total_count, sample_line, created_time, updated_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(template_id) DO UPDATE SET
			last_seen = excluded.last_seen,
			total_count = t_log_template.total_count + excluded.total_count,
			sample_line = excluded.sample_line,
			updated_time = excluded.updated_time`,
		tmpl.TemplateId, tmpl.TemplateText, tmpl.Service, tmpl.Level,
		tmpl.FirstSeen, tmpl.LastSeen, tmpl.TotalCount, tmpl.SampleLine,
		tmpl.CreatedTime, tmpl.UpdatedTime)
	if err != nil {
		return fmt.Errorf("UpsertTemplate 失败: %w", err)
	}
	return nil
}

// BatchUpsertTemplates 批量写入或更新日志模板（事务内执行）。
func BatchUpsertTemplates(templates []*TLogTemplate) error {
	if len(templates) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("BatchUpsertTemplates: begin tx failed: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				log.Printf("[template] rollback failed: %v", rbErr)
			}
		}
	}()

	now := time.Now().Format("2006-01-02 15:04:05")
	for _, tmpl := range templates {
		if tmpl.CreatedTime == "" {
			tmpl.CreatedTime = now
		}
		tmpl.UpdatedTime = now
		_, err = tx.ExecContext(ctx, `INSERT INTO t_log_template
			(template_id, template_text, service, level, first_seen, last_seen, total_count, sample_line, created_time, updated_time)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(template_id) DO UPDATE SET
				last_seen = excluded.last_seen,
				total_count = t_log_template.total_count + excluded.total_count,
				sample_line = excluded.sample_line,
				updated_time = excluded.updated_time`,
			tmpl.TemplateId, tmpl.TemplateText, tmpl.Service, tmpl.Level,
			tmpl.FirstSeen, tmpl.LastSeen, tmpl.TotalCount, tmpl.SampleLine,
			tmpl.CreatedTime, tmpl.UpdatedTime)
		if err != nil {
			return fmt.Errorf("BatchUpsertTemplates: exec failed for %s: %w", tmpl.TemplateId, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("BatchUpsertTemplates: commit failed: %w", err)
	}
	return nil
}

// CountTemplates 返回模板总数。
func CountTemplates() (int, error) {
	var count int
	if err := DB.Get(&count, `SELECT COUNT(*) FROM t_log_template`); err != nil {
		return 0, fmt.Errorf("CountTemplates 失败: %w", err)
	}
	return count, nil
}

// StoreEmbedding 写入或更新一条向量索引记录（幂等）。
//
// 若 (target_type, target_id, model_version) 已存在则跳过（向量不变）；
// 否则插入新记录。vector 为 float32 切片，内部序列化为 BLOB。
func StoreEmbedding(targetType, targetID, modelVersion string, vector []float32) error {
	if len(vector) == 0 {
		return fmt.Errorf("StoreEmbedding: vector is empty")
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	blob := serializeVector(vector)

	_, err := DB.Exec(`INSERT INTO t_embedding
		(target_type, target_id, model_version, vector_dim, vector_blob, created_time)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(target_type, target_id, model_version) DO NOTHING`,
		targetType, targetID, modelVersion, len(vector), blob, now)
	if err != nil {
		return fmt.Errorf("StoreEmbedding 失败: %w", err)
	}
	return nil
}

// BatchStoreEmbeddings 批量写入向量索引记录（事务内执行）。
func BatchStoreEmbeddings(items []EmbeddingWriteItem) error {
	if len(items) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("BatchStoreEmbeddings: begin tx failed: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				log.Printf("[embedding] rollback failed: %v", rbErr)
			}
		}
	}()

	now := time.Now().Format("2006-01-02 15:04:05")
	for _, item := range items {
		blob := serializeVector(item.Vector)
		_, err = tx.ExecContext(ctx, `INSERT INTO t_embedding
			(target_type, target_id, model_version, vector_dim, vector_blob, created_time)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(target_type, target_id, model_version) DO NOTHING`,
			item.TargetType, item.TargetID, item.ModelVersion, len(item.Vector), blob, now)
		if err != nil {
			return fmt.Errorf("BatchStoreEmbeddings: exec failed for %s/%s: %w", item.TargetType, item.TargetID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("BatchStoreEmbeddings: commit failed: %w", err)
	}
	return nil
}

// EmbeddingWriteItem 批量写入向量时的单项结构。
type EmbeddingWriteItem struct {
	TargetType   string
	TargetID     string
	ModelVersion string
	Vector       []float32
}

// serializeVector 将 float32 切片序列化为 little-endian BLOB。
// 与 pkg/embedding.sql_store.go 的 vectorToBlob 格式一致。
func serializeVector(vec []float32) []byte {
	buf := make([]byte, 4*len(vec))
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// CountEmbeddings 返回向量索引总数。
func CountEmbeddings() (int, error) {
	var count int
	if err := DB.Get(&count, `SELECT COUNT(*) FROM t_embedding`); err != nil {
		return 0, fmt.Errorf("CountEmbeddings 失败: %w", err)
	}
	return count, nil
}
