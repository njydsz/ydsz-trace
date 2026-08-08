package routers

import (
	"os"
	"strings"
	"time"

	"ydsz-trace/logs/controllers/admin"
	"ydsz-trace/logs/controllers/client"
	"ydsz-trace/logs/controllers/item"
	logsctl "ydsz-trace/logs/controllers/logs"
	"ydsz-trace/pkg/config"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

// SessionSecret session 加密密钥（生产环境应通过环境变量 YDSZ_SESSION_SECRET 覆盖）
var sessionSecret = "ydsz-trace-session-secret"

// SetupRouter 构建 Gin 路由
func SetupRouter(cfg *config.Config) *gin.Engine {
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

	// Session 中间件（cookie 存储）
	if secret := os.Getenv("YDSZ_SESSION_SECRET"); secret != "" {
		sessionSecret = secret
	}
	store := cookie.NewStore([]byte(sessionSecret))
	store.Options(sessions.Options{Path: "/", MaxAge: 86400 * 7, HttpOnly: true})
	r.Use(sessions.Sessions("ydsz_trace_session", store))

	// CORS 白名单：从环境变量 YDSZ_CORS_ORIGINS 读取，逗号分隔
	r.Use(cors.New(cors.Config{
		AllowOrigins:     getCORSOrigins(),
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type", "X-Token", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// 公开路由
	r.GET("/", admin.Login)
	r.GET("/health", admin.Health)
	r.GET("/ready", admin.Ready)
	r.POST("/admin/login", admin.Login)
	r.GET("/admin/exit", admin.Exit)
	// 客户端注册（logc 代理调用，免鉴权）
	r.POST("/client/register", client.Register)

	// 需要鉴权的路由组
	authGroup := r.Group("/")
	authGroup.Use(authMiddleware())
	{
		authGroup.GET("/admin/test", admin.Test)
		authGroup.POST("/client/add", client.Add)
		authGroup.POST("/client/delete", client.Delete)
		authGroup.POST("/client/update", client.Update)
		authGroup.POST("/client/changeStatus", client.ChangeClientStatus)
		authGroup.GET("/client/query", client.Query)
		authGroup.GET("/client/queryAll", client.QueryAll)
		authGroup.GET("/client/queryPage", client.QueryPage)

		authGroup.POST("/item/add", item.Add)
		authGroup.POST("/item/delete", item.Delete)
		authGroup.POST("/item/update", item.Update)
		authGroup.POST("/item/changeStatus", item.ChangeItemStatus)
		authGroup.GET("/item/query", item.Query)
		authGroup.GET("/item/queryAll", item.QueryAll)
		authGroup.GET("/item/queryPage", item.QueryPage)

		authGroup.POST("/logs/query", logsctl.Query)
		authGroup.GET("/logs/queryClients", logsctl.QueryClient)
		authGroup.GET("/logs/queryItems", logsctl.QueryItem)
	}

	return r
}

// getCORSOrigins 从环境变量读取 CORS 白名单，逗号分隔
func getCORSOrigins() []string {
	origins := os.Getenv("YDSZ_CORS_ORIGINS")
	if origins == "" {
		// 默认：仅允许本地开发
		return []string{"http://localhost:*", "http://127.0.0.1:*"}
	}
	return strings.Split(origins, ",")
}

// authMiddleware 鉴权中间件：检查用户是否已登录
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		username := session.Get(admin.SessionKey)
		if username == nil {
			c.AbortWithStatusJSON(401, gin.H{"code": "401", "msg": "未登录或登录已过期", "data": nil})
			return
		}
		c.Next()
	}
}
