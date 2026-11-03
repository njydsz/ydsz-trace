// Package alert 实现日志告警规则与多渠道路由：
//   - 周期性评估启用的告警规则（调用 logc /file/search 统计命中行数）
//   - 命中数达到阈值时，按 channel 字段选择对应 Notifier 投递，并记录一次事件
//   - 5 分钟聚合窗口内同模板多次触发合并为一条告警（Aggregator）
//   - 基于模式时序计数的异常检测（突变/骤降）
//   - 同源异常去重保护（按 ruleID + interval 窗口去重）
//
// 设计要点：
//   - 不同 rule 的评估间隔相互独立（各自保存 interval_sec）
//   - 同一 rule 在 interval 窗口内最多触发一次，避免告警风暴
//   - 调用方通过公开函数 FireRule(ctx, rule) 触发单次评估，便于 cron 与测试接口复用
//   - 评估命中后写 t_pattern_stat 供基线学习使用，再做异常检测
package alert

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/stats"

	"github.com/robfig/cron/v3"
)

// dedupeWindow 同规则在 N 秒内不重复投递的内存去重表（FR-806）。
// key = ruleID，value = 上次投递时间。
type dedupeWindow struct {
	mu      sync.Mutex
	windows map[int64]time.Time // ruleID -> last fire time
}

// ShouldFire 判断规则是否应投递（已过冷却期）。
func (d *dedupeWindow) ShouldFire(ruleID int64, cooldownSec int) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	last, ok := d.windows[ruleID]
	if !ok || time.Since(last) > time.Duration(cooldownSec)*time.Second {
		d.windows[ruleID] = time.Now()
		return true
	}
	return false
}

// Scheduler 周期性评估启用的告警规则；命中阈值且不在冷却期时投递告警。
type Scheduler struct {
	cron   *cron.Cron
	mu     sync.Mutex
	sem    chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
	dedupe *dedupeWindow

	anomalyEngine *stats.AnomalyEngine
	aggregator    *Aggregator

	// suppressor 维护期静默窗口（V2.2-02），nil 时视为无静默配置
	suppressor *Suppressor
}

// SetSuppressor 注入静默窗口管理器（可选）。
func (s *Scheduler) SetSuppressor(sup *Suppressor) {
	s.suppressor = sup
}

// Suppressor 返回当前静默窗口管理器（未设置时返回 nil）。
func (s *Scheduler) Suppressor() *Suppressor {
	return s.suppressor
}

