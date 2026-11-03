package index

import (
	"testing"
	"time"
)

func TestNewLocalIndex(t *testing.T) {
	idx := NewLocalIndex()
	if idx == nil {
		t.Fatal("NewLocalIndex() 返回 nil")
	}
}

func TestLocalIndex_Add(t *testing.T) {
	idx := NewLocalIndex()

	entries := []IndexEntry{
		{LineNumber: 1, Timestamp: "2024-01-15 10:30:45", TemplateHash: "aaa", Level: "INFO", Content: "line1"},
		{LineNumber: 2, Timestamp: "2024-01-15 10:30:46", TemplateHash: "bbb", Level: "ERROR", Content: "line2"},
	}

	for _, e := range entries {
		idx.Add(1, "20240115", e)
	}

	if idx.Count(1, "20240115") != 2 {
		t.Errorf("Count() = %d, 期望 2", idx.Count(1, "20240115"))
	}
}

func TestLocalIndex_SearchByTemplate(t *testing.T) {
	idx := NewLocalIndex()

	idx.Add(1, "20240115", IndexEntry{LineNumber: 1, Timestamp: "2024-01-15 10:30:45", TemplateHash: "aaa", Level: "INFO"})
	idx.Add(1, "20240115", IndexEntry{LineNumber: 2, Timestamp: "2024-01-15 10:30:46", TemplateHash: "bbb", Level: "ERROR"})
	idx.Add(1, "20240115", IndexEntry{LineNumber: 3, Timestamp: "2024-01-15 10:30:47", TemplateHash: "aaa", Level: "WARN"})

	result := idx.SearchByTemplate(1, "aaa")
	if len(result) != 2 {
		t.Errorf("SearchByTemplate() 返回 %d 条, 期望 2", len(result))
	}

	// 验证排序（按时间戳升序）
	if len(result) >= 2 && result[0].Timestamp > result[1].Timestamp {
		t.Error("SearchByTemplate() 结果未按时间戳升序排序")
	}

	// 搜索不存在的模板
	empty := idx.SearchByTemplate(1, "ccc")
	if len(empty) != 0 {
		t.Errorf("SearchByTemplate() 不存在模板返回 %d 条, 期望 0", len(empty))
	}
}

func TestLocalIndex_SearchByTimeRange(t *testing.T) {
	idx := NewLocalIndex()

	idx.Add(1, "20240115", IndexEntry{LineNumber: 1, Timestamp: "2024-01-15 10:00:00", TemplateHash: "aaa", Level: "INFO"})
	idx.Add(1, "20240115", IndexEntry{LineNumber: 2, Timestamp: "2024-01-15 11:00:00", TemplateHash: "bbb", Level: "ERROR"})
	idx.Add(1, "20240115", IndexEntry{LineNumber: 3, Timestamp: "2024-01-15 12:00:00", TemplateHash: "ccc", Level: "WARN"})

	start, _ := time.Parse("2006-01-02 15:04:05", "2024-01-15 10:30:00")
	end, _ := time.Parse("2006-01-02 15:04:05", "2024-01-15 11:30:00")

	result := idx.SearchByTimeRange(1, start, end)
	if len(result) != 1 {
		t.Errorf("SearchByTimeRange() 返回 %d 条, 期望 1", len(result))
	}
	if len(result) > 0 && result[0].TemplateHash != "bbb" {
		t.Errorf("SearchByTimeRange() 返回模板 %s, 期望 bbb", result[0].TemplateHash)
	}
}

func TestLocalIndex_SearchByTimeRange_Boundary(t *testing.T) {
	idx := NewLocalIndex()

	idx.Add(1, "20240115", IndexEntry{LineNumber: 1, Timestamp: "2024-01-15 10:00:00", TemplateHash: "aaa", Level: "INFO"})

	start, _ := time.Parse("2006-01-02 15:04:05", "2024-01-15 10:00:00")
	end, _ := time.Parse("2006-01-02 15:04:05", "2024-01-15 10:00:00")

	// 边界值应包含
	result := idx.SearchByTimeRange(1, start, end)
	if len(result) != 1 {
		t.Errorf("SearchByTimeRange() 边界值返回 %d 条, 期望 1", len(result))
	}
}

func TestLocalIndex_Clear(t *testing.T) {
	idx := NewLocalIndex()

	idx.Add(1, "20240115", IndexEntry{LineNumber: 1, Timestamp: "2024-01-15 10:00:00", TemplateHash: "aaa", Level: "INFO"})
	idx.Add(1, "20240116", IndexEntry{LineNumber: 2, Timestamp: "2024-01-16 10:00:00", TemplateHash: "bbb", Level: "INFO"})

	idx.Clear(1, "20240115")

	if idx.Count(1, "20240115") != 0 {
		t.Errorf("Clear() 后 Count() = %d, 期望 0", idx.Count(1, "20240115"))
	}
	if idx.Count(1, "20240116") != 1 {
		t.Errorf("Clear() 不应影响其他日期, Count() = %d, 期望 1", idx.Count(1, "20240116"))
	}
}

func TestLocalIndex_TotalCount(t *testing.T) {
	idx := NewLocalIndex()

	idx.Add(1, "20240115", IndexEntry{LineNumber: 1, Timestamp: "2024-01-15 10:00:00", TemplateHash: "aaa", Level: "INFO"})
	idx.Add(1, "20240115", IndexEntry{LineNumber: 2, Timestamp: "2024-01-15 11:00:00", TemplateHash: "bbb", Level: "INFO"})
	idx.Add(2, "20240115", IndexEntry{LineNumber: 1, Timestamp: "2024-01-15 10:00:00", TemplateHash: "ccc", Level: "INFO"})

	if idx.TotalCount() != 3 {
		t.Errorf("TotalCount() = %d, 期望 3", idx.TotalCount())
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		input   string
		wantZero bool
	}{
		{"2024-01-15 10:30:45", false},
		{"2024-01-15T10:30:45", false},
		{"2024-01-15T10:30:45Z", false},
		{"", true},
		{"invalid", true},
	}

	for _, tt := range tests {
		got := parseTimestamp(tt.input)
		if tt.wantZero && !got.IsZero() {
			t.Errorf("parseTimestamp(%q) 应为 zero, 得到 %v", tt.input, got)
		}
		if !tt.wantZero && got.IsZero() {
			t.Errorf("parseTimestamp(%q) 不应为 zero", tt.input)
		}
	}
}

// ===== 并发安全测试 =====

func TestLocalIndex_ConcurrentAccess(t *testing.T) {
	idx := NewLocalIndex()
	done := make(chan bool)

	// 并发写入
	go func() {
		for i := 0; i < 100; i++ {
			idx.Add(1, "20240115", IndexEntry{LineNumber: i, Timestamp: "2024-01-15 10:00:00", TemplateHash: "aaa", Level: "INFO"})
		}
		done <- true
	}()

	// 并发读取
	go func() {
		for i := 0; i < 100; i++ {
			_ = idx.SearchByTemplate(1, "aaa")
			_ = idx.Count(1, "20240115")
		}
		done <- true
	}()

	<-done
	<-done

	// 最终计数应为 100
	if idx.Count(1, "20240115") != 100 {
		t.Errorf("并发写入后 Count() = %d, 期望 100", idx.Count(1, "20240115"))
	}
}
