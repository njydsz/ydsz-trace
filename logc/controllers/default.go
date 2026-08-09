// Package controllers 包含 logc 客户端的公共控制器（健康检查等）。
package controllers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// HealthResp 健康检查响应结构。
type HealthResp struct {
	// Status 状态标识：ok / ready
	Status string `json:"status"`
	// App 服务标识
	App string `json:"app"`
	// Time 当前服务器本地时间
	Time string `json:"time"`
}

// Main 返回一行纯文本，表示服务存活。
func Main(c *gin.Context) {
	c.String(http.StatusOK, "Ydsz Trace logc agent is running.")
}

// Health 返回存活探针结果（K8s liveness probe 使用）。
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResp{
		Status: "ok",
		App:    "ydsz-trace-logc",
		Time:   time.Now().Format("2006-01-02 15:04:05"),
	})
}

// Ready 返回就绪探针结果（K8s readiness probe 使用）。
func Ready(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResp{
		Status: "ready",
		App:    "ydsz-trace-logc",
		Time:   time.Now().Format("2006-01-02 15:04:05"),
	})
}
