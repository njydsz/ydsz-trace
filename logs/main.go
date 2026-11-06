// Package main 是 logs 日志收集服务端的入口。
//
// 职责：
//   - 管理客户端（logc agent）注册、心跳状态维护
//   - 接收前端查询请求，并发拉取各客户端日志并合并返回
//   - 提供前端控制台（SPA 托管，前端工程位于 logv/）
//
// 启动流程：加载配置 → 初始化数据库 → 启动定时任务 → 清理临时文件 → 启动 HTTP 服务
//
// @title Ydsz Trace API
// @version 2.0
// @description AI 时代日志检索分析引擎 API
// @termsOfService http://swagger.org/terms/
// @contact.name API Support
// @contact.email support@ydsz.local
// @license.name Apache 2.0
// @host localhost:2021
// @BasePath /api
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Cookie
package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"ydsz-trace/logs/agentlink"
	"ydsz-trace/logs/controllers/alert"
	"ydsz-trace/logs/controllers/client"
	"ydsz-trace/logs/controllers/task"
	models "ydsz-trace/logs/models"
	"ydsz-trace/logs/pipeline"
	"ydsz-trace/logs/routers"
	agentproto "ydsz-trace/pkg/agentlink"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/embedding"
	pkgtls "ydsz-trace/pkg/tls"
	"ydsz-trace/pkg/ratelimit"
	"ydsz-trace/pkg/session"
	"ydsz-trace/pkg/util"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
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

// jwtDefaultSecret JWT 模块内置的出厂默认密钥。
// 与 enforceProductionSecurity 配合，在生产模式下检测并拒绝弱密钥启动。
const jwtDefaultSecret = "ydsz-trace-jwt-secret-change-in-production"

// enforceProductionSecurity 生产模式下强制校验：拒绝弱口令 + 默认 JWT 密钥启动。
//
// 仅在 runmode != "dev" 时生效，开发模式跳过以保留零配置体验。
// 发现安全问题时直接 log.Fatalf 拒绝启动，避免带病上线。
// 符合 PRD FR-1310（JWT 安全增强）与编码规范 §9.1。
//
// 校验项：
//   - admin 密码不得为已知弱口令/出厂默认
//   - JWT 签名密钥不得使用出厂默认（生产必须通过 YDSZ_JWT_SECRET 注入）
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

	// JWT 密钥不得使用出厂默认（防令牌伪造）
	jwtSecret := os.Getenv("YDSZ_JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = cfg.StringOr("jwt_secret", "")
	}
	if jwtSecret == "" || jwtSecret == jwtDefaultSecret {
		log.Fatalf("[security] 拒绝启动：生产模式检测到 JWT 使用出厂默认密钥。\n" +
			"请通过 YDSZ_JWT_SECRET 环境变量注入强密钥（建议 32 字节以上随机字符串）。")
	}
}

// getEnv 获取环境变量；未设置时返回 fallback。
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// buildWritePipelineEmbedder 构建写入管线使用的 L2 Embedder。
//
// 优先级：环境变量（YDSZ_EMBED_*）→ 数据库能力配置（t_model_capability, capability='embedding'）。
//
// 探测失败时返回 (nil, ProviderLocal)，管线据此跳过向量化阶段并降级为 L0+L1，
// 其余 Drain/模板/统计/异常阶段不受影响（FR-801 可用性隔离）。
func buildWritePipelineEmbedder() (embedding.Embedder, embedding.Provider) {
	e, provider, err := embedding.NewFromEnv()
	if err != nil {
		log.Printf("[pipeline] L2 嵌入层未启用: %v", err)
		return nil, embedding.ProviderLocal
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if perr := embedding.Probe(ctx, e); perr != nil {
		log.Printf("[pipeline] L2 嵌入层探测失败，降级为 L0+L1: %v", perr)
		return nil, provider
	}

	log.Printf("[pipeline] L2 嵌入层已启用 provider=%d model=%s dim=%d",
		provider, os.Getenv(embedding.EnvEmbedModel), e.Dimension())
	return e, provider
}

// buildSpoolBuffer 构建管线背压缓冲队列（V2.2-09）。
//
// 路径优先级：YDSZ_SPOOL_FILE > YDSZ_DATA_DIR/spool.bin > ./spool.bin
// 容量默认 10000 条（可通过 YDSZ_SPOOL_CAPACITY 覆盖）。
func buildSpoolBuffer() (*pipeline.SpoolBuffer, error) {
	path := os.Getenv("YDSZ_SPOOL_FILE")
	if path == "" {
		if dir := os.Getenv("YDSZ_DATA_DIR"); dir != "" {
			path = filepath.Join(dir, "spool.bin")
		}
	}

	capacity := 10000
	if v := os.Getenv("YDSZ_SPOOL_CAPACITY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			capacity = n
		}
	}

	return pipeline.NewSpoolBuffer(path, capacity, 30*time.Second)
}

