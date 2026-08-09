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
- **分布式架构**：每台服务器部署 Client Agent，通过 Web 控制台集中管理
- **灵活的检索模式**：
  - 关键字检索与正则匹配
  - 日志级别过滤（DEBUG/INFO/WARN/ERROR/FATAL）
  - 时间范围过滤（HH:MM:SS 格式）
  - 上下文行数控制
- **实时日志追踪**：SSE 流式推送新增日志行，支持关键字过滤
- **轻量级**：资源占用极低，无需 Elasticsearch 等重量级依赖
- **多数据库支持**：兼容 MySQL 和 PostgreSQL
- **安全加固**：密码加盐哈希、登录速率限制、Cookie 安全属性
- **可观测性**：Prometheus metrics 端点，支持 QPS/延迟/在线数监控

## 🏗️ 架构

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  logc 代理   │     │  logc 代理   │     │  logc 代理   │
│  (服务器 A)  │     │  (服务器 B)  │     │  (服务器 C)  │
│  端口 2020  │     │  端口 2020  │     │  端口 2020  │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                   │                   │
       └───────────────────┼───────────────────┘
                           │ HTTP
                    ┌──────▼──────┐
                    │  logs 服务端 │
                    │  端口 2021  │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │   数据库     │
                    │ MySQL / PG  │
                    └─────────────┘
```

## 🚀 快速开始

### 环境要求

- Go 1.17+
- MySQL 5.7+ 或 PostgreSQL 10+
- Linux / macOS / Windows

### 安装部署

```bash
# 克隆仓库
git clone https://github.com/njydsz/ydsz-trace.git
cd ydsz-trace

# 初始化数据库
# MySQL:
mysql -u root -p < sqls/mysql.sql
# PostgreSQL:
psql -U postgres -f sqls/postgresql.sql

# 修改配置
# 服务端: 编辑 logs/conf/app.conf
# 客户端: 编辑 logc/conf/app.conf

# 使用 Makefile 构建（推荐）
make all           # 构建全部模块
make run-logs      # 运行服务端
make run-logc      # 运行客户端（需另开终端）

# 或手动构建
cd logs && go build -o ../bin/logs && ./bin/logs
cd logc && go build -o ../bin/logc && ./bin/logc -s <服务端IP>:2021 -v 123456

# 前端构建（可选，默认使用构建产物）
make frontend
```

### 常用 make 目标

| 目标 | 说明 |
|------|------|
| `make all` | 构建所有 Go 模块 |
| `make deps` | 下载依赖 |
| `make test` | 运行所有测试（带竞态检测） |
| `make lint` | 静态检查（go vet） |
| `make fmt` | 代码格式化 |
| `make build` | 编译二进制到 bin/ |
| `make clean` | 清理构建产物 |
| `make run-logs` | 运行 logs 服务端 |
| `make run-logc` | 运行 logc 客户端 |
| `make frontend` | 构建前端 |
| `make ci` | CI 流水线（lint + test + build） |

## 📦 模块说明

| 模块 | 说明 | 默认端口 |
|------|------|---------|
| `logs` | 集中管理服务端，提供 Web 控制台 | 2021 |
| `logc` | 部署在目标机器上的客户端代理 | 2020 |

## 📖 API 参考

### 公开端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | 存活探针 |
| GET | `/ready` | 就绪探针 |
| GET | `/metrics` | Prometheus 指标端点 |
| POST | `/admin/login` | 登录（json: username + password） |

### 鉴权端点（需登录）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/logs/query` | 日志检索（json: client + item + date + key + line + regex? + level? + startTime? + endTime?） |
| POST | `/logs/stream` | SSE 流式搜索进度推送 |
| POST | `/logs/tail` | SSE 实时日志跟踪 |

### 检索参数说明

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| client | int | 是 | 客户端 ID（0 = 全部） |
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
| `ydsz_query_duration_ms` | gauge | 查询平均耗时 |
| `ydsz_clients_total` | gauge | 注册客户端总数 |
| `ydsz_clients_online` | gauge | 在线客户端数 |
| `ydsz_http_requests_total` | counter | HTTP 请求总数 |
| `ydsz_go_goroutines` | gauge | 当前 goroutine 数量 |

Prometheus 配置示例：

```yaml
scrape_configs:
  - job_name: 'ydsz-trace'
    static_configs:
      - targets: ['localhost:2021']
    scrape_interval: 15s
```

## 🔒 安全部署建议

### 生产环境必须项

1. **启用 HTTPS**：Cookie 的 Secure 属性要求 TLS 环境
2. **加强密码哈希**：网络通畅后迁移至 bcrypt（修改 `pkg/auth/password.go`）
3. **CORS 白名单**：设置 `YDSZ_CORS_ORIGINS` 环境变量限制跨域来源
4. **防火墙**：logc 端口（2020）仅对 logs 服务端开放
5. **Session secret**：修改 `pkg/session` 中的默认 HMAC 密钥

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `YDSZ_CORS_ORIGINS` | CORS 白名单（逗号分隔） | localhost only |
| `YDSZ_SESSION_SECRET` | Session HMAC 密钥 | 内置默认值（生产环境必须修改） |
| `YDSZ_LOGIN_RATE` | 登录接口速率限制（每分钟） | 10 |

## ⚙️ 配置说明

### 服务端 (`logs/conf/app.conf`)

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `httpport` | HTTP 监听端口 | 2021 |
| `sqlhost` | 数据库地址 | 127.0.0.1 |
| `sqlport` | 数据库端口 | 3306 |
| `sqluser` | 数据库用户 | root |
| `sqlpwd` | 数据库密码 | - |
| `database` | 数据库名称 | demo |
| `cron` | 健康检查间隔（cron 表达式） | `0 0/5 * * * *` |

### 客户端 (`logc/conf/app.conf`)

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `httpport` | HTTP 监听端口 | 2020 |
| `logs` | 服务端地址 | 127.0.0.1:2021 |
| `key` | 认证密钥 | 123456 |
| `temppath` | 临时文件存储路径 | - |

## 📊 性能测试

### 覆盖的单元测试

| 包 | 覆盖内容 |
|----|---------|
| `pkg/auth` | 密码哈希、验证、格式检测 |
| `pkg/metrics` | 指标收集、Handler、中间件 |
| `logc/controllers/file` | 安全校验、时间解析、日志行匹配 |

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
| `master` | 稳定发布分支 |
| `dev` | 开发分支，接受 PR |

### 问题反馈

提交 Issue 时请附上 Go 版本、Ydsz Trace 版本及相关依赖版本信息。

- [GitHub Issues](https://github.com/your-org/ydsz-trace/issues)

## 📄 开源协议

详见 [LICENSE](LICENSE)
