# AGENTS.md — 跨 AI 工具通用入口

> 本文件是所有 AI 工具（Claude Code / Cursor / GitHub Copilot / 妙手 / 通义灵码 / Tabnine）的通用入口。
> 详细条款见 `CLAUDE.md`，AI 专用规则见 `docs/ai-rules/`。

## 项目快速摘要

云顶追踪 v2.0 — Go + Vue 3 日志检索分析引擎，AI 驱动排障定位。

## 关键规则速查

| 规则 | 强制 | 说明 |
|------|------|------|
| Store 导出名 | ✅ | `useXxxStorage`，包装 `useXxx()` |
| 请求结构体 | ✅ | `Req`/`Resp` 后缀 |
| 错误处理 | ✅ | 禁止 `_ =` 忽略错误 |
| 文件路径 | ✅ | 必须 `pkg/sanitize.SafeFilename` |
| SQL | ✅ | `?` 参数化，禁止拼接 |
| goroutine | ✅ | ctx 取消 + select |
| 常量 | ✅ | `UPPER_SNAKE_CASE` |
| API 响应格式 | ✅ | `pkg/api.Success` / `Fail` / `Paginated` |

## 文档索引

- `CLAUDE.md` — 完整项目上下文 + 技术栈声明
- `docs/云顶编码规范.md` — 强制性编码公约（人+AI 共同遵守）
- `docs/AI代码生成规约.md` — AI 行为约束协议
- `docs/ai-rules/P0-blockers.md` — P0 不可违反规则
- `docs/ai-rules/anti-patterns.md` — 反模式清单
- `docs/ai-rules/project-context.md` — AI 注入用项目上下文
