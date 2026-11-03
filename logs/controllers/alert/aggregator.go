// Package alert 告警聚合降噪：5 分钟窗口内同模板多次触发合并为一条告警。
//
// 设计目标：
//   - 同模板（templateHash）在聚合窗口内多次触发时，只投递一次，后续触发累加计数
//   - 窗口到期后自动"flush"：投递一条包含 firstTime / lastTime / count 的合并告警
//   - 投递由 aggregator 自身触发，Scanner 仅调用 Aggregate 记录命中
//
// 关键结构：
//   - aggregateEntry：单模板在窗口内的合并状态
//   - Aggregator：全局聚合器，按 templateHash 维护 entry 映射
//
// 使用方式：
//
//	agg := NewAggregator(5 * time.Minute)
//	if agg.Aggregate(rule, templateHash, matchCount, samples) {
//	    // 达到首次触发条件，立即投递
//	}
//	// 后台定期调用 FlushExpired(now) 投递到期合并告警
package alert

import (
	"context"
	"sync"
	"time"

	models "ydsz-trace/logs/models"
)

// DefaultAggregateWindow 默认聚合窗口时长（5 分钟）。
const DefaultAggregateWindow = 5 * time.Minute

// aggregateEntry 单个模板在聚合窗口内的合并状态。
type aggregateEntry struct {
	TemplateHash string
	RuleID       int64
	RuleName     string
	ItemID       int64
	ItemName     string
	Channel      string
	WebhookURL   string
	Threshold    int
	FirstTime    time.Time
	LastTime     time.Time
	Count        int    // 累计命中数（所有触发的 matchCount 之和）
	TriggerCount int    // 触发次数（窗口内被 Aggregate 调用的次数）
	Samples      []string // 最近一次样本（最多 3 条）
}

// Aggregator 告警聚合器：维护 5 分钟窗口，同模板多次触发合并为一条。
//
// 线程安全：所有导出方法均加锁，可在 Scanner 的并发 goroutine 中安全调用。
type Aggregator struct {
	mu      sync.Mutex
	window  time.Duration
	entries map[string]*aggregateEntry // key = templateHash

	// sendFunc 投递函数，便于单测替换；默认使用 ResolveNotifier。
	sendFunc func(ctx context.Context, rule models.AlertRule, matchCount int, samples []string) dispatchResult
}

// NewAggregator 创建聚合器；window <= 0 时使用 DefaultAggregateWindow。
func NewAggregator(window time.Duration) *Aggregator {
	if window <= 0 {
		window = DefaultAggregateWindow
	}
	return &Aggregator{
		window:   window,
		entries:  make(map[string]*aggregateEntry),
		sendFunc: defaultSend,
	}
}

// defaultSend 默认投递实现：根据 rule.Channel 路由到对应 Notifier。
func defaultSend(ctx context.Context, rule models.AlertRule, matchCount int, samples []string) dispatchResult {
	if n, ok := notifierRegistry[rule.Channel]; ok {
		return n.Send(ctx, rule, matchCount, samples)
	}
	logNotifierWarning(rule.Channel)
	return notifierRegistry[ChannelWebhook].Send(ctx, rule, matchCount, samples)
}

// Aggregate 记录一次命中，并判断是否需要立即投递。
//
// 返回 true 表示这是该模板在窗口内的首次命中，调用方应立即投递（并写 alert_event）。
// 返回 false 表示窗口内已有同模板告警，仅累加计数；到期后由 FlushExpired 统一投递。
//
// 注意：本方法不直接投递，只更新聚合状态；投递动作由 Scanner 协调完成，
// 便于统一写 alert_event / 更新 last_fired。
func (a *Aggregator) Aggregate(rule models.AlertRule, templateHash string, matchCount int, samples []string) (shouldFireNow bool, entry *aggregateEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	e, ok := a.entries[templateHash]
	if !ok || now.Sub(e.FirstTime) >= a.window {
		// 首次命中或窗口已过期：创建新 entry，标记立即投递
		e = &aggregateEntry{
			TemplateHash: templateHash,
			RuleID:       rule.ID,
			RuleName:     rule.Name,
			ItemID:       rule.ItemID,
			ItemName:     rule.ItemName,
			Channel:      rule.Channel,
			WebhookURL:   rule.WebhookURL,
			Threshold:    rule.Threshold,
			FirstTime:    now,
			LastTime:     now,
			Count:        matchCount,
			TriggerCount: 1,
			Samples:      truncateSamples(samples, 3),
		}
		a.entries[templateHash] = e
		return true, e
	}

	// 窗口内已存在：累加计数，更新最近触发时间与样本
	e.LastTime = now
	e.Count += matchCount
	e.TriggerCount++
	if len(samples) > 0 {
		e.Samples = truncateSamples(samples, 3)
	}
	return false, e
}

// FlushExpired 投递所有已到期（窗口已过）的聚合 entry，并从 map 中移除。
// 返回本次 flush 的 entry 列表（按 FirstTime 升序），供 Scanner 写 alert_event。
//
// 通常由 Scanner 的定时任务周期性调用；单测中可直接调用以验证聚合行为。
func (a *Aggregator) FlushExpired(now time.Time) []*aggregateEntry {
	a.mu.Lock()
	defer a.mu.Unlock()

	var expired []*aggregateEntry
	for k, e := range a.entries {
		if now.Sub(e.FirstTime) >= a.window {
			expired = append(expired, e)
			delete(a.entries, k)
		}
	}
	return expired
}

// FlushEntry 投递指定 entry（从 FlushExpired 返回的列表中取出），写 alert_event 并返回结果。
//
// 注意：本方法假定 entry 已从 Aggregator.entries 中移除（由 FlushExpired 完成）。
// 若 entry 仍在 map 中，调用方应自行决定是否再次调用 FlushExpired。
func (a *Aggregator) FlushEntry(ctx context.Context, e *aggregateEntry) dispatchResult {
	rule := models.AlertRule{
		ID:         e.RuleID,
		Name:       e.RuleName,
		ItemID:     e.ItemID,
		ItemName:   e.ItemName,
		Channel:    e.Channel,
		WebhookURL: e.WebhookURL,
		Threshold:  e.Threshold,
	}
	// 合并告警：matchCount 使用累计命中数，样本使用最近一次。
	return a.sendFunc(ctx, rule, e.Count, e.Samples)
}

// EntriesCount 返回当前聚合窗口内的 entry 数量（供单测/运维观察）。
func (a *Aggregator) EntriesCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.entries)
}

// truncateSamples 截断样本数量到 max，并限制单条长度到 200。
func truncateSamples(samples []string, max int) []string {
	if len(samples) > max {
		samples = samples[:max]
	}
	out := make([]string, 0, len(samples))
	for _, s := range samples {
		if len(s) > 200 {
			s = s[:200]
		}
		out = append(out, s)
	}
	return out
}
