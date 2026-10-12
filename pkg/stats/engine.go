// Package stats 独立的统计聚合引擎。
//
// 提供异常检测（突变/骤降）、基线学习（滑动窗口均值+标准差），
// 可被 logs 模块的 Scanner 引用，实现 FR-801/FR-802 功能。
package stats

import (
	"math"
	"sync"
	"time"
)

// maxWindowSize 滑动窗口大小（最近 N 个时间桶）。
const maxWindowSize = 30

// AnomalyEngine 异常检测引擎（单实例，并发安全）。
type AnomalyEngine struct {
	baselines map[string]*Baseline // key = itemID:templateHash
	mu        sync.RWMutex
}

// Baseline 某个模板的周期基线。
type Baseline struct {
	TemplateHash string
	ItemID       int64
	Mean         float64   // 近 30 桶均值
	StdDev       float64   // 近 30 桶标准差
	Last30       []float64 // 最近 30 个时间桶的计数
	UpdatedAt    time.Time
}

// AnomalyEvent 异常事件（用于告警）。
type AnomalyEvent struct {
	TemplateHash string  `json:"templateHash"`
	ItemID       int64   `json:"itemId"`
	CurrentCount int     `json:"currentCount"`
	BaselineMean float64 `json:"baselineMean"`
	ZScore       float64 `json:"zScore"`
	Direction    string  `json:"direction"` // "surge" / "drop"
	Severity     string  `json:"severity"`  // "critical" / "warning"
}

// anomalyDirection 方向常量。
const (
	DirectionSurge = "surge"
	DirectionDrop  = "drop"
)

// severity 级别常量。
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
)

// NewAnomalyEngine 创建异常检测引擎单例。
func NewAnomalyEngine() *AnomalyEngine {
	return &AnomalyEngine{
		baselines: make(map[string]*Baseline),
	}
}

// baselineKey 构造 baselines map 的复合 key。
func baselineKey(itemID int64, templateHash string) string {
	return itoa(itemID) + ":" + templateHash
}

// itoa int64 转字符串的轻量替代。
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// DetectAnomalies 对当前计数做突变检测：Z-score > 3 或泊松 P < 0.01 即视为异常。
//
// current 的 key = templateHash，value = 当前桶计数。
func (e *AnomalyEngine) DetectAnomalies(current map[string]int, timestamp string) []AnomalyEvent {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var events []AnomalyEvent
	for templateHash, count := range current {
		key := baselineKey(0, templateHash)
		bl, ok := e.baselines[key]
		if !ok || len(bl.Last30) < 3 {
			// 数据不足，跳过（需要至少 3 桶数据）
			continue
		}

		mean := bl.Mean
		stddev := bl.StdDev

		var zscore float64
		if stddev > 0 {
			zscore = math.Abs(float64(count)-mean) / stddev
		} else {
			// stddev=0 时改用 mean 的绝对变化率
			if mean > 0 {
				zscore = math.Abs(float64(count)-mean) / mean
			} else {
				zscore = float64(count)
			}
		}

		// 泊松简化：如果 current > Mean + 3*sqrt(Mean)，标记异常
		threshold := mean + 3*math.Sqrt(math.Max(mean, 1))

		event := AnomalyEvent{
			TemplateHash: templateHash,
			ItemID:       bl.ItemID,
			CurrentCount: count,
			BaselineMean: round2(mean),
			ZScore:       round2(zscore),
		}

		if float64(count) > threshold && zscore > 3 {
			event.Direction = DirectionSurge
			event.Severity = SeverityCritical
			events = append(events, event)
		} else if float64(count) < mean-stddev*2 && mean > 5 && zscore > 2 {
			// drop: 下降超过 2 个标准差且均值 > 5（避免低噪误报）
			event.Direction = DirectionDrop
			event.Severity = SeverityWarning
			events = append(events, event)
		}
	}
	return events
}

// UpdateBaseline 更新某模式的基线（滑动窗口）。
func (e *AnomalyEngine) UpdateBaseline(templateHash string, itemID int64, count int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	key := baselineKey(itemID, templateHash)
	bl, ok := e.baselines[key]
	if !ok {
		bl = &Baseline{TemplateHash: templateHash, ItemID: itemID}
		e.baselines[key] = bl
	}

	// 滑动窗口：追加新值，超出窗口则丢弃旧值
	bl.Last30 = append(bl.Last30, float64(count))
	if len(bl.Last30) > maxWindowSize {
		bl.Last30 = bl.Last30[len(bl.Last30)-maxWindowSize:]
	}

	// 重新计算均值和标准差
	bl.Mean = calcMean(bl.Last30)
	bl.StdDev = calcStdDev(bl.Last30, bl.Mean)
	bl.UpdatedAt = time.Now()
}

// GetBaseline 读取指定模式的基线状态。
func (e *AnomalyEngine) GetBaseline(templateHash string, itemID int64) (*Baseline, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	key := baselineKey(itemID, templateHash)
	bl, ok := e.baselines[key]
	if !ok {
		return nil, false
	}
	// 返回副本避免外部修改
	cp := *bl
	return &cp, true
}

// calcMean 计算平均值。
func calcMean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

// calcStdDev 计算总体标准差（非样本标准差）。
func calcStdDev(vals []float64, mean float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	sumSq := 0.0
	for _, v := range vals {
		diff := v - mean
		sumSq += diff * diff
	}
	return math.Sqrt(sumSq / float64(len(vals)))
}

// round2 保留两位小数。
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
