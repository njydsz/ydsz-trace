// Package alert 单元测试：告警去重保护（FR-806）。
package alert

import (
	"sync"
	"testing"
	"time"
)

// TestDedupeWindow_Cooldown 验证冷却期内不应触发。
func TestDedupeWindow_Cooldown(t *testing.T) {
	d := &dedupeWindow{windows: make(map[int64]time.Time)}

	// 首次应触发
	if !d.ShouldFire(1, 300) {
		t.Error("首次调用 ShouldFire 应返回 true")
	}

	// 冷却期内不应触发
	if d.ShouldFire(1, 300) {
		t.Error("冷却期内重复调用 ShouldFire 应返回 false")
	}

	// 不同 ruleID 不受影响
	if !d.ShouldFire(2, 300) {
		t.Error("不同 ruleID 应独立计算冷却期")
	}
}

// TestDedupeWindow_Expire 验证冷却期过后再次触发。
func TestDedupeWindow_Expire(t *testing.T) {
	d := &dedupeWindow{windows: make(map[int64]time.Time)}

	// 设置 5 秒前的触发时间
	d.mu.Lock()
	d.windows[1] = time.Now().Add(-5 * time.Second)
	d.mu.Unlock()

	// 冷却期 3 秒，5 秒前触发过，应再次触发
	if !d.ShouldFire(1, 3) {
		t.Error("冷却期已过，应再次触发")
	}

	// 冷却期 10 秒，5 秒前触发过，不应触发
	d.mu.Lock()
	d.windows[2] = time.Now().Add(-5 * time.Second)
	d.mu.Unlock()
	if d.ShouldFire(2, 10) {
		t.Error("冷却期内，应抑制触发")
	}
}

// TestDedupeWindow_ManualSet 验证手动设置上次触发时间后冷却生效。
func TestDedupeWindow_ManualSet(t *testing.T) {
	d := &dedupeWindow{windows: make(map[int64]time.Time)}

	// 设置 10 秒前的触发时间
	d.mu.Lock()
	d.windows[1] = time.Now().Add(-10 * time.Second)
	d.mu.Unlock()

	// 冷却期 300 秒，10 秒前触发过，不应再次触发
	if d.ShouldFire(1, 300) {
		t.Error("距上次触发仅 10 秒（冷却期 300 秒），应抑制")
	}
}

// TestDedupeWindow_ParallelSafety 验证并发安全。
func TestDedupeWindow_ParallelSafety(t *testing.T) {
	d := &dedupeWindow{windows: make(map[int64]time.Time)}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			d.ShouldFire(id%10, 300)
		}(int64(i))
	}
	wg.Wait()

	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.windows) != 10 {
		t.Errorf("期望 10 个独立的 rule 窗口，实际 %d", len(d.windows))
	}
}
