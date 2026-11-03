// Package embedding 子文件：t_embedding 表向量存储（V2.1）。
//
// 查询 logs 服务端的 t_embedding 表（由写入管线填充），
// 与 pkg/embedding/sql_store.go 的 SQLStore（查询 embedding_vec 表）互补。
package embedding

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"time"
)

// TStore 查询 t_embedding 表的向量存储。
//
// t_embedding 表结构：
//
//	target_type, target_id, model_version, vector_dim, vector_blob, created_time
//
// vector_blob 格式：float32 → little-endian 4 字节 × dim。
type TStore struct {
	db *sql.DB
}

// NewTStore 创建 t_embedding 表查询存储。
func NewTStore(db *sql.DB) (*TStore, error) {
	if db == nil {
		return nil, fmt.Errorf("embedding: sql.DB 不可为 nil")
	}
	return &TStore{db: db}, nil
}

// SearchT 在 t_embedding 表中检索 topK 相似向量。
//
// 参数：
//   - ctx     上下文
//   - query   查询向量
//   - topK    返回条数
//   - modelVersion 模型版本过滤（空则不过滤）
//   - targetType   目标类型过滤（空则不过滤）
func (s *TStore) SearchT(ctx context.Context, query []float32, topK int, modelVersion, targetType string) ([]SearchResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 构造查询条件
	querySQL := `SELECT target_id, vector_blob, vector_dim, model_version, target_type
		FROM t_embedding WHERE vector_dim = ?`
	args := []interface{}{len(query)}
	if modelVersion != "" {
		querySQL += ` AND model_version = ?`
		args = append(args, modelVersion)
	}
	if targetType != "" {
		querySQL += ` AND target_type = ?`
		args = append(args, targetType)
	}

	rows, err := s.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("embedding: 查询 t_embedding 失败: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			log.Printf("[embedding] 关闭 rows 失败: %v", cerr)
		}
	}()

	var results []SearchResult
	for rows.Next() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		var targetID string
		var blob []byte
		var dim int
		var mv string
		var tt string
		if err := rows.Scan(&targetID, &blob, &dim, &mv, &tt); err != nil {
			return nil, fmt.Errorf("embedding: 扫描 t_embedding 行失败: %w", err)
		}

		vec, err := blobToVector(blob, dim)
		if err != nil {
			log.Printf("[embedding] 向量反序列化失败 target_id=%s: %v", targetID, err)
			continue
		}

		score := CosineSimilarity(query, vec)
		results = append(results, SearchResult{
			Key:   targetID,
			Score: score,
			Metadata: map[string]string{
				"modelVersion": mv,
				"targetType":   tt,
				"dim":          fmt.Sprintf("%d", dim),
			},
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("embedding: 遍历 t_embedding 结果集失败: %w", err)
	}

	return topKResults(results, topK), nil
}

// CountT 返回 t_embedding 表中的向量总数。
func (s *TStore) CountT(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_embedding`).Scan(&count); err != nil {
		return 0, fmt.Errorf("embedding: 统计 t_embedding 失败: %w", err)
	}
	return count, nil
}

// EnsureUnusedImport 保证 time 包被使用。
var _ = time.Now
var _ = math.MaxFloat32
var _ = binary.LittleEndian
