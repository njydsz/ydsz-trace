// Package logs 子文件：纠正回流案例库（V2.3-08，FR-907）。
//
// 背景：
//   Agent 的结论不可能永远正确。PRD FR-907 要求「用户标记错结论后，
//   纠正内容进入案例库，后续相似会话可检索复用」。
//
// 闭环设计：
//
//	用户标记「结论有误」→ 填写纠正后的根因与方案
//	                → 写入 t_case_library（source=feedback）
//	                → 相似症状检索时该案例可被召回
//	                → Agent 在推理前查询相似案例，作为先验参考
//
// 本文件实现：
//   1. SubmitCaseFeedback  提交纠正并落库
//   2. SearchSimilarCases  检索相似案例（供 Agent 推理前调用）
//   3. AgentPromptWithCases 把相似案例注入 Agent system prompt
//
// 设计约束：
//   - 纠正必须由用户显式确认（避免误标污染案例库）
//   - 案例召回限制条数，防止 prompt 膨胀
//   - 命中次数递增，高频有效方案排序更靠前
package logs

import (
	"context"
	"fmt"
	"log"
	"strings"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// 案例回流常量。
const (
	// maxSimilarCases Agent 推理时注入的相似案例上限
	maxSimilarCases = 3
	// maxSymptomKeywords 从查询中提取的症状关键词上限
	maxSymptomKeywords = 5
)

// CaseFeedbackReq 纠正提交请求。
type CaseFeedbackReq struct {
	// ConvID 关联会话 ID（可空）
	ConvID string `json:"convId"`
	// OriginalQuery 触发推理的原始问题
	OriginalQuery string `json:"originalQuery"`
	// WrongConclusion Agent 给出的错误结论
	WrongConclusion string `json:"wrongConclusion"`
	// CorrectedRootCause 用户纠正后的根因（必填）
	CorrectedRootCause string `json:"correctedRootCause"`
	// CorrectedSolution 用户纠正后的解决方案
	CorrectedSolution string `json:"correctedSolution"`
	// Symptoms 症状关键词列表（可空，为空时自动提取）
	Symptoms []string `json:"symptoms"`
	// Tags 标签（可空）
	Tags []string `json:"tags"`
}

// SubmitCaseFeedback 提交纠正并写入案例库。
func SubmitCaseFeedback(c *gin.Context) {
	var req CaseFeedbackReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "参数非法")
		return
	}

	// 根因是案例的核心价值，必须由用户填写
	if strings.TrimSpace(req.CorrectedRootCause) == "" {
		api.Fail(c, api.CodeBadRequest, "纠正后的根因不能为空")
		return
	}

	// 症状为空时从原始问题自动提取关键词
	symptoms := req.Symptoms
	if len(symptoms) == 0 {
		symptoms = extractSymptomKeywords(req.OriginalQuery)
	}
	if len(symptoms) == 0 {
		// 仍为空则用原始问题作为兜底症状，保证案例可检索
		symptoms = []string{truncateStr(req.OriginalQuery, 50)}
	}

	title := buildCaseTitle(req.OriginalQuery, symptoms)
	tagList := strings.Join(req.Tags, ",")

	entry := models.CaseEntry{
		Title:     title,
		Symptoms:  symptoms,
		RootCause: req.CorrectedRootCause,
		Solution:  req.CorrectedSolution,
		TagList:   tagList,
		Source:    models.CaseSourceFeedback,
		ConvID:    req.ConvID,
		CreatedBy: currentOperator(c),
	}

	id, err := models.SaveCase(entry)
	if err != nil {
		log.Printf("[case] 保存纠正案例失败: %v", err)
		api.Fail(c, api.CodeServerError, "保存案例失败")
		return
	}

	log.Printf("[case] 纠正案例已入库: id=%d title=%s", id, title)
	api.Success(c, "案例已保存，将在后续相似会话中被检索", gin.H{
		"id":       id,
		"title":    title,
		"symptoms": symptoms,
	})
}

