// Package audit 审计日志控制器：分页查询与统计接口。
//
// 路由：
//   - GET /admin/audit/logs   — 分页查询审计日志（支持 username / action 筛选）
//   - GET /admin/audit/stats  — 审计统计（按 action 分组 + 近 7 天每日操作量）
package audit

import (
	"strconv"

	"ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// ListAuditLogs 分页查询审计日志。
//
// GET /admin/audit/logs?pageNo=1&pageSize=20&username=xxx&action=xxx
func ListAuditLogs(c *gin.Context) {
	pageNo, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	username := c.Query("username")
	action := c.Query("action")

	logs, total, err := models.QueryAuditLogs(pageNo, pageSize, username, action)
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询审计日志失败")
		return
	}

	api.Paginated(c, "查询审计日志成功", logs, total, pageNo, pageSize)
}

// AuditStats 审计统计响应体。
type AuditStats struct {
	ByAction []models.AuditStatByAction `json:"byAction"`
	ByDate   []models.AuditStatByDate   `json:"byDate"`
}

// GetAuditStats 返回审计统计数据（按 action 分组计数 + 近 7 天每日操作量）。
//
// GET /admin/audit/stats
func GetAuditStats(c *gin.Context) {
	byAction, byDate, err := models.QueryAuditStats()
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询审计统计失败")
		return
	}

	api.Success(c, "查询审计统计成功", AuditStats{
		ByAction: byAction,
		ByDate:   byDate,
	})
}
