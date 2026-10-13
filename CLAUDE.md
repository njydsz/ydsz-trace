# CLAUDE.md — Claude Code 项目入口

> **跨工具通用入口**：本文件同时被 Claude Code、Cursor、GitHub Copilot 读取，作为 AI 协作的统一上下文。

## 项目概述

云顶追踪（Ydsz Trace）— Go + Vue 3 的日志检索分析引擎 v2.0，AI 驱动的日志排障定位平台。

## 技术栈

- **后端**: Go 1.26.5 + Gin 1.12 + sqlx + SQLite (modernc.org)
- **前端**: Vue 3.5 + Vite 6 + Element Plus 2.9 + Tailwind CSS 3.4
- **状态**: Pinia 2.3（组合式 setup store，**必须使用 `useXxxStorage` 命名**）
- **HTTP**: Axios 1.7（统一实例 `logv/src/api/http.js`）
- **构建**: Makefile (build/test/lint/fmt/ci)
- **质量**: golangci-lint + gofmt + ESLint + Prettier

## 模块结构

```
pkg/        公共库（无业务依赖，不得依赖 logs/logc）
logs/       服务端（端口 2021）
logc/       客户端代理（端口 2020）
mcp/        v2.0 MCP Server 入口（待实现）
logv/       Vue 3 前端控制台（对话式工作台为 v2.0 默认首页）
sqls/       DDL 脚本
deploy/     Docker/K8s 配置
```

## 核心编码约定

### Store 命名
- Pinia 组合式 store 导出名以 `Storage` 后缀：`useAuthStorage`、`useThemeStorage`
- 兼容包装函数导出为 `useAuth()`、`useTheme()`
- 文件命名：`auth.js`、`theme.js`（允许不含 Storage 后缀）

### 后端命名
- 数据库结构体：v1 用 `T` 前缀（`TClient`、`TItem`）；v2.0 用 `Log` 前缀（`LogSegment`、`LogTemplate`）
- 接口：`er/able/ior` 结尾（`Source`、`Backend`、`Limiter`、`Provider`、`Tool`）
- 请求/响应：`Req`/`Resp`/`DTO` 后缀（`SearchReq`、`LogcSearchResp`）
- 常量：`UPPER_SNAKE_CASE`
- JSON tag: camelCase；DB tag: snake_case（必须成对出现）

### 目录职责
- `controllers/` → 请求编排，**禁止 SQL 拼接**
- `models/` → ORM CRUD，**禁止调用 gin 上下文**
- `routers/` → 路由注册 + 中间件，**禁止业务逻辑**

### AI 系统函数前缀（v2.0）
- L0 规则层：`rule_`、`match_`、`filter_`
- L1 统计层：`stats_`、`baseline_`、`detect_`
- L2 嵌入层：`embed_`、`cluster_`、`similar_`
- L3 生成层：`nl2dsl_`、`reasoning_`、`summarize_`

### 环境变量
- 前缀统一 `YDSZ_`
- 敏感值**必须**通过环境变量注入，禁止硬编码

## 安全底线（不可违反）

1. SQL 必须使用 `?` 参数化查询
2. 文件路径输入必须经 `pkg/sanitize` 白名单过滤
3. 密码存储使用 Argon2id
4. JWT 禁用 `none` 算法
5. API 响应不暴露内部堆栈
6. **v2.0 数据不出内网**: L0/L1/L2 层禁止调用远程模型；云端外发必须脱敏

## 验证命令

```bash
make build              # 编译全部模块
make test               # 运行测试
make lint               # golangci-lint + go vet
make ci                 # lint + test + build
cd logv && npm run build # 前端构建
cd logv && npm run lint  # ESLint
```

## 详细规范

完整条款请参阅：
- `docs/云顶编码规范.md` — 人类与 AI 共同遵守的强制性编码公约
- `docs/AI代码生成规约.md` — AI 工具运行的上下文协议与行为约束
- `docs/ai-rules/P0-blockers.md` — P0 不可违反规则（精简版）
- `docs/ai-rules/anti-patterns.md` — 反模式清单
- `docs/ai-rules/project-context.md` — 项目上下文描述
