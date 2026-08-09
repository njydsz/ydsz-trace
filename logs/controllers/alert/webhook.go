// Package alert 实现日志告警规则与 webhook 推送：
//   - 周期性评估启用的告警规则（调用 logc /file/search 统计命中行数）
//   - 命中数达到阈值时，POST JSON payload 到 webhook_url，并记录一次事件
//
// 设计要点：
//   - 不同 rule 的评估间隔相互独立（各自保存 interval_sec）
//   - 同一 rule 在 interval 窗口内最多触发一次，避免告警风暴
//   - 调用方通过公开函数 FireRule(ctx, rule) 触发单次评估，便于 cron 与测试接口复用
package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	models "ydsz-trace/logs/models"
)

// dispatchResult 记录一次 webhook 投递的结果，供事件持久化与单测断言使用。
type dispatchResult struct {
	StatusCode int
	Body       string
	Err        error
}

// webhookPayload 是发给 webhook_url 的 JSON payload 结构。
type webhookPayload struct {
	RuleID      int64    `json:"rule_id"`
	RuleName    string   `json:"rule_name"`
	ItemID      int64    `json:"item_id"`
	ItemName    string   `json:"item_name"`
	MatchCount  int      `json:"match_count"`
	Threshold   int      `json:"threshold"`
	FireTime    string   `json:"fire_time"`
	SampleLines []string `json:"sample_lines,omitempty"`
}

// httpClient 暴露一个可覆盖的 http client，便于测试注入。
var httpClient = &http.Client{Timeout: 15 * time.Second}

// FireWebhook 向 rule.WebhookUrl 投递告警 payload，返回投递结果。
func FireWebhook(ctx context.Context, rule models.AlertRule, matchCount int, sampleLines []string) dispatchResult {
	payload := webhookPayload{
		RuleID:      rule.ID,
		RuleName:    rule.Name,
		ItemID:      rule.ItemID,
		ItemName:    rule.ItemName,
		MatchCount:  matchCount,
		Threshold:   rule.Threshold,
		FireTime:    time.Now().Format("2006-01-02 15:04:05"),
		SampleLines: sampleLines,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return dispatchResult{Err: fmt.Errorf("序列化 payload 失败: %w", err)}
	}

	url := rule.WebhookURL
	if url == "" {
		return dispatchResult{Err: fmt.Errorf("rule %d webhook_url 为空", rule.ID)}
	}

	var req *http.Request
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return dispatchResult{Err: fmt.Errorf("构造 webhook 请求失败: %w", err)}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return dispatchResult{Err: fmt.Errorf("webhook 请求失败: %w", err)}
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return dispatchResult{
		StatusCode: resp.StatusCode,
		Body:       string(respBody),
		Err:        nil,
	}
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
