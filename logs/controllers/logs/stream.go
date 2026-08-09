// Package logs 子文件：SSE 流式日志搜索与实时跟踪。
//
// 提供两个 SSE 端点：
//   - /logs/stream：多客户端并发搜索，流式返回每个节点的进度
//   - /logs/tail：代理 logc 的实时跟踪，SSE 推送新增日志行
package logs

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// StreamReq 流式搜索请求体（与 LogsReq 对齐）。
type StreamReq struct {
	Client    int64  `json:"client"`
	Item      int64  `json:"item"`
	Date      string `json:"date"`
	Key       string `json:"key"`
	Line      int64  `json:"line"`
	Regex     bool   `json:"regex"`
	Level     string `json:"level"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
	Query     string `json:"query"`
}

// Stream 多客户端并发搜索的 SSE 进度推送端点。
//
// SSE 事件流：
//   - connected：{ "total": N } —— 搜索开始
//   - progress：{ "index": i, "server": "ip:port", "status": "ok|fail" } —— 单节点完成
//   - done：{ "success": N, "fail": N } —— 全部完成
//
// 注意：流式模式下不直接下载 zip，仅推送进度与汇总结果下载链接。
func Stream(c *gin.Context) {
	var req StreamReq
	data, err := c.GetRawData()
	if err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}
	if err := json.Unmarshal(data, &req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}

	// SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	if req.Item == 0 || (req.Key == "" && req.Query == "") || req.Date == "" {
		sseError(c, "item/query+key 不能同时为空，且 date 必填")
		return
	}

	item := models.ReadItem(req.Item)
	if item.Id == 0 {
		sseError(c, "项目不存在")
		return
	}
	path := item.LogPath + item.LogPrefix + req.Date + item.LogSuffix + ".log"

	var clients []models.TClient
	if req.Client != 0 {
		clients = []models.TClient{models.ReadClient(req.Client)}
	} else {
		clients, err = models.QueryAllClient()
		if err != nil || len(clients) == 0 {
			sseError(c, "无可用客户端")
			return
		}
	}

	total := len(clients)
	sseEvent(c, "connected", map[string]interface{}{"total": total})
	c.Writer.Flush()

	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	fail := 0

	for i, cl := range clients {
		if cl.Id == 0 {
			continue
		}
		wg.Add(1)
		go func(idx int, client models.TClient) {
			defer wg.Done()
			server := client.Ip + ":" + client.Port
			sseEvent(c, "progress", map[string]interface{}{
				"index": idx, "server": server, "status": "start",
			})
			c.Writer.Flush()

			err := postToLogc(server, path, req.Key, req.Line,
				fmt.Sprintf("./temp/stream/%d.zip", client.Id),
				req.Regex, req.Level, req.StartTime, req.EndTime, req.Query)

			mu.Lock()
			status := "ok"
			if err != nil {
				status = "fail"
				fail++
				log.Printf("流式搜索节点 %s 失败: %v", server, err)
			} else {
				success++
			}
			mu.Unlock()

			sseEvent(c, "progress", map[string]interface{}{
				"index": idx, "server": server, "status": status,
			})
			c.Writer.Flush()
		}(i, cl)
	}

	wg.Wait()
	sseEvent(c, "done", map[string]interface{}{
		"success": success, "fail": fail,
	})
	c.Writer.Flush()
}

// Tail 代理 logc tail 端点：将 logc 的 SSE 流式响应转发给客户端。
//
// 请求参数通过 JSON body 传递：client + item + date + key + regex + level + followDuration
func Tail(c *gin.Context) {
	var req StreamReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}
	if req.Item == 0 || req.Date == "" {
		api.Fail(c, api.CodeBadRequest, "item/date 不能为空")
		return
	}

	item := models.ReadItem(req.Item)
	if item.Id == 0 {
		api.Fail(c, api.CodeNotFound, "项目不存在")
		return
	}
	path := item.LogPath + item.LogPrefix + req.Date + item.LogSuffix + ".log"

	// 确定要 tail 的客户端
	var servers []string
	if req.Client != 0 {
		cl := models.ReadClient(req.Client)
		if cl.Id != 0 {
			servers = []string{cl.Ip + ":" + cl.Port}
		}
	} else {
		clients, err := models.QueryAllClient()
		if err != nil || len(clients) == 0 {
			api.Fail(c, api.CodeNotFound, "无可用客户端")
			return
		}
		for _, cl := range clients {
			servers = append(servers, cl.Ip+":"+cl.Port)
		}
	}

	if len(servers) == 0 {
		api.Fail(c, api.CodeNotFound, "未找到有效客户端")
		return
	}

	// 单客户端：直接代理 SSE
	if len(servers) == 1 {
		proxyTailSSE(c, servers[0], path, req.Key, req.Regex, req.Level, req.Line)
		return
	}

	// 多客户端：合并 SSE 流
	mergedTailSSE(c, servers, path, req.Key, req.Regex, req.Level, req.Line)
}

// proxyTailSSE 代理单个 logc 的 tail 响应。
func proxyTailSSE(c *gin.Context, server, path, key string, regex bool, level string, line int64) {
	tailReqBody, _ := json.Marshal(map[string]interface{}{
		"path":           path,
		"key":            key,
		"regex":          regex,
		"level":          level,
		"followDuration": 60,
	})

	resp, err := httpClient.Post(
		"http://"+server+"/file/tail",
		"application/json",
		bytes.NewReader(tailReqBody),
	)
	if err != nil {
		api.Fail(c, api.CodeServerError, "连接 logc 代理失败")
		return
	}
	defer resp.Body.Close()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// 透传 SSE 流
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				log.Printf("代理读取 SSE 失败: %v", err)
			}
			return
		}
		c.Writer.WriteString(line)
		c.Writer.Flush()
	}
}

// mergedTailSSE 合并多个 logc server 的 tail 流。
func mergedTailSSE(c *gin.Context, servers []string, path, key string, regex bool, level string, line int64) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	sseEvent(c, "connected", map[string]interface{}{"servers": len(servers)})
	c.Writer.Flush()

	var wg sync.WaitGroup
	type logLine struct {
		Server string `json:"server"`
		Line   string `json:"line"`
	}
	lineCh := make(chan logLine, 100)
	doneCh := make(chan string, len(servers))

	for _, server := range servers {
		wg.Add(1)
		go func(srv string) {
			defer wg.Done()
			tailReqBody, _ := json.Marshal(map[string]interface{}{
				"path":           path,
				"key":            key,
				"regex":          regex,
				"level":          level,
				"followDuration": 60,
			})

			resp, err := httpClient.Post(
				"http://"+srv+"/file/tail",
				"application/json",
				bytes.NewReader(tailReqBody),
			)
			if err != nil {
				doneCh <- srv
				return
			}
			defer resp.Body.Close()

			reader := bufio.NewReader(resp.Body)
			for {
				data, err := reader.ReadString('\n')
				if err != nil {
					break
				}
				// 解析 SSE data
				if len(data) > 6 && data[:6] == "data: " {
					content := data[6:]
					// 去掉末尾换行
					content = strings.TrimRight(content, "\r\n")
					if len(content) > 0 {
						lineCh <- logLine{Server: srv, Line: content}
					}
				}
			}
			doneCh <- srv
		}(server)
	}

	// 等待所有 goroutine 结束后关闭 channel
	go func() {
		wg.Wait()
		close(lineCh)
	}()

	finished := 0
	for {
		select {
		case ll, ok := <-lineCh:
			if !ok {
				sseEvent(c, "done", map[string]interface{}{"servers": len(servers)})
				c.Writer.Flush()
				return
			}
			b, _ := json.Marshal(ll)
			c.Writer.WriteString("data: " + string(b) + "\n\n")
			c.Writer.Flush()
		case <-doneCh:
			finished++
			if finished >= len(servers) {
				// 排空 lineCh
				for ll := range lineCh {
					b, _ := json.Marshal(ll)
					c.Writer.WriteString("data: " + string(b) + "\n\n")
					c.Writer.Flush()
				}
				sseEvent(c, "done", map[string]interface{}{"servers": len(servers)})
				c.Writer.Flush()
				return
			}
		case <-c.Request.Context().Done():
			return
		}
	}
}

// sseEvent 发送一个 SSE 事件。
func sseEvent(c *gin.Context, event string, data interface{}) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	c.Writer.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(b)))
}

// sseError 发送 SSE 错误事件。
func sseError(c *gin.Context, msg string) {
	sseEvent(c, "error", map[string]interface{}{"message": msg})
	c.Writer.Flush()
}
