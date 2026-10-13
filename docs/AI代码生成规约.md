# AI 代码生成规约

> **版本**: v1.1.0  
> **生效日期**: 2026-10-12  
> **适用范围**: 使用 AI 辅助生成、补全、重构云顶追踪项目 v2.0 代码的全部工作流  
> **适用工具**: Cursor / Claude Code / GitHub Copilot / 妙手（CatPaw）/ 通义灵码 / Tabnine  
> **对标基准**: Cursor Rules 最佳实践、Anthropic Claude Code Engineering Blog、Google AI-Assisted Coding Guidelines、awesome-cursor-rules 社区共识、Addy Osmani Agent Skills 方法论  
> **关联文档**: 《Ydsz Trace · AI 时代的日志检索分析引擎 PRD V2.0》

---

## 一、总则

### 1.1 规约定位

本规约定义的是**"AI 需遵守什么"**而非**"人需遵守什么"**。它是人与 AI 协作的接口协议——人提供准确上下文，AI 产出高质量代码。

与《云顶编码规范》的关系：
- 《编码规范》= 人类与 AI 共同遵守的**强制性编码公约**
- 《AI 代码生成规约》= AI 工具在云顶项目中运行的**上下文协议和行为约束**

### 1.2 核心原则

| 原则 | 含义 |
|------|------|
| **上下文最小信噪比** | 每次请求注入的上下文必须精确、无冗余，避免注意力摊薄 |
| **分而治之** | 大任务拆分为 ≤ 50% 上下文窗口可独立完成的小任务 |
| **先想后做** | 先 Plan Mode 确认方案再编码，避免方向性偏差 |
| **自闭环验证** | AI 必须能在无人工干预下验证产出（测试/lint/build/截图） |
| **显式优于隐式** | 依赖、副作用、意图必须通过类型/命名/注释显式表达 |
| **失败时留 TODO** | 无法完成的任务标记 `TODO(ai): 原因` 而非静默跳过 |

---

## 二、AI 工作流协议

### 2.1 任务分级与执行策略

```
所有 AI 任务按复杂度分级，采用不同策略：

Level 1: 原子操作（≤ 1 文件，≤ 50 行改动）
  → 直接执行，无需 Plan Mode
  → 示例：添加日志、修复 lint 告警、重命名变量

Level 2: 功能开发（≤ 5 文件，≤ 300 行改动）
  → 先展示分步计划（3-5 步），确认后执行
  → 示例：新增 API 端点、添加限流中间件

Level 3: 架构变更（≥ 5 文件，跨模块）
  → 必须 Plan Mode → CR 评审 → 分批次执行
  → 示例：新增数据库表 + API + 前端页面
```

### 2.2 Plan Mode 强制场景

以下场景必须启用 Plan Mode 或等效的分步推理：

- 涉及数据库 Schema 变更的任务
- 涉及多文件（≥ 3）联动修改的任务
- 涉及安全机制（认证/加密/鉴权）的任务
- 引入新依赖（第三方库/新 API 端点）的任务
- 需求描述中存在歧义或未明确约束的任务
- **v2.0 涉及列式段存储引擎、模板解析器、向量化、Agent 任务编排的任务**
- **v2.0 涉及 DSL 语法设计、NL2DSL 服务、Embedding Provider 实现的任务**
- **v2.0 涉及 MCP Server 工具暴露、评测集设计的任务**

### 2.3 上下文管理

```
【强制】
- 每次请求携带完整的类型信息（不依赖跨文件推断）
- 长文件（> 300 行）必须通过 @file 精确引用，不整体注入
- 修改前必须先 Read 目标文件，禁止盲改
- 完成一次编辑后验证（Read 结果 / Run test / Run lint）

【建议】
- 上下文使用超过 50% 时拆分请求
- 无关文件通过 .catpawignore / .cursorignore 排除
- 定期清理会话历史，避免累积噪声
```

### 2.4 AI 代理运行约束

