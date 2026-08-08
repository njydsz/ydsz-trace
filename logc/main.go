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
	_ "ydsz-trace/logc/routers"

	"github.com/astaxie/beego"
)

func main() {
	// 定义变量，用于接收命令行的参数值
	var server string
	var vkey string
	flag.StringVar(&server, "s", "", "ip+port")
	flag.StringVar(&vkey, "v", "", "密钥")
	flag.Parse()

	// 密钥优先使用命令行参数，其次环境变量，最后配置文件
	if vkey == "" {
		vkey = register.GetVKey()
	}
	if server == "" {
		server = beego.AppConfig.String("logs")
	}

	log.Printf("logc register -server=%v -vkey=%v\n", server, vkey)
	register.RegisterLocalIp(server, vkey)

	// 优雅关闭：监听系统信号
	go gracefulShutdown()

	beego.Run()
}

// gracefulShutdown 监听 SIGTERM/SIGINT，优雅关闭 HTTP 服务
func gracefulShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("收到退出信号，正在优雅关闭...")

	// 设置关闭超时 10 秒
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := beego.BeeApp.Server.Shutdown(ctx); err != nil {
		log.Printf("HTTP服务关闭失败: %v", err)
	} else {
		log.Println("HTTP服务已优雅关闭")
	}
	os.Exit(0)
}
