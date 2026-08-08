package logs

import (
	"bytes"
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
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/util"

	"github.com/gin-gonic/gin"
)

// LogsReq 日志查询请求
type LogsReq struct {
	Client int64  `json:"client"`
	Item   int64  `json:"item"`
	Date   string `json:"date"`
	Key    string `json:"key"`
	Line   int64  `json:"line"`
}

// LogsResp 日志查询响应
type LogsResp struct {
	Code string      `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

// httpClient 带超时的 HTTP 客户端
var httpClient = &http.Client{Timeout: 120 * time.Second}

// errNonOK 非 200 状态码错误
func errNonOK(status int) error {
	return &statusError{status: status}
}

// statusError HTTP 状态码错误
type statusError struct {
	status int
}

func (e *statusError) Error() string {
	return "logc 返回非 200 状态码: " + strconv.Itoa(e.status)
}

// postToLogc 调用 logc 客户端 /file/query 并保存 zip 文件
func postToLogc(server, path, key string, line int64, savePath string) error {
	body, err := json.Marshal(map[string]interface{}{
		"path": path, "key": key, "line": line,
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

// Query 日志查询：支持单客户端和多客户端并发查询
func Query(c *gin.Context) {
	cfg := c.MustGet("cfg").(*config.Config)

	var logsReq LogsReq
	data, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusOK, LogsResp{"400", "请求参数错误", nil})
		return
	}
	err = json.Unmarshal(data, &logsReq)
	if err != nil {
		c.JSON(http.StatusOK, LogsResp{"400", "请求参数错误", nil})
		return
	}

	// 获取临时目录
	temppath := cfg.StringOr("temppath", "./temp/logs/")
	workDir := filepath.Join(temppath, logsReq.Key)

	// 创建工作目录
	if err := util.CreateDir(workDir); err != nil {
		log.Printf("创建工作目录失败: %v", err)
		c.JSON(http.StatusOK, LogsResp{"500", "系统错误", nil})
		return
	}

	if logsReq.Client != 0 {
		// 单客户端查询
		client := models.ReadClient(logsReq.Client)
		server := client.Ip + ":" + client.Port
		item := models.ReadItem(logsReq.Item)
		path := item.LogPath + item.LogPrefix + logsReq.Date + item.LogSuffix + ".log"

		err := postToLogc(server, path, logsReq.Key, logsReq.Line, filepath.Join(workDir, client.Ip+".zip"))
		if err != nil {
			log.Printf("调用客户端 %s 失败: %v", server, err)
		}
	} else {
		// 多客户端并发查询
		clients, err := models.QueryAllClient()
		if err != nil || len(clients) == 0 {
			c.JSON(http.StatusOK, LogsResp{"404", "无可用客户端", nil})
			return
		}

		log.Printf("%s 多客户端并发查询开始，共 %d 个客户端\n", time.Now().Format("2006-01-02 15:04:05"), len(clients))

		// 局部 WaitGroup，避免全局状态并发问题
		var wg sync.WaitGroup
		wg.Add(len(clients))

		for i := 0; i < len(clients); i++ {
			go func(idx int, cl models.TClient) {
				defer wg.Done()
				server := cl.Ip + ":" + cl.Port
				item := models.ReadItem(logsReq.Item)
				path := item.LogPath + item.LogPrefix + logsReq.Date + item.LogSuffix + ".log"

				log.Printf("%s 调用客户端 %d 开始: %s\n", time.Now().Format("2006-01-02 15:04:05"), idx, cl.Ip)
				err := postToLogc(server, path, logsReq.Key, logsReq.Line, filepath.Join(workDir, cl.Ip+".zip"))
				if err != nil {
					log.Printf("%s 调用客户端 %d 失败: %s, err: %v\n", time.Now().Format("2006-01-02 15:04:05"), idx, cl.Ip, err)
				}
				log.Printf("%s 调用客户端 %d 结束: %s\n", time.Now().Format("2006-01-02 15:04:05"), idx, cl.Ip)
			}(i, clients[i])
		}

		wg.Wait()
		log.Printf("%s 多客户端并发查询结束\n", time.Now().Format("2006-01-02 15:04:05"))
	}

	// 压缩所有结果
	zipFile := filepath.Join(temppath, logsReq.Key+".zip")
	if err := util.Zip(zipFile, workDir); err != nil {
		log.Printf("压缩结果失败: %v", err)
		c.JSON(http.StatusOK, LogsResp{"500", "压缩结果失败", nil})
		return
	}

	defer func() {
		os.Remove(zipFile)
		os.RemoveAll(workDir)
	}()

	c.Header("Content-Disposition", `attachment; filename="`+logsReq.Key+`.zip"`)
	c.File(zipFile)
}

// QueryClient 查询所有客户端列表
func QueryClient(c *gin.Context) {
	clients, _ := models.QueryAllClient()
	c.JSON(http.StatusOK, LogsResp{"200", "查询客户端列表成功", clients})
}

// QueryItem 根据客户端ID查询项目日志
func QueryItem(c *gin.Context) {
	clientId, _ := strconv.ParseInt(c.Query("client_id"), 10, 64)
	items, _ := models.QueryItemsByClientId(clientId)
	c.JSON(http.StatusOK, LogsResp{"200", "根据客户端ID查询项目日志成功", items})
}
