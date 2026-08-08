// Package admin 包含用户认证与 SPA 控制台相关控制器。
//
// 安全说明：
//	- 账号密码来自环境变量 / 配置文件（当前为单账号，后续可扩展）
//	- 密码明文比对，生产建议改为加盐哈希
package admin

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/session"

	"github.com/gin-gonic/gin"
)

// UserResp 用户相关接口响应体。
type UserResp struct {
	// Code 业务状态码，"200" 表示成功
	Code string `json:"code"`
	// Msg 提示信息
	Msg string `json:"msg"`
	// Data 用户数据负载
	Data User `json:"data"`
}

// User 用户登录凭证。
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
// 安全提示：密码明文比对，生产环境建议改为 bcrypt/argon2 加盐哈希。
func Login(c *gin.Context) {
	cfg := c.MustGet("cfg").(*config.Config)

	// 先获取 session，判断用户是否已经登录
	userName := session.GetString(c, "username")
	if userName != "" {
		c.JSON(http.StatusOK, UserResp{"200", "用户已经登陆", User{userName, ""}})
		return
	}

	// 用户没有登录，获取请求参数
	var user User
	data, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusOK, UserResp{"400", "请求参数错误", User{}})
		return
	}
	err = json.Unmarshal(data, &user)
	if err != nil {
		c.JSON(http.StatusOK, UserResp{"400", "请求参数错误", User{}})
		return
	}

	// 优先从环境变量获取，其次从配置文件获取
	uname := getAdminUser(cfg)
	upwd := getAdminPassword(cfg)

	// 判断用户名、密码是否正确
	if uname == user.Username && upwd == user.Password {
		session.Set(c, "username", user.Username)
		c.JSON(http.StatusOK, UserResp{"200", "登陆成功", User{uname, ""}})
	} else {
		c.JSON(http.StatusOK, UserResp{"401", "用户名或密码错误", User{}})
	}
}

// Exit 退出登录：清除会话中的 username 并销毁会话。
func Exit(c *gin.Context) {
	session.Delete(c, "username")
	session.Destroy(c)
	c.JSON(http.StatusOK, UserResp{"200", "退出成功", User{}})
}
