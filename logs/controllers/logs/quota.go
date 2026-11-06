// Package logs 子文件：成本配额 API（V2.1 FR-1305）。
//
// 提供配额检查、消费、统计三个端点：
//   - POST /logs/quota/check      检查配额是否充足
//   - POST /logs/quota/consume    消费配额
//   - GET  /logs/quota/stats/:id  查询模型成本统计
package logs

import (
	"net/http"
	"strconv"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// QuotaCheckReq 配额检查请求。
type QuotaCheckReq struct {
	ModelID string `json:"modelId" binding:"required"`
	Tokens  int64  `json:"tokens"`
}

// QuotaCheckHandler 检查模型配额是否充足。
// POST /logs/quota/check
func QuotaCheckHandler(c *gin.Context) {
	var req QuotaCheckReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: modelId 为必填项")
		return
	}
	if req.Tokens <= 0 {
		req.Tokens = 1000 // 默认预估 1k tokens
	}

	ok, remaining, usagePercent, err := models.CheckQuotaAndAlert(req.ModelID, req.Tokens)
	if err != nil {
		api.Fail(c, api.CodeServerError, "配额检查失败: "+err.Error())
		return
	}

	api.Success(c, "检查成功", gin.H{
		"modelId":      req.ModelID,
		"available":    ok,
		"remaining":    remaining,
		"usagePercent": usagePercent,
	})
}

// QuotaConsumeReq 配额消费请求。
type QuotaConsumeReq struct {
	ModelID string `json:"modelId" binding:"required"`
	Tokens  int64  `json:"tokens" binding:"required"`
}

// QuotaConsumeHandler 消费模型配额。
// POST /logs/quota/consume
func QuotaConsumeHandler(c *gin.Context) {
	var req QuotaConsumeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: modelId 和 tokens 为必填项")
		return
	}
	if req.Tokens <= 0 {
		api.Fail(c, api.CodeBadRequest, "tokens 必须大于 0")
		return
	}

	if err := models.ConsumeQuotaAndAlert(req.ModelID, req.Tokens); err != nil {
		api.Fail(c, api.CodeServerError, "配额消费失败: "+err.Error())
		return
	}

	api.Success(c, "消费成功", gin.H{
		"modelId": req.ModelID,
		"tokens":  req.Tokens,
	})
}

// QuotaStatsHandler 查询模型成本统计。
// GET /logs/quota/stats/:modelId
func QuotaStatsHandler(c *gin.Context) {
	modelID := c.Param("modelId")
	if modelID == "" {
		api.Fail(c, api.CodeBadRequest, "modelId 不能为空")
		return
	}

	stats, err := models.GetCostStats(modelID)
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询成本统计失败: "+err.Error())
		return
	}

	api.Success(c, "查询成功", stats)
}

// QuotaSummaryHandler 查询全部模型的成本配额摘要。
// GET /logs/quota/summary
func QuotaSummaryHandler(c *gin.Context) {
	// 查询所有启用的 chat 模型
	list, err := models.GetEnabledModelCapabilities("chat")
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询模型列表失败: "+err.Error())
		return
	}

	summary := make([]gin.H, 0, len(list))
	for _, m := range list {
		stats, err := models.GetCostStats(m.ModelId)
		if err != nil {
			continue
		}
		summary = append(summary, stats)
	}

	totalCost := 0.0
	for _, s := range summary {
		if v, ok := s["todayCost"].(string); ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				totalCost += f
			}
		}
	}

	api.Success(c, "查询成功", gin.H{
		"models":    summary,
		"totalCost": strconv.FormatFloat(totalCost, 'f', 4, 64),
	})
}

// 确保 net/http 被使用。
var _ = http.StatusOK
