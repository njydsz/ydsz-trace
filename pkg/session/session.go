// Package session 提供轻量内存会话管理，替代 beego session。
//
// 设计要点：
//   - 会话数据保存在进程内存 map 中，重启后丢失（适合单副本或短会话）
//   - token 通过 HttpOnly Cookie（YDSZ_SESSION）传递，24 小时过期
//   - 通过 Gin 中间件为每个请求注入 *Session，支持 Get/Set/Delete/Destroy
//   - 并发安全：Manager 和 Session 各自使用 RWMutex
//
// 注意：多副本部署时同一用户的请求若路由到不同实例会导致会话丢失；
//       如需跨副本共享，请替换为 Redis 等外部存储后端。
package session

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
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

// Manager 会话管理器，维护 token -> Session 的映射。
//
// 并发安全：通过 sessionsMu 保护 map 读写。
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	cookie   string
	maxAge   time.Duration
}

// Session 单个用户的会话数据。
type Session struct {
	mu        sync.RWMutex
	values    map[string]interface{}
	expiresAt time.Time
}

// NewManager 创建默认配置的会话管理器（cookie=YDSZ_SESSION，有效期 24h）。
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		cookie:   cookieName,
		maxAge:   maxAge,
	}
}

// Middleware 返回 Gin 中间件，为每个请求绑定会话。
//
// 流程：
//   1. 从 Cookie 读取 token
//   2. token 有效且未过期则复用，否则创建新会话并设置 Cookie
//   3. 将 *Session 与 *Manager 注入 context 供 handler 使用
func (m *Manager) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := ""
		if cookie, err := c.Cookie(m.cookie); err == nil {
			token = cookie
		}

		var s *Session
		m.mu.Lock()
		if token != "" {
			if existing, ok := m.sessions[token]; ok {
				if time.Now().Before(existing.expiresAt) {
					s = existing
				} else {
					delete(m.sessions, token)
				}
			}
		}
		if s == nil {
			// 创建新会话
			newToken := generateToken()
			s = &Session{
				values:    make(map[string]interface{}),
				expiresAt: time.Now().Add(m.maxAge),
			}
			m.sessions[newToken] = s
			token = newToken
			http.SetCookie(c.Writer, &http.Cookie{
				Name:     m.cookie,
				Value:    newToken,
				Path:     "/",
				MaxAge:   int(m.maxAge.Seconds()),
				HttpOnly: true,
			})
		}
		m.mu.Unlock()

		c.Set(contextKey, s)
		c.Set(mgrKey, m)
		c.Next()
	}
}

// Get 从 gin 上下文中获取当前 Session，不存在返回 nil。
func Get(c *gin.Context) *Session {
	if v, ok := c.Get(contextKey); ok {
		if s, ok := v.(*Session); ok {
			return s
		}
	}
	return nil
}

// GetString 从会话中取 key 对应的字符串值，不存在或非字符串返回 ""。
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

// Set 写入键值到当前会话（并发安全）。
func Set(c *gin.Context, key string, value interface{}) {
	s := Get(c)
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
}

// Delete 从会话中移除指定 key。
func Delete(c *gin.Context, key string) {
	s := Get(c)
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
}

// Destroy 销毁当前会话：从 Manager 移除会话并清除客户端 Cookie。
func Destroy(c *gin.Context) {
	token, err := c.Cookie(cookieName)
	if err == nil {
		if v, ok := c.Get(mgrKey); ok {
			if m, ok := v.(*Manager); ok {
				m.mu.Lock()
				delete(m.sessions, token)
				m.mu.Unlock()
			}
		}
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

// generateToken 生成 32 字节随机 hex 字符串作为会话 token。
//
// crypto/rand 不可用时退回到时间戳 token（仅作兜底）。
func generateToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(buf)
}
