package logs

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/util"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/httplib"
)

// LogsController 日志查询控制器
type LogsController struct {
	beego.Controller
}

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

// Query 日志查询：支持单客户端和多客户端并发查询
func (this *LogsController) Query() {
	var logsReq LogsReq
	data := this.Ctx.Input.RequestBody
	err := json.Unmarshal(data, &logsReq)
	if err != nil {
		resp := LogsResp{"400", "请求参数错误", nil}
		this.Data["json"] = &resp
		this.ServeJSON()
		return
	}

	// 获取临时目录
	temppath := beego.AppConfig.String("temppath")
	workDir := filepath.Join(temppath, logsReq.Key)

	// 创建工作目录
	if err := util.CreateDir(workDir); err != nil {
		log.Printf("创建工作目录失败: %v", err)
		resp := LogsResp{"500", "系统错误", nil}
		this.Data["json"] = &resp
		this.ServeJSON()
		return
	}

	if logsReq.Client != 0 {
		// 单客户端查询
		client := models.ReadClient(logsReq.Client)
		url := "http://" + client.Ip + ":" + client.Port + "/file/query"
		item := models.ReadItem(logsReq.Item)
		path := item.LogPath + item.LogPrefix + logsReq.Date + item.LogSuffix + ".log"

		req := httplib.Post(url).Debug(true).SetTimeout(120*time.Second, 120*time.Second)
		req.JSONBody(map[string]interface{}{"path": path, "key": logsReq.Key, "line": logsReq.Line})
		req.ToFile(filepath.Join(workDir, client.Ip+".zip"))
	} else {
		// 多客户端并发查询
		clients, err := models.QueryAllClient()
		if err != nil || len(clients) == 0 {
			resp := LogsResp{"404", "无可用客户端", nil}
			this.Data["json"] = &resp
			this.ServeJSON()
			return
		}

		log.Printf("%s 多客户端并发查询开始，共 %d 个客户端\n", time.Now().Format("2006-01-02 15:04:05"), len(clients))

		// 局部 WaitGroup，避免全局状态并发问题
		var wg sync.WaitGroup
		wg.Add(len(clients))

		for i := 0; i < len(clients); i++ {
			go func(idx int, c models.TClient) {
				defer wg.Done()
				url := "http://" + c.Ip + ":" + c.Port + "/file/query"
				item := models.ReadItem(logsReq.Item)
				path := item.LogPath + item.LogPrefix + logsReq.Date + item.LogSuffix + ".log"

				log.Printf("%s 调用客户端 %d 开始: %s\n", time.Now().Format("2006-01-02 15:04:05"), idx, c.Ip)
				req := httplib.Post(url).Debug(true).SetTimeout(120*time.Second, 120*time.Second)
				req.JSONBody(map[string]interface{}{"path": path, "key": logsReq.Key, "line": logsReq.Line})
				req.ToFile(filepath.Join(workDir, c.Ip+".zip"))
				log.Printf("%s 调用客户端 %d 结束: %s\n", time.Now().Format("2006-01-02 15:04:05"), idx, c.Ip)
			}(i, clients[i])
		}

		wg.Wait()
		log.Printf("%s 多客户端并发查询结束\n", time.Now().Format("2006-01-02 15:04:05"))
	}

	// 压缩所有结果
	zipFile := filepath.Join(temppath, logsReq.Key+".zip")
	if err := util.Zip(zipFile, workDir); err != nil {
		log.Printf("压缩结果失败: %v", err)
		resp := LogsResp{"500", "压缩结果失败", nil}
		this.Data["json"] = &resp
		this.ServeJSON()
		return
	}

	defer func() {
		os.Remove(zipFile)
		os.RemoveAll(workDir)
	}()

	this.Ctx.Output.Download(zipFile)
}

// QueryClient 查询所有客户端列表
func (this *LogsController) QueryClient() {
	clients, _ := models.QueryAllClient()
	data := LogsResp{"200", "查询客户端列表成功", clients}
	this.Data["json"] = &data
	this.ServeJSON()
}

// QueryItem 根据客户端ID查询项目日志
func (this *LogsController) QueryItem() {
	clientId, _ := this.GetInt64("client_id")
	items, _ := models.QueryItemsByClientId(clientId)
	data := LogsResp{"200", "根据客户端ID查询项目日志成功", items}
	this.Data["json"] = &data
	this.ServeJSON()
}
