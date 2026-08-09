// Package routers 定义 logs 服务端的 HTTP 路由。
//
// 中间件链（按序）：
//
//	gin.Logger → gin.Recovery → 配置注入 → 会话中间件 → CORS → metrics → 鉴权中间件
//
// 路由分组：
//   - 公开：/, /health, /ready, /metrics, /admin/login, /admin/exit, /client/register
//   - 需鉴权：/client/*, /item/*, /logs/*
//   - SPA 回退：未命中 API 的 GET 请求回退到 index.html
package routers

import (
	"net/http"
	"os"
	"strings"
	"time"

	"ydsz-trace/logs/controllers/admin"
	"ydsz-trace/logs/controllers/client"
	"ydsz-trace/logs/controllers/item"
	"ydsz-trace/logs/controllers/alert"
	"ydsz-trace/logs/controllers/logs"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/metrics"
	"ydsz-trace/pkg/session"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// SetupRouter 构建 Gin 路由引擎，注入会话管理器。
func SetupRouter(cfg *config.Config, sessionMgr *session.Manager) *gin.Engine {
	// 非 dev 模式关闭 gin Debug 输出。
	runmode := cfg.StringOr("runmode", "dev")
	if runmode != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// 访问日志 + panic 恢复。
	r.Use(gin.Logger(), gin.Recovery())

	// 配置注入：下游 handler 通过 c.MustGet("cfg") 获取配置。
	r.Use(func(c *gin.Context) {
		c.Set("cfg", cfg)
		c.Next()
	})

	// 会话中间件：为每个请求加载/创建会话。
	r.Use(sessionMgr.Middleware())

	// CORS 白名单从 YDSZ_CORS_ORIGINS 环境变量读取。
	r.Use(cors.New(cors.Config{
		AllowOrigins:     getCORSOrigins(),
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type", "X-Token", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Prometheus 指标收集中间件（记录请求数/状态码/耗时）
	r.Use(metrics.HTTPMetricsMiddleware())

	// 健康检查与登录（无需鉴权）
	r.GET("/", admin.Index)
	r.GET("/health", admin.Health)
	r.GET("/ready", admin.Ready)
	r.GET("/metrics", metrics.Global().Handler())
	r.GET("/admin/login", admin.Login)
	r.POST("/admin/login", admin.Login)
	r.GET("/admin/exit", admin.Exit)
	r.POST("/admin/exit", admin.Exit)

	// 控制台（SPA 入口，由 Vite 构建的 web/dist 提供）
	r.GET("/admin/console", admin.Console)

	// logc 代理注册接口（代理调用，无 session，需放行）
	r.POST("/client/register", client.Register)

	// 鉴权分组：client / item / logs 需要登录
	auth := r.Group("")
	auth.Use(filterAuth)
	auth.Use(csrfProtection)
	{
		auth.POST("/client/add", client.Add)
		auth.POST("/client/delete", client.Delete)
		auth.POST("/client/update", client.Update)
		auth.POST("/client/changeStatus", client.ChangeClientStatus)
		auth.GET("/client/query", client.Query)
		auth.GET("/client/queryAll", client.QueryAll)
		auth.GET("/client/queryPage", client.QueryPage)

		auth.POST("/item/add", item.Add)
		auth.POST("/item/delete", item.Delete)
		auth.POST("/item/update", item.Update)
		auth.POST("/item/changeStatus", item.ChangeItemStatus)
		auth.GET("/item/query", item.Query)
		auth.GET("/item/queryAll", item.QueryAll)
		auth.GET("/item/queryPage", item.QueryPage)

		auth.POST("/logs/query", logs.Query)
		auth.POST("/logs/search", logs.Search)
		auth.GET("/logs/queryClients", logs.QueryClient)
		auth.GET("/logs/queryItems", logs.QueryItem)
		auth.POST("/logs/stream", logs.Stream)
		auth.POST("/logs/tail", logs.Tail)

		// 检索任务持久化：列表 / 详情 / 重试 / 删除
		auth.GET("/logs/tasks/:taskNo", logs.QueryTask)
		auth.GET("/logs/tasks", logs.ListTasks)
		auth.POST("/logs/tasks/:taskNo/retry", logs.RetryTask)
		auth.DELETE("/logs/tasks/:taskNo", logs.DeleteTask)

		// 告警 webhook
		auth.POST("/logs/alerts/rules", alert.AddRule)
		auth.GET("/logs/alerts/rules", alert.ListRules)
		auth.GET("/logs/alerts/rules/:id", alert.GetRule)
		auth.PUT("/logs/alerts/rules/:id", alert.UpdateRule)
		auth.DELETE("/logs/alerts/rules/:id", alert.DeleteRule)
		auth.POST("/logs/alerts/rules/toggle", alert.ToggleRule)
		auth.POST("/logs/alerts/rules/test", alert.TestFire)
		auth.GET("/logs/alerts/events", alert.ListEvents)
		auth.DELETE("/logs/alerts/events/:id", alert.DeleteEvent)
		auth.GET("/logs/alerts/quota", alert.Quota)
	}

	// SPA 回退：未命中 API 的 GET/HEAD 请求回退到 index.html。
	r.NoRoute(admin.ServeStatic)

	return r
}

// getCORSOrigins 从 YDSZ_CORS_ORIGINS 环境变量解析 CORS 白名单。
func getCORSOrigins() []string {
	origins := os.Getenv("YDSZ_CORS_ORIGINS")
	if origins == "" {
		return []string{"http://localhost:*", "http://127.0.0.1:*"}
	}
	return strings.Split(origins, ",")
}

// csrfProtection 防护中间件：拒绝缺少 X-Requested-With 头的非安全方法请求。
// 浏览器 CORS 策略禁止跨域页面发送自定义头，因此第三方站点无法伪造 POST/DELETE。
func csrfProtection(c *gin.Context) {
	method := c.Request.Method
	if method == "GET" || method == "HEAD" || method == "OPTIONS" {
		c.Next()
		return
	}
	if c.GetHeader("X-Requested-With") == "" {
		c.JSON(http.StatusForbidden, map[string]interface{}{
			"code": "403",
			"msg":  "CSRF 校验失败：缺少 X-Requested-With 请求头",
			"data": nil,
		})
		c.Abort()
		return
	}
	c.Next()
}

// filterAuth 鉴权中间件：未登录返回 401 并 abort。
func filterAuth(c *gin.Context) {
	username := session.GetString(c, "username")
	if username == "" {
		c.JSON(http.StatusUnauthorized, map[string]interface{}{
			"code": "401",
			"msg":  "未登录或登录已过期",
			"data": nil,
		})
		c.Abort()
		return
	}
	c.Next()
}
