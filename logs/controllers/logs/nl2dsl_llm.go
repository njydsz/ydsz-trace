// Package logs 子文件：NL2DSL 的 LLM 兜底（V2.3-04，FR-504/FR-506）。
//
// 背景：
//   规则解析器对规范表达（"近 3 小时 ERROR 日志"）准确率高、零成本；
//   但对口语化/复合表达（"昨天下午服务挂了的原因"）容易漏提取。
//   规则失败或低置信时，调用 LLM 做兜底解析。
//
// 三级降级策略（FR-504）：
//
//	规则解析成功（置信度足够） → 直接使用
//	规则解析失败/低置信      → LLM 解析
//	LLM 不可用或失败          → 关键词 + 语义检索降级（不报错给用户）
//
// 安全约束：
//   - LLM 仅做「结构化抽取」，不接触日志原文（避免数据外发，FR-2）
//   - 仅把用户的自然语言 query 发给模型，且限制输出 schema
//   - 超时 10s，超时立即降级，不阻塞用户请求
package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// NL2DSL LLM 相关常量。
const (
	// nl2dslLLMTimeout LLM 兜底调用超时（秒）
	nl2dslLLMTimeout = 10 * time.Second
	// nl2dslMinRuleConfidence 规则解析的最小置信度；低于此值触发 LLM 兜底
	nl2dslMinRuleConfidence = 0.5
)

// nl2dslLLMResponse LLM 返回的结构化解析结果。
type nl2dslLLMResponse struct {
	Level     string `json:"level"`
	Date      string `json:"date"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	Key       string `json:"key"`
	TimeRange string `json:"timeRangeDetail"`
	Intent    string `json:"intentType"`
}

// nl2dslConfidence 评估规则解析结果的置信度。
//
// 评估维度（满分 1.0）：
//   - 提取到级别 +0.25
//   - 提取到有效关键词 +0.35
//   - 解析到显式时间范围 +0.40（startTime/endTime/timeRangeDetail 任一）
//
// 关键约束：Date 字段在默认路径下总会被填充为「今天」，因此**不计入**置信度，
// 否则任何输入都会拿到保底 0.3 分，导致 LLM 兜底几乎不会被触发。
//
// 意图为 general 时不额外扣分——只提取到关键词的查询同样可信。
func nl2dslConfidence(resp Nl2DslParseResp) float64 {
	score := 0.0
	if resp.Level != "" {
		score += 0.25
	}
	if hasUsableKeyword(resp.Key) {
		score += 0.35
	}
	if resp.StartTime != "" || resp.EndTime != "" || resp.TimeRangeDetail != "" {
		score += 0.40
	}

	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score
}

// hasUsableKeyword 判断关键词是否为有效检索词。
//
// 过滤掉纯噪声（过短、纯数字、纯标点），避免把「查一下」这类
// 清理后残留的字符当作有效提取。
func hasUsableKeyword(key string) bool {
	k := trimSpace(key)
	if len([]rune(k)) < 2 {
		return false
	}
	// 至少包含一个字母或汉字
	for _, r := range k {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r >= 0x4E00 {
			return true
		}
	}
	return false
}

// nl2dslParseWithFallback 带 LLM 兜底的 NL2DSL 解析（V2.3-04）。
//
// 返回解析结果与使用的策略（rule / llm / degraded），供前端展示解释卡。
func nl2dslParseWithFallback(query string) (Nl2DslParseResp, string) {
	// 路径 1：规则解析
	ruleResp := nl2dslBuildResponse(query)
	confidence := nl2dslConfidence(ruleResp)
	ruleResp.Confidence = confidence
	if confidence >= nl2dslMinRuleConfidence {
		return ruleResp, "rule"
	}

	// 路径 2：LLM 兜底
	if llmResp, ok := nl2dslLLMParse(query); ok {
		// 合并：LLM 结果为主，规则结果补充空字段
		merged := nl2dslMerge(ruleResp, llmResp)
		return merged, "llm"
	}

	// 路径 3：降级（保留规则结果，标记低置信）
	log.Printf("[nl2dsl] 规则置信度 %.2f 低于阈值且 LLM 不可用，降级为关键词检索", confidence)
	return ruleResp, "degraded"
}

// nl2dslLLMParse 调用 LLM 做结构化抽取。
//
// 成功返回 (结果, true)；LLM 不可用/超时/解析失败返回 (_, false)。
func nl2dslLLMParse(query string) (nl2dslLLMResponse, bool) {
	llm := NewLLMClient()
	if !llm.Available() {
		return nl2dslLLMResponse{}, false
	}

	systemPrompt := buildNL2DSLSystemPrompt()
	userPrompt := fmt.Sprintf("请解析以下检索意图：\n\n%s", query)

	ctx, cancel := context.WithTimeout(context.Background(), nl2dslLLMTimeout)
	defer cancel()

	output, _, err := llm.CallWithUsage(ctx, systemPrompt, userPrompt)
	if err != nil {
		log.Printf("[nl2dsl] LLM 调用失败: %v", err)
		return nl2dslLLMResponse{}, false
	}

	parsed, ok := parseNL2DSLJSON(output)
	if !ok {
		log.Printf("[nl2dsl] LLM 输出解析失败: %s", truncateStr(output, 120))
		return nl2dslLLMResponse{}, false
	}
	return parsed, true
}

// buildNL2DSLSystemPrompt 构造 NL2DSL 抽取用的 system prompt。
//
// 关键约束：只输出 JSON，不解释；不索取日志原文。
func buildNL2DSLSystemPrompt() string {
	var b strings.Builder
	b.WriteString("你是日志检索意图解析器。把用户的自然语言检索意图抽取为结构化 JSON。\n")
	b.WriteString("只输出 JSON，不要任何解释文字或 markdown 代码块标记。\n\n")
	b.WriteString("输出字段：\n")
	b.WriteString("- level: 日志级别，取值 ERROR/WARN/INFO/DEBUG/FATAL 或空字符串\n")
	b.WriteString("- date: 日期，格式 YYYYMMDD，无法确定留空\n")
	b.WriteString("- startTime: 起始时间，格式 HH:MM:SS，无法确定留空\n")
	b.WriteString("- endTime: 结束时间，格式 HH:MM:SS，无法确定留空\n")
	b.WriteString("- key: 检索关键词（引号内文本优先），无则留空\n")
	b.WriteString("- timeRangeDetail: 人类可读的时间范围描述\n")
	b.WriteString("- intentType: 意图类型，取值 error_search/time_range/keyword_search/general\n\n")
	b.WriteString("示例：\n")
	b.WriteString(`输入："近3小时的ERROR日志"
输出：{"level":"ERROR","date":"","startTime":"","endTime":"","key":"","timeRangeDetail":"近3小时","intentType":"error_search"}` + "\n")
	b.WriteString(`输入："查一下 timeout 相关的日志"
输出：{"level":"","date":"","startTime":"","endTime":"","key":"timeout","timeRangeDetail":"","intentType":"keyword_search"}` + "\n")
	return b.String()
}

// parseNL2DSLJSON 从 LLM 输出中解析 JSON（容忍 markdown 代码块）。
func parseNL2DSLJSON(output string) (nl2dslLLMResponse, bool) {
	text := strings.TrimSpace(output)

	// 去除 markdown 代码块标记
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimPrefix(text, "```")
		if idx := strings.LastIndex(text, "```"); idx >= 0 {
			text = text[:idx]
		}
		text = strings.TrimSpace(text)
	}

	// 截取第一个完整 JSON 对象（防止模型附加说明文字）
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start < 0 || end <= start {
		return nl2dslLLMResponse{}, false
	}
	text = text[start : end+1]

	var resp nl2dslLLMResponse
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		return nl2dslLLMResponse{}, false
	}
	return resp, true
}

