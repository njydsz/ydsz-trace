// Package session 提供可插拔后端的会话管理。
//
// 后端通过 Backend 接口抽象：进程内存（默认，零依赖）或 Redis（多副本共享）。
//
// 公开 API 保持不变：NewManager / Middleware / Get / GetString / Set / Delete / Destroy。
// 多副本共享会话时，请使用 NewRedisManager 并在反向代理后保持 Cookie 透传。
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	// cookieName 会话 token 的 Cookie 键名
	cookieName = "YDSZ_SESSION"
	// maxAge 默认会话有效期
	maxAge = 24 * time.Hour
	// contextKey gin context 中存储 Session 的键名
	contextKey = "session"
	// mgrKey gin context 中存储 Manager 的键名
	mgrKey = "session_manager"
)

// ========== Backend 接口 ==========

// Backend 会话存储后端。
//
// 实现须保证 Load / Save / Remove 幂等且并发安全（Manager 不再持有 sessions map）。
type Backend interface {
	// Load 读取 token 对应会话值；不存在或已过期返回 nil, nil。
	Load(ctx context.Context, token string) (map[string]interface{}, error)

	// Save 写入/覆盖会话数据并设置过期（ttlSeconds）。
	Save(ctx context.Context, token string, values map[string]interface{}, ttlSeconds int) error

	// Remove 删除会话数据；不存在不报错。
	Remove(ctx context.Context, token string) error

	// Close 关闭底层连接。
	Close() error
}

// ========== Memory Backend ==========

type memBackend struct {
	mu       sync.RWMutex
	sessions map[string]memEntry
}

type memEntry struct {
	values    map[string]interface{}
	expiresAt time.Time
}

// NewMemoryBackend 返回进程内存后端（默认，适合单副本或开发）。
func NewMemoryBackend() Backend {
	return &memBackend{sessions: make(map[string]memEntry)}
}

func (b *memBackend) Load(_ context.Context, token string) (map[string]interface{}, error) {
	b.mu.RLock()
	e, ok := b.sessions[token]
	b.mu.RUnlock()
	if !ok {
		return nil, nil
	}
	if time.Now().After(e.expiresAt) {
		b.mu.Lock()
		delete(b.sessions, token)
		b.mu.Unlock()
		return nil, nil
	}
	return copyValues(e.values), nil
}

func (b *memBackend) Save(_ context.Context, token string, values map[string]interface{}, ttlSeconds int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sessions[token] = memEntry{
		values:    copyValues(values),
		expiresAt: time.Now().Add(time.Duration(ttlSeconds) * time.Second),
	}
	return nil
}

func (b *memBackend) Remove(_ context.Context, token string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.sessions, token)
	return nil
}

func (b *memBackend) Close() error { return nil }

// ========== Redis Backend ==========

type redisBackend struct {
	client    *redis.Client
	keyPrefix string
}

// NewRedisBackend 创建 Redis 后端。addr 格式 "host:port"，空 password 表示无鉴权，db 默认 0。
func NewRedisBackend(addr, password string, db int) Backend {
	return &redisBackend{
		client: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       db,
		}),
		keyPrefix: "session:",
	}
}

func (b *redisBackend) key(token string) string { return b.keyPrefix + token }

func (b *redisBackend) Load(ctx context.Context, token string) (map[string]interface{}, error) {
	raw, err := b.client.Get(ctx, b.key(token)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (b *redisBackend) Save(ctx context.Context, token string, values map[string]interface{}, ttlSeconds int) error {
	if ttlSeconds <= 0 {
		ttlSeconds = int(maxAge.Seconds())
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return err
	}
	return b.client.Set(ctx, b.key(token), raw, time.Duration(ttlSeconds)*time.Second).Err()
}

func (b *redisBackend) Remove(ctx context.Context, token string) error {
	return b.client.Del(ctx, b.key(token)).Err()
}

func (b *redisBackend) Close() error { return b.client.Close() }

// ========== Session / Manager ==========

// Manager 会话管理器，通过 Backend 接口与存储后端解耦。
type Manager struct {
	mu       sync.RWMutex
	cookie   string
	maxAge   time.Duration
	secure   bool
	sameSite http.SameSite
	backend  Backend
}

// Session 单次请求内的会话值容器；token 唯一标识会话，values 为本次请求的快照。
//
// Set/Delete 修改 values 后立即调用后端 Save，保证多副本下下个请求能读到最新值。
type Session struct {
	mu     sync.RWMutex
	token  string
	values map[string]interface{}
	mgr    *Manager
}

// NewManager 创建默认进程内存后端的会话管理器。
//
// 默认：cookie=YDSZ_SESSION, 24h 过期, secure=true, sameSite=Lax。
func NewManager() *Manager {
	return &Manager{
		cookie:   cookieName,
		maxAge:   maxAge,
		secure:   true,
		sameSite: http.SameSiteLaxMode,
		backend:  NewMemoryBackend(),
	}
}

// NewRedisManager 创建 Redis 后端的多副本共享会话管理器。
//
// addr 为空时退化为进程内存后端，确保"零 Redis 依赖也能启动"。
func NewRedisManager(addr, password string, db int) *Manager {
	backend := NewMemoryBackend()
	if addr != "" {
		backend = NewRedisBackend(addr, password, db)
	}
	return &Manager{
		cookie:   cookieName,
		maxAge:   maxAge,
		secure:   true,
		sameSite: http.SameSiteLaxMode,
		backend:  backend,
	}
}

// SetInsecureDevMode 关闭 Cookie Secure 属性（仅用于开发/调试环境）。
func (m *Manager) SetInsecureDevMode() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secure = false
}

// SetSameSite 设置 Cookie SameSite 属性。
func (m *Manager) SetSameSite(mode http.SameSite) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sameSite = mode
}

