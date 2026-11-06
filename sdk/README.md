# ydsz-trace SDK

面向业务方接入云顶追踪的三语言客户端，统一封装 HTTP API、鉴权与重试。

## 支持语言

| 语言 | 目录 | 包名 / 坐标 |
|------|------|--------------|
| Go | `go/` | `ydsz-trace-go-sdk` |
| Java | `java/` | `com.njydsz:ydsz-trace-sdk` |
| Python | `python/` | `ydsz-trace-sdk` |

## 设计原则

**统一 API 表面**：三个 SDK 暴露相同的方法集合，语义一一对应：

```
SearchLogs     按条件检索日志
GetContext     上下文回溯
GetTrace       链路查询
Nl2DslParse    自然语言解析
SemanticSearch 语义检索
ListTemplates  模板列表
GetAnomalies   异常事件
ReasonIncident 根因推理
```

**统一错误模型**：非 200 或业务码非 0 时抛出 `APIError`，包含 `code` / `message` / `traceId`。

**统一重试策略**：可重试错误（网络错误、429、5xx）指数退避重试，默认 2 次。

**统一鉴权**：优先使用 `YDSZ_TOKEN` 环境变量，其次构造参数。

## 快速开始

### Go

```go
client := ydsz.NewClient("http://localhost:2021", "your-token")

resp, err := client.SearchLogs(ctx, &ydsz.SearchRequest{
    Item:  1,
    Date:  "20261106",
    Key:   "timeout",
    Level: "ERROR",
})
if err != nil {
    log.Fatal(err)
}
for _, line := range resp.Lines {
    fmt.Println(line)
}
```

### Java

```java
YdszClient client = YdszClient.builder()
    .baseUrl("http://localhost:2021")
    .token("your-token")
    .build();

SearchResponse resp = client.searchLogs(SearchRequest.builder()
    .item(1L)
    .date("20261106")
    .key("timeout")
    .level("ERROR")
    .build());

resp.getLines().forEach(System.out::println);
```

### Python

```python
from ydsz_trace import YdszClient

client = YdszClient("http://localhost:2021", token="your-token")

resp = client.search_logs(item=1, date="20261106", key="timeout", level="ERROR")
for line in resp["lines"]:
    print(line)
```

## 各语言构建

```bash
# Go
cd go && go test ./...

# Java
cd java && mvn -q package

# Python
cd python && python -m pytest
```
