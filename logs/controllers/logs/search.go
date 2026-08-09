// Package logs 子文件：在线分页搜索。
//
// 面向前端日志检索页的表格式展示：聚合多节点匹配行后切片分页，
// 避免"下载 zip → 解压 → 浏览"的性能与交互成本。
//
// 请求比 /logs/query 多 pageNo / pageSize；语义上返回 JSON 而非 octet-stream。
package logs

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// searchReq 在线分页搜索请求体。
type searchReq struct {
	Client    int64  `json:"client"`
	Item      int64  `json:"item"`
	Date      string `json:"date"`
	Key       string `json:"key"`
	Line      int64  `json:"line"`
	Regex     bool   `json:"regex"`
	Level     string `json:"level"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	// PageNo 当前页（>=1，默认 1）。
	PageNo int `json:"pageNo"`
	// PageSize 每页条数（1..200，默认 50）。
	PageSize int `json:"pageSize"`
}

// logRow 单条日志行（前端表格展示一行）。
type logRow struct {
	// Line 日志原文。
	Line string `json:"line"`
	// Source 来源节点 ip:port，便于排查单点问题。
	Source string `json:"source,omitempty"`
	nodeIdx int
}

// logcSearchResp 与 logc searchResponse 对应。
type logcSearchResp struct {
	Lines []string `json:"lines"`
	Count int      `json:"count"`
}

// Search 在线分页搜索入口：
//
//   - client != 0：单节点同步请求（保留历史使用场景）
//   - client == 0：多节点并发请求，聚合后分页
//
// 响应：api.Paginated（data.list = []logRow，data.total / pageNo / pageSize / totalPage）。
func Search(c *gin.Context) {
	var req searchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}
	if req.Item == 0 || req.Key == "" || req.Date == "" {
		api.Fail(c, api.CodeBadRequest, "item/key/date 不能为空")
		return
	}
	if req.PageNo <= 0 {
		req.PageNo = 1
	}
	if req.PageSize <= 0 || req.PageSize > 200 {
		req.PageSize = 50
	}

	item := models.ReadItem(req.Item)
	if item.Id == 0 {
		api.Fail(c, api.CodeNotFound, "项目不存在")
		return
	}
	path := item.LogPath + item.LogPrefix + req.Date + item.LogSuffix + ".log"

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

	lines := fetchAllLines(c, targets, path, req)

	// 分页切片
	total := len(lines)
	start := (req.PageNo - 1) * req.PageSize
	if start > total {
		start = total
	}
	end := start + req.PageSize
	if end > total {
		end = total
	}
	page := make([]logRow, 0, end-start)
	for _, lr := range lines[start:end] {
		page = append(page, logRow{Line: lr.Line, Source: lr.Source})
	}

	api.Paginated(c, "查询成功", page, total, req.PageNo, req.PageSize)
}

// targetSpec 单个待查询的 logc 节点。
type targetSpec struct {
	server string
	label  string
}

// fetchAllLines 并发调用每个 target 的 logc /file/search，按 nodeIdx 稳定聚合。
func fetchAllLines(c *gin.Context, targets []targetSpec, path string, req searchReq) []logRow {
	var mu sync.Mutex
	all := make([]logRow, 0)
	var wg sync.WaitGroup

	// 限流：并发不超过 20。
	sem := make(chan struct{}, 20)

	for idx, t := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(nodeIdx int, tgt targetSpec) {
			defer wg.Done()
			defer func() { <-sem }()

			lines, err := queryOneLogc(tgt.server, path, req)
			if err != nil {
				log.Printf("查询节点 %s 失败: %v", tgt.server, err)
				return
			}
			mu.Lock()
			for _, l := range lines {
				all = append(all, logRow{Line: l, Source: tgt.label, nodeIdx: nodeIdx})
			}
			mu.Unlock()
		}(idx, t)
	}
	wg.Wait()

	// 多节点按 nodeIdx 排序，同节点内按原始行序。
	sortLogRows(all)
	return all
}

// sortLogRows 简单稳定排序：nodeIdx 升序；同 nodeIdx 内保持插入顺序。
// 采用计数式分段法（节点数量通常不大）。
func sortLogRows(rows []logRow) {
	if len(rows) <= 1 {
		return
	}
	// 统计每个 nodeIdx 的行。
	buckets := map[int][]logRow{}
	order := []int{}
	seen := map[int]bool{}
	for _, r := range rows {
		if !seen[r.nodeIdx] {
			seen[r.nodeIdx] = true
			order = append(order, r.nodeIdx)
		}
		buckets[r.nodeIdx] = append(buckets[r.nodeIdx], r)
	}
	// 按 nodeIdx 顺序回填。
	idx := 0
	// 简单选择：对每个 bucket 保持 fifo，按 nodeIdx 原始出现顺序。
	for _, nIdx := range order {
		for _, r := range buckets[nIdx] {
			rows[idx] = r
			idx++
		}
	}
}

// queryOneLogc 调用单个 logc /file/search，返回行切片。
func queryOneLogc(server, path string, req searchReq) ([]string, error) {
	if !strings.Contains(server, ":") {
		server += ":2020"
	}
	body, err := json.Marshal(map[string]interface{}{
		"path":      path,
		"key":       req.Key,
		"line":      req.Line,
		"regex":     req.Regex,
		"level":     req.Level,
		"startTime": req.StartTime,
		"endTime":   req.EndTime,
	})
	if err != nil {
		return nil, err
	}

	url := "http://" + server + "/file/search"
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, errNonOK(resp.StatusCode)
	}

	var result logcSearchResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Lines, nil
}
