// Package ratelimit 提供可插拔后端的登录速率限制器。
//
// 默认使用进程内存实现；多副本部署可替换为 Redis 后端，避免单 IP 限额被副本数放大。
// 规则：滑动窗口内 maxFail 次失败触发 blockFor 封禁，期间所有尝试被拒。
//
// 使用：
//
//	ratelimit.Default.Allow(ip)          // 是否允许
//	ratelimit.Default.RecordFailure(ip)  // 记录一次失败
//	ratelimit.Default.RecordSuccess(ip)  // 清除计数
//
// 在多副本场景，应在启动时通过 ratelimit.SetDefault(ratelimit.NewRedisLimiter(...))
// 将 Default 切换为 Redis 后端。
package ratelimit

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// DefaultWindow 统计窗口
	DefaultWindow = 60 * time.Second
	// DefaultMaxFail 窗口内最大失败次数
	DefaultMaxFail = 5
	// DefaultBlockFor 封禁时长
	DefaultBlockFor = 300 * time.Second
	// DefaultKeyPrefix Redis key 前缀
	DefaultKeyPrefix = "rt:"
)

// Limiter 速率限制器接口。
//
// Allow: 先于每个保护操作调用。返回 (是否允许, 剩余封禁秒数)。
// RecordFailure: 操作失败后调用。
// RecordSuccess: 操作成功后调用，清除该 key 的计数和封禁。
type Limiter interface {
	Allow(ip string) (allowed bool, remainSec int)
	RecordFailure(ip string)
	RecordSuccess(ip string)
}

// Default 全局默认速率限制器，可在启动时替换。
var Default Limiter = NewMemoryLimiter(DefaultWindow, DefaultMaxFail, DefaultBlockFor)

// SetDefault 替换全局默认速率限制器（通常在 main 中根据配置切换为 Redis 后端）。
func SetDefault(l Limiter) { Default = l }

// ========== 内存实现 ==========

// loginAttempt 单 IP 尝试记录。
type loginAttempt struct {
	count     int
	lastFail  time.Time
	blockedAt time.Time
}

// memoryLimiter 进程内存速率限制器。
type memoryLimiter struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt
	window   time.Duration
	maxFail  int
	blockFor time.Duration
}

// NewMemoryLimiter 创建进程内存限制器。
func NewMemoryLimiter(window time.Duration, maxFail int, blockFor time.Duration) Limiter {
	return &memoryLimiter{
		attempts: make(map[string]*loginAttempt),
		window:   window,
		maxFail:  maxFail,
		blockFor: blockFor,
	}
}

func (l *memoryLimiter) Allow(ip string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	attempt, ok := l.attempts[ip]
	if !ok {
		return true, 0
	}

	if !attempt.blockedAt.IsZero() {
		if elapsed := time.Since(attempt.blockedAt); elapsed < l.blockFor {
			return false, int((l.blockFor - elapsed).Seconds()) + 1
		}
		delete(l.attempts, ip)
		return true, 0
	}

	if time.Since(attempt.lastFail) > l.window {
		attempt.count = 0
		return true, 0
	}

	return attempt.count < l.maxFail, 0
}

func (l *memoryLimiter) RecordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	attempt, ok := l.attempts[ip]
	if !ok {
		l.attempts[ip] = &loginAttempt{count: 1, lastFail: time.Now()}
		return
	}
	attempt.count++
	attempt.lastFail = time.Now()
	if attempt.count >= l.maxFail {
		attempt.blockedAt = time.Now()
	}
}

func (l *memoryLimiter) RecordSuccess(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}

// ========== Redis 实现 ==========

// redisLimiter Redis 限速器，使用两个 key：
//   - cnt:<ip>：计数，TTL = window（每次失败重置）
//   - block:<ip>：封禁标记（含时间戳），TTL = blockFor
//
// 失败计数使用 INCR + EXPIRE（仅在首次写入时设 TTL）；达到阈值时写 block key。
// Redis 单命令原子，多副本共享状态无需额外事务。
type redisLimiter struct {
	client    *redis.Client
	keyPrefix string
	window    time.Duration
	maxFail   int
	blockFor  time.Duration
}

// NewRedisLimiter 创建 Redis 后端速率限制器。
//
// client 须非 nil；多个限制器实例可共享同一个 *redis.Client。
func NewRedisLimiter(client *redis.Client, window time.Duration, maxFail int, blockFor time.Duration) Limiter {
	return &redisLimiter{
		client:    client,
		keyPrefix: DefaultKeyPrefix,
		window:    window,
		maxFail:   maxFail,
		blockFor:  blockFor,
	}
}

func (l *redisLimiter) Allow(ip string) (bool, int) {
	ctx := context.Background()
	blockKey := l.key("block", ip)

	ttl, err := l.client.TTL(ctx, blockKey).Result()
	if err == nil && ttl > 0 {
		return false, int(ttl.Seconds()) + 1
	}

	cnt, err := l.client.Get(ctx, l.key("cnt", ip)).Int64()
	if err != nil || cnt == 0 {
		return true, 0
	}
	if int(cnt) >= l.maxFail {
		// 阈值已到但未设 block key（防守 window=blockFor 边界情况），补设
		_ = l.client.Set(ctx, blockKey, 1, l.blockFor).Err()
		return false, int(l.blockFor.Seconds())
	}
	return true, 0
}

func (l *redisLimiter) RecordFailure(ip string) {
	ctx := context.Background()
	cntKey := l.key("cnt", ip)
	newCnt, err := l.client.Incr(ctx, cntKey).Result()
	if err != nil {
		return
	}
	if newCnt == 1 {
		_ = l.client.Expire(ctx, cntKey, l.window).Err()
	}
	if int(newCnt) >= l.maxFail {
		_ = l.client.Set(ctx, l.key("block", ip), 1, l.blockFor).Err()
	}
}

func (l *redisLimiter) RecordSuccess(ip string) {
	ctx := context.Background()
	_, _ = l.client.Del(ctx, l.key("cnt", ip), l.key("block", ip)).Result()
}

func (l *redisLimiter) key(kind, ip string) string {
	return l.keyPrefix + kind + ":" + ip
}
