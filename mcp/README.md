# MCP Server for Ydsz Trace

将 ydsz-trace 的日志检索、异常检测、Trace 分析、统计能力暴露为 MCP 标准工具。

## 安装

```bash
cd mcp && go build -o ydsz-mcp
```

## 工具集

| 工具名称 | 说明 |
|---------|------|
| `search_logs` | 按条件搜索日志（支持关键词、正则、级别、时间范围、布尔查询） |
| `tail_logs` | 实时追踪日志（代理 logc 实时追踪能力） |
| `get_trace` | 按 TraceID 获取调用链（自动识别 W3C/B3/SkyWalking/自定义格式） |
| `list_anomalies` | 列出最近的异常事件（模式突变 + 告警触发历史） |
| `get_stats` | 获取模式统计基线（模板命中时序数据） |

## 通信协议

- **stdio 模式**（默认，TTY 环境）：通过 JSON-RPC 2.0 与 IDE（Cursor/Claude Code）通信
- **HTTP 模式**（非 TTY 环境）：启动本机 HTTP 服务（默认端口 9527），通过 REST 调用工具

## 配置

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `YDSZ_API_BASE` | `http://localhost:2021` | ydsz-trace logs 服务地址 |
| `MCP_HTTP_PORT` | `9527` | HTTP 模式监听端口 |
| `MCP_FORCE_HTTP` | `` | 非空时强制使用 HTTP 模式 |
| `MCP_FORCE_STDIO` | `` | 非空时强制使用 stdio 模式 |

## 使用示例

### stdio 模式（IDE 集成）

```
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search_logs","arguments":{"item":1,"date":"20260101","key":"ERROR"}}}
```

### HTTP 模式（开发调试）

```
export YDSZ_API_BASE=http://localhost:2021
export MCP_FORCE_HTTP=1
./ydsz-mcp

# 列出工具
curl http://localhost:9527/mcp/list

# 调用搜索
curl -X POST http://localhost:9527/mcp/call \
  -H "Content-Type: application/json" \
  -d '{"name":"search_logs","arguments":{"item":1,"date":"20260101","key":"exception"}}'
```
