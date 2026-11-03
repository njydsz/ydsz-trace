// Package storage — 列式段存储引擎测试。
package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"ydsz-trace/logs/models"
)

// setupTestStorage 初始化全局 DB 并返回 writer/reader 实例。
func setupTestStorage(t *testing.T) (*SegmentWriter, *SegmentReader) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_segment.db")
	if err := models.InitDB(&models.SQLiteConfig{FilePath: dbPath}); err != nil {
		t.Fatalf("初始化测试数据库失败: %v", err)
	}
	t.Cleanup(func() {
		if models.DB != nil {
			_ = models.DB.Close()
		}
	})
	db := models.DB
	return NewSegmentWriter(db), NewSegmentReader(db)
}

// TestWriteReadSegment_RoundTrip 验证写入后读取的字段一致性（含压缩/解压）。
func TestWriteReadSegment_RoundTrip(t *testing.T) {
	ctx := context.Background()
	writer, reader := setupTestStorage(t)

	original := []byte("column-data-for-testing-round-trip")
	seg := &models.TLogSegment{
		ItemId:     1001,
		ClientId:   2001,
		SegDate:    "20250715",
		SegHour:    "14",
		TemplateId: "tmpl_abc",
		LineCount:  1024,
		ErrorCount: 5,
		WarnCount:  23,
		FirstTs:    "2025-07-15 14:00:00",
		LastTs:     "2025-07-15 14:59:59",
		Compressed: original,
	}

	// 写入
	if err := writer.WriteSegment(ctx, seg); err != nil {
		t.Fatalf("WriteSegment failed: %v", err)
	}

	// 读取
	got, err := reader.ReadSegment(ctx, 1001, "20250715", "14")
	if err != nil {
		t.Fatalf("ReadSegment failed: %v", err)
	}

	// 验证元数据字段
	if got.ItemId != seg.ItemId {
		t.Errorf("ItemId: got %d, want %d", got.ItemId, seg.ItemId)
	}
	if got.ClientId != seg.ClientId {
		t.Errorf("ClientId: got %d, want %d", got.ClientId, seg.ClientId)
	}
	if got.SegDate != seg.SegDate {
		t.Errorf("SegDate: got %s, want %s", got.SegDate, seg.SegDate)
	}
	if got.SegHour != seg.SegHour {
		t.Errorf("SegHour: got %s, want %s", got.SegHour, seg.SegHour)
	}
	if got.TemplateId != seg.TemplateId {
		t.Errorf("TemplateId: got %s, want %s", got.TemplateId, seg.TemplateId)
	}
	if got.LineCount != seg.LineCount {
		t.Errorf("LineCount: got %d, want %d", got.LineCount, seg.LineCount)
	}
	if got.ErrorCount != seg.ErrorCount {
		t.Errorf("ErrorCount: got %d, want %d", got.ErrorCount, seg.ErrorCount)
	}
	if got.WarnCount != seg.WarnCount {
		t.Errorf("WarnCount: got %d, want %d", got.WarnCount, seg.WarnCount)
	}
	if got.FirstTs != seg.FirstTs {
		t.Errorf("FirstTs: got %s, want %s", got.FirstTs, seg.FirstTs)
	}
	if got.LastTs != seg.LastTs {
		t.Errorf("LastTs: got %s, want %s", got.LastTs, seg.LastTs)
	}

	// 验证 compressed 解压后与原数据一致
	if string(got.Compressed) != string(original) {
		t.Errorf("Compressed: got %q, want %q", got.Compressed, original)
	}
}

// TestReadSegment_NotFound 验证读取不存在的段返回 ErrSegmentNotFound。
func TestReadSegment_NotFound(t *testing.T) {
	ctx := context.Background()
	_, reader := setupTestStorage(t)

	_, err := reader.ReadSegment(ctx, 9999, "20250715", "00")
	if !errors.Is(err, ErrSegmentNotFound) {
		t.Fatalf("expected ErrSegmentNotFound, got: %v", err)
	}
}

// TestQuerySegments_Range 验证范围查询结果与排序。
func TestQuerySegments_Range(t *testing.T) {
	ctx := context.Background()
	writer, reader := setupTestStorage(t)

	// 写入 3 个小时段
	for _, hour := range []string{"08", "12", "18"} {
		seg := &models.TLogSegment{
			ItemId:     1001,
			ClientId:   2001,
			SegDate:    "20250715",
			SegHour:    hour,
			TemplateId: "tmpl_range",
			LineCount:  100,
			ErrorCount: 1,
			WarnCount:  2,
			FirstTs:    "2025-07-15 " + hour + ":00:00",
			LastTs:     "2025-07-15 " + hour + ":59:59",
			Compressed: []byte("hour-data-" + hour),
		}
		if err := writer.WriteSegment(ctx, seg); err != nil {
			t.Fatalf("WriteSegment hour=%s failed: %v", hour, err)
		}
	}

	// 查询 09~17 范围，应返回 hour=12
	segs, err := reader.QuerySegments(ctx, 1001, "20250715", "09", "17")
	if err != nil {
		t.Fatalf("QuerySegments failed: %v", err)
	}
	if len(segs) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segs))
	}
	if segs[0].SegHour != "12" {
		t.Errorf("expected hour=12, got %s", segs[0].SegHour)
	}
	if string(segs[0].Compressed) != "hour-data-12" {
		t.Errorf("Compressed mismatch: got %q, want %q", segs[0].Compressed, "hour-data-12")
	}

	// 查询全范围 00~23，应返回 3 条且按 hour 升序
	allSegs, err := reader.QuerySegments(ctx, 1001, "20250715", "00", "23")
	if err != nil {
		t.Fatalf("QuerySegments full range failed: %v", err)
	}
	if len(allSegs) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(allSegs))
	}
	expectedHours := []string{"08", "12", "18"}
	for i, s := range allSegs {
		if s.SegHour != expectedHours[i] {
			t.Errorf("segment[%d] hour: got %s, want %s", i, s.SegHour, expectedHours[i])
		}
	}
}

