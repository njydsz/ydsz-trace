// Package routers 子文件：配置热下发 HTTP 接口（P1-4）。
//
// 提供两个端点：
//   - POST /agent/config/push       向单个在线客户端下发配置
//   - POST /agent/config/broadcast  向所有在线客户端广播配置
//
// 配置内容由前端/调用方在请求体中给出，经长连接通道实时推送到 agent，
// agent 侧通过 SetConfigHandler 回调即时生效（如采样率热更新）。
package routers

import (
	"time"

	"ydsz-trace/pkg/agentlink"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// ConfigPushReq 配置下发请求体。
type ConfigPushReq struct {
	// ClientID 目标客户端 ID（Push 时必填；Broadcast 时忽略）。
	ClientID int64 `json:"clientId"`
	// SamplingRate 采样率（0.0~1.0，0 表示不下发该字段）。
	SamplingRate float64 `json:"samplingRate"`
	// MaskTemplates 脱敏模板 ID 列表。
	MaskTemplates []string `json:"maskTemplates"`
	// MultilinePatterns 多行合并规则。
	MultilinePatterns []string `json:"multilinePatterns"`
}

// validate 校验请求体。
func (r *ConfigPushReq) validate() (bool, string) {
	if r.SamplingRate < 0 || r.SamplingRate > 1 {
		return false, "samplingRate 必须在 0.0~1.0 之间"
	}
	if len(r.MaskTemplates) > 1000 {
		return false, "maskTemplates 数量不能超过 1000"
	}
	if len(r.MultilinePatterns) > 100 {
		return false, "multilinePatterns 数量不能超过 100"
	}
	return true, ""
}

// toConfigPush 转换为协议载荷。
func (r *ConfigPushReq) toConfigPush() *agentlink.ConfigPush {
	return &agentlink.ConfigPush{
		SamplingRate:      r.SamplingRate,
		MaskTemplates:     r.MaskTemplates,
		MultilinePatterns: r.MultilinePatterns,
		EffectiveAt:       time.Now().Unix(),
	}
}

// PushConfigHandler 向单个客户端下发配置。
func PushConfigHandler(c *gin.Context) {
	srv := GetAgentLinkServer()
	if srv == nil {
		api.Fail(c, api.CodeServerError, "长连接通道未启用")
		return
	}

	var req ConfigPushReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if ok, msg := req.validate(); !ok {
		api.Fail(c, api.CodeBadRequest, msg)
		return
	}
	if req.ClientID <= 0 {
		api.Fail(c, api.CodeBadRequest, "clientId 必须为正整数")
		return
	}

	cfg := req.toConfigPush()
	if err := srv.PushConfig(req.ClientID, cfg); err != nil {
		api.Fail(c, api.CodeBadRequest, "下发失败: "+err.Error())
		return
	}

	api.Success(c, "配置下发成功", gin.H{
		"clientId": req.ClientID,
		"version":  cfg.Version,
	})
}

// BroadcastConfigHandler 向所有在线客户端广播配置。
func BroadcastConfigHandler(c *gin.Context) {
	srv := GetAgentLinkServer()
	if srv == nil {
		api.Fail(c, api.CodeServerError, "长连接通道未启用")
		return
	}

	var req ConfigPushReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if ok, msg := req.validate(); !ok {
		api.Fail(c, api.CodeBadRequest, msg)
		return
	}

	cfg := req.toConfigPush()
	before := srv.OnlineCount()
	srv.PushConfigAll(cfg)
	after := srv.OnlineCount()

	api.Success(c, "配置广播完成", gin.H{
		"onlineBefore": before,
		"onlineAfter":  after,
		"version":      cfg.Version,
	})
}
