// Package perf 子文件：日志分层生命周期（V2.4-04，FR-305）。
//
// 背景：
//   日志保留成本随时间线性增长，但价值随时间衰减。
//   PRD FR-305 要求按热/温/冷三级管理日志生命周期：
//
//     热（hot）  ：最近 N 天，完整存储，实时可查
//     温（warm） ：N~M 天，压缩存储，查询稍慢
//     冷（cold） ：M 天以前，仅保留统计与模板，原文归档或删除
//
// 设计：
//   - 判定基于日志日期（YYYYMMDD），而非文件修改时间
//   - 冷却动作可配置：仅标记 / 压缩 / 删除原文
//   - 提供 dry-run 预览接口，避免误删
//   - 所有策略参数可通过环境变量覆盖
package perf

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 分层常量。
const (
	// TierHot 热数据（完整存储，实时可查）
	TierHot = "hot"
	// TierWarm 温数据（压缩存储）
	TierWarm = "warm"
	// TierCold 冷数据（仅统计，原文归档/删除）
	TierCold = "cold"
)

// 默认保留策略。
const (
	// DefaultHotDays 热数据保留天数
	DefaultHotDays = 7
	// DefaultWarmDays 温数据保留天数（超过即转冷）
	DefaultWarmDays = 30
	// DefaultColdAction 冷数据动作：keep / compress / delete
	DefaultColdAction = "compress"
)

// 配置环境变量。
const (
	// EnvHotDays 热数据天数
	EnvHotDays = "YDSZ_HOT_DAYS"
	// EnvWarmDays 温数据天数
	EnvWarmDays = "YDSZ_WARM_DAYS"
	// EnvColdAction 冷数据动作
	EnvColdAction = "YDSZ_COLD_ACTION"
)

// LifecyclePolicy 生命周期策略。
type LifecyclePolicy struct {
	// HotDays 热数据保留天数
	HotDays int `json:"hotDays"`
	// WarmDays 温数据保留天数（HotDays < age <= WarmDays 为温）
	WarmDays int `json:"warmDays"`
	// ColdAction 冷数据动作：keep / compress / delete
	ColdAction string `json:"coldAction"`
}

// DefaultLifecyclePolicy 返回默认策略。
func DefaultLifecyclePolicy() LifecyclePolicy {
	p := LifecyclePolicy{
		HotDays:    DefaultHotDays,
		WarmDays:   DefaultWarmDays,
		ColdAction: DefaultColdAction,
	}
	if v := os.Getenv(EnvHotDays); v != "" {
		if n, err := atoi(v); err == nil && n > 0 {
			p.HotDays = n
		}
	}
	if v := os.Getenv(EnvWarmDays); v != "" {
		if n, err := atoi(v); err == nil && n > 0 {
			p.WarmDays = n
		}
	}
	if v := os.Getenv(EnvColdAction); v != "" {
		switch v {
		case "keep", "compress", "delete":
			p.ColdAction = v
		}
	}
	return p
}

// Validate 校验策略合法性。
func (p LifecyclePolicy) Validate() error {
	if p.HotDays <= 0 || p.WarmDays <= 0 {
		return fmt.Errorf("lifecycle: 保留天数必须为正")
	}
	if p.WarmDays <= p.HotDays {
		return fmt.Errorf("lifecycle: WarmDays(%d) 必须大于 HotDays(%d)", p.WarmDays, p.HotDays)
	}
	switch p.ColdAction {
	case "keep", "compress", "delete":
	default:
		return fmt.Errorf("lifecycle: ColdAction 非法: %q", p.ColdAction)
	}
	return nil
}

// Classify 按日志日期判定分层。
//
// dateStr 格式 YYYYMMDD；解析失败时归入热数据（保守策略，避免误删）。
func (p LifecyclePolicy) Classify(dateStr string) string {
	t, err := time.ParseInLocation("20060102", dateStr, time.Local)
	if err != nil {
		// 无法解析日期 → 保守归为热数据
		return TierHot
	}

	ageDays := int(time.Since(t).Hours() / 24)
	switch {
	case ageDays <= p.HotDays:
		return TierHot
	case ageDays <= p.WarmDays:
		return TierWarm
	default:
		return TierCold
	}
}

// ShouldArchive 判断该日期的日志是否应归档（转温或转冷）。
func (p LifecyclePolicy) ShouldArchive(dateStr string) bool {
	return p.Classify(dateStr) != TierHot
}

// ShouldDelete 判断该日期的日志是否应删除原文。
func (p LifecyclePolicy) ShouldDelete(dateStr string) bool {
	return p.ColdAction == "delete" && p.Classify(dateStr) == TierCold
}

// LifecycleReport 单个文件的处理报告。
type LifecycleReport struct {
	// Path 文件路径
	Path string `json:"path"`
	// Date 日志日期
	Date string `json:"date"`
	// Tier 判定分层
	Tier string `json:"tier"`
	// Action 建议动作：keep / compress / delete
	Action string `json:"action"`
	// SizeBytes 文件大小
	SizeBytes int64 `json:"sizeBytes"`
	// Reason 动作原因
	Reason string `json:"reason"`
}

