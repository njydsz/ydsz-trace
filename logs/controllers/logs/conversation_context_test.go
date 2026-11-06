package logs

import (
	"testing"
)

// TestResolveCoref_NoHistory 无历史时不做消解。
func TestResolveCoref_NoHistory(t *testing.T) {
	got := ResolveCoref("近3小时 ERROR", nil)
	if got.Changed {
		t.Error("无历史不应触发消解")
	}
	if got.Strategy != "none" {
		t.Errorf("strategy = %q want none", got.Strategy)
	}
}

// TestResolveCoref_FillsLevelFromHistory 从历史补全级别。
func TestResolveCoref_FillsLevelFromHistory(t *testing.T) {
	history := []ConversationTurn{
		{Query: "近1小时 ERROR 日志", Parsed: Nl2DslParseResp{Level: "ERROR", TimeRangeDetail: "近1小时"}},
	}

	got := ResolveCoref("有没有 WARN", history)
	// 本轮已给出 WARN，不应被历史覆盖
	if got.Changed {
		t.Errorf("本轮显式给出级别时不应消解: %+v", got)
	}
}

// TestResolveCoref_FillsEmptyLevel 本轮无级别时从历史补全。
func TestResolveCoref_FillsEmptyLevel(t *testing.T) {
	history := []ConversationTurn{
		{Query: "近1小时 ERROR", Parsed: Nl2DslParseResp{
			Level: "ERROR", StartTime: "10:00:00", EndTime: "11:00:00", TimeRangeDetail: "近1小时",
		}},
	}

	got := ResolveCoref("那 timeout 的呢", history)
	if !got.Changed {
		t.Fatal("应触发消解")
	}
	if got.Strategy != "rule" {
		t.Errorf("strategy = %q want rule", got.Strategy)
	}
	// 补全后应包含历史级别或时间范围
	found := false
	for _, f := range got.FilledFields {
		if f == "timeRange" || f == "level" {
			found = true
		}
	}
	if !found {
		t.Errorf("filledFields = %v 应包含 level 或 timeRange", got.FilledFields)
	}
}

// TestResolveCoref_DoesNotOverrideExplicit 本轮显式条件优先。
func TestResolveCoref_DoesNotOverrideExplicit(t *testing.T) {
	history := []ConversationTurn{
		{Query: "ERROR", Parsed: Nl2DslParseResp{Level: "ERROR", Key: "old_key"}},
	}

	got := ResolveCoref("查一下 INFO 级别 connection refused", history)
	if got.Changed {
		t.Errorf("本轮已提供完整条件时不应覆盖: %+v", got)
	}
}

// TestResolveCoref_TrimHistory 历史超过上限时裁剪。
func TestResolveCoref_TrimHistory(t *testing.T) {
	turns := make([]ConversationTurn, maxContextTurns+10)
	for i := range turns {
		turns[i] = ConversationTurn{Query: "q", Parsed: Nl2DslParseResp{}}
	}
	got := trimContextTurns(turns)
	if len(got) != maxContextTurns {
		t.Errorf("len = %d want %d", len(got), maxContextTurns)
	}
}

// TestIsReferentialQuery 指代性查询识别。
func TestIsReferentialQuery(t *testing.T) {
	referential := []string{"它呢", "那这个呢", "刚才那个模板", "继续查", "再查一下", "同上"}
	for _, q := range referential {
		if !isReferentialQuery(q) {
			t.Errorf("应识别为指代查询: %q", q)
		}
	}
	normal := []string{"近3小时 ERROR", "timeout 日志", "20261101 的 WARN"}
	for _, q := range normal {
		if isReferentialQuery(q) {
			t.Errorf("不应识别为指代查询: %q", q)
		}
	}
}

// TestBuildResolvedQuery 可读查询描述构造。
func TestBuildResolvedQuery(t *testing.T) {
	cases := []struct {
		name string
		in   Nl2DslParseResp
		want string
	}{
		{
			name: "全字段",
			in:   Nl2DslParseResp{Level: "ERROR", Key: "timeout", TimeRangeDetail: "近3小时"},
			want: "ERROR，关键词「timeout」，时间范围 近3小时",
		},
		{
			name: "空结果",
			in:   Nl2DslParseResp{},
			want: "全量日志",
		},
		{
			name: "仅时间",
			in:   Nl2DslParseResp{StartTime: "10:00:00", EndTime: "11:00:00"},
			want: "时间 10:00:00~11:00:00",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := buildResolvedQuery(c.in); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

// TestBuildExplanationCard 解释卡字段完整。
func TestBuildExplanationCard(t *testing.T) {
	parsed := Nl2DslParseResp{
		Level: "ERROR", Key: "timeout", TimeRangeDetail: "近3小时",
		IntentType: INTENT_ERROR_SEARCH, Confidence: 0.8,
	}

	card := BuildExplanationCard("近3小时 ERROR timeout", parsed, "rule", nil)

	if card.Original == "" {
		t.Error("original 为空")
	}
	if card.Resolved == "" {
		t.Error("resolved 为空")
	}
	if len(card.Conditions) < 3 {
		t.Errorf("conditions = %d want >= 3", len(card.Conditions))
	}
	if card.Strategy != "rule" {
		t.Errorf("strategy = %q", card.Strategy)
	}
	if card.Confidence != 0.8 {
		t.Errorf("confidence = %f", card.Confidence)
	}

	// 校验条件来源标注
	for _, c := range card.Conditions {
		if c.Source == "" {
			t.Errorf("condition %s 缺少 source", c.Field)
		}
	}
}

// TestBuildExplanationCard_DegradedWarning 降级时给出提示。
func TestBuildExplanationCard_DegradedWarning(t *testing.T) {
	card := BuildExplanationCard("abcdefg", Nl2DslParseResp{IntentType: INTENT_GENERAL}, "degraded", nil)

	found := false
	for _, w := range card.Warnings {
		if containsStr(w, "降级") {
			found = true
		}
	}
	if !found {
		t.Errorf("降级场景应给出提示, warnings=%v", card.Warnings)
	}
}

// TestBuildExplanationCard_LowConfidenceWarning 低置信给出提示。
func TestBuildExplanationCard_LowConfidenceWarning(t *testing.T) {
	card := BuildExplanationCard("x", Nl2DslParseResp{Confidence: 0.2}, "rule", nil)

	found := false
	for _, w := range card.Warnings {
		if containsStr(w, "置信度") {
			found = true
		}
	}
	if !found {
		t.Errorf("低置信应给出提示, warnings=%v", card.Warnings)
	}
}

// TestExplanationCard_EmptyConditions 无有效条件时 conditions 为空数组。
func TestExplanationCard_EmptyConditions(t *testing.T) {
	card := BuildExplanationCard("abc", Nl2DslParseResp{}, "degraded", nil)
	if card.Conditions == nil {
		t.Error("conditions 应为非 nil 空数组")
	}
	if len(card.Conditions) != 0 {
		t.Errorf("len = %d want 0", len(card.Conditions))
	}
}
