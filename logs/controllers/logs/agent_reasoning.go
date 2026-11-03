// Package logs 子文件：根因分析 ReAct 编排器（FR-903~905）。
//
// 实现 ReAct 循环：
//  1. 每步构造 prompt（含工具定义 + 历史步骤 + 用户问题）
//  2. 调用 LLM（优先 chat 模型）
//  3. 解析工具调用 JSON
//  4. 执行工具（5s 超时）
//  5. 追加结果到历史
//  6. LLM 返回 final_answer 或达到 maxSteps 时结束
//
// LLM 不可用时降级为基于 AnomalyEngine + TopPatterns 的统计结论，不报错。
package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/stats"
)

// ReasoningAgent 根因分析 ReAct 编排器。
type ReasoningAgent struct {
	// tools 可用工具集（key = 工具名）
	tools map[string]AgentTool
	// maxSteps 最大推理步数
	maxSteps int
	// llmClient LLM 客户端（不可用时为 nil 或 model 为 nil）
	llmClient *LLMClient
	// anomalyEngine 异常检测引擎（降级模式使用）
	anomalyEngine *stats.AnomalyEngine
}

// NewReasoningAgent 构造 ReasoningAgent，注册 4 个只读工具。
func NewReasoningAgent() *ReasoningAgent {
	tools := map[string]AgentTool{
		(&SearchLogsTool{}).Name():  &SearchLogsTool{},
		(&AggStatsTool{}).Name():    &AggStatsTool{},
		(&GetContextTool{}).Name():  &GetContextTool{},
		(&GetTraceTool{}).Name():    &GetTraceTool{},
	}
	return &ReasoningAgent{
		tools:         tools,
		maxSteps:      agentMaxSteps,
		llmClient:     NewLLMClient(),
		anomalyEngine: stats.NewAnomalyEngine(),
	}
}

// reactStep 单步推理的历史记录。
type reactStep struct {
	// Thought LLM 的思考过程
	Thought string `json:"thought"`
	// Action 工具调用（工具名 + 参数），final_answer 时为 nil
	Action *reactAction `json:"action,omitempty"`
	// Observation 工具执行结果（JSON 序列化字符串）
	Observation string `json:"observation,omitempty"`
	// FinalAnswer 若 LLM 给出最终答案，则填充此字段
	FinalAnswer string `json:"finalAnswer,omitempty"`
}

// reactAction LLM 请求的工具调用。
type reactAction struct {
	// Tool 工具名
	Tool string `json:"tool"`
	// Params 工具参数
	Params map[string]interface{} `json:"params"`
}

// Run 执行 ReAct 根因分析循环。
//
// query 为用户问题，convID 为可选会话 ID（用于消息持久化）。
func (a *ReasoningAgent) Run(ctx context.Context, query string, convID string) (*ReasoningResult, error) {
	result := &ReasoningResult{
		Query:    query,
		ConvID:   convID,
		Steps:    0,
		ToolCalls: []ToolCallRecord{},
	}

	// 路径 1：LLM 可用 → ReAct 循环
	if a.llmClient.Available() {
		result.Degraded = false
		result.ModelName = a.llmClient.ModelName()
		a.runReAct(ctx, query, result)
		return result, nil
	}

	// 路径 2：LLM 不可用 → 降级为统计结论
	log.Printf("[agent] LLM 不可用，降级为统计结论模式")
	result.Degraded = true
	result.ModelName = ""
	a.runDegraded(ctx, query, result)
	return result, nil
}