// MaxAge 返回当前配置的最大有效期。
func (m *Manager) MaxAge() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.maxAge
}

// SetMaxAge 设置会话有效期（默认 24h）。生产建议 >= 2h 且 <= 7d。
func (m *Manager) SetMaxAge(d time.Duration) {
	if d <= 0 {
		d = maxAge
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxAge = d
}

// Close 关闭底层后端连接。
func (m *Manager) Close() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.backend != nil {
		return m.backend.Close()
	}
	return nil
}

// Middleware Gin 会话中间件。
//
// 流程：
//  1. 从 Cookie 读取 token
//  2. 通过 Backend.Load 查找会话值（未命中 / 过期时创建新 token + 设置 Cookie）
//  3. 将 *Session 注入 context
//  4. c.Next() 后不再批量 flush：Set/Delete 已即时 Save、Destroy 即时 Remove
func (m *Manager) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, values, created := m.resolveSession(c)
		s := &Session{
			token:  token,
			values: values,
			mgr:    m,
		}
		c.Set(contextKey, s)
		c.Set(mgrKey, m)
		if created {
			m.setSessionCookie(c, token, int(m.maxAge.Seconds()))
		}
		c.Next()
	}
}

// resolveSession 读取 token 并尝试从后端加载会话值；未命中时返回空白会话和 created=true。
func (m *Manager) resolveSession(c *gin.Context) (token string, values map[string]interface{}, created bool) {
	raw, err := c.Cookie(m.cookie)
	if err == nil && raw != "" {
		token = raw
		if v, err := m.backend.Load(c.Request.Context(), token); err == nil && v != nil {
			return token, v, false
		}
	}

	token = generateToken()
	return token, make(map[string]interface{}), true
}

// ========== 公开便捷函数 ==========

// Get 从 gin 上下文获取当前 Session；未命中返回 nil。
func Get(c *gin.Context) *Session {
	if v, ok := c.Get(contextKey); ok {
		if s, ok := v.(*Session); ok {
			return s
		}
	}
	return nil
}

// GetString 从会话中取 key 对应的字符串值；不存在或非字符串返回 ""。
func GetString(c *gin.Context, key string) string {
	s := Get(c)
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.values[key]; ok {
		if str, ok := v.(string); ok {
			return str
		}
	}
	return ""
}

// Set 写入键值到当前会话，并立即写入后端。
//
// 立即写入保证多副本共享时下个请求能读到当前值；单-key 会话不需要额外的请求末尾批量 flush。
func Set(c *gin.Context, key string, value interface{}) {
	s := Get(c)
	if s == nil {
		return
	}
	s.mu.Lock()
	s.values[key] = value
	vals := s.snapshotLocked()
	s.mu.Unlock()
	_ = s.mgr.backend.Save(context.Background(), s.token, vals, int(s.mgr.maxAge.Seconds()))
}

// Delete 移除会话中的 key 并立即同步到后端。
func Delete(c *gin.Context, key string) {
	s := Get(c)
	if s == nil {
		return
	}
	s.mu.Lock()
	previousLen := len(s.values)
	delete(s.values, key)
	if len(s.values) == previousLen {
		s.mu.Unlock()
		return
	}
	vals := s.snapshotLocked()
	s.mu.Unlock()
	_ = s.mgr.backend.Save(context.Background(), s.token, vals, int(s.mgr.maxAge.Seconds()))
}

// Destroy 销毁当前会话：从后端移除 key、清 Cookie；下个同 token 请求将生成全新会话。
func Destroy(c *gin.Context) {
	s := Get(c)
	mgr := GetManager(c)

	var token string
	if s != nil {
		s.mu.Lock()
		token = s.token
		s.mu.Unlock()
	}
	if token == "" {
		if v, err := c.Cookie(cookieName); err == nil {
			token = v
		}
	}
	if token != "" {
		backend := func() Backend {
			if mgr != nil {
				mgr.mu.RLock()
				defer mgr.mu.RUnlock()
				return mgr.backend
			}
			return nil
		}()
		if backend != nil {
			_ = backend.Remove(context.Background(), token)
		}
	}

	if mgr != nil {
		mgr.setSessionCookie(c, "", -1)
	} else {
		http.SetCookie(c.Writer, &http.Cookie{
			Name: cookieName, Value: "", Path: "/", MaxAge: -1,
			HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
		})
	}
}

// GetManager 从当前上下文取出 Manager；未命中返回 nil。
func GetManager(c *gin.Context) *Manager {
	if v, ok := c.Get(mgrKey); ok {
		if m, ok := v.(*Manager); ok {
			return m
		}
	}
	return nil
}

// ========== 内部工具 ==========

// generateToken 生成 32 字节随机 hex 字符串作为会话 token。
func generateToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(buf)
}

// snapshotLocked 在已持有写锁时拷贝当前 values。
func (s *Session) snapshotLocked() map[string]interface{} {
	dst := make(map[string]interface{}, len(s.values))
	for k, v := range s.values {
		dst[k] = v
	}
	return dst
}

// setSessionCookie 设置会话 Cookie（MaxAge > 0 设置；= -1 删除）。
func (m *Manager) setSessionCookie(c *gin.Context, value string, maxAge int) {
	m.mu.RLock()
	secure := m.secure
	sameSite := m.sameSite
	m.mu.RUnlock()
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     m.cookie,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})
}

// copyValues 深拷贝会话值 map，避免并发读写互相干扰。
func copyValues(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
