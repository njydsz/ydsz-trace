# ydsz-trace 前端 E2E 测试计划

## 说明

本文档描述 ydsz-trace 管理控制台的端到端测试计划，当前**不实际安装 Playwright**（避免增加 build 依赖复杂度）。后续集成时按本清单创建对应测试文件。

## 测试框架

- **Playwright**（待安装）：`@playwright/test`
- **测试环境**：测试模式下启动 Vite dev server + mock API 后端
- **浏览器**：Chromium（CI）+ Firefox + WebKit（本地开发）

## 关键用户旅程覆盖清单

### Journey 1：登录
| 场景 | 描述 | 优先级 |
|------|------|--------|
| 正确凭证登录 | 输入用户名 → 跳转仪表板 | P0 |
| 空用户名拒绝 | 空用户名不跳转，提示错误 | P1 |
| 会话恢复 | 刷新页面后保持登录态（localStorage） | P1 |

### Journey 2：搜索（日志检索）
| 场景 | 描述 | 优先级 |
|------|------|--------|
| 选择客户端 | 加载客户端下拉列表 | P0 |
| 按日志项检索 | 选择客户端 → 加载日志项 → 查询 | P0 |
| 空关键词拒绝 | 无关键词时不发起检索 | P1 |
| 高级检索切换 | 全文模糊搜索 vs 关键词精确匹配互斥 | P1 |
| 搜索结果分页 | 分页组件切换页码 | P2 |

### Journey 3：查看直方图
| 场景 | 描述 | 优先级 |
|------|------|--------|
| 直方图渲染 | 查询后直方图按时间分桶展示 | P0 |
| 级别分色 | INFO/WARN/ERROR 颜色区分 | P1 |
| 柱状图 tooltip | hover 显示各级别数量详情 | P2 |

### Journey 4：TraceID 查询
| 场景 | 描述 | 优先级 |
|------|------|--------|
| 节点分组视图 | 按节点分组展示命中日志行 | P0 |
| 瀑布视图切换 | 切换到瀑布视图展示调用链时序 | P0 |
| URL query 自动追踪 | 从 URL 参数自动发起 trace | P1 |
| 空结果提示 | 未找到匹配时给出提示 | P1 |
| 正则模式 | 勾选正则匹配后按正则搜索 | P2 |

### Journey 5：瀑布视图
| 场景 | 描述 | 优先级 |
|------|------|--------|
| SVG 瀑布渲染 | span 在时间轴上正确分布 | P0 |
| 错误 span 高亮 | ERROR 级别 span 红色突出显示 | P1 |
| Span 表格 | 表格列表与 SVG 视图数据一致 | P1 |
| 空状态 | 无数据时显示空状态图 | P2 |

### Journey 6：告警管理
| 场景 | 描述 | 优先级 |
|------|------|--------|
| 告警规则列表 | 加载并展示规则列表 | P0 |
| 新增规则 | 填写表单并保存 | P0 |
| 启停切换 | 切换规则启用状态 | P1 |
| 删除规则 | 确认删除后移除 | P1 |
| 告警事件 | 展示告警触发记录 | P2 |

### Journey 7：客户端/日志项管理
| 场景 | 描述 | 优先级 |
|------|------|--------|
| 客户端 CRUD | 新增/编辑/删除客户端 | P1 |
| 日志项 CRUD | 新增/编辑/删除日志项 | P1 |
| 批量操作 | 批量删除所选记录 | P2 |

## 测试文件清单（待创建）

```
logv/tests/e2e/
  auth.spec.ts            # Journey 1: 登录 / 会话
  log-search.spec.ts      # Journey 2: 搜索
  histogram.spec.ts       # Journey 3: 直方图
  trace-id-search.spec.ts # Journey 4: TraceID 查询
  waterfall.spec.ts       # Journey 5: 瀑布视图
  alerts.spec.ts          # Journey 6: 告警管理
  admin-crud.spec.ts      # Journey 7: 客户端/日志项管理
  support/
    fixtures.ts           # 共享 fixtures
    helpers.ts            # 登录 / 导航等通用方法
```

## 数据隔离策略

- 每个测试运行前通过 `test.beforeEach` 重置 mock 数据后端
- 使用独立 localStorage key（`test:` 前缀）避免跨测试污染
- 配置文件：`playwright.config.ts` 中配置 `testIdAttribute` 为 `data-testid`

## 推荐 CI 流程

```
npm run build  →  npx playwright install chromium  →  npx playwright test
```

## 后续行动

1. 安装 `@playwright/test` devDependency
2. 配置 `playwright.config.ts`（baseURL、webServer 复用 vite dev）
3. 按优先级 P0 → P1 → P2 顺序创建测试文件
4. 在关键交互元素上添加 `data-testid` 属性
