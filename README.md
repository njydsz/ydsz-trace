<p align="center">
  <h1 align="center">Ydsz Trace</h1>
  <p align="center">
    轻量级高性能分布式日志追踪与检索系统
  </p>
    <p align="center">
    比 grep 快、比 ELK 轻、比 Loki 更适合内网
  </p>
</p>

## 📖 简介

**Ydsz Trace** 是一款基于 Go 语言开发的轻量级高性能分布式日志追踪检索系统。通过简洁的 Client-Server 架构，实现跨多台服务器节点的集中式实时日志搜索，让分布式环境下的问题排查变得简单高效。

### 核心特性

- **极致性能**：纯 Go 编写，单线程读取 17.8GB 日志文件仅需约 33 秒
- **分布式架构**：每台服务器部署 Client Agent（logc），通过 Web 控制台集中管理
- **灵活的检索模式**：
  - 关键字检索与正则匹配
  - 日志级别过滤（DEBUG/INFO/WARN/ERROR/FATAL）
  - 时间范围过滤（HH:MM:SS 格式）
  - 上下文行数控制
- **实时日志追踪**：SSE 流式推送新增日志行，支持关键字过滤，单/多节点合并跟踪
- **流式进度推送**：多客户端并发搜索时 SSE 实时上报各节点进度与结果
- **零外部依赖存储**：内置 SQLite（纯 Go 驱动，WAL 模式），开箱即用，无需额外数据库服务
- **开箱即用的管理控制台**：内置 Vue 3 + Vite 构建的 SPA，由服务端直接托管
- **安全加固**：加盐密码哈希、登录速率限制、HttpOnly Cookie 会话、路径穿越防护、正则 ReDoS 防护
- **可观测性**：Prometheus metrics 端点，支持 QPS/延迟/在线数/内存等监控

## 🏗️ 架构

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  logc 代理   │     │  logc 代理   │     │  logc 代理   │
│  (服务器 A)  │     │  (服务器 B)  │     │  (服务器 C)  │
│  端口 2020  │     │  端口 2020  │     │  端口 2020  │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                   │                   │
       └───────────────────┼───────────────────┘
                           │ HTTP（注册/心跳/查询/追踪）
                    ┌──────▼──────┐
                    │  logs 服务端 │
                    │  端口 2021  │  ┌────────────────┐
                    │  (Gin + SQLite)│──── SPA 控制台 │
                    └──────────────┘  └────────────────┘
```

**技术栈**：Go 1.26+（Go Workspace 多模块）、Gin Web 框架、SQLite（modernc.org/sqlite 纯 Go 驱动 + jmoiron/sqlx ORM）、Vue 3 + Vite + Element Plus + Tailwind CSS。

## 📦 目录结构

| 模块 | 说明 | 默认端口 |
|------|------|---------|
| `logs` | 集中管理服务端（Gin），提供 Web 控制台，内置 SQLite 存储 | 2021 |
| `logc` | 部署在目标机器上的客户端代理，负责读取本机日志文件 | 2020 |
| `pkg` | 跨模块共享库（配置/会话/鉴权/指标/聚合/工具） | - |
| `web` | 前端控制台（Vue 3 + Vite + Element Plus），构建产物由 logs 托管 | - |
| `sqls` | SQLite 数据库初始化脚本（服务端首次启动自动建表） | - |

`pkg` 共享库包含：`config`（INI 配置解析 + 环境变量覆盖）、`session`（内存会话）、`auth`（密码哈希）、`metrics`（Prometheus 指标）、`logmerger`（跨节点日志聚合/过滤/排序/统计）、`util`（zip 压缩解压/临时文件清理/HTTP 客户端）、`api`（统一响应封装）、`logger`（零依赖结构化日志）。

## 🚀 快速开始

### 环境要求

- Go 1.26+（项目使用 Go Workspace 管理多模块）
- Node.js 18+（仅构建前端时需要）
- Linux / macOS / Windows

### 方式一：本地源码运行（推荐）

```bash
# 克隆仓库
git clone https://github.com/njydsz/ydsz-trace.git
cd ydsz-trace

# 构建前端（可选，不构建时控制台会提示）
make frontend        # 等价于 cd web && npm install && npm run build

# 启动服务端（首次启动自动创建 SQLite 数据库并建表）
make run-logs        # 默认监听 2021，控制台访问 http://localhost:2021

