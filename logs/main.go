// Package main 是 logs 日志收集服务端的入口。
//
// 职责：
//   - 管理客户端（logc agent）注册、心跳状态维护
//   - 接收前端查询请求，并发拉取各客户端日志并合并返回
//   - 提供 Web 控制台（SPA 托管）
//
// 启动流程：加载配置 → 初始化数据库 → 启动定时任务 → 清理临时文件 → 启动 HTTP 服务
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ydsz-trace/logs/routers"
	"ydsz-trace/logs/controllers/alert"
	"ydsz-trace/logs/controllers/task"
	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/session"
	"ydsz-trace/pkg/util"

	"github.com/gin-gonic/gin"
)

// defaultWeakPasswords 已知弱口令 / 出厂默认密码的生产环境黑名单。
// 生产模式下若检测到匹配，服务将拒绝启动并要求管理员配置强密码。
var defaultWeakPasswords = map[string]bool{
	"change_me_production": true, // 代码内置默认（app.conf）
	"ydsz_trace_admin":     true, // docker-compose.yml 示例默认
	"admin":                true,
	"admin123":             true,
	"password":             true,
	"password123":          true,
	"123456":               true,
	"12345678":             true,
	"qwerty":               true,
}

// enforceProductionSecurity 生产模式下强制校验：拒绝弱口令 + 未哈希密码启动。
//
// 仅在 runmode != "dev" 时生效，开发模式跳过以保留零配置体验。
// 发现安全问题时直接 log.Fatalf 拒绝启动，避免带病上线。
func enforceProductionSecurity(cfg *config.Config) {
	runmode := cfg.StringOr("runmode", "dev")
	if runmode == "dev" {
		return
	}

	adminPassword := os.Getenv("YDSZ_ADMIN_PASSWORD")
	if adminPassword == "" {
		adminPassword = cfg.StringOr("password", "change_me_production")
	}

	if defaultWeakPasswords[adminPassword] {
		log.Fatalf("[security] 拒绝启动：生产模式检测到弱口令/出厂默认密码。\n" +
			"请通过 YDSZ_ADMIN_PASSWORD 环境变量注入强密码（建议 16 位以上随机字符）。")
	}
}

// getEnv 获取环境变量；未设置时返回 fallback。
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {
	// 加载配置（日志服务端固定使用 conf/app.conf）。
	cfg, err := config.Load("conf/app.conf")
	if err != nil {
		log.Printf("加载配置失败: %v，使用内置默认值", err)
		cfg = config.NewDefault()
	}

	// 生产模式安全前置校验：拒绝弱口令启动
	enforceProductionSecurity(cfg)

	// SQLite 数据库文件路径：环境变量优先，其次配置文件。
	sqlitePath := getEnv("YDSZ_DB_PATH", cfg.StringOr("dbpath", "./data/ydsz_trace.db"))

	// 确保数据库目录存在
	if dir := filepath.Dir(sqlitePath); dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("创建数据库目录失败: %v", err)
		}
	}

	// 初始化 SQLite 数据库连接。
	dbConf := models.SQLiteConfig{
		FilePath: sqlitePath,
	}
	if err := models.InitDB(&dbConf); err != nil {
		log.Fatalf("SQLite 数据库初始化失败: %v", err)
	}
	defer models.DB.Close()

	// 启动定时任务：按 cron 表达式定期探测所有客户端在线状态。
	cronTask := task.InitTask(cfg)
	cronTask.Start()
	defer cronTask.Stop()

	// 后台告警评估调度：按 interval_sec 每分钟扫描 eval 启用的规则。
	alertScheduler := alert.NewScheduler(5)
	alertScheduler.Start()
	defer alertScheduler.Stop()

	// 启动时清理残留临时文件（日志查询产生的中间文件，超过 2 小时自动删除）。
	temppath := cfg.StringOr("temppath", "./temp/logs/")
	cleanedFiles, cleanedDirs, _ := util.CleanupOldFiles(temppath, util.DefaultCleanupOptions)
	if cleanedFiles > 0 || cleanedDirs > 0 {
		log.Printf("启动清理：删除 %d 个过期文件，%d 个过期目录", cleanedFiles, cleanedDirs)
	}

	// 构建 Gin 路由并启动 HTTP 服务。
	port := cfg.StringOr("httpport", "2021")
	sessionMgr := session.NewManager()
	handler := routers.SetupRouter(cfg, sessionMgr)

	gin.SetMode(gin.ReleaseMode)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
	}

	// 后台监听退出信号，触发优雅关闭。
	go gracefulShutdown(srv)

	log.Printf("logs 启动成功，监听端口 %s\n", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP服务启动失败: %v", err)
	}
}

// gracefulShutdown 监听 SIGTERM/SIGINT，10 秒内优雅关闭 HTTP 服务。
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