// buildCaseTitle 构造案例标题。
//
// 优先使用症状关键词组合（更具区分度），缺省时回退到原始问题。
func buildCaseTitle(query string, symptoms []string) string {
	if len(symptoms) > 0 {
		title := strings.Join(symptoms, "/")
		if len([]rune(title)) <= 40 {
			return title
		}
		return string([]rune(title)[:40])
	}
	q := strings.TrimSpace(query)
	if len([]rune(q)) > 40 {
		q = string([]rune(q)[:40])
	}
	return q
}

// extractSymptomKeywords 从查询文本提取症状关键词。
//
// 策略：
//   1. 提取引号内文本（用户明确指定的关键词）
//   2. 提取英文单词（长度 ≥4，排除常见停用词）
//   3. 提取错误码模式（如 ERR_001、timeout）
func extractSymptomKeywords(query string) []string {
	seen := make(map[string]bool)
	keywords := make([]string, 0, maxSymptomKeywords)

	// 引号内文本
	quoted := extractQuotedTexts(query)
	for _, t := range quoted {
		if !seen[t] {
			seen[t] = true
			keywords = append(keywords, t)
			if len(keywords) >= maxSymptomKeywords {
				return keywords
			}
		}
	}

	// 英文/数字关键词
	fields := strings.FieldsFunc(query, func(r rune) bool {
		return r == ' ' || r == '\t' || r == ',' || r == '，' || r == '。' || r == '、'
	})
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if len(f) < 4 || len(f) > 40 {
			continue
		}
		if isStopWord(f) {
			continue
		}
		// 至少包含一个字母
		hasLetter := false
		for _, r := range f {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				hasLetter = true
				break
			}
		}
		if !hasLetter {
			continue
		}
		if !seen[f] {
			seen[f] = true
			keywords = append(keywords, f)
			if len(keywords) >= maxSymptomKeywords {
				return keywords
			}
		}
	}

	return keywords
}

// stopWords 症状关键词停用词（中文排障常用但无区分度）。
var stopWords = map[string]bool{
	"日志": true, "错误": true, "异常": true, "问题": true, "查询": true,
	"检索": true, "分析": true, "排查": true, "看看": true, "一下": true,
	"error": true, "failed": true, "failure": true, "issue": true,
}

// isStopWord 判断是否为停用词。
func isStopWord(s string) bool {
	return stopWords[strings.ToLower(s)]
}

// extractQuotedTexts 提取引号内的文本片段。
func extractQuotedTexts(s string) []string {
	pairs := [][]rune{{'「', '」'}, {'“', '”'}, {'"', '"'}, {'\'', '\''}}
	out := make([]string, 0, 2)

	for _, pair := range pairs {
		open, close := pair[0], pair[1]
		rest := s
		for {
			start := strings.IndexRune(rest, open)
			if start < 0 {
				break
			}
			rest = rest[start+len(string(open)):]
			end := strings.IndexRune(rest, close)
			if end < 0 {
				break
			}
			text := strings.TrimSpace(rest[:end])
			if text != "" {
				out = append(out, text)
			}
			rest = rest[end+len(string(close)):]
		}
	}
	return out
}

// SimilarCase 相似案例（供 Agent 推理使用）。
type SimilarCase struct {
	// CaseID 案例 ID
	CaseID string `json:"caseId"`
	// Title 标题
	Title string `json:"title"`
	// RootCause 根因
	RootCause string `json:"rootCause"`
	// Solution 解决方案
	Solution string `json:"solution"`
	// HitCount 历史命中次数（用于加权）
	HitCount int `json:"hitCount"`
}

