// Package logs 子文件：事故简报生成与 Markdown 导出（V2.3-07，FR-906）。
//
// 背景：
//   排障结束后往往需要一份可分享的事故简报（时间线、影响面、根因、待办）。
//   本模块把 Agent 的推理结果 + 异常事件 + 模板统计渲染为 Markdown，
//   支持直接导出与粘贴到工单系统。
//
// 设计：
//   - 纯文本渲染，不依赖外部模板引擎
//   - 所有数据来自实际检索结果，不编造
//   - 生成结果落库到 t_incident_report，支持重复生成与历史查询
//   - 导出接口以 text/markdown 返回，前端可直接下载
package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// IncidentReport 事故简报。
type IncidentReport struct {
	// ID 简报 ID
	ID int64 `json:"id"`
	// Query 触发本次简报生成的检索问题
	Query string `json:"query"`
	// Title 标题
	Title string `json:"title"`
	// StartedAt 事故开始时间
	StartedAt string `json:"startedAt"`
	// Duration 持续时长（人类可读）
	Duration string `json:"duration"`
	// Severity 严重级别（P0/P1/P2/P3）
	Severity string `json:"severity"`
	// Summary 一句话概述
	Summary string `json:"summary"`
	// RootCause 根因
	RootCause string `json:"rootCause"`
	// Impact 影响面
	Impact string `json:"impact"`
	// Timeline 时间线条目
	Timeline []IncidentTimelineItem `json:"timeline"`
	// Evidence 证据引用
	Evidence []EvidenceRef `json:"evidence"`
	// Actions 后续动作
	Actions []string `json:"actions"`
	// Markdown 渲染后的 Markdown 全文
	Markdown string `json:"markdown"`
	// CreatedAt 生成时间
	CreatedAt string `json:"createdAt"`
}

// IncidentTimelineItem 时间线条目。
type IncidentTimelineItem struct {
	Time        string `json:"time"`
	Description string `json:"description"`
}

// GenerateIncidentReport 生成事故简报（V2.3-07）。
//
// 数据来源：
//   - 异常事件（t_anomaly_event）：构成时间线与严重度判定
//   - 模板统计（t_pattern_stat）：构成影响面
//   - Agent 推理结果（可选）：提供根因与建议
func GenerateIncidentReport(ctx context.Context, query string, timeRange string,
	reasonResult *ReasoningResult) (*IncidentReport, error) {

	dur, err := parseDurationForAgent(timeRange)
	if err != nil {
		dur = 24 * time.Hour
	}

	from := time.Now().Add(-dur)
	events, err := models.QueryAnomalyEvents(0,
		from.Format("2006-01-02 15:04:05"),
		time.Now().Format("2006-01-02 15:04:05"), 50)
	if err != nil {
		return nil, fmt.Errorf("查询异常事件失败: %w", err)
	}

	patterns, err := models.TopPatterns(0, timeRange, 10)
	if err != nil {
		return nil, fmt.Errorf("查询模板统计失败: %w", err)
	}

	report := &IncidentReport{
		Query:     query,
		Title:     buildReportTitle(query, events),
		StartedAt: from.Format("2006-01-02 15:04:05"),
		Duration:  formatDuration(dur),
		Severity:  assessSeverity(events, patterns),
		Timeline:  buildTimeline(events),
		Evidence:  buildReportEvidence(events, patterns),
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
	}

	// 根因与建议优先采用 Agent 结论
	if reasonResult != nil {
		report.RootCause = reasonResult.Conclusion.RootCause
		report.Actions = reasonResult.Conclusion.Suggestions
		if reasonResult.ThreeSection != nil {
			report.Summary = reasonResult.ThreeSection.Hypothesis
		}
	} else {
		report.RootCause = buildFallbackRootCause(events, patterns)
		report.Summary = "基于异常事件与模板统计自动生成，建议结合 Agent 推理获取更精确根因"
		report.Actions = buildFallbackActions(events)
	}

	report.Impact = buildImpact(patterns)
	report.Markdown = renderReportMarkdown(report)

	return report, nil
}

// buildReportTitle 构造简报标题。
func buildReportTitle(query string, events []models.TAnomalyEvent) string {
	if len(events) > 0 {
		return fmt.Sprintf("事故简报：%s 相关异常分析", truncateStr(query, 40))
	}
	return fmt.Sprintf("日志分析报告：%s", truncateStr(query, 40))
}

// assessSeverity 评估严重级别。
//
// 规则：
//   - 存在 FATAL 模板或异常 score > 8 → P0
//   - 异常 score > 5 或 3+ 条异常 → P1
//   - 有异常事件 → P2
//   - 无异常 → P3
func assessSeverity(events []models.TAnomalyEvent, patterns []models.PatternStat) string {
	maxScore := 0.0
	for _, ev := range events {
		if ev.Score > maxScore {
			maxScore = ev.Score
		}
	}
	for _, p := range patterns {
		if extractLevel(p.Sample) == "FATAL" {
			return "P0"
		}
	}

	switch {
	case maxScore > 8:
		return "P0"
	case maxScore > 5 || len(events) >= 3:
		return "P1"
	case len(events) > 0:
		return "P2"
	}
	return "P3"
}

