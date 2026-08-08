package logs

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

// postJSONToFile 向 logc 代理发起查询请求，结果保存到目标文件
func postJSONToFile(url string, payload interface{}, dstFile string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return logError("代理返回非200状态: %d", resp.StatusCode)
	}

	f, err := os.Create(dstFile)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = copyStream(f, resp.Body)
	return err
}

// copyStream 从 Reader 拷贝到 Writer（避免直接引用 io 包别名冲突）
func copyStream(dst *os.File, src interface{ Read([]byte) (int, error) }) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return total, nil
			}
			return total, err
		}
	}
}

// logError 返回一个带日志的错误
func logError(format string, args ...interface{}) error {
	log.Printf(format, args...)
	return errBadStatus
}

// errBadStatus 非200状态错误
var errBadStatus = &statusError{}

type statusError struct{}

func (e *statusError) Error() string { return "代理返回非200状态" }

// Query 日志查询：支持单客户端和多客户端并发查询
func Query(c *gin.Context) {
	cfg := c.MustGet("cfg").(*config.Config)

	var logsReq LogsReq
	if err := c.ShouldBindJSON(&logsReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "400", "msg": "请求参数错误", "data": nil})
		return
	}

	// 获取临时目录
	temppath := cfg.StringOr("temppath", "./temp/logs/")
	workDir := filepath.Join(temppath, logsReq.Key)

	// 创建工作目录
	if err := util.CreateDir(workDir); err != nil {
		log.Printf("创建工作目录失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "系统错误", "data": nil})
		return
	}

	if logsReq.Client != 0 {
		// 单客户端查询
		client, err := models.ReadClient(logsReq.Client)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": "404", "msg": "客户端不存在", "data": nil})
			return
		}
		url := "http://" + client.Ip + ":" + client.Port + "/file/query"
		item, err := models.ReadItem(logsReq.Item)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": "404", "msg": "项目不存在", "data": nil})
			return
		}
		path := item.LogPath + item.LogPrefix + logsReq.Date + item.LogSuffix + ".log"

		if err := postJSONToFile(url, map[string]interface{}{"path": path, "key": logsReq.Key, "line": logsReq.Line},
			filepath.Join(workDir, client.Ip+".zip")); err != nil {
			log.Printf("单客户端查询失败: %v", err)
			c.JSON(http.StatusBadGateway, gin.H{"code": "502", "msg": "客户端查询失败", "data": nil})
			return
		}
	} else {
		// 多客户端并发查询
		clients, err := models.QueryAllClient()
		if err != nil || len(clients) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"code": "404", "msg": "无可用客户端", "data": nil})
			return
		}

		log.Printf("%s 多客户端并发查询开始，共 %d 个客户端\n", time.Now().Format("2006-01-02 15:04:05"), len(clients))

		// 局部 WaitGroup，避免全局状态并发问题
		var wg sync.WaitGroup
		wg.Add(len(clients))

		for i := 0; i < len(clients); i++ {
			go func(idx int, cl models.TClient) {
				defer wg.Done()
				url := "http://" + cl.Ip + ":" + cl.Port + "/file/query"
				item, err := models.ReadItem(logsReq.Item)
				if err != nil {
					log.Printf("读取项目[%d]失败: %v", logsReq.Item, err)
					return
				}
				path := item.LogPath + item.LogPrefix + logsReq.Date + item.LogSuffix + ".log"

				log.Printf("%s 调用客户端 %d 开始: %s\n", time.Now().Format("2006-01-02 15:04:05"), idx, cl.Ip)
				if err := postJSONToFile(url, map[string]interface{}{"path": path, "key": logsReq.Key, "line": logsReq.Line},
					filepath.Join(workDir, cl.Ip+".zip")); err != nil {
					log.Printf("调用客户端 %d 失败: %v", idx, err)
					return
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
		c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "压缩结果失败", "data": nil})
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
	clients, err := models.QueryAllClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "查询客户端列表失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "查询客户端列表成功", "data": clients})
}

// QueryItem 根据客户端ID查询项目日志
func QueryItem(c *gin.Context) {
	clientId, err := strconvParseInt(c.Query("client_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "400", "msg": "参数错误", "data": nil})
		return
	}
	items, err := models.QueryItemsByClientId(clientId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "查询项目日志失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "根据客户端ID查询项目日志成功", "data": items})
}

// strconvParseInt 解析字符串为 int64
func strconvParseInt(s string) (int64, error) {
	var v int64
	_, err := fmtSscan(s, &v)
	return v, err
}
