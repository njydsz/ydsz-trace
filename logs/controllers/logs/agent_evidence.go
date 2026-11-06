// Package logs 子文件：证据引用与三段式结论（V2.3-06，FR-904）。
//
// 背景：
//   Agent 的结论若无可追溯的证据，用户无法验证其正确性。
//   PRD FR-904 要求「结论必须可回溯原文」——每条结论都要能点回原始日志。
//
// 三段式结构（假设 / 验证 / 结论）：
//
//	1. 假设（Hypothesis）  ：基于工具结果推断的可能根因
//	2. 验证（Verification）：支撑/反驳该假设的证据与数据
//	3. 结论（Conclusion）  ：最终判断 + 建议动作
//
// 证据引用模型：
//
//	EvidenceRef{
//	  Source   evidence | anomaly | template | trace | stats
//	  RefID    具体 ID（日志行号 / 模板 hash / 异常事件 ID / traceID）
//	  Summary  一句话摘要
//	  Quote    原文片段（可点击回看）
//	}
//
// 设计约束：
//   - 证据只引用「实际被检索到的数据」，不编造
//   - Quote 截断到合理长度，避免响应体过大
//   - 无 LLM 时用统计信息自动生成三段式（降级模式）
package logs

import (
	"fmt"
	"strings"
)

// 证据引用常量。
const (
	// maxEvidencePerSection 每个段落最多引用的证据条数
	maxEvidencePerSection = 5
	// maxQuoteLength 单条证据的原文截断长度
	maxQuoteLength = 200
)

// evidenceSource 证据来源类型。
type evidenceSource string

const (
	// EvidenceSourceLog 原始日志
	EvidenceSourceLog evidenceSource = "log"
	// EvidenceSourceAnomaly 异常事件
	EvidenceSourceAnomaly evidenceSource = "anomaly"
	// EvidenceSourceTemplate 日志模板
	EvidenceSourceTemplate evidenceSource = "template"
	// EvidenceSourceTrace 链路追踪
	EvidenceSourceTrace evidenceSource = "trace"
	// EvidenceSourceStats 统计数据
	EvidenceSourceStats evidenceSource = "stats"
)

// ThreeSectionReport 三段式报告（V2.3-06）。
type ThreeSectionReport struct {
	// Hypothesis 假设段：可能的根因推断
	Hypothesis string `json:"hypothesis"`
	// HypothesisRefs 假设依据
	HypothesisRefs []EvidenceRef `json:"hypothesisRefs,omitempty"`
	// Verification 验证段：支撑或反驳的数据
	Verification string `json:"verification"`
	// VerificationRefs 验证证据
	VerificationRefs []EvidenceRef `json:"verificationRefs,omitempty"`
	// Conclusion 结论段：最终判断
	Conclusion string `json:"conclusion"`
	// ConclusionRefs 结论依据
	ConclusionRefs []EvidenceRef `json:"conclusionRefs,omitempty"`
	// Suggestions 建议动作清单
	Suggestions []string `json:"suggestions,omitempty"`
}

// EvidenceRef 单条证据引用（可点击回溯原文）。
type EvidenceRef struct {
	// Source 证据来源
	Source evidenceSource `json:"source"`
	// RefID 具体引用 ID（日志行号 / 模板 hash / 异常事件 ID / traceID）
	RefID string `json:"refId"`
	// Summary 一句话摘要
	Summary string `json:"summary"`
	// Quote 原文片段（截断到 maxQuoteLength）
	Quote string `json:"quote,omitempty"`
	// Time 发生时间（可解析时填充）
	Time string `json:"time,omitempty"`
}

// buildEvidenceFromSearch 从日志检索结果构造证据引用。
func buildEvidenceFromSearch(result *searchLogsResult) []EvidenceRef {
	if result == nil || len(result.Lines) == 0 {
		return nil
	}

	refs := make([]EvidenceRef, 0, minInt(len(result.Lines), maxEvidencePerSection))
	for i, line := range result.Lines {
		if i >= maxEvidencePerSection {
			break
		}
		refs = append(refs, EvidenceRef{
			Source:  EvidenceSourceLog,
			RefID:   fmt.Sprintf("line-%d", i+1),
			Summary: truncateStr(line, 80),
			Quote:   truncateStr(line, maxQuoteLength),
		})
	}
	return refs
}

