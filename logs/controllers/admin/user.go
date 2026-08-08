package admin

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/session"

	"github.com/gin-gonic/gin"
)

// UserResp 用户响应体
type UserResp struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data User   `json:"data"`
}

// User 用户信息
type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// HealthResp 健康检查响应
type HealthResp struct {
	Status string `json:"status"`
	App    string `json:"app"`
	Time   string `json:"time"`
}

// Health 健康检查端点（K8s liveness probe）
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResp{
		Status: "ok",
		App:    "ydsz-trace-logs",
		Time:   time.Now().Format("2006-01-02 15:04:05"),
	})
}

// Ready 就绪检查端点（K8s readiness probe）
func Ready(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResp{
		Status: "ready",
		App:    "ydsz-trace-logs",
		Time:   time.Now().Format("2006-01-02 15:04:05"),
	})
}

// getAdminUser 从环境变量获取管理员用户名，降级到配置文件
func getAdminUser(cfg *config.Config) string {
	if v := os.Getenv("YDSZ_ADMIN_USER"); v != "" {
		return v
	}
	return cfg.StringOr("username", "admin")
}

// getAdminPassword 从环境变量获取管理员密码，降级到配置文件
func getAdminPassword(cfg *config.Config) string {
	if v := os.Getenv("YDSZ_ADMIN_PASSWORD"); v != "" {
		return v
	}
	return cfg.StringOr("password", "change_me_production")
}

// Console 控制台（前端 SPA 入口，直接返回提示，前端由 Vite 构建）
func Console(c *gin.Context) {
	c.String(http.StatusOK, "Ydsz Trace console. Please serve the web frontend (web/) separately.")
}

// Login 用户登陆接口
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

// Exit 退出登陆
func Exit(c *gin.Context) {
	session.Delete(c, "username")
	session.Destroy(c)
	c.JSON(http.StatusOK, UserResp{"200", "退出成功", User{}})
}
