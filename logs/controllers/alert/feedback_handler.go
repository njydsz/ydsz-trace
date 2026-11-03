// Package alert 子文件：异常反馈与静默窗口的 HTTP 处理器（V2.2-03/04）。
//
// 路由（在 routers/router.go 注册）：
//   POST   /logs/anomalies/feedback            标记异常反馈（误报/确认/忽略）
//   GET    /logs/anomalies/feedback/stats      反馈统计（供看板）
//   GET    /logs/anomalies/timeline            异常时间线（供前端标注下钻）
//   GET    /logs/maintenance-windows           静默窗口列表
//   POST   /logs/maintenance-windows           全量保存静默窗口
//
// 安全约定：
//   - 反馈写入与窗口配置均为 operator 权限（路由层由 RequireOperator 保证）
//   - 不暴露内部错误堆栈（统一走 api.Fail）
package alert

import (
	"strconv"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/session"

	"github.com/gin-gonic/gin"
)

// feedbackReq 异常反馈请求体。
type feedbackReq struct {
	AnomalyID    int64  `json:"anomalyId" binding:"required"`
	TemplateHash string `json:"templateHash" binding:"required"`
	ItemID       int64  `json:"itemId"`
	Type         string `json:"type" binding:"required"` // false_positive / true_positive / ignored
	Reason       string `json:"reason"`
}

// validFeedbackType 校验反馈类型合法性。
func validFeedbackType(t string) (models.FeedbackType, bool) {
	switch models.FeedbackType(t) {
	case models.FeedbackFalsePositive:
		return models.FeedbackFalsePositive, true
	case models.FeedbackTruePositive:
		return models.FeedbackTruePositive, true
	case models.FeedbackIgnored:
		return models.FeedbackIgnored, true
	}
	return "", false
}

// currentOperator 从会话中提取当前操作人（失败时返回空串）。
func currentOperator(c *gin.Context) string {
	if v := session.GetString(c, "username"); v != "" {
		return v
	}
	return "unknown"
}

// MarkFeedback POST /logs/anomalies/feedback
//
// 标记异常反馈并触发该模板的阈值自适应。
func MarkFeedback(c *gin.Context) {
	var req feedbackReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "参数非法")
		return
	}

	fbType, ok := validFeedbackType(req.Type)
	if !ok {
		api.Fail(c, api.CodeBadRequest, "反馈类型非法（应为 false_positive / true_positive / ignored）")
		return
	}

	// 读取触发本次异常的 score 作为阈值上调基准
	triggerScore := 0.0
	if ev, err := models.GetAnomalyEvent(req.AnomalyID); err == nil {
		triggerScore = ev.Score
	}

	if err := MarkAnomalyFeedback(req.AnomalyID, req.TemplateHash, req.ItemID,
		fbType, req.Reason, triggerScore, currentOperator(c)); err != nil {
		api.Fail(c, api.CodeServerError, "记录反馈失败")
		return
	}

	threshold, _ := models.GetAdaptiveThreshold(req.TemplateHash)
	api.Success(c, "反馈已记录", gin.H{
		"templateHash": req.TemplateHash,
		"type":         fbType,
		"threshold":    threshold,
	})
}

// FeedbackStats GET /logs/anomalies/feedback/stats
//
// 返回反馈统计与自适应阈值概览，供洞察看板使用。
func FeedbackStats(c *gin.Context) {
	stats := GetFeedbackStats()
	api.Success(c, "查询成功", stats)
}

// timelineReq 异常时间线查询参数。
type timelineReq struct {
	ItemID   int64
	From     time.Time
	To       time.Time
	Template string
	PageNo   int
	PageSize int
}

// parseTimelineReq 解析时间线查询参数。
//
// 默认时间范围为最近 24 小时；pageSize 上限 200。
func parseTimelineReq(c *gin.Context) timelineReq {
	itemID, _ := strconv.ParseInt(c.Query("itemId"), 10, 64)

	to := time.Now()
	from := to.Add(-24 * time.Hour)

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

	pageNo, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "50"))
	if pageNo <= 0 {
		pageNo = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}

	return timelineReq{
		ItemID:   itemID,
		From:     from,
		To:       to,
		Template: c.Query("template"),
		PageNo:   pageNo,
		PageSize: pageSize,
	}
}

// TimelinePoint 异常时间线单个数据点。
type TimelinePoint struct {
	ID           int64   `json:"id"`
	Time         string  `json:"time"`
	TemplateHash string  `json:"templateHash"`
	ItemID       int64   `json:"itemId"`
	Type         string  `json:"type"`
	Score        float64 `json:"score"`
	Actual       float64 `json:"actual"`
	Baseline     float64 `json:"baseline"`
	Status       string  `json:"status"`
	Description  string  `json:"description"`
}

// AnomalyTimeline GET /logs/anomalies/timeline
//
// 返回异常事件列表，供前端在时间线上标注并支持点击下钻原文。
func AnomalyTimeline(c *gin.Context) {
	req := parseTimelineReq(c)

	events, err := models.QueryAnomalyEvents(req.ItemID, req.From.Format("2006-01-02 15:04:05"),
		req.To.Format("2006-01-02 15:04:05"), req.PageSize)
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询异常时间线失败")
		return
	}

	points := make([]TimelinePoint, 0, len(events))
	for _, ev := range events {
		if req.Template != "" && ev.TemplateId != req.Template {
			continue
		}
		points = append(points, TimelinePoint{
			ID:           ev.Id,
			Time:         ev.CreatedTime,
			TemplateHash: ev.TemplateId,
			ItemID:       ev.ItemId,
			Type:         ev.AnomalyType,
			Score:        ev.Score,
			Actual:       ev.Actual,
			Baseline:     ev.Baseline,
			Status:       ev.Status,
			Description:  ev.Description,
		})
	}

	api.Success(c, "查询成功", gin.H{
		"points":  points,
		"total":   len(points),
		"from":    req.From.Format("2006-01-02 15:04:05"),
		"to":      req.To.Format("2006-01-02 15:04:05"),
		"hasMore": len(events) >= req.PageSize,
	})
}

