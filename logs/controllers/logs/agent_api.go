// Package logs 子文件：根因分析 Agent API（FR-906~908）。
//
// 暴露两个端点：
//   - POST /logs/agent/reason   执行根因分析（ReAct 循环 + 三段式输出）
//   - GET  /logs/agent/tools    列出可用工具
package logs

import (
	"context"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"ydsz-trace/pkg/api"
)

// AgentReasonReq 根因分析请求体。
type AgentReasonReq struct {
	// Query 用户问题（必填）
	Query string `json:"query" binding:"required"`
	// ConvID 关联的会话 ID（可选，用于消息持久化）
	ConvID string `json:"convId"`
}

// AgentToolInfo 工具元信息（供前端展示）。
type AgentToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// AgentReasonHandler 执行根因分析。
//
// @Summary 根因分析
// @Description 基于 ReAct 循环调用只读工具检索日志、统计与链路信息，返回三段式根因结论（假设/证据/结论）。LLM 不可用时自动降级为统计结论。
// @Tags Agent
// @Accept json
// @Produce json
// @Param request body AgentReasonReq true "根因分析请求"
// @Success 200 {object} api.Success{data=ReasoningResult}
// @Router /logs/agent/reason [post]
func AgentReasonHandler(c *gin.Context) {
	var req AgentReasonReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: query 为必填项")
		return
	}

	req.Query = trimSpace(req.Query)
	if req.Query == "" {
		api.Fail(c, api.CodeBadRequest, "query 不能为空")
		return
	}

	// 执行根因分析（带整体超时，防止 ReAct 循环无限占用）
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()

	agent := NewReasoningAgent()
	result, err := agent.Run(ctx, req.Query, req.ConvID)
	if err != nil {
		log.Printf("[agent] 根因分析失败: %v", err)
		api.Fail(c, api.CodeServerError, "根因分析失败: "+err.Error())
		return
	}

	api.Success(c, "根因分析完成", result)
}

// AgentToolsHandler 列出可用工具。
//
// @Summary 列出 Agent 可用工具
// @Description 返回根因分析 Agent 的只读工具集及其描述。
// @Tags Agent
// @Produce json
// @Success 200 {object} api.Success{data=[]AgentToolInfo}
// @Router /logs/agent/tools [get]
func AgentToolsHandler(c *gin.Context) {
	// 全部 7 类只读工具（V2.3-03），与 ReasoningAgent 注册保持一致
	tools := []AgentToolInfo{
		{Name: (&SearchLogsTool{}).Name(), Description: (&SearchLogsTool{}).Description()},
		{Name: (&AggStatsTool{}).Name(), Description: (&AggStatsTool{}).Description()},
		{Name: (&GetContextTool{}).Name(), Description: (&GetContextTool{}).Description()},
		{Name: (&GetTraceTool{}).Name(), Description: (&GetTraceTool{}).Description()},
		{Name: (&ListTemplatesTool{}).Name(), Description: (&ListTemplatesTool{}).Description()},
		{Name: (&GetAnomaliesTool{}).Name(), Description: (&GetAnomaliesTool{}).Description()},
		{Name: (&DiffTimeWindowTool{}).Name(), Description: (&DiffTimeWindowTool{}).Description()},
	}
	api.Success(c, "查询成功", tools)
}

// AgentToolStats 工具成功率统计（V2.3-03，FR-903 验收：工具成功率 ≥95%）。
type AgentToolStats struct {
	Tool         string  `json:"tool"`
	TotalCalls   int     `json:"totalCalls"`
	SuccessCalls int     `json:"successCalls"`
	FailureCalls int     `json:"failureCalls"`
	SuccessRate  float64 `json:"successRate"`
}

// AgentToolStatsHandler GET /logs/agent/tools/stats
//
// 汇总工具调用成功率，供运维监控 Agent 健康度。
func AgentToolStatsHandler(c *gin.Context) {
	stats := collectToolStats()
	api.Success(c, "查询成功", stats)
}

// trimSpace 去除字符串首尾空白。
func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