```
【强制】
- 不引入新依赖文件（除非任务明确要求）
- 不改变任务范围外的代码逻辑
- Git 提交前必须通过 lint/test 验证
- 破坏性操作（删除/重命名公共 API）必须显式声明并请求确认

【建议】
- 每完成一个文件就提交（小步 commit）
- 复杂任务使用子代理（SubAgent）隔离上下文
- 并行修改独立文件时使用并发 Agent
```

---

## 三、项目上下文描述（Project Context）

### 3.1 技术栈声明

```
技术栈：
- 后端: Go 1.26.5 + Gin 1.12 + sqlx 1.4 + SQLite (modernc.org/sqlite 1.38)
- 存储引擎 v2.0: 列式段存储 (parquet/zstd) + DuckDB (可选加速)
- 向量/语义 v2.0: Embedding API + 向量索引 (HNSW)
- AI 适配层 v2.0: aimodel.Provider 接口 + local(Ollama/vLLM)/cloud/fallback 路由
- Agent v2.0: ReAct 模式工具编排 (SearchLogs/GetPatterns/GetTimeline/SemanticSearch/...)
- MCP v2.0: JSON-RPC 2.0 over HTTP (MCP Streamable HTTP)
- 前端: Vue 3.5 + Vite 6 + Element Plus 2.9 + Tailwind CSS 3.4
- 状态: Pinia 2.3
- HTTP 客户端: Axios 1.7
- 部署: Docker Compose 3.8 / Kubernetes
- 包管理: Go Modules (workspace) + npm
- 构建: Makefile (build/test/lint/fmt/ci)
- 代码质量: golangci-lint + gofmt + ESLint 9 + Prettier 3
- 测试: Go testing + httptest + eval 集 / Vitest 2 + jsdom
- 安全: Argon2id / JWT / RBAC / CSRF Token / AI 数据脱敏
```

### 3.2 模块分工

```
模块结构：
- pkg/          → 公共库（无业务依赖）
  - schema/     →   v2.0 数据模型 (LogSegment/LogTemplate/LogEvent/...)
  - dsl/        →   v2.0 DSL 解析器
  - parser/     →   v2.0 日志模板引擎 (Drain 算法)
  - aimodel/    →   v2.0 模型适配层 (Provider/Router)
  - agent/      →   v2.0 ReAct Agent 工具编排
  - mask/       →   v2.0 脱敏规则引擎
  - embedding/  →   v2.0 向量索引与管理
  - (原有 pkg 子包保留)
- logs/         → v2.0 服务端（端口 2021，含列式存储引擎 + 语义服务）
- logc/         → v2.0 客户端代理（端口 2020，含采集管道 + 端侧索引）
- logv/        → v2.0 前端控制台（对话式工作台为默认首页）
- mcp/          → v2.0 MCP Server 入口
- sqls/         → DDL 脚本
- deploy/       → Docker/K8s 配置

入口文件：
- logs/main.go       → 服务端启动入口
- logc/main.go       → 客户端启动入口
- mcp/serve.go       → v2.0 MCP Server 入口
- logv/src/main.js   → 前端挂载入口
```

### 3.3 核心约定速查

```
Go 项目约定：
- 包名 = 目录名，全小写无下划线
- DB 结构体以 T 前缀：TClient、TItem、TTask（v1 兼容）
- v2.0 DB 结构体：LogSegment、LogTemplate、LogEvent、PatternStat（以 Log 前缀命名）
- 接口以 er/able 结尾：Source、Limiter、Backend、Provider、Tool
- 错误哨兵集中声明 + errors.Is 判断
- 统一响应：api.Success(c, data) / api.Fail(c, code, msg, data)
- 配置读取：cfg.String("key") / cfg.Int("key") / cfg.Bool("key")
- 日志：logger.Info("msg", "key", value) 结构化 slog
- v2.0 智能漏斗函数前缀：L0 rule_/match_、L1 stats_/baseline_、L2 embed_/cluster_、L3 nl2dsl_/reasoning_
- v2.0 Agent 工具：SearchLogsTool、GetPatternsTool...（PascalCase + Tool 后缀）
- v2.0 MCP 工具：query_logs、search_similar（snake_case）
- v2.0 环境变量前缀仍为 YDSZ_；模型相关追加 YDSZ_MODEL_ / YDSZ_AGENT_

前端约定：
- 组件：<script setup> + Composition API + PascalCase 文件名
- Store：Pinia 组合式语法 + useXxx() 导出
- API：统一 axios 实例（api/http.js），业务域按文件拆分（新增 api/conversations.js、api/dsl.js）
- 路径别名：@ → logv/src/
- 路由：camelCase name + kebab-case path
- v2.0 默认首页：Chat（对话式工作台）；v1 页面移入次级导航
- v2.0 组合式函数：useNLQuery()、useAgentConversation()、useAnomalyTimeline()
```