// buildEvidenceFromAnomaly 从异常事件构造证据引用。
func buildEvidenceFromAnomaly(anomalies []anomalyBrief) []EvidenceRef {
	if len(anomalies) == 0 {
		return nil
	}

	refs := make([]EvidenceRef, 0, minInt(len(anomalies), maxEvidencePerSection))
	for i, a := range anomalies {
		if i >= maxEvidencePerSection {
			break
		}
		refs = append(refs, EvidenceRef{
			Source:  EvidenceSourceAnomaly,
			RefID:   a.TemplateHash,
			Summary: fmt.Sprintf("%s (score=%.2f)", a.Type, a.Score),
			Quote:   truncateStr(a.Description, maxQuoteLength),
			Time:    a.Time,
		})
	}
	return refs
}

// buildEvidenceFromTemplate 从模板列表构造证据引用。
func buildEvidenceFromTemplate(templates []templateBrief) []EvidenceRef {
	if len(templates) == 0 {
		return nil
	}

	refs := make([]EvidenceRef, 0, minInt(len(templates), maxEvidencePerSection))
	for i, t := range templates {
		if i >= maxEvidencePerSection {
			break
		}
		refs = append(refs, EvidenceRef{
			Source:  EvidenceSourceTemplate,
			RefID:   t.TemplateHash,
			Summary: fmt.Sprintf("%s 命中 %d 次", t.Level, t.Count),
			Quote:   truncateStr(t.Sample, maxQuoteLength),
		})
	}
	return refs
}

// buildEvidenceFromStats 从聚合统计构造证据引用。
func buildEvidenceFromStats(stats *aggStatsResult) []EvidenceRef {
	if stats == nil || len(stats.TopPatterns) == 0 {
		return nil
	}

	// 取命中最多的高频模式作为关键证据
	maxIdx := 0
	for i, p := range stats.TopPatterns {
		if p.Count > stats.TopPatterns[maxIdx].Count {
			maxIdx = i
		}
	}

	p := stats.TopPatterns[maxIdx]
	return []EvidenceRef{{
		Source:  EvidenceSourceStats,
		RefID:   p.TemplateHash,
		Summary: fmt.Sprintf("高频模式命中 %d 次（%s）", p.Count, stats.TimeRange),
		Quote:   truncateStr(p.Sample, maxQuoteLength),
	}}
}

// buildEvidenceFromContext 从上下文回溯构造证据引用。
func buildEvidenceFromContext(lines []string) []EvidenceRef {
	if len(lines) == 0 {
		return nil
	}
	refs := make([]EvidenceRef, 0, minInt(len(lines), maxEvidencePerSection))
	for i, line := range lines {
		if i >= maxEvidencePerSection {
			break
		}
		refs = append(refs, EvidenceRef{
			Source:  EvidenceSourceLog,
			RefID:   fmt.Sprintf("ctx-%d", i+1),
			Summary: truncateStr(line, 80),
			Quote:   truncateStr(line, maxQuoteLength),
		})
	}
	return refs
}

// collectAllEvidence 汇总全部工具调用的证据引用。
func collectAllEvidence(result *ReasoningResult) []EvidenceRef {
	if result == nil {
		return nil
	}

	all := make([]EvidenceRef, 0, maxEvidencePerSection*2)
	for _, call := range result.ToolCalls {
		all = append(all, extractEvidenceFromToolCall(call)...)
	}
	return all
}

// extractEvidenceFromToolCall 从单条工具调用记录提取证据。
//
// 工具结果以 ResultSummary 形式存储（已截断），这里将其转为证据引用。
// 完整结果未持久化，因此 Summary 即 Quote。
func extractEvidenceFromToolCall(call ToolCallRecord) []EvidenceRef {
	if call.ResultSummary == "" {
		return nil
	}
	return []EvidenceRef{{
		Source:  sourceForTool(call.Tool),
		RefID:   fmt.Sprintf("step-%d", call.Step),
		Summary: truncateStr(call.ResultSummary, 80),
		Quote:   truncateStr(call.ResultSummary, maxQuoteLength),
	}}
}

// sourceForTool 把工具名映射为证据来源。
func sourceForTool(tool string) evidenceSource {
	switch tool {
	case "search_logs", "get_context":
		return EvidenceSourceLog
	case "get_anomalies":
		return EvidenceSourceAnomaly
	case "list_templates":
		return EvidenceSourceTemplate
	case "get_trace":
		return EvidenceSourceTrace
	case "agg_stats", "diff_time_window":
		return EvidenceSourceStats
	}
	return EvidenceSourceStats
}

