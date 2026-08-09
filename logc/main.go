// Package main 是 logc 客户端代理服务的入口。
//
// 职责：
//   - 部署在每台需要被集中收集日志的机器上
//   - 接收 logs 服务端的查询请求，读取本机日志文件并返回压缩结果
//   - 自动向 logs 注册并保持心跳，上报存活状态
//
// 启动流程：解析参数 → 加载配置 → 注册到 logs → 启动心跳 → 启动 HTTP 服务
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

// main 入口函数。
//
// 命令行参数：
//   - -s: 日志服务地址（ip:port）
//   - -v: 认证密钥
//   - -c: 配置文件路径（默认 conf/app.conf）
func main() {
	// 定义变量，用于接收命令行的参数值。
	// 优先级：命令行参数 > 环境变量 > 配置文件 > 内置默认
	var server string
	var vkey string
	var confPath string
	flag.StringVar(&server, "s", "", "日志服务地址（ip:port）")
	flag.StringVar(&vkey, "v", "", "客户端认证密钥")
	flag.StringVar(&confPath, "c", "conf/app.conf", "INI 配置文件路径")
	flag.Parse()

	// 加载配置文件；失败时使用空配置（后续通过 EnvOrConfig 降级）。
	cfg, err := config.Load(confPath)
	if err != nil {
		log.Printf("加载配置失败: %v，使用内置默认值", err)
		cfg = config.NewDefault()
	}

	// 密钥/服务地址优先级：命令行 > 环境变量 > 配置文件 > 内置默认。
	if vkey == "" {
		vkey = config.EnvOrConfig("YDSZ_CLIENT_KEY", cfg.String("key"), "123456")
	}
	if server == "" {
		server = config.EnvOrConfig("YDSZ_LOG_SERVER", cfg.String("logs"), "127.0.0.1:2021")
	}

	// 弱密钥告警：提示管理员必要时通过 YDSZ_CLIENT_KEY 替换
	if vkey == "123456" || vkey == "ydsz_trace_key" {
		log.Printf("[security] 警告：logc 使用默认预共享密钥（%v），建议通过 YDSZ_CLIENT_KEY 环境变量配置强密钥", vkey)
	}

	log.Printf("logc register -server=%v -vkey=%v\n", server, vkey)
	register.RegisterLocalIp(server, vkey)

	// 启动心跳续约（启动时注册一次，之后每 60 秒重注册，带指数退避）。
	register.StartGlobalHeartbeat(server, vkey)

	// 构建 Gin 路由并启动 HTTP 服务。
	port := cfg.StringOr("httpport", "2020")
	handler := routers.SetupRouter(cfg)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
	}

	// 启动后台 goroutine 监听退出信号，触发优雅关闭。
	go gracefulShutdown(srv)

	log.Printf("logc 启动成功，监听端口 %s\n", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP服务启动失败: %v", err)
	}
}

// gracefulShutdown 监听 SIGTERM/SIGINT，停止心跳并优雅关闭 HTTP 服务。
//
// 关闭超时 10 秒；超时后强制退出。
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