---

## 四、规则分级体系

### 4.1 P0 — 不可违反（Blocker）

违反 P0 规则的产出必须回滚或重做。

```
P0-01: 安全底线
  - 禁止在代码中硬编码密码、密钥、Token
  - 禁止使用 MD5/SHA1 存储密码（必须 Argon2id）
  - SQL 必须使用参数化查询，禁止字符串拼接
  - 文件路径输入必须经 sanitizeKey 过滤
  - JWT 不得使用 none 算法

P0-02: 模块边界
  - 禁止反向依赖（pkg → logs、pkg → logc、logs ↔ logc）
  - 禁止前端直接引用 Go 模块
  - 禁止跨层调用（Controller 直接调 DB 需经过 Model 层）

P0-03: 响应格式
  - 所有 API 必须使用 pkg/api 的 Success/Fail/Paginated
  - 禁止直接 c.JSON/b 返回非标准格式
  - 禁止 HTTP handler 返回裸 error

P0-04: 错误处理
  - 禁止 _ = someFunc() 忽略错误（除非注释说明原因）
  - log.Fatal 仅限 startup 阶段
  - 数据库操作返回的 error 必须处理

P0-05: 类型契约
  - 公开函数必须有完整签名类型（无 any / interface{} 返回值）
  - JSON/DB tag 必须成对出现
  - 前端 API 返回值必须可推导类型

P0-06: 数据不出内网（v2.0 新增）
  - L0/L1/L2 层不得调用任何远程模型 API
  - 云端外发字段必须经过 pkg/mask 脱敏处理
  - 禁止将整段原始日志发送给云端模型（只发模板串 + 聚合数值 + ≤ 20 采样）
  - 模型密钥不得明文落配置，必须通过 YDSZ_MODEL_KEY_SECRET 注入
```

### 4.2 P1 — 最佳实践（Strongly Recommended）

违反 P1 规则不阻塞合入，但必须在 CR 中说明理由。

```
P1-01: 命名规范
  - 变量名不短于 2 个字符（循环变量/坐标除外）
  - 临时变量也需有意：ok → found/valid、err → readErr/dbErr
  - ORM 结构体字段注释说明DB列映射

P1-02: 注释文档
  - 公开 Go 函数必须有 GoDoc 注释
  - 前端组件的 Props/Emits 必须有 JSDoc 类型描述
  - 安全注释：CSRF、限流等"为什么"需要注释

P1-03: 日志规范
  - 关键操作（登录/配置变更）使用 [security] 前缀
  - 使用占位符而非拼接：logger.Info("user login", "name", name)
  - Debug 级别日志不进入生产

P1-04: 测试覆盖
  - 新增 API 端点必须有至少一个集成测试
  - 安全相关变更必须有负面测试用例
  - 测试函数命名：Test{Module}_{Action}_{Scenario}

P1-05: 依赖管理
  - 引入新第三方库需评估体积、维护状态、License
  - Go 依赖通过 go get 获取（不手动写 go.mod）
  - 前端依赖通过 npm install 获取（不手动写 package.json）

P1-06: 前端性能
  - 列表 > 100 项时使用虚拟滚动
  - API 调用避免瀑布请求
  - 大组件拆分子组件减少 re-render
```

### 4.3 P2 — 代码优雅（Nice to Have）

违反 P2 规则不做强制要求，但显著影响可维护性时应改进。

