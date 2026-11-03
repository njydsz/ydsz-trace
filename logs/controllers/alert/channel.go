// Package alert 子文件：告警渠道连通性测试（V2.2-05）。
//
// 用途：
//   用户配置完钉钉/企微/Webhook 后，往往无法确认是否填对。
//   本接口发送一条真实测试消息并同步返回投递结果（状态码 + 响应体 + 错误），
//   让配置问题在保存时即可发现，而非等到真实告警才发现。
//
// 路由：POST /logs/alerts/channels/test
//
// 请求体：
//
//	{ "channel": "dingtalk", "webhookUrl": "https://..." }
//
// 或按规则测试：
//
//	{ "ruleId": 123 }（读取该规则的 channel + webhookUrl）
package alert

import (
	"context"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// channelTestReq 渠道测试请求体。
type channelTestReq struct {
	RuleID     int64  `json:"ruleId"`
	Channel    string `json:"channel"`
	WebhookURL string `json:"webhookUrl"`
}

// ChannelTestResult 渠道测试结果。
type ChannelTestResult struct {
	Channel    string `json:"channel"`
	WebhookURL string `json:"webhookUrl"`
	Success    bool   `json:"success"`
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body,omitempty"`
	Error      string `json:"error,omitempty"`
	CostMS     int64  `json:"costMs"`
}

// TestChannel POST /logs/alerts/channels/test
//
// 同步发送一条测试告警并返回真实投递结果。
func TestChannel(c *gin.Context) {
	var req channelTestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "参数非法")
		return
	}

	// 按规则测试：读取规则的渠道与地址
	if req.RuleID > 0 {
		rule, err := models.GetAlertRule(req.RuleID)
		if err != nil || rule.ID == 0 {
			api.Fail(c, api.CodeNotFound, "规则不存在")
			return
		}
		req.Channel = rule.Channel
		req.WebhookURL = rule.WebhookURL
	}

	if req.Channel == "" {
		req.Channel = ChannelWebhook
	}
	if req.WebhookURL == "" {
		api.Fail(c, api.CodeBadRequest, "webhookUrl 不能为空")
		return
	}

	notifier, ok := notifierRegistry[req.Channel]
	if !ok {
		api.Fail(c, api.CodeBadRequest, "未知渠道")
		return
	}

	// 构造一条模拟规则用于测试消息
	rule := models.AlertRule{
		ID:          0,
		Name:        "渠道连通性测试",
		ItemID:      0,
		ItemName:    "测试",
		Channel:     req.Channel,
		WebhookURL:  req.WebhookURL,
		Threshold:   1,
		IntervalSec: 300,
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	result := notifier.Send(ctx, rule, 1, []string{"这是一条来自云顶追踪的渠道测试消息，可忽略。"})
	cost := time.Since(start).Milliseconds()

	out := ChannelTestResult{
		Channel:    req.Channel,
		WebhookURL: req.WebhookURL,
		Success:    result.Err == nil,
		StatusCode: result.StatusCode,
		Body:       result.Body,
		CostMS:     cost,
	}
	if result.Err != nil {
		out.Error = result.Err.Error()
	}

	if result.Err != nil {
		api.Success(c, "投递失败", out)
		return
	}
	api.Success(c, "投递成功", out)
}

// ListChannels GET /logs/alerts/channels
//
// 返回系统支持的全部渠道清单（供前端下拉选择）。
func ListChannels(c *gin.Context) {
	channels := []gin.H{
		{"id": ChannelWebhook, "name": "通用 Webhook", "desc": "POST JSON 到任意 HTTP 地址"},
		{"id": ChannelDingTalk, "name": "钉钉群机器人", "desc": "支持加签（URL 附带 secret 参数）"},
		{"id": ChannelWeCom, "name": "企业微信机器人", "desc": "支持加签（URL 附带 secret 参数）"},
		{"id": ChannelFeishu, "name": "飞书群机器人", "desc": "自定义机器人"},
		{"id": ChannelEmail, "name": "邮件（SMTP）", "desc": "webhookUrl 格式：smtp://user:pass@host:port/?to=xxx"},
	}
	api.Success(c, "查询成功", gin.H{"channels": channels})
}
