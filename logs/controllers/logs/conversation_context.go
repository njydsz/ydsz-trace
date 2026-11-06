// Package logs 子文件：多轮会话上下文解析与解释卡契约（V2.3-05）。
//
// 两个能力：
//
//  1. 指代消解（Coreference Resolution）
//     多轮对话中用户会说「那它呢」「再查一下这个」「刚才那个模板」等指代性表达，
//     单独解析会得到空关键词。本模块把历史轮次的解析结果作为上下文，
//     用规则 + LLM 两级策略补全指代字段。
//
//  2. 解释卡后端契约（Explanation Card）
//     PRD FR-505 要求 NL2DSL 输出「解释卡」：告诉用户系统如何理解了他的问题。
//     本模块定义该契约的数据结构，并作为 NL2DSL 响应的一部分返回。
//
// 安全约束：
//   - 上下文仅取最近 N 轮，避免 prompt 膨胀
//   - 指代消解只补全「空字段」，不覆盖用户本轮显式给出的条件
package logs

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// 多轮上下文常量。
const (
	// maxContextTurns 参与指代消解的历史轮数上限
	maxContextTurns = 5
	// corefLLMTimeout 指代消解 LLM 超时
	corefLLMTimeout = 8 * time.Second
	// maxResolvedQueryLen 消解后 query 的最大长度（防止拼接爆炸）
	maxResolvedQueryLen = 500
)

// ConversationTurn 单轮对话上下文。
type ConversationTurn struct {
	// Query 用户本轮原始输入
	Query string `json:"query"`
	// Parsed 本轮解析结果
	Parsed Nl2DslParseResp `json:"parsed"`
}

// CorefResult 指代消解结果。
type CorefResult struct {
	// ResolvedQuery 补全指代后的完整查询
	ResolvedQuery string `json:"resolvedQuery"`
	// Changed 是否有字段被补全
	Changed bool `json:"changed"`
	// FilledFields 被补全的字段名列表（供解释卡展示）
	FilledFields []string `json:"filledFields,omitempty"`
	// Strategy 消解策略：rule / llm / none
	Strategy string `json:"strategy"`
}

// ExplanationCard 解释卡（FR-505）。
//
// 目的：让用户知道系统「如何理解」自己的问题，并能在理解错误时快速纠正。
type ExplanationCard struct {
	// Original 用户原始输入
	Original string `json:"original"`
	// Resolved 消解/补全后的完整语义
	Resolved string `json:"resolved"`
	// Conditions 解析出的检索条件清单
	Conditions []ExplanationCondition `json:"conditions"`
	// Strategy 解析策略（rule / llm / degraded）
	Strategy string `json:"strategy"`
	// Confidence 置信度（0~1）
	Confidence float64 `json:"confidence"`
	// Warnings 低置信或降级时的提示
	Warnings []string `json:"warnings,omitempty"`
}

// ExplanationCondition 单条检索条件。
type ExplanationCondition struct {
	// Field 字段名（level / key / timeRange / intent）
	Field string `json:"field"`
	// Value 字段值
	Value string `json:"value"`
	// Source 来源：rule / llm / coref（便于用户判断可信度）
	Source string `json:"source"`
}

// ResolveCoref 对当前轮查询做指代消解。
//
// 参数：
//   - current 本轮用户输入
//   - history 最近若干轮的历史（按时间正序，最新在最后）
//
// 返回消解结果；无需消解时 Changed=false。
func ResolveCoref(current string, history []ConversationTurn) CorefResult {
	// 先按规则解析本轮
	currentParsed := nl2dslBuildResponse(current)
	currentParsed.Confidence = nl2dslConfidence(currentParsed)

	// 无历史 → 无需消解
	if len(history) == 0 {
		return CorefResult{ResolvedQuery: current, Strategy: "none"}
	}

	// 规则消解：用最近一轮的非空字段补全本轮空字段
	filled := make([]string, 0, 3)
	merged := currentParsed

	for i := len(history) - 1; i >= 0 && len(filled) < 3; i-- {
		prev := history[i].Parsed
		if prev.Level == "" && prev.Key == "" && prev.StartTime == "" {
			continue
		}

		if merged.Level == "" && prev.Level != "" {
			merged.Level = prev.Level
			filled = append(filled, "level")
		}
		if !hasUsableKeyword(merged.Key) && hasUsableKeyword(prev.Key) {
			merged.Key = prev.Key
			filled = append(filled, "key")
		}
		if merged.StartTime == "" && prev.StartTime != "" {
			merged.StartTime = prev.StartTime
			merged.EndTime = prev.EndTime
			merged.TimeRangeDetail = prev.TimeRangeDetail
			filled = append(filled, "timeRange")
		}
	}

	if len(filled) > 0 {
		merged.DslConditions = nl2dslFormatConditions(merged.Level, merged.Key, merged.Date, merged.StartTime, merged.EndTime)
		return CorefResult{
			ResolvedQuery: buildResolvedQuery(merged),
			Changed:       true,
			FilledFields:  filled,
			Strategy:      "rule",
		}
	}

	// 规则无法消解且本轮信息极少 → 尝试 LLM
	if isReferentialQuery(current) {
		if resolved, ok := corefLLMResolve(current, history); ok {
			return CorefResult{
				ResolvedQuery: resolved,
				Changed:       true,
				FilledFields:  []string{"referent"},
				Strategy:      "llm",
			}
		}
	}

	return CorefResult{ResolvedQuery: current, Strategy: "none"}
}

