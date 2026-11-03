// Package logs 子文件：根因分析三段式输出结构（FR-901~908）。
//
// 根因分析结果采用三段式结构：
//   1. HypothesisSection — 根因假设（1-3 条）
//   2. EvidenceSection   — 验证证据（引用模板 hash + 行号 + 统计数值）
//   3. ConclusionSection — 根因结论 + 修复建议 + 置信度
//
// 该结构是 LLM 生成与降级统计两条路径的共同输出契约。
package logs

// ReasoningResult 根因分析整体结果（三段式 + 元信息）。
type ReasoningResult struct {
	// Query 用户原始问题
	Query string `json:"query"`
	// ConvID 关联的会话 ID（可为空）
	ConvID string `json:"convId"`
	// Hypotheses 根因假设（1-3 条）
	Hypotheses []HypothesisSection `json:"hypotheses"`
	// Evidence 验证证据（引用模板 hash + 行号 + 统计数值）
	Evidence []EvidenceSection `json:"evidence"`
	// Conclusion 根因结论 + 修复建议 + 置信度
	Conclusion ConclusionSection `json:"conclusion"`
	// Steps 实际执行的推理步数
	Steps int `json:"steps"`
	// ToolCalls 本次推理过程中调用的工具记录（用于审计与前端展示）
	ToolCalls []ToolCallRecord `json:"toolCalls"`
	// Degraded 是否为降级模式（LLM 不可用时基于统计直接生成）
	Degraded bool `json:"degraded"`
	// ModelName 使用的模型名称（降级模式为空字符串）
	ModelName string `json:"modelName"`
}

// HypothesisSection 根因假设。
type HypothesisSection struct {
	// Index 假设序号（1-based）
	Index int `json:"index"`
	// Statement 假设陈述
	Statement string `json:"statement"`
	// Confidence 该假设的置信度（0.0-1.0）
	Confidence float64 `json:"confidence"`
}

// EvidenceSection 验证证据：引用模板 hash + 行号 + 统计数值。
type EvidenceSection struct {
	// TemplateHash 关联的模板 hash（fnv32a）
	TemplateHash string `json:"templateHash"`
	// Template 模板串（参数位用 <*> 占位）
	Template string `json:"template"`
	// LineNo 命中行号（来自上下文回溯；未知时为 0）
	LineNo int `json:"lineNo"`
	// SampleLine 示例日志行（脱敏后）
	SampleLine string `json:"sampleLine"`
	// Count 该模板在时间窗内的命中次数
	Count int `json:"count"`
	// BaselineMean 基线均值（统计数值，用于佐证异常）
	BaselineMean float64 `json:"baselineMean"`
	// ZScore 异常 Z-Score（统计数值）
	ZScore float64 `json:"zScore"`
	// Source 证据来源工具名（search_logs / agg_stats / get_context / get_trace）
	Source string `json:"source"`
}

// ConclusionSection 根因结论 + 修复建议 + 置信度。
type ConclusionSection struct {
	// RootCause 根因结论文本
	RootCause string `json:"rootCause"`
	// Suggestions 修复建议（按优先级排序）
	Suggestions []string `json:"suggestions"`
	// Confidence 整体置信度（0.0-1.0）
	Confidence float64 `json:"confidence"`
}

// ToolCallRecord 单次工具调用记录（用于 ReAct 历史与审计）。
type ToolCallRecord struct {
	// Step 推理步序号（1-based）
	Step int `json:"step"`
	// Tool 工具名
	Tool string `json:"tool"`
	// Params 调用参数（JSON 序列化后的 map）
	Params map[string]interface{} `json:"params"`
	// ResultSummary 结果摘要（截断，避免 prompt 过长）
	ResultSummary string `json:"resultSummary"`
	// DurationMs 工具执行耗时（毫秒）
	DurationMs int64 `json:"durationMs"`
}
