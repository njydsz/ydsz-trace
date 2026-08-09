package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	models "ydsz-trace/logs/models"

	"github.com/robfig/cron/v3"
)

// Scanner 周期性评估启用的告警规则；命中阈值且不在冷却期时投递 webhook。
type Scheduler struct {
	cron *cron.Cron
	mu   sync.Mutex
	sem  chan struct{}
}

// probeFunc 实际调 logc 的钩子，便于单测替换。
var probeFunc = func(ctx context.Context, server string, req alertProbeReq, maxLines int) ([]string, error) {
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
	return &Scheduler{
		cron: cron.New(cron.WithSeconds()),
		sem:  make(chan struct{}, maxConcurrent),
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
}

// Stop 停止后台扫描。
func (s *Scheduler) Stop() {
	if s.cron != nil {
		s.cron.Stop()
	}
}

// ScanOnce 单次全量评估。
func (s *Scheduler) ScanOnce() {
	s.mu.Lock()
	defer s.mu.Unlock()

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
			evaluateAndFire(context.Background(), r, t)
		}(rule, now)
	}
}

func evaluateAndFire(ctx context.Context, rule models.AlertRule, _ time.Time) {
	targets, path := resolveAlertTargets(rule.ItemID, rule.ClientID)
	if len(targets) == 0 || path == "" {
		return
	}

	probe := alertProbeReq{
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

	result := FireWebhook(ctx, rule, totalMatches, samples)
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

// alertProbeReq 调用 logc /file/search 的请求字段。
type alertProbeReq struct {
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
