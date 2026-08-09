# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security

- **BREAKING**: 破坏性接口（`/client/delete`、`/client/changeStatus`、`/item/delete`、`/item/changeStatus`）从 `GET` 改为 `POST`，前端同步改为 JSON body 调用，并在服务端引入 `X-Requested-With` 自定义头 CSRF 防护中间件 (#p0)
- 前端不再明文展示客户端密钥 `vkey`（脱敏为 `******`），仅编辑回显 (#p0)
- 密码哈希算法从 SHA-256 升级为 **Argon2id**（golang.org/x/crypto/argon2，time=3/memory=64MB/threads=4），兼容旧版 SHA-256 哈希登录验证并支持平滑迁移 (#p0)
- 生产模式启动时拒绝弱口令/出厂默认密码（黑名单：`change_me_production`、`ydsz_trace_admin`、`123456` 等 10 项），避免带病上线 (#p0)
- fetch SSE 客户端（LogTail.vue）增加 `AbortController`，卸载/停止时真正断开流，防止资源泄漏；移除死代码 `eventSource` ref (#p0)
- 修复 SSE endpoint 路径错配（`/api/logs/tail` → `/logs/tail`，适配 Vite 代理与 Go 静态托管）(#p0)

### Fixed

- README 全面对齐 SQLite 单库实现：删除误导性 MySQL/PG 描述、修正 API 参考表、修正分支名（master→main）
- 修正 docker-compose.yml 中错误的环境变量名 `YDSZ_LOGS_SERVER` → `YDSZ_LOG_SERVER`

### Added

- 新增 CI 流水线 `.github/workflows/ci.yml`：多模块 Go 测试（带竞态检测）、golangci-lint 全量静态分析、govulncheck 漏洞扫描、前端构建验证、v* 标签自动发布 GHCR Docker 镜像
- 新增 `.golangci.yml` 统一静态分析配置（enabled: gofmt/govet/errcheck/staticcheck/gosec/revive/ineffassign/unused/misspell/gosimple/gocyclo）
- 新增 `docs/pull_request_template.md`（含安全检查 checklist）

## [0.1.0] - 2026-08-01

### Added

- 初始版本发布：纯 Go + Gin + SQLite 的轻量级分布式日志追踪检索系统
- 支持关键字/正则/日志级别/时间范围检索，多客户端并发搜索进度 SSE 流式推送
- 单文件 17.8GB 日志顺序读取约 33 秒（实测）
- 内置 Vue 3 + Vite + Element Plus SPA 控制台（客户端管理、日志项管理、实时跟踪、zip 导出）
- 登录速率限制（60秒5次失败→封禁300秒）、HttpOnly Cookie 会话、密码加盐哈希、路径穿越防护
- Prometheus metrics 端点（`/metrics`）暴露 QPS / 延迟 / 在线数 / 内存指标
