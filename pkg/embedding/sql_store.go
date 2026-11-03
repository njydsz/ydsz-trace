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

// DDL 建表语句。
const ddlCreateTable = `CREATE TABLE IF NOT EXISTS embedding_vec (
	key           TEXT PRIMARY KEY,
	target_type   TEXT,
	target_id     TEXT,
	dim           INTEGER,
	model_version TEXT,
	data          BLOB,
	created_at    TEXT
)`

// NewSQLStore 创建 SQLite 向量存储。
//
// db 须使用现代 SQLite 驱动（如 modernc.org/sqlite）并提前注册。
// dbPath 为独立数据库文件路径（非共享 logs 的 db）。
func NewSQLStore(db *sql.DB) (*SQLStore, error) {
	if db == nil {
		return nil, fmt.Errorf("embedding: sql.DB 不可为 nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := db.ExecContext(ctx, ddlCreateTable); err != nil {
		return nil, fmt.Errorf("embedding: 建表失败: %w", err)
	}

	return &SQLStore{db: db}, nil
}

// SQLStore SQLite 持久化向量存储。
//
// vector 序列化格式：float32 → little-endian 4 字节 × dim，存 BLOB。
type SQLStore struct {
	db *sql.DB
}

// Store 持久化向量及其元数据。
func (s *SQLStore) Store(ctx context.Context, key string, vector []float32, metadata map[string]string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	blob, err := vectorToBlob(vector)
	if err != nil {
		return err
	}

	targetType := ""
	targetID := ""
	modelVersion := DefaultLocalModel
	if metadata != nil {
		targetType = metadata["targetType"]
		targetID = metadata["targetID"]
		if v, ok := metadata["modelVersion"]; ok {
			modelVersion = v
		}
	}

	query := `INSERT OR REPLACE INTO embedding_vec
		(key, target_type, target_id, dim, model_version, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	createdAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, query,
		key, targetType, targetID, len(vector), modelVersion, blob, createdAt,
	); err != nil {
		return fmt.Errorf("embedding: 存入向量失败: %w", err)
	}

	return nil
}

// Search 检索 topK 相似向量。
//
// 检索策略：按 dim + model_version 预过滤候选集，内存 cosine similarity 排序。
func (s *SQLStore) Search(ctx context.Context, query []float32, topK int) ([]SearchResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	querySQL := `SELECT key, data, dim, model_version FROM embedding_vec WHERE dim = ? AND model_version = ?`

	rows, err := s.db.QueryContext(ctx, querySQL, len(query), DefaultLocalModel)
	if err != nil {
		return nil, fmt.Errorf("embedding: 查询向量失败: %w", err)
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

		var key string
		var blob []byte
		var dim int
		var modelVersion string
		if err := rows.Scan(&key, &blob, &dim, &modelVersion); err != nil {
			return nil, fmt.Errorf("embedding: 扫描行失败: %w", err)
		}

		vec, err := blobToVector(blob, dim)
		if err != nil {
			log.Printf("[embedding] 向量反序列化失败 key=%s: %v", key, err)
			continue
		}

		score := CosineSimilarity(query, vec)
		results = append(results, SearchResult{
			Key:   key,
			Score: score,
			Metadata: map[string]string{
				"modelVersion": modelVersion,
				"dim":          fmt.Sprintf("%d", dim),
			},
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("embedding: 遍历结果集失败: %w", err)
	}

	return topKResults(results, topK), nil
}

// vectorToBlob 将 float32 切片序列化为 little-endian BLOB。
func vectorToBlob(vec []float32) ([]byte, error) {
	buf := make([]byte, 4*len(vec))
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf, nil
}

// blobToVector 将 BLOB 反序列化为 float32 切片。
func blobToVector(blob []byte, dim int) ([]float32, error) {
	if len(blob) != 4*dim {
		return nil, fmt.Errorf("embedding: blob 长度不匹配: got %d, want %d", len(blob), 4*dim)
	}
	vec := make([]float32, dim)
	for i := 0; i < dim; i++ {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
	}
	return vec, nil
}
