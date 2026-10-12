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
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ydsz-trace/logs/controllers/audit"
	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/cache"
	"ydsz-trace/pkg/session"

	"github.com/gin-gonic/gin"
)

// queryCache 全局 LRU 缓存实例，缓存历史日期查询结果（当日数据会实时追加，不缓存）。
var queryCache = cache.NewQueryCache(200, 30*time.Second)

// InvalidateItemCache 按 item ID 清除全部缓存项（供 item 变更时调用）。
func InvalidateItemCache(itemID int64) {
	queryCache.InvalidateByItem(itemID)
}

// SearchReq 在线分页搜索请求体。
type SearchReq struct {
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

// SearchRespExt 扩展分页响应：在 api.Paginated 标准字段上追加 truncated、taskNo 与模板聚类。
type SearchRespExt struct {
	TaskNo         string          `json:"taskNo"`
	List           []logRow        `json:"list"`
	Total          int             `json:"total"`
	PageNo         int             `json:"pageNo"`
	PageSize       int             `json:"pageSize"`
	TotalPage      int             `json:"totalPage"`
	Truncated      bool            `json:"truncated"`
	TemplateGroups []templateGroup `json:"templateGroups"`
}

// templateGroup 模板聚类段：共享同一模板的命中行折叠展示。
type templateGroup struct {
	TemplateID  string   `json:"templateId"`  // 模板 hash（去掉参数后的稳定标识）
	Template    string   `json:"template"`    // 模板串（参数位用 <*> 占位）
	Count       int      `json:"count"`       // 该模板下的命中行数
	SampleLines []string `json:"sampleLines"` // 最多 3 条示例行
	FirstSeen   string   `json:"firstSeen"`   // 该模板下首次命中行
	Level       string   `json:"level"`       // 模板级别（取该组第一行的级别字段）
}

// logRow 单条日志行（前端表格展示一行）。
type logRow struct {
	// Line 日志原文。
	Line string `json:"line"`
	// Source 来源节点 ip:port，便于排查单点问题。
	Source  string `json:"source,omitempty"`
	nodeIdx int
}

// LogcSearchResp 与 logc searchResponse 对应。
type LogcSearchResp struct {
	Lines []string `json:"lines"`
	Count int      `json:"count"`
}

// Search 在线分页搜索入口：
//
//   - client != 0：单节点同步请求（保留历史使用场景）
//   - client == 0：多节点并发请求，聚合后分页
//
// @Summary 在线分页检索
// @Description 跨节点并发搜索日志行，聚合后分页返回 JSON 行列表。支持关键词、正则、布尔查询、日志级别、时间范围过滤。
// @Tags LogSearch
// @Accept json
// @Produce json
// @Param body body SearchReq true "检索条件：item + date 必填；key 与 query 不可同时为空"
// @Success 200 {object} api.Response{data=SearchRespExt} "检索成功，list = []logRow"
// @Failure 400 {object} api.Response "参数错误（item/date 为空 或 key+query 同时为空）"
// @Failure 404 {object} api.Response "项目不存在"
// @Router /logs/search [post]
func Search(c *gin.Context) {
	var req SearchReq
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

	// 缓存命中检查：仅对历史日期（非当日）的查询进行缓存。
	today := time.Now().Format("20060102")
	isHistorical := req.Date != today
	if isHistorical {
		cacheKey := cache.BuildCacheKey(req.Item, req.Date, req.Key, req.Query, req.Regex)
		if cached, ok := queryCache.Get(cacheKey); ok {
			if resp, valid := cached.(SearchRespExt); valid {
				// 审计埋点：搜索操作（缓存命中）
				audit.RecordAudit(c, "search", "/logs/search", map[string]interface{}{
					"itemId":     req.Item,
					"clientId":   req.Client,
					"date":       req.Date,
					"key":        req.Key,
					"query":      req.Query,
					"matchCount": resp.Total,
					"cached":     true,
				}, "success")
				c.JSON(http.StatusOK, api.Response{
					Code:    api.CodeSuccess,
					Message: "查询成功（缓存命中）",
					Data:    resp,
				})
				return
			}
		}
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

	// 模板聚类：对全量命中行做模板分组（最多处理前 2000 行）。
	templateGroups := groupByTemplate(lines, 2000)

	resp := SearchRespExt{
		TaskNo:         taskNo,
		List:           page,
		Total:          total,
		PageNo:         req.PageNo,
		PageSize:       req.PageSize,
		TotalPage:      totalPage,
		Truncated:      truncated,
		TemplateGroups: templateGroups,
	}

	// 缓存写入：仅缓存历史日期（非当日）的查询结果，当日数据会实时追加。
	if isHistorical {
		cacheKey := cache.BuildCacheKey(req.Item, req.Date, req.Key, req.Query, req.Regex)
		queryCache.Set(cacheKey, resp)
	}

	c.JSON(http.StatusOK, api.Response{
		Code:    api.CodeSuccess,
		Message: "查询成功",
		Data:    resp,
	})

	// 审计埋点：搜索操作
	audit.RecordAudit(c, "search", "/logs/search", map[string]interface{}{
		"itemId":     req.Item,
		"clientId":   req.Client,
		"date":       req.Date,
		"key":        req.Key,
		"query":      req.Query,
		"matchCount": total,
		"cached":     false,
	}, "success")
}

// targetSpec 单个待查询的 logc 节点。
type targetSpec struct {
	server string
	label  string
}

// insertSearchTask 根据 SearchReq 写入一条 running 状态的检索任务记录。
func insertSearchTask(taskNo, userName string, req SearchReq, itemName string, nodeTotal, maxLines int) error {
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
func fetchAllLines(c *gin.Context, targets []targetSpec, path string, req SearchReq, maxTotal, perNodeBudget int) ([]logRow, bool) {
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
func queryOneLogc(server, path string, req SearchReq, maxLines int) ([]string, error) {
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

	var result LogcSearchResp
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
	// 审计埋点：删除检索任务
	audit.RecordAudit(c, "delete", "/logs/tasks/:taskNo", map[string]interface{}{
		"taskNo": taskNo,
	}, "success")
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
	// 审计埋点：重试检索任务
	audit.RecordAudit(c, "retry", "/logs/tasks/:taskNo/retry", map[string]interface{}{
		"oldTaskNo": taskNo,
		"newTaskNo": newTaskNo,
	}, "success")
	api.Success(c, "已创建重试任务", map[string]string{"taskNo": newTaskNo})
}

// normalizeLine 将日志行中的参数化部分替换为占位符 <*>，得到稳定模板串。
//
// 依次替换：文件路径 / UUID / IPv4 / 数字（含浮点数），最后压缩多余空格。
// 使用包级预编译正则，避免每次调用重复编译。
var (
	normPath    = regexp.MustCompile(`(?:/[\w\-\.]+){2,}`)
	normUUID    = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	normIPv4    = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	normNumbers = regexp.MustCompile(`\b\d+\.\d+\b|\b\d+\b`)
	normHex     = regexp.MustCompile(`\b[0-9a-fA-F]{16,}\b`)
)

// normalizeLine 将一行日志中的动态参数替换为 <*> 占位，用于模板聚类。
func normalizeLine(line string) string {
	line = normPath.ReplaceAllString(line, "<*>")
	line = normUUID.ReplaceAllString(line, "<*>")
	line = normIPv4.ReplaceAllString(line, "<*>")
	line = normHex.ReplaceAllString(line, "<*>")
	line = normNumbers.ReplaceAllString(line, "<*>")
	// 去除多余空格
	line = strings.Join(strings.Fields(line), " ")
	return line
}

// hashTemplate 计算模板串的 FNV-1a hash，用作稳定模板 ID。
func hashTemplate(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return strconv.FormatUint(uint64(h.Sum32()), 16)
}

// extractLevel 从日志行中提取级别字段（INFO / WARN / ERROR / DEBUG / FATAL）。
func extractLevel(line string) string {
	upper := strings.ToUpper(line)
	for _, lvl := range []string{"FATAL", "ERROR", "WARN", "INFO", "DEBUG"} {
		if strings.Contains(upper, lvl) {
			return lvl
		}
	}
	return ""
}

// groupByTemplate 对日志行做模板分组（取前 maxLines 行做聚类）。
//
// 算法：
//  1. 取前 min(len(lines), maxLines) 行
//  2. 每行做 normalize（参数 -> <*>）得到 pattern
//  3. 用 pattern 的 fnv32a hash 作为 template ID
//  4. 按 template ID 聚合行
//  5. 按 Count 降序排列
//  6. 每组取最多 3 条示例行
func groupByTemplate(lines []logRow, maxLines int) []templateGroup {
	if len(lines) == 0 {
		return nil
	}
	// 1. 截取前 N 行
	n := len(lines)
	if n > maxLines {
		n = maxLines
	}

	// 2. 聚合：map[templateID] -> *templateGroup
	type groupAcc struct {
		template    string
		count       int
		sampleLines []string
		firstSeen   string
		level       string
	}
	m := map[string]*groupAcc{}

	for i := 0; i < n; i++ {
		rawLine := lines[i].Line
		pattern := normalizeLine(rawLine)
		tid := hashTemplate(pattern)

		g, exists := m[tid]
		if !exists {
			m[tid] = &groupAcc{
				template:    pattern,
				count:       1,
				sampleLines: []string{rawLine},
				firstSeen:   rawLine,
				level:       extractLevel(rawLine),
			}
		} else {
			g.count++
			if len(g.sampleLines) < 3 {
				g.sampleLines = append(g.sampleLines, rawLine)
			}
		}
	}

	// 3. 转切片并排序
	groups := make([]templateGroup, 0, len(m))
	for tid, acc := range m {
		groups = append(groups, templateGroup{
			TemplateID:  tid,
			Template:    acc.template,
			Count:       acc.count,
			SampleLines: acc.sampleLines,
			FirstSeen:   acc.firstSeen,
			Level:       acc.level,
		})
	}

	// 按 Count 降序排序
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Count > groups[j].Count
	})

	return groups
}
