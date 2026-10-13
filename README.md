<p align="center">
  <h1 align="center">Ydsz Trace</h1>
  <p align="center">
    AI 时代的日志检索分析引擎
  </p>
  <p align="center">
    对话式工作台、自然语言查询、实时聚类压缩、异常主动检测、智能告警降噪
  </p>
</p>

<p align="center">
  <strong>当前版本：v1.x（稳定）</strong> | <strong>规划版本：v2.0-v2.4</strong> — AI 时代日志检索分析引擎
</p>

---

## 目录

- [简介](#简介)
- [核心能力](#核心能力)
- [架构蓝图](#架构蓝图)
- [目录结构](#目录结构)
- [快速开始](#快速开始)
- [部署形态](#部署形态)
- [配置说明](#配置说明)
- [API 参考](#api-参考)
- [数据模型](#数据模型)
- [Prometheus 指标](#prometheus-指标)
- [安全部署建议](#安全部署建议)
- [性能测试](#性能测试)
- [版本路线图](#版本路线图)
- [工程规范](#工程规范)
- [参与贡献](#参与贡献)
- [开源协议](#开源协议)

---

## 简介

**Ydsz Trace** 是一款基于 Go 语言开发的轻量级高性能分布式日志追踪检索系统。通过简洁的 Client-Server 架构，实现跨多台服务器节点的集中式实时日志搜索、聚合分析与智能诊断，让分布式环境下的问题排查变得简单高效。

当前 v1.x 版本提供**关键字检索、正则匹配、日志级别过滤、时间范围控制、上下文回溯、SSE 实时追踪**等核心能力，已在内网环境中稳定运行。

从 v2.0 开始，Ydsz Trace 升级为**"AI 时代的日志检索分析引擎"**，引入自然语言查询（NL2Query）、语义检索、实时模板聚类、异常检测与根因分析 Agent 等智能化能力。v2 坚定遵循"轻量 + 智能 + 私有化"的设计哲学：所有 AI 能力均可独立降级、关闭，断网环境下核心检索能力 100% 可用。

### 核心特性

**当前 v1.x 已具备**：

| 特性 | 说明 |
|------|------|
| 极致性能 | 纯 Go 编写，单线程读取 17.8GB 日志文件约 33 秒 |
| 分布式架构 | 每台服务器部署 logc Agent（端口 2020），Web 控制台集中管理 |
| 灵活检索 | 关键字检索、正则匹配、级别过滤、时间范围、上下文行数控制 |
| 实时追踪 | SSE 流式推送新增日志行，支持关键字过滤，单/多节点合并跟踪 |
| 流式进度 | 多客户端并发搜索时 SSE 上报各节点进度与结果 |
| 零依赖存储 | 内置 SQLite（modernc.org/sqlite 纯 Go 驱动 + WAL 模式），开箱即用 |
| 管理控制台 | 内置 Vue 3 + Vite + Element Plus + Tailwind CSS 构建的 SPA |
| 安全加固 | Argon2id 加盐密码哈希、登录速率限制、HttpOnly Cookie 会话、路径穿越防护、正则 ReDoS 防护 |
| 可观测性 | Prometheus metrics 端点，支持 QPS/延迟/在线数/内存等监控 |
| 三级鉴权 | viewer/operator/admin RBAC 角色模型 |

**v2.0-v2.4 规划中**：

| 特性 | 版本 | 说明 |
|------|------|------|
| NL 自然语言查询 | V2.0 | "订单服务过去一小时为什么报错变多" → 自动转 DSL |
| 模板化解析 | V2.0 | 基于 Drain 算法的增量模板挖掘与字段抽取 |
| 实时聚类压缩 | V2.0 | 同服务同期压缩比 ≥ 50:1，噪音排行榜 |
| 语义检索 | V2.1 | 基于 bge-m3 / gte-Qwen2 的向量检索，纯 CPU 推理 |
| 对话式工作台 | V2.1 | 结论 + 证据 + 建议 + 可下钻原文，多轮追问 |
| 异常主动检测 | V2.2 | 基线学习、突变检测、新增/消失模式识别 |
| 智能告警降噪 | V2.2 | 同源合并、静默窗口、维护期抑制 |
| 根因分析 Agent | V2.3 | ReAct 多步推理、三段式输出、事故简报生成 |
| MCP Server | V2.4 | 标准工具暴露，IDE 与外部 Agent 直连 |
| 多用户体系 | V2.1 | 多账号、行级权限、审计日志 |

---

## 架构蓝图

### v1.x 当前架构

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  logc 代理   │     │  logc 代理   │     │  logc 代理   │
│  (服务器 A)  │     │  (服务器 B)  │     │  (服务器 C)  │
│  端口 2020  │     │  端口 2020  │     │  端口 2020  │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                   │                   │
       └───────────────────┼───────────────────┘
                           │ HTTP（注册/心跳/查询/追踪）
                           │ SSE（实时追踪/进度推送）
                    ┌──────▼──────┐     ┌────────────────┐
                    │  logs 服务端 │────▶│  SPA 控制台    │
                    │  端口 2021  │     │  Vue 3 + Vite  │
                    │ (Gin + SQLite)│    │  + Tailwind    │
                    └─────────────┘     └────────────────┘
```

### v2 演进架构

v2 在保留 v1 Client-Server 骨架的前提下，引入**智能漏斗（L0-L3）分层架构**：

| 层级 | 职责 | 技术选型 | 是否必须 |
|------|------|---------|---------|
| L0 规则层 | 采集、多行合并、正则匹配、字段解析 | Go + Drain 算法 | 永远在线 |
| L1 统计层 | 模式计数、基线学习、突变检测、TopN | SQLite + 时序聚合 | 永远在线 |
| L2 嵌入层 | 模板向量化、HNSW 索引、语义检索 | bge-m3 / HNSW | 默认开启 |
| L3 生成层 | NL2DSL、根因推理、摘要生成 | Qwen2.5-7B-14B / 云端 API | 按需触发 |

强制约束：**日志检索主链路（L0+L1）不得依赖任何外部模型或网络连接**；L3 任一层故障不影响写入与核心检索可用性。

---

## 目录结构

```text
ydsz-trace/
├── logs/            集中式管理服务端（Gin Web + SQLite 存储）
│   └── 端口 2021 ——│ 鉴权中间件层（JWT + RBAC + RateLimit + CSRF + CORS）
│                   ├── 业务控制器（client/item/logs/task/alert/admin）
│                   ├── 数据模型（SQLite ORM via sqlx）
│                   └── 路由注册（公开/鉴权/admin 三级中间件链）
│
├── logc/            客户端代理（部署在各日志节点）
│   └── 端口 2020 ──│ 数据源抽象层（file/docker/k8s）
│                   ├── 位点管理（inode + offset 双键持久化）
│                   ├── Source Registry + Factory 插件化
│                   └── 多行合并 / 资源护栏 / 背压队列
│
├── pkg/             跨模块共享库
│   ├── api/           统一响应封装（v2 将迁移为数字业务码）
│   ├── auth/          Argon2id 密码哈希 + 自研 JWT（v2 增强 aud/nbf）
│   ├── config/        INI 配置 + 环境变量双层覆盖
│   ├── integration/   跨模块集成测试工具
│   ├── logger/        零依赖结构化日志
│   ├── logmerger/     跨节点日志合并/过滤/排序/统计
│   ├── metrics/       Prometheus 指标暴露（QPS/延迟/在线数/内存）
│   ├── ratelimit/     滑动窗口限流
│   ├── session/       可插拔会话后端（内嵌内存 + Redis）
│   ├── source/        数据源抽象层（文件/Docker/K8s）
│   └── util/          通用工具（HTTP 客户端/Zip/环境变量）
│
├── logv/            Vue 3 前端控制台
│   ├── src/api/       按域划分为 7 个模块（auth/client/item/logs/tasks/alert）
│   ├── src/views/     8 大页面（Dashboard/Clients/Items/LogSearch/LogTail/Alerts/Tasks/TraceView）
│   ├── src/store/     Pinia 状态管理
│   └── src/router/    公开 + 受保护两级路由守卫
│
├── sqls/            SQLite 初始化脚本（v1.x 5 张表）
├── deploy/          Docker Compose + K8s YAML 部署配置
├── docs/            项目文档（PRD / 编码规范 / AI 代码规约）
│   ├── Ydsz_Trace_AI时代的日志检索分析引擎_PRD_V2.1.docx
│   ├── 云顶编码规范.md
│   └── AI代码生成规约.md
├── Makefile         统一构建与运维命令（build/test/run/docker/deploy）
├── go.work          Go Workspace 多模块管理
└── docker-compose.yml
```

---

## 快速开始

### 环境要求

- **Go 1.25.5+**（项目使用 Go Workspace 管理多模块）
- **Node.js 18+**（仅构建前端时需要）
- Linux / macOS / Windows

### 方式一：本地源码运行（推荐快速体验）

```bash
# 1. 克隆仓库
git clone https://github.com/njydsz/ydsz-trace.git
cd ydsz-trace

# 2. 构建前端（首次必须；不构建时控制台首页会提示）
make frontend        # 等价于 cd logv && npm install && npm run build

# 3. 启动服务端（首次启动自动建库建表）
make run-logs        # 默认监听 2021，控制台访问 http://localhost:2021

# 4. 另开终端，启动客户端代理
make run-logc        # 默认监听 2020，自动注册到服务端并保持心跳
```

> 使用默认配置即可跑通。生产环境请通过环境变量覆盖账号密码等敏感配置（见 [配置说明](#配置说明) 与 [安全部署建议](#安全部署建议)）。

### 方式二：Docker Compose（推荐内网/信创交付）

```bash
# 构建并启动全部服务（包含 logs + logc + logv 前端）
docker compose up -d --build

# 查看运行状态
docker compose ps

# 停止并清理
docker compose down
```

控制台访问：http://localhost:2021

### 方式三：分离部署（生产推荐）

```bash
# 构建 Docker 镜像
make docker-all                    # 构建 logs 与 logc 两个镜像

# 或通过 docker-compose 分别启动不同模式
make deploy-docker-file            # 文件采集模式
make deploy-docker-docker          # Docker 容器日志采集模式

# K8s 部署
make deploy-k8s                    # kubectl apply RBAC + Deployment + DaemonSet
make remove-k8s                    # 卸载
```

### 常用 Makefile 目标

| 目标 | 说明 |
|------|------|
| `make all` | 构建所有 Go 模块（pkg/logs/logc） |
| `make deps` | 下载 Go 模块依赖 |
| `make test` | 运行全量测试（带 -race 竞态检测） |
| `make test-verbose` | 运行全量测试（逐包输出详情） |
| `make lint` | 静态检查（go vet） |
| `make lint-install` | 安装 golangci-lint（若本地不存在） |
| `make fmt` | 代码格式化（go fmt） |
| `make build` | 编译二进制到 `bin/` |
| `make run-logs` | 运行 logs 服务端 |
| `make run-logc` | 运行 logc 客户端 |
| `make run-logc-docker` | logc 以 Docker 模式运行（自动发现容器） |
| `make run-logc-k8s` | logc 以 K8s 模式运行（自动发现 Pod） |
| `make frontend` | 构建前端（输出到 `logv/dist/`） |
| `make frontend-dev` | 启动前端开发服务器（Vite HMR 热更新） |
| `make docker-logs` / `make docker-logc` | 构建单模块 Docker 镜像 |
| `make ci` | 完整 CI 流水线（lint + test + build） |
| `make deploy-docker-file/docker` | Docker Compose 部署 |
| `make deploy-k8s` / `make remove-k8s` | K8s 部署 / 卸载 |
| `make release` | 构建并拷贝产物到 `dist/` |
| `make clean` | 清理构建产物与测试缓存 |

---

## 部署形态

v2 定义三档部署形态，同一份二进制与同一套配置格式，通过切换档位配置实现。

| 档位 | 适用场景 | 存储引擎 | AI 能力档位 | 最低资源 | 部署形态 |
|------|---------|---------|-----------|---------|---------|
| **L 轻量档** | 单机 / 小团队 / 边缘节点 | SQLite + 本地段文件 | 仅 L0 + L1（AI 全关） | 2C4G | 单二进制 |
| **S 标准档** | 中小集群（≤ 200 节点） | DuckDB / 单机 ClickHouse | 本地嵌入 + 本地 7B 或云端 LLM | 8C16G | Docker Compose / K8s |
| **E 企业档** | 大型内网 / 强合规 | ClickHouse 集群 + 对象存储冷层 | 全本地 GPU（vLLM + bge-m3 + 14B/32B） | 16C64G + GPU | K8s / 专有云 |

关键档位约定：
- **L 档是基线**：AI 全部可关闭，L 档安装包不含任何模型文件；断网环境下 100% 功能可用。
- **渐进式启用**：S/E 档默认开启 L0+L1+L2（语义检索），L3（大模型推理）需管理员显式配置与授权。
- **模型完全本地化选项**：S/E 档支持管理员一键切换"纯本地推理" / "云端辅助" / "全关闭"三种模式。

---

## 配置说明

配置采用 **INI 文件 + 环境变量** 双层机制，优先级为 **环境变量 > 命令行参数 > 配置文件 > 内置默认值**。

### 服务端 (`logs/conf/app.conf`)

| 配置项 | 说明 | 默认值 | 环境变量替代 |
|--------|------|--------|-------------|
| `httpport` | HTTP 监听端口 | 2021 | — |
| `username` | 管理员账号（v2.1 起支持多用户） | admin | `YDSZ_ADMIN_USER` |
| `password` | 管理员密码（支持明文→Argon2id 平滑迁移） | change_me | `YDSZ_ADMIN_PASSWORD` |
| `dbpath` | SQLite 数据库文件路径 | ./data/ydsz.db | `YDSZ_DB_PATH` |
| `temppath` | 日志检索临时目录 | ./temp/logs/ | `YDSZ_TEMP_PATH` |
| `cron` | 客户端在线探测间隔（cron 表达式） | `0 */5 * * * *` | `YDSZ_CRON` |
| `webroot` | 前端构建产物根目录 | ./logv/dist | `YDSZ_LOGV_ROOT` |

### 客户端 (`logc/conf/app.conf`)

| 配置项 | 说明 | 默认值 | 环境变量替代 |
|--------|------|--------|-------------|
| `httpport` | HTTP 监听端口 | 2020 | — |
| `logs` | 服务端地址（ip:port） | 127.0.0.1:2021 | `YDSZ_LOG_SERVER` |
| `key` | 客户端认证密钥（预共享密钥） | 123456 | `YDSZ_CLIENT_KEY` |
| `temppath` | 临时文件存储路径 | ./temp/logc/ | `YDSZ_TEMP_PATH` |

### 关键环境变量一览

| 变量 | 适用 | 说明 | 默认值 |
|------|------|------|--------|
| `YDSZ_ADMIN_USER` | logs | 管理员用户名 | 配置文件 `username` |
| `YDSZ_ADMIN_PASSWORD` | logs | 管理员密码（生产务必使用） | 配置文件 `password` |
| `YDSZ_DB_PATH` | logs | SQLite 数据库文件路径 | 配置文件 `dbpath` |
| `YDSZ_LOGV_ROOT` | logs | 前端构建产物根目录 | `logv/dist` |
| `YDSZ_LOG_SOURCE` | logc | 日志源类型：`file` / `docker` / `k8s` | file |
| `YDSZ_LOG_SERVER` | logc | 日志服务端地址 | 配置文件 `logs` |
| `YDSZ_CLIENT_KEY` | logc | 客户端认证密钥 | 配置文件 `key` |
| `YDSZ_CORS_ORIGINS` | 两者 | CORS 允许来源（逗号分隔） | `localhost:*,127.0.0.1:*` |
| `YDSZ_SESSION_REDIS` | logs | Redis 会话后端地址（可选，不填用内存） | (空) |
| `YDSZ_RATE_LIMIT` | logs | 登录接口速率限制（次数/窗口） | `5/60s` |

### 日志源模式切换

logc 支持三种采集模式，通过配置文件或环境变量切换：

```bash
# 文件模式（默认）：采集本地日志文件
make run-logc

# Docker 模式：自动发现容器并采集 stdout/stderr
YDSZ_LOG_SOURCE=docker make run-logc-docker

# K8s 模式：自动发现 Pod 并采集容器日志
YDSZ_LOG_SOURCE=k8s make run-logc-k8s
```

---

## API 参考

v1.x 接口网关为 `/`（路由前缀 `/admin`、`/client`、`/item`、`/logs`）；v2 新增 `/api/v1`（兼容保留）与 `/api/v2`（新功能）两段前缀。详细接口规范参见 [docs/](docs/) 目录 PRD 文档。

### 响应格式

**v1.x（当前稳定）** — 采用字符串业务码：

```json
{
  "code": "200",
  "message": "success",
  "data": {},
  "traceId": "..."
}
```

**v2（规划中）** — 将迁移为数字业务码（参照字节跳动《后端对外接口规范》）：

```json
{
  "code": 0,
  "message": "success",
  "data": {},
  "trace_id": "abc123",
  "pagination": { "page": 1, "page_size": 20, "total": 156 }
}
```

> v1 响应格式将在 `/api/v1/*` 路径上至少保留一个大版本（至 v2.4+）；v2 新接口统一使用数字业务码。

### 公开端点（无需登录）

| 方法 | 路径 | 说明 | 鉴权 |
|------|------|------|------|
| GET | `/` / `/admin/console` | 控制台 SPA 入口 | 公开 |
| GET | `/health` | 存活探针（K8s） | 公开 |
| GET | `/ready` | 就绪探针（K8s） | 公开 |
| GET | `/metrics` | Prometheus 指标端点 | 公开 |
| POST | `/admin/login` | 登录（json: username + password） | 公开 |
| GET | `/admin/exit` | 退出登录 | 公开 |
| POST | `/client/register` | logc 代理注册（预共享密钥校验） | 预共享密钥 |

### 鉴权端点（需登录，Cookie: `YDSZ_SESSION`）

**客户端管理**

| 方法 | 路径 | 说明 | 鉴权等级 |
|------|------|------|---------|
| POST | `/client/add` | 新增客户端 | operator |
| POST | `/client/delete` | 删除客户端 | operator |
| POST | `/client/update` | 更新客户端 | operator |
| POST | `/client/changeStatus` | 切换启用/禁用 | operator |
| GET | `/client/query/queryAll/queryPage` | 查看详情/列表/分页 | viewer |

**日志项管理**

| 方法 | 路径 | 说明 | 鉴权等级 |
|------|------|------|---------|
| POST | `/item/add/delete/update/changeStatus` | 日志项 CRUD | operator |
| GET | `/item/query/queryAll/queryPage` | 项查询 | viewer |

**日志检索与追踪**

| 方法 | 路径 | 说明 | 鉴权等级 |
|------|------|------|---------|
| POST | `/logs/query` | 日志检索（单客户端同步 / 多客户端并发） → 返回 zip | viewer |
| POST | `/logs/stream` | SSE 流式搜索进度推送 | viewer |
| POST | `/logs/tail` | SSE 实时日志跟踪（支持单/多节点合并） | viewer |
| GET | `/logs/queryClients` | 客户端列表（下拉框数据） | viewer |
| GET | `/logs/queryItems` | 按客户端查询日志项 | viewer |
| POST | `/logs/search` | 在线分页 JSON 检索 | viewer |
| POST | `/logs/trace` | TraceID 链路聚合 | viewer |

**任务管理**

| 方法 | 路径 | 说明 | 鉴权等级 |
|------|------|------|---------|
| POST | `/logs/tasks` | 检索任务列表（按状态筛选） | viewer |
| GET | `/logs/tasks/:taskNo` | 任务详情 | viewer |
| POST | `/logs/tasks/:taskNo/retry` | 重试失败/完成的任务 | operator |
| DELETE | `/logs/tasks/:taskNo` | 删除任务 | operator |

**告警规则管理**

| 方法 | 路径 | 说明 | 鉴权等级 |
|------|------|------|---------|
| POST | `/logs/alerts/rules` | 规则 CRUD | operator |
| GET | `/logs/alerts/rules` | 规则分页列表 | viewer |
| POST | `/logs/alerts/rules/:id/toggle` | 启用/禁用规则 | operator |
| POST | `/logs/alerts/rules/:id/test` | 测试触发 | operator |
| GET | `/logs/alerts/events` | 事件投递列表 | viewer |
| DELETE | `/logs/alerts/events/:id` | 删除投递事件 | operator |
| GET | `/logs/alerts/quota` | 配额查询 | viewer |

### v2 新增接口规划

| 方法 | 路径 | 说明 | 版本 |
|------|------|------|------|
| POST | `/api/v2/query` | DSL 查询 → 聚类折叠结果 | v2.0 |
| POST | `/api/v2/query/nl` | 自然语言查询 → DSL + 解释卡 + 结果 | v2.1 |
| POST | `/api/v2/semantic/search` | 语义相似检索 | v2.1 |
| GET | `/api/v2/patterns` | 模板列表/趋势/新增标记 | v2.0 |
| GET | `/api/v2/anomalies` | 异常事件列表与详情 | v2.2 |
| POST | `/api/v2/conversations` | 创建诊断会话 | v2.3 |
| GET | `/api/v2/traces/{trace_id}` | 链路与 span 详情 | v2.3 |
| GET | `/api/v2/tail` SSE | 实时日志追踪（v1 兼容 + 模板过滤增强） | v2.0 |

---

## 数据模型

### v1.x 当前表结构

v1.x 使用 5 张 SQLite 表，通过 `models/def.go` 中的 Go 代码程序化建表（自动迁移），同时提供 `sqls/sqlite.sql` 手动初始化脚本。

| 表名 | 用途 | 关键字段 |
|------|------|---------|
| `t_client` | 客户端节点 | id、name、ip、port、status、source_type（file/docker/k8s）、tags(JSON) |
| `t_item` | 日志项 | id、client_id(FK)、path、prefix、suffix、status |
| `t_task` | 检索任务 | task_no、status、hit_count、node_count |
| `t_alert_rule` | 告警规则 | id、item_id(FK)、keyword、level、threshold、interval、webhook_url |
| `t_alert_event` | 投递记录 | id、rule_id(FK)、hit_count、http_code、sample_text |

### v2 数据模型规划

v2 采用"原始段 + 模式表 + 向量 + 元数据 + 审计"五组存储，迁移分阶段实施：

| 类别 | 引擎 | 表/实体 | 版本 |
|------|------|---------|------|
| 元数据 | SQLite | t_client、t_item、t_task、t_alert_rule、t_alert_event（继承 + 扩展）、**新增 t_user、t_audit_log、t_conversation** | V2.0-V2.4 |
| 日志主体 | 列式段文件（zstd） | log_segment、log_event、log_template、pattern_stat | V2.0 |
| 向量索引 | HNSW 内存/文件 | embedding（模板级 + 异常样本级） | V2.2 |
| AI 会话 | SQLite | conversation_msg、case_library、feedback、insight_report | V2.4 |

迁移策略：提供 v1 → v2 元数据迁移工具，支持一键回滚；列式段仅存储 v2 新写入数据，未迁移的原始日志通过原始文件降级查询。

---

## Prometheus 指标

`/metrics` 端点暴露两类指标：业务指标（带 `ydsz_` 前缀）与 Go runtime 指标。

| 指标名 | 类型 | 说明 |
|--------|------|------|
| `ydsz_uptime_seconds` | counter | 应用运行时长 |
| `ydsz_queries_total` | counter | 查询总次数 |
| `ydsz_queries_success` / `ydsz_queries_failed` | counter | 成功/失败查询次数 |
| `ydsz_query_duration_ms` | gauge | 查询平均耗时（毫秒） |
| `ydsz_clients_total` / `ydsz_clients_online` | gauge | 注册/在线客户端数 |
| `ydsz_http_requests_total` | counter | HTTP 请求总数 |
| `ydsz_http_requests_4xx` / `ydsz_http_requests_5xx` | counter | HTTP 4xx/5xx 错误数 |
| `ydsz_http_request_duration_ms` | gauge | HTTP 请求平均耗时（毫秒） |
| `ydsz_go_goroutines` | gauge | 当前 goroutine 数量 |
| `ydsz_go_mem_alloc_bytes` / `ydsz_go_mem_sys_bytes` | gauge | 堆分配 / 系统申请内存 |
| `ydsz_go_gc_total` | counter | GC 总次数 |

**v2 新增指标（规划中）**：`ydsz_ai_tokens_total`、`ydsz_ai_llm_duration_seconds`、`ydsz_ai_anomalies_total`、`ydsz_index_vectors_total`、`ydsz_parser_templates_total` 等。

Grafana 配置示例：

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'ydsz-trace'
    static_configs:
      - targets: ['localhost:2021']
    scrape_interval: 15s
```

---

## 安全部署建议

### v1.x 当前安全基线（已实现）

1. **口令安全**：Argon2id 加盐哈希（OWASP 推荐参数 time=3, memory=64MB, threads=4），支持旧版 SHA-256 与新账户明文密码自动迁移；使用 `crypto/subtle.ConstantTimeCompare` 防时序攻击。
2. **HTTPS 强制**：会话 Cookie 默认启用 `HttpOnly` + `Secure` + `SameSite=Lax`，要求 TLS 终端；生产部署请使用反向代理（Nginx/Traefik）或 K8s Ingress 终止 TLS。
3. **登录速率限制**：单 IP 滑动窗口（默认 60 秒 5 次失败封禁 300 秒），支持切换为 Redis 后端（多副本场景）。
4. **CORS 白名单**：通过 `YDSZ_CORS_ORIGINS` 环境变量精确控制跨域来源，避免任意网站携带 Cookie 请求接口。
5. **最小权限原则**：RBAC 三级角色（viewer/operator/admin），logc 端口（2020）仅对 logs 服务端开放。
6. **输入防护**：路径遍历检测（`..` 字符串 + rootDir 沙箱）、正则 ReDoS 防护（可打印 ASCII + 长度限制）。

### v2 增强目标（规划中）

1. **JWT 安全升级**：补齐 `aud`、`nbf` 标准字段验证，RefreshToken 刷新时重新校验签名（V2.1）。
2. **Agent 注册鉴权**：`/client/register` 增加预共享 Token 或 mTLS 双向认证，防止恶意节点注册（V2.0）。
3. **字段级脱敏**：内置手机号、身份证、银行卡、JWT、密码、API Key 规则，区分"展示脱敏"与"外发脱敏"两套策略（V2.0）。
4. **AI 会话全量审计**：NL 查询、工具调用、结论输出全留痕，可回溯可导出（V2.2）。
5. **API 响应格式统一**：消除 String/Number 业务码双风格，全项目走统一响应中间件 + 静态 Lint 门禁（V2.0）。
6. **SSE 请求安全封装**：复用 axios 拦截器逻辑处理 CSRF 注入与 401 自动跳转，避免绕过安全中间件（V2.0）。
7. **渗透测试门禁**：每个版本发布前通过自动化 + 人工渗透测试，安全缺陷阻断发布（V2.0 CI）。

---

## 性能测试

### 硬件环境

| 配置 | 参数 |
|------|------|
| CPU | Intel Core i5-10210U @ 1.60GHz × 8 |
| 内存 | 16 GB |
| 硬盘 | 512.1 GB SSD |
| 操作系统 | Ubuntu 20.04.2 LTS (64位) |

### 单线程顺序读取 17.8GB 日志文件（v1.x 基线）

| 语言 | 第1次 | 第2次 | 第3次 | 第4次 | 第5次 | 总耗时 | 平均耗时 |
|------|-------|-------|-------|-------|-------|--------|----------|
| **Go（本项目）** | 32.99s | 34.24s | 30.33s | 31.21s | 35.70s | 164.16s | **32.83s** |
| Python | 超时未完成 | — | — | — | — | — | — |
| Java | 226s | 206s | 153s | 219s | 183s | 987s | 197.4s |

> 此基准测试反映 v1.x 纯文件读取性能。v2 引入列式段存储后目标压缩比 ≥ 8:1，写入吞吐 ≥ 10 万行/秒。

### v2 性能目标（规划中）

| 指标 | 目标 | 口径 |
|------|------|------|
| Agent 采集与解析吞吐 | ≥ 50,000 行/秒 | 单 Agent，CPU ≤ 1 核 |
| 服务端写入吞吐 | ≥ 100,000 行/秒 | 标准档，批量提交 |
| 关键词查询 P95 | ≤ 2 秒 | 10 亿行规模 |
| 语义检索 P95 | ≤ 1 秒 | 100 万模板向量 |
| Agent 常驻内存 | ≤ 50 MB | 默认配置 |
| 轻量档整机资源 | ≤ 2C4G | AI 全关，50 节点 |

---

## 版本路线图

详细规划参见 [docs/Ydsz_Trace_AI时代的日志检索分析引擎_PRD_V2.1.md](docs/Ydsz_Trace_AI时代的日志检索分析引擎_PRD_V2.1.md)，以下为版本主题与核心范围摘要：

| 版本 | 主题 | 核心范围 | 关键里程碑 | 状态 |
|------|------|---------|-----------|------|
| **V2.0** | 语义基座 | 采集增强、模板解析（Drain）、列式段存储、统一 DSL、实时聚类、经典视图保留、v1 迁移工具 | 存储引擎切换、解析准确率 ≥ 95%、v1 数据无损迁移 | 规划中 |
| **V2.1** | 智能检索 + 工程规范 | NL2Query、语义检索、对话工作台、JWT 安全增强、多用户基础、响应中间件统一、SSE 安全封装 | NL 评测集 ≥ 90%、安全规范 100% 达标 | 规划中 |
| **V2.2** | 主动洞察 | 异常检测、智能告警、洞察看板、模式摘要 | 20 例故障演练召回 ≥ 80%、告警渠道全连通 | 规划中 |
| **V2.3** | 诊断 Agent | 根因分析 Agent、链路视图、事故简报、反馈回流 | 30 例复盘根因命中 ≥ 60%、链路视图可用 | 规划中 |
| **V2.4** | 生态开放 | MCP Server、多语言 SDK、数据生命周期治理、小屏适配 | MCP 客户端可直连、SDK 三语言发布 | 规划中 |

每版本交付约束：必须通过对应评测门禁；发布前必须提供 v1 兼容回归报告与回滚方案。

---

## 工程规范

### 代码质量目标

| 维度 | v1.x 当前 | v2 目标 |
|------|-----------|---------|
| Go 单元测试覆盖率 | 核心包 ~60% | ≥ 80% |
| 前端核心组件测试覆盖 | ~2%（仅 auth store） | ≥ 70% |
| E2E 集成测试覆盖 | 0 | ≥ 5 条核心链路 |
| API 文档完整度 | 无 | 100% OpenAPI 3.0 |
| 响应格式合规率 | String/Number 双风格并存 | 100% 数字码 |
| Lint 通过率 | 99% | 100% |

### 测试命令

```bash
# 运行全部 Go 测试（带 -race 竞态检测）
make test

# 前端测试
cd logv && npm run test

# 前端 E2E 测试（v2 引入 Playwright）
cd logv && npx playwright test

# 完整 CI
make ci
```

### 分支规范

| 分支 | 用途 | 合并规则 |
|------|------|---------|
| `main` | 稳定发布分支 | 接受 feature PR + 至少 1 人 CR |
| `develop` | v2 开发主干 | 各版本 feature 分支合入 |
| `feature/xxx` | 功能开发 | 从 develop 派生，开发完成后 PR 回 develop |
| `release/v2.x` | 版本发布 | 从 develop 派生，仅接受 bugfix |
| `hotfix/xxx` | 紧急修复 | 从 main 派生，修复后 cherry-pick 回 develop |

### 提交信息规范

遵循 Conventional Commits + Emoji 前缀（参考字节跳动/阿里提交规范）：

```
✨ feat(parser): 新增 Drain 模板解析引擎
🐛 fix(auth): 修复 JWT 缺少 aud 字段验证
📝 docs(readme): 更新 V2 部署形态章节
♻️ refactor(api): 统一响应中间件迁移为数字业务码
✅ test(logs): 新增日志检索 E2E 测试
⚡ perf(cache): 热点查询结果缓存性能优化
🔒 security(session): 增强 SSE 请求 401 自动跳转
```

### 编码规范与文化

项目编码规范详见 [docs/云顶编码规范.md](docs/云顶编码规范.md)，AI 代码生成规约详见 [docs/AI代码生成规约.md](docs/AI代码生成规约.md)。核心原则：

- **先跑通测试，再合并代码**：测试门禁是硬性要求，CI 不通过禁止合并。
- **小步迭代**：单次 PR 不超过 500 行（LOC），超过需拆分或特别说明。
- **语义化提交**：每次 commit 只做一件事，描述清晰。
- **安全与性能并重**：日志系统是基础组件，任何变更必须兼顾安全与性能影响。
- **文档先行**：新功能必须先更新 README / API 文档 / PRD 再写代码。

---

## 参与贡献

### 前置条件

- Go 1.25.5+
- Node.js 18+（仅前端）
- Git 2.30+

### 贡献流程

1. Fork 本仓库
2. 从 `develop` 创建 feature 分支
3. 开发 + 单元测试（覆盖率门禁）
4. 提交 PR 到 `develop` 分支（触发 CI：lint + test + build）
5. 至少 1 位维护者 Code Review
6. CI 通过 + CR 通过 → squash merge 至 develop

### Issue 反馈

提交 Issue 时请附上：

- Ydsz Trace 版本（v1.x / v2.x）、Go 版本、Node.js 版本
- 问题描述或功能请求（含复现步骤）
- 相关日志或截图

相关资源：

- [PRD V2.1 文档](docs/Ydsz_Trace_AI时代的日志检索分析引擎_PRD_V2.1.docx) — 完整产品需求
- [编码规范](docs/云顶编码规范.md) — 项目约定
- [AI 代码生成规约](docs/AI代码生成规约.md) — 规约与约束

---

## 开源协议

详见根目录 [LICENSE](LICENSE)。
