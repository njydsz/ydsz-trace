package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ydsz-trace/logs/controllers/task"
	models "ydsz-trace/logs/models"
	"ydsz-trace/logs/routers"
	"ydsz-trace/pkg/config"
)

func main() {
	var confPath string
	flag.StringVar(&confPath, "c", "conf/app.conf", "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(confPath)
	if err != nil {
		log.Printf("加载配置失败: %v，使用内置默认值", err)
		cfg = config.NewDefault()
	}

	// 初始化数据库连接
	if err := models.InitDB(cfg); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	log.Println("数据库连接成功")

	// 启动定时任务（客户端在线状态检测）
	cron := task.StartMonitor(cfg)
	defer cron.Stop()

	// 构建 Gin 路由
	port := cfg.StringOr("httpport", "2021")
	handler := routers.SetupRouter(cfg)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
	}

	// 优雅关闭：监听系统信号
	go gracefulShutdown(srv)

	log.Printf("ydsz-trace-logs 启动成功，监听端口 %s\n", port)
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
}
