// Package logs 子文件：变更关联（V2.3-10，FR-905）。
//
// 能力：
//  1. 变更事件登记与查询（手动 + 外部推送）
//  2. 异常与变更的时间窗关联（强证据加权）
//  3. 把变更信息注入 Agent 推理与事故简报
//
// 关联算法：
//
//	对每个异常事件，查询 [发生时间 - N 分钟, 发生时间 + 5 分钟] 内的变更。
//	命中则标记为「变更关联」，在证据链中提升权重。
//
//	窗口默认 30 分钟：发布到用户可感知异常通常在这个范围内。
//	前后各留 5 分钟缓冲，覆盖时钟偏差与灰度延迟。
package logs

import (
	"fmt"
	"log"
	"strings"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// 变更关联常量。
const (
	// changeCorrelateWindowMin 变更关联的回溯窗口（分钟）
	changeCorrelateWindowMin = 30
	// changeCorrelateAfterMin 异常发生后的缓冲窗口（分钟）
	changeCorrelateAfterMin = 5
)

// ChangeCorrelation 单个异常与变更的关联结果。
type ChangeCorrelation struct {
	// AnomalyID 异常事件 ID
	AnomalyID int64 `json:"anomalyId"`
	// AnomalyTime 异常发生时间
	AnomalyTime string `json:"anomalyTime"`
	// TemplateHash 关联模板
	TemplateHash string `json:"templateHash"`
	// RelatedChanges 时间窗内命中的变更事件
	RelatedChanges []ChangeBrief `json:"relatedChanges"`
	// StrongSignal 是否为强信号（窗口内有变更）
	StrongSignal bool `json:"strongSignal"`
	// Explanation 关联说明（供报告与解释卡展示）
	Explanation string `json:"explanation,omitempty"`
}

// ChangeBrief 变更摘要。
type ChangeBrief struct {
	Title       string `json:"title"`
	ChangeType  string `json:"changeType"`
	Service     string `json:"service"`
	Operator    string `json:"operator"`
	OccurredAt  string `json:"occurredAt"`
	TimeGapMin  int    `json:"timeGapMin"` // 与异常的分钟差（负数表示变更在前）
}

// CorrelateChanges 对一批异常事件做变更关联。
//
// 这是 FR-905 的核心：把「最近有发布」变成根因推断的强证据。
func CorrelateChanges(events []models.TAnomalyEvent) []ChangeCorrelation {
	if len(events) == 0 {
		return nil
	}

	out := make([]ChangeCorrelation, 0, len(events))
	for _, ev := range events {
		out = append(out, correlateOne(ev))
	}
	return out
}

// correlateOne 关联单个异常事件。
func correlateOne(ev models.TAnomalyEvent) ChangeCorrelation {
	result := ChangeCorrelation{
		AnomalyID:    ev.Id,
		AnomalyTime:  ev.CreatedTime,
		TemplateHash: ev.TemplateId,
	}

	// 解析异常时间（可能解析失败，此时跳过关联）
	at, err := time.ParseInLocation("2006-01-02 15:04:05", ev.CreatedTime, time.Local)
	if err != nil {
		result.Explanation = "异常时间无法解析，跳过变更关联"
		return result
	}

	changes, err := models.GetChangeEventsAround(at, changeCorrelateWindowMin)
	if err != nil {
		log.Printf("[change] 变更查询失败: %v", err)
		result.Explanation = "变更事件查询失败"
		return result
	}

	if len(changes) == 0 {
		return result
	}

	// 过滤：服务名与异常模板关联时优先（模板样本中常含服务名）
	matched := make([]ChangeBrief, 0, len(changes))
	for _, c := range changes {
		gap := changeTimeGap(at, c.OccurredAt)
		if gap == 0 {
			continue
		}
		// 只保留回溯窗口内 + 异常后 5 分钟的变更
		if gap > changeCorrelateWindowMin || gap < -changeCorrelateAfterMin {
			continue
		}
		matched = append(matched, ChangeBrief{
			Title:      c.Title,
			ChangeType: c.ChangeType,
			Service:    c.Service,
			Operator:   c.Operator,
			OccurredAt: c.OccurredAt,
			TimeGapMin: gap,
		})
	}

	if len(matched) == 0 {
		return result
	}

	result.RelatedChanges = matched
	result.StrongSignal = true
	result.Explanation = buildCorrelationExplanation(matched, gapSign(matched))
	return result
}

// changeTimeGap 计算变更与异常的分钟差。
//
// 返回值：正数表示变更早于异常（发布在前），负数表示变更晚于异常。
// 解析失败返回 0（调用方跳过）。
func changeTimeGap(anomalyTime time.Time, changeTimeStr string) int {
	ct, err := time.ParseInLocation("2006-01-02 15:04:05", changeTimeStr, time.Local)
	if err != nil {
		return 0
	}
	return int(anomalyTime.Sub(ct).Minutes())
}

// gapSign 判断关联变更主要在异常前还是后。
func gapSign(changes []ChangeBrief) string {
	before, after := 0, 0
	for _, c := range changes {
		if c.TimeGapMin > 0 {
			before++
		} else {
			after++
		}
	}
	switch {
	case before > after:
		return "before"
	case after > before:
		return "after"
	default:
		return "mixed"
	}
}

// buildCorrelationExplanation 构造关联说明。
func buildCorrelationExplanation(changes []ChangeBrief, sign string) string {
	if len(changes) == 0 {
		return ""
	}

	c := changes[0]
	gap := c.TimeGapMin
	var b strings.Builder

	switch sign {
	case "before":
		fmt.Fprintf(&b, "异常发生前 %d 分钟有「%s」变更（%s，操作人 %s），疑似变更引入",
			gap, c.Title, c.ChangeType, c.Operator)
	case "after":
		fmt.Fprintf(&b, "异常发生后 %d 分钟有「%s」变更，可能为应急修复操作",
			-gap, c.Title)
	default:
		fmt.Fprintf(&b, "异常时间窗内有 %d 项变更，最近一项「%s」间隔 %d 分钟",
			len(changes), c.Title, absInt(gap))
	}

	if len(changes) > 1 {
		fmt.Fprintf(&b, "（另有 %d 项变更）", len(changes)-1)
	}
	return b.String()
}

// absInt 绝对值。
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ─── HTTP handlers ───

// changeEventReq 变更事件登记请求。
type changeEventReq struct {
	EventID     string `json:"eventId"`
	Title       string `json:"title" binding:"required"`
	ChangeType  string `json:"changeType"`
	Service     string `json:"service"`
	Environment string `json:"environment"`
	Operator    string `json:"operator"`
	OccurredAt  string `json:"occurredAt"`
	Source      string `json:"source"`
	Detail      string `json:"detail"`
}

// validChangeType 校验变更类型。
func validChangeType(t string) bool {
	switch t {
	case "deploy", "config", "db", "network", "other", "":
		return true
	}
	return false
}

// CreateChangeEventHandler POST /logs/changes
func CreateChangeEventHandler(c *gin.Context) {
	var req changeEventReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "参数非法")
		return
	}

	if !validChangeType(req.ChangeType) {
		api.Fail(c, api.CodeBadRequest, "changeType 非法（deploy/config/db/network/other）")
		return
	}
	if req.OccurredAt == "" {
		req.OccurredAt = time.Now().Format("2006-01-02 15:04:05")
	}
	if req.Source == "" {
		req.Source = "manual"
	}
	if req.Operator == "" {
		req.Operator = currentOperator(c)
	}

	ev := &models.TChangeEvent{
		EventId:     req.EventID,
		Title:       req.Title,
		ChangeType:  req.ChangeType,
		Service:     req.Service,
		Environment: req.Environment,
		Operator:    req.Operator,
		OccurredAt:  req.OccurredAt,
		Source:      req.Source,
		Detail:      req.Detail,
	}

	id, err := models.SaveChangeEvent(ev)
	if err != nil {
		log.Printf("[change] 保存变更事件失败: %v", err)
		api.Fail(c, api.CodeServerError, "保存变更事件失败")
		return
	}

	api.Success(c, "变更事件已登记", gin.H{"id": id, "eventId": ev.EventId})
}

