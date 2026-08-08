package controllers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// HealthResp 健康检查响应
type HealthResp struct {
	Status string `json:"status"`
	App    string `json:"app"`
	Time   string `json:"time"`
}

// Main 根路径返回服务信息
func Main(c *gin.Context) {
	c.String(http.StatusOK, "Ydsz Trace logc agent is running.")
}

// Health 健康检查端点（K8s liveness probe）
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResp{
		Status: "ok",
		App:    "ydsz-trace-logc",
		Time:   time.Now().Format("2006-01-02 15:04:05"),
	})
}

// Ready 就绪检查端点（K8s readiness probe）
func Ready(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResp{
		Status: "ready",
		App:    "ydsz-trace-logc",
		Time:   time.Now().Format("2006-01-02 15:04:05"),
	})
}
