// Package cache 单测：LRU 命中率、淘汰、失效。
package cache

import (
	"testing"
	"time"
)

func TestLRU_Evict(t *testing.T) {
	c := NewQueryCache(3, 10*time.Second)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)
	c.Set("d", 4)

	if _, ok := c.Get("a"); ok {
		t.Fatal("期望 a 被淘汰，但仍然命中")
	}
	if v, ok := c.Get("d"); !ok || v != 4 {
		t.Fatal("期望 d 命中")
	}
}

func TestLRU_HitRate(t *testing.T) {
	c := NewQueryCache(10, 10*time.Second)
	c.Set("x", 1)
	c.Get("x")  // hit
	c.Get("y")  // miss

	rate := c.HitRate()
	if rate < 0.4 || rate > 0.6 {
		t.Fatalf("命中率应约 0.5，实际 %.2f", rate)
	}
}

func TestLRU_TTLExpire(t *testing.T) {
	c := NewQueryCache(10, 50*time.Millisecond)
	c.Set("a", 1)
	time.Sleep(100 * time.Millisecond)
	if _, ok := c.Get("a"); ok {
		t.Fatal("期望 a 因 TTL 过期被淘汰")
	}
}

func TestLRU_InvalidateByItem(t *testing.T) {
	c := NewQueryCache(100, 10*time.Second)
	c.Set("i1_d20240101_aaa", "result1")
	c.Set("i1_d20240101_bbb", "result2")
	c.Set("i2_d20240101_ccc", "result3")

	c.InvalidateByItem(1)

	if _, ok := c.Get("i1_d20240101_aaa"); ok {
		t.Fatal("i1 缓存应被失效")
	}
	if _, ok := c.Get("i1_d20240101_bbb"); ok {
		t.Fatal("i1 缓存应被失效")
	}
	if _, ok := c.Get("i2_d20240101_ccc"); !ok {
		t.Fatal("i2 缓存不应被失效")
	}
}

func TestBuildCacheKey(t *testing.T) {
	k1 := BuildCacheKey(1, "20240101", "error", "", false)
	k2 := BuildCacheKey(1, "20240101", "error", "", false)
	if k1 != k2 {
		t.Fatal("相同参数应生成相同 key")
	}
	k3 := BuildCacheKey(1, "20240102", "error", "", false)
	if k1 == k3 {
		t.Fatal("不同 date 应生成不同 key")
	}
}
