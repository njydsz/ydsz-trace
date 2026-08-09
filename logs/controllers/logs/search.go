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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/session"

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
	// MaxTotal 服务端聚合上限（0 使用默认值，用于内存保护）。
	MaxTotal int `json:"maxTotal"`
	// Query 可选布尔查询表达式（field:value AND/OR/NOT，隐式 AND）。
	// 与 Key 并用时取交集。空表示不使用布尔查询。
	Query string `json:"query"`
}

// searchRespExt 扩展分页响应：在 api.Paginated 标准字段上追加 truncated 与 taskNo。
type searchRespExt struct {
	TaskNo    string   `json:"taskNo"`
	List      []logRow `json:"list"`
	Total     int      `json:"total"`
	PageNo    int      `json:"pageNo"`
	PageSize  int      `json:"pageSize"`
	TotalPage int      `json:"totalPage"`
	Truncated bool     `json:"truncated"`
}

// logRow 单条日志行（前端表格展示一行）。
type logRow struct {
	// Line 日志原文。
	Line string `json:"line"`
	// Source 来源节点 ip:port，便于排查单点问题。
	Source  string `json:"source,omitempty"`
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
	if req.Item == 0 || (req.Key == "" && req.Query == "") || req.Date == "" {
		api.Fail(c, api.CodeBadRequest, "item/query+key 不能同时为空，且 date 必填")
		return
	}
	if req.PageNo <= 0 {
		req.PageNo = 1
	}
	if req.PageSize <= 0 || req.PageSize > 200 {
		req.PageSize = 50
	}
	// 全局限流：单请求聚合行上限（保护内存）。
	const defaultMaxTotal = 100000
	maxTotal := req.MaxTotal
	if maxTotal <= 0 || maxTotal > defaultMaxTotal {
		maxTotal = defaultMaxTotal
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

	// per-node 分片预算：总上限按节点数切分，留有冗余以应对分布不均（单节点最坏可拿到 3x 均值）。
	perNodeBudget := maxTotal / len(targets)
	if perNodeBudget < 2000 {
		perNodeBudget = 2000
	}
	if perNodeBudget > 10000 {
		perNodeBudget = 10000
	}

	// 落库检索任务（running → success/failed），便于前端轮询。
	userName := session.GetString(c, "username")
	taskNo := models.NewTaskNo()
	if err := insertSearchTask(taskNo, userName, req, item.ItemName, len(targets), maxTotal); err != nil {
		log.Printf("创建任务记录失败: %v", err)
	}
	var taskFinished bool
	var taskMatchCount int64
	defer func() {
		if taskFinished {
			return
		}
		status := models.TaskStatusSuccess
		if r := recover(); r != nil {
			status = models.TaskStatusFailed
			log.Printf("检索任务 %s panic: %v", taskNo, r)
		}
		_ = models.TouchTaskFinished(taskNo, status, "", "", taskMatchCount)
	}()

	lines, truncated := fetchAllLines(c, targets, path, req, maxTotal, perNodeBudget)
	taskMatchCount = int64(len(lines))

	// 分页切片
	total := len(lines)
	taskFinished = true
	_ = models.TouchTaskFinished(taskNo, models.TaskStatusSuccess, "", "", taskMatchCount)
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

	totalPage := (total + req.PageSize - 1) / req.PageSize
	c.JSON(http.StatusOK, api.Response{
		Code:    api.CodeSuccess,
		Message: "查询成功",
		Data: searchRespExt{
			TaskNo:    taskNo,
			List:      page,
			Total:     total,
			PageNo:    req.PageNo,
			PageSize:  req.PageSize,
			TotalPage: totalPage,
			Truncated: truncated,
		},
	})
}

// targetSpec 单个待查询的 logc 节点。
type targetSpec struct {
	server string
	label  string
}

// insertSearchTask 根据 searchReq 写入一条 running 状态的检索任务记录。
func insertSearchTask(taskNo, userName string, req searchReq, itemName string, nodeTotal, maxLines int) error {
	t := &models.TTask{
		TaskNo:    taskNo,
		UserName:  userName,
		ClientID:  req.Client,
		ItemID:    req.Item,
		ItemName:  itemName,
		LogDate:   req.Date,
		KeyWord:   req.Key,
		Regex:     req.Regex,
		Level:     req.Level,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		QueryExpr: req.Query,
		MaxLines:  maxLines,
		Status:    models.TaskStatusRunning,
		NodeTotal: nodeTotal,
	}
	if _, err := models.InsertTask(t); err != nil {
		return err
	}
	return models.TouchTaskStarted(taskNo)
}

// fetchAllLines 并发调用每个 target 的 logc /file/search，按 nodeIdx 稳定聚合。
// truncated 为 true 表示结果达到了 maxTotal 上限。
func fetchAllLines(c *gin.Context, targets []targetSpec, path string, req searchReq, maxTotal, perNodeBudget int) ([]logRow, bool) {
	var mu sync.Mutex
	all := make([]logRow, 0, maxTotal)
	var wg sync.WaitGroup
	var truncated atomic.Bool
	// 请求级别的已聚合行数计数器（避免多 goroutine 全量读取导致内存溢出）。
	var aggregated atomic.Int64

	// 限流：并发不超过 20。
	sem := make(chan struct{}, 20)

	for idx, t := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(nodeIdx int, tgt targetSpec) {
			defer wg.Done()
			defer func() { <-sem }()
			// 快速路径：已达上限时跳过剩余节点，省掉不必要的网络往返。
			if aggregated.Load() >= int64(maxTotal) {
				truncated.Store(true)
				return
			}

			lines, err := queryOneLogc(tgt.server, path, req, perNodeBudget)
			if err != nil {
				log.Printf("查询节点 %s 失败: %v", tgt.server, err)
				return
			}
			if len(lines) == 0 {
				return
			}

			mu.Lock()
			appended := 0
			for _, l := range lines {
				if len(all) >= maxTotal {
					truncated.Store(true)
					break
				}
				all = append(all, logRow{Line: l, Source: tgt.label, nodeIdx: nodeIdx})
				appended++
			}
			mu.Unlock()
			aggregated.Add(int64(appended))
		}(idx, t)
	}
	wg.Wait()

	// 多节点按 nodeIdx 稳定排序，同节点内保持 logc 原始行序。
	sortAllByNodeIdx(all)
	return all, truncated.Load()
}

