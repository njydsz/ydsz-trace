// Package storage — 列式段存储引擎（Segment Storage Engine）。
//
// 按小时分桶存储日志聚合段数据，每条记录包含统计元信息与压缩后的列式数据。
// 写入时压缩，读取时解压；支持生命周期管理（按日期清理）。
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"ydsz-trace/logs/models"
)

// ErrSegmentNotFound 哨兵错误：查询的段不存在。
var ErrSegmentNotFound = errors.New("segment not found")

// SegmentWriter 段写入器。
//
// 持有 *sqlx.DB 句柄，提供段写入与删除能力。
type SegmentWriter struct {
	db *sqlx.DB
}

// SegmentReader 段读取器。
//
// 持有 *sqlx.DB 句柄，提供段查询与范围读取能力。
type SegmentReader struct {
	db *sqlx.DB
}

// NewSegmentWriter 创建 SegmentWriter 实例。
//
// 传入的 db 必须已初始化（通常直接使用 models.DB 全局句柄）。
func NewSegmentWriter(db *sqlx.DB) *SegmentWriter {
	return &SegmentWriter{db: db}
}

// NewSegmentReader 创建 SegmentReader 实例。
//
// 传入的 db 必须已初始化（通常直接使用 models.DB 全局句柄）。
func NewSegmentReader(db *sqlx.DB) *SegmentReader {
	return &SegmentReader{db: db}
}

// segmentColumns 查询段表时的列列表（不含压缩列按需选择）。
const segmentColumns = `id, item_id, client_id, seg_date, seg_hour, template_id,
       line_count, error_count, warn_count, first_ts, last_ts, compressed, created_time`

// WriteSegment 写入或更新一条列式段记录。
//
// 对 compressed 字段执行压缩后存储。若 (item_id, client_id, seg_date, seg_hour) 已存在则更新，
// 利用表级 UNIQUE 约束 + INSERT OR REPLACE 语义实现幂等写入。
//
// ctx 用于传递取消信号，超时或取消时数据库操作会中断。
func (w *SegmentWriter) WriteSegment(ctx context.Context, seg *models.TLogSegment) error {
	if seg == nil {
		return errors.New("WriteSegment: seg is nil")
	}

	compressed, err := Compress(seg.Compressed)
	if err != nil {
		return fmt.Errorf("WriteSegment: compress failed: %w", err)
	}

	// INSERT OR REPLACE：利用 UNIQUE(item_id, client_id, seg_date, seg_hour) 实现幂等写入
	query := `INSERT OR REPLACE INTO t_log_segment
		(item_id, client_id, seg_date, seg_hour, template_id,
		 line_count, error_count, warn_count, first_ts, last_ts, compressed, created_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	createdTime := seg.CreatedTime
	if createdTime == "" {
		createdTime = time.Now().Format("2006-01-02 15:04:05")
	}

	_, err = w.db.ExecContext(ctx, query,
		seg.ItemId, seg.ClientId, seg.SegDate, seg.SegHour, seg.TemplateId,
		seg.LineCount, seg.ErrorCount, seg.WarnCount, seg.FirstTs, seg.LastTs,
		compressed, createdTime,
	)
	if err != nil {
		return fmt.Errorf("WriteSegment: exec failed: %w", err)
	}

	return nil
}

// ReadSegment 按 item_id + date + hour 读取单条段记录。
//
// 读取后自动执行解压，seg.Compressed 字段将被还原为原始列式数据。
// 段不存在时返回 ErrSegmentNotFound 哨兵错误。
func (r *SegmentReader) ReadSegment(ctx context.Context, itemID int64, date, hour string) (*models.TLogSegment, error) {
	query := `SELECT ` + segmentColumns + ` FROM t_log_segment
		WHERE item_id = ? AND seg_date = ? AND seg_hour = ?`

	var seg models.TLogSegment
	err := r.db.GetContext(ctx, &seg, query, itemID, date, hour)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSegmentNotFound
		}
		return nil, fmt.Errorf("ReadSegment: query failed: %w", err)
	}

	// 解压 compressed 字段
	decompressed, err := Decompress(seg.Compressed)
	if err != nil {
		return nil, fmt.Errorf("ReadSegment: decompress failed: %w", err)
	}
	seg.Compressed = decompressed

	return &seg, nil
}

// QuerySegments 批量读取指定日期、小时范围内的段记录。
//
// startHour / endHour 为闭区间，格式 "00"~"23"（如 "09", "17"）。
// 结果按 seg_hour 升序排列。返回的每条记录 compressed 字段均已解压。
func (r *SegmentReader) QuerySegments(ctx context.Context, itemID int64, date string, startHour, endHour string) ([]*models.TLogSegment, error) {
	query := `SELECT ` + segmentColumns + ` FROM t_log_segment
		WHERE item_id = ? AND seg_date = ? AND seg_hour >= ? AND seg_hour <= ?
		ORDER BY seg_hour ASC`

	var rows []models.TLogSegment
	err := r.db.SelectContext(ctx, &rows, query, itemID, date, startHour, endHour)
	if err != nil {
		return nil, fmt.Errorf("QuerySegments: query failed: %w", err)
	}

	result := make([]*models.TLogSegment, 0, len(rows))
	for i := range rows {
		decompressed, err := Decompress(rows[i].Compressed)
		if err != nil {
			return nil, fmt.Errorf("QuerySegments: decompress failed at row %d: %w", i, err)
		}
		rows[i].Compressed = decompressed
		result = append(result, &rows[i])
	}

	return result, nil
}

// DeleteSegmentsBefore 清理 beforeDate 之前的所有段记录（按 seg_date 比较）。
//
// beforeDate 格式 YYYYMMDD（如 "20250701"）。
// 返回被删除的记录行数。
func (w *SegmentWriter) DeleteSegmentsBefore(ctx context.Context, beforeDate string) (int64, error) {
	query := `DELETE FROM t_log_segment WHERE seg_date < ?`

	res, err := w.db.ExecContext(ctx, query, beforeDate)
	if err != nil {
		return 0, fmt.Errorf("DeleteSegmentsBefore: exec failed: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("DeleteSegmentsBefore: rows affected failed: %w", err)
	}

	return affected, nil
}
