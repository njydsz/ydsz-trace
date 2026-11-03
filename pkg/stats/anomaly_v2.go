// Package stats 子文件：异常检测算法集（V2.2 升级）。
//
// 本文件在 engine.go 的滑动窗口 Z-score 基础上，补充三类检测器：
//
//   1. EWMA（指数加权移动平均）——对趋势变化更敏感，自适应衰减历史权重
//   2. 周期基线（同环比）——按小时/星期几分桶，消除日内周期性误报
//   3. 新增 / 消失模式检测——覆盖 Z-score 无法表达的"从无到有""从有到无"
//
// 设计原则：
//   - 各检测器相互独立，任一命中即产出 AnomalyEvent，由调用方决定如何合并
//   - 全部并发安全（AnomalyEngine 持有全局锁）
//   - 阈值可配置，默认值取自工程实践并允许通过 Options 覆盖
package stats

import (
	"math"
	"time"
)

// EWMA 默认参数。
const (
	// DefaultEWMALambda 指数平滑系数（0~1，越大越敏感）。
	// 0.3 是工程常用折中：既不过度反应单点噪声，也能在 3~5 个桶内捕捉趋势。
	DefaultEWMALambda = 0.3
	// DefaultEWMAMinSamples 进入 EWMA 判定所需的最小样本数。
	DefaultEWMAMinSamples = 4
	// DefaultEWMAThresholdSigma EWMA 判定的 sigma 倍数阈值。
	DefaultEWMAThresholdSigma = 2.5
)

// 周期基线默认参数。
const (
	// DefaultPeriodicBucketCount 周期桶数量（一天按 10 分钟分桶 = 144）。
	// 默认 24（小时级），兼顾内存与精度。
	DefaultPeriodicBucketCount = 24
	// DefaultPeriodicMinSamples 单桶最小样本数，低于此值不做周期判定。
	DefaultPeriodicMinSamples = 3
	// DefaultPeriodicSigma 周期判定的 sigma 倍数阈值。
	DefaultPeriodicSigma = 3.0
	// DefaultPeriodicMinMean 周期判定的最小均值，避免低基数误报。
	DefaultPeriodicMinMean = 5.0
)

// EWMAState 单个模板的 EWMA 状态。
type EWMAState struct {
	TemplateHash string
	ItemID       int64
	Value        float64   // 当前 EWMA 值
	Variance     float64   // EWMA 方差估计（用于 sigma 判定）
	Count        int       // 累计样本数
	LastUpdate   time.Time // 上次更新时间
}

