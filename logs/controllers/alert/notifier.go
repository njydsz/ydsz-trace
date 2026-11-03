// Package alert 通知渠道抽象：Notifier 接口与五种实现。
//
// 设计目标：
//   - 统一投递入口：Scanner 只依赖 Notifier 接口，按 channel 字段路由到具体实现
//   - 渠道可插拔：新增渠道只需实现 Notifier 并在 registry 中注册
//   - 失败可观测：所有实现返回统一的 dispatchResult，供事件持久化
//
// 支持渠道：
//   - webhook  : 通用 HTTP POST JSON（原有能力迁移至此）
//   - wecom    : 企业微信机器人（text 消息）
//   - dingtalk : 钉钉机器人（text 消息）
//   - feishu   : 飞书机器人（text 消息）
//   - email    : SMTP 邮件（简化实现，使用 net/smtp 纯文本）
package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	models "ydsz-trace/logs/models"
)

// 通知渠道常量（与 t_alert_rule.channel 字段值对应）。
const (
	ChannelWebhook = "webhook"
	ChannelWeCom   = "wecom"
	ChannelDingTalk = "dingtalk"
	ChannelFeishu  = "feishu"
	ChannelEmail   = "email"
)

// Notifier 告警通知渠道统一接口。
//
// Send 将告警内容投递到目标渠道，返回投递结果（状态码 + 响应体 + 错误）。
// 实现必须保证幂等性由调用方（去重窗口 + 聚合器）控制，自身不维护冷却状态。
type Notifier interface {
	// Channel 返回该实现对应的渠道标识（用于 registry 注册与日志）。
	Channel() string
	// Send 投递告警。
	Send(ctx context.Context, rule models.AlertRule, matchCount int, sampleLines []string) dispatchResult
}

// httpClient 暴露一个可覆盖的 http client，便于测试注入。
// 同包内共享，webhook.go / notifier.go 均复用此变量。
var httpClient = &http.Client{Timeout: 15 * time.Second}

// ============ 通用 payload ============

// alertPayload 通用告警消息结构，各渠道按需转换。
type alertPayload struct {
	RuleID      int64    `json:"rule_id"`
	RuleName    string   `json:"rule_name"`
	ItemID      int64    `json:"item_id"`
	ItemName    string   `json:"item_name"`
	MatchCount  int      `json:"match_count"`
	Threshold   int      `json:"threshold"`
	FireTime    string   `json:"fire_time"`
	SampleLines []string `json:"sample_lines,omitempty"`
}

// buildAlertPayload 从规则与命中信息构造统一 payload。
func buildAlertPayload(rule models.AlertRule, matchCount int, sampleLines []string) alertPayload {
	return alertPayload{
		RuleID:      rule.ID,
		RuleName:    rule.Name,
		ItemID:      rule.ItemID,
		ItemName:    rule.ItemName,
		MatchCount:  matchCount,
		Threshold:   rule.Threshold,
		FireTime:    time.Now().Format("2006-01-02 15:04:05"),
		SampleLines: sampleLines,
	}
}

// textSummary 生成纯文本告警摘要（供 IM / 邮件渠道复用）。
func textSummary(p alertPayload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "【告警】%s\n", p.RuleName)
	fmt.Fprintf(&b, "命中数: %d (阈值: %d)\n", p.MatchCount, p.Threshold)
	fmt.Fprintf(&b, "日志项: %s (id=%d)\n", p.ItemName, p.ItemID)
	fmt.Fprintf(&b, "触发时间: %s\n", p.FireTime)
	if len(p.SampleLines) > 0 {
		b.WriteString("样本:\n")
		for i, s := range p.SampleLines {
			if i >= 3 {
				break
			}
			if len(s) > 200 {
				s = s[:200]
			}
			fmt.Fprintf(&b, "  %d. %s\n", i+1, s)
		}
	}
	return b.String()
}

