// Package routers 定义 logs 服务端的 HTTP 路由。
//
// 中间件链（按序）：
//
//	gin.Logger → gin.Recovery → 配置注入 → 会话中间件 → CORS → metrics → 鉴权中间件
//
// 路由分组：
//   - 公开：/, /health, /ready, /metrics, /admin/login, /admin/exit, /client/register
//   - 需鉴权：/client/*, /item/*, /logs/*
//   - SPA 回退：未命中 API 的 GET 请求回退到 index.html
package routers

import (
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"ydsz-trace/logs/controllers/admin"
	"ydsz-trace/logs/controllers/audit"
	"ydsz-trace/logs/controllers/client"
	"ydsz-trace/logs/controllers/item"
	"ydsz-trace/logs/controllers/alert"
	"ydsz-trace/logs/controllers/logs"
	"ydsz-trace/logs/controllers/migration"
	"ydsz-trace/logs/controllers/system"
	logsmiddleware "ydsz-trace/logs/middleware"
	"ydsz-trace/pkg/api"
	pkgauth "ydsz-trace/pkg/auth"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/metrics"
	"ydsz-trace/pkg/middleware"
	"ydsz-trace/pkg/session"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// SetupRouter 构建 Gin 路由引擎，注入会话管理器。
func SetupRouter(cfg *config.Config, sessionMgr *session.Manager) *gin.Engine {
	// 非 dev 模式关闭 gin Debug 输出。
	runmode := cfg.StringOr("runmode", "dev")
	if runmode != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// 访问日志 + panic 恢复。
	r.Use(gin.Logger(), gin.Recovery())

	// TraceId 注入（PRD FR-1405）：为每个请求生成/透传 traceId。
	r.Use(middleware.TraceID())
	// V2.3-09 链路追踪传播：识别 W3C/B3/SkyWalking 头并注入追踪载体
	r.Use(logsmiddleware.TraceMiddleware())

	// 配置注入：下游 handler 通过 c.MustGet("cfg") 获取配置。
	r.Use(func(c *gin.Context) {
		c.Set("cfg", cfg)
		c.Next()
	})

	// 会话中间件：为每个请求加载/创建会话。
	r.Use(sessionMgr.Middleware())

	// CORS 白名单：环境变量 > 本地开发放行所有 localhost 端口。
	r.Use(cors.New(cors.Config{
		AllowOriginFunc:  isAllowedOrigin,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type", "X-Token", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Prometheus 指标收集中间件（记录请求数/状态码/耗时）
	r.Use(metrics.HTTPMetricsMiddleware())

	// 健康检查与登录（无需鉴权）
	r.GET("/", admin.Index)
	r.GET("/health", admin.Health)
	r.GET("/ready", admin.Ready)
	r.GET("/api/v2/health", system.Health)
	r.GET("/api/v2/health/dependencies", system.HealthDependencies)
	r.GET("/metrics", metrics.Global().GinHandler())
	r.GET("/admin/login", admin.Login)
	r.POST("/admin/login", admin.Login)
	r.GET("/admin/exit", admin.Exit)
	r.POST("/admin/exit", admin.Exit)

	// 控制台（SPA 入口，由 Vite 构建的 logv/dist 提供）
	r.GET("/admin/console", admin.Console)

	// logc 代理注册/心跳接口（代理调用，无 session，需放行）
	r.POST("/client/register", client.Register)
	r.POST("/client/ping", client.Ping)

	// logc 长连接通道（P1：WebSocket 升级，代理调用，需放行）
	// 由注入的 AgentLinkServer 处理；nil 时返回 501。
	r.GET("/agent/link", func(c *gin.Context) {
		srv := GetAgentLinkServer()
		if srv == nil {
			api.Fail(c, api.CodeServerError, "长连接通道未启用")
			return
		}
		srv.ServeWS(c.Writer, c.Request)
	})

	// 鉴权分组：client / item / logs 需要登录 + 按角色分读写
	// viewer：只读查询（可浏览资源与搜索日志）
	// operator：写操作（增删改资源、触发检索任务 / 告警配置）
	// admin：用户/角色管理（预留，当前仅一个管理账号，全部对应到 admin）
	//
	// 注意：不使用 r.Group("") 模式，Gin 1.12 中该模式会导致部分注册路由
	// 在运行阶段回落到 NoRoute handler。改为直接注册在 r 上 + 路径感知中间件。

	// 鉴权中间件：对非公开路径统一应用 filterAuth + csrfProtection。
	r.Use(authBlockMiddleware)

	// 行级 RBAC 中间件（V2.1 FR-1302）：对非 admin 用户注入可访问资源 ID 列表。
	// 必须在 authBlockMiddleware 之后执行（需要 session 中的 username）。
	r.Use(logsmiddleware.AccessControlMiddleware())

	// ━━ viewer 级别（只读）━━
	// 行级 RBAC（V2.1-04）：对携带 ?id= 的单资源查询做访问校验。
	// admin 由中间件跳过（ItemIDsFromContext 返回 false），直接放行。
	r.GET("/client/query", logsmiddleware.FilterClientHandler(client.Query))
	r.GET("/client/queryAll", client.QueryAll)
	r.GET("/client/queryPage", client.QueryPage)
	r.GET("/item/query", logsmiddleware.FilterItemHandler(item.Query))
	r.GET("/item/queryAll", item.QueryAll)
	r.GET("/item/queryPage", item.QueryPage)
	r.GET("/logs/queryClients", logs.QueryClient)
	r.GET("/logs/queryItems", logs.QueryItem)
	r.GET("/logs/tasks/:taskNo", logs.QueryTask)
	r.GET("/logs/tasks/:taskNo/progress", logs.QueryTaskProgress)
	r.GET("/logs/tasks", logs.ListTasks)
	// /logs/context/:itemId — path 参数形式的行级校验
	r.GET("/logs/context/:itemId", logsmiddleware.FilterItemHandler(logs.Context))
	r.GET("/logs/dashboard/insight", logs.DashboardInsightHandler)
	r.GET("/logs/alerts/rules", alert.ListRules)
	r.GET("/logs/alerts/rules/:id", alert.GetRule)
	r.GET("/logs/alerts/events", alert.ListEvents)
	r.GET("/logs/alerts/quota", alert.Quota)
	// V2.2-05 渠道清单与连通性测试
	r.GET("/logs/alerts/channels", alert.ListChannels)
	r.GET("/logs/templates/search", logs.SearchTemplates)

	// V2.2 异常反馈闭环 / 静默窗口 / 异常时间线（FR-806 / FR-807）
	r.GET("/logs/anomalies/timeline", alert.AnomalyTimeline)
	r.GET("/logs/anomalies/feedback/stats", alert.FeedbackStats)
	r.GET("/logs/maintenance-windows", alert.ListMaintenanceWindows)

	// 审计日志 V2 路由（与 /admin/audit/* 并存，便于 v2 前端/API 调用）
	r.GET("/api/v2/audit/logs", audit.ListAuditLogs)
	r.GET("/api/v2/audit/stats", audit.GetAuditStats)

	// ━━ operator 级别（可写资源 + 触发查询）━━
	r.POST("/client/add", pkgauth.RequireOperator(), client.Add)
	r.POST("/client/delete", pkgauth.RequireOperator(), client.Delete)
	r.POST("/client/update", pkgauth.RequireOperator(), client.Update)
	r.POST("/client/changeStatus", pkgauth.RequireOperator(), client.ChangeClientStatus)
	// 配置热下发（P1-4）：单客户端 / 广播。
	r.POST("/agent/config/push", pkgauth.RequireOperator(), PushConfigHandler)
	r.POST("/agent/config/broadcast", pkgauth.RequireOperator(), BroadcastConfigHandler)
	r.POST("/agent/client/:id/mode", pkgauth.RequireOperator(), SetClientModeHandler)
	r.GET("/agent/client/:id/mode", GetClientModeHandler)
	r.POST("/item/add", pkgauth.RequireOperator(), item.Add)
	r.POST("/item/delete", pkgauth.RequireOperator(), item.Delete)
	r.POST("/item/update", pkgauth.RequireOperator(), item.Update)
	r.POST("/item/changeStatus", pkgauth.RequireOperator(), item.ChangeItemStatus)
	r.POST("/logs/query", pkgauth.RequireOperator(), logs.Query)
	r.POST("/logs/search", pkgauth.RequireOperator(), logs.Search)
	r.POST("/logs/trace", pkgauth.RequireOperator(), logs.Trace)
	r.POST("/logs/trace/waterfall", pkgauth.RequireOperator(), logs.TraceWaterfall)
	r.POST("/logs/stream", pkgauth.RequireOperator(), logs.Stream)
	r.POST("/logs/tail", pkgauth.RequireOperator(), logs.Tail)
	// NL2DSL 自然语言转结构化检索参数（FR-501）
	r.POST("/logs/nl2dsl/parse", pkgauth.RequireOperator(), logs.Nl2DslParse)
	// 根因分析 Agent（FR-901~908）
	r.POST("/logs/agent/reason", pkgauth.RequireOperator(), logs.AgentReasonHandler)
	r.GET("/logs/agent/tools", pkgauth.RequireOperator(), logs.AgentToolsHandler)
	// V2.3-03 工具成功率统计与调用明细
	r.GET("/logs/agent/tools/stats", pkgauth.RequireOperator(), logs.AgentToolStatsHandler)
	r.GET("/logs/agent/calls/recent", pkgauth.RequireOperator(), logs.AgentRecentCallsHandler)
	// V2.3-07 事故简报生成与导出
	r.POST("/logs/agent/report", pkgauth.RequireOperator(), logs.GenerateReportHandler)
	r.GET("/logs/agent/report/export", pkgauth.RequireOperator(), logs.ExportReportHandler)
	// V2.3-08 纠正回流案例库
	r.POST("/logs/cases/feedback", pkgauth.RequireOperator(), logs.SubmitCaseFeedback)
	r.GET("/logs/cases", pkgauth.RequireOperator(), logs.ListCasesHandler)
	r.GET("/logs/cases/search", pkgauth.RequireOperator(), logs.SearchCasesHandler)
	// V2.4-06 AI 会话审计查询与统计（FR-309）
	r.GET("/logs/audit/logs", pkgauth.RequireOperator(), logs.AuditLogsHandler)
	r.GET("/logs/audit/stats", pkgauth.RequireOperator(), logs.AuditStatsHandler)
	// V2.3-10 变更事件登记与关联
	r.POST("/logs/changes", pkgauth.RequireOperator(), logs.CreateChangeEventHandler)
	r.GET("/logs/changes", pkgauth.RequireOperator(), logs.ListChangesHandler)
	r.GET("/logs/changes/correlate", pkgauth.RequireOperator(), logs.CorrelateChangesHandler)
	// AI 诊断会话管理（FR-901）
	r.POST("/logs/conversation/create", pkgauth.RequireOperator(), logs.ConvCreate)
	r.GET("/logs/conversation/list", pkgauth.RequireOperator(), logs.ConvList)
	r.GET("/logs/conversation/:convId/messages", pkgauth.RequireOperator(), logs.ConvMessages)
	r.POST("/logs/conversation/append", pkgauth.RequireOperator(), logs.ConvAppend)
	r.DELETE("/logs/conversation/:convId", pkgauth.RequireOperator(), logs.ConvDelete)
	r.PUT("/logs/conversation/:convId/title", pkgauth.RequireOperator(), logs.ConvUpdateTitle)
	// 案例库管理（FR-907）
	r.POST("/logs/cases/save", pkgauth.RequireOperator(), logs.CaseSaveHandler)
	// 模型路由策略（FR-1304）
	r.GET("/logs/model-router/:capability", pkgauth.RequireOperator(), logs.ModelRouterResolveHandler)
	// 成本配额管理（FR-1305）
	r.POST("/logs/quota/check", pkgauth.RequireOperator(), logs.QuotaCheckHandler)
	r.POST("/logs/quota/consume", pkgauth.RequireOperator(), logs.QuotaConsumeHandler)
	r.GET("/logs/quota/stats/:modelId", pkgauth.RequireOperator(), logs.QuotaStatsHandler)
	r.GET("/logs/quota/summary", pkgauth.RequireOperator(), logs.QuotaSummaryHandler)
	r.POST("/logs/tasks/:taskNo/retry", pkgauth.RequireOperator(), logs.RetryTask)
	r.DELETE("/logs/tasks/:taskNo", pkgauth.RequireOperator(), logs.DeleteTask)
	r.POST("/logs/alerts/rules", pkgauth.RequireOperator(), alert.AddRule)
	r.PUT("/logs/alerts/rules/:id", pkgauth.RequireOperator(), alert.UpdateRule)
	r.DELETE("/logs/alerts/rules/:id", pkgauth.RequireOperator(), alert.DeleteRule)
	r.POST("/logs/alerts/rules/toggle", pkgauth.RequireOperator(), alert.ToggleRule)
	r.POST("/logs/alerts/rules/test", pkgauth.RequireOperator(), alert.TestFire)
	r.POST("/logs/alerts/channels/test", pkgauth.RequireOperator(), alert.TestChannel)
	r.DELETE("/logs/alerts/events/:id", pkgauth.RequireOperator(), alert.DeleteEvent)
	// V2.2 写操作：异常反馈标记与静默窗口配置
	r.POST("/logs/anomalies/feedback", pkgauth.RequireOperator(), alert.MarkFeedback)
	r.POST("/logs/maintenance-windows", pkgauth.RequireOperator(), alert.SaveMaintenanceWindows)
	r.POST("/logs/maintenance-windows/sync", pkgauth.RequireOperator(), alert.SyncMaintenanceWindows)

	// 自助修改密码（任何已登录用户可操作）
	r.POST("/admin/users/change-password", admin.ChangePassword)

	// 模型能力配置—只读操作（已登录用户可查看）。
	r.GET("/admin/model-capability/list", admin.ListModelCapabilities)
	r.GET("/admin/model-capability/:id", admin.GetModelCapability)

	// ━━ admin 级别（用户 / 角色管理）━━
	adm := r.Group("/admin")
	adm.Use(pkgauth.RequireRole(pkgauth.RoleAdmin))
	{
		// @Tags Admin
		adm.GET("/users", admin.ListUsers)
		adm.POST("/users", admin.CreateUser)
		adm.PUT("/users/:id", admin.UpdateUser)
		adm.DELETE("/users/:id", admin.DisableUser)

		// 审计日志（仅 admin 可查看）
		adm.GET("/audit/logs", audit.ListAuditLogs)
		adm.GET("/audit/stats", audit.GetAuditStats)

		// 统计与异常检测（仅 admin 可查看）
		adm.GET("/stats/patterns", admin.StatsPatterns)
		adm.GET("/stats/anomalies", admin.StatsAnomalyEvents)
		adm.GET("/stats/baseline", admin.StatsBaseline)
	adm.GET("/ai/status", admin.AIStatusHandler)
	// V2.4-04 日志分层生命周期（预览 / 执行 / 统计）
	adm.GET("/lifecycle/plan", admin.LifecyclePlanHandler)
	adm.POST("/lifecycle/apply", admin.LifecycleApplyHandler)
	adm.GET("/lifecycle/stats", admin.LifecycleStatsHandler)

		// 迁移工具：v1 -> v2 元数据迁移（FR-307，仅 admin）
		// run: 字段补全 + 自动备份 + 迁移确认
		// status: 各表行数 / schema 版本 / 备份列表
		// backups: 备份文件列表
		// restore: 从备份一键回滚
		adm.POST("/migration/run", migration.Run)
		adm.GET("/migration/status", migration.StatusHandler)
		adm.GET("/migration/backups", migration.ListBackupsHandler)
		// V2.4-05 v1 元数据导入与预览
		adm.GET("/migration/inspect-v1", migration.V1InspectHandler)
		adm.POST("/migration/import-v1", migration.V1ImportHandler)
		adm.POST("/migration/restore", migration.RestoreHandler)
		adm.POST("/migration/rollback", migration.RollbackHandler)

		// 备份与恢复（仅 admin，FR-1307）
		adm.POST("/backup/create", logs.BackupCreateHandler)
		adm.GET("/backup/list", logs.BackupListHandler)
		adm.POST("/backup/restore", logs.BackupRestoreHandler)
		adm.POST("/backup/cleanup", logs.BackupCleanupHandler)

		// @Tags ModelCapability
		// 模型能力配置—写操作（仅 admin）。
		adm.POST("/model-capability", admin.AddModelCapability)
		adm.PUT("/model-capability", admin.UpdateModelCapability)
		adm.POST("/model-capability/toggle", admin.ToggleModelCapability)
		adm.POST("/model-capability/delete", admin.DeleteModelCapability)
		adm.POST("/model-capability/selfcheck", admin.SelfCheckModelCapability)
	}

	// SPA 回退：未命中 API 的 GET/HEAD 请求回退到 index.html。
	r.NoRoute(admin.ServeStatic)

	return r
}

// localOriginPattern 匹配 http(s)://localhost:port 和 http(s)://127.0.0.1:port。
var localOriginPattern = regexp.MustCompile(`^https?://(localhost|127\.0\.0\.1)(:\d+)?$`)

// allowedExplicitOrigins 缓存从 YDSZ_CORS_ORIGINS 环境变量解析的精确白名单。
var allowedExplicitOrigins = func() map[string]bool {
	env := os.Getenv("YDSZ_CORS_ORIGINS")
	m := make(map[string]bool)
	if env == "" {
		return m
	}
	for _, o := range strings.Split(env, ",") {
		if t := strings.TrimSpace(o); t != "" {
			m[t] = true
		}
	}
	return m
}()

// isAllowedOrigin 校验 CORS 请求来源是否被允许。
// 1) 未设置 YDSZ_CORS_ORIGINS 时：放行所有 localhost/127.0.0.1（含任意端口），便于本地开发调试。
// 2) 已设置 YDSZ_CORS_ORIGINS 时：仅放行白名单中的精确 origin。
func isAllowedOrigin(origin string) bool {
	if len(allowedExplicitOrigins) > 0 {
		return allowedExplicitOrigins[origin]
	}
	// 未配置白名单时，额外校验 localhost 形式（防止直接放行了其他 origin）
	if localOriginPattern.MatchString(origin) {
		return true
	}
	// 兜底：校验 host 部分（无端口模式的变体）
	if u, err := url.Parse(origin); err == nil {
		if u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" {
			return true
		}
	}
	return false
}

// publicPaths 是不需要鉴权的公开路径白名单。
//
// 注意：此表不包含由 ServeStatic（r.NoRoute）处理的 SPA 路由 / 静态资产，
// 因为 ServeStatic 仅在路由未匹配时触发，中间件不会重复执行。
// 此处仅列出需要显式跳过认证中间件的 API 路径。
var publicPaths = map[string]bool{
	"/":               true,
	"/health":         true,
	"/ready":          true,
	"/api/v2/health":  true,
	"/api/v2/health/dependencies": true,
	"/metrics":        true,
	"/admin/login":    true,
	"/admin/exit":     true,
	"/admin/console":   true,
	"/client/register": true,
	"/client/ping":     true,
	"/agent/link":     true,
}

// authBlockMiddleware 对非公开 API 路径统一应用 filterAuth + csrfProtection。
//
// 处理顺序：
//  1. 公开路径直接放行（登录、健康检查、静态资源）
//  2. 非安全方法（POST/PUT/DELETE）校验 X-Requested-With 头（CSRF 防护）
//  3. 校验会话中的 username（登录鉴权）
//
// 背景：Gin 1.12 中 r.Group("") 模式会导致部分注册路由在运行时无法匹配
// （回落到 NoRoute handler），因此改为在 r.Use() 上应用路径感知的鉴权中间件。
func authBlockMiddleware(c *gin.Context) {
	if publicPaths[c.Request.URL.Path] {
		c.Next()
		return
	}

	// CSRF 防护（非安全方法）
	method := c.Request.Method
	if method != "GET" && method != "HEAD" && method != "OPTIONS" {
		if c.GetHeader("X-Requested-With") == "" {
			api.Fail(c, api.CodeForbidden, "CSRF 校验失败：缺少 X-Requested-With 请求头")
			c.Abort()
			return
		}
	}

	// 鉴权
	if username := session.GetString(c, "username"); username == "" {
		api.Fail(c, api.CodeUnauthorized, "未登录或登录已过期")
		c.Abort()
		return
	}
	c.Next()
}
