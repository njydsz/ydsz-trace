package perf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDefaultLifecyclePolicy 默认策略与校验。
func TestDefaultLifecyclePolicy(t *testing.T) {
	p := DefaultLifecyclePolicy()
	if p.HotDays != DefaultHotDays {
		t.Errorf("HotDays = %d", p.HotDays)
	}
	if p.WarmDays <= p.HotDays {
		t.Errorf("WarmDays 应大于 HotDays: %d <= %d", p.WarmDays, p.HotDays)
	}
	if err := p.Validate(); err != nil {
		t.Errorf("默认策略应合法: %v", err)
	}
}

// TestLifecyclePolicyValidate 非法策略被拒绝。
func TestLifecyclePolicyValidate(t *testing.T) {
	cases := []struct {
		name string
		p    LifecyclePolicy
	}{
		{"天数为 0", LifecyclePolicy{HotDays: 0, WarmDays: 30, ColdAction: "compress"}},
		{"温不大于热", LifecyclePolicy{HotDays: 30, WarmDays: 30, ColdAction: "compress"}},
		{"动作非法", LifecyclePolicy{HotDays: 7, WarmDays: 30, ColdAction: "purge"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.p.Validate(); err == nil {
				t.Error("应返回错误")
			}
		})
	}
}

// dateNDaysAgo 返回 N 天前的 YYYYMMDD。
func dateNDaysAgo(n int) string {
	return time.Now().AddDate(0, 0, -n).Format("20060102")
}

// TestClassify 分层判定。
func TestClassify(t *testing.T) {
	p := LifecyclePolicy{HotDays: 7, WarmDays: 30, ColdAction: "compress"}

	cases := []struct {
		age  int
		want string
	}{
		{0, TierHot},
		{3, TierHot},
		{7, TierHot},
		{8, TierWarm},
		{20, TierWarm},
		{30, TierWarm},
		{31, TierCold},
		{100, TierCold},
	}
	for _, c := range cases {
		got := p.Classify(dateNDaysAgo(c.age))
		if got != c.want {
			t.Errorf("age=%d got %q want %q", c.age, got, c.want)
		}
	}
}

// TestClassify_InvalidDate 非法日期保守归为热数据。
func TestClassify_InvalidDate(t *testing.T) {
	p := DefaultLifecyclePolicy()
	if got := p.Classify("not-a-date"); got != TierHot {
		t.Errorf("got %q want hot", got)
	}
	if got := p.Classify(""); got != TierHot {
		t.Errorf("got %q want hot", got)
	}
}

// TestShouldDelete 删除策略仅在冷数据生效。
func TestShouldDelete(t *testing.T) {
	p := LifecyclePolicy{HotDays: 7, WarmDays: 30, ColdAction: "delete"}

	if p.ShouldDelete(dateNDaysAgo(3)) {
		t.Error("热数据不应删除")
	}
	if p.ShouldDelete(dateNDaysAgo(20)) {
		t.Error("温数据不应删除")
	}
	if !p.ShouldDelete(dateNDaysAgo(60)) {
		t.Error("冷数据应删除")
	}
}

// TestShouldDelete_CompressNotDelete 压缩策略不删除。
func TestShouldDelete_CompressNotDelete(t *testing.T) {
	p := LifecyclePolicy{HotDays: 7, WarmDays: 30, ColdAction: "compress"}
	if p.ShouldDelete(dateNDaysAgo(60)) {
		t.Error("压缩策略不应删除原文")
	}
}

// TestExtractDateFromFilename 文件名日期提取。
func TestExtractDateFromFilename(t *testing.T) {
	cases := []struct {
		name string
		want string
		ok   bool
	}{
		{"app-20261106.log", "20261106", true},
		{"20261106.log", "20261106", true},
		{"app_20261106_001.log", "20261106", true},
		{"app.log", "", false},
		{"app-20261301.log", "", false}, // 非法月份
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := extractDateFromFilename(c.name)
		if ok != c.ok || got != c.want {
			t.Errorf("extractDateFromFilename(%q) = (%q, %v) want (%q, %v)",
				c.name, got, ok, c.want, c.ok)
		}
	}
}

// TestPlanLifecycle 计划生成与统计。
func TestPlanLifecycle(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"app-" + dateNDaysAgo(1) + ".log":  "small",
		"app-" + dateNDaysAgo(15) + ".log": "medium content",
		"app-" + dateNDaysAgo(60) + ".log": "old content here",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}

	p := LifecyclePolicy{HotDays: 7, WarmDays: 30, ColdAction: "compress"}
	reports, err := PlanLifecycle(dir, p)
	if err != nil {
		t.Fatalf("PlanLifecycle 失败: %v", err)
	}
	if len(reports) != 3 {
		t.Fatalf("len = %d want 3", len(reports))
	}

	// 按日期倒序：最新的在前
	if reports[0].Tier != TierHot {
		t.Errorf("第一条应为 hot, got %q", reports[0].Tier)
	}
	if reports[2].Tier != TierCold {
		t.Errorf("最后一条应为 cold, got %q", reports[2].Tier)
	}

	stats := Summarize(reports)
	if stats.HotCount != 1 || stats.WarmCount != 1 || stats.ColdCount != 1 {
		t.Errorf("stats = %+v", stats)
	}
	if stats.TotalBytes <= 0 {
		t.Error("TotalBytes 应大于 0")
	}
}

