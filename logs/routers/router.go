package routers

import (
	"net/http"
	"os"
	"strings"
	"time"

	"ydsz-trace/logs/controllers/admin"
	"ydsz-trace/logs/controllers/client"
	"ydsz-trace/logs/controllers/item"
	"ydsz-trace/logs/controllers/logs"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/session"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// SetupRouter 构建 Gin 路由
func SetupRouter(cfg *config.Config, sessionMgr *session.Manager) *gin.Engine {
	// 根据运行模式设置 gin 模式
	runmode := cfg.StringOr("runmode", "dev")
	if runmode != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// 注入配置到 context
	r.Use(func(c *gin.Context) {
		c.Set("cfg", cfg)
		c.Next()
	})

	// 会话中间件
	r.Use(sessionMgr.Middleware())

	// CORS 白名单：从环境变量 YDSZ_CORS_ORIGINS 读取，逗号分隔
	r.Use(cors.New(cors.Config{
		AllowOrigins:     getCORSOrigins(),
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type", "X-Token", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// 健康检查与登录（无需鉴权）
	r.GET("/", admin.Login)
	r.GET("/health", admin.Health)
	r.GET("/ready", admin.Ready)
	r.GET("/admin/login", admin.Login)
	r.POST("/admin/login", admin.Login)
	r.GET("/admin/exit", admin.Exit)
	r.POST("/admin/exit", admin.Exit)

	// 控制台（SPA 入口，无需鉴权即可返回提示）
	r.GET("/admin/console", admin.Console)

	// logc 代理注册接口（代理调用，无 session，需放行）
	r.POST("/client/register", client.Register)

	// 鉴权分组：client / item / logs 需要登录
	auth := r.Group("")
	auth.Use(filterAuth)
	{
		auth.POST("/client/add", client.Add)
		auth.GET("/client/delete", client.Delete)
		auth.POST("/client/update", client.Update)
		auth.GET("/client/changeStatus", client.ChangeClientStatus)
		auth.GET("/client/query", client.Query)
		auth.GET("/client/queryAll", client.QueryAll)
		auth.GET("/client/queryPage", client.QueryPage)

		auth.POST("/item/add", item.Add)
		auth.GET("/item/delete", item.Delete)
		auth.POST("/item/update", item.Update)
		auth.GET("/item/changeStatus", item.ChangeItemStatus)
		auth.GET("/item/query", item.Query)
		auth.GET("/item/queryAll", item.QueryAll)
		auth.GET("/item/queryPage", item.QueryPage)

		auth.POST("/logs/query", logs.Query)
		auth.GET("/logs/queryClients", logs.QueryClient)
		auth.GET("/logs/queryItems", logs.QueryItem)
	}

	return r
}

// getCORSOrigins 从环境变量读取 CORS 白名单
func getCORSOrigins() []string {
	origins := os.Getenv("YDSZ_CORS_ORIGINS")
	if origins == "" {
		return []string{"http://localhost:*", "http://127.0.0.1:*"}
	}
	return strings.Split(origins, ",")
}

// filterAuth 鉴权中间件：检查用户是否已登录
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
