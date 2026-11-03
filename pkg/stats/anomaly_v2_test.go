package stats

import (
	"testing"
	"time"
)

// TestEWMA_FirstSampleNoAlarm 首次样本只初始化，不告警。
func TestEWMA_FirstSampleNoAlarm(t *testing.T) {
	e := NewAnomalyEngine()
	ev, ok := e.EWMA("tpl-a", 1, 100, DefaultEWMALambda, DefaultEWMAThresholdSigma)
	if ok {
		t.Fatalf("首次样本不应告警, got %+v", ev)
	}
}

// TestEWMA_SurgeDetected 稳定基线后突增应被 EWMA 捕获。
func TestEWMA_SurgeDetected(t *testing.T) {
	e := NewAnomalyEngine()
	const hash = "tpl-surge"

	// 预热：稳定在 100 附近
	for i := 0; i < DefaultEWMAMinSamples; i++ {
		_, _ = e.EWMA(hash, 1, 100+i%3, DefaultEWMALambda, DefaultEWMAThresholdSigma)
	}

	ev, ok := e.EWMA(hash, 1, 500, DefaultEWMALambda, DefaultEWMAThresholdSigma)
	if !ok {
		t.Fatal("突增未被 EWMA 检出")
	}
	if ev.Direction != DirectionSurge {
		t.Errorf("direction = %q want %q", ev.Direction, DirectionSurge)
	}
	if ev.Algorithm != AlgorithmEWMA {
		t.Errorf("algorithm = %q want %q", ev.Algorithm, AlgorithmEWMA)
	}
	if ev.Severity != SeverityCritical {
		t.Errorf("severity = %q want %q", ev.Severity, SeverityCritical)
	}
}

// TestEWMA_SteadyNoAlarm 稳定序列不应误报。
func TestEWMA_SteadyNoAlarm(t *testing.T) {
	e := NewAnomalyEngine()
	const hash = "tpl-steady"
	alarms := 0
	for i := 0; i < 20; i++ {
		_, ok := e.EWMA(hash, 1, 100, DefaultEWMALambda, DefaultEWMAThresholdSigma)
		if ok {
			alarms++
		}
	}
	if alarms != 0 {
		t.Errorf("稳定序列误报 %d 次", alarms)
	}
}

// TestEWMA_DropDetected 骤降应被识别为 warning。
func TestEWMA_DropDetected(t *testing.T) {
	e := NewAnomalyEngine()
	const hash = "tpl-drop"
	for i := 0; i < DefaultEWMAMinSamples; i++ {
		_, _ = e.EWMA(hash, 1, 100, DefaultEWMALambda, DefaultEWMAThresholdSigma)
	}
	ev, ok := e.EWMA(hash, 1, 1, DefaultEWMALambda, DefaultEWMAThresholdSigma)
	if !ok {
		t.Fatal("骤降未被检出")
	}
	if ev.Direction != DirectionDrop {
		t.Errorf("direction = %q want %q", ev.Direction, DirectionDrop)
	}
}

// TestPeriodic_NeedsHistory 样本不足时不告警。
func TestPeriodic_NeedsHistory(t *testing.T) {
	e := NewAnomalyEngine()
	const hash = "tpl-periodic"
	for i := 0; i < DefaultPeriodicMinSamples-1; i++ {
		_, ok := e.DetectPeriodic(hash, 1, 100, 3, 24, DefaultPeriodicSigma)
		if ok {
			t.Fatalf("样本不足时不应告警 (i=%d)", i)
		}
	}
}

// TestPeriodic_CyclicPatternDetected 同桶位突增应被检出。
func TestPeriodic_CyclicPatternDetected(t *testing.T) {
	e := NewAnomalyEngine()
	const hash = "tpl-cycle"

	// 用 5 个完整周期预热桶位 3（每小时固定 100 次）
	for cycle := 0; cycle < 6; cycle++ {
		for bucket := 0; bucket < 24; bucket++ {
			count := 100
			if bucket == 3 {
				count = 100 // 保持稳定基线
			}
			_, _ = e.DetectPeriodic(hash, 1, count, bucket, 24, DefaultPeriodicSigma)
		}
	}

	ev, ok := e.DetectPeriodic(hash, 1, 900, 3, 24, DefaultPeriodicSigma)
	if !ok {
		t.Fatal("周期基线突增未被检出")
	}
	if ev.Algorithm != AlgorithmPeriodic {
		t.Errorf("algorithm = %q want %q", ev.Algorithm, AlgorithmPeriodic)
	}
}

