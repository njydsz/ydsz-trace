// Package admin 子文件：统计与异常检测管理接口（FR-801/FR-802/FR-304）。
//
// 提供 TopN 模式、异常事件查询、基线状态查看等管理端点。
package admin

import (
	"strconv"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// StatsPatterns GET /admin/stats/patterns — TopN 模式列表。
// Query: itemId(int, required) timeRange(str, default=1h) n(int, default=20)
func StatsPatterns(c *gin.Context) {
	itemID, err := strconv.ParseInt(c.Query("itemId"), 10, 64)
	if err != nil || itemID <= 0 {
		api.Fail(c, api.CodeBadRequest, "itemId 非法")
		return
	}
	timeRange := c.DefaultQuery("timeRange", "1h")
	n, _ := strconv.Atoi(c.DefaultQuery("n", "20"))
	if n <= 0 || n > 100 {
		n = 20
	}
	patterns, err := models.TopPatterns(itemID, timeRange, n)
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询失败: "+err.Error())
		return
	}
	api.Success(c, "查询成功", gin.H{
		"list":  patterns,
		"total": len(patterns),
	})
}

// StatsAnomalyEvents GET /admin/stats/anomalies — 最近的异常事件。
// Query: hours(int, default=1)（查 t_alert_event 中 status='anomaly' 的记录）
func StatsAnomalyEvents(c *gin.Context) {
	pageNo, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if pageNo <= 0 {
		pageNo = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	p := models.QueryAlertEvents(pageNo, pageSize, 0, "anomaly")
	api.Success(c, "查询成功", p)
}

// StatsBaseline GET /admin/stats/baseline — 某模板基线状态。
// Query: templateHash(str, required) itemId(int, required)
func StatsBaseline(c *gin.Context) {
	templateHash := c.Query("templateHash")
	if templateHash == "" {
		api.Fail(c, api.CodeBadRequest, "templateHash 必填")
		return
	}
	itemID, err := strconv.ParseInt(c.Query("itemId"), 10, 64)
	if err != nil || itemID <= 0 {
		api.Fail(c, api.CodeBadRequest, "itemId 非法")
		return
	}

	trend, err := models.QueryPatternTrend(templateHash, "", "")
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询趋势失败")
		return
	}
	api.Success(c, "查询成功", gin.H{
		"templateHash": templateHash,
		"itemId":       itemID,
		"trendPoints":  len(trend),
		"trend":        trend,
	})
}