```
P2-01: 函数长度
  - 单函数不超过 50 行（不含注释和空行）
  - 超过 30 行考虑提取子函数

P2-02: 文件长度
  - 单文件不超过 800 行（Go）/ 600 行（Vue）
  - 超出时按职责拆分

P2-03: 魔法值
  - 数字/字符串常量定义为具名常量
  - 使用到的魔法值集中在 const 块声明

P2-04: 提前返回
  - 使用 guard clause 替代深层嵌套 if
  - 错误路径优先 return

P2-05: 资源清理
  - HTTP Response Body 确保 Close
  - SSE 连接设置断开清理
  - 前端 onUnmounted 清理定时器/监听器
```

---

## 五、分场景生成指南

### 5.1 Go Controller 生成模板

```go
// AI 生成新 Controller 时遵循此模板

// Package logs 处理日志检索相关请求。
package logs

import (
    "net/http"
    
    "github.com/gin-gonic/gin"
    
    "ydsz-trace/pkg/api"
    "ydsz-trace/logs/models"
)

// SearchLogsRequest 日志搜索请求参数。
type SearchLogsRequest struct {
    ItemID    int64  `json:"itemId" binding:"required,min=1"`
    Keyword   string `json:"keyword" binding:"max=100"`
    StartTime string `json:"startTime"`  // RFC3339 格式
    EndTime   string `json:"endTime"`    // RFC3339 格式
    PageNo    int    `json:"pageNo" binding:"min=1"`
    PageSize  int    `json:"pageSize" binding:"min=1,max=100"`
}

// HandleSearchLogs 执行日志全文搜索。
// 注意：keyword 为空时返回 400，不允许无条件全表扫描。
func HandleSearchLogs(c *gin.Context) {
    var req SearchLogsRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        api.Fail(c, "INVALID_PARAM", "参数错误: "+err.Error(), nil)
        return
    }
    
    result, err := models.SearchLogs(req.ItemID, req.Keyword, 
        req.StartTime, req.EndTime, req.PageNo, req.PageSize)
    if err != nil {
        logger.Error("search logs failed", "itemId", req.ItemID, "err", err)
        api.Fail(c, "SEARCH_FAILED", "搜索失败", nil)
        return
    }
    
    api.Paginated(c, result.List, result.Total, req.PageNo, req.PageSize)
}
```

### 5.2 Go Model 生成模板

```go
// AI 生成新 Model 时遵循此模板

// SearchLogs 从指定日志项中搜索匹配的日志条目。
// 当 keyword 为空时返回空切片而非全量查询。
func SearchLogs(itemID int64, keyword, startTime, endTime string, 
    pageNo, pageSize int) (*SearchResult, error) {
    
    offset := (pageNo - 1) * pageSize
    args := []interface{}{itemID, "%" + keyword + "%", startTime, endTime}
    
    var total int
    countQuery := `SELECT COUNT(*) FROM t_log_??? 
        WHERE item_id = ? AND content LIKE ? 
        AND timestamp >= ? AND timestamp <= ?`
    if err := db.Get(&total, countQuery, args...); err != nil {
        return nil, fmt.Errorf("count logs: %w", err)
    }
    
    dataQuery := `SELECT * FROM t_log_??? 
        WHERE item_id = ? AND content LIKE ? 
        AND timestamp >= ? AND timestamp <= ?
        ORDER BY timestamp DESC LIMIT ? OFFSET ?`
    args = append(args, pageSize, offset)
    
    var items []TLog???
    if err := db.Select(&items, dataQuery, args...); err != nil {
        return nil, fmt.Errorf("query logs: %w", err)
    }
    
    return &SearchResult{
        List:  items,
        Total: total,
    }, nil
}
```

### 5.3 Vue 组件生成模板

