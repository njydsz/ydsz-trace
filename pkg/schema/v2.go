// Package schema v2.0 跨模块数据模型定义（DTO）。
//
// 定义 v2.0 所涉全部传输对象：日志模板、字段元数据、搜索查询计划、Trace 跨度、时间桶等。
// 此文件与 schema.go（v1.0 基础包声明）共同构成 schema 包。
package schema

// LogTemplate 日志模板（Drain 产出）。
type LogTemplate struct {
	// ID fnv hash（字符串形式，兼容前端大整数）。
	ID string `json:"id"`
	// Pattern 模板（参数替换为 <*>）。
	Pattern string `json:"pattern"`
	// ParamIdx 参数位置（递增）。
	ParamIdx []int `json:"paramIdx"`
	// Count 累计命中。
	Count int `json:"count"`
	// FirstTime 首次命中。
	FirstTime string `json:"firstTime"`
	// LastTime 最近命中。
	LastTime string `json:"lastTime"`
}

// LogField 抽取字段元数据。
type LogField struct {
	// Name 字段名。
	Name string `json:"name"`
	// Type 值类型：string/int/float/bool/timestamp/uuid。
	Type string `json:"type"`
}

// SearchQuery DSL 编译后的查询计划（供 logs 内部流转）。
type SearchQuery struct {
	// Level 日志级别过滤。
	Level string `json:"level"`
	// Key 关键词。
	Key string `json:"key"`
	// IsRegex 是否为正则查询。
	IsRegex bool `json:"isRegex"`
	// ClientID 指定客户端（0=全部）。
	ClientID int64 `json:"clientId"`
	// StartTime 起始时间（ISO）。
	StartTime string `json:"startTime"`
	// EndTime 结束时间（ISO）。
	EndTime string `json:"endTime"`
	// Filters 扩展过滤条件。
	Filters map[string]string `json:"filters"`
	// PageNo 页码（1-based）。
	PageNo int `json:"pageNo"`
	// PageSize 每页大小。
	PageSize int `json:"pageSize"`
}

// TraceSpan Trace 跨度（TraceID 聚合单元）。
type TraceSpan struct {
	// Node 来源节点。
	Node string `json:"node"`
	// Timestamp 跨度时间戳（ISO）。
	Timestamp string `json:"timestamp"`
	// Level 级别。
	Level string `json:"level"`
	// Content 跨度原始内容。
	Content string `json:"content"`
	// Service 服务名。
	Service string `json:"service"`
	// Thread 线程名。
	Thread string `json:"thread"`
	// IsError 是否错误跨度。
	IsError bool `json:"isError"`
}

// TimeBucket 时间桶聚合（用于直方图）。
type TimeBucket struct {
	// Time bucket 起始（ISO）。
	Time string `json:"time"`
	// Total 该桶总数。
	Total int `json:"total"`
	// ByLevel level -> count 映射。
	ByLevel map[string]int `json:"byLevel"`
}
