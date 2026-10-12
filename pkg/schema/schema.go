// Package schema 定义 v2.0 数据模型。
//
// 命名约定（《云顶编码规范》§16.2）：
//   - 原始段：LogSegment（JSON: logSegment / DB: t_log_segment）
//   - 模板：LogTemplate（JSON: logTemplate / DB: t_log_template）
//   - 事件/行级：LogEvent（JSON: logEvent / DB: t_log_event）
//   - 模式计数：PatternStat（JSON: patternStat / DB: t_pattern_stat）
//   - 向量索引：Embedding（JSON: embedding）
//   - 异常事件：AnomalyEvent（JSON: anomalyEvent）
//   - 链路：Trace / Span（JSON: trace / span）
//   - 诊断会话：Conversation / Message（JSON: conversation / message）
//   - 报告：InsightReport（JSON: insightReport）
//   - 反馈：Feedback（JSON: feedback）
//
// 时间字段统一使用 RFC3339 字符串；模板 ID 使用 uint64（JSON 中为字符串）。
package schema