// loadMaintenanceWindows 从数据库加载静默窗口并转换为 alert 包的结构。
//
// 查询失败或无记录时返回 nil（等价于无静默配置，不影响告警正常运行）。
func loadMaintenanceWindows() []alert.MaintenanceWindow {
	rows := models.QueryMaintenanceWindows()
	if len(rows) == 0 {
		return nil
	}

	out := make([]alert.MaintenanceWindow, 0, len(rows))
	for _, w := range rows {
		out = append(out, alert.MaintenanceWindow{
			Name:      w.Name,
			Start:     parseClockTime(w.StartTime),
			End:       parseClockTime(w.EndTime),
			Recurring: w.Recurring,
			Weekdays:  w.Weekdays,
			Enabled:   w.Enabled,
		})
	}
	log.Printf("[alert] 已加载 %d 个静默窗口", len(out))
	return out
}

// parseClockTime 将 "HH:MM" 转为仅含时分的时间值（用于静默窗口比较）。
func parseClockTime(hhmm string) time.Time {
	if len(hhmm) != 5 || hhmm[2] != ':' {
		return time.Time{}
	}
	h, err1 := strconv.Atoi(hhmm[:2])
	m, err2 := strconv.Atoi(hhmm[3:])
	if err1 != nil || err2 != nil {
		return time.Time{}
	}
	return time.Date(0, 1, 1, h, m, 0, 0, time.Local)
}

