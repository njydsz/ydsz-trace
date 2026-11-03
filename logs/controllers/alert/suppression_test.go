package alert

import (
	"testing"
	"time"
)

// TestSuppressor_NoWindowNotSuppressed 无窗口时不静默。
func TestSuppressor_NoWindowNotSuppressed(t *testing.T) {
	s := NewSuppressor()
	suppressed, name := s.IsSuppressed(time.Now())
	if suppressed {
		t.Fatalf("空窗口不应静默, got name=%q", name)
	}
}

// TestSuppressor_DailyWindow 日周期窗口命中。
func TestSuppressor_DailyWindow(t *testing.T) {
	s := NewSuppressor()
	s.ReplaceWindows([]MaintenanceWindow{{
		Name:      "nightly-deploy",
		Start:     time.Date(0, 1, 1, 2, 0, 0, 0, time.Local),
		End:       time.Date(0, 1, 1, 4, 0, 0, 0, time.Local),
		Recurring: "daily",
		Enabled:   true,
	}})

	inside := time.Date(2026, 11, 3, 3, 0, 0, 0, time.Local)
	if suppressed, name := s.IsSuppressed(inside); !suppressed || name != "nightly-deploy" {
		t.Fatalf("03:00 应在窗口内, got suppressed=%v name=%q", suppressed, name)
	}

	outside := time.Date(2026, 11, 3, 12, 0, 0, 0, time.Local)
	if suppressed, _ := s.IsSuppressed(outside); suppressed {
		t.Fatal("12:00 不应在窗口内")
	}
}

// TestSuppressor_CrossMidnight 跨天窗口（23:00~01:00）。
func TestSuppressor_CrossMidnight(t *testing.T) {
	s := NewSuppressor()
	s.ReplaceWindows([]MaintenanceWindow{{
		Name:      "maintenance",
		Start:     time.Date(0, 1, 1, 23, 0, 0, 0, time.Local),
		End:       time.Date(0, 1, 1, 1, 0, 0, 0, time.Local),
		Recurring: "daily",
		Enabled:   true,
	}})

	cases := []struct {
		hour int
		want bool
	}{
		{23, true},
		{0, true},
		{12, false},
		{22, false},
	}
	for _, c := range cases {
		at := time.Date(2026, 11, 3, c.hour, 0, 0, 0, time.Local)
		got, _ := s.IsSuppressed(at)
		if got != c.want {
			t.Errorf("hour=%d suppressed=%v want %v", c.hour, got, c.want)
		}
	}
}

// TestSuppressor_DisabledWindow 禁用窗口不生效。
func TestSuppressor_DisabledWindow(t *testing.T) {
	s := NewSuppressor()
	s.ReplaceWindows([]MaintenanceWindow{{
		Name:      "disabled",
		Start:     time.Date(0, 1, 1, 0, 0, 0, 0, time.Local),
		End:       time.Date(0, 1, 1, 23, 59, 59, 0, time.Local),
		Recurring: "daily",
		Enabled:   false,
	}})

	if suppressed, _ := s.IsSuppressed(time.Now()); suppressed {
		t.Fatal("禁用窗口不应静默")
	}
}

// TestSuppressor_WeeklyWindow 周周期窗口仅指定星期生效。
func TestSuppressor_WeeklyWindow(t *testing.T) {
	s := NewSuppressor()
	s.ReplaceWindows([]MaintenanceWindow{{
		Name:      "weekly",
		Start:     time.Date(0, 1, 1, 2, 0, 0, 0, time.Local),
		End:       time.Date(0, 1, 1, 4, 0, 0, 0, time.Local),
		Recurring: "weekly",
		Weekdays:  []int{int(time.Saturday), int(time.Sunday)},
		Enabled:   true,
	}})

	saturday := time.Date(2026, 11, 7, 3, 0, 0, 0, time.Local) // 周六
	if suppressed, _ := s.IsSuppressed(saturday); !suppressed {
		t.Fatal("周六 03:00 应在窗口内")
	}

	monday := time.Date(2026, 11, 9, 3, 0, 0, 0, time.Local) // 周一
	if suppressed, _ := s.IsSuppressed(monday); suppressed {
		t.Fatal("周一 03:00 不应在窗口内")
	}
}