```vue
<script setup>
/**
 * LogSearch 日志搜索页面组件。
 * 提供关键词搜索、时间范围过滤、结果分页功能。
 */

import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { searchLogs } from '@/api/logs'
import { useAuth } from '@/store/auth'

const router = useRouter()
const auth = useAuth()

// === 响应式状态 ===
const keyword = ref('')
const timeRange = ref([])
const loading = ref(false)
const resultList = ref([])
const pagination = ref({ pageNo: 1, pageSize: 20, total: 0 })

// === 计算属性 ===
const hasResults = computed(() => resultList.value.length > 0)
const isSearchDisabled = computed(() => !keyword.value.trim() || loading.value)

// === 方法 ===
async function handleSearch() {
    if (isSearchDisabled.value) return
    loading.value = true
    try {
        const resp = await searchLogs({
            keyword: keyword.value.trim(),
            startTime: timeRange.value?.[0] ?? '',
            endTime: timeRange.value?.[1] ?? '',
            pageNo: pagination.value.pageNo,
            pageSize: pagination.value.pageSize,
        })
        resultList.value = resp.data.list
        pagination.value.total = resp.data.total
    } catch (err) {
        // 错误由 axios 拦截器统一处理
    } finally {
        loading.value = false
    }
}

function handlePageChange(pageNo) {
    pagination.value.pageNo = pageNo
    handleSearch()
}

// === 生命周期 ===
// (无特殊清理需求)
</script>
```

### 5.4 API 模块生成模板

```javascript
/**
 * logs.js — 日志检索相关 API。
 * 所有函数返回 Promise<ApiResponse>，错误由 axios 拦截器统一处理。
 */

import http from './http'

/**
 * 搜索日志。
 * @param {Object} params
 * @param {string} params.keyword - 搜索关键词
 * @param {string} [params.startTime] - 起始时间（RFC3339）
 * @param {string} [params.endTime] - 结束时间（RFC3339）
 * @param {number} [params.pageNo=1] - 页码
 * @param {number} [params.pageSize=20] - 每页条数（最大100）
 * @returns {Promise<{list: LogEntry[], total: number}>}
 */
export function searchLogs(params) {
    return http.post('/logs/search', params).then(res => res.data.data)
}

/**
 * 获取日志流（SSE 长连接）。
 * @param {number} itemId - 日志项 ID
 * @returns {EventSource}
 */
export function streamLogs(itemId) {
    return new EventSource(`/logs/stream?itemId=${encodeURIComponent(itemId)}`, {
        withCredentials: true,
    })
}
```

---

## 六、反模式清单（Anti-Patterns）

### 6.1 禁止 AI 引入的代码模式

```
【强制禁止】

① 安全裸奔
   - 未参数化 SQL：fmt.Sprintf("SELECT * FROM t WHERE id = %s", userInput)
   - 未校验文件路径：os.Open(c.Query("path"))
   - 未过滤的正则直接编译用户输入

② 全局可变状态泄漏
   - 跨请求修改包级变量（无锁保护）
   - 使用 init() 做非平凡的初始化逻辑
   - 环境变量直接读取而不经过 cfg.Xxx()

③ 资源泄漏
   - resp.Body 未 defer Close()
   - goroutine 无 context 取消机制
   - SSE 连接无超时和断开清理

④ 类型逃逸
   - 函数返回 interface{} 或 any
   - 使用 _ = 忽略错误
   - 类型断言不检查 ok：val := m["key"].(string)

⑤ 上下文断裂
   - 跨 3+ 文件修改但不同步更新
   - 修改函数签名但遗漏调用方
   - 修改 DB 结构但不更新 DDL 脚本

⑥ 前端性能反模式
   - v-for 中过滤器（应使用 computed）
   - 深层 watch 引起连锁 re-render
   - 未防抖的搜索输入触发

⑦ AI 系统特有反模式（v2.0 新增）
   - L0/L1/L2 层调用远程模型（aimodel 路由必须 fallback-only 在规则/统计/嵌入层）
   - NL2DSL LLM 调用结果不缓存在 Redis（要求缓存 TTL 1h+）
   - 模板解析器在日志行数 < 100 时触发 Drain（最小阈值 100 行）
   - Agent 工具输出不带 evidence_ref（每次工具调用必须返回证据引用 ID）
   - Agent 单会话步数不设上限（硬上限 ≤ 8 步，超时 30 秒）
   - 语义检索 Embedding 维度不校验直接入库（必须校验维度 == 模型声明维度）
   - 向量索引构建时不分批/不流式（> 10000 条需分批 Commit 防 OOM）
   - 对话消息落库时包含完整工具调用栈外的敏感数据
   - 脱敏规则引擎处理不含正则超时/honeypot 攻击防护
   - MCP Server 响应不包含 evidence_ref 字段
   - 前端对话组件不虚拟滚动（10 万行日志结果必须虚拟滚动）
   - 评测集未变更情况下自动降低通过门槛
```

