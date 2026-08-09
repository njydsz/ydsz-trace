// Package logs 子文件：traceId 调用链聚合。
//
// 给定 traceId，并发检索所有 logc 节点对应日志文件，按节点分组返回命中行，
// 便于排查跨节点调用链异常。底层复用 /logs/search 同源的 queryOneLogc 管道，
// 使用布尔查询 `traceId:<value>` 让 logc 侧识别结构化字段（若日志行未显式
// 出现 traceId 字段名，logc 会退化为全文包含匹配）。
package logs

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// traceReq traceId 调用链聚合请求体。
type traceReq struct {
	// TraceID 要检索的 traceId（必填）。
	TraceID string `json:"traceId"`
	// Date 日志日期（YYYYMMDD，必填）。
	Date string `json:"date"`
	// Item 日志项 ID（0 表示使用 client 对应的全部配置，但不推荐）。
	Item int64 `json:"item"`
	// Client 客户端 ID（0 表示全部客户端并发检索）。
	Client int64 `json:"client"`
	// Regex 是否把 traceId 作为正则表达式匹配（默认 false = 精确包含）。
	Regex bool `json:"regex"`
	// MaxPerNode 单节点返回行上限（0 使用默认值 5000）。
	MaxPerNode int `json:"maxPerNode"`
}

// traceNodeResult 单个节点的命中结果。
type traceNodeResult struct {
	Node  string   `json:"node"`  // ip:port
	Count int      `json:"count"` // 命中行数
	Lines []string `json:"lines"` // 命中行（按原始行序）
}

// traceResp 聚合响应。
type traceResp struct {
	TraceID string            `json:"traceId"` // 实际检索的 traceId
	Nodes   int               `json:"nodes"`   // 参与检索的节点总数
	Total   int               `json:"total"`   // 全部节点命中行总和
	Results []traceNodeResult `json:"results"` // 各节点命中明细
}

// Trace 端点：按 traceId 跨节点检索日志，返回按节点分组的调用链片段。
// POST /logs/trace
func Trace(c *gin.Context) {
	var req traceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}
	req.TraceID = strings.TrimSpace(req.TraceID)
	if req.TraceID == "" || req.Date == "" {
		api.Fail(c, api.CodeBadRequest, "traceId 与 date 均为必填")
		return
	}
	if req.Item == 0 {
		api.Fail(c, api.CodeBadRequest, "item 未指定")
		return
	}

	const defaultMaxPerNode = 5000
	maxPerNode := req.MaxPerNode
	if maxPerNode <= 0 || maxPerNode > defaultMaxPerNode {
		maxPerNode = defaultMaxPerNode
	}

	item := models.ReadItem(req.Item)
	if item.Id == 0 {
		api.Fail(c, api.CodeNotFound, "项目不存在")
		return
	}
	path := item.LogPath + item.LogPrefix + req.Date + item.LogSuffix + ".log"

	// 构造 logc searchReq：使用布尔查询 `traceId:<value>`。
	// logc 侧会识别 traceId 字段，未命中字段名时退化为全文包含。
	searchReq := searchReq{
		Client:   req.Client,
		Item:     req.Item,
		Date:     req.Date,
		Key:      "",
		Regex:    req.Regex,
		Query:    "traceId:" + req.TraceID,
		PageNo:   1,
		PageSize: maxPerNode,
	}

	var targets []targetSpec
	if req.Client != 0 {
		cl := models.ReadClient(req.Client)
		if cl.Id == 0 {
			api.Fail(c, api.CodeNotFound, "客户端不存在")
			return
		}
		targets = []targetSpec{{server: cl.Ip + ":" + cl.Port, label: cl.Ip + ":" + cl.Port}}
	} else {
		clients, err := models.QueryAllClient()
		if err != nil || len(clients) == 0 {
			api.Fail(c, api.CodeNotFound, "无可用客户端")
			return
		}
		for _, cl := range clients {
			if cl.Id == 0 {
				continue
			}
			targets = append(targets, targetSpec{server: cl.Ip + ":" + cl.Port, label: cl.Ip + ":" + cl.Port})
		}
	}
	if len(targets) == 0 {
		api.Fail(c, api.CodeNotFound, "未找到有效客户端")
		return
	}

	// 并发检索所有节点
	var mu sync.Mutex
	results := make([]traceNodeResult, 0, len(targets))
	var wg sync.WaitGroup
	var total atomic.Int64
	sem := make(chan struct{}, 20)

	for _, tgt := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(t targetSpec) {
			defer wg.Done()
			defer func() { <-sem }()

			lines, err := queryTraceFromLogc(t.server, path, searchReq)
			if err != nil {
				log.Printf("trace 查询节点 %s 失败: %v", t.server, err)
				return
			}
			if len(lines) == 0 {
				return
			}
			mu.Lock()
			results = append(results, traceNodeResult{
				Node:  t.label,
				Count: len(lines),
				Lines: lines,
			})
			mu.Unlock()
			total.Add(int64(len(lines)))
		}(tgt)
	}
	wg.Wait()

	c.JSON(http.StatusOK, api.Response{
		Code:    api.CodeSuccess,
		Message: "查询成功",
		Data: traceResp{
			TraceID: req.TraceID,
			Nodes:   len(targets),
			Total:   int(total.Load()),
			Results: results,
		},
	})
}

// queryTraceFromLogc 调用单个 logc 节点检索 traceId 匹配行。
// 复用 searchReq 结构；maxLines 通过 PageSize 透传。
func queryTraceFromLogc(server string, path string, req searchReq) ([]string, error) {
	return queryOneLogc(server, path, req, req.PageSize)
}