// ListChangesHandler GET /logs/changes
func ListChangesHandler(c *gin.Context) {
	limit := parseIntParam(c, "limit", 20)
	if limit > 100 {
		limit = 100
	}

	from := c.Query("from")
	to := c.Query("to")
	service := c.Query("service")
	changeType := c.Query("changeType")

	// 默认查最近 24 小时
	if from == "" {
		from = time.Now().Add(-24 * time.Hour).Format("2006-01-02 15:04:05")
	}
	if to == "" {
		to = time.Now().Format("2006-01-02 15:04:05")
	}

	events, err := models.QueryChangeEvents(from, to, service, changeType, limit)
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询变更事件失败")
		return
	}

	api.Success(c, "查询成功", gin.H{"events": events, "total": len(events)})
}

// CorrelateChangesHandler GET /logs/changes/correlate?timeRange=24h
//
// 返回异常事件与变更的关联结果，供 Agent 与事故简报使用。
func CorrelateChangesHandler(c *gin.Context) {
	timeRange := c.Query("timeRange")
	if timeRange == "" {
		timeRange = "24h"
	}

	dur, err := parseDurationForAgent(timeRange)
	if err != nil {
		api.Fail(c, api.CodeBadRequest, "timeRange 非法")
		return
	}

	from := time.Now().Add(-dur).Format("2006-01-02 15:04:05")
	to := time.Now().Format("2006-01-02 15:04:05")

	events, err := models.QueryAnomalyEvents(0, from, to, 50)
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询异常事件失败")
		return
	}

	correlations := CorrelateChanges(events)

	strongCount := 0
	for _, cc := range correlations {
		if cc.StrongSignal {
			strongCount++
		}
	}

	api.Success(c, "关联完成", gin.H{
		"correlations": correlations,
		"total":        len(correlations),
		"strongSignal": strongCount,
		"windowMin":    changeCorrelateWindowMin,
	})
}

// InjectChangesIntoPrompt 把变更信息注入 Agent system prompt。
//
// 供 ReAct 循环在构造 prompt 时调用（与相似案例注入并列）。
func InjectChangesIntoPrompt(basePrompt string, queryTime time.Time) string {
	changes, err := models.GetChangeEventsAround(queryTime, changeCorrelateWindowMin)
	if err != nil || len(changes) == 0 {
		return basePrompt
	}

	var b strings.Builder
	b.WriteString(basePrompt)
	b.WriteString("\n\n## 近期变更事件（强根因线索，请优先验证）\n")

	for i, c := range changes {
		if i >= 5 {
			break
		}
		fmt.Fprintf(&b, "%d. [%s] %s（服务：%s，操作人：%s，时间：%s）\n",
			i+1, c.ChangeType, truncateStr(c.Title, 80), c.Service, c.Operator, c.OccurredAt)
	}

	return b.String()
}