// SearchSimilarCases 检索相似案例。
//
// 供 Agent 推理前调用，把历史经验作为先验注入 prompt。
func SearchSimilarCases(query string) []SimilarCase {
	keywords := extractSymptomKeywords(query)
	if len(keywords) == 0 {
		return nil
	}

	// 用前两个关键词分别检索，合并去重
	merged := make(map[string]SimilarCase)
	for i, kw := range keywords {
		if i >= 2 {
			break
		}
		cases, err := models.SearchCasesBySymptoms(kw, maxSimilarCases)
		if err != nil {
			log.Printf("[case] 检索相似案例失败: %v", err)
			continue
		}
		for _, c := range cases {
			if _, exists := merged[c.CaseId]; exists {
				continue
			}
			merged[c.CaseId] = SimilarCase{
				CaseID:    c.CaseId,
				Title:     c.Title,
				RootCause: c.RootCause,
				Solution:  c.Solution,
				HitCount:  c.HitCount,
			}
		}
	}

	// 转为切片并限制条数
	out := make([]SimilarCase, 0, len(merged))
	for _, c := range merged {
		out = append(out, c)
		if len(out) >= maxSimilarCases {
			break
		}
	}
	return out
}

// AgentPromptWithCases 把相似案例注入 Agent 的 system prompt。
//
// 注入格式：
//
//	## 历史相似案例（供参考，不可直接照搬）
//	1. [title] 根因：... 方案：...（历史命中 N 次）
func AgentPromptWithCases(basePrompt string, cases []SimilarCase) string {
	if len(cases) == 0 {
		return basePrompt
	}

	var b strings.Builder
	b.WriteString(basePrompt)
	b.WriteString("\n\n## 历史相似案例（供参考，请结合当前证据判断，不可直接照搬）\n")

	for i, c := range cases {
		fmt.Fprintf(&b, "%d. [%s]\n", i+1, c.Title)
		if c.RootCause != "" {
			fmt.Fprintf(&b, "   根因：%s\n", truncateStr(c.RootCause, 120))
		}
		if c.Solution != "" {
			fmt.Fprintf(&b, "   方案：%s\n", truncateStr(c.Solution, 120))
		}
		fmt.Fprintf(&b, "   历史命中：%d 次\n", c.HitCount)
	}

	return b.String()
}

// ListCasesHandler GET /logs/cases
//
// 案例库列表查询。
func ListCasesHandler(c *gin.Context) {
	pageNo := parseIntParam(c, "pageNo", 1)
	pageSize := parseIntParam(c, "pageSize", 20)
	if pageSize > 100 {
		pageSize = 100
	}

	cases, total, err := models.ListCases(pageSize, (pageNo-1)*pageSize)
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询案例库失败")
		return
	}

	api.Success(c, "查询成功", gin.H{
		"list":  cases,
		"total": total,
		"pageNo": pageNo,
		"pageSize": pageSize,
	})
}

// SearchCasesHandler GET /logs/cases/search?keyword=xxx
func SearchCasesHandler(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	if keyword == "" {
		api.Fail(c, api.CodeBadRequest, "keyword 不能为空")
		return
	}

	cases, err := models.SearchCasesBySymptoms(keyword, 20)
	if err != nil {
		api.Fail(c, api.CodeServerError, "检索案例失败")
		return
	}

	// 命中即递增计数（用于方案有效性排序）
	for _, c := range cases {
		if err := models.IncrementCaseHit(c.CaseId); err != nil {
			log.Printf("[case] 命中计数失败 %s: %v", c.CaseId, err)
		}
	}

	api.Success(c, "检索成功", gin.H{"cases": cases, "total": len(cases)})
}

// currentOperator 从会话中提取当前操作人（失败返回空串）。
func currentOperator(c *gin.Context) string {
	if v, ok := c.Get("username"); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// parseIntParam 解析整型 query 参数（失败返回默认值）。
func parseIntParam(c *gin.Context, key string, def int) int {
	v := c.Query(key)
	if v == "" {
		return def
	}
	n := 0
	for _, r := range v {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return def
	}
	return n
}

// ensureContextUsed 避免 context 包在部分构建下被判定为未使用。
var _ = context.Background
