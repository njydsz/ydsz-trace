// Package system 提供系统级管理接口（健康检查、依赖探测）。
//
// 分级健康检查：
//
//	GET /api/v2/health               基础存活探针（无依赖检查，极速响应）
//	GET /api/v2/health/dependencies  依赖就绪探针（检查 SQLite 连通性）
//
// 设计原则：
//   - /health 仅在进程存活时返回 200，不检查外部依赖（K8s liveness）。
//   - /dependencies 返回各依赖状态，任一 down 则整体返回 503（K8s readiness）。
package system

import (
	"time"

	"ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// dependencyStatus 单个依赖的健康状态。
type dependencyStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "up" | "down"
	Error  string `json:"error,omitempty"`
}

// healthResponse /api/v2/health 响应数据体。
type healthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Timestamp string `json:"timestamp"`
	Uptime    string `json:"uptime,omitempty"`
}

// dependenciesResponse /api/v2/health/dependencies 响应数据体。
type dependenciesResponse struct {
	Status       string             `json:"status"`
	Dependencies []dependencyStatus `json:"dependencies"`
}

// startTime 全局启动时间，用于计算 uptime。
var startTime = time.Now()

// Health 基础存活探针。
//
// 仅返回服务标识与时间戳，不涉及任何 I/O。
// K8s liveness probe 配置指向此端点。
func Health(c *gin.Context) {
	api.Success(c, "", healthResponse{
		Status:    "ok",
		Version:   "2.1.0",
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Uptime:    time.Since(startTime).Truncate(time.Second).String(),
	})
}

// HealthDependencies 分级依赖就绪探针。
//
// 探测 SQLite 数据库连通性。全部 up 返回 200，任一 down 返回 503。
// K8s readiness probe 配置指向此端点。
func HealthDependencies(c *gin.Context) {
	deps := []dependencyStatus{
		checkSQLite(),
	}

	allUp := true
	for _, d := range deps {
		if d.Status != "up" {
			allUp = false
			break
		}
	}

	resp := dependenciesResponse{
		Status:       "ok",
		Dependencies: deps,
	}
	if !allUp {
		resp.Status = "degraded"
		api.Fail(c, api.CodeServerError, "依赖异常")
		return
	}
	api.Success(c, "", resp)
}

// checkSQLite 探测 SQLite 数据库连通性。
func checkSQLite() dependencyStatus {
	var one int
	if err := models.DB.Get(&one, "SELECT 1"); err != nil {
		return dependencyStatus{
			Name:   "sqlite",
			Status: "down",
			Error:  err.Error(),
		}
	}
	return dependencyStatus{
		Name:   "sqlite",
		Status: "up",
	}
}