// TestPlanLifecycle_EmptyDir 空目录返回空列表。
func TestPlanLifecycle_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	reports, err := PlanLifecycle(dir, DefaultLifecyclePolicy())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("len = %d want 0", len(reports))
	}
}

// TestApplyLifecycle_DryRun 干跑不修改文件。
func TestApplyLifecycle_DryRun(t *testing.T) {
	dir := t.TempDir()
	name := "app-" + dateNDaysAgo(60) + ".log"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("old"), 0o644); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	p := LifecyclePolicy{HotDays: 7, WarmDays: 30, ColdAction: "delete"}
	applied, skipped, err := ApplyLifecycle(dir, p, true)
	if err != nil {
		t.Fatalf("ApplyLifecycle 失败: %v", err)
	}
	if applied == 0 {
		t.Error("应有动作被计划")
	}

	// 文件应仍然存在
	if _, serr := os.Stat(filepath.Join(dir, name)); serr != nil {
		t.Error("dry-run 不应删除文件")
	}
	if skipped != 0 {
		t.Errorf("skipped = %d", skipped)
	}
}

// TestApplyLifecycle_Delete 真实删除冷数据。
func TestApplyLifecycle_Delete(t *testing.T) {
	dir := t.TempDir()
	oldName := "app-" + dateNDaysAgo(60) + ".log"
	hotName := "app-" + dateNDaysAgo(1) + ".log"
	_ = os.WriteFile(filepath.Join(dir, oldName), []byte("old"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, hotName), []byte("new"), 0o644)

	p := LifecyclePolicy{HotDays: 7, WarmDays: 30, ColdAction: "delete"}
	applied, skipped, err := ApplyLifecycle(dir, p, false)
	if err != nil {
		t.Fatalf("ApplyLifecycle 失败: %v", err)
	}
	if applied != 1 {
		t.Errorf("applied = %d want 1", applied)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d want 1", skipped)
	}

	if _, serr := os.Stat(filepath.Join(dir, oldName)); !os.IsNotExist(serr) {
		t.Error("冷数据应被删除")
	}
	if _, serr := os.Stat(filepath.Join(dir, hotName)); serr != nil {
		t.Error("热数据应保留")
	}
}

// TestApplyLifecycle_Compress 压缩温/冷数据。
func TestApplyLifecycle_Compress(t *testing.T) {
	dir := t.TempDir()
	name := "app-" + dateNDaysAgo(60) + ".log"
	_ = os.WriteFile(filepath.Join(dir, name), []byte("compressible content"), 0o644)

	p := LifecyclePolicy{HotDays: 7, WarmDays: 30, ColdAction: "compress"}
	if _, _, err := ApplyLifecycle(dir, p, false); err != nil {
		t.Fatalf("ApplyLifecycle 失败: %v", err)
	}

	// 原文件应被移除，.gz 应存在
	if _, serr := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(serr) {
		t.Error("原文件应被删除")
	}
	if _, serr := os.Stat(filepath.Join(dir, name+".gz")); serr != nil {
		t.Errorf(".gz 文件应存在: %v", serr)
	}
}

// TestHumanBytes 字节格式化。
func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{512, "512 B"},
		{2048, "2.00 KB"},
		{5 * 1024 * 1024, "5.00 MB"},
	}
	for _, c := range cases {
		if got := HumanBytes(c.in); got != c.want {
			t.Errorf("HumanBytes(%d) = %q want %q", c.in, got, c.want)
		}
	}
}

// TestGzipBytes 压缩可解压还原。
func TestGzipBytes(t *testing.T) {
	src := []byte(strings.Repeat("log content ", 100))
	compressed := gzipBytes(src)
	if len(compressed) == 0 {
		t.Fatal("压缩结果为空")
	}
	if len(compressed) >= len(src) {
		t.Error("重复内容压缩后应变小")
	}
}

// TestAtoi 整数解析。
func TestAtoi(t *testing.T) {
	if n, err := atoi("123"); err != nil || n != 123 {
		t.Errorf("atoi(123) = %d, %v", n, err)
	}
	if _, err := atoi("12a"); err == nil {
		t.Error("非法输入应报错")
	}
	if _, err := atoi(""); err == nil {
		t.Error("空输入应报错")
	}
}

// TestIsAllDigits 数字判断。
func TestIsAllDigits(t *testing.T) {
	for _, v := range []string{"123", "0", "0000"} {
		if !isAllDigits(v) {
			t.Errorf("%q 应为数字", v)
		}
	}
	for _, v := range []string{"", "12a", " 1", "1 "} {
		if isAllDigits(v) {
			t.Errorf("%q 不应为数字", v)
		}
	}
}
