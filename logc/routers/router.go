package routers

import (
	"os"
	"strings"
	"time"

	"ydsz-trace/logc/controllers"
	"ydsz-trace/logc/controllers/file"
	"ydsz-trace/logc/controllers/register"
	"ydsz-trace/pkg/config"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// SetupRouter 构建 Gin 路由
func SetupRouter(cfg *config.Config) *gin.Engine {
	// 根据运行模式设置 gin 模式
	runmode := cfg.StringOr("runmode", "dev")
	if runmode != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// 日志中间件 + 恢复中间件
	r.Use(gin.Logger(), gin.Recovery())

	// CORS 白名单：从环境变量 YDSZ_CORS_ORIGINS 读取，逗号分隔
	r.Use(cors.New(cors.Config{
		AllowOrigins:     getCORSOrigins(),
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type", "X-Token", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// 注册路由
	r.GET("/", controllers.Main)
	r.GET("/health", controllers.Health)
	r.GET("/ready", controllers.Ready)
	r.POST("/file/query", file.Query)
	r.POST("/register", register.Register)
	r.GET("/checkOn", register.CheckOnline)

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
