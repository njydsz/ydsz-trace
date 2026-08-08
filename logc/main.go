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

	register "ydsz-trace/logc/controllers/register"
	"ydsz-trace/logc/routers"
	"ydsz-trace/pkg/config"
)

func main() {
	// 定义变量，用于接收命令行的参数值
	var server string
	var vkey string
	var confPath string
	flag.StringVar(&server, "s", "", "ip+port")
	flag.StringVar(&vkey, "v", "", "密钥")
	flag.StringVar(&confPath, "c", "conf/app.conf", "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(confPath)
	if err != nil {
		log.Printf("加载配置失败: %v，使用内置默认值", err)
		cfg = config.NewDefault()
	}

	// 密钥优先使用命令行参数，其次环境变量，最后配置文件
	if vkey == "" {
		vkey = config.EnvOrConfig("YDSZ_CLIENT_KEY", cfg.String("key"), "123456")
	}
	if server == "" {
		server = config.EnvOrConfig("YDSZ_LOG_SERVER", cfg.String("logs"), "127.0.0.1:2021")
	}

	log.Printf("logc register -server=%v -vkey=%v\n", server, vkey)
	register.RegisterLocalIp(server, vkey)

	// 启动定期心跳续约（每 60 秒重注册一次）
	register.StartGlobalHeartbeat(server, vkey)

	// 构建 Gin 路由
	port := cfg.StringOr("httpport", "2020")
	handler := routers.SetupRouter(cfg)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
	}

	// 优雅关闭：监听系统信号
	go gracefulShutdown(srv)

	log.Printf("logc 启动成功，监听端口 %s\n", port)
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

	// 停放心跳续约
	register.StopGlobalHeartbeat()

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
