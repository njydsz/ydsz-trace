// Package routers 定义 logc 客户端代理的 HTTP 路由。
//
// 中间件链：gin.Logger → gin.Recovery → 配置注入 → Source 注入 → CORS。
// 注意：logc 不需要会话中间件（由 logs 服务端鉴权，代理调用接口放行）。
package routers

import (
	"os"
	"strings"
	"time"

	"ydsz-trace/logc/controllers"
	"ydsz-trace/logc/controllers/file"
	"ydsz-trace/logc/controllers/register"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/source"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// activeSource 当前日志源实例（由 main.go 在 SetupRouter 之前设置）。
var activeSource source.Source

// SetActiveSource 主入口调用，设置路由层使用的 Source 实例。
func SetActiveSource(s source.Source) {
	activeSource = s
}

// SetupRouter 构建 Gin 路由引擎。
//
// 路由分组：
//   - /, /health, /ready：健康检查
//   - /file/query：日志查询（供 logs 服务端调用）
//   - /file/tail：实时跟踪（供 logs 服务端调用）
//   - /register, /checkOn：注册与在线检测
//   - /source/info：当前 Source 状态（调试用）
//   - /source/targets：已发现的目标（Docker / K8s 模式）
func SetupRouter(cfg *config.Config) *gin.Engine {
	// 非 dev 模式关闭 gin Debug 输出。
	runmode := cfg.StringOr("runmode", "dev")
	if runmode != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// 配置注入：使 handler 通过 c.MustGet("cfg") 获取 *config.Config。
	r.Use(func(c *gin.Context) {
		c.Set("cfg", cfg)
		c.Next()
	})

	// Source 注入：把全局 activeSource 放进 Context，handler 通过 c.MustGet 获取。
	r.Use(func(c *gin.Context) {
		c.Set("__source__", activeSource)
		c.Next()
	})

	// CORS 白名单，支持环境变量 YDSZ_CORS_ORIGINS（逗号分隔）。
	r.Use(cors.New(cors.Config{
		AllowOrigins:     getCORSOrigins(),
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type", "X-Token", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// 路由注册。
	r.GET("/", controllers.Main)
	r.GET("/health", controllers.Health)
	r.GET("/ready", controllers.Ready)
	r.GET("/source/info", controllers.SourceInfo)
	r.GET("/source/targets", controllers.SourceTargets)
	r.POST("/file/query", file.Query)
	r.POST("/file/search", file.Search)
	r.POST("/file/tail", file.Tail)
	r.POST("/register", register.Register)
	r.GET("/checkOn", register.CheckOnline)

	return r
}

// getCORSOrigins 从 YDSZ_CORS_ORIGINS 解析白名单，默认仅允许本地开发源。
func getCORSOrigins() []string {
	origins := os.Getenv("YDSZ_CORS_ORIGINS")
	if origins == "" {
		return []string{"http://localhost:*", "http://127.0.0.1:*"}
	}
	return strings.Split(origins, ",")
}
