// Package models 子测试：模式时序统计功能。
package models

import (
	"testing"
	"time"
)

// TestTimeBucket_5minAlign 验证 5 分钟对齐逻辑。
func TestTimeBucket_5minAlign(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2025-07-01 14:03:00", "202507011400"},
		{"2025-07-01 14:07:00", "202507011405"},
		{"2025-07-01 14:59:00", "202507011455"},
		{"2025-07-01 14:00:00", "202507011400"},
		{"2025-07-01 14:04:59", "202507011400"},
		{"2025-07-01 14:05:00", "202507011405"},
	}

	for _, tt := range tests {
		got, err := TimeBucket(tt.input)
		if err != nil {
			t.Fatalf("TimeBucket(%q) 失败: %v", tt.input, err)
		}
		if got != tt.expected {
			t.Errorf("TimeBucket(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// TestTimeBucket_InvalidTime 验证非法时间输入错误处理。
func TestTimeBucket_InvalidTime(t *testing.T) {
	_, err := TimeBucket("not-a-time")
	if err == nil {
		t.Error("非法时间输入应返回错误")
	}
}

// TestTemplateHash_Deterministic 验证哈希确定性。
func TestTemplateHash_Deterministic(t *testing.T) {
	h1 := TemplateHash("hello world")
	h2 := TemplateHash("hello world")
	if h1 != h2 {
		t.Errorf("TemplateHash 非确定性: %q != %q", h1, h2)
	}
	if len(h1) != 16 {
		t.Errorf("TemplateHash 长度应为 16，实际 %d", len(h1))
	}
}

// TestRecordPatternHit_Upsert 验证同 bucket 累加（需要 DB 初始化）。
func TestRecordPatternHit_Upsert(t *testing.T) {
	if DB == nil {
		t.Skip("DB 未初始化，跳过集成测试")
	}

	hash := "test_absolut_hash"
	ts := time.Now().Format("2006-01-02 15:04:05")
	sample := "test log line"

	// 首次写入
	_ = RecordPatternHit(1, 1, hash, ts, sample)
	// 第二次写入同 bucket（应累加）
	_ = RecordPatternHit(1, 1, hash, ts, sample)

	bucket, _ := TimeBucket(ts)
	var stat PatternStat
	err := DB.Get(&stat,
		`SELECT * FROM t_pattern_stat WHERE item_id=1 AND client_id=1 AND template_hash=? AND time_bucket=?`,
		hash, bucket)
	if err != nil {
		t.Fatalf("查询统计记录失败: %v", err)
	}
	if stat.Count != 2 {
		t.Errorf("同 bucket 应累加到 2，实际 %d", stat.Count)
	}
	if stat.FirstTime == "" {
		t.Error("FirstTime 不应为空")
	}

	// 清理
	_, _ = DB.Exec(`DELETE FROM t_pattern_stat WHERE template_hash=?`, hash)
}

// TestTopPatterns_Order 验证降序排列。
func TestTopPatterns_Order(t *testing.T) {
	if DB == nil {
		t.Skip("DB 未初始化，跳过集成测试")
	}

	// 准备测试数据
	now := time.Now().Format("2006-01-02 15:04:05")
	hashes := []string{"aaa_hash_low", "bbb_hash_high", "ccc_hash_mid"}
	counts := []int{1, 5, 3}
	for i, h := range hashes {
		for j := 0; j < counts[i]; j++ {
			_ = RecordPatternHit(99, 1, h, now, "sample")
		}
	}

	patterns, err := TopPatterns(99, "1h", 20)
	if err != nil {
		t.Fatalf("TopPatterns 失败: %v", err)
	}

	// 清理
	_, _ = DB.Exec(`DELETE FROM t_pattern_stat WHERE item_id=99`)

	if len(patterns) < 3 {
		t.Fatalf("期望至少 3 条，实际 %d", len(patterns))
	}
	// 验证降序
	if patterns[0].Count < patterns[1].Count {
		t.Errorf("结果未按降序排列: %d < %d", patterns[0].Count, patterns[1].Count)
	}
}
