// Package logs 子文件：洞察看板数据接口（FR-1203）。
//
// 为控制台首页提供模板趋势、异常事件时间线、服务健康度三类洞察数据，
// 前端一次性加载即可渲染全貌，无需多次请求。
package logs

import (
	"strconv"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// DashboardInsight 响应体：模板趋势 + 异常事件时间线 + 服务健康度。
type DashboardInsight struct {
	TemplateTrends  []TemplateTrendItem  `json:"templateTrends"`
	ErrorTimeline   []ErrorTimelinePoint `json:"errorTimeline"`
	ServiceHealth   ServiceHealthInfo   `json:"serviceHealth"`
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
// GET /logs/dashboard/insight?timeRange=24h&itemId=0
func DashboardInsightHandler(c *gin.Context) {
	timeRange := c.DefaultQuery("timeRange", "24h")
	itemID, _ := strconv.ParseInt(c.Query("itemId"), 10, 64)

	// 1. 模板趋势：Top 10
	trends := queryTemplateTrends(itemID, timeRange)

	// 2. 异常事件时间线（近 1 小时 ERROR 桶聚合）
	timeline := queryErrorTimeline()

	// 3. 服务健康度
	health := queryServiceHealth()

	api.Success(c, "查询成功", DashboardInsight{
		TemplateTrends: trends,
		ErrorTimeline:  timeline,
		ServiceHealth:  health,
	})
}

// queryTemplateTrends 查询 Top 模板趋势。
func queryTemplateTrends(itemID int64, timeRange string) []TemplateTrendItem {
	if itemID <= 0 {
		// 未指定 item 时使用第 1 个可用 item
		items := models.ReadAllItems()
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

// queryErrorTimeline 查询近 1 小时 ERROR 事件时间线（5min 桶聚合）。
func queryErrorTimeline() []ErrorTimelinePoint {
	bucketStart := time.Now().Add(-1 * time.Hour)
	minute := (bucketStart.Minute() / 5) * 5
	bucket := strconv.Itoa(bucketStart.Year()) +
		strconv.Itoa(int(bucketStart.Month())) +
		strconv.Itoa(bucketStart.Day()) +
		strconv.Itoa(bucketStart.Hour()) +
		strconv.Itoa(minute)

	var rows []struct {
		TimeBucket string `db:"time_bucket"`
		Count     int    `db:"count"`
	}
	err := models.DB.Select(&rows,
		`SELECT time_bucket, SUM(count) AS count
		 FROM t_pattern_stat
		 WHERE time_bucket >= ?
		 GROUP BY time_bucket
		 ORDER BY time_bucket ASC`,
		bucket)
	if err != nil {
		return []ErrorTimelinePoint{}
	}
	points := make([]ErrorTimelinePoint, 0, len(rows))
	for _, r := range rows {
		points = append(points, ErrorTimelinePoint{
			TimeBucket: formatBucket(r.TimeBucket),
			Count:     r.Count,
		})
	}
	return points
}

// queryServiceHealth 查询服务健康度。
func queryServiceHealth() ServiceHealthInfo {
	health := ServiceHealthInfo{}

	clients := models.QueryAllClientSafe()
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

	items := models.ReadAllItems()
	health.TotalItems = len(items)

	tasks := models.QueryRecentTasks(100)
	for _, t := range tasks {
		health.TasksRecent.Total++
		switch t.Status {
		case models.TaskStatusSuccess:
			health.TasksRecent.Success++
		case models.TaskStatusFailed:
			health.TasksRecent.Failed++
		}
	}

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