// embeddingModelVersion 返回向量索引的模型版本标识，用于 t_embedding.model_version。
//
// 未显式配置模型名时按 Provider 内置默认推断，保证同一索引内模型版本一致
// （避免混用不同模型导致余弦相似度无意义）。
func embeddingModelVersion(e embedding.Embedder) string {
	if e == nil {
		return "disabled"
	}
	model := os.Getenv(embedding.EnvEmbedModel)
	if model == "" {
		model = embedding.DefaultLocalModel
	}
	return model
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

	// 注册密钥强度校验（P0-2）：生产模式拒绝默认/弱密钥启动。
	if err := client.ValidateRegisterKeyAtStartup(cfg); err != nil {
		log.Fatalf("[security] 拒绝启动：%v", err)
	}

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

	// 创建 V2.1 新增表（幂等）：t_user_binding / t_conversation / t_conversation_msg / t_incident_report。
	if err := models.CreateUserBindingTable(); err != nil {
		log.Printf("[warn] 创建 t_user_binding 表失败: %v", err)
	}
	if err := models.CreateConversationTable(); err != nil {
		log.Printf("[warn] 创建 t_conversation 表失败: %v", err)
	}
	if err := models.CreateIncidentReportTable(); err != nil {
		log.Printf("[warn] 创建 t_incident_report 表失败: %v", err)
	}

	// 首次启动种子：t_user 为空时使用配置账号创建 admin 用户
	seedUname := getEnv("YDSZ_ADMIN_USER", cfg.StringOr("username", "admin"))
	seedPwd := getEnv("YDSZ_ADMIN_PASSWORD", cfg.StringOr("password", "123456"))
	if err := models.SeedAdminUser(seedUname, seedPwd); err != nil {
		log.Printf("[warn] 种子 admin 用户失败: %v", err)
	}

	// 启动定时任务：按 cron 表达式定期探测所有客户端在线状态。
	cronTask := task.InitTask(cfg)
	cronTask.Start()
	defer cronTask.Stop()

	// 后台告警评估调度：按 interval_sec 每分钟扫描 eval 启用的规则。
	alertScheduler := alert.NewScheduler(5)
	// 加载数据库中的静默窗口（V2.2 FR-806）：启动时生效，无需手动 sync
	if sup := alert.NewSuppressor(); sup != nil {
		sup.ReplaceWindows(loadMaintenanceWindows())
		alertScheduler.SetSuppressor(sup)
	}
	alert.RegisterScheduler(alertScheduler)
	defer alert.RegisterScheduler(nil)
	alertScheduler.Start()
	defer alertScheduler.Stop()

	// 后台写入管线（V2.1）：定时扫描日志 → Drain → 模板/向量/统计/异常落库。
	// L2 嵌入层按环境变量/能力配置构建；不可达时降级为 L0+L1，管线其余阶段照常运行。
	embedder, _ := buildWritePipelineEmbedder()
	writePipeline := pipeline.New(embedder, embeddingModelVersion(embedder))

	// 背压本地缓冲（V2.2-09，FR-103）：下游写入失败时暂存，链路恢复后补传。
	// 队列文件路径：YDSZ_SPOOL_FILE > YDSZ_DATA_DIR/spool.bin
	if buf, berr := buildSpoolBuffer(); berr != nil {
		log.Printf("[pipeline] 背压缓冲未启用: %v", berr)
	} else {
		writePipeline.SetSpoolBuffer(buf)
		defer buf.Stop()
	}

	writePipeline.Start()
	defer writePipeline.Stop()

	// 启动时清理残留临时文件（日志查询产生的中间文件，超过 2 小时自动删除）。
	temppath := cfg.StringOr("temppath", "./temp/logs/")
	cleanedFiles, cleanedDirs, _ := util.CleanupOldFiles(temppath, util.DefaultCleanupOptions)
	if cleanedFiles > 0 || cleanedDirs > 0 {
		log.Printf("启动清理：删除 %d 个过期文件，%d 个过期目录", cleanedFiles, cleanedDirs)
	}

	// 构建 Gin 路由并启动 HTTP 服务。
	port := cfg.StringOr("httpport", "2021")
	sessionMgr, redisClient := buildSessionManager(cfg)

	// 长连接通道服务端（P1-1/P1-2）：复用 HTTP 端口，WebSocket 升级接入。
	// 仅在 YDSZ_AGENT_MODE != "pull" 时启用；默认 push 模式。
	if agentlinkMode(cfg) != "pull" {
		regKey := resolveRegisterKey(cfg)
		agentSrv := agentlink.NewServer(regKey)
		agentSrv.SetBatchHandler(func(batch *agentproto.LogBatch) {
			// P1-3：接收日志批次，交由写入管线实时入库。
			if len(batch.Lines) == 0 {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := writePipeline.IngestBatch(ctx, batch.ClientID, batch.ItemID, batch.Lines); err != nil {
				log.Printf("[agentlink] 批次入库处理失败 batchNo=%s clientId=%d itemId=%d lines=%d: %v",
					batch.BatchNo, batch.ClientID, batch.ItemID, len(batch.Lines), err)
				return
			}
			log.Printf("[agentlink] 批次已入库 batchNo=%s clientId=%d itemId=%d lines=%d",
				batch.BatchNo, batch.ClientID, batch.ItemID, len(batch.Lines))
		})
		routers.SetAgentLinkServer(agentSrv)
		stopReaper := agentSrv.StartReaper(30 * time.Second)
		defer stopReaper()
		log.Printf("[agentlink] 长连接通道已启用，在线连接数=%d", agentSrv.OnlineCount())
	}

	handler := routers.SetupRouter(cfg, sessionMgr)

	gin.SetMode(gin.ReleaseMode)

	// TLS/mTLS 配置（PRD FR-1308）：启用后要求客户端证书，未启用保持明文 HTTP。
	tlsCfg := pkgtls.LoadFromEnv()
	if err := tlsCfg.ValidateServer(); err != nil {
		log.Fatalf("[tls] 配置校验失败: %v", err)
	}
	var tlsConfig *tls.Config
	if tlsCfg.Enabled {
		var terr error
		tlsConfig, terr = tlsCfg.ServerTLSConfig()
		if terr != nil {
			log.Fatalf("[tls] 服务端配置加载失败: %v", terr)
		}
		log.Printf("[tls] mTLS 已启用（最低 %s，要求并校验客户端证书）", util.TLSVersionName(tlsConfig.MinVersion))
	}

	srv := &http.Server{
		Addr:      ":" + port,
		Handler:   handler,
		TLSConfig: tlsConfig,
	}

	// 后台监听退出信号，触发优雅关闭。
	go gracefulShutdown(srv, sessionMgr, redisClient)

	log.Printf("logs 启动成功，监听端口 %s\n", port)
	if tlsConfig != nil {
		// TLS 模式：通过 ListenAndServeTLS 加载证书并监听。
		if err := srv.ListenAndServeTLS(tlsCfg.CertFile, tlsCfg.KeyFile); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTPS服务启动失败: %v", err)
		}
		return
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP服务启动失败: %v", err)
	}
}