### 6.2 AI 特有的幻觉风险

```
高频幻觉场景及防护：

① 引用不存在的 API
   → 例：c.QueryInt64() 在 gin 0.x 不存在（项目有封装）
   → 防护：使用项目提供的封装函数

② 不存在的 struct 字段
   → 例：TClient.UpdatedAt 不存在
   → 防护：先生成代码前先 Read 结构体定义

③ 路径别名错误
   → 例：import { xxx } from '@/api/log' (应为 logs.js)
   → 防护：先确认文件存在再引用

④ 不恰当的第三方库选择
   → 例：推荐 gorm 但项目使用 sqlx
   → 防护：技术栈声明中明确"不允许新 ORM 依赖"

⑤ 异步混淆
   → 例：混用 callback 和 async/await（前端）
   → 防护：统一使用 async/await

⑥ 错误处理过度乐观
   → 例：happy-path 代码无 error 分支
   → 防护：P0-04 规则强制要求 error handling
```

---

## 七、验证闭环机制

### 7.1 AI 自验证检查表

```
每次代码生成完成后，AI 必须按序验证：

【阶段一：语法/编译】
□ go build ./... 通过（Go）
□ npm run build 通过（前端）
□ golangci-lint 无新增告警（Go）
□ ESLint 无新增告警（前端）

【阶段二：测试】
□ go test -race ./... 通过（Go 改动文件涉及的测试）
□ npm run test 通过（前端改动文件涉及的测试）
□ 新增代码有对应测试覆盖（L2 以上任务）
□ **v2.0 改动 AI 模块时：对应 eval 集运行通过（go test ./evals/...）**
□ **v2.0 eval 分数不降级：变更前后对比，超过 -2% 的下降必须 CR 说明**

【阶段三：一致性】
□ 修改的文件 import/syntax 无断裂
□ DDL 变更已同步到 sqlite.sql
□ 新增环境变量已描述在示例配置
□ API 变更已更新前端对应调用
□ **v2.0 Provider 接口变更已更新所有实现**
□ **v2.0 Agent Tool 签名变更已更新工具注册方**
□ **v2.0 MCP 工具描述已同步更新（Name + Description）**

【阶段四：交付确认】
□ 所有测试通过
□ 无遗留 TODO（或明确标注原因）
□ 变更范围未超出任务描述
```

### 7.2 无法自验证的场景

当 AI 无法完成自验证时（如需真实数据库、外部服务交互、视觉检查），必须：

1. 显式声明"以下项需人工验证：..."
2. 给出具体的验证命令或操作路径
3. 在 TODO 中标注 `TODO(verify): 需人工确认 xxx`

---

## 八、提示词工程规范

### 8.1 AI 请求的结构化格式

```
推荐的任务描述结构：

【目标】一句话说明要做什么
【上下文】涉及哪些文件/模块，当前状态
【约束】技术限制、不可违反的规则
【输出】期望的文件路径、函数签名、测试策略
【验证】如何确认产出正确

示例：
"新增日志导出 CSV 功能：
- 上下文：参考 logs/controllers/logs/search.go 已有搜索逻辑
- 约束：使用现有 pkg/api 响应格式，复用 SearchLogs 模型层，不引入新依赖
- 输出：新增 /logs/export 端点（POST），返回 application/octet-stream
- 验证：编写测试验证 CSV 格式正确 + 测试大文件不 OOM"
```

### 8.2 上下文注入策略

