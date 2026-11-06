// Package routers 子文件：客户端长连接模式（灰度）管理接口（P1-5）。
//
// 提供端点：
//   - POST /agent/client/:id/mode  设置单个客户端的 agent_mode（push/pull）
//   - GET  /agent/client/:id/mode  查询单个客户端的 agent_mode
//
// 设置后：
//   - 若客户端在线，立即经长连接下发模式切换指令；
//   - 若客户端离线，模式持久化在 t_client.agent_mode，下次注册时下发。
package routers

import (
	"strconv"

	"ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// SetClientModeHandler 设置客户端长连接模式。
//
// 请求体：{"mode": "push" | "pull"}
func SetClientModeHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		api.Fail(c, api.CodeBadRequest, "非法客户端 ID")
		return
	}

	var req struct {
		Mode string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if req.Mode != "push" && req.Mode != "pull" {
		api.Fail(c, api.CodeBadRequest, "mode 必须为 push 或 pull")
		return
	}

	// 持久化到 t_client.agent_mode。
	if err := models.UpdateClientAgentMode(id, req.Mode); err != nil {
		api.Fail(c, api.CodeServerError, "更新失败: "+err.Error())
		return
	}

	// 在线则立即下发模式切换指令。
	pushed := false
	if srv := GetAgentLinkServer(); srv != nil {
		if err := srv.PushMode(id, req.Mode); err == nil {
			pushed = true
		}
	}

	api.Success(c, "模式已更新", gin.H{
		"clientId": id,
		"mode":     req.Mode,
		"pushed":   pushed,
	})
}

// GetClientModeHandler 查询客户端长连接模式。
func GetClientModeHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		api.Fail(c, api.CodeBadRequest, "非法客户端 ID")
		return
	}

	client := models.ReadClient(id)
	if client.Id == 0 {
		api.Fail(c, api.CodeNotFound, "客户端不存在")
		return
	}

	mode := client.AgentMode
	if mode == "" {
		mode = "default"
	}

	api.Success(c, "查询成功", gin.H{
		"clientId": id,
		"mode":     mode,
	})
}