// TestDeleteSegmentsBefore 验证过期段清理与存活段保留。
func TestDeleteSegmentsBefore(t *testing.T) {
	ctx := context.Background()
	writer, reader := setupTestStorage(t)

	// 写入跨多个日期的段
	dates := []struct {
		date string
		hour string
	}{
		{"20250710", "10"},
		{"20250712", "14"},
		{"20250715", "18"},
		{"20250716", "08"},
	}
	for _, d := range dates {
		seg := &models.TLogSegment{
			ItemId:     1001,
			ClientId:   2001,
			SegDate:    d.date,
			SegHour:    d.hour,
			TemplateId: "tmpl_lifecycle",
			LineCount:  50,
			FirstTs:    d.date + " " + d.hour + ":00:00",
			LastTs:     d.date + " " + d.hour + ":59:59",
			Compressed: []byte("data-" + d.date),
		}
		if err := writer.WriteSegment(ctx, seg); err != nil {
			t.Fatalf("WriteSegment date=%s failed: %v", d.date, err)
		}
	}

	// 删除 20250714 之前的段（应删除 20250710 和 20250712）
	affected, err := writer.DeleteSegmentsBefore(ctx, "20250714")
	if err != nil {
		t.Fatalf("DeleteSegmentsBefore failed: %v", err)
	}
	if affected != 2 {
		t.Errorf("expected 2 deleted rows, got %d", affected)
	}

	// 验证 20250710 已删除
	_, err = reader.ReadSegment(ctx, 1001, "20250710", "10")
	if !errors.Is(err, ErrSegmentNotFound) {
		t.Errorf("segment 2025071010 should be deleted, err=%v", err)
	}

	// 验证 20250712 已删除
	_, err = reader.ReadSegment(ctx, 1001, "20250712", "14")
	if !errors.Is(err, ErrSegmentNotFound) {
		t.Errorf("segment 2025071214 should be deleted, err=%v", err)
	}

	// 验证 20250715 仍存活
	seg, err := reader.ReadSegment(ctx, 1001, "20250715", "18")
	if err != nil {
		t.Fatalf("segment 2025071518 should still exist: %v", err)
	}
	if string(seg.Compressed) != "data-20250715" {
		t.Errorf("Compressed mismatch after delete: got %q", seg.Compressed)
	}

	// 验证 20250716 仍存活
	seg2, err := reader.ReadSegment(ctx, 1001, "20250716", "08")
	if err != nil {
		t.Fatalf("segment 2025071608 should still exist: %v", err)
	}
	if string(seg2.Compressed) != "data-20250716" {
		t.Errorf("Compressed mismatch after delete: got %q", seg2.Compressed)
	}
}

// TestWriteSegment_Idempotent 验证同一 key 重复写入为 UPDATE 语义。
func TestWriteSegment_Idempotent(t *testing.T) {
	ctx := context.Background()
	writer, reader := setupTestStorage(t)

	seg := &models.TLogSegment{
		ItemId:     3001,
		ClientId:   4001,
		SegDate:    "20250801",
		SegHour:    "09",
		TemplateId: "tmpl_idem",
		LineCount:  10,
		Compressed: []byte("v1"),
	}
	if err := writer.WriteSegment(ctx, seg); err != nil {
		t.Fatalf("first WriteSegment failed: %v", err)
	}

	// 同一 key 再次写入（更新）
	seg.LineCount = 999
	seg.Compressed = []byte("v2-updated")
	if err := writer.WriteSegment(ctx, seg); err != nil {
		t.Fatalf("second WriteSegment failed: %v", err)
	}

	// 验证只存在一条记录且为最新值
	got, err := reader.ReadSegment(ctx, 3001, "20250801", "09")
	if err != nil {
		t.Fatalf("ReadSegment failed: %v", err)
	}
	if got.LineCount != 999 {
		t.Errorf("LineCount after update: got %d, want 999", got.LineCount)
	}
	if string(got.Compressed) != "v2-updated" {
		t.Errorf("Compressed after update: got %q, want %q", got.Compressed, "v2-updated")
	}

	// 验证全表只有一条记录
	segs, err := reader.QuerySegments(ctx, 3001, "20250801", "00", "23")
	if err != nil {
		t.Fatalf("QuerySegments failed: %v", err)
	}
	if len(segs) != 1 {
		t.Errorf("expected exactly 1 segment after idempotent write, got %d", len(segs))
	}
}