// doPostJSON 通用 HTTP POST JSON 投递，返回 dispatchResult。
func doPostJSON(ctx context.Context, url string, body interface{}, headers map[string]string) dispatchResult {
	payload, err := json.Marshal(body)
	if err != nil {
		return dispatchResult{Err: fmt.Errorf("序列化 payload 失败: %w", err)}
	}
	if url == "" {
		return dispatchResult{Err: fmt.Errorf("url 为空")}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return dispatchResult{Err: fmt.Errorf("构造请求失败: %w", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return dispatchResult{Err: fmt.Errorf("请求失败: %w", err)}
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return dispatchResult{
		StatusCode: resp.StatusCode,
		Body:       string(respBody),
		Err:        nil,
	}
}

// ============ WebhookNotifier ============

// WebhookNotifier 通用 Webhook 渠道（原 webhook.go 能力迁移）。
type WebhookNotifier struct{}

// Channel 实现 Notifier。
func (w *WebhookNotifier) Channel() string { return ChannelWebhook }

// Send 实现 Notifier：POST JSON payload 到 rule.WebhookURL。
func (w *WebhookNotifier) Send(ctx context.Context, rule models.AlertRule, matchCount int, sampleLines []string) dispatchResult {
	p := buildAlertPayload(rule, matchCount, sampleLines)
	return doPostJSON(ctx, rule.WebhookURL, p, nil)
}

// ============ WeComNotifier（企业微信）============

// WeComNotifier 企业微信群机器人。
// 文档: https://developer.work.weixin.qq.com/document/path/91770
type WeComNotifier struct{}

// Channel 实现 Notifier。
func (w *WeComNotifier) Channel() string { return ChannelWeCom }

// wecomPayload 企业微信 text 消息体。
type wecomPayload struct {
	MsgType string `json:"msgtype"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

// Send 实现 Notifier。
//
// 自动识别 URL 中的 secret 参数并附加加签（V2.2-05）：
// 企微机器人开启「加签」安全设置后必须携带 timestamp + sign。
func (w *WeComNotifier) Send(ctx context.Context, rule models.AlertRule, matchCount int, sampleLines []string) dispatchResult {
	p := buildAlertPayload(rule, matchCount, sampleLines)
	var body wecomPayload
	body.MsgType = "text"
	body.Text.Content = textSummary(p)

	target, err := SignWebhookURL(rule.WebhookURL)
	if err != nil {
		return dispatchResult{Err: fmt.Errorf("构造加签 URL 失败: %w", err)}
	}
	return doPostJSON(ctx, target, body, nil)
}

// ============ DingTalkNotifier（钉钉）============

// DingTalkNotifier 钉钉群机器人。
// 文档: https://open.dingtalk.com/document/robots/custom-robot-access
type DingTalkNotifier struct{}

// Channel 实现 Notifier。
func (d *DingTalkNotifier) Channel() string { return ChannelDingTalk }

// dingTalkPayload 钉钉 text 消息体。
type dingTalkPayload struct {
	MsgType string `json:"msgtype"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

// Send 实现 Notifier。
//
// 自动识别 URL 中的 secret 参数并附加加签（V2.2-05）：
// 钉钉机器人开启「加签」安全设置后必须携带 timestamp + sign。
func (d *DingTalkNotifier) Send(ctx context.Context, rule models.AlertRule, matchCount int, sampleLines []string) dispatchResult {
	p := buildAlertPayload(rule, matchCount, sampleLines)
	var body dingTalkPayload
	body.MsgType = "text"
	body.Text.Content = textSummary(p)

	target, err := SignWebhookURL(rule.WebhookURL)
	if err != nil {
		return dispatchResult{Err: fmt.Errorf("构造加签 URL 失败: %w", err)}
	}
	return doPostJSON(ctx, target, body, nil)
}

// ============ FeishuNotifier（飞书）============

// FeishuNotifier 飞书群机器人（自定义机器人）。
// 文档: https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot
type FeishuNotifier struct{}

// Channel 实现 Notifier。
func (f *FeishuNotifier) Channel() string { return ChannelFeishu }

// feishuPayload 飞书 text 消息体。
type feishuPayload struct {
	MsgType string `json:"msg_type"`
	Content struct {
		Text string `json:"text"`
	} `json:"content"`
}

// Send 实现 Notifier。
func (f *FeishuNotifier) Send(ctx context.Context, rule models.AlertRule, matchCount int, sampleLines []string) dispatchResult {
	p := buildAlertPayload(rule, matchCount, sampleLines)
	var body feishuPayload
	body.MsgType = "text"
	body.Content.Text = textSummary(p)
	return doPostJSON(ctx, rule.WebhookURL, body, nil)
}

// ============ EmailNotifier（SMTP，简化实现）============

// EmailNotifier 邮件渠道（简化为 SMTP 纯文本发送）。
//
// 复用 t_alert_rule.webhook_url 字段作为 SMTP 目标地址，格式约定：
//
//	smtp://user:pass@smtp.example.com:587/?to=alert@example.com
//
// 该设计避免为邮件单独引入配置表；channel=email 时由 webhook_url 承载完整 SMTP 连接串。
type EmailNotifier struct{}

// Channel 实现 Notifier。
func (e *EmailNotifier) Channel() string { return ChannelEmail }

// smtpConfig 从 smtp:// URL 解析出的连接参数。
type smtpConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	To       []string
}

// parseSMTPConfig 解析 smtp://user:pass@host:port/?to=a@x.com&to=b@y.com。
// 解析失败返回 error；调用方据此记录 fail 事件。
func parseSMTPConfig(rawURL string) (*smtpConfig, error) {
	const prefix = "smtp://"
	if !strings.HasPrefix(rawURL, prefix) {
		return nil, fmt.Errorf("email 渠道要求 webhook_url 以 smtp:// 开头")
	}
	rest := rawURL[len(prefix):]
	// 分离 query
	var queryPart string
	if idx := strings.Index(rest, "?"); idx >= 0 {
		queryPart = rest[idx+1:]
		rest = rest[:idx]
	}
	// 分离 userinfo
	var userInfo string
	if idx := strings.LastIndex(rest, "@"); idx >= 0 {
		userInfo = rest[:idx]
		rest = rest[idx+1:]
	}
	cfg := &smtpConfig{}
	if userInfo != "" {
		parts := strings.SplitN(userInfo, ":", 2)
		cfg.Username = parts[0]
		if len(parts) == 2 {
			cfg.Password = parts[1]
		}
	}
	// host:port
	host := rest
	port := "25"
	if idx := strings.LastIndex(rest, ":"); idx >= 0 {
		host = rest[:idx]
		port = rest[idx+1:]
	}
	cfg.Host = host
	cfg.Port = port
	// to 列表
	for _, kv := range strings.Split(queryPart, "&") {
		if strings.HasPrefix(kv, "to=") {
			cfg.To = append(cfg.To, strings.TrimPrefix(kv, "to="))
		}
	}
	if cfg.Host == "" {
		return nil, fmt.Errorf("smtp host 为空")
	}
	if len(cfg.To) == 0 {
		return nil, fmt.Errorf("缺少 to 收件人")
	}
	return cfg, nil
}

// Send 实现 Notifier：通过 SMTP 发送纯文本告警邮件。
func (e *EmailNotifier) Send(ctx context.Context, rule models.AlertRule, matchCount int, sampleLines []string) dispatchResult {
	cfg, err := parseSMTPConfig(rule.WebhookURL)
	if err != nil {
		return dispatchResult{Err: err}
	}
	p := buildAlertPayload(rule, matchCount, sampleLines)

	subject := fmt.Sprintf("[ydsz-alert] %s 命中 %d 次", rule.Name, matchCount)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		cfg.Username, strings.Join(cfg.To, ","), subject, textSummary(p))

	addr := cfg.Host + ":" + cfg.Port
	var auth smtp.Auth
	if cfg.Username != "" && cfg.Password != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}

	// 在独立 goroutine 中执行 SMTP 调用，以支持 ctx 取消。
	done := make(chan dispatchResult, 1)
	go func() {
		sendErr := smtp.SendMail(addr, auth, cfg.Username, cfg.To, []byte(msg))
		if sendErr != nil {
			done <- dispatchResult{Err: fmt.Errorf("smtp 发送失败: %w", sendErr)}
			return
		}
		done <- dispatchResult{StatusCode: http.StatusOK, Body: "sent"}
	}()

	select {
	case <-ctx.Done():
		return dispatchResult{Err: fmt.Errorf("smtp 发送取消: %w", ctx.Err())}
	case res := <-done:
		return res
	}
}

// ============ Registry（渠道路由）============

// notifierRegistry channel -> Notifier 单例映射。
// 所有实现均为无状态结构体，可安全复用。
var notifierRegistry = map[string]Notifier{
	ChannelWebhook:  &WebhookNotifier{},
	ChannelWeCom:    &WeComNotifier{},
	ChannelDingTalk: &DingTalkNotifier{},
	ChannelFeishu:   &FeishuNotifier{},
	ChannelEmail:    &EmailNotifier{},
}

// ResolveNotifier 根据 channel 字符串返回对应 Notifier。
// 未识别的 channel 回退到 Webhook（保持向后兼容：老数据 channel='' 或 'webhook'）。
func ResolveNotifier(channel string) Notifier {
	if n, ok := notifierRegistry[channel]; ok {
		return n
	}
	return notifierRegistry[ChannelWebhook]
}

// logNotifierWarning 在渠道未识别时打印告警日志（便于运维排查配置错误）。
func logNotifierWarning(channel string) {
	log.Printf("[alert] 未知通知渠道 %q，已回退到 webhook", channel)
}
