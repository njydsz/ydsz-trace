// Package routers 子文件：v2.0 API 路由注册。
//
// v2.0 核心路由清单（《云顶编码规范》§7.2）：
//   POST /api/v2/semantic/search   语义检索（NL2DSL + embedding）
//   POST /api/v2/conversations      创建诊断会话
//   GET  /api/v2/traces/{trace_id}  链路详情（复用 waterfall 逻辑）
//
// 鉴权：所有 v2.0 接口需已登录（Session/Cookie + RBAC）。
package routers

import (
	"github.com/gin-gonic/gin"
	"ydsz-trace/logs/controllers/logs"
	pkgauth "ydsz-trace/pkg/auth"
	"ydsz-trace/pkg/api"
)

// RequireAuth v2 路由鉴权中间件。
//
// Session 鉴权由来全局 authBlockMiddleware（router.go）保证，此处作为
// 显式声明保留，确保 v2 路由组语义自洽；实际执行 c.Next() 将控制权交给
// RequireOperator() 与后续处理器。
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// RequireOperator 要求 operator 及以上角色（admin 也满足）。
//
// 委托 pkgauth.RequireOperator() = RequireRole(RoleOperator)，与 router.go
// 中的既有 RBAC 策略保持一致。
func RequireOperator() gin.HandlerFunc {
	return pkgauth.RequireOperator()
}

// RegisterV2Routes 注册 v2.0 API 路由组。
//
// 路由前缀 /api/v2，统一 RequireAuth + RequireOperator 鉴权。
// 语义检索：POST /api/v2/semantic/search
// NL2DSL：POST /api/v2/nl2dsl/parse（当前为纯规则实现，LLM 兜底待 V2.3）
// 诊断会话：POST /api/v2/conversations（占位，Agent 循环待 V2.3 实现）
// 链路详情：GET  /api/v2/traces/{traceId}（复用 waterfall 逻辑）
func RegisterV2Routes(r *gin.Engine) {
	v2 := r.Group("/api/v2")
	v2.Use(RequireAuth(), RequireOperator())
	{
		// NL2DSL 语义检索入口
		v2.POST("/semantic/search", logs.SemanticSearch)

		// NL2DSL 解析（V2.1 纯规则实现；LLM 兜底增强待 V2.3）
		v2.POST("/nl2dsl/parse", logs.Nl2DslParse)

		// 日志查询（在线分页搜索，多客户端并发）
		v2.POST("/query", logs.Search)

		// 链路追踪：瀑布视图
		v2.POST("/traces/waterfall", logs.TraceWaterfall)

		// TODO: logs.ContextPreview 尚未实现，待 PRD 细化后补充
		// v2.POST("/context", logs.ContextPreview)

		// TODO: logs.DashboardInsight 尚未实现
		//       现有导出函数为 DashboardInsightHandler（controllers/logs/dashboard.go），
		//       如需接入，请在此重命名或新增包装函数后再取消注释。
		// v2.GET("/dashboard/insight", logs.DashboardInsight)

		// 诊断会话（FR-9 根因 Agent）— V2.1 未实现，返回明确的未实现标识。
		//
		// 不使用 Success 包装，避免前端误以为会话已创建；
		// V2.3 接入 ReAct 循环后替换为真实实现。
		v2.POST("/conversations", func(c *gin.Context) {
			api.Fail(c, api.CodeNotImplemented, "诊断会话尚未实现，计划于 V2.3 版本提供")
		})
	}
}