// runReAct 执行 ReAct 循环。
func (a *ReasoningAgent) runReAct(ctx context.Context, query string, result *ReasoningResult) {
	history := make([]reactStep, 0, a.maxSteps)

	for step := 1; step <= a.maxSteps; step++ {
		// 构造 prompt
		systemPrompt := buildAgentSystemPrompt()
		userPrompt := buildAgentUserPrompt(query, history)

		// 调用 LLM（带超时）
		callCtx, cancel := context.WithTimeout(ctx, agentLLMTimeout)
		output, err := a.llmClient.Call(callCtx, systemPrompt, userPrompt)
		cancel()
		if err != nil {
			log.Printf("[agent] LLM 调用失败（step=%d）: %v", step, err)
			// 首次即失败则降级为统计结论
			if step == 1 {
				result.Degraded = true
				a.runDegraded(ctx, query, result)
				return
			}
			// 后续步骤失败：用已有历史直接生成结论
			a.synthesizeFromHistory(query, history, result)
			return
		}

		// 解析 LLM 输出
		stepRecord, finalAnswer := parseLLMOutput(output)
		history = append(history, stepRecord)
		result.Steps = step

		// 若 LLM 给出 final_answer，结束循环
		if finalAnswer != "" {
			parseFinalAnswer(finalAnswer, result)
			return
		}

		// 执行工具调用
		if stepRecord.Action == nil {
			// LLM 未给出工具调用也未给出最终答案，结束
			a.synthesizeFromHistory(query, history, result)
			return
		}

		toolName := stepRecord.Action.Tool
		tool, ok := a.tools[toolName]
		if !ok {
			// 工具不存在：追加错误观察，让 LLM 下一轮纠正
			stepRecord.Observation = fmt.Sprintf(`{"error":"未知工具: %s"}`, toolName)
			// 替换 history 最后一项（含 observation）
			history[len(history)-1] = stepRecord
			continue
		}

		// 执行工具（5s 超时）
		start := time.Now()
		toolCtx, toolCancel := context.WithTimeout(ctx, agentToolTimeout)
		toolResult, toolErr := tool.Execute(toolCtx, stepRecord.Action.Params)
		toolCancel()
		durationMs := time.Since(start).Milliseconds()

		var obsStr string
		if toolErr != nil {
			obsStr = fmt.Sprintf(`{"error":%q}`, toolErr.Error())
		} else {
			jsonBytes, mErr := json.Marshal(toolResult)
			if mErr != nil {
				obsStr = fmt.Sprintf(`{"error":"序列化结果失败: %s"}`, mErr.Error())
			} else {
				obsStr = string(jsonBytes)
			}
		}

		stepRecord.Observation = obsStr
		history[len(history)-1] = stepRecord

		result.ToolCalls = append(result.ToolCalls, ToolCallRecord{
			Step:          step,
			Tool:          toolName,
			Params:        stepRecord.Action.Params,
			ResultSummary: truncateStr(obsStr, 200),
			DurationMs:    durationMs,
		})
	}

	// 达到最大步数：基于历史生成结论
	a.synthesizeFromHistory(query, history, result)
}

