// Package perf 提供端侧资源护栏与采样（V2.2-11，FR-105/FR-106）。
//
// 背景：
//   logc 作为常驻 Agent 部署在业务机器上，必须严格自律：
//   CPU 抢占会影响业务，内存膨胀会被 OOM Kill。
//   本包提供两类保护：
//     1. 采样器（Sampler）：高流量时按比例丢弃低优先级日志
//     2. 资源护栏（Guardian）：周期性检查 CPU/内存，超阈值时自动降级采样
//
// 设计原则：
//   - 纯内存计算，无外部依赖（不读 /proc 以外的系统接口）
//   - 所有阈值可通过环境变量覆盖，默认值保守
//   - 采样决策必须 O(1)，不能成为新的性能瓶颈
//
// 环境变量：
//   YDSZ_SAMPLE_RATE        默认采样率（1.0 = 不采样）
//   YDSZ_SAMPLE_LEVEL       保留的最低级别（低于该级别才参与采样）
//   YDSZ_MAX_CPU_PERCENT    CPU 使用率阈值（%）
//   YDSZ_MAX_MEM_MB         内存阈值（MB）
package perf

import (
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 默认阈值。
const (
	// DefaultSampleRate 默认采样率（1.0 = 全量保留）
	DefaultSampleRate = 1.0
	// DefaultMaxCPUPercent CPU 阈值默认 20%（单核占比）
	DefaultMaxCPUPercent = 20.0
	// DefaultMaxMemMB 内存阈值默认 50MB
	DefaultMaxMemMB = 50.0
	// DefaultSampleLevel 默认只对 INFO 及以下级别采样（ERROR/FATAL 永不丢）
	DefaultSampleLevel = "INFO"
)

// Sampler 日志采样器。
//
// 决策逻辑：
//   - ERROR / FATAL / WARN 永远保留（告警不能丢）
//   - 其余级别按 sampleRate 概率保留
//   - 高负载时由 Guardian 动态降低 sampleRate
type Sampler struct {
	// rate 当前采样率（原子读写，1.0 = 全量）
	rate atomic.Uint64
	// sampleLevel 参与采样的最低级别
	sampleLevel string
	// rng 并发安全的随机源
	rngMu sync.Mutex
	rng   *rand.Rand
}

// NewSampler 按环境变量创建采样器。
//
// 未配置时返回 rate=1.0 的直通采样器（不影响性能）。
func NewSampler() *Sampler {
	rate := DefaultSampleRate
	if v := os.Getenv("YDSZ_SAMPLE_RATE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f <= 1.0 {
			rate = f
		}
	}

	s := &Sampler{
		sampleLevel: strings.ToUpper(os.Getenv("YDSZ_SAMPLE_LEVEL")),
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	if s.sampleLevel == "" {
		s.sampleLevel = DefaultSampleLevel
	}
	s.setRate(rate)
	return s
}

// setRate 设置采样率（内部用，保证原子编码）。
func (s *Sampler) setRate(rate float64) {
	// 用 uint64 存储定点数（精度 1e-6），避免原子操作 float64 的平台差异
	encoded := uint64(rate * 1e6)
	s.rate.Store(encoded)
}

// Rate 返回当前采样率。
func (s *Sampler) Rate() float64 {
	return float64(s.rate.Load()) / 1e6
}

// SetRate 外部调整采样率（供 Guardian 动态降级）。
func (s *Sampler) SetRate(rate float64) {
	if rate <= 0 {
		rate = 0.01 // 下限 1%，避免完全丢弃
	}
	if rate > 1.0 {
		rate = 1.0
	}
	s.setRate(rate)
}

// ShouldKeep 判断一条日志是否应保留。
//
// level 为日志级别字符串（如 "INFO"、"ERROR"）。
// 返回 true 表示保留，false 表示丢弃。
func (s *Sampler) ShouldKeep(level string) bool {
	rate := s.Rate()
	if rate >= 1.0 {
		return true
	}

	lv := strings.ToUpper(strings.TrimSpace(level))
	if lv == "" {
		// 无法识别级别时保守保留
		return true
	}

	// 未知级别：保守保留（不参与采样），避免误丢无法判断重要性的日志
	if !isKnownLevel(lv) {
		return true
	}

	// 高于 sampleLevel 的级别永不采样（ERROR/FATAL 必留）
	if levelRank(lv) > levelRank(s.sampleLevel) {
		return true
	}

	s.rngMu.Lock()
	keep := s.rng.Float64() < rate
	s.rngMu.Unlock()
	return keep
}

// isKnownLevel 判断是否为系统可识别的日志级别。
func isKnownLevel(level string) bool {
	switch level {
	case "FATAL", "ERROR", "WARN", "WARNING", "INFO", "DEBUG", "TRACE":
		return true
	}
	return false
}

// levelRank 级别优先级（数值越大越严重）。
func levelRank(level string) int {
	switch level {
	case "FATAL":
		return 5
	case "ERROR":
		return 4
	case "WARN", "WARNING":
		return 3
	case "INFO":
		return 2
	case "DEBUG":
		return 1
	case "TRACE":
		return 0
	}
	// 未知级别按 INFO 处理
	return 2
}

// Guardian 资源护栏：周期性检查 CPU/内存并动态调整采样率。
type Guardian struct {
	sampler      *Sampler
	maxCPUPercent float64
	maxMemMB     float64
	checkInterval time.Duration

	stopCh chan struct{}
	doneCh chan struct{}
}

// NewGuardian 按环境变量创建资源护栏。
func NewGuardian(sampler *Sampler) *Guardian {
	g := &Guardian{
		sampler:       sampler,
		maxCPUPercent: DefaultMaxCPUPercent,
		maxMemMB:      DefaultMaxMemMB,
		checkInterval: 30 * time.Second,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}

	if v := os.Getenv("YDSZ_MAX_CPU_PERCENT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			g.maxCPUPercent = f
		}
	}
	if v := os.Getenv("YDSZ_MAX_MEM_MB"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			g.maxMemMB = f
		}
	}
	if v := os.Getenv("YDSZ_GUARD_INTERVAL_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			g.checkInterval = time.Duration(n) * time.Second
		}
	}
	return g
}