```
精准度优先级（从高到低）：

1. @file 精确文件引用    → 最高精确度
2. @folder 目录注入      → 适度范围
3. 技术栈声明            → 全局基线
4. 隐式推断             → 最低优先级（可能出错）

【建议】
- 修改已有文件使用 @file + 需要改动的具体行号
- 新增文件提供参考实现路径（"参考 xxx.go 的写法"）
- 技术栈相关约束写在请求中，不假设 AI 知道
```

### 8.3 分而治之的拆分原则

```
将 L3 任务拆分的标准模式：

任务："实现告警功能"

拆分第 1 次调用（DB 层）：
  → 输入：告警表结构定义
  → 输出：DDL 脚本 + model CRUD

拆分第 2 次调用（API 层）：
  → 输入：model 签名 + 统一响应规则
  → 输出：controller + router 注册

拆分第 3 次调用（前端层）：
  → 输入：API 端点文档 + 组件模板
  → 输出：告警列表页 + 告警规则配置页

拆分第 4 次调用（测试）：
  → 输入：已实现的函数签名
  → 输出：单元测试 + 集成测试

每次调用独立验证，不跨调用依赖上下文记忆。
```

---

## 九、团队适配建议

### 9.1 规则文件存储

```
推荐存放位置（与工具无关）：

根目录/
├── docs/
│   ├── 云顶编码规范.md          # 人类与 AI 共同遵守的编码公约
│   ├── AI代码生成规约.md        # AI 行为约束（本文件）
│   └── ai-rules/               # AI 工具特定的规则文件
│       ├── P0-blockers.md      # P0 不可违反规则（精简版）
│       ├── P1-practices.md     # P1 最佳实践
│       ├── project-context.md  # 项目上下文描述
│       └── anti-patterns.md    # 反模式清单
├── .cursor/rules/              # Cursor 专用（如使用）
│   └── ...
├── CLAUDE.md                   # Claude Code 专用（如使用）
├── .github/
│   └── copilot-instructions.md # GitHub Copilot 专用（如使用）
└── AGENTS.md                   # 跨工具通用入口
```

### 9.2 规则文件精简原则

```
【规则文件编写要点】

1. 总量控制：所有规则文件合计 ≤ 500 行
2. 单文件控制：单个规则文件 ≤ 150 行（Claude Code 建议）
3. 元数据：使用 frontmatter description/globs/alwaysApply 实现精准命中
4. 更新频率：模型升级后重新评估，删减低效规则
5. 版本化：规则文件进入 Git，变更走 MR 评审

// .cursor/rules/example.mdc 示例
---
description: "安全编码强制规则"
globs: ["**/*.go"]
alwaysApply: true
---

- 所有 SQL 使用 ? 参数化，禁止字符串拼接
- 文件路径必须经 sanitizeKey 白名单过滤
- 密码哈希使用项目 auth 模块，禁止自行实现
```

### 9.3 渐进式引入路径

```
阶段 0：基线建立（当前）
  → 完成本规约文档
  → 配置 P0 规则文件
  → 建立模板库

阶段 1：工具接入
  → 生成 CLAUDE.md / .cursor/rules/ 等工具特定文件
  → 配置 .gitignore / .catpawignore 排除无关文件

阶段 2：CI 集成
  → linters 质量门禁（已部分完成）
  → AI 产出首次通过率度量

阶段 3：持续优化
  → 每月回顾 AI 产出失败案例
  → 补充规则 or 删减低效规则
  → 更新模板库
```

---

## 附录 A：AI 自检 Quick Reference

```
每次代码生成前（3 秒自检）：

☑ 我 Read 了这个文件吗？
☑ 我使用了项目已有的封装吗？（不重复造轮子）
☑ 我处理了所有 error 吗？
☑ 我的改动在任务范围内吗？
☑ 我的改动会导致跨文件断裂吗？
☑ 我使用了正确的 tag 风格吗？（json=camelCase / db=snake_case）
☑ 我的日志不含敏感信息吗？
☑ 我的测试能跑吗？

每次代码生成后（5 秒自检）：

☑ 我运行了 make lint / make test 吗？
☑ 未完成的任务我标记了 TODO(ai) 吗？
☑ 我检查了依赖方向吗？
☑ 我检查了资源清理吗？
```

