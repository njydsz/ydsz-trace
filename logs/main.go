package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ydsz-trace/logs/controllers/task"
	models "ydsz-trace/logs/models"
	_ "ydsz-trace/logs/routers"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/toolbox"
)

// getEnv 获取环境变量，如果不存在则返回默认值
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func init() {
	// 数据库配置：优先从环境变量读取，其次从配置文件读取
	sqlhost := getEnv("YDSZ_DB_HOST", beego.AppConfig.String("sqlhost"))
	sqlport := getEnv("YDSZ_DB_PORT", beego.AppConfig.String("sqlport"))
	sqluser := getEnv("YDSZ_DB_USER", beego.AppConfig.String("sqluser"))
	sqlpwd := getEnv("YDSZ_DB_PASSWORD", beego.AppConfig.String("sqlpwd"))
	database := getEnv("YDSZ_DB_NAME", beego.AppConfig.String("database"))
	maxIdleConns, _ := beego.AppConfig.Int("maxIdleConns")
	maxOpenConns, _ := beego.AppConfig.Int("maxOpenConns")

	// 初始化数据链接
	db := models.DBConfig{
		Host:         sqlhost,
		Port:         sqlport,
		Username:     sqluser,
		Password:     sqlpwd,
		Database:     database,
		MaxIdleConns: maxIdleConns,
		MaxOpenConns: maxOpenConns,
	}
	models.NewDef(&db)

	// 初始化定时任务
	task.InitTask()
}

func main() {
	// 定时任务启动
	toolbox.StartTask()
	defer toolbox.StopTask()

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
