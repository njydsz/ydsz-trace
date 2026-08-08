package controllers

import (
	"github.com/astaxie/beego"
)

// MainController 根路径控制器
type MainController struct {
	beego.Controller
}

// Get 根路径返回服务信息
func (c *MainController) Get() {
	c.Ctx.WriteString("Ydsz Trace logs server is running.")
}