// nl2dslMerge 合并规则结果与 LLM 结果（LLM 优先，规则补空）。
func nl2dslMerge(rule Nl2DslParseResp, llm nl2dslLLMResponse) Nl2DslParseResp {
	out := Nl2DslParseResp{
		UserIntent:      rule.UserIntent,
		Level:           rule.Level,
		Date:            rule.Date,
		StartTime:       rule.StartTime,
		EndTime:         rule.EndTime,
		Key:             rule.Key,
		TimeRangeDetail: rule.TimeRangeDetail,
		IntentType:      rule.IntentType,
	}

	if llm.Level != "" {
		out.Level = llm.Level
	}
	if llm.Date != "" {
		out.Date = llm.Date
	}
	if llm.StartTime != "" {
		out.StartTime = llm.StartTime
	}
	if llm.EndTime != "" {
		out.EndTime = llm.EndTime
	}
	if llm.Key != "" {
		out.Key = llm.Key
	}
	if llm.TimeRange != "" {
		out.TimeRangeDetail = llm.TimeRange
	}
	if llm.Intent != "" && llm.Intent != INTENT_GENERAL {
		out.IntentType = llm.Intent
	}

	// 重新生成 DSL 条件表达式，保证与合并后的字段一致
	out.DslConditions = nl2dslFormatConditions(out.Level, out.Key, out.Date, out.StartTime, out.EndTime)
	return out
}
