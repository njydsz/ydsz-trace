// Package admin 子文件：日志生命周期管理接口（V2.4-04，FR-305）。
//
// 提供三类接口：
//   GET  /admin/lifecycle/plan    预览分层计划（dry-run，不执行）
//   POST /admin/lifecycle/apply   执行分层处理（body: {"dryRun": true}）
//   GET  /admin/lifecycle/stats   分层存储统计
//
// 安全约束：
//   - apply 默认 dryRun=true，必须显式传 false 才会真实执行
//   - 计划与统计接口可安全反复调用
package admin

import (
	"os"
	"time"

	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/perf"

	"github.com/gin-gonic/gin"
)

// lifecyclePlanReq 计划查询请求（query 参数）。
type lifecyclePlanReq struct {
	// Dir 待扫描目录（为空时用服务端配置的日志根目录）
	Dir string
}

// LifecyclePlanHandler GET /admin/lifecycle/plan
//
// 预览分层计划，不执行任何文件操作。
func LifecyclePlanHandler(c *gin.Context) {
	dir := c.Query("dir")
	if dir == "" {
		dir = defaultLogRoot()
	}

	policy := perf.DefaultLifecyclePolicy()
	if err := policy.Validate(); err != nil {
		api.Fail(c, api.CodeServerError, "生命周期策略非法")
		return
	}

	reports, err := perf.PlanLifecycle(dir, policy)
	if err != nil {
		api.Fail(c, api.CodeServerError, "生成计划失败")
		return
	}

	api.Success(c, "计划生成成功", gin.H{
		"policy":  policy,
		"reports": reports,
		"stats":   perf.Summarize(reports),
	})
}

// lifecycleApplyReq 执行请求。
type lifecycleApplyReq struct {
	// Dir 待扫描目录（为空时用默认根目录）
	Dir string `json:"dir"`
	// DryRun 是否为干跑（默认 true，避免误操作）
	DryRun bool `json:"dryRun"`
}

// LifecycleApplyHandler POST /admin/lifecycle/apply
//
// 执行分层处理。默认 dry-run，需显式传 dryRun=false 才真实操作文件。
func LifecycleApplyHandler(c *gin.Context) {
	var req lifecycleApplyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "参数非法")
		return
	}

	dir := req.Dir
	if dir == "" {
		dir = defaultLogRoot()
	}

	// 安全默认：未显式指定 dryRun 时按 true 处理（请求体未带该字段时为零值 false，
	// 因此用指针区分「未传」与「传 false」）
	dryRun := true
	if v, present := c.GetQuery("dryRun"); present && (v == "false" || v == "0") {
		dryRun = false
	}

	policy := perf.DefaultLifecyclePolicy()
	if err := policy.Validate(); err != nil {
		api.Fail(c, api.CodeServerError, "生命周期策略非法")
		return
	}

	applied, skipped, err := perf.ApplyLifecycle(dir, policy, dryRun)
	if err != nil {
		api.Fail(c, api.CodeServerError, "执行失败")
		return
	}

	api.Success(c, "执行完成", gin.H{
		"dryRun":  dryRun,
		"applied": applied,
		"skipped": skipped,
		"policy":  policy,
	})
}

// LifecycleStatsHandler GET /admin/lifecycle/stats
//
// 返回当前分层存储统计（只读，不产生文件操作）。
func LifecycleStatsHandler(c *gin.Context) {
	dir := c.Query("dir")
	if dir == "" {
		dir = defaultLogRoot()
	}

	reports, err := perf.PlanLifecycle(dir, perf.DefaultLifecyclePolicy())
	if err != nil {
		api.Fail(c, api.CodeServerError, "统计失败")
		return
	}

	stats := perf.Summarize(reports)
	api.Success(c, "查询成功", gin.H{
		"stats":        stats,
		"humanHot":     perf.HumanBytes(stats.HotBytes),
		"humanWarm":    perf.HumanBytes(stats.WarmBytes),
		"humanCold":    perf.HumanBytes(stats.ColdBytes),
		"humanTotal":   perf.HumanBytes(stats.TotalBytes),
		"checkedAt":    time.Now().Format("2006-01-02 15:04:05"),
	})
}

// defaultLogRoot 返回默认日志根目录。
//
// 优先级：YDSZ_LOG_ROOT 环境变量 > 当前目录 logs。
func defaultLogRoot() string {
	if v := os.Getenv("YDSZ_LOG_ROOT"); v != "" {
		return v
	}
	return "logs"
}
