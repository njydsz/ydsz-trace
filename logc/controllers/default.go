package controllers

import (
	"github.com/astaxie/beego"
)

// MainController 根路径控制器
type MainController struct {
	beego.Controller
}

// HealthResp 健康检查响应
type HealthResp struct {
	Status string `json:"status"`
	App    string `json:"app"`
	Time   string `json:"time"`
}

// Get 根路径返回服务信息
func (c *MainController) Get() {
	c.Ctx.WriteString("Ydsz Trace logc agent is running.")
}

// Health 健康检查端点（K8s liveness probe）
func (c *MainController) Health() {
	data := HealthResp{
		Status: "ok",
		App:    "ydsz-trace-logc",
		Time:   beego.Date(0, "2006-01-02 15:04:05"),
	}
	c.Data["json"] = &data
	c.ServeJSON()
}

// Ready 就绪检查端点（K8s readiness probe）
func (c *MainController) Ready() {
	data := HealthResp{
		Status: "ready",
		App:    "ydsz-trace-logc",
		Time:   beego.Date(0, "2006-01-02 15:04:05"),
	}
	c.Data["json"] = &data
	c.ServeJSON()
}
