// Package routers 子文件：v2.0 API 路由注册。
//
// v2.0 核心路由清单（《云顶编码规范》§7.2）：
//   POST /api/v2/query       DSL 查询
//   POST /api/v2/query/nl    NL 转 DSL
//   POST /api/v2/semantic/search   语义检索
//   GET  /api/v2/patterns   模板列表
//   GET  /api/v2/anomalies  异常事件
//   POST /api/v2/conversations  创建诊断会话
//   GET  /api/v2/traces/{trace_id}  链路详情
//   GET  /api/v2/tail (SSE)  实时日志追踪
//
// 鉴权：所有 v2.0 接口需已登录（Session/Cookie + RBAC）。
package routers