// runDegraded 降级模式：基于 AnomalyEngine + TopPatterns 直接生成统计结论。
//
// 该模式不依赖 LLM，保证根因分析接口始终可用。
func (a *ReasoningAgent) runDegraded(ctx context.Context, query string, result *ReasoningResult) {
	// 从 query 中推断 itemID（前端默认传 1；生产环境应由 Agent 工具调用确定）
	itemID := inferItemIDFromQuery(query)

	// DB 未初始化时直接走空数据分支，不调用 TopPatterns
	if models.DB == nil {
		result.Hypotheses = []HypothesisSection{
			{Index: 1, Statement: "当前无可用的模式统计数据（数据库未初始化），建议先执行日志检索确认问题现象。", Confidence: 0.3},
		}
		result.Evidence = []EvidenceSection{}
		result.Conclusion = ConclusionSection{
			RootCause:   "数据不足，无法给出确定性根因。",
			Suggestions: []string{"检查数据库连接", "缩小时间范围后重试"},
			Confidence:  0.2,
		}
		result.Steps = 1
		return
	}

	patterns, err := models.TopPatterns(itemID, "1h", 10)
	if err != nil || len(patterns) == 0 {
		// 无统计数据：给出空结论但不报错
		result.Hypotheses = []HypothesisSection{
			{Index: 1, Statement: "当前时间窗内无可用的模式统计数据，建议先执行日志检索确认问题现象。", Confidence: 0.3},
		}
		result.Evidence = []EvidenceSection{}
		result.Conclusion = ConclusionSection{
			RootCause:   "数据不足，无法给出确定性根因。",
			Suggestions: []string{"缩小时间范围后重试", "检查日志采集是否正常"},
			Confidence:  0.2,
		}
		return
	}

	// 构造假设与证据
	hypotheses := make([]HypothesisSection, 0, 3)
	evidence := make([]EvidenceSection, 0, len(patterns))

	for i, p := range patterns {
		if i >= 3 {
			break
		}
		level := extractLevel(p.Sample)
		statement := fmt.Sprintf("高频模板 %s（命中 %d 次，级别=%s）可能是主要异常来源", p.TemplateHash, p.Count, level)
		confidence := 0.5
		if level == "ERROR" || level == "FATAL" {
			confidence = 0.7
		}
		hypotheses = append(hypotheses, HypothesisSection{
			Index:      i + 1,
			Statement:  statement,
			Confidence: confidence,
		})

		evidence = append(evidence, EvidenceSection{
			TemplateHash: p.TemplateHash,
			Template:     truncateStr(p.Sample, 120),
			SampleLine:   truncateStr(p.Sample, 200),
			Count:        p.Count,
			Source:       "agg_stats",
		})
	}

	// 结论
	rootCause := fmt.Sprintf("根据近 1 小时模式统计，Top1 模板（%s）命中 %d 次，是主要异常来源。", patterns[0].TemplateHash, patterns[0].Count)
	suggestions := buildSuggestions(patterns)
	conclusion := ConclusionSection{
		RootCause:   rootCause,
		Suggestions: suggestions,
		Confidence:  0.6,
	}

	result.Hypotheses = hypotheses
	result.Evidence = evidence
	result.Conclusion = conclusion
	result.Steps = 1
}

// synthesizeFromHistory 基于已有 ReAct 历史生成结论（LLM 不可用或达到最大步数时）。
func (a *ReasoningAgent) synthesizeFromHistory(query string, history []reactStep, result *ReasoningResult) {
	// 收集工具观察结果中的证据
	evidence := collectEvidenceFromHistory(history)

	// 构造假设（基于工具调用类型）
	hypotheses := make([]HypothesisSection, 0, 3)
	for i, s := range history {
		if i >= 3 {
			break
		}
		thought := s.Thought
		if thought == "" && s.Action != nil {
			thought = fmt.Sprintf("调用工具 %s 进行排查", s.Action.Tool)
		}
		if thought == "" {
			thought = "已执行相关检索"
		}
		hypotheses = append(hypotheses, HypothesisSection{
			Index:      i + 1,
			Statement:  truncateStr(thought, 200),
			Confidence: 0.5,
		})
	}
	if len(hypotheses) == 0 {
		hypotheses = append(hypotheses, HypothesisSection{
			Index:      1,
			Statement:  "已执行日志检索与统计分析，未发现明确异常模式。",
			Confidence: 0.3,
		})
	}

	// 结论
	conclusion := ConclusionSection{
		RootCause:   fmt.Sprintf("基于 %d 步工具调用结果完成初步排查。", len(history)),
		Suggestions: []string{"查看上方证据引用以定位具体模板", "结合链路追踪进一步确认跨节点影响"},
		Confidence:  0.5,
	}

	result.Hypotheses = hypotheses
	result.Evidence = evidence
	result.Conclusion = conclusion
}

// ============================================================================
// Prompt 构造
// ============================================================================