// TestPeriodic_DifferentBucketNoAlarm 不同桶位的波动不应互相干扰。
func TestPeriodic_DifferentBucketNoAlarm(t *testing.T) {
	e := NewAnomalyEngine()
	const hash = "tpl-isolated"

	// 桶位 5 建立高基线，桶位 6 保持低值
	for cycle := 0; cycle < 6; cycle++ {
		_, _ = e.DetectPeriodic(hash, 1, 1000, 5, 24, DefaultPeriodicSigma)
		_, _ = e.DetectPeriodic(hash, 1, 5, 6, 24, DefaultPeriodicSigma)
	}

	_, ok := e.DetectPeriodic(hash, 1, 6, 6, 24, DefaultPeriodicSigma)
	if ok {
		t.Fatal("不同桶位不应互相影响")
	}
}

// TestLifecycle_NewPatternDetected 首次出现且计数达标应报新增。
func TestLifecycle_NewPatternDetected(t *testing.T) {
	e := NewAnomalyEngine()
	const hash = "tpl-brand-new"

	ev, ok := e.DetectPatternLifecycle(hash, 1, 50, time.Now())
	if !ok {
		t.Fatal("新增模式未被检出")
	}
	if ev.AnomalyType != AnomalyTypeNew {
		t.Errorf("anomalyType = %q want %q", ev.AnomalyType, AnomalyTypeNew)
	}
}

// TestLifecycle_NewPatternBelowThreshold 低计数首次出现不报。
func TestLifecycle_NewPatternBelowThreshold(t *testing.T) {
	e := NewAnomalyEngine()
	const hash = "tpl-quiet"

	_, ok := e.DetectPatternLifecycle(hash, 1, 2, time.Now())
	if ok {
		t.Fatal("低计数首次出现不应报新增")
	}
}

// TestLifecycle_DisappearDetected 基线稳定后归零且过宽限期应报消失。
func TestLifecycle_DisappearDetected(t *testing.T) {
	e := NewAnomalyEngine()
	const hash = "tpl-vanish"

	// 建立基线
	for i := 0; i < 10; i++ {
		e.UpdateBaseline(hash, 1, 100)
	}
	// 强制让基线更新时间落在宽限期之前
	e.mu.Lock()
	e.baselines[baselineKey(1, hash)].UpdatedAt = time.Now().Add(-20 * time.Minute)
	e.mu.Unlock()

	ev, ok := e.DetectPatternLifecycle(hash, 1, 0, time.Now())
	if !ok {
		t.Fatal("消失模式未被检出")
	}
	if ev.AnomalyType != AnomalyTypeDisappear {
		t.Errorf("anomalyType = %q want %q", ev.AnomalyType, AnomalyTypeDisappear)
	}
}

// TestLifecycle_DisappearWithinGrace 宽限期内不报消失。
func TestLifecycle_DisappearWithinGrace(t *testing.T) {
	e := NewAnomalyEngine()
	const hash = "tpl-grace"

	for i := 0; i < 10; i++ {
		e.UpdateBaseline(hash, 1, 100)
	}
	// 刚刚更新过基线，处于宽限期内
	_, ok := e.DetectPatternLifecycle(hash, 1, 0, time.Now())
	if ok {
		t.Fatal("宽限期内不应报消失")
	}
}

// TestLifecycle_LowMeanNoDisappear 低均值归零不报消失（本就稀疏）。
func TestLifecycle_LowMeanNoDisappear(t *testing.T) {
	e := NewAnomalyEngine()
	const hash = "tpl-lowmean"

	for i := 0; i < 10; i++ {
		e.UpdateBaseline(hash, 1, 2)
	}
	e.mu.Lock()
	e.baselines[baselineKey(1, hash)].UpdatedAt = time.Now().Add(-20 * time.Minute)
	e.mu.Unlock()

	_, ok := e.DetectPatternLifecycle(hash, 1, 0, time.Now())
	if ok {
		t.Fatal("低均值归零不应报消失")
	}
}

// TestDetectAnomalies_AlgorithmField 基础 Z-score 路径应标注算法。
func TestDetectAnomalies_AlgorithmField(t *testing.T) {
	e := NewAnomalyEngine()
	const hash = "tpl-z"

	for i := 0; i < 10; i++ {
		e.UpdateBaseline(hash, 1, 100)
	}
	events := e.DetectAnomalies(map[string]int{hash: 900}, time.Now().Format("2006-01-02 15:04:05"))
	if len(events) == 0 {
		t.Fatal("未检出异常")
	}
	if events[0].Algorithm != AlgorithmZScore {
		t.Errorf("algorithm = %q want %q", events[0].Algorithm, AlgorithmZScore)
	}
}