// isReferentialQuery 判断是否为指代性查询。
//
// 特征：包含「它/这个/那个/刚才/上面/继续/再」等指代词，且本身无可提取信息。
func isReferentialQuery(q string) bool {
	referentials := []string{"它", "这个", "那个", "刚才", "上面", "刚才那个", "继续", "再查", "再找", "同上"}
	for _, r := range referentials {
		if strings.Contains(q, r) {
			return true
		}
	}
	return false
}

// corefLLMResolve 用 LLM 补全指代。
//
// 只把「历史轮次的解析条件」而非日志原文交给模型，符合数据不出内网约束。
func corefLLMResolve(current string, history []ConversationTurn) (string, bool) {
	llm := NewLLMClient()
	if !llm.Available() {
		return "", false
	}

	var b strings.Builder
	b.WriteString("以下是多轮日志检索对话的历史条件：\n")
	for i, turn := range history {
		fmt.Fprintf(&b, "%d. 用户：%s\n   解析：级别=%s 关键词=%s 时间=%s\n",
			i+1, turn.Query, turn.Parsed.Level, turn.Parsed.Key, turn.Parsed.TimeRangeDetail)
	}
	b.WriteString("\n当前用户输入：")
	b.WriteString(current)
	b.WriteString("\n\n请补全当前输入中的指代内容，输出一句完整的检索意图（不要 JSON，不要解释）。")

	ctx, cancel := context.WithTimeout(context.Background(), corefLLMTimeout)
	defer cancel()

	out, _, err := llm.CallWithUsage(ctx, "你是日志检索意图补全助手。", b.String())
	if err != nil {
		log.Printf("[coref] LLM 消解失败: %v", err)
		return "", false
	}

	resolved := strings.TrimSpace(out)
	if resolved == "" || len(resolved) > maxResolvedQueryLen {
		return "", false
	}
	return resolved, true
}

// buildResolvedQuery 根据解析结果构造可读的完整查询描述。
func buildResolvedQuery(p Nl2DslParseResp) string {
	var parts []string
	if p.Level != "" {
		parts = append(parts, p.Level)
	}
	if hasUsableKeyword(p.Key) {
		parts = append(parts, fmt.Sprintf("关键词「%s」", p.Key))
	}
	if p.TimeRangeDetail != "" {
		parts = append(parts, "时间范围 "+p.TimeRangeDetail)
	} else if p.StartTime != "" {
		parts = append(parts, fmt.Sprintf("时间 %s~%s", p.StartTime, p.EndTime))
	}
	if len(parts) == 0 {
		return "全量日志"
	}
	return strings.Join(parts, "，")
}

// BuildExplanationCard 构造解释卡（FR-505）。
func BuildExplanationCard(original string, parsed Nl2DslParseResp, strategy string, warnings []string) ExplanationCard {
	card := ExplanationCard{
		Original:   original,
		Resolved:   buildResolvedQuery(parsed),
		Strategy:   strategy,
		Confidence: parsed.Confidence,
		Conditions: make([]ExplanationCondition, 0, 4),
		Warnings:   warnings,
	}

	if parsed.Level != "" {
		card.Conditions = append(card.Conditions, ExplanationCondition{Field: "level", Value: parsed.Level, Source: strategy})
	}
	if hasUsableKeyword(parsed.Key) {
		card.Conditions = append(card.Conditions, ExplanationCondition{Field: "key", Value: parsed.Key, Source: strategy})
	}
	if parsed.TimeRangeDetail != "" {
		card.Conditions = append(card.Conditions, ExplanationCondition{Field: "timeRange", Value: parsed.TimeRangeDetail, Source: strategy})
	} else if parsed.StartTime != "" {
		card.Conditions = append(card.Conditions, ExplanationCondition{
			Field:  "timeRange",
			Value:  parsed.StartTime + "~" + parsed.EndTime,
			Source: strategy,
		})
	}
	if parsed.IntentType != "" {
		card.Conditions = append(card.Conditions, ExplanationCondition{Field: "intent", Value: parsed.IntentType, Source: "rule"})
	}

	// 低置信与降级场景给出明确提示
	if strategy == "degraded" {
		card.Warnings = append(card.Warnings, "语义解析降级：建议补充日志级别或关键词")
	}
	if card.Confidence > 0 && card.Confidence < nl2dslMinRuleConfidence {
		card.Warnings = append(card.Warnings, "解析置信度较低，结果可能不准确")
	}

	return card
}

// trimContextTurns 裁剪历史轮次到上限（保留最近的）。
func trimContextTurns(history []ConversationTurn) []ConversationTurn {
	if len(history) <= maxContextTurns {
		return history
	}
	return history[len(history)-maxContextTurns:]
}
