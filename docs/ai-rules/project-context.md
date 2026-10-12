# 项目上下文（AI 注入用）

> 每次 AI 请求注入此文件的精炼版本（≤ 50 行）作为全局基线上下文。

## 工程形态

Go Workspace 三模块（`go.work`）+ 独立 Vite 前端工程。
三模块：`pkg`（公共库）、`logs`：服务端（端口 2021）、`logc`：客户端代理（端口 2020）。
前端：Vue 3 + Vite + Element Plus + Pinia，路径别名 `@` → `web/src/`。

## 入口文件

- `logs/main.go` → 服务端启动
- `logc/main.go` → 客户端启动
- `web/src/main.js` → 前端挂载

## 关键封装

- 统一响应：`pkg/api` 的 `Success` / `Fail` / `Paginated`
- 配置读取：`cfg.String("key")` / `cfg.Int("key")` / `cfg.Bool("key")`
- 结构化日志：`pkg/logger`（基于 slog JSON）；局部可用 `log.Printf` 但新代码优先 slog
- 限流：`pkg/ratelimit`（内存/Redis 可插拔）
- 脱敏：`pkg/sanitize`（文件名/日期白名单）→ **v2.0 升级至 `pkg/mask`**

## 统一响应格式

```json
{ "code": "200", "message": "success", "data": { ... } }
```

code 为字符串，业务成功：`"200"`；前端拦截器已兼容字符串/数字双态。

## 前端 API 层

- 统一 Axios 实例：`web/src/api/http.js`
- 拦截器职责：仅处理 HTTP 层（401/403/5xx）；业务 `code` 非 200 时 Toast warning 但不 reject
- API 模块按业务域拆分：`logs.js`、`client.js`、`item.js`、`auth.js`、`tasks.js`、`alert.js`
- 路径规范：v1 `/client/add`, `/client/queryPage`；v2.0 `/api/v2/*`（待实现）

## 已知陷阱

1. gin 1.12（非最新版），API 签名以项目实际为准
2. `modernc.org/sqlite`（纯 Go，非 CGO）
3. DB 结构体：v1 用 `T` 前缀（`TClient`），v2.0 用 `Log` 前缀
4. 包级 DB 句柄使用 `models.DB`，不新建连接
5. Pinia store 导出名 `useXxxStorage`，包装函数 `useXxx()`
6. localStorage key 固定为 `ydsz_username`（auth）、`ydsz-theme-preference`（theme）
7. 路径别名 `@` → `web/src/`，构建失败先检查 import 路径
8. 前端 `v-model` 直接绑定搜索关键词也需 debounce（300ms）

## 不要使用的 API/模式

- ❌ `c.QueryInt64()` — gin 0.x 不存在，项目有封装
- ❌ `gorm` — 项目使用 `sqlx`
- ❌ 直接操作 `document.cookie` — 会话用 HttpOnly Cookie
- ❌ 在 `v-for` 中使用函数调用做过滤 — 应使用 `computed`
