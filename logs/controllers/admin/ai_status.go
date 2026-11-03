// Package admin 子文件：AI 四层状态面板。
//
// 暴露 L0-L3 智能漏斗的运行状态（active/inactive/degraded），
// 近 24h 调用量分别按层回填。供前端 AIStatusPanel 组件展示。
package admin

import (
	"sync/atomic"
	"time"

	"ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// aiLayerStatus 单层运行状态。
type aiLayerStatus struct {
	Name         string `json:"name"`
	Layer        string `json:"layer"`
	Status       string `json:"status"` // "active" | "inactive" | "degraded"
	CallCount24H int64  `json:"callCount24H"`
	Model        string `json:"model,omitempty"`
	Description  string `json:"description,omitempty"`
	// Algorithms 该层实际使用的算法清单（与实现对齐，避免文档误导）。
	Algorithms []string `json:"algorithms,omitempty"`
}

// aiStatusResponse 整体 AI 状态响应。
type aiStatusResponse struct {
	GeneratedAt string           `json:"generatedAt"`
	Layers      []aiLayerStatus  `json:"layers"`
	Total24H    int64            `json:"total24H"`
}

// 近 24h 调用计数（运行时累加，非持久化）。
var (
	embeddingCallCount int64
	nl2dslCallCount    int64
)

// IncrementEmbeddingCall 记录一次嵌入调用。
func IncrementEmbeddingCall() {
	atomic.AddInt64(&embeddingCallCount, 1)
}

// IncrementNL2DSLCall 记录一次 NL2DSL 调用。
func IncrementNL2DSLCall() {
	atomic.AddInt64(&nl2dslCallCount, 1)
}

// ResetDailyCounters 重置日计数器（由定时任务每日 0 点调用）。
func ResetDailyCounters() {
	atomic.StoreInt64(&embeddingCallCount, 0)
	atomic.StoreInt64(&nl2dslCallCount, 0)
}

// AIStatusHandler GET /admin/ai/status — AI 四层面板状态。
//
// @Summary AI 四层状态面板
// @Description 展示 L0-L3 规则/统计/嵌入/生成层的运行状态与调用量。
// @Tags Admin
// @Produce json
// @Success 200 {object} api.Response{data=aiStatusResponse}
// @Router /admin/ai/status [get]
func AIStatusHandler(c *gin.Context) {
	layers := []aiLayerStatus{
		{
			Name:         "L0 规则层",
			Layer:        "rule",
			Status:       "active",
			CallCount24H: nl2dslCallCount,
			Description:  "正则 + 模板匹配 + 字段过滤",
		},
		{
			Name:         "L1 统计层",
			Layer:        "stats",
			Status:       detectStatsLayerStatus(),
			CallCount24H: 0,
			Description:  "滑动窗口基线 + Z-score 突变检测 + 泊松阈值",
			Algorithms:   []string{"z-score", "poisson"},
		},
		{
			Name:         "L2 嵌入层",
			Layer:        "embedding",
			Status:       detectEmbeddingLayerStatus(),
			CallCount24H: embeddingCallCount,
			Model:        models.GetDefaultEmbeddingModelName(),
			Description:  "模板/样本向量化 + 余弦相似度检索",
			Algorithms:   []string{"cosine"},
		},
		{
			Name:         "L3 生成层",
			Layer:        "generation",
			Status:       detectGenerationLayerStatus(),
			CallCount24H: nl2dslCallCount,
			Description:  "NL2DSL 规则解析（根因推理/报告生成待 V2.3）",
			Algorithms:   []string{"regex"},
		},
	}

	var total24H int64
	for _, l := range layers {
		total24H += l.CallCount24H
	}

	api.Success(c, "", aiStatusResponse{
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),
		Layers:      layers,
		Total24H:    total24H,
	})
}

// detectStatsLayerStats 检测统计层是否激活（有最近 1h 数据即视为 active）。
func detectStatsLayerStatus() string {
	var count int
	if err := models.DB.Get(&count, "SELECT COUNT(*) FROM t_pattern_stat WHERE time_bucket > datetime('-1 hour')"); err != nil {
		return "degraded"
	}
	if count > 0 {
		return "active"
	}
	return "inactive"
}

// detectGenerationLayerStatus 检测生成层状态。
//
// 当前 L3 仅有规则实现的 NL2DSL（无 LLM 兜底），因此：
//   - 有调用记录 → active（规则路径可用）
//   - 无调用记录 → inactive（尚未产生流量，非故障）
//
// 待 V2.3 接入 LLM 后改为检测 Provider 可用性。
func detectGenerationLayerStatus() string {
	if atomic.LoadInt64(&nl2dslCallCount) > 0 {
		return "active"
	}
	return "inactive"
}

// detectEmbeddingLayerStatus 检测嵌入层状态。
func detectEmbeddingLayerStatus() string {
	if atomic.LoadInt64(&embeddingCallCount) > 0 {
		return "active"
	}
	var count int
	if err := models.DB.Get(&count, "SELECT COUNT(*) FROM t_embedding"); err != nil {
		return "degraded"
	}
	if count > 0 {
		return "active"
	}
	return "inactive"
}
