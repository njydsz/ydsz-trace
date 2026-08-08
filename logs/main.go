package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ydsz-trace/logs/routers"
	"ydsz-trace/logs/controllers/task"
	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/session"

	"github.com/gin-gonic/gin"
)

// getEnv 获取环境变量，如果不存在则返回默认值
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {
	// 加载配置
	cfg, err := config.Load("conf/app.conf")
	if err != nil {
		log.Printf("加载配置失败: %v，使用内置默认值", err)
		cfg = config.NewDefault()
	}

	// 数据库配置：优先从环境变量读取，其次从配置文件读取
	sqlhost := getEnv("YDSZ_DB_HOST", cfg.StringOr("sqlhost", "127.0.0.1"))
	sqlport := getEnv("YDSZ_DB_PORT", cfg.StringOr("sqlport", "3306"))
	sqluser := getEnv("YDSZ_DB_USER", cfg.StringOr("sqluser", "root"))
	sqlpwd := getEnv("YDSZ_DB_PASSWORD", cfg.StringOr("sqlpwd", "change_me_production"))
	database := getEnv("YDSZ_DB_NAME", cfg.StringOr("database", "ydsz_trace"))
	maxIdleConns := cfg.Int("maxIdleConns", 10)
	maxOpenConns := cfg.Int("maxOpenConns", 50)

	// 初始化数据库连接
	dbConf := models.DBConfig{
		Host:         sqlhost,
		Port:         sqlport,
		Username:     sqluser,
		Password:     sqlpwd,
		Database:     database,
		MaxIdleConns: maxIdleConns,
		MaxOpenConns: maxOpenConns,
	}
	if err := models.InitDB(&dbConf); err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	defer models.DB.Close()

	// 初始化定时任务
	cronTask := task.InitTask(cfg)
	cronTask.Start()
	defer cronTask.Stop()

	// 构建 Gin 路由
	port := cfg.StringOr("httpport", "2021")
	sessionMgr := session.NewManager()
	handler := routers.SetupRouter(cfg, sessionMgr)

	// 释放 gin 调试模式内存（可选）
	gin.SetMode(gin.ReleaseMode)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
	}

	// 优雅关闭：监听系统信号
	go gracefulShutdown(srv)

	log.Printf("logs 启动成功，监听端口 %s\n", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP服务启动失败: %v", err)
	}
}

// gracefulShutdown 监听 SIGTERM/SIGINT，优雅关闭 HTTP 服务
func gracefulShutdown(srv *http.Server) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("收到退出信号，正在优雅关闭...")

	// 设置关闭超时 10 秒
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP服务关闭失败: %v", err)
	} else {
		log.Println("HTTP服务已优雅关闭")
	}
	os.Exit(0)
}