// buildThreeSectionReport 从 Agent 结果构造三段式报告。
//
// LLM 可用时由模型生成三段内容；此处提供基于证据的兜底构造，
// 保证降级模式下结论同样结构化。
func buildThreeSectionReport(result *ReasoningResult, allEvidence []EvidenceRef) *ThreeSectionReport {
	if result == nil {
		return nil
	}

	report := &ThreeSectionReport{
		Hypothesis:       buildHypothesis(result),
		Verification:     buildVerification(result, allEvidence),
		Conclusion:       result.Conclusion.RootCause,
		Suggestions:      collectSuggestions(result),
		HypothesisRefs:   pickRefs(allEvidence, EvidenceSourceAnomaly, EvidenceSourceStats),
		VerificationRefs: pickRefs(allEvidence, EvidenceSourceLog, EvidenceSourceTemplate),
		ConclusionRefs:   allEvidence,
	}

	// 限制引用数量，避免响应体过大
	if len(report.ConclusionRefs) > maxEvidencePerSection {
		report.ConclusionRefs = report.ConclusionRefs[:maxEvidencePerSection]
	}

	return report
}

// buildHypothesis 基于工具调用推断假设段。
func buildHypothesis(result *ReasoningResult) string {
	if len(result.ToolCalls) == 0 {
		return "未执行任何工具，无法形成假设"
	}

	// 优先使用异常事件作为假设依据
	for _, call := range result.ToolCalls {
		if call.Tool == "get_anomalies" && call.Success {
			return "检测到异常事件，疑似存在突增或新增的错误模式"
		}
	}
	for _, call := range result.ToolCalls {
		if call.Tool == "diff_time_window" && call.Success {
			return "时间窗对比显示模式分布变化，疑似有新增异常"
		}
	}
	return "基于检索结果推断可能的原因"
}

// buildVerification 构造验证段。
func buildVerification(result *ReasoningResult, evidence []EvidenceRef) string {
	if len(evidence) == 0 {
		return "无可用证据"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "共采集 %d 条证据，来自 %d 次工具调用", len(evidence), len(result.ToolCalls))

	// 统计各来源数量
	counts := make(map[evidenceSource]int)
	for _, e := range evidence {
		counts[e.Source]++
	}
	if n := counts[EvidenceSourceLog]; n > 0 {
		fmt.Fprintf(&b, "（日志 %d 条", n)
	}
	if n := counts[EvidenceSourceAnomaly]; n > 0 {
		fmt.Fprintf(&b, "，异常 %d 条", n)
	}
	if n := counts[EvidenceSourceTemplate]; n > 0 {
		fmt.Fprintf(&b, "，模板 %d 条", n)
	}
	if len(counts) > 0 {
		b.WriteString("）")
	}
	return b.String()
}

// collectSuggestions 从既有推理结果中提取建议动作。
func collectSuggestions(result *ReasoningResult) []string {
	if result == nil || result.Conclusion.Suggestions == nil {
		return nil
	}
	return result.Conclusion.Suggestions
}

// pickRefs 按来源筛选证据。
func pickRefs(evidence []EvidenceRef, sources ...evidenceSource) []EvidenceRef {
	if len(evidence) == 0 {
		return nil
	}

	want := make(map[evidenceSource]bool, len(sources))
	for _, s := range sources {
		want[s] = true
	}

	out := make([]EvidenceRef, 0, maxEvidencePerSection)
	for _, e := range evidence {
		if want[e.Source] {
			out = append(out, e)
			if len(out) >= maxEvidencePerSection {
				break
			}
		}
	}
	return out
}

// minInt 返回较小值（避免引入 math 包）。
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// attachEvidenceAndReport 为 Agent 结果附加证据引用与三段式报告。
//
// 统一在此收口，避免三条返回路径（ReAct 正常结束 / 预算耗尽 / 降级）
// 各自重复构造逻辑。
func attachEvidenceAndReport(result *ReasoningResult) {
	result.TraceableRefs = collectAllEvidence(result)
	result.ThreeSection = buildThreeSectionReport(result, result.TraceableRefs)
}
