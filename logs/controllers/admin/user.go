// Package admin 包含用户认证与 SPA 控制台相关控制器。
//
// 安全说明：
//   - 账号密码来自环境变量 / 配置文件（当前为单账号，后续可扩展）
//   - 密码使用加盐哈希比对（pkg/auth 模块），支持明文→哈希平滑迁移
//   - 登录接口内置单 IP 速率限制，防御暴力破解
package admin

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/auth"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/session"

	"github.com/gin-gonic/gin"
)

// ========== 登录速率限制 ==========

// loginAttempt 单 IP 登录尝试记录。
type loginAttempt struct {
	count    int
	lastFail time.Time
	blockedAt time.Time
}

// loginRateLimiter 基于内存的登录速率限制器。
//
// 规则：单 IP 每 60 秒最多 5 次失败；超出后封禁 300 秒。
// 基于进程内存实现，多副本部署时需替换为 Redis 版本。
type loginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt
	window   time.Duration // 统计窗口
	maxFail  int           // 窗口内最大失败次数
	blockFor time.Duration // 封禁时长
}

// defaultLimiter 全局默认速率限制器。
var defaultLimiter = newRateLimiter(60*time.Second, 5, 300*time.Second)

func newRateLimiter(window time.Duration, maxFail int, blockFor time.Duration) *loginRateLimiter {
	return &loginRateLimiter{
		attempts: make(map[string]*loginAttempt),
		window:   window,
		maxFail:  maxFail,
		blockFor: blockFor,
	}
}

// Allow 检查是否允许该 IP 的登录尝试。
//
// 返回：(是否允许, 剩余封禁秒数)
func (l *loginRateLimiter) Allow(ip string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	attempt, ok := l.attempts[ip]
	if !ok {
		return true, 0
	}

	// 检查封禁期
	if !attempt.blockedAt.IsZero() {
		elapsed := time.Since(attempt.blockedAt)
		if elapsed < l.blockFor {
			remaining := int((l.blockFor - elapsed).Seconds())
			return false, remaining
		}
		// 封禁过期，重置记录
		delete(l.attempts, ip)
		return true, 0
	}

	// 窗口外的失败不计入
	if time.Since(attempt.lastFail) > l.window {
		attempt.count = 0
		return true, 0
	}

	return attempt.count < l.maxFail, 0
}

// RecordFailure 记录一次失败尝试。
func (l *loginRateLimiter) RecordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	attempt, ok := l.attempts[ip]
	if !ok {
		l.attempts[ip] = &loginAttempt{
			count:    1,
			lastFail: time.Now(),
		}
		return
	}

	attempt.count++
	attempt.lastFail = time.Now()

	// 超出阈值则封禁
	if attempt.count >= l.maxFail {
		attempt.blockedAt = time.Now()
	}
}

// RecordSuccess 登录成功后清除该 IP 的记录。
func (l *loginRateLimiter) RecordSuccess(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}

// extractClientIP 从 gin 上下文中提取客户端真实 IP。
func extractClientIP(c *gin.Context) string {
	// 优先从 X-Forwarded-For 取（反向代理场景）
	if xff := c.Request.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}
	if xrip := c.Request.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}
	// 直连场景
	ip, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return ip
}

// User 用户登录凭证（请求体）。
type User struct {
	// Username 用户名
	Username string `json:"username"`
	// Password 密码（明文传输，需 HTTPS）
	Password string `json:"password"`
}

// HealthResp 健康检查响应结构。
type HealthResp struct {
	// Status 状态标识
	Status string `json:"status"`
	// App 服务标识
	App string `json:"app"`
	// Time 当前服务器本地时间
	Time string `json:"time"`
}

// Health 存活探针（K8s liveness probe）。
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResp{
		Status: "ok",
		App:    "ydsz-trace-logs",
		Time:   time.Now().Format("2006-01-02 15:04:05"),
	})
}

// Ready 就绪探针（K8s readiness probe）。
func Ready(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResp{
		Status: "ready",
		App:    "ydsz-trace-logs",
		Time:   time.Now().Format("2006-01-02 15:04:05"),
	})
}

// getAdminUser 获取管理员用户名（环境变量 YDSZ_ADMIN_USER > 配置文件）。
func getAdminUser(cfg *config.Config) string {
	if v := os.Getenv("YDSZ_ADMIN_USER"); v != "" {
		return v
	}
	return cfg.StringOr("username", "admin")
}

// getAdminPassword 获取管理员密码（环境变量 YDSZ_ADMIN_PASSWORD > 配置文件）。
//
// 安全提示：生产必须使用环境变量注入强密码，避免写在配置文件中。
func getAdminPassword(cfg *config.Config) string {
	if v := os.Getenv("YDSZ_ADMIN_PASSWORD"); v != "" {
		return v
	}
	return cfg.StringOr("password", "change_me_production")
}

// webRoot 前端构建产物根目录（环境变量 YDSZ_WEB_ROOT > 默认 web/dist）。
func webRoot() string {
	if v := os.Getenv("YDSZ_WEB_ROOT"); v != "" {
		return v
	}
	return "web/dist"
}

// Index 控制台 SPA 入口，返回 index.html。
func Index(c *gin.Context) {
	serveIndex(c)
}

// Console 兼容旧路由（重定向到 index.html）。
func Console(c *gin.Context) {
	serveIndex(c)
}