# 另开终端，启动客户端代理
make run-logc        # 默认监听 2020，自动向服务端注册并保持心跳
```

> 使用默认配置即可跑通。生产环境请通过环境变量覆盖账号密码等敏感配置（见下文）。

### 方式二：Docker Compose（推荐内网/信创交付）

```bash
docker compose up -d --build
# 访问 http://localhost:2021
```

### 常用 make 目标

| 目标 | 说明 |
|------|------|
| `make all` | 构建所有 Go 模块 |
| `make deps` | 下载依赖 |
| `make test` | 运行所有测试（带竞态检测与覆盖率） |
| `make lint` | 静态检查（go vet） |
| `make fmt` | 代码格式化 |
| `make build` | 编译二进制到 bin/ |
| `make run-logs` | 运行 logs 服务端 |
| `make run-logc` | 运行 logc 客户端 |
| `make frontend` | 构建前端（web/dist） |
| `make frontend-dev` | 启动前端开发服务器（Vite HMR） |
| `make ci` | CI 流水线（lint + test + build） |
| `make docker-logs` / `make docker-logc` | 构建单模块 Docker 镜像 |

## ⚙️ 配置说明

配置采用 **INI 文件 + 环境变量** 双层机制，优先级为 **环境变量 > 配置文件 > 内置默认值**。

### 服务端 (`logs/conf/app.conf`)

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `httpport` | HTTP 监听端口 | 2021 |
| `username` / `password` | 管理员账号密码 | admin / change_me_production |
| `dbpath` | SQLite 数据库文件路径 | ./data/ydsz_trace.db |
| `temppath` | 日志查询临时目录 | ./temp/logs/ |
| `cron` | 客户端在线探测间隔（cron 表达式） | `0 0/5 * * * *` |

### 客户端 (`logc/conf/app.conf`)

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `httpport` | HTTP 监听端口 | 2020 |
| `logs` | 服务端地址（ip:port） | 127.0.0.1:2021 |
| `key` | 客户端认证密钥（预共享密钥） | 123456 |
| `temppath` | 临时文件存储路径 | ./temp/logc/ |

### 环境变量

| 变量 | 适用 | 说明 | 默认值 |
|------|------|------|--------|
| `YDSZ_ADMIN_USER` | logs | 管理员用户名 | 配置文件 `username` |
| `YDSZ_ADMIN_PASSWORD` | logs | 管理员密码（生产务必使用） | 配置文件 `password` |
| `YDSZ_DB_PATH` | logs | SQLite 数据库文件路径 | 配置文件 `dbpath` |
| `YDSZ_WEB_ROOT` | logs | 前端构建产物根目录 | `web/dist` |
| `YDSZ_LOG_SERVER` | logc | 日志服务端地址 | 配置文件 `logs` |
| `YDSZ_CLIENT_KEY` | logc | 客户端认证密钥 | 配置文件 `key` |
| `YDSZ_CORS_ORIGINS` | 两者 | CORS 白名单（逗号分隔） | localhost:* / 127.0.0.1:* |

logc 命令行参数（优先级高于环境变量）：`-s <ip:port>` 指定服务端、`-v <key>` 指定认证密钥、`-c <path>` 指定配置文件。

## 📖 API 参考

统一响应格式：`{ "code": "200", "message": "...", "data": ..., "traceId": "..." }`。所有业务接口均需登录（除标注公开者外），登录态通过 HttpOnly Cookie `YDSZ_SESSION` 传递。

### 公开端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/` / `/admin/console` | 控制台 SPA 入口 |
| GET | `/health` | 存活探针 |
| GET | `/ready` | 就绪探针 |
| GET | `/metrics` | Prometheus 指标端点 |
| POST | `/admin/login` | 登录（json: username + password） |
| GET | `/admin/exit` | 退出登录 |
| POST | `/client/register` | logc 代理注册接口（预共享密钥校验） |

### 鉴权端点（需登录）

**客户端管理**

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/client/add` | 新增客户端 |
| POST | `/client/delete` | 删除客户端（json: id） |
| POST | `/client/update` | 更新客户端 |
| POST | `/client/changeStatus` | 切换客户端启用/禁用（json: id, status） |
| GET | `/client/query` / `/client/queryAll` / `/client/queryPage` | 客户端查询（详情/列表/分页） |

**日志项管理**

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/item/add` | 新增日志项 |
| POST | `/item/delete` | 删除日志项（json: id） |
| POST | `/item/update` | 更新日志项 |
| POST | `/item/changeStatus` | 切换日志项启用/禁用（json: id, status） |
| GET | `/item/query` / `/item/queryAll` / `/item/queryPage` | 日志项查询 |