## 附录 B：与《云顶编码规范》条款映射

| 编码规范章节 | AI 规约对应章节 | 说明 |
|-------------|----------------|------|
| 三、命名规范 | §3.3 + §4.1 P1-01 | AI 必须遵循项目既有命名约定 |
| 四、代码格式 | §4.3 P2-06 | 格式化由工具自动完成但 AI 需产出合规内容 |
| 五、注释规范 | §4.2 P1-02 | AI 生成的公开 API 必须有注释 |
| 错误处理 | §4.1 P0-04 + §6.1 反模式 | 错误处理是最强制的约束之一 |
| 安全规范 | §4.1 P0-01 + §6.1 反模式 | 安全是 P0 不可违反底线 |
| 测试规范 | §7.1 验证闭环 | AI 必须生成可运行的测试 |
| 并发与性能 | §4.3 P2-04 + §6.1 | 资源清理作为 P2 反模式 |
| 数据库规范 | §4.1 P0-02 + §7.1 阶段三 | DDL 一致性验证 |
| 前端规范 | §5.3 模板 + §6.1 | 组件模板和反模式 |
| Git 工作流 | §2.2 Plan Mode | AI 遵循提交前自查清单 |
| 十六、存储与引擎规范 | §6.1 ⑦ AI 特有反模式 | 列式段命名与生命周期规则 |
| 十七、AI 系统专项规范 | §2.2 Plan 强制 + P0-06 | 隐私/漏斗/评测的 AI 行为约束 |
| 十八、前端交互规范 | §8.2 分而治之 | 对话组件拆分与组合式函数 |

## 附录 C：已知陷阱速查

```
这些是本 AI 在云顶项目中容易犯错的已知点：

① gin 版本：1.12.0（非最新版），某些 API 签名以项目实际为准
② SQLite：modernc.org/sqlite（纯 Go，非 CGO），不支持某些扩展 SQL
③ T 前缀结构体：DB 表结构体记为 TClient（不要擅自加 Table 前缀）
④ Tag 映射：json=camelCase / db=snake_case，两套 tag 不可混淆
⑤ 包级 DB 句柄：使用 models.DB 而非新建连接
⑥ 日志：pkg/logger 已封装 slog，不直接使用 log 包新增日志
⑦ 前端 axios：统一从 api/http.js 引用，不创建新的 axios 实例
⑧ 存储：localStorage key 固定为 'ydsz_username'
⑨ 路径别名：@ → src/，路径错误会导致 vite 构建失败
⑩ 测试：go test -race 竞态检测必须通过，golangci-lint 圈复杂度 <= 15

== v2.0 新增陷阱 ==
⑪ 智能漏斗跨层调用：L0 会直接调用 nl2dsl_ 函数（禁止），必须通过 L1→L2→L3 相邻层传递
⑫ aimodel 超时 Provider 不降级：实现了 fallback 但没写超时自动切换逻辑
⑬ Agent 工具 Execute() 忽略 ctx 取消：必须监听 ctx.Done() 并在 30s 内返回
⑭ NL2DSL 缓存 key 不含用户/Session 隔离：导致 A 用户的缓存覆盖 B 用户的查询
⑮ 模板模板引擎 Drain 在日志过少时强制聚类：<100行时应跳过聚类
⑯ MCP 工具遗漏 evidence_ref：每个工具调用结果必须包含证据链回溯字段
⑰ 向量索引维度不匹配：不同 Provider 输出维度不同，入库前必须校验
⑱ 对话落库未脱敏：用户原始输入含敏感信息时直接写 t_message.content
⑲ 评测集 eval 不具幂等性：随机种子固定、模型温度设 0 确保可复现
⑳ 前端对话气泡不虚拟滚动：日志证据超过 200 条时使用 vue-virtual-scroller
```

---

> **文档维护**：本规约随 v2.0 工程实施进度持续迭代。每次 AI 引擎（模板/语义/Agent/MCP）接口变更时，应重新评估 §5 分场景模板的有效性。当 PRD 升级到 v2.1+ 版本时同步更新此规约。
