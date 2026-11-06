package logs

import (
	"strings"
	"testing"
	"time"

	models "ydsz-trace/logs/models"
)

// TestChangeTimeGap 分钟差计算。
func TestChangeTimeGap(t *testing.T) {
	at, _ := time.ParseInLocation("2006-01-02 15:04:05", "2026-11-06 10:30:00", time.Local)

	cases := []struct {
		changeTime string
		want       int
	}{
		{"2026-11-06 10:00:00", 30},  // 变更在异常前 30 分钟
		{"2026-11-06 10:30:00", 0},   // 同时
		{"2026-11-06 10:35:00", -5},  // 变更在异常后 5 分钟
		{"2026-11-06 09:00:00", 90},  // 前 90 分钟
		{"invalid", 0},               // 解析失败
	}

	for _, c := range cases {
		if got := changeTimeGap(at, c.changeTime); got != c.want {
			t.Errorf("changeTimeGap(%q) = %d want %d", c.changeTime, got, c.want)
		}
	}
}

// TestGapSign 变更方位判断。
func TestGapSign(t *testing.T) {
	cases := []struct {
		name    string
		changes []ChangeBrief
		want    string
	}{
		{
			name:    "全部在前",
			changes: []ChangeBrief{{TimeGapMin: 10}, {TimeGapMin: 20}},
			want:    "before",
		},
		{
			name:    "全部在后",
			changes: []ChangeBrief{{TimeGapMin: -2}, {TimeGapMin: -5}},
			want:    "after",
		},
		{
			name:    "混合",
			changes: []ChangeBrief{{TimeGapMin: 10}, {TimeGapMin: -5}},
			want:    "mixed",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := gapSign(c.changes); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

// TestAbsInt 绝对值。
func TestAbsInt(t *testing.T) {
	if absInt(-5) != 5 || absInt(5) != 5 || absInt(0) != 0 {
		t.Error("absInt 行为错误")
	}
}

// TestValidChangeType 类型校验。
func TestValidChangeType(t *testing.T) {
	valid := []string{"deploy", "config", "db", "network", "other", ""}
	for _, v := range valid {
		if !validChangeType(v) {
			t.Errorf("%q 应合法", v)
		}
	}
	if validChangeType("unknown") {
		t.Error("未知类型应被拒绝")
	}
}

// TestBuildCorrelationExplanation 关联说明生成。
func TestBuildCorrelationExplanation(t *testing.T) {
	cases := []struct {
		name  string
		sign  string
		check string
	}{
		{name: "变更在前", sign: "before", check: "变更引入"},
		{name: "变更在后", sign: "after", check: "应急修复"},
		{name: "混合", sign: "mixed", check: "变更"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			changes := []ChangeBrief{{Title: "发布 v1.2.3", ChangeType: "deploy", TimeGapMin: 15}}
			got := buildCorrelationExplanation(changes, c.sign)
			if !strings.Contains(got, c.check) {
				t.Errorf("got %q 应包含 %q", got, c.check)
			}
			if !strings.Contains(got, "发布 v1.2.3") {
				t.Errorf("应包含变更标题: %q", got)
			}
		})
	}
}

// TestBuildCorrelationExplanation_Empty 空变更列表。
func TestBuildCorrelationExplanation_Empty(t *testing.T) {
	if got := buildCorrelationExplanation(nil, "before"); got != "" {
		t.Errorf("got %q want empty", got)
	}
}

// TestCorrelateChanges_EmptyEvents 无异常时返回 nil。
func TestCorrelateChanges_EmptyEvents(t *testing.T) {
	if got := CorrelateChanges(nil); got != nil {
		t.Errorf("got %v want nil", got)
	}
}

// TestCorrelateChanges_InvalidTime 时间解析失败时跳过。
func TestCorrelateChanges_InvalidTime(t *testing.T) {
	events := []models.TAnomalyEvent{{Id: 1, CreatedTime: "not-a-time"}}
	got := CorrelateChanges(events)
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].StrongSignal {
		t.Error("时间解析失败不应产生强信号")
	}
}

// TestInjectChangesIntoPrompt_NoDB DB 未初始化时原样返回。
func TestInjectChangesIntoPrompt_NoDB(t *testing.T) {
	base := "prompt"
	got := InjectChangesIntoPrompt(base, time.Now())
	if got != base {
		t.Errorf("无变更时应原样返回")
	}
}

// TestChangeBrief_Fields 字段完整性。
func TestChangeBrief_Fields(t *testing.T) {
	b := ChangeBrief{
		Title: "t", ChangeType: "deploy", Service: "s",
		Operator: "op", OccurredAt: "2026-11-06 10:00:00", TimeGapMin: 10,
	}
	if b.TimeGapMin != 10 || b.Title != "t" {
		t.Error("字段赋值错误")
	}
}
