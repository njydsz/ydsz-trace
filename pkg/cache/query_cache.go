// Package cache 热点查询结果缓存。
package cache

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// QueryCache LRU 缓存（最近最少使用淘汰）。
// key = hash(itemId + query params), value = cachedResult
type QueryCache struct {
	mu      sync.Mutex
	items   map[string]*list.Element // key -> list node
	order   *list.List               // LRU order (front = newest)
	maxSize int                      // 默认 200 条
	ttl     time.Duration            // 默认 30s

	hits   atomic.Int64
	misses atomic.Int64
}

// cachedResult 缓存项。
type cachedResult struct {
	key      string
	data     interface{}
	expireAt time.Time
	hits     atomic.Int64
}

// NewQueryCache 创建缓存实例。
func NewQueryCache(maxSize int, ttl time.Duration) *QueryCache {
	if maxSize <= 0 {
		maxSize = 200
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &QueryCache{
		items:   make(map[string]*list.Element),
		order:   list.New(),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get 从缓存获取结果（miss 返回 nil, false）。
func (c *QueryCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		c.misses.Add(1)
		return nil, false
	}

	cr := elem.Value.(*cachedResult)
	if time.Now().After(cr.expireAt) {
		c.moveToFrontLocked(elem)
		delete(c.items, key)
		c.order.Remove(elem)
		c.misses.Add(1)
		return nil, false
	}

	c.hits.Add(1)
	cr.hits.Add(1)
	c.order.MoveToFront(elem)
	return cr.data, true
}

// Set 写入缓存。
func (c *QueryCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		cr := elem.Value.(*cachedResult)
		cr.data = value
		cr.expireAt = time.Now().Add(c.ttl)
		return
	}

	evictCount := c.order.Len() - c.maxSize + 1
	for i := 0; i < evictCount; i++ {
		back := c.order.Back()
		if back == nil {
			break
		}
		cr := back.Value.(*cachedResult)
		delete(c.items, cr.key)
		c.order.Remove(back)
	}

	elem := c.order.PushFront(&cachedResult{
		key:      key,
		data:     value,
		expireAt: time.Now().Add(c.ttl),
	})
	c.items[key] = elem
}

// HitRate 返回缓存命中率（统计用）。
func (c *QueryCache) HitRate() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	hits := c.hits.Load()
	misses := c.misses.Load()
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

// InvalidateByItem 当 item 更新时，清除该 item 的全部缓存。
func (c *QueryCache) InvalidateByItem(itemID int64) {
	prefix := fmt.Sprintf("i%d_", itemID)

	c.mu.Lock()
	defer c.mu.Unlock()

	var keysToRemove []string
	for k := range c.items {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			keysToRemove = append(keysToRemove, k)
		}
	}
	for _, k := range keysToRemove {
		elem := c.items[k]
		c.order.Remove(elem)
		delete(c.items, k)
	}
}

// moveToFrontLocked 将元素移到 LRU 队列最前面（调用方需持有锁）。
func (c *QueryCache) moveToFrontLocked(elem *list.Element) {
	c.order.MoveToFront(elem)
}

// BuildCacheKey 根据查询参数构建缓存 key。
func BuildCacheKey(itemID int64, date, key, query string, regex bool) string {
	raw := fmt.Sprintf("i%d_d%s_k%s_q%s_r%v", itemID, date, key, query, regex)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