// buildTimeline 从异常事件构造时间线。
func buildTimeline(events []models.TAnomalyEvent) []IncidentTimelineItem {
	if len(events) == 0 {
		return nil
	}

	items := make([]IncidentTimelineItem, 0, len(events))
	for i, ev := range events {
		if i >= 20 {
			break
		}
		items = append(items, IncidentTimelineItem{
			Time:        ev.CreatedTime,
			Description: truncateStr(ev.Description, 120),
		})
	}
	return items
}

// buildReportEvidence 汇总证据引用。
func buildReportEvidence(events []models.TAnomalyEvent, patterns []models.PatternStat) []EvidenceRef {
	refs := make([]EvidenceRef, 0, 6)

	for i, ev := range events {
		if i >= 3 {
			break
		}
		refs = append(refs, EvidenceRef{
			Source:  EvidenceSourceAnomaly,
			RefID:   fmt.Sprintf("%d", ev.Id),
			Summary: truncateStr(ev.Description, 80),
			Quote:   truncateStr(ev.Description, maxQuoteLength),
			Time:    ev.CreatedTime,
		})
	}
	for i, p := range patterns {
		if i >= 3 {
			break
		}
		refs = append(refs, EvidenceRef{
			Source:  EvidenceSourceTemplate,
			RefID:   p.TemplateHash,
			Summary: fmt.Sprintf("命中 %d 次", p.Count),
			Quote:   truncateStr(p.Sample, maxQuoteLength),
		})
	}
	return refs
}

// buildImpact 构造影响面描述。
func buildImpact(patterns []models.PatternStat) string {
	if len(patterns) == 0 {
		return "未统计到明显影响"
	}

	total := 0
	for _, p := range patterns {
		total += p.Count
	}

	return fmt.Sprintf("时间窗内共命中 %d 条异常日志，涉及 %d 个模板，最高频模板命中 %d 次",
		total, len(patterns), patterns[0].Count)
}

// buildFallbackRootCause 无 Agent 结果时的根因推断。
func buildFallbackRootCause(events []models.TAnomalyEvent, patterns []models.PatternStat) string {
	if len(events) == 0 {
		return "当前时间窗内未检测到明显异常"
	}
	if len(patterns) == 0 {
		return fmt.Sprintf("检测到 %d 条异常事件，但缺少模板样本，建议人工排查", len(events))
	}
	return fmt.Sprintf("检测到 %d 条异常事件，高频模板为 %s（命中 %d 次），建议优先排查该模式",
		len(events), patterns[0].TemplateHash, patterns[0].Count)
}

// buildFallbackActions 无 Agent 结果时的建议。
func buildFallbackActions(events []models.TAnomalyEvent) []string {
	if len(events) == 0 {
		return []string{"持续观察日志趋势", "确认告警阈值是否合理"}
	}
	return []string{
		"查看异常模板的完整堆栈",
		"对比历史同期确认是否为首次出现",
		"检查相关服务的变更记录",
	}
}

// formatDuration 把时长格式化为人类可读。
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	switch {
	case hours > 0 && mins > 0:
		return fmt.Sprintf("%d 小时 %d 分钟", hours, mins)
	case hours > 0:
		return fmt.Sprintf("%d 小时", hours)
	default:
		return fmt.Sprintf("%d 分钟", mins)
	}
}

