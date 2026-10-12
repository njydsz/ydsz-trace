# P0 不可违反规则（Blocker）

> **适用范围**：所有 AI 辅助生成的代码。违反以下规则的产出必须回滚或重做。
> **触发文件**：`**/*.go`、`**/*.vue`、`**/*.js`、`**/*.ts`

## P0-01: 安全底线

- 禁止在代码中硬编码密码、密钥、Token
- 禁止使用 MD5/SHA1 存储密码（必须 Argon2id）
- SQL 必须使用参数化查询（`?` 占位符），禁止字符串拼接
- 文件路径输入必须经 `pkg/sanitize.SafeFilename` 白名单过滤
- JWT 不得使用 `none` 算法

## P0-02: 模块边界

- 禁止反向依赖：`pkg` 不得依赖 `logs`/`logc`；`logs` 与 `logc` 互不可依赖
- 禁止前端直接引用 Go 模块
- 禁止跨层调用：Controller → Model → DB（不得绕过 Model 层直接操作 DB）

## P0-03: 响应格式

- 所有 API 必须使用 `pkg/api` 的 `Success` / `Fail` / `Paginated`
- 禁止直接 `c.JSON` / `c.Data` 返回非标准格式
- 禁止 HTTP handler 返回裸 `error`

## P0-04: 错误处理

- **禁止** `_ = someFunc()` 忽略错误（除非注释说明原因）
- `log.Fatal` 仅限 startup 阶段不可恢复的错误
- 数据库操作返回的 error 必须处理（至少记日志）
- Redis 限流操作错误不得静默忽略（`pkg/ratelimit`）

## P0-05: 类型契约

- 公开函数必须有完整签名类型（禁止 `interface{}` / `any` 返回值）
- JSON/DB tag 必须成对出现
- 前端 API 返回值必须可推导类型

## P0-06: 数据不出内网（v2.0）

- L0/L1/L2 层不得调用任何远程模型 API
- 云端外发字段必须经过脱敏处理
- 禁止将整段原始日志发送给云端模型
- 模型密钥不得明文落配置（必须环境变量注入）

## P0-07: 前端特定

- Pinia store 导出名以 `Storage` 后缀：`useAuthStorage`、`useThemeStorage`
- 组件使用 `<script setup>` + Composition API
- 搜索输入必须 debounce 300ms+
- API 调用错误不穿透到视图层（拦截器仅处理 HTTP 层错误）

## P0-08: 并发安全

- goroutine 必须通过 `context.Context` 传递取消信号
- 阻塞的 channel 发送必须用 `select { case <-ctx.Done(): return }` 保护
- 共享变量必须加锁（`sync.Mutex` 或 `sync.RWMutex`）