// Start 启动护栏 goroutine。
func (g *Guardian) Start() {
	go func() {
		defer close(g.doneCh)
		ticker := time.NewTicker(g.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-g.stopCh:
				return
			case <-ticker.C:
				g.check()
			}
		}
	}()
}

// check 执行一次资源检查并调整采样率。
func (g *Guardian) check() {
	memMB := currentMemMB()
	if memMB <= 0 {
		return
	}

	currentRate := g.sampler.Rate()

	switch {
	case memMB > g.maxMemMB:
		// 内存超限：采样率减半
		newRate := currentRate * 0.5
		g.sampler.SetRate(newRate)
	case memMB > g.maxMemMB*0.8:
		// 内存接近阈值：轻微收紧
		newRate := currentRate * 0.9
		g.sampler.SetRate(newRate)
	case memMB < g.maxMemMB*0.5 && currentRate < 1.0:
		// 内存充裕：逐步恢复到全量
		newRate := currentRate * 1.1
		g.sampler.SetRate(newRate)
	}
}

// Stop 停止护栏 goroutine。
func (g *Guardian) Stop() {
	close(g.stopCh)
	<-g.doneCh
}

// Stats 返回护栏当前状态（供监控接口使用）。
type Stats struct {
	SampleRate   float64 `json:"sampleRate"`
	MaxCPUPercent float64 `json:"maxCpuPercent"`
	MaxMemMB     float64 `json:"maxMemMb"`
	CurrentMemMB float64 `json:"currentMemMb"`
	CheckIntervalSec int `json:"checkIntervalSec"`
}

// Stats 返回当前统计。
func (g *Guardian) Stats() Stats {
	return Stats{
		SampleRate:       g.sampler.Rate(),
		MaxCPUPercent:    g.maxCPUPercent,
		MaxMemMB:         g.maxMemMB,
		CurrentMemMB:     currentMemMB(),
		CheckIntervalSec: int(g.checkInterval / time.Second),
	}
}
