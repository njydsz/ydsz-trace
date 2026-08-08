package routers

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
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

// defaultSessionSecret 默认 Session 密钥（仅开发环境可用）
const defaultSessionSecret = "ydsz-trace-session-secret"

// generateTraceID 生成追踪 ID（标准库实现，无需外部依赖）
func generateTraceID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// 降级使用时间戳+随机数
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405"))) + "-fallback"
	}
	return hex.EncodeToString(buf)
}

// checkDangerousDefaults 检测到危险默认值时返回问题列表
func checkDangerousDefaults(cfg *config.Config) []string {
	var problems []string

	// 检查是否为生产模式
	runmode := cfg.StringOr("runmode", "dev")
	isProd := runmode != "dev"

	// 1. Session Secret：生产环境必须通过环境变量覆盖
	if os.Getenv("YDSZ_SESSION_SECRET") == "" {
		if isProd {
			problems = append(problems, "生产环境必须设置 YDSZ_SESSION_SECRET 环境变量")
		} else {
			log.Printf("[WARN] 使用默认 Session 密钥仅允许开发环境，生产环境请设置 YDSZ_SESSION_SECRET")
		}
	}

	// 2. 管理员密码：生产环境必须覆盖默认值
	adminPass := config.EnvOrConfig("YDSZ_ADMIN_PASSWORD", cfg.String("password"), "")
	if adminPass == "" || adminPass == "change_me_production" {
		if isProd {
			problems = append(problems, "生产环境必须设置 YDSZ_ADMIN_PASSWORD 环境变量（禁止使用默认值）")
		} else {
			log.Printf("[WARN] 使用空/默认管理员密码仅允许开发环境")
		}
	}

	// 3. 数据库密码检查
	dbPwd := config.EnvOrConfig("YDSZ_DB_PASSWORD", cfg.String("sqlpwd"), "")
	if dbPwd == "" || dbPwd == "change_me_production" {
		if isProd {
			problems = append(problems, "生产环境必须设置 YDSZ_DB_PASSWORD 环境变量")
		}
	}

	return problems
}

// SetupRouter 构建 Gin 路由
func SetupRouter(cfg *config.Config) *gin.Engine {
	// 危险默认值检查
	if problems := checkDangerousDefaults(cfg); len(problems) > 0 {
		log.Printf("启动安全校验失败，存在 %d 个风险项:", len(problems))
		for _, p := range problems {
			log.Printf("  - %s", p)
		}
		log.Fatal("请修复以上安全配置后再启动服务")
	}

	// 根据运行模式设置 gin 模式
	runmode := cfg.StringOr("runmode", "dev")
	if runmode != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// TraceId 注入中间件
	r.Use(traceIDMiddleware())

	// 注入配置到 context
	r.Use(func(c *gin.Context) {
		c.Set("cfg", cfg)
		c.Next()
	})

	// Session 中间件（cookie 存储）
	sessionSecret := defaultSessionSecret
	if secret := os.Getenv("YDSZ_SESSION_SECRET"); secret != "" {
		sessionSecret = secret
	}
	store := cookie.NewStore([]byte(sessionSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		Secure:   runmode != "dev",
		SameSite: http.SameSiteStrictMode,
	})
	r.Use(sessions.Sessions("ydsz_trace_session", store))

	// CORS 白名单：从环境变量 YDSZ_CORS_ORIGINS 读取，逗号分隔
	r.Use(cors.New(cors.Config{
		AllowOrigins:     getCORSOrigins(),
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type", "X-Token", "X-Requested-With", "X-Trace-ID"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type", "X-Trace-ID"},
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

// traceIDMiddleware 注入请求追踪 ID
func traceIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-ID")
		if traceID == "" {
			traceID = generateTraceID()
		}
		c.Set("traceId", traceID)
		c.Header("X-Trace-ID", traceID)
		c.Next()
	}
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
