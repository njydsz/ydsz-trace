// Package logs 包含日志查询控制器：并发拉取多节点日志并打包返回。
package logs

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/metrics"
	"ydsz-trace/pkg/util"

	"github.com/gin-gonic/gin"
)

// LogsReq 日志查询请求体。
type LogsReq struct {
	Client    int64  `json:"client"`
	Item      int64  `json:"item"`
	Date      string `json:"date"`
	Key       string `json:"key"`
	Line      int64  `json:"line"`
	Regex     bool   `json:"regex"`
	Level     string `json:"level"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	// Query 可选布尔查询表达式（field:value AND/OR/NOT，隐式 AND）。
	// 与 Key 并用时取交集。空表示不使用布尔查询。
	Query string `json:"query"`
}

// httpClient 共享 HTTP 客户端（带连接池，120 秒超时）。
var httpClient = util.NewClientWithTimeout(120 * time.Second)

// errNonOK 构造非 200 状态码错误。
func errNonOK(status int) error {
	return &statusError{status: status}
}

// statusError HTTP 状态码错误类型。
type statusError struct {
	status int
}

func (e *statusError) Error() string {
	return "logc 返回非 200 状态码: " + strconv.Itoa(e.status)
}

// postToLogc 调用 logc 客户端 /file/query 并保存返回的 zip 到 savePath。
// regex/level/startTime/endTime/query 为可选的高级搜索参数。
func postToLogc(server, path, key string, line int64, savePath string, regex bool, level, startTime, endTime, query string) error {
	body, err := json.Marshal(map[string]interface{}{
		"path": path, "key": key, "line": line,
		"regex": regex, "level": level,
		"startTime": startTime, "endTime": endTime,
		"query": query,
	})
	if err != nil {
		return err
	}

	url := "http://" + server + "/file/query"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errNonOK(resp.StatusCode)
	}

	// 保存 zip 到本地
	out, err := os.Create(savePath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// Query 日志查询入口：
//
//   - client != 0：单客户端同步查询
//   - client == 0：多客户端并发查询（限流 20 并发，整体 5 分钟超时）
//
// 返回：zip 文件（application/octet-stream）。
func Query(c *gin.Context) {
	cfg := c.MustGet("cfg").(*config.Config)

	// 记录查询指标
	metrics.Global().QueryStarted()
	queryStart := time.Now()
	defer func() {
		duration := time.Since(queryStart)
		// 根据响应状态判断成功/失败（2xx = 成功）
		if c.Writer.Status() < 400 {
			metrics.Global().QuerySucceeded(duration)
		} else {
			metrics.Global().QueryFailed(duration)
		}
	}()

	var logsReq LogsReq
	data, err := c.GetRawData()
	if err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}
	err = json.Unmarshal(data, &logsReq)
	if err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}

	// 获取临时目录
	temppath := cfg.StringOr("temppath", "./temp/logs/")
	workDir := filepath.Join(temppath, logsReq.Key)

	// 创建工作目录
	if err := util.CreateDir(workDir); err != nil {
		log.Printf("创建工作目录失败: %v", err)
		api.Fail(c, api.CodeServerError, "系统错误")
		return
	}

	if logsReq.Client != 0 {
		// 单客户端查询
		client := models.ReadClient(logsReq.Client)
		server := client.Ip + ":" + client.Port
		item := models.ReadItem(logsReq.Item)
		path := item.LogPath + item.LogPrefix + logsReq.Date + item.LogSuffix + ".log"

		err := postToLogc(server, path, logsReq.Key, logsReq.Line, filepath.Join(workDir, client.Ip+".zip"),
			logsReq.Regex, logsReq.Level, logsReq.StartTime, logsReq.EndTime, logsReq.Query)
		if err != nil {
			log.Printf("调用客户端 %s 失败: %v", server, err)
		}
	} else {
		// 多客户端并发查询（带并发限流）
		clients, err := models.QueryAllClient()
		if err != nil || len(clients) == 0 {
			api.Fail(c, api.CodeNotFound, "无可用客户端")
			return
		}

		log.Printf("%s 多客户端并发查询开始，共 %d 个客户端\n", time.Now().Format("2006-01-02 15:04:05"), len(clients))

		// 并发限流：同时最多查询 maxConcurrentClients 个客户端
		const maxConcurrentClients = 20
		sem := make(chan struct{}, maxConcurrentClients)

		// 使用带超时的 context，整体查询不超过 5 分钟
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		var wg sync.WaitGroup
		var mu sync.Mutex
		successCount := 0
		failCount := 0

		for i := 0; i < len(clients); i++ {
			// 检查是否整体超时
			select {
			case <-ctx.Done():
				log.Printf("查询整体超时，跳过剩余 %d 个客户端", len(clients)-i)
				goto waitClients
			default:
			}

			// 获取信号量（限流）
			sem <- struct{}{}
			wg.Add(1)

			go func(idx int, cl models.TClient) {
				defer wg.Done()
				defer func() { <-sem }() // 释放信号量

				item := models.ReadItem(logsReq.Item)
				if item.Id == 0 {
					log.Printf("读取项目[%d]失败: 项目不存在", logsReq.Item)
					mu.Lock()
					failCount++
					mu.Unlock()
					return
				}
				path := item.LogPath + item.LogPrefix + logsReq.Date + item.LogSuffix + ".log"
				serverAddr := cl.Ip + ":" + cl.Port

				log.Printf("%s 调用客户端 %d 开始: %s\n", time.Now().Format("2006-01-02 15:04:05"), idx, cl.Ip)
				err := postToLogc(serverAddr, path, logsReq.Key, logsReq.Line, filepath.Join(workDir, cl.Ip+".zip"),
					logsReq.Regex, logsReq.Level, logsReq.StartTime, logsReq.EndTime, logsReq.Query)
				mu.Lock()
				if err != nil {
					failCount++
					log.Printf("%s 调用客户端 %d 失败: %s, err: %v\n", time.Now().Format("2006-01-02 15:04:05"), idx, cl.Ip, err)
				} else {
					successCount++
				}
				mu.Unlock()
				log.Printf("%s 调用客户端 %d 结束: %s\n", time.Now().Format("2006-01-02 15:04:05"), idx, cl.Ip)
			}(i, clients[i])
		}

	waitClients:
		wg.Wait()
		log.Printf("%s 多客户端并发查询结束，成功: %d, 失败: %d\n",
			time.Now().Format("2006-01-02 15:04:05"), successCount, failCount)
	}

	// 压缩所有结果
	zipFile := filepath.Join(temppath, logsReq.Key+".zip")
	if err := util.Zip(zipFile, workDir); err != nil {
		log.Printf("压缩结果失败: %v", err)
		api.Fail(c, api.CodeServerError, "压缩结果失败")
		return
	}

	defer func() {
		os.Remove(zipFile)
		os.RemoveAll(workDir)
	}()

	c.Header("Content-Disposition", `attachment; filename="`+logsReq.Key+`.zip"`)
	c.File(zipFile)
}

// QueryClient 查询所有客户端列表（供前端下拉框使用）。
func QueryClient(c *gin.Context) {
	clients, err := models.QueryAllClient()
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询客户端列表失败")
		return
	}
	api.Success(c, "查询客户端列表成功", clients)
}

// QueryItem 根据 client_id 查询其下的日志项列表。
func QueryItem(c *gin.Context) {
	clientId, _ := strconv.ParseInt(c.Query("client_id"), 10, 64)
	items, err := models.QueryItemsByClientId(clientId)
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询项目日志失败")
		return
	}
	api.Success(c, "查询项目日志成功", items)
}
