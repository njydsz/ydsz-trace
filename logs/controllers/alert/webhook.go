// Package alert 实现日志告警规则与 webhook 推送：
//   - 周期性评估启用的告警规则（调用 logc /file/search 统计命中行数）
//   - 命中数达到阈值时，按 channel 字段选择对应 Notifier 投递，并记录一次事件
//
// 设计要点：
//   - 不同 rule 的评估间隔相互独立（各自保存 interval_sec）
//   - 同一 rule 在 interval 窗口内最多触发一次，避免告警风暴
//   - 调用方通过公开函数 FireRule(ctx, rule) 触发单次评估，便于 cron 与测试接口复用
package alert

import (
	"context"
	"log"
	"time"

	models "ydsz-trace/logs/models"
)

// dispatchResult 记录一次投递的结果，供事件持久化与单测断言使用。
type dispatchResult struct {
	StatusCode int
	Body       string
	Err        error
}

// FireWebhook 向 rule.WebhookUrl 投递告警 payload，返回投递结果。
//
// 已废弃直接调用，保留作为向后兼容包装：内部委托给 WebhookNotifier。
// 新代码请使用 ResolveNotifier(rule.Channel).Send(...)。
func FireWebhook(ctx context.Context, rule models.AlertRule, matchCount int, sampleLines []string) dispatchResult {
	return (&WebhookNotifier{}).Send(ctx, rule, matchCount, sampleLines)
}

// Suppress 检查 rule 是否在 interval_sec 冷却期内（距上次触发 < interval）。
// 返回 true 表示应当抑制（避免在去重窗口内再次触发）。
func Suppress(rule models.AlertRule, intervalSec int) bool {
	if rule.LastFired == "" || intervalSec <= 0 {
		return false
	}
	last, err := time.ParseInLocation("2006-01-02 15:04:05", rule.LastFired, time.Local)
	if err != nil {
		log.Printf("[alert] parse last_fired 失败: %v", err)
		return false
	}
	return time.Since(last) < time.Duration(intervalSec)*time.Second
}