// maintenanceWindowReq 静默窗口保存请求体（全量替换）。
type maintenanceWindowReq struct {
	Windows []maintenanceWindowDTO `json:"windows"`
}

// maintenanceWindowDTO 静默窗口传输对象。
type maintenanceWindowDTO struct {
	Name      string   `json:"name" binding:"required"`
	StartTime string   `json:"startTime" binding:"required"` // HH:MM
	EndTime   string   `json:"endTime" binding:"required"`   // HH:MM
	Recurring string   `json:"recurring"`                    // daily / weekly / once
	Weekdays  []int    `json:"weekdays,omitempty"`
	Enabled   bool     `json:"enabled"`
}

// validHHMM 校验 HH:MM 格式。
func validHHMM(s string) bool {
	if len(s) != 5 || s[2] != ':' {
		return false
	}
	h, err1 := strconv.Atoi(s[:2])
	m, err2 := strconv.Atoi(s[3:])
	return err1 == nil && err2 == nil && h >= 0 && h < 24 && m >= 0 && m < 60
}

// ListMaintenanceWindows GET /logs/maintenance-windows
func ListMaintenanceWindows(c *gin.Context) {
	windows := models.QueryMaintenanceWindows()
	api.Success(c, "查询成功", gin.H{"windows": windows})
}

// SaveMaintenanceWindows POST /logs/maintenance-windows（全量替换）
func SaveMaintenanceWindows(c *gin.Context) {
	var req maintenanceWindowReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "参数非法")
		return
	}

	for i, w := range req.Windows {
		if !validHHMM(w.StartTime) || !validHHMM(w.EndTime) {
			api.Fail(c, api.CodeBadRequest, "时间格式非法（应为 HH:MM）")
			return
		}
		recurring := w.Recurring
		if recurring == "" {
			recurring = "daily"
		}
		if recurring != "daily" && recurring != "weekly" && recurring != "once" {
			api.Fail(c, api.CodeBadRequest, "recurring 非法（应为 daily / weekly / once）")
			return
		}
		if recurring == "weekly" && len(w.Weekdays) == 0 {
			api.Fail(c, api.CodeBadRequest, "weekly 窗口必须指定 weekdays")
			return
		}
		_ = i
	}

	if err := models.ReplaceMaintenanceWindows(maintenanceDTOToModel(req.Windows)); err != nil {
		api.Fail(c, api.CodeServerError, "保存静默窗口失败")
		return
	}

	// 同步刷新调度器内的静默窗口（若调度器已注入）
	api.Success(c, "保存成功", gin.H{"count": len(req.Windows)})
}

// maintenanceDTOToModel 将 DTO 转为模型对象。
func maintenanceDTOToModel(dtos []maintenanceWindowDTO) []models.MaintenanceWindow {
	out := make([]models.MaintenanceWindow, 0, len(dtos))
	for _, d := range dtos {
		recurring := d.Recurring
		if recurring == "" {
			recurring = "daily"
		}
		out = append(out, models.MaintenanceWindow{
			Name:      d.Name,
			StartTime: d.StartTime,
			EndTime:   d.EndTime,
			Recurring: recurring,
			Weekdays:  d.Weekdays,
			Enabled:   d.Enabled,
		})
	}
	return out
}

// SyncMaintenanceWindows POST /logs/maintenance-windows/sync
//
// 将数据库中的静默窗口同步到运行中的调度器（配置变更后手动触发，
// 避免每次保存都依赖全局单例）。
func SyncMaintenanceWindows(c *gin.Context) {
	windows := models.QueryMaintenanceWindows()

	// 通过注册表找到运行中的调度器（由 main.go 注入）
	sched := GetRunningScheduler()
	if sched == nil {
		api.Fail(c, api.CodeServerError, "调度器未初始化")
		return
	}
	if sched.Suppressor() == nil {
		sched.SetSuppressor(NewSuppressor())
	}
	sched.Suppressor().ReplaceWindows(toAlertWindows(windows))

	api.Success(c, "同步成功", gin.H{"count": len(windows)})
}

// maintenanceDTOToModelFromModel 将模型转为 alert 包的窗口结构。
func maintenanceDTOToModelFromModel(ws []models.MaintenanceWindow) []MaintenanceWindow {
	out := make([]MaintenanceWindow, 0, len(ws))
	for _, w := range ws {
		out = append(out, MaintenanceWindow{
			Name:      w.Name,
			Start:     parseClock(w.StartTime),
			End:       parseClock(w.EndTime),
			Recurring: w.Recurring,
			Weekdays:  w.Weekdays,
			Enabled:   w.Enabled,
		})
	}
	return out
}

// parseClock 将 "HH:MM" 转为仅含时分的时间值。
func parseClock(hhmm string) time.Time {
	if !validHHMM(hhmm) {
		return time.Time{}
	}
	h, _ := strconv.Atoi(hhmm[:2])
	m, _ := strconv.Atoi(hhmm[3:])
	return time.Date(0, 1, 1, h, m, 0, 0, time.Local)
}

// toAlertWindows 类型别名转换（models.MaintenanceWindow → alert.MaintenanceWindow）。
func toAlertWindows(ws []models.MaintenanceWindow) []MaintenanceWindow {
	return maintenanceDTOToModelFromModel(ws)
}