// probeFunc 实际调 logc 的钩子，便于单测替换。
var probeFunc = func(ctx context.Context, server string, req AlertProbeReq, maxLines int) ([]string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	url := "http://" + server + "/file/search"
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")

	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	var payload struct {
		Lines []string `json:"lines"`
		Count int      `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Lines, nil
}

// NewScheduler 创建调度器；maxConcurrent 控制并行评估规则数。
func NewScheduler(maxConcurrent int) *Scheduler {
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		cron:          cron.New(cron.WithSeconds()),
		sem:           make(chan struct{}, maxConcurrent),
		ctx:           ctx,
		cancel:        cancel,
		dedupe:        &dedupeWindow{windows: make(map[int64]time.Time)},
		anomalyEngine: stats.NewAnomalyEngine(),
		aggregator:    NewAggregator(DefaultAggregateWindow),
	}
}

// Start 开启后台扫描（每分钟开头 + 首次立即执行）。
func (s *Scheduler) Start() {
	if _, err := s.cron.AddFunc("0 * * * * *", s.ScanOnce); err != nil {
		log.Printf("[alert] 注册扫描任务失败: %v", err)
		return
	}
	s.cron.Start()
	go s.ScanOnce()
	// 后台 goroutine：每 30s 检查一次到期聚合 entry 并投递合并告警。
	go s.flushLoop()
}

// flushLoop 周期性 flush 到期聚合 entry。
func (s *Scheduler) flushLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.flushExpired()
		}
	}
}

// flushExpired 投递所有已到期聚合 entry。
func (s *Scheduler) flushExpired() {
	expired := s.aggregator.FlushExpired(time.Now())
	for _, e := range expired {
		result := s.aggregator.FlushEntry(s.ctx, e)
		status := "ok"
		errMsg := ""
		if result.Err != nil {
			status = "fail"
			errMsg = result.Err.Error()
		}
		if _, err := models.InsertAlertEvent(e.RuleID, e.RuleName, e.WebhookURL, status,
			result.StatusCode, e.Count, joinSamples(e.Samples), errMsg); err != nil {
			log.Printf("[alert] 写聚合 event 失败: %v", err)
		}
		if err := models.UpdateAlertRuleFired(e.RuleID); err != nil {
			log.Printf("[alert] 更新 rule last_fired 失败: %v", err)
		}
		log.Printf("[alert] 聚合投递: rule=%s template=%s count=%d triggers=%d",
			e.RuleName, e.TemplateHash, e.Count, e.TriggerCount)
	}
}

// Stop 停止后台扫描并取消所有在途评估 goroutine。
func (s *Scheduler) Stop() {
	if s.cron != nil {
		s.cron.Stop()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

// ScanOnce 单次全量评估。
func (s *Scheduler) ScanOnce() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 维护期静默（V2.2-02）：落在任一启用的静默窗口内时跳过本轮全量扫描
	if s.suppressor != nil {
		if suppressed, name := s.suppressor.IsSuppressed(time.Now()); suppressed {
			log.Printf("[alert] 当前处于维护静默窗口 %q，跳过本轮扫描", name)
			return
		}
	}

	rules, err := models.QueryEnabledAlertRules()
	if err != nil || len(rules) == 0 {
		return
	}

	now := time.Now()
	for i := range rules {
		rule := rules[i]
		models.FillRuleItemName(&rule)
		if Suppress(rule, rule.IntervalSec) {
			continue
		}
		if rule.IntervalSec < 60 {
			rule.IntervalSec = 60
		}
		s.sem <- struct{}{}
		go func(r models.AlertRule, t time.Time) {
			defer func() {
				<-s.sem
				if rec := recover(); rec != nil {
					log.Printf("[alert] 评估规则 %d panic: %v", r.ID, rec)
				}
			}()
			evaluateAndFire(s.ctx, r, t, s.dedupe, s.anomalyEngine, s.aggregator)
		}(rule, now)
	}
}

func evaluateAndFire(ctx context.Context, rule models.AlertRule, now time.Time, dedupe *dedupeWindow, eng *stats.AnomalyEngine, agg *Aggregator) {
	targets, path := resolveAlertTargets(rule.ItemID, rule.ClientID)
	if len(targets) == 0 || path == "" {
		return
	}

	probe := AlertProbeReq{
		Path: path, Key: rule.KeyWord, Regex: rule.Regex, Level: rule.Level,
		MaxLines: 50,
	}

	totalMatches := 0
	var samples []string
	for _, server := range targets {
		lines, err := probeFunc(ctx, server, probe, probe.MaxLines)
		if err != nil {
			log.Printf("[alert] probe %s 失败: %v", server, err)
			continue
		}
		totalMatches += len(lines)
		for _, l := range lines {
			if len(samples) < 3 {
				if len(l) > 200 {
					l = l[:200]
				}
				samples = append(samples, l)
			}
		}
		if totalMatches >= rule.Threshold {
			break
		}
	}

	if totalMatches < rule.Threshold {
		return
	}

	// ── FR-304: 写入模式时序统计 ──
	ts := now.Format("2006-01-02 15:04:05")
	templateHash := hashProbeKey(rule.KeyWord, rule.Regex)
	if sampleForStat := pickSample(samples); sampleForStat != "" {
		if err := models.RecordPatternHit(rule.ItemID, rule.ClientID, templateHash, ts, sampleForStat); err != nil {
			log.Printf("[alert] RecordPatternHit 失败: %v", err)
		}
	}

	// ── FR-802: 异常检测（V2.2-01 多算法编排）──
	// 检测与学习解耦：先跑四类检测器，再统一更新基线。
	bucketIdx := BucketIndexFromTime(now)
	anomalyEvents := DetectOne(eng, templateHash, rule.ItemID, totalMatches, bucketIdx, now)

	// ── V2.2-02 跨算法合并 ──
	// 同一模板同一桶被多个算法命中时合并为一条，避免重复告警与事件表膨胀
	anomalyEvents = mergeStatsEvents(anomalyEvents, bucketIdx)

	// ── V2.2-03 自适应阈值过滤 ──
	// 用户标记过的误报模板会抬高该模板的 score 阈值，此处据此过滤
	anomalyEvents = FilterByAdaptiveThreshold(anomalyEvents)

	for _, ev := range anomalyEvents {
		// 原始 alert_event（兼容）
		jsonDetail, err := json.Marshal(ev)
		if err != nil {
			log.Printf("[alert] 序列化 anomaly event 失败: %v", err)
		} else if _, err := models.InsertAlertEvent(0, "anomaly:"+ev.TemplateHash, "", "anomaly",
			0, ev.CurrentCount, string(jsonDetail), ""); err != nil {
			log.Printf("[alert] 写 anomaly event 失败: %v", err)
		}

		// V2.1 新表：t_anomaly_event
		anomalyType := anomalyTypeOf(ev)
		desc := fmt.Sprintf("模板 %s %s [%s]: 实际=%d, 基线=%.2f, Z=%.2f",
			ev.TemplateHash, ev.Direction, ev.Algorithm, ev.CurrentCount, ev.BaselineMean, ev.ZScore)
		if err := models.RecordAnomalyEvent(
			ev.TemplateHash, rule.ItemID, rule.ClientID, anomalyType,
			ev.ZScore, ev.BaselineMean, float64(ev.CurrentCount), 300, desc,
		); err != nil {
			log.Printf("[alert] RecordAnomalyEvent 失败: %v", err)
		}
	}

	// 学习阶段统一在检测之后进行
	eng.UpdateBaseline(templateHash, rule.ItemID, totalMatches)

	// ── FR-806: 去重检查，避免同源异常重复投递 ──
	if dedupe != nil && !dedupe.ShouldFire(rule.ID, rule.IntervalSec) {
		log.Printf("[alert] rule %d 命中阈值但在冷却期内，跳过投递", rule.ID)
		return
	}

	// ── 聚合降噪：同模板 5 分钟窗口内多次触发合并 ──
	if agg != nil {
		shouldFireNow, _ := agg.Aggregate(rule, templateHash, totalMatches, samples)
		if !shouldFireNow {
			log.Printf("[alert] rule %d template %s 窗口内已存在告警，累加计数跳过本次投递",
				rule.ID, templateHash)
			return
		}
	}

	// ── 渠道路由：根据 rule.Channel 选择 Notifier 投递 ──
	if _, ok := notifierRegistry[rule.Channel]; !ok {
		logNotifierWarning(rule.Channel)
	} else {
		log.Printf("[alert] rule %d 使用渠道 %s 投递", rule.ID, rule.Channel)
	}
	notifier := ResolveNotifier(rule.Channel)
	result := notifier.Send(ctx, rule, totalMatches, samples)

	status := "ok"
	errMsg := ""
	if result.Err != nil {
		status = "fail"
		errMsg = result.Err.Error()
	}
	if _, err := models.InsertAlertEvent(rule.ID, rule.Name, rule.WebhookURL, status,
		result.StatusCode, totalMatches, joinSamples(samples), errMsg); err != nil {
		log.Printf("[alert] 写 event 失败: %v", err)
	}
	if err := models.UpdateAlertRuleFired(rule.ID); err != nil {
		log.Printf("[alert] 更新 rule last_fired 失败: %v", err)
	}
}

// resolveAlertTargets 解析告警目标服务器列表和日志路径。
func resolveAlertTargets(itemID, clientID int64) ([]string, string) {
	item := models.ReadItem(itemID)
	if item.Id == 0 {
		return nil, ""
	}
	date := time.Now().Format("20060102")
	path := item.LogPath + item.LogPrefix + date + item.LogSuffix + ".log"

	var servers []string
	if clientID != 0 {
		cl := models.ReadClient(clientID)
		if cl.Id == 0 {
			return nil, ""
		}
		servers = []string{cl.Ip + ":" + cl.Port}
	} else {
		clients, err := models.QueryAllClient()
		if err != nil {
			return nil, ""
		}
		for _, cl := range clients {
			if cl.Id == 0 {
				continue
			}
			servers = append(servers, cl.Ip+":"+cl.Port)
		}
	}
	return servers, path
}

func joinSamples(samples []string) string {
	if len(samples) == 0 {
		return ""
	}
	out := ""
	for i, s := range samples {
		if i > 0 {
			out += " | "
		}
		out += s
	}
	return out
}

// hashProbeKey 对告警规则的 KeyWord 做归一化后计算 sha1 前 8 字节。
func hashProbeKey(keyword string, isRegex bool) string {
	var normalized string
	if isRegex {
		// 正则：去除空白后取正则本身作为模板
		normalized = regexp.MustCompile(`\s+`).ReplaceAllString(keyword, "")
	} else {
		// 精确匹配：转为小写
		normalized = strings.ToLower(strings.TrimSpace(keyword))
	}
	h := sha1.Sum([]byte(normalized))
	return hex.EncodeToString(h[:8])
}

// pickSample 从 samples 中挑一条非空样本供统计存储。
func pickSample(samples []string) string {
	for _, s := range samples {
		if s != "" {
			return s
		}
	}
	return ""
}

// AlertProbeReq 调用 logc /file/search 的请求字段。
type AlertProbeReq struct {
	Path      string `json:"path"`
	Key       string `json:"key"`
	Line      int64  `json:"line"`
	Regex     bool   `json:"regex"`
	Level     string `json:"level"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	MaxLines  int    `json:"maxLines"`
	Query     string `json:"query"`
}
