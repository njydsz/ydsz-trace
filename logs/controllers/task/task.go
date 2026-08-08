package task

import (
	"encoding/json"
	"io"
	"log"
	"time"

	"ydsz-trace/logs/models"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/util"

	"github.com/robfig/cron/v3"
)

// Resp 探测响应
type Resp struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
}

// checkOnlineTask 定时检测所有客户端在线状态
func checkOnlineTask() {
	log.Printf("定时检测客户端是否在线 Time: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	clients, err := models.QueryAllClient()
	if err != nil {
		log.Printf("query all client err: %v", err)
		return
	}

	httpClient := util.NewClientWithTimeout(10 * time.Second)
	for _, client := range clients {
		logcServer := client.Ip + ":" + client.Port
		resp, err := httpClient.Get("http://" + logcServer + "/checkOn")
		if err != nil {
			log.Printf("check logc Online status err: %v", err)
			// 探测失败，标记为离线
			updateOnline(client.Id, "0")
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			log.Printf("读取探测响应失败: %v", readErr)
			updateOnline(client.Id, "0")
			continue
		}

		var r Resp
		// 字符串转结构体
		if err := json.Unmarshal(body, &r); err != nil {
			log.Printf("解析探测响应失败: %v", err)
			updateOnline(client.Id, "0")
			continue
		}

		if "200" == r.Code {
			log.Printf("check logc Online status resp code: %v", r.Code)
			updateOnline(client.Id, "1")
		} else {
			log.Printf("check logc Online status resp code: %v", r.Code)
			updateOnline(client.Id, "0")
		}
	}
}

// updateOnline 更新客户端在线状态
func updateOnline(clientId int64, online string) {
	c := models.TClient{}
	c.Id = clientId
	c.Online = online
	c.UpdatedTime = time.Now().Format("2006-01-02 15:04:05")
	_, err := models.ChangeClientOnline(&c)
	if err != nil {
		log.Printf("更新客户端[%d]在线状态失败: %v", clientId, err)
	}
}

// InitTask 初始化定时任务，返回 cron 实例
func InitTask(cfg *config.Config) *cron.Cron {
	c := cron.New(cron.WithSeconds())

	cronExpr := cfg.StringOr("cron", "0 0/5 * * * *")
	if _, err := c.AddFunc(cronExpr, checkOnlineTask); err != nil {
		log.Printf("注册定时任务失败: %v", err)
	}

	return c
}
