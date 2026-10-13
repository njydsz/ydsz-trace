// Package models 子文件：上下文回溯数据结构。
package models

// ContextResult 上下文回溯响应。
type ContextResult struct {
	// RefTime 匹配参考行的时间戳（回显）。
	RefTime string `json:"refTime"`
	// RefLine 匹配到的参考行原文。
	RefLine string `json:"refLine"`
	// BeforeLines 命中前的若干行（ newest-first 倒序：[ref-1, ref-2, ...]）。
	BeforeLines []string `json:"beforeLines"`
	// AfterLines 命中后的若干行（正序：[ref+1, ref+2, ...]）。
	AfterLines []string `json:"afterLines"`
	// TotalBefore 命中前行数（不含参考行本身）。
	TotalBefore int `json:"totalBefore"`
	// TotalAfter 命中后行数（不含参考行本身）。
	TotalAfter int `json:"totalAfter"`
}

// ContextReq 上下文回溯请求体（GET query 绑定）。
type ContextReq struct {
	// ClientID 客户端 ID（可 0）。
	ClientID int64 `form:"clientId"`
	// RefTime 参考时间（ISO8601 / RFC3339 / "2026-01-01 10:00:00"）。
	RefTime string `form:"refTime"`
	// RefLine 参考行原文（服务端用时间匹配最近行；refLine 作为备用匹配）。
	RefLine string `form:"refLine"`
	// Before 命中前行数（默认 10，最大 100）。
	Before int `form:"before"`
	// After 命中后行数（默认 10，最大 100）。
	After int `form:"after"`
	// Date 日志日期（YYYYMMDD 或 YYYY-MM-DD），refTime 不足以确定日期时必填。
	Date string `form:"date"`
}