// renderReportMarkdown 渲染 Markdown 全文。
func renderReportMarkdown(r *IncidentReport) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", r.Title)
	fmt.Fprintf(&b, "> 生成时间：%s　|　严重级别：**%s**　|　时间范围：%s（%s）\n\n",
		r.CreatedAt, r.Severity, r.StartedAt, r.Duration)

	fmt.Fprintf(&b, "## 概述\n\n%s\n\n", r.Summary)
	fmt.Fprintf(&b, "## 根因分析\n\n%s\n\n", r.RootCause)
	fmt.Fprintf(&b, "## 影响面\n\n%s\n\n", r.Impact)

	if len(r.Timeline) > 0 {
		b.WriteString("## 时间线\n\n")
		b.WriteString("| 时间 | 事件 |\n|------|------|\n")
		for _, item := range r.Timeline {
			fmt.Fprintf(&b, "| %s | %s |\n", item.Time, escapeMarkdown(item.Description))
		}
		b.WriteString("\n")
	}

	if len(r.Evidence) > 0 {
		b.WriteString("## 证据\n\n")
		for i, e := range r.Evidence {
			fmt.Fprintf(&b, "%d. **[%s]** %s\n", i+1, e.Source, escapeMarkdown(e.Summary))
			if e.Quote != "" {
				fmt.Fprintf(&b, "   > %s\n", escapeMarkdown(e.Quote))
			}
		}
		b.WriteString("\n")
	}

	if len(r.Actions) > 0 {
		b.WriteString("## 后续动作\n\n")
		for i, a := range r.Actions {
			fmt.Fprintf(&b, "%d. %s\n", i+1, escapeMarkdown(a))
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n\n*本简报由云顶追踪自动生成，结论可点击证据回溯原文验证。*\n")
	return b.String()
}

// escapeMarkdown 转义 Markdown 表格中的特殊字符。
func escapeMarkdown(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// --- HTTP handlers ---

// incidentReportReq 生成请求。
type incidentReportReq struct {
	Query     string `json:"query"`
	TimeRange string `json:"timeRange"`
	ConvID    string `json:"convId"`
}

// GenerateReportHandler POST /logs/agent/report
func GenerateReportHandler(c *gin.Context) {
	var req incidentReportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "参数非法")
		return
	}
	if req.Query == "" {
		api.Fail(c, api.CodeBadRequest, "query 不能为空")
		return
	}
	if req.TimeRange == "" {
		req.TimeRange = "24h"
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	report, err := GenerateIncidentReport(ctx, req.Query, req.TimeRange, nil)
	if err != nil {
		api.Fail(c, api.CodeServerError, "生成简报失败")
		return
	}

	// 落库保存（失败不影响返回）
	model := toIncidentReportModel(report)
	if id, serr := models.SaveIncidentReport(model); serr != nil {
		log.Printf("[incident] 简报落库失败: %v", serr)
		api.Success(c, "简报生成成功", gin.H{"report": report, "saved": false})
	} else {
		report.ID = id
		api.Success(c, "简报生成成功", gin.H{"report": report, "saved": true})
	}
}

// ExportReportHandler GET /logs/agent/report/export?id=xxx
//
// 以 text/markdown 返回简报全文，前端可直接触发下载。
func ExportReportHandler(c *gin.Context) {
	id := parseInt64(c.Query("id"))
	if id <= 0 {
		api.Fail(c, api.CodeBadRequest, "id 非法")
		return
	}

	m, err := models.GetIncidentReport(id)
	if err != nil || m.Id == 0 {
		api.Fail(c, api.CodeNotFound, "简报不存在")
		return
	}

	// 从模型重建控制器结构并补上 Markdown
	report := fromIncidentReportModel(m)
	if report.Markdown == "" {
		report.Markdown = renderReportMarkdown(report)
	}

	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="incident-report-%d.md"`, id))
	c.String(httpStatusOK, "%s", report.Markdown)
}

// toIncidentReportModel 控制器结构 → 持久化模型。
func toIncidentReportModel(r *IncidentReport) *models.TIncidentReport {
	timelineJSON, _ := json.Marshal(r.Timeline)
	evidenceJSON, _ := json.Marshal(r.Evidence)
	actionsJSON, _ := json.Marshal(r.Actions)

	return &models.TIncidentReport{
		Title:       r.Title,
		Query:       r.Query,
		StartedAt:   r.StartedAt,
		Duration:    r.Duration,
		Severity:    r.Severity,
		Summary:     r.Summary,
		RootCause:   r.RootCause,
		Impact:      r.Impact,
		Timeline:    string(timelineJSON),
		Evidence:    string(evidenceJSON),
		Actions:     string(actionsJSON),
		Markdown:    r.Markdown,
		CreatedAt:   r.CreatedAt,
	}
}

// fromIncidentReportModel 持久化模型 → 控制器结构。
func fromIncidentReportModel(m *models.TIncidentReport) *IncidentReport {
	r := &IncidentReport{
		ID:        m.Id,
		Title:     m.Title,
		StartedAt: m.StartedAt,
		Duration:  m.Duration,
		Severity:  m.Severity,
		Summary:   m.Summary,
		RootCause: m.RootCause,
		Impact:    m.Impact,
		Markdown:  m.Markdown,
		CreatedAt: m.CreatedAt,
	}

	if len(m.Timeline) > 0 {
		_ = json.Unmarshal([]byte(m.Timeline), &r.Timeline)
	}
	if len(m.Evidence) > 0 {
		_ = json.Unmarshal([]byte(m.Evidence), &r.Evidence)
	}
	if len(m.Actions) > 0 {
		_ = json.Unmarshal([]byte(m.Actions), &r.Actions)
	}
	return r
}

// parseInt64 解析正整数（失败返回 0）。
func parseInt64(s string) int64 {
	if s == "" {
		return 0
	}
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int64(r-'0')
	}
	return n
}

// httpStatusOK HTTP 200（避免在文件中重复引入 net/http）。
const httpStatusOK = 200
