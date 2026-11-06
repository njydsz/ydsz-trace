package logs

import (
	"strings"
	"testing"
	"time"

	models "ydsz-trace/logs/models"
)

// TestAssessSeverity 严重级别判定。
func TestAssessSeverity(t *testing.T) {
	cases := []struct {
		name     string
		events   []models.TAnomalyEvent
		patterns int
		want     string
	}{
		{name: "无异常", events: nil, patterns: 0, want: "P3"},
		{name: "少量异常", events: []models.TAnomalyEvent{{Score: 2}}, patterns: 1, want: "P2"},
		{name: "多条异常", events: []models.TAnomalyEvent{{Score: 2}, {Score: 3}, {Score: 2}}, patterns: 1, want: "P1"},
		{name: "高 score", events: []models.TAnomalyEvent{{Score: 9}}, patterns: 1, want: "P0"},
		{name: "中等 score", events: []models.TAnomalyEvent{{Score: 6}}, patterns: 1, want: "P1"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			patterns := make([]models.PatternStat, c.patterns)
			if got := assessSeverity(c.events, patterns); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

// TestAssessSeverity_FatalPattern FATAL 模板直接判 P0。
func TestAssessSeverity_FatalPattern(t *testing.T) {
	patterns := []models.PatternStat{{Sample: "FATAL"}}
	if got := assessSeverity(nil, patterns); got != "P0" {
		t.Errorf("got %q want P0", got)
	}
}

// TestFormatDuration 时长格式化。
func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{3 * time.Hour, "3 小时"},
		{3*time.Hour + 30*time.Minute, "3 小时 30 分钟"},
		{45 * time.Minute, "45 分钟"},
	}
	for _, c := range cases {
		got := formatDuration(c.d)
		if got != c.want {
			t.Errorf("formatDuration(%v) = %q want %q", c.d, got, c.want)
		}
	}
}

// TestEscapeMarkdown 表格特殊字符转义。
func TestEscapeMarkdown(t *testing.T) {
	got := escapeMarkdown("a|b\nc")
	// 竖线应被转义为 \|（Markdown 表格内合法）
	if !strings.Contains(got, `\|`) {
		t.Errorf("应转义竖线为 \\|: %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Errorf("应替换换行: %q", got)
	}
}

// TestRenderReportMarkdown 渲染包含必要章节。
func TestRenderReportMarkdown(t *testing.T) {
	report := &IncidentReport{
		Title:     "事故简报：测试",
		CreatedAt: "2026-11-06 10:00:00",
		Severity:  "P1",
		StartedAt: "2026-11-05 10:00:00",
		Duration:  "24 小时",
		Summary:   "服务异常",
		RootCause: "连接池耗尽",
		Impact:    "命中 100 条",
		Timeline:  []IncidentTimelineItem{{Time: "10:00", Description: "异常开始"}},
		Evidence:  []EvidenceRef{{Source: EvidenceSourceLog, Summary: "ERROR timeout", Quote: "timeout"}},
		Actions:   []string{"扩容连接池"},
	}

	md := renderReportMarkdown(report)

	for _, want := range []string{
		"# 事故简报：测试",
		"## 概述",
		"## 根因分析",
		"## 影响面",
		"## 时间线",
		"## 证据",
		"## 后续动作",
		"连接池耗尽",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown 缺少 %q", want)
		}
	}
}

// TestRenderReportMarkdown_EmptySections 无时间线时不渲染空章节。
func TestRenderReportMarkdown_EmptySections(t *testing.T) {
	report := &IncidentReport{
		Title:     "t",
		CreatedAt: "now",
		Severity:  "P3",
		Summary:   "s",
		RootCause: "r",
		Impact:    "i",
	}

	md := renderReportMarkdown(report)
	if strings.Contains(md, "## 时间线") {
		t.Error("无时间线时不应渲染该章节")
	}
	if strings.Contains(md, "## 证据") {
		t.Error("无证据时不应渲染该章节")
	}
}

// TestBuildReportTitle 标题生成。
func TestBuildReportTitle(t *testing.T) {
	withEvents := buildReportTitle("timeout 错误", []models.TAnomalyEvent{{}})
	if !strings.Contains(withEvents, "事故简报") {
		t.Errorf("有异常时应为事故简报: %q", withEvents)
	}

	without := buildReportTitle("timeout 错误", nil)
	if !strings.Contains(without, "日志分析报告") {
		t.Errorf("无异常时应为分析报告: %q", without)
	}
}

// TestBuildImpact 影响面描述。
func TestBuildImpact(t *testing.T) {
	if got := buildImpact(nil); !strings.Contains(got, "未统计") {
		t.Errorf("空模板应给出兜底: %q", got)
	}

	patterns := []models.PatternStat{{Count: 100}, {Count: 50}}
	got := buildImpact(patterns)
	if !strings.Contains(got, "150") {
		t.Errorf("应包含总数: %q", got)
	}
	if !strings.Contains(got, "2") {
		t.Errorf("应包含模板数: %q", got)
	}
}

// TestBuildFallbackRootCause 兜底根因。
func TestBuildFallbackRootCause(t *testing.T) {
	if got := buildFallbackRootCause(nil, nil); !strings.Contains(got, "未检测到") {
		t.Errorf("无事件时应说明: %q", got)
	}

	events := []models.TAnomalyEvent{{}, {}}
	patterns := []models.PatternStat{{TemplateHash: "abc", Count: 10}}
	got := buildFallbackRootCause(events, patterns)
	if !strings.Contains(got, "abc") {
		t.Errorf("应包含高频模板: %q", got)
	}
}

// TestBuildTimeline 时间线截断。
func TestBuildTimeline(t *testing.T) {
	if got := buildTimeline(nil); got != nil {
		t.Errorf("nil want nil, got %v", got)
	}

	events := make([]models.TAnomalyEvent, 30)
	for i := range events {
		events[i] = models.TAnomalyEvent{CreatedTime: "t", Description: "d"}
	}
	got := buildTimeline(events)
	if len(got) > 20 {
		t.Errorf("len = %d 应截断到 20", len(got))
	}
}

// TestBuildReportEvidence 证据汇总。
func TestBuildReportEvidence(t *testing.T) {
	refs := buildReportEvidence(
		[]models.TAnomalyEvent{{Id: 1, Description: "a", Score: 3}},
		[]models.PatternStat{{TemplateHash: "h", Count: 5}},
	)
	if len(refs) < 2 {
		t.Fatalf("len = %d want >= 2", len(refs))
	}
	if refs[0].Source != EvidenceSourceAnomaly {
		t.Errorf("first source = %q", refs[0].Source)
	}
	if refs[1].Source != EvidenceSourceTemplate {
		t.Errorf("second source = %q", refs[1].Source)
	}
}
