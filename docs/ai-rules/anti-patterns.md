# AI 反模式清单

> **禁止 AI 引入的代码模式**。每次生成代码前快速自检（3 秒）。

## ① 安全裸奔

- ❌ 未参数化 SQL：`fmt.Sprintf("SELECT * FROM t WHERE id = %s", userInput)`
- ❌ 未校验文件路径：`os.Open(c.Query("path"))`
- ✅ 正确：经过 `sanitize.SafeFilename()` 后再拼入路径

## ② 全局可变状态泄漏

- ❌ 跨请求修改包级变量（无锁保护）
- ❌ 使用 `init()` 做非平凡的初始化逻辑
- ✅ 使用依赖注入或工厂模式

## ③ 资源泄漏

- ❌ HTTP `resp.Body` 未 `defer Close()`
- ❌ goroutine 无 context 取消机制
- ❌ SSE 连接无超时和断开清理
- ✅ 阻塞 channel 发送加 `select { case <-ctx.Done(): return }`

## ④ 类型逃逸

- ❌ 函数返回 `interface{}` 或 `any`
- ❌ 使用 `_ =` 忽略错误
- ✅ 错误至少记日志：`log.Printf("[module] action failed: %v", err)`

## ⑤ 上下文断裂

- ❌ 跨 3+ 文件修改但不同步更新
- ❌ 修改函数签名但遗漏调用方
- ❌ 修改 DB 结构但不更新 `sqls/sqlite.sql`

## ⑥ 前端性能反模式

- ❌ `v-for` 中过滤器（应使用 `computed`）
- ❌ 深层 `watch` 引起连锁 re-render
- ❌ 未防抖的搜索输入（必须 debounce 300ms+）
- ❌ 同一行重复 `.find()` 调用（应缓存为变量）

## ⑦ AI 系统特有反模式（v2.0）

- ❌ L0/L1/L2 层调用远程模型
- ❌ NL2DSL LLM 调用结果不缓存（要求 Redis TTL 1h+）
- ❌ Agent 工具输出不带 `evidence_ref`
- ❌ 向量索引维度不校验直接入库
- ❌ 对话消息落库时包含未脱敏敏感数据

## ⑧ 命名违规

- ❌ 常量 lowerCamelCase（必须 `UPPER_SNAKE_CASE`）
- ❌ Store 无 `Storage` 后缀
- ❌ 请求结构体无 `Req`/`Resp` 后缀
- ❌ 接口不以 `er/able/ior` 结尾