// buildAgentSystemPrompt 构造系统提示词（含工具定义）。
func buildAgentSystemPrompt() string {
	var b strings.Builder
	b.WriteString("你是云顶追踪的日志根因分析专家。你的任务是通过调用只读工具检索日志、统计与链路信息，")
	b.WriteString("形成根因假设并用证据验证，最终给出三段式结论（假设/证据/结论）。\n\n")
	b.WriteString("可用工具（JSON 格式调用）：\n")
	b.WriteString("{\"tool\": \"<name>\", \"params\": {...}}\n\n")

	// 注入工具定义
	tools := []AgentTool{
		&SearchLogsTool{},
		&AggStatsTool{},
		&GetContextTool{},
		&GetTraceTool{},
	}
	for _, t := range tools {
		fmt.Fprintf(&b, "- %s: %s\n", t.Name(), t.Description())
	}

	b.WriteString("\n输出格式要求：\n")
	b.WriteString("1. 若需要调用工具，仅输出 JSON：{\"thought\":\"...\",\"action\":{\"tool\":\"...\",\"params\":{...}}}\n")
	b.WriteString("2. 若已有足够信息，输出：{\"thought\":\"...\",\"finalAnswer\":\"...\"}，")
	b.WriteString("finalAnswer 需包含三段式内容（hypotheses/evidence/conclusion）。\n")
	b.WriteString("3. 每次只输出一个 JSON 对象，不要附加其他文字。\n")
	return b.String()
}

