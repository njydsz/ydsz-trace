// Package stats 单元测试：异常检测引擎。
package stats

import (
	"testing"
)

// TestDetectAnomalies_Surge 验证 Z-score > 3 触发 critical surge。
func TestDetectAnomalies_Surge(t *testing.T) {
	eng := NewAnomalyEngine()

	// 建立稳定基线：30 桶均值约 10
	for i := 0; i < 30; i++ {
		eng.UpdateBaseline("hash_surge", 1, 10)
	}

	// 注入异常值：100 远超基线
	current := map[string]int{"hash_surge": 100}
	events := eng.DetectAnomalies(current, "2025-07-01 14:00:00")

	if len(events) != 1 {
		t.Fatalf("期望 1 个异常事件，实际 %d", len(events))
	}
	ev := events[0]
	if ev.Direction != DirectionSurge {
		t.Errorf("期望 direction=surge, 实际 %q", ev.Direction)
	}
	if ev.Severity != SeverityCritical {
		t.Errorf("期望 severity=critical, 实际 %q", ev.Severity)
	}
	if ev.CurrentCount != 100 {
		t.Errorf("期望 currentCount=100, 实际 %d", ev.CurrentCount)
	}
}

// TestDetectAnomalies_Normal 验证正常数据不产生异常。
func TestDetectAnomalies_Normal(t *testing.T) {
	eng := NewAnomalyEngine()

	// 建立稳定基线
	for i := 0; i < 30; i++ {
		eng.UpdateBaseline("hash_normal", 1, 10)
	}

	// 正常波动值
	current := map[string]int{"hash_normal": 12}
	events := eng.DetectAnomalies(current, "2025-07-01 14:00:00")

	if len(events) != 0 {
		t.Errorf("正常数据不应产生异常，实际 %d 个事件", len(events))
	}
}

// TestDetectAnomalies_Drop 验证下降异常检测。
func TestDetectAnomalies_Drop(t *testing.T) {
	eng := NewAnomalyEngine()

	// 建立较高均值基线
	for i := 0; i < 30; i++ {
		eng.UpdateBaseline("hash_drop", 1, 20)
	}

	// 骤降至 1
	current := map[string]int{"hash_drop": 1}
	events := eng.DetectAnomalies(current, "2025-07-01 14:00:00")

	found := false
	for _, ev := range events {
		if ev.Direction == DirectionDrop {
			found = true
			if ev.Severity != SeverityWarning {
				t.Errorf("drop 应为 warning 级，实际 %q", ev.Severity)
			}
		}
	}
	if !found {
		t.Error("期望检测到 drop 异常，实际未检测到")
	}
}

// TestDetectAnomalies_InsufficientData 验证数据不足时不误报。
func TestDetectAnomalies_InsufficientData(t *testing.T) {
	eng := NewAnomalyEngine()

	// 只给 2 桶数据（需要 >= 3）
	eng.UpdateBaseline("hash_new", 1, 10)
	eng.UpdateBaseline("hash_new", 1, 100)

	current := map[string]int{"hash_new": 1000}
	events := eng.DetectAnomalies(current, "2025-07-01 14:00:00")

	if len(events) != 0 {
		t.Errorf("数据不足时不应产生异常，实际 %d 个事件", len(events))
	}
}

// TestUpdateBaseline_SlidingWindow 验证滑动窗口更新。
func TestUpdateBaseline_SlidingWindow(t *testing.T) {
	eng := NewAnomalyEngine()

	// 插入 35 个值（窗口大小 30，应只保留最近 30 个）
	for i := 0; i < 35; i++ {
		eng.UpdateBaseline("hash_win", 1, i)
	}

	bl, ok := eng.GetBaseline("hash_win", 1)
	if !ok {
		t.Fatal("基线应存在")
	}

	if len(bl.Last30) != 30 {
		t.Errorf("滑动窗口应保留 30 个值，实际 %d", len(bl.Last30))
	}

	// 期望均值 = (5+6+...+34)/30 = (sum 0..34 - sum 0..4)/30
	// sum 0..34 = 34*35/2 = 595, sum 0..4 = 10
	// expected = (595-10)/30 = 585/30 = 19.5
	expected := 19.5
	if bl.Mean != expected {
		t.Errorf("期望均值 %.1f，实际 %.4f", expected, bl.Mean)
	}
}

// TestUpdateBaseline_StdDev 验证标准差计算。
func TestUpdateBaseline_StdDev(t *testing.T) {
	eng := NewAnomalyEngine()

	// 全部相同值 → stddev 应为 0
	for i := 0; i < 10; i++ {
		eng.UpdateBaseline("hash_const", 1, 5)
	}
	bl, _ := eng.GetBaseline("hash_const", 1)
	if bl.StdDev != 0 {
		t.Errorf("常量值的标准差应为 0，实际 %.4f", bl.StdDev)
	}
}

// TestDetectAnomalies_ZeroStdDev 验证 stddev=0 时的降级逻辑。
func TestDetectAnomalies_ZeroStdDev(t *testing.T) {
	eng := NewAnomalyEngine()

	// 建立常值基线（stddev=0）
	for i := 0; i < 10; i++ {
		eng.UpdateBaseline("hash_zero_std", 1, 5)
	}

	// 稍微变化，不应异常
	current := map[string]int{"hash_zero_std": 8}
	events := eng.DetectAnomalies(current, "2025-07-01 14:00:00")
	if len(events) != 0 {
		t.Errorf("stddev=0 时不应对小幅波动产生异常，实际 %d 个事件", len(events))
	}

	// 大幅变化
	current2 := map[string]int{"hash_zero_std": 1000}
	events2 := eng.DetectAnomalies(current2, "2025-07-01 14:05:00")
	if len(events2) == 0 {
		t.Error("stddev=0 且大幅变化应产生异常")
	}
}
