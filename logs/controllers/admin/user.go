package admin

import (
	"net/http"
	"time"

	"ydsz-trace/pkg/config"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// SessionKey 登录 session 键
const SessionKey = "username"

// User 用户信息
type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login 用户登录接口
func Login(c *gin.Context) {
	cfg := c.MustGet("cfg").(*config.Config)

	// 已登录则直接返回
	session := sessions.Default(c)
	if uname := session.Get(SessionKey); uname != nil {
		if s, ok := uname.(string); ok && s != "" {
			c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "用户已经登陆", "data": gin.H{"username": s}})
			return
		}
	}

	var user User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "400", "msg": "请求参数错误", "data": nil})
		return
	}

	// 优先从环境变量获取，其次从配置文件获取
	uname := config.EnvOrConfig("YDSZ_ADMIN_USER", cfg.String("username"), "admin")
	upwd := config.EnvOrConfig("YDSZ_ADMIN_PASSWORD", cfg.String("password"), "")

	if uname == user.Username && upwd == user.Password {
		session.Set(SessionKey, user.Username)
		if err := session.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "Session保存失败", "data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "登陆成功", "data": gin.H{"username": uname}})
		return
	}

	c.JSON(http.StatusUnauthorized, gin.H{"code": "401", "msg": "用户名或密码错误", "data": nil})
}

// Exit 退出登录
func Exit(c *gin.Context) {
	session := sessions.Default(c)
	session.Delete(SessionKey)
	session.Clear()
	_ = session.Save()
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "退出成功", "data": nil})
}

// Health 健康检查端点（K8s liveness probe）
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"app":    "ydsz-trace-logs",
		"time":   time.Now().Format("2006-01-02 15:04:05"),
	})
}

// Ready 就绪检查端点（K8s readiness probe）
func Ready(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
		"app":    "ydsz-trace-logs",
		"time":   time.Now().Format("2006-01-02 15:04:05"),
	})
}

// Test 测试路由
func Test(c *gin.Context) {
	c.String(http.StatusOK, "Ydsz Trace logs test endpoint")
}