// PlanLifecycle 扫描目录并生成生命周期处理计划（dry-run）。
//
// 不执行任何实际操作，仅返回建议清单。
func PlanLifecycle(rootDir string, policy LifecyclePolicy) ([]LifecycleReport, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %w", err)
	}

	reports := make([]LifecycleReport, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		date, ok := extractDateFromFilename(name)
		if !ok {
			continue
		}

		info, ierr := e.Info()
		if ierr != nil {
			continue
		}

		report := LifecycleReport{
			Path:      filepath.Join(rootDir, name),
			Date:      date,
			Tier:      policy.Classify(date),
			SizeBytes: info.Size(),
		}

		switch report.Tier {
		case TierHot:
			report.Action = "keep"
			report.Reason = fmt.Sprintf("age <= %d 天，保留完整", policy.HotDays)
		case TierWarm:
			report.Action = "compress"
			report.Reason = fmt.Sprintf("%d < age <= %d 天，压缩存储", policy.HotDays, policy.WarmDays)
		case TierCold:
			if policy.ColdAction == "delete" {
				report.Action = "delete"
				report.Reason = fmt.Sprintf("age > %d 天，策略为删除原文", policy.WarmDays)
			} else {
				report.Action = policy.ColdAction
				report.Reason = fmt.Sprintf("age > %d 天，策略为 %s", policy.WarmDays, policy.ColdAction)
			}
		}

		reports = append(reports, report)
	}

	// 按日期倒序（新→旧）
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].Date > reports[j].Date
	})
	return reports, nil
}

// ExtractDateFromFilename 从文件名提取 YYYYMMDD 日期。
//
// 支持 log-20261106.log / 20261106.log / app_20261106_001.log 三种命名。
func extractDateFromFilename(name string) (string, bool) {
	// 取前 8 位连续数字
	for i := 0; i+8 <= len(name); i++ {
		seg := name[i : i+8]
		if !isAllDigits(seg) {
			continue
		}
		// 校验月份与日期范围，避免把版本号当日期
		month := seg[4:6]
		day := seg[6:8]
		if month >= "01" && month <= "12" && day >= "01" && day <= "31" {
			return seg, true
		}
	}
	return "", false
}

// ApplyLifecycle 执行生命周期处理（真实操作）。
//
// dryRun=true 时仅打印计划不执行。
func ApplyLifecycle(rootDir string, policy LifecyclePolicy, dryRun bool) (applied, skipped int, err error) {
	reports, err := PlanLifecycle(rootDir, policy)
	if err != nil {
		return 0, 0, err
	}

	for _, r := range reports {
		if r.Action == "keep" {
			skipped++
			continue
		}
		if dryRun {
			log.Printf("[lifecycle][dry-run] %s → %s (%s)", r.Path, r.Action, r.Reason)
			applied++
			continue
		}

		switch r.Action {
		case "compress":
			if cerr := compressFile(r.Path); cerr != nil {
				log.Printf("[lifecycle] 压缩失败 %s: %v", r.Path, cerr)
				continue
			}
		case "delete":
			if derr := os.Remove(r.Path); derr != nil {
				log.Printf("[lifecycle] 删除失败 %s: %v", r.Path, derr)
				continue
			}
		}
		applied++
	}
	return applied, skipped, nil
}

// compressFile 压缩单个文件（gzip + .gz 后缀，保留原文件直到压缩成功）。
func compressFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	tmp := path + ".gz.tmp"
	if err := os.WriteFile(tmp, gzipBytes(raw), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path+".gz"); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// 压缩成功后删除原文件
	return os.Remove(path)
}

// LifecycleStats 分层统计。
type LifecycleStatsReport struct {
	HotCount   int   `json:"hotCount"`
	WarmCount  int   `json:"warmCount"`
	ColdCount  int   `json:"coldCount"`
	HotBytes   int64 `json:"hotBytes"`
	WarmBytes  int64 `json:"warmBytes"`
	ColdBytes  int64 `json:"coldBytes"`
	TotalBytes int64 `json:"totalBytes"`
}

// Stats 汇总分层统计。
func Summarize(reports []LifecycleReport) LifecycleStatsReport {
	var s LifecycleStatsReport
	for _, item := range reports {
		switch item.Tier {
		case TierHot:
			s.HotCount++
			s.HotBytes += item.SizeBytes
		case TierWarm:
			s.WarmCount++
			s.WarmBytes += item.SizeBytes
		case TierCold:
			s.ColdCount++
			s.ColdBytes += item.SizeBytes
		}
		s.TotalBytes += item.SizeBytes
	}
	return s
}

// atoi 解析正整数（失败返回 0）。
func atoi(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid digit %q", r)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

// isAllDigits 判断字符串是否全为数字。
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// HumanBytes 把字节数格式化为人类可读。
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	v := float64(n)
	for _, u := range units {
		v /= unit
		if v < unit {
			return strings.TrimSpace(fmt.Sprintf("%.2f %s", v, u))
		}
	}
	return fmt.Sprintf("%.2f PB", v/unit)
}
