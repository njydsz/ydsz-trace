// Package logs 子文件：AI 会话审计（V2.4-06，FR-309）。
//
// 背景：
//   PRD FR-309 要求审计覆盖「查询 / 导出 / AI 会话 / 配置变更」四类操作。
//   既有实现已覆盖前三类的大部分，但 **AI 会话**（根因推理、NL2DSL 解析、
//   事故简报生成）缺少审计留痕——而这些操作会消耗 Token、可能外发数据，
//   恰恰是最需要可追溯的。
//
// 本模块补充：
//   1. 在 Agent 推理 / NL2DSL 解析 / 简报生成处记录审计
//   2. 提供审计查询接口（按操作类型、时间、用户过滤）
//
// 审计内容：
//   - 操作人、IP、时间
//   - 输入摘要（query 截断，不记录完整 prompt 以控制体积）
//   - 结果状态（success / fail）
//   - Token 消耗（AI 操作）
//   - 耗时
package logs

import (
	"time"

	"ydsz-trace/logs/controllers/audit"
	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/session"

	"github.com/gin-gonic/gin"
)

// 审计动作常量。
const (
	// AuditActionAIReason AI 根因推理
	AuditActionAIReason = "ai_reason"
	// AuditActionAINl2Dsl NL2DSL 解析
	AuditActionAINl2Dsl = "ai_nl2dsl"
	// AuditActionAIReport 事故简报生成
	AuditActionAIReport = "ai_report"
	// AuditActionAISemantic 语义检索
	AuditActionAISemantic = "ai_semantic"
)

// AuditAIReason 记录 AI 推理审计。
//
// detail 建议包含：query 摘要、模型名、Token 消耗、步数、是否降级。
func AuditAIReason(c *gin.Context, detail map[string]interface{}, result string) {
	audit.RecordAudit(c, AuditActionAIReason, "/logs/agent/reason", detail, result)
}

// AuditAINl2Dsl 记录 NL2DSL 解析审计。
func AuditAINl2Dsl(c *gin.Context, detail map[string]interface{}, result string) {
	audit.RecordAudit(c, AuditActionAINl2Dsl, "/logs/nl2dsl/parse", detail, result)
}

// AuditAIReport 记录事故简报生成审计。
func AuditAIReport(c *gin.Context, detail map[string]interface{}, result string) {
	audit.RecordAudit(c, AuditActionAIReport, "/logs/agent/report", detail, result)
}

// AuditAISemantic 记录语义检索审计。
func AuditAISemantic(c *gin.Context, detail map[string]interface{}, result string) {
	audit.RecordAudit(c, AuditActionAISemantic, "/api/v2/semantic/search", detail, result)
}

// BuildAIReasonDetail 构造 AI 推理审计详情（统一字段便于查询）。
func BuildAIReasonDetail(query string, modelName string, tokens, steps int,
	degraded bool, costMs int64) map[string]interface{} {

	return map[string]interface{}{
		"query":      truncateStr(query, 200),
		"model":      modelName,
		"tokens":     tokens,
		"steps":      steps,
		"degraded":   degraded,
		"costMs":     costMs,
	}
}

// BuildNl2DslDetail 构造 NL2DSL 审计详情。
func BuildNl2DslDetail(query, strategy string, confidence float64, costMs int64) map[string]interface{} {
	return map[string]interface{}{
		"query":      truncateStr(query, 200),
		"strategy":   strategy,
		"confidence": confidence,
		"costMs":     costMs,
	}
}

// BuildReportDetail 构造简报生成审计详情。
func BuildReportDetail(query, timeRange, severity string, costMs int64) map[string]interface{} {
	return map[string]interface{}{
		"query":     truncateStr(query, 200),
		"timeRange": timeRange,
		"severity":  severity,
		"costMs":    costMs,
	}
}

// AuditQuery 审计查询请求参数。
type AuditQuery struct {
	Action   string
	Username string
	From     time.Time
	To       time.Time
	PageNo   int
	PageSize int
}

// parseAuditQuery 解析审计查询参数。
func parseAuditQuery(c *gin.Context) AuditQuery {
	pageNo, _ := atoiSafe(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := atoiSafe(c.DefaultQuery("pageSize", "50"))
	if pageNo <= 0 {
		pageNo = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}

	to := time.Now()
	from := to.Add(-7 * 24 * time.Hour)

	if v := c.Query("from"); v != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", v, time.Local); err == nil {
			from = t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", v, time.Local); err == nil {
			to = t
		}
	}

	return AuditQuery{
		Action:   c.Query("action"),
		Username: c.Query("username"),
		From:     from,
		To:       to,
		PageNo:   pageNo,
		PageSize: pageSize,
	}
}

// atoiSafe 安全解析整数。
func atoiSafe(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, nil
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

// AuditLogsHandler GET /logs/audit/logs
//
// 审计日志查询（按操作类型 / 用户 / 时间范围过滤）。
func AuditLogsHandler(c *gin.Context) {
	q := parseAuditQuery(c)

	logs, total, err := models.QueryAuditLogs(q.PageNo, q.PageSize, q.Username, q.Action, q.From, q.To)
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询审计日志失败")
		return
	}

	actions, aerr := models.RecentAuditActions(20)
	if aerr != nil {
		actions = nil
	}

	api.Success(c, "查询成功", gin.H{
		"logs":        logs,
		"total":       total,
		"pageNo":      q.PageNo,
		"pageSize":    q.PageSize,
		"actions":     actions,
		"from":        q.From.Format("2006-01-02 15:04:05"),
		"to":          q.To.Format("2006-01-02 15:04:05"),
	})
}

// AuditStatsHandler GET /logs/audit/stats
//
// 审计统计（按操作类型聚合），供管理界面展示。
func AuditStatsHandler(c *gin.Context) {
	from := time.Now().Add(-7 * 24 * time.Hour)
	to := time.Now()
	if v := c.Query("from"); v != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", v, time.Local); err == nil {
			from = t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", v, time.Local); err == nil {
			to = t
		}
	}

	stats, err := models.AuditStats(from, to)
	if err != nil {
		api.Fail(c, api.CodeServerError, "统计失败")
		return
	}
	api.Success(c, "查询成功", gin.H{
		"stats": stats,
		"from":  from.Format("2006-01-02 15:04:05"),
		"to":    to.Format("2006-01-02 15:04:05"),
	})
}

// AuditUsername 从会话取当前操作人（供测试与辅助函数使用）。
func AuditUsername(c *gin.Context) string {
	return session.GetString(c, "username")
}
