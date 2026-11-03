// Package logs 子文件：洞察看板数据接口（FR-1203）。
//
// 为控制台首页提供模板趋势、异常事件时间线、服务健康度三类洞察数据，
// 前端一次性加载即可渲染全貌，无需多次请求。
package logs

import (
	"strconv"
	"sync"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// DashboardInsight 响应体：模板趋势 + 错误时间线 + 服务健康度 + 异常概览（V2.2）。
type DashboardInsight struct {
	TemplateTrends  []TemplateTrendItem  `json:"templateTrends"`
	ErrorTimeline   []ErrorTimelinePoint `json:"errorTimeline"`
	ServiceHealth   ServiceHealthInfo   `json:"serviceHealth"`
	// AnomalyOverview 异常概览（V2.2-06）：总数 / 按类型分布 / 最近 5 条，
	// 供首页直接展示异常态势，无需跳转到告警页。
	AnomalyOverview AnomalyOverview `json:"anomalyOverview"`
	// FeedbackStats 反馈统计（V2.2-06）：误报/确认/忽略计数，反映阈值自适应状态。
	FeedbackStats FeedbackSummary `json:"feedbackStats"`
}

// AnomalyOverview 异常概览。
type AnomalyOverview struct {
	Total       int            `json:"total"`
	ByType      map[string]int `json:"byType"`
		Recent      []AnomalyBrief `json:"recent"`
}

// AnomalyBrief 异常摘要（列表项）。
type AnomalyBrief struct {
	ID           int64   `json:"id"`
	Time         string  `json:"time"`
	TemplateHash string  `json:"templateHash"`
	Type         string  `json:"type"`
	Score        float64 `json:"score"`
	Description  string  `json:"description"`
}

// FeedbackSummary 反馈统计摘要。
type FeedbackSummary struct {
	FalsePositive int `json:"falsePositive"`
	TruePositive  int `json:"truePositive"`
	Ignored       int `json:"ignored"`
}

// TemplateTrendItem 模板趋势条目。
type TemplateTrendItem struct {
	Hash      string `json:"hash"`
	Count     int    `json:"count"`
	Sample    string `json:"sample"`
	LastTime  string `json:"lastTime"`
	Level     string `json:"level"`
}

// ErrorTimelinePoint 错误时间线节点（5min 桶聚合）。
type ErrorTimelinePoint struct {
	TimeBucket string `json:"timeBucket"`
	Count     int    `json:"count"`
}

// ServiceHealthInfo 服务健康度总览。
type ServiceHealthInfo struct {
	TotalClients int              `json:"totalClients"`
	OnlineClients int             `json:"onlineClients"`
	TotalItems   int              `json:"totalItems"`
	TasksRecent  TasksRecentStat  `json:"tasksRecent"`
	ClientHealth []ClientHealth   `json:"clientHealth"`
}

// TasksRecentStat 近期任务统计。
type TasksRecentStat struct {
	Total   int `json:"total"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
}

// ClientHealth 单个客户端健康状态。
type ClientHealth struct {
	ID     int64  `json:"id"`
	Label  string `json:"label"`
	Online bool   `json:"online"`
}

// DashboardInsightHandler 洞察看板数据端点。
//
// @Summary 洞察看板
// @Description 为控制台首页提供模板趋势、异常事件时间线、服务健康度三类洞察数据，一次加载即可渲染全貌。
// @Tags Dashboard
// @Produce json
// @Param timeRange query string false "时间范围（默认 24h，如 1h/6h/24h/7d）"
// @Param itemId query int false "日志项 ID（0 表示使用第一个可用项）"
// @Success 200 {object} api.Response{data=DashboardInsight}
// @Router /logs/dashboard/insight [get]
func DashboardInsightHandler(c *gin.Context) {
	timeRange := c.DefaultQuery("timeRange", "24h")
	itemID, _ := strconv.ParseInt(c.Query("itemId"), 10, 64)

	// 三个查询相互独立，使用 sync.WaitGroup 并行执行以降低总体延迟。
	var (
		wg       sync.WaitGroup
		trends   []TemplateTrendItem
		timeline []ErrorTimelinePoint
		health   ServiceHealthInfo
		anomaly  AnomalyOverview
		feedback FeedbackSummary
	)
	wg.Add(5)
	go func() {
		defer wg.Done()
		trends = queryTemplateTrends(itemID, timeRange)
	}()
	go func() {
		defer wg.Done()
		// 时间线范围跟随 timeRange（V2.2-06：原先固定 1 小时，与看板切换不匹配）
		timeline = queryErrorTimelineRange(timeRange)
	}()
	go func() {
		defer wg.Done()
		health = queryServiceHealth()
	}()
	go func() {
		defer wg.Done()
		anomaly = queryAnomalyOverview(timeRange)
	}()
	go func() {
		defer wg.Done()
		feedback = queryFeedbackSummary()
	}()
	wg.Wait()

	api.Success(c, "查询成功", DashboardInsight{
		TemplateTrends:  trends,
		ErrorTimeline:   timeline,
		ServiceHealth:   health,
		AnomalyOverview: anomaly,
		FeedbackStats:   feedback,
	})
}

// queryTemplateTrends 查询 Top 模板趋势。
func queryTemplateTrends(itemID int64, timeRange string) []TemplateTrendItem {
	if itemID <= 0 {
		// 未指定 item 时使用第 1 个可用 item
		items, _ := models.QueryAllItem()
		if len(items) > 0 {
			itemID = items[0].Id
		}
	}
	patterns, err := models.TopPatterns(itemID, timeRange, 10)
	if err != nil {
		return []TemplateTrendItem{}
	}
	items := make([]TemplateTrendItem, 0, len(patterns))
	for _, p := range patterns {
		level := extractLevel(p.Sample)
		items = append(items, TemplateTrendItem{
			Hash:     p.TemplateHash,
			Count:    p.Count,
			Sample:   truncateStr(p.Sample, 120),
			LastTime: p.LastTime,
			Level:    level,
		})
	}
	return items
}

// queryErrorTimeline 查询事件时间线（保持向后兼容，默认近 1 小时）。
func queryErrorTimeline() []ErrorTimelinePoint {
	return queryErrorTimelineRange("1h")
}

// queryErrorTimelineRange 按时间范围查询错误时间线（V2.2-06）。
//
// 支持 1h / 6h / 24h / 7d；非法值回退 24h。
// 数据源为 t_pattern_stat（含 ERROR/FATAL 关键词的样本），按 5min 桶聚合。
func queryErrorTimelineRange(timeRange string) []ErrorTimelinePoint {
	duration, ok := parseTimeRange(timeRange)
	if !ok {
		duration = 24 * time.Hour
	}

	bucketStart := time.Now().Add(-duration)
	var bucketStr string
	if b, err := models.TimeBucket(bucketStart.Format("2006-01-02 15:04:05")); err == nil {
		bucketStr = b
	} else {
		return []ErrorTimelinePoint{}
	}

	var rows []struct {
		TimeBucket string `db:"time_bucket"`
		Count      int    `db:"count"`
	}
	// 按 error 关键词过滤：sample 中含 ERROR 视为异常事件。
	err := models.DB.Select(&rows,
		`SELECT time_bucket, SUM(count) AS count
		 FROM t_pattern_stat
		 WHERE time_bucket >= ? AND (
		 	sample LIKE '%ERROR%' OR sample LIKE '%FATAL%'
		 )
		 GROUP BY time_bucket
		 ORDER BY time_bucket ASC`,
		bucketStr)
	if err != nil {
		return []ErrorTimelinePoint{}
	}
	points := make([]ErrorTimelinePoint, 0, len(rows))
	for _, r := range rows {
		points = append(points, ErrorTimelinePoint{
			TimeBucket: formatBucket(r.TimeBucket),
			Count:      r.Count,
		})
	}
	return points
}

// parseTimeRange 将 timeRange 字符串解析为时长。
func parseTimeRange(s string) (time.Duration, bool) {
	switch s {
	case "1h":
		return time.Hour, true
	case "6h":
		return 6 * time.Hour, true
	case "24h":
		return 24 * time.Hour, true
	case "7d":
		return 7 * 24 * time.Hour, true
	}
	return 0, false
}

// queryAnomalyOverview 查询异常概览（V2.2-06）。
func queryAnomalyOverview(timeRange string) AnomalyOverview {
	overview := AnomalyOverview{ByType: map[string]int{}}

	duration, ok := parseTimeRange(timeRange)
	if !ok {
		duration = 24 * time.Hour
	}
	from := time.Now().Add(-duration).Format("2006-01-02 15:04:05")
	now := time.Now().Format("2006-01-02 15:04:05")

	// 按类型分组计数
	var typeRows []struct {
		AnomalyType string `db:"anomaly_type"`
		Count       int    `db:"count"`
	}
	if err := models.DB.Select(&typeRows, `SELECT anomaly_type, COUNT(*) AS count
		FROM t_anomaly_event WHERE created_time BETWEEN ? AND ?
		GROUP BY anomaly_type`, from, now); err == nil {
		for _, r := range typeRows {
			overview.ByType[r.AnomalyType] = r.Count
			overview.Total += r.Count
		}
	}

	// 最近 5 条异常
	events, err := models.QueryAnomalyEvents(0, from, now, 5)
	if err == nil {
		recent := make([]AnomalyBrief, 0, len(events))
		for _, ev := range events {
			recent = append(recent, AnomalyBrief{
				ID:           ev.Id,
				Time:         ev.CreatedTime,
				TemplateHash: ev.TemplateId,
				Type:         ev.AnomalyType,
				Score:        ev.Score,
				Description:  truncateStr(ev.Description, 120),
			})
		}
		overview.Recent = recent
	}

	return overview
}

// queryFeedbackSummary 查询反馈统计摘要（V2.2-06）。
func queryFeedbackSummary() FeedbackSummary {
	fp, tp, ig := models.QueryAnomalyFeedbackStats()
	return FeedbackSummary{
		FalsePositive: fp,
		TruePositive:  tp,
		Ignored:       ig,
	}
}

// queryRecentTasksStats 查询最近 N 条任务统计。
func queryRecentTasksStats(limit int) TasksRecentStat {
	stat := TasksRecentStat{}
	// QueryTasks 按 id 倒序，取第一页
	page := models.QueryTasks(1, limit, "")
	tasks, ok := page.List.([]models.TTask)
	if !ok {
		return stat
	}
	for _, t := range tasks {
		stat.Total++
		switch t.Status {
		case models.TaskStatusSuccess:
			stat.Success++
		case models.TaskStatusFailed:
			stat.Failed++
		}
	}
	return stat
}

// queryServiceHealth 查询服务健康度。
func queryServiceHealth() ServiceHealthInfo {
	health := ServiceHealthInfo{}

	clients, _ := models.QueryAllClient()
	health.TotalClients = len(clients)
	for _, cl := range clients {
		online := cl.Online == "1"
		if online {
			health.OnlineClients++
		}
		health.ClientHealth = append(health.ClientHealth, ClientHealth{
			ID:     cl.Id,
			Label:  cl.Ip + ":" + cl.Port,
			Online: online,
		})
	}

	items, _ := models.QueryAllItem()
	health.TotalItems = len(items)
	health.TasksRecent = queryRecentTasksStats(100)

	return health
}

// formatBucket 将 14 位 bucket (YYYYMMDDHHMI) 格式化为 HH:MI。
func formatBucket(bucket string) string {
	if len(bucket) < 12 {
		return bucket
	}
	hour := bucket[8:10]
	min := bucket[10:12]
	return hour + ":" + min
}

// truncateStr 截断字符串到 max 长度。
func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