// sortAllByNodeIdx 按 nodeIdx 升序稳定排序；同 nodeIdx 内保持插入顺序（节点数量通常不大）。
func sortAllByNodeIdx(rows []logRow) {
	if len(rows) <= 1 {
		return
	}
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
	idx := 0
	for _, nIdx := range order {
		for _, r := range buckets[nIdx] {
			rows[idx] = r
			idx++
		}
	}
}

// queryOneLogc 调用单个 logc /file/search，maxLines 透传给 logc 以限制单节点返回行数。
func queryOneLogc(server, path string, req searchReq, maxLines int) ([]string, error) {
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
		"maxLines":  maxLines,
		"query":     req.Query,
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

// QueryTask 按 taskNo 查询单个检索任务状态。
// GET /logs/tasks/:taskNo
func QueryTask(c *gin.Context) {
	taskNo := c.Param("taskNo")
	if taskNo == "" {
		api.Fail(c, api.CodeBadRequest, "taskNo 不能为空")
		return
	}
	task, err := models.GetTaskByNo(taskNo)
	if err != nil || task.ID == 0 {
		api.Fail(c, api.CodeNotFound, "任务不存在")
		return
	}
	api.Success(c, "查询成功", task)
}

// ListTasks 分页查询检索任务列表。
// GET /logs/tasks?page=1&pageSize=20&status=running
func ListTasks(c *gin.Context) {
	pageNo, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	statusFilter := c.Query("status")
	p := models.QueryTasks(pageNo, pageSize, statusFilter)
	api.Success(c, "查询成功", p)
}

// DeleteTask 删除一条检索任务记录。
// DELETE /logs/tasks/:taskNo
func DeleteTask(c *gin.Context) {
	taskNo := c.Param("taskNo")
	if taskNo == "" {
		api.Fail(c, api.CodeBadRequest, "taskNo 不能为空")
		return
	}
	n := models.DeleteTaskByNo(taskNo)
	if n == 0 {
		api.Fail(c, api.CodeNotFound, "任务不存在或已删除")
		return
	}
	api.Success(c, "删除成功", map[string]string{"taskNo": taskNo})
}

// RetryTask 重新触发一条失败/超时的检索任务（仅前端可再次发起调用）。
// POST /logs/tasks/:taskNo/retry 会删除旧记录并返回新的 taskNo 给前端跳转。
func RetryTask(c *gin.Context) {
	taskNo := c.Param("taskNo")
	task, err := models.GetTaskByNo(taskNo)
	if err != nil || task.ID == 0 {
		api.Fail(c, api.CodeNotFound, "任务不存在")
		return
	}
	if task.Status != models.TaskStatusFailed && task.Status != models.TaskStatusSuccess {
		api.Fail(c, api.CodeBadRequest, "仅失败或已完成的任务可重试")
		return
	}
	_ = models.DeleteTaskByNo(taskNo)
	newTaskNo := models.NewTaskNo()
	api.Success(c, "已创建重试任务", map[string]string{"taskNo": newTaskNo})
}