// TestSuppressor_ZeroLengthWindow 零长度窗口视为全天静默。
func TestSuppressor_ZeroLengthWindow(t *testing.T) {
	s := NewSuppressor()
	s.ReplaceWindows([]MaintenanceWindow{{
		Name:      "all-day",
		Start:     time.Date(0, 1, 1, 0, 0, 0, 0, time.Local),
		End:       time.Date(0, 1, 1, 0, 0, 0, 0, time.Local),
		Recurring: "daily",
		Enabled:   true,
	}})

	for _, h := range []int{0, 6, 12, 23} {
		at := time.Date(2026, 11, 3, h, 0, 0, 0, time.Local)
		if suppressed, _ := s.IsSuppressed(at); !suppressed {
			t.Errorf("零长度窗口应全天静默, hour=%d 未命中", h)
		}
	}
}

// TestMergeAnomalies_DedupeSameTemplate 同模板多算法命中应合并。
func TestMergeAnomalies_DedupeSameTemplate(t *testing.T) {
	events := []AnomalyEventLike{
		&fakeEvent{hash: "tpl-a", algorithm: "z-score", severity: "warning", score: 2.1, count: 100},
		&fakeEvent{hash: "tpl-a", algorithm: "ewma", severity: "critical", score: 3.5, count: 100},
	}

	merged := MergeAnomalies(events, 3)
	if len(merged) != 1 {
		t.Fatalf("合并后应为 1 条, got %d", len(merged))
	}

	if merged[0].GetSeverity() != "critical" {
		t.Errorf("应保留最高严重度 critical, got %s", merged[0].GetSeverity())
	}
	if merged[0].GetScore() < 3.5 {
		t.Errorf("应保留最大 score, got %.2f", merged[0].GetScore())
	}
}

// TestMergeAnomalies_DifferentTemplateNotMerged 不同模板不合并。
func TestMergeAnomalies_DifferentTemplateNotMerged(t *testing.T) {
	events := []AnomalyEventLike{
		&fakeEvent{hash: "tpl-a", algorithm: "z-score", severity: "warning", score: 2.1, count: 100},
		&fakeEvent{hash: "tpl-b", algorithm: "z-score", severity: "warning", score: 2.2, count: 200},
	}

	merged := MergeAnomalies(events, 3)
	if len(merged) != 2 {
		t.Fatalf("不同模板不应合并, got %d", len(merged))
	}
}

// TestMergeAnomalies_AlgorithmsCollected 算法清单应去重收集。
func TestMergeAnomalies_AlgorithmsCollected(t *testing.T) {
	events := []AnomalyEventLike{
		&fakeEvent{hash: "tpl-a", algorithm: "z-score", severity: "warning", score: 2.1, count: 100},
		&fakeEvent{hash: "tpl-a", algorithm: "ewma", severity: "warning", score: 2.2, count: 100},
		&fakeEvent{hash: "tpl-a", algorithm: "z-score", severity: "warning", score: 2.3, count: 100},
	}

	merged := MergeAnomalies(events, 3)
	if len(merged) != 1 {
		t.Fatalf("合并后应为 1 条, got %d", len(merged))
	}

	ad, ok := merged[0].(*fakeEvent)
	if !ok {
		t.Fatal("类型断言失败")
	}
	if len(ad.algorithms) != 2 {
		t.Errorf("算法清单应去重为 2 项, got %v", ad.algorithms)
	}
}

// TestNormalizeAlgorithms 算法清单去重排序。
func TestNormalizeAlgorithms(t *testing.T) {
	got := NormalizeAlgorithms([]string{"ewma", "z-score", "ewma", "", "  periodic  "})
	want := []string{"ewma", "periodic", "z-score"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("idx %d: got %q want %q", i, got[i], want[i])
		}
	}
}

// fakeEvent 测试用假事件。
type fakeEvent struct {
	hash       string
	algorithm  string
	severity   string
	score      float64
	count      int
	algorithms []string
}

func (f *fakeEvent) GetTemplateHash() string     { return f.hash }
func (f *fakeEvent) GetAlgorithm() string        { return f.algorithm }
func (f *fakeEvent) GetSeverity() string         { return f.severity }
func (f *fakeEvent) GetScore() float64           { return f.score }
func (f *fakeEvent) GetCurrentCount() int        { return f.count }
func (f *fakeEvent) SetScore(v float64)          { f.score = v }
func (f *fakeEvent) SetCurrentCount(v int)       { f.count = v }
func (f *fakeEvent) SetAlgorithms(list []string) { f.algorithms = list }