// EWMA 基于指数加权移动平均的突变检测。
//
// 原理：
//
//	s_t = λ·x_t + (1-λ)·s_{t-1}
//	v_t = λ·(x_t - s_{t-1})² + (1-λ)·v_{t-1}
//	z_t = |x_t - s_{t-1}| / sqrt(v_{t-1})
//
// 当 z_t > thresholdSigma 时判定为异常。相比滑动窗口 Z-score，
// EWMA 不需要维护固定长度窗口，且对近期变化权重更高。
//
// 返回 (event, isAnomaly)。非异常时 event 为零值。
func (e *AnomalyEngine) EWMA(templateHash string, itemID int64, count int, lambda, thresholdSigma float64) (AnomalyEvent, bool) {
	if lambda <= 0 || lambda > 1 {
		lambda = DefaultEWMALambda
	}
	if thresholdSigma <= 0 {
		thresholdSigma = DefaultEWMAThresholdSigma
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	key := baselineKey(itemID, templateHash)
	st, ok := e.ewmaStates[key]
	if !ok {
		st = &EWMAState{TemplateHash: templateHash, ItemID: itemID}
		e.ewmaStates[key] = st
	}

	// 首次样本仅初始化，不判定（无历史可参照）
	if st.Count == 0 {
		st.Value = float64(count)
		st.Variance = 0
		st.Count = 1
		st.LastUpdate = time.Now()
		return AnomalyEvent{}, false
	}

	prev := st.Value
	prevVar := st.Variance
	if prevVar <= 0 {
		// 方差为零时用均值的平方根作保守估计，避免除零
		prevVar = math.Max(math.Abs(prev), 1)
	}

	// 计算 EWMA 偏移量（当前值 vs 上期平滑值）
	deviation := math.Abs(float64(count) - prev)
	z := deviation / math.Sqrt(prevVar)

	// 更新状态
	st.Value = lambda*float64(count) + (1-lambda)*prev
	st.Variance = lambda*deviation*deviation + (1-lambda)*prevVar
	st.Count++
	st.LastUpdate = time.Now()

	if st.Count < DefaultEWMAMinSamples {
		return AnomalyEvent{}, false
	}

	if z < thresholdSigma {
		return AnomalyEvent{}, false
	}

	direction := DirectionSurge
	severity := SeverityCritical
	if float64(count) < prev {
		direction = DirectionDrop
		severity = SeverityWarning
	}

	return AnomalyEvent{
		TemplateHash: templateHash,
		ItemID:       itemID,
		CurrentCount: count,
		BaselineMean: round2(prev),
		ZScore:       round2(z),
		Direction:    direction,
		Severity:     severity,
		Algorithm:    AlgorithmEWMA,
	}, true
}

// PeriodicBaseline 周期基线（同环比）状态。
//
// 将一天划分为固定数量的桶，分别维护每个桶的历史分布。
// 检测时取"同一桶位"的历史数据做 Z-score，消除日内周期性波动。
type PeriodicBaseline struct {
	TemplateHash string
	ItemID       int64
	// Buckets 每个桶位的计数样本（len = BucketCount）
	Buckets [][]float64
	// BucketCount 桶总数
	BucketCount int
	// LastUpdate 上次更新时间
	LastUpdate time.Time
}

// DetectPeriodic 基于周期基线的同环比检测。
//
// bucketIndex 由调用方给出（例如小时数或 10 分钟桶序号）。
// 返回 (event, isAnomaly)。
func (e *AnomalyEngine) DetectPeriodic(templateHash string, itemID int64, count int, bucketIndex int, bucketCount int, sigma float64) (AnomalyEvent, bool) {
	if bucketCount <= 0 {
		bucketCount = DefaultPeriodicBucketCount
	}
	if bucketIndex < 0 || bucketIndex >= bucketCount {
		return AnomalyEvent{}, false
	}
	if sigma <= 0 {
		sigma = DefaultPeriodicSigma
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	key := baselineKey(itemID, templateHash)
	pb, ok := e.periodic[key]
	if !ok {
		pb = &PeriodicBaseline{
			TemplateHash: templateHash,
			ItemID:       itemID,
			Buckets:      make([][]float64, bucketCount),
			BucketCount:  bucketCount,
		}
		e.periodic[key] = pb
	}

	// 桶位数量变化时重建（通常不会发生，防御性处理）
	if pb.BucketCount != bucketCount {
		pb.Buckets = make([][]float64, bucketCount)
		pb.BucketCount = bucketCount
	}

	history := pb.Buckets[bucketIndex]
	if len(history) < DefaultPeriodicMinSamples {
		// 样本不足，仅记录
		pb.Buckets[bucketIndex] = append(history, float64(count))
		pb.LastUpdate = time.Now()
		return AnomalyEvent{}, false
	}

	mean := calcMean(history)
	stddev := calcStdDev(history, mean)
	if mean < DefaultPeriodicMinMean {
		pb.Buckets[bucketIndex] = append(history, float64(count))
		pb.LastUpdate = time.Now()
		return AnomalyEvent{}, false
	}

	var z float64
	if stddev > 0 {
		z = math.Abs(float64(count)-mean) / stddev
	} else {
		z = math.Abs(float64(count)-mean) / mean * 5
	}

	// 滚动更新该桶的历史（保留最近 maxPeriodicHistory 个）
	pb.Buckets[bucketIndex] = append(history, float64(count))
	if len(pb.Buckets[bucketIndex]) > maxPeriodicHistory {
		pb.Buckets[bucketIndex] = pb.Buckets[bucketIndex][len(pb.Buckets[bucketIndex])-maxPeriodicHistory:]
	}
	pb.LastUpdate = time.Now()

	if z < sigma {
		return AnomalyEvent{}, false
	}

	direction := DirectionSurge
	severity := SeverityCritical
	if float64(count) < mean {
		direction = DirectionDrop
		severity = SeverityWarning
	}

	return AnomalyEvent{
		TemplateHash: templateHash,
		ItemID:       itemID,
		CurrentCount: count,
		BaselineMean: round2(mean),
		ZScore:       round2(z),
		Direction:    direction,
		Severity:     severity,
		Algorithm:    AlgorithmPeriodic,
	}, true
}

// maxPeriodicHistory 单个桶位保留的历史样本数。
const maxPeriodicHistory = 30

// DetectPatternLifecycle 检测模板的新增 / 消失。
//
// 新增（new）：首次出现的模板，且绝对计数达到 newThreshold。
// 消失（disappear）：历史有稳定基线，但当前窗口计数为 0 或接近 0。
//
// 这两个信号 Z-score 无法覆盖（前者无基线，后者被"低基数不判定"规则跳过），
// 因此独立成检测器。
//
// 返回 (event, isAnomaly)。
func (e *AnomalyEngine) DetectPatternLifecycle(templateHash string, itemID int64, count int, now time.Time) (AnomalyEvent, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	key := baselineKey(itemID, templateHash)
	bl, ok := e.baselines[key]

	// ── 新增模式 ──
	if !ok {
		// 首次见到：登记但不立即判定（避免启动瞬间全量误报）
		e.seenTemplates[key] = now
		if count >= DefaultNewPatternThreshold {
			// 同时存在历史 seen 记录但无基线，视为新增
			return AnomalyEvent{
				TemplateHash: templateHash,
				ItemID:       itemID,
				CurrentCount: count,
				BaselineMean: 0,
				ZScore:       float64(count),
				Direction:    DirectionSurge,
				Severity:     SeverityWarning,
				Algorithm:    AlgorithmNewPattern,
				AnomalyType:  AnomalyTypeNew,
			}, true
		}
		return AnomalyEvent{}, false
	}

	// ── 消失模式 ──
	// 条件：基线均值足够高（说明此前稳定出现），当前计数为 0，且距基线更新已超过宽限期
	if bl.Mean >= DefaultDisappearMinMean && count == 0 {
		grace := time.Duration(DefaultDisappearGraceBuckets) * time.Minute
		if now.Sub(bl.UpdatedAt) >= grace {
			return AnomalyEvent{
				TemplateHash: templateHash,
				ItemID:       itemID,
				CurrentCount: 0,
				BaselineMean: round2(bl.Mean),
				ZScore:       round2(bl.Mean),
				Direction:    DirectionDrop,
				Severity:     SeverityWarning,
				Algorithm:    AlgorithmDisappear,
				AnomalyType:  AnomalyTypeDisappear,
			}, true
		}
	}

	return AnomalyEvent{}, false
}

// 生命周期检测默认参数。
const (
	// DefaultNewPatternThreshold 新增模式的最小计数阈值。
	DefaultNewPatternThreshold = 10
	// DefaultDisappearMinMean 消失模式的最小基线均值（低于此值视为本就稀疏，不算异常）。
	DefaultDisappearMinMean = 5
	// DefaultDisappearGraceBuckets 消失判定的宽限期（分钟）。
	// 避免因单次采集失败导致误报。
	DefaultDisappearGraceBuckets = 15
)

// AnomalyType 异常事件子类型（用于区分新增/消失/突增）。
type AnomalyType string

const (
	// AnomalyTypeSpike 突增。
	AnomalyTypeSpike AnomalyType = "spike"
	// AnomalyTypeNew 新增模板。
	AnomalyTypeNew AnomalyType = "new"
	// AnomalyTypeDisappear 消失模板。
	AnomalyTypeDisappear AnomalyType = "disappear"
)

// 算法标识常量（写入 AnomalyEvent.Algorithm，便于前端展示与阈值调优）。
const (
	// AlgorithmZScore 滑动窗口 Z-score（engine.go 默认）。
	AlgorithmZScore = "z-score"
	// AlgorithmEWMA 指数加权移动平均。
	AlgorithmEWMA = "ewma"
	// AlgorithmPeriodic 周期基线同环比。
	AlgorithmPeriodic = "periodic"
	// AlgorithmNewPattern 新增模式。
	AlgorithmNewPattern = "new-pattern"
	// AlgorithmDisappear 消失模式。
	AlgorithmDisappear = "disappear"
)
