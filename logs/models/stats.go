// Package models 子文件：模式时序计数与基线学习。
//
// t_pattern_stat 按 5 分钟桶聚合模板命中次数，供异常检测引擎计算基线并识别突变。
package models

import (
	"crypto/sha1"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// PatternStat 一行模式统计记录。
type PatternStat struct {
	ID           int64  `json:"id" db:"id"`
	ItemID       int64  `json:"itemId" db:"item_id"`
	ClientID     int64  `json:"clientId" db:"client_id"`
	TemplateHash string `json:"templateHash" db:"template_hash"`
	TimeBucket   string `json:"timeBucket" db:"time_bucket"`
	Count        int    `json:"count" db:"count"`
	FirstTime    string `json:"firstTime" db:"first_time"`
	LastTime     string `json:"lastTime" db:"last_time"`
	Sample       string `json:"sample" db:"sample"`
	CreatedTime  string `json:"createdTime" db:"created_time"`
}

// TemplateHash 计算模板模式 hash（sha1 前 16 字符）。
// normalized 为已归一化的日志行（数字/UUID/时间戳替换为占位符之后的结果）。
func TemplateHash(normalized string) string {
	h := sha1.Sum([]byte(normalized))
	return fmt.Sprintf("%x", h[:8])
}

// TimeBucket 对给定时间戳向下取整到 5min bucket，格式 YYYYMMDDHHMI。
func TimeBucket(timestamp string) (string, error) {
	ts, err := parseTime(timestamp)
	if err != nil {
		return "", err
	}
	minute := (ts.Minute() / 5) * 5
	return fmt.Sprintf("%04d%02d%02d%02d%02d", ts.Year(), ts.Month(), ts.Day(), ts.Hour(), minute), nil
}

// parseTime 尝试多种格式解析时间字符串。
func parseTime(ts string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, ts, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析时间: %s", ts)
}

// RecordPatternHit 记录一次模式命中（按 5min 时间桶聚合）。
// 同 bucket 命中时累加计数、更新末次时间，不覆盖 first_time 和 sample。
func RecordPatternHit(itemID, clientID int64, templateHash, timestamp, sample string) error {
	bucket, err := TimeBucket(timestamp)
	if err != nil {
		return err
	}

	// UPSERT：INSERT … ON CONFLICT … DO UPDATE
	_, err = DB.Exec(`INSERT INTO t_pattern_stat
		(item_id, client_id, template_hash, time_bucket, count, first_time, last_time, sample)
		VALUES (?, ?, ?, ?, 1, ?, ?, ?)
		ON CONFLICT(item_id, client_id, template_hash, time_bucket) DO UPDATE SET
			count = count + 1,
			last_time = excluded.last_time`,
		itemID, clientID, templateHash, bucket, timestamp, timestamp, sample,
	)
	if err != nil {
		log.Printf("[stats] RecordPatternHit 失败: %v", err)
	}
	return err
}

// QueryPatternTrend 查询某模板在时间范围内的计数趋势（用于趋势图）。
func QueryPatternTrend(templateHash string, startTime, endTime string) ([]PatternStat, error) {
	var rows []PatternStat
	err := DB.Select(&rows,
		`SELECT * FROM t_pattern_stat
		 WHERE template_hash = ? AND time_bucket >= ? AND time_bucket <= ?
		 ORDER BY time_bucket ASC`,
		templateHash, startTime, endTime)
	if err != nil {
		log.Printf("[stats] QueryPatternTrend 失败: %v", err)
	}
	return rows, err
}

// countByHash 按模板 hash 聚合计数（内部用）。
type countByHash struct {
	TemplateHash string `db:"template_hash"`
	TotalCount   int    `db:"total_count"`
}

// TopPatterns 取时间范围内出现次数最多的模式 TopN。
// timeRange 格式: "1h" / "6h" / "24h" / "7d"。
func TopPatterns(itemID int64, timeRange string, n int) ([]PatternStat, error) {
	if n <= 0 {
		n = 20
	}
	bucketStart := timeRangeToBucket(timeRange)
	if bucketStart == "" {
		return nil, fmt.Errorf("非法 timeRange: %s", timeRange)
	}

	type row struct {
		TemplateHash string `db:"template_hash"`
		TotalCount   int    `db:"total_count"`
		Sample       string `db:"sample"`
		LastTime     string `db:"last_time"`
	}
	var rows []row
	err := DB.Select(&rows,
		`SELECT template_hash, SUM(count) AS total_count,
		        (SELECT sample FROM t_pattern_stat p2
		         WHERE p2.template_hash = p1.template_hash
		         ORDER BY first_time ASC LIMIT 1) AS sample,
		        MAX(last_time) AS last_time
		 FROM t_pattern_stat p1
		 WHERE item_id = ? AND time_bucket >= ?
		 GROUP BY template_hash
		 ORDER BY total_count DESC
		 LIMIT ?`,
		itemID, bucketStart, n)
	if err != nil {
		log.Printf("[stats] TopPatterns 失败: %v", err)
		return nil, err
	}

	result := make([]PatternStat, 0, len(rows))
	for _, r := range rows {
		result = append(result, PatternStat{
			ItemID:       itemID,
			TemplateHash: r.TemplateHash,
			Count:        r.TotalCount,
			Sample:       r.Sample,
			LastTime:     r.LastTime,
		})
	}
	return result, nil
}

// timeRangeToBucket 将时间范围转为 5min bucket 起始字符串。
func timeRangeToBucket(timeRange string) string {
	if timeRange == "" {
		timeRange = "1h"
	}
	d, err := parseDuration(timeRange)
	if err != nil {
		return ""
	}
	ts := time.Now().Add(-d)
	minute := (ts.Minute() / 5) * 5
	return fmt.Sprintf("%04d%02d%02d%02d%02d", ts.Year(), ts.Month(), ts.Day(), ts.Hour(), minute)
}

// parseDuration 解析人类友好时长（1h / 6h / 24h / 7d）。
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	// 尝试标准库解析
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	// 处理 "7d" 这类
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err == nil {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	return 0, fmt.Errorf("非法时长: %s", s)
}