// gracefulShutdown 监听 SIGTERM/SIGINT，10 秒内优雅关闭 HTTP 服务。
func gracefulShutdown(srv *http.Server, sessionMgr *session.Manager, redisClient *redis.Client) {
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
	if sessionMgr != nil {
		_ = sessionMgr.Close()
	}
	if redisClient != nil {
		_ = redisClient.Close()
	}
	os.Exit(0)
}

// agentlinkMode 返回长连接通道模式：push（默认）| pull。
//
// 环境变量 YDSZ_AGENT_MODE 优先，其次配置文件 agent_mode。
func agentlinkMode(cfg *config.Config) string {
	mode := os.Getenv("YDSZ_AGENT_MODE")
	if mode == "" {
		mode = cfg.StringOr("agent_mode", "push")
	}
	if mode != "pull" {
		return "push"
	}
	return "pull"
}

// resolveRegisterKey 解析注册密钥（与 client 包一致）。
func resolveRegisterKey(cfg *config.Config) string {
	if v := os.Getenv("YDSZ_REGISTER_KEY"); v != "" {
		return v
	}
	if cfg != nil {
		if v := cfg.StringOr("register_key", ""); v != "" {
			return v
		}
	}
	return client.DefaultRegisterKey
}

// buildSessionManager 根据配置创建会话管理器：检测到 redisenv / YDSZ_REDIS_ADDR 时使用 Redis，
// 否则回落到进程内存后端（单副本默认零依赖）。
func buildSessionManager(cfg *config.Config) (*session.Manager, *redis.Client) {
	addr := getEnv("YDSZ_REDIS_ADDR", cfg.StringOr("redisaddr", ""))
	if addr == "" {
		log.Println("[session] 使用进程内存后端（单副本模式）")
		mgr := session.NewManager()
		if cfg.StringOr("runmode", "dev") == "dev" {
			mgr.SetInsecureDevMode()
			log.Println("[session] dev 模式：已关闭 Cookie Secure 属性")
		}
		return mgr, nil
	}

	password := getEnv("YDSZ_REDIS_PASSWORD", cfg.StringOr("redispwd", ""))
	db := cfg.Int("redisdb", 0)

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// 验证连通性
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("[session] Redis 连接失败 (%v)，回落到内存后端", err)
		client.Close()
		mgr := session.NewManager()
		if cfg.StringOr("runmode", "dev") == "dev" {
			mgr.SetInsecureDevMode()
			log.Println("[session] dev 模式：已关闭 Cookie Secure 属性")
		}
		return mgr, nil
	}

	log.Printf("[session] 使用 Redis 后端（addr=%s, db=%d）", addr, db)
	ratelimit.SetDefault(ratelimit.NewRedisLimiter(client, ratelimit.DefaultWindow, ratelimit.DefaultMaxFail, ratelimit.DefaultBlockFor))
	return session.NewRedisManager(addr, password, db), client
}
