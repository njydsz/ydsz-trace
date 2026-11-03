// Package alert 子文件：多算法异常检测编排器（V2.2-01）。
//
// 职责：
//   - 对单个模板在一个时间桶的计数，并行运行 Z-score / EWMA / 周期基线 / 生命周期四类检测
//   - 合并结果并按严重度去重（同一模板同一桶只产出一条最高严重度事件）
//   - 对外暴露 DetectOne，供 pipeline 与 scanner 复用，避免各写一套
//
// 设计要点：
//   - 各检测器在 pkg/stats 内已保证并发安全，此处只做编排
//   - 新增算法只需在 detectors 列表中追加，不影响调用方
//   - 阈值常量集中在 Default*，便于后续从配置读取
package alert

import (
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/stats"
)

// Detector 单个检测算法的签名。
//
// 参数：
//   - current 当前桶计数
//   - bucketIndex 当前桶位（周期基线用，例如小时数）
//   - now 当前时间（生命周期宽限期用）
//
// 返回 (event, hit)。未命中时 event 为零值。
type Detector func(eng *stats.AnomalyEngine, templateHash string, itemID int64, current int, bucketIndex int, now time.Time) (stats.AnomalyEvent, bool)

// DefaultBucketCount 周期基线的默认桶数（24 = 小时级）。
const DefaultBucketCount = 24

// detectors 全部启用的检测算法。
//
// 顺序即优先级：先命中的算法会先填充 event，后续命中者仅在更严重时覆盖。
var detectors = []struct {
	Name string
	Fn   Detector
}{
	{Name: stats.AlgorithmZScore, Fn: detectZScore},
	{Name: stats.AlgorithmEWMA, Fn: detectEWMA},
	{Name: stats.AlgorithmPeriodic, Fn: detectPeriodic},
	{Name: stats.AlgorithmNewPattern, Fn: detectLifecycle},
}

// detectZScore 滑动窗口 Z-score 检测。
//
// 注意：Z-score 需要历史基线，样本不足时不命中（由 EWMA / 生命周期补充）。
func detectZScore(eng *stats.AnomalyEngine, hash string, itemID int64, current int, _ int, _ time.Time) (stats.AnomalyEvent, bool) {
	events := eng.DetectAnomalies(map[string]int{hash: current}, time.Now().Format("2006-01-02 15:04:05"))
	for _, ev := range events {
		if ev.TemplateHash == hash {
			return ev, true
		}
	}
	return stats.AnomalyEvent{}, false
}

// detectEWMA 指数加权移动平均检测。
func detectEWMA(eng *stats.AnomalyEngine, hash string, itemID int64, current int, _ int, _ time.Time) (stats.AnomalyEvent, bool) {
	return eng.EWMA(hash, itemID, current, stats.DefaultEWMALambda, stats.DefaultEWMAThresholdSigma)
}

// detectPeriodic 周期基线（同环比）检测。
func detectPeriodic(eng *stats.AnomalyEngine, hash string, itemID int64, current int, bucketIndex int, _ time.Time) (stats.AnomalyEvent, bool) {
	return eng.DetectPeriodic(hash, itemID, current, bucketIndex, DefaultBucketCount, stats.DefaultPeriodicSigma)
}

// detectLifecycle 新增 / 消失模式检测。
func detectLifecycle(eng *stats.AnomalyEngine, hash string, itemID int64, current int, _ int, now time.Time) (stats.AnomalyEvent, bool) {
	return eng.DetectPatternLifecycle(hash, itemID, current, now)
}

// DetectOne 对单个模板运行全部检测算法，返回合并后的事件列表。
//
// 合并规则：同一算法只保留一条；不同算法同时命中时全部保留（它们描述的是不同维度的异常，
// 例如 EWMA 报"趋势突变"、生命周期报"模板新增"，对用户都是有价值的信息）。
//
// 调用方需负责 UpdateBaseline（本函数不修改基线，避免检测与学习耦合）。
func DetectOne(eng *stats.AnomalyEngine, hash string, itemID int64, current int, bucketIndex int, now time.Time) []stats.AnomalyEvent {
	if eng == nil {
		return nil
	}

	events := make([]stats.AnomalyEvent, 0, 2)
	for _, d := range detectors {
		ev, hit := d.Fn(eng, hash, itemID, current, bucketIndex, now)
		if hit {
			// 补全算法标识（部分检测器返回时已带，此处兜底）
			if ev.Algorithm == "" {
				ev.Algorithm = d.Name
			}
			events = append(events, ev)
		}
	}
	return events
}

// BucketIndexFromTime 从时间计算周期基线桶位（小时级 0~23）。
//
// 与 DefaultBucketCount=24 配合使用；若后续改为更细粒度，只需替换本函数。
func BucketIndexFromTime(t time.Time) int {
	return t.Hour() % DefaultBucketCount
}

// SeverityRank 严重度排序权重（数值越大越严重）。
func SeverityRank(sev string) int {
	switch sev {
	case stats.SeverityCritical:
		return 2
	case stats.SeverityWarning:
		return 1
	default:
		return 0
	}
}

// anomalyTypeOf 将检测事件映射为持久化的异常类型（models.AnomalyType）。
//
// 优先使用检测器给出的 AnomalyType（新增/消失有明确语义）；
// 缺失时按方向推断：下降归为 disappear，其余归为 spike。
func anomalyTypeOf(ev stats.AnomalyEvent) models.AnomalyType {
	// stats.AnomalyType 与 models.AnomalyType 是两套独立定义（值相同），
	// 此处按字符串值映射，避免 pkg 依赖 logs 造成架构倒置。
	switch string(ev.AnomalyType) {
	case string(models.AnomalyNew):
		return models.AnomalyNew
	case string(models.AnomalyDisappear):
		return models.AnomalyDisappear
	case string(models.AnomalySpike):
		return models.AnomalySpike
	}

	if ev.Direction == stats.DirectionDrop {
		return models.AnomalyDisappear
	}
	return models.AnomalySpike
}
