// Package session 轻量内存会话管理，替代 beego session。
// 使用随机 token + 内存 map 存储，支持 Get/Set/Delete。
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
	cookieName = "YDSZ_SESSION"
	maxAge     = 24 * time.Hour
	contextKey = "session"
	mgrKey     = "session_manager"
)

// Manager 会话管理器
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	cookie   string
	maxAge   time.Duration
}

// Session 单个会话
type Session struct {
	mu        sync.RWMutex
	values    map[string]interface{}
	expiresAt time.Time
}

// NewManager 创建会话管理器
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		cookie:   cookieName,
		maxAge:   maxAge,
	}
}

// Middleware Gin 中间件：为每个请求绑定会话
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

// Get 从 context 获取会话
func Get(c *gin.Context) *Session {
	if v, ok := c.Get(contextKey); ok {
		if s, ok := v.(*Session); ok {
			return s
		}
	}
	return nil
}

// GetString 获取会话字符串值
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

// Set 设置会话值
func Set(c *gin.Context, key string, value interface{}) {
	s := Get(c)
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
}

// Delete 删除会话中的键
func Delete(c *gin.Context, key string) {
	s := Get(c)
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
}

// Destroy 销毁整个会话（从 context 取 manager）
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

// generateToken 生成随机 token
func generateToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(buf)
}