**日志检索与追踪**

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/logs/query` | 日志检索（单客户端同步 / 多客户端并发，返回 zip） |
| POST | `/logs/stream` | SSE 流式搜索进度推送 |
| POST | `/logs/tail` | SSE 实时日志跟踪（单节点代理 / 多节点合并） |
| GET | `/logs/queryClients` | 查询客户端列表（检索页下拉框） |
| GET | `/logs/queryItems` | 按 client_id 查询日志项（?client_id=） |

### 检索参数说明（`/logs/query`、`/logs/stream`）

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| client | int | 是 | 客户端 ID（0 = 全部并发检索） |
| item | int | 是 | 日志项 ID |
| date | string | 是 | 日期（YYYYMMDD 格式） |
| key | string | 是 | 关键词/正则表达式 |
| line | int | 是 | 上下文行数（0-500） |
| regex | bool | 否 | 启用正则匹配（默认 false） |
| level | string | 否 | 日志级别过滤（DEBUG/INFO/WARN/ERROR/FATAL） |
| startTime | string | 否 | 时间范围起始（HH:MM:SS） |
| endTime | string | 否 | 时间范围结束（HH:MM:SS） |

## 📊 Prometheus 指标

`/metrics` 端点暴露以下指标：

| 指标名 | 类型 | 说明 |
|--------|------|------|
| `ydsz_uptime_seconds` | counter | 应用运行时长 |
| `ydsz_queries_total` | counter | 查询总次数 |
| `ydsz_queries_success` / `ydsz_queries_failed` | counter | 成功/失败查询次数 |
| `ydsz_query_duration_ms` | gauge | 查询平均耗时 |
| `ydsz_clients_total` / `ydsz_clients_online` | gauge | 注册/在线客户端数 |
| `ydsz_http_requests_total` | counter | HTTP 请求总数 |
| `ydsz_http_requests_4xx` / `ydsz_http_requests_5xx` | counter | HTTP 4xx/5xx 错误数 |
| `ydsz_http_request_duration_ms` | gauge | HTTP 请求平均耗时 |
| `ydsz_go_goroutines` / `ydsz_go_mem_alloc_bytes` | gauge | 运行时 goroutine 数/堆分配 |
| `ydsz_go_mem_sys_bytes` / `ydsz_go_gc_total` | gauge/counter | 系统内存/GC 次数 |

Prometheus 配置示例：

```yaml
scrape_configs:
  - job_name: 'ydsz-trace'
    static_configs:
      - targets: ['localhost:2021']
    scrape_interval: 15s
```

## 🔒 安全部署建议

1. **启用 HTTPS**：会话 Cookie 默认 `Secure` 属性，要求 TLS 环境
2. **使用强密码**：通过 `YDSZ_ADMIN_PASSWORD` 注入强密码；密码支持明文→哈希平滑迁移，首次登录后按日志提示将哈希值更新到配置
3. **修改默认密钥**：logc 认证密钥（`YDSZ_CLIENT_KEY`）与服务端默认账号密码必须在生产环境修改；生产模式下若检测到默认配置，服务将拒绝启动
4. **CORS 白名单**：设置 `YDSZ_CORS_ORIGINS` 限制跨域来源
5. **防火墙**：logc 端口（2020）仅对 logs 服务端开放
6. **会话存储**：当前为进程内内存会话（重启失效），多副本部署时需替换为 Redis 等外部后端

## 📊 性能测试

### 覆盖的单元测试

| 包 | 覆盖内容 |
|----|---------|
| `pkg/auth` | 密码哈希、验证、格式检测 |
| `pkg/config` | 配置解析、环境变量覆盖 |
| `pkg/metrics` | 指标收集、Handler、中间件 |
| `pkg/session` | 会话创建、读写、销毁 |
| `pkg/util` | 环境变量、zip 压缩解压 |
| `pkg/api` | 统一响应封装 |
| `pkg/integration` | 跨模块集成测试 |
| `logc/controllers/file` | 安全校验、时间解析、日志行匹配 |
| `logs/e2e` | 服务端端到端测试 |

运行测试：`make test`

### 硬件环境

| 配置 | 参数 |
|------|------|
| CPU | Intel Core i5-10210U @ 1.60GHz × 8 |
| 内存 | 16 GB |
| 硬盘 | 512.1 GB SSD |
| 操作系统 | Ubuntu 20.04.2 LTS (64位) |

### 性能对比：17.8GB 日志文件单行顺序读取（单线程）

| 语言 | 第1次 | 第2次 | 第3次 | 第4次 | 第5次 | 总耗时 | 平均耗时 |
|------|-------|-------|-------|-------|-------|--------|----------|
| **Go** | 32.99s | 34.24s | 30.33s | 31.21s | 35.70s | 164.16s | **32.83s** |
| Python | 超时未完成 | - | - | - | - | - | - |
| Java | 226s | 206s | 153s | 219s | 183s | 987s | 197.4s |

## 🤝 参与贡献

### 分支策略

| 分支 | 用途 |
|------|------|
| `main` | 稳定发布分支（默认），接受 PR |

### 问题反馈

提交 Issue 时请附上 Go 版本、Ydsz Trace 版本及相关依赖版本信息。

- [GitHub Issues](https://github.com/your-org/ydsz-trace/issues)

## 📄 开源协议

详见 [LICENSE](LICENSE)