// buildAgentUserPrompt 构造用户提示词（含历史步骤 + 用户问题）。
func buildAgentUserPrompt(query string, history []reactStep) string {
	var b strings.Builder
	fmt.Fprintf(&b, "用户问题：%s\n\n", query)
	if len(history) > 0 {
		b.WriteString("历史推理步骤：\n")
		for i, s := range history {
			fmt.Fprintf(&b, "[Step %d]\n", i+1)
			if s.Thought != "" {
				fmt.Fprintf(&b, "Thought: %s\n", s.Thought)
			}
			if s.Action != nil {
				paramsJSON, _ := json.Marshal(s.Action.Params)
				fmt.Fprintf(&b, "Action: %s(%s)\n", s.Action.Tool, string(paramsJSON))
			}
			if s.Observation != "" {
				obs := truncateStr(s.Observation, 800)
				fmt.Fprintf(&b, "Observation: %s\n", obs)
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("请基于以上信息决定下一步动作（调用工具或给出 finalAnswer）。\n")
	return b.String()
}

// ============================================================================
// LLM 输出解析
// ============================================================================

// parseLLMOutput 解析 LLM 输出为 reactStep，并提取 finalAnswer（若有）。
func parseLLMOutput(output string) (reactStep, string) {
	output = strings.TrimSpace(output)

	// 尝试提取 ```json ... ``` 代码块
	if strings.Contains(output, "```") {
		output = extractJSONFromCodeBlock(output)
	}

	var step reactStep
	if err := json.Unmarshal([]byte(output), &step); err != nil {
		// 解析失败：将原始输出作为 finalAnswer 处理（容错）
		return reactStep{}, output
	}

	// 检查 finalAnswer
	if step.FinalAnswer != "" {
		return step, step.FinalAnswer
	}
	return step, ""
}

// extractJSONFromCodeBlock 从 markdown 代码块中提取 JSON。
func extractJSONFromCodeBlock(text string) string {
	start := strings.Index(text, "```json")
	if start < 0 {
		start = strings.Index(text, "```")
	}
	if start < 0 {
		return text
	}
	rest := text[start:]
	// 跳过 ```json 或 ```
	if idx := strings.Index(rest, "\n"); idx >= 0 {
		rest = rest[idx+1:]
	}
	end := strings.Index(rest, "```")
	if end < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}

// parseFinalAnswer 解析 finalAnswer 文本，填充三段式结构。
//
// 优先尝试 JSON 解析；失败则退化为文本提取。
func parseFinalAnswer(text string, result *ReasoningResult) {
	text = strings.TrimSpace(text)
	// 尝试提取 JSON 块
	var payload struct {
		Hypotheses []HypothesisSection `json:"hypotheses"`
		Evidence   []EvidenceSection   `json:"evidence"`
		Conclusion ConclusionSection   `json:"conclusion"`
	}
	if err := json.Unmarshal([]byte(extractJSONFromCodeBlock(text)), &payload); err == nil {
		result.Hypotheses = payload.Hypotheses
		result.Evidence = payload.Evidence
		result.Conclusion = payload.Conclusion
		return
	}

	// 退化：将 finalAnswer 作为结论文本
	result.Hypotheses = []HypothesisSection{
		{Index: 1, Statement: truncateStr(text, 200), Confidence: 0.5},
	}
	result.Evidence = []EvidenceSection{}
	result.Conclusion = ConclusionSection{
		RootCause:  truncateStr(text, 1000),
		Suggestions: []string{},
		Confidence: 0.4,
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

// collectEvidenceFromHistory 从 ReAct 历史中提取证据引用。
func collectEvidenceFromHistory(history []reactStep) []EvidenceSection {
	evidence := make([]EvidenceSection, 0, len(history))
	for _, s := range history {
		if s.Observation == "" {
			continue
		}
		// 尝试解析 observation JSON
		var obs map[string]interface{}
		if err := json.Unmarshal([]byte(s.Observation), &obs); err != nil {
			continue
		}

		// 根据不同工具结果提取证据
		source := ""
		if s.Action != nil {
			source = s.Action.Tool
		}

		// agg_stats 结果：提取 templateHash + count
		if tp, ok := obs["topPatterns"].([]interface{}); ok {
			for _, item := range tp {
				if m, ok := item.(map[string]interface{}); ok {
					hash := toString(m["templateHash"])
					count := toInt(m["count"], 0)
					sample := toString(m["sample"])
					evidence = append(evidence, EvidenceSection{
						TemplateHash: hash,
						Template:     truncateStr(sample, 120),
						SampleLine:   truncateStr(sample, 200),
						Count:        count,
						Source:       source,
					})
				}
			}
		}

		// get_context 结果：提取 refLine + before/after lines
		if refLine, ok := obs["refLine"].(string); ok && refLine != "" {
			evidence = append(evidence, EvidenceSection{
				Template:   truncateStr(refLine, 120),
				SampleLine: truncateStr(refLine, 200),
				Source:     source,
			})
		}
	}
	return evidence
}

// inferItemIDFromQuery 从 query 中推断 itemID。
//
// MVP 阶段使用启发式规则；生产环境应由前端传入或通过 NL2DSL 解析。
func inferItemIDFromQuery(query string) int64 {
	// DB 未初始化时返回默认值 1
	if models.DB == nil {
		return 1
	}
	// 默认使用第一个可用 item
	items, err := models.QueryAllItem()
	if err != nil || len(items) == 0 {
		return 1
	}
	return items[0].Id
}

// buildSuggestions 基于 TopPatterns 构造修复建议。
func buildSuggestions(patterns []models.PatternStat) []string {
	suggestions := make([]string, 0, 3)
	for i, p := range patterns {
		if i >= 3 {
			break
		}
		level := extractLevel(p.Sample)
		switch level {
		case "ERROR", "FATAL":
			suggestions = append(suggestions,
				fmt.Sprintf("优先排查 ERROR 模板 %s（命中 %d 次），结合上下文回溯确认触发条件", p.TemplateHash, p.Count))
		case "WARN":
			suggestions = append(suggestions,
				fmt.Sprintf("关注 WARN 模板 %s（命中 %d 次），可能存在潜在风险", p.TemplateHash, p.Count))
		default:
			suggestions = append(suggestions,
				fmt.Sprintf("分析模板 %s 的命中分布，确认是否为正常业务流量", p.TemplateHash))
		}
	}
	if len(suggestions) == 0 {
		suggestions = append(suggestions, "暂无明确建议，建议结合链路追踪进一步分析")
	}
	return suggestions
}