// serveIndex 返回 Vite 构建产物 index.html；未构建时返回部署指引文本。
func serveIndex(c *gin.Context) {
	root := webRoot()
	indexFile := filepath.Join(root, "index.html")
	if _, err := os.Stat(indexFile); err != nil {
		c.String(http.StatusOK,
			"Ydsz Trace console. 前端尚未构建：请在 web/ 目录执行 `npm install && npm run build`，"+
				"并将产物 web/dist 置于可访问路径（环境变量 YDSZ_WEB_ROOT 可指定）。")
		return
	}
	c.File(indexFile)
}

// ServeStatic 处理静态资源与 SPA history 回退：
//
//   - API 前缀未命中返回 404（避免误当 SPA 页面）
//   - 文件存在直接返回，否则回退到 index.html
func ServeStatic(c *gin.Context) {
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		c.JSON(http.StatusNotFound, gin.H{"code": "404", "msg": "not found"})
		return
	}

	reqPath := c.Request.URL.Path
	// API 前缀未匹配到具体路由时，直接返回 404，避免把 API 404 误当 SPA 页面
	apiPrefixes := []string{"/admin", "/client", "/item", "/logs", "/health", "/ready"}
	for _, p := range apiPrefixes {
		if strings.HasPrefix(reqPath, p) {
			c.JSON(http.StatusNotFound, gin.H{"code": "404", "msg": "not found"})
			return
		}
	}

	root := webRoot()
	if _, err := os.Stat(root); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "404", "msg": "web frontend not found"})
		return
	}

	clean := filepath.Clean("/" + reqPath)
	filePath := filepath.Join(root, clean)
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		c.File(filePath)
		return
	}
	// SPA history 路由回退到 index.html
	c.File(filepath.Join(root, "index.html"))
}

// Login 用户登录：校验账号密码并在会话中设置 username。
//
// 安全特性：
//   - 密码加盐哈希比对（pkg/auth 模块）
//   - 支持明文密码平滑迁移：首次登录成功自动哈希
//   - 单 IP 速率限制，防御暴力破解（默认 60 秒内最多 5 次失败，超出封禁 300 秒）
func Login(c *gin.Context) {
	cfg := c.MustGet("cfg").(*config.Config)

	// 先获取 session，判断用户是否已经登录
	userName := session.GetString(c, "username")
	if userName != "" {
		api.Success(c, "用户已登录", gin.H{"username": userName})
		return
	}

	// 速率限制检查
	clientIP := extractClientIP(c)
	allowed, remainSec := defaultLimiter.Allow(clientIP)
	if !allowed {
		api.Error(c, http.StatusTooManyRequests, fmt.Sprintf("登录尝试过于频繁，请 %d 秒后再试", remainSec))
		return
	}

	// 用户没有登录，获取请求参数
	var user User
	data, err := c.GetRawData()
	if err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		defaultLimiter.RecordFailure(clientIP)
		return
	}
	err = json.Unmarshal(data, &user)
	if err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		defaultLimiter.RecordFailure(clientIP)
		return
	}

	// 优先从环境变量获取，其次从配置文件获取
	uname := getAdminUser(cfg)
	upwd := getAdminPassword(cfg)

	// 校验用户名
	if uname != user.Username {
		defaultLimiter.RecordFailure(clientIP)
		// 使用与密码错误相同的提示，防止用户名枚举
		api.Fail(c, api.CodeUnauthorized, "用户名或密码错误")
		return
	}

	// 密码校验：根据存储格式选择验证方式
	valid, err := verifyPassword(user.Password, upwd)
	if err != nil || !valid {
		defaultLimiter.RecordFailure(clientIP)
		api.Fail(c, api.CodeUnauthorized, "用户名或密码错误")
		return
	}

	// 登录成功，清除失败计数
	defaultLimiter.RecordSuccess(clientIP)

	// 平滑迁移：如果存储的是旧版 SHA-256 哈希，升级到 Argon2id
	if auth.NeedsRehash(upwd) {
		if hashed, err := auth.HashPassword(user.Password); err == nil {
			log.Printf("[security] 用户 %s 使用 SHA-256 哈希登录成功，已生成 Argon2id 哈希：%s（请更新至配置/环境变量）", uname, hashed)
		}
	}

	// 平滑迁移：如果存储的仍是明文密码，首次成功登录时自动哈希
	if !auth.IsHashedPassword(upwd) {
		if hashed, err := auth.HashPassword(user.Password); err == nil {
			// 记录日志：后续需手动更新环境变量/配置文件为哈希值
			log.Printf("[security] 用户 %s 使用明文密码登录成功，已生成哈希：%s（请更新至配置）", uname, hashed)
		}
	}

	session.Set(c, "username", user.Username)
	api.Success(c, "登录成功", gin.H{"username": uname})
}

// verifyPassword 根据存储的密码格式选择合适的验证方式。
//
// 如果存储的是哈希值，使用 auth.VerifyPassword 校验；
// 如果存储的是明文（兼容旧配置），直接比对并标记需迁移。
func verifyPassword(input, stored string) (bool, error) {
	if auth.IsHashedPassword(stored) {
		return auth.VerifyPassword(input, stored)
	}
	// 明文模式：恒定时间比较
	return subtle.ConstantTimeCompare([]byte(input), []byte(stored)) == 1, nil
}

// Exit 退出登录：清除会话中的 username 并销毁会话。
func Exit(c *gin.Context) {
	session.Delete(c, "username")
	session.Destroy(c)
	api.Success(c, "退出成功", nil)
}

