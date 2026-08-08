package task

import (
	"encoding/json"
	"log"
	"time"

	"ydsz-trace/logs/models"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/httplib"
	"github.com/astaxie/beego/toolbox"
)

// Resp 探测响应
type Resp struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
}

// task 定时检测所有客户端在线状态
func task() error {
	log.Printf("定时检测客户端是否在线 Time: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	clients, err := models.QueryAllClient()
	if err != nil {
		log.Printf("orm query all client err: %v", err)
		return err
	}

	for _, client := range clients {
		logcServer := client.Ip + ":" + client.Port
		req := httplib.Get("http://" + logcServer + "/checkOn").SetTimeout(10*time.Second, 10*time.Second)
		str, err := req.String()
		if err != nil {
			log.Printf("check logc Online status err: %v", err)
			// 探测失败，标记为离线
			updateOnline(client.Id, "0")
			continue
		}

		var resp Resp
		// 字符串转结构体
		if err := json.Unmarshal([]byte(str), &resp); err != nil {
			log.Printf("解析探测响应失败: %v", err)
			updateOnline(client.Id, "0")
			continue
		}

		if "200" == resp.Code {
			log.Printf("check logc Online status resp code: %v", resp.Code)
			updateOnline(client.Id, "1")
		} else {
			log.Printf("check logc Online status resp code: %v", resp.Code)
			updateOnline(client.Id, "0")
		}
	}
	return nil
}

// updateOnline 更新客户端在线状态
func updateOnline(clientId int64, online string) {
	c := models.TClient{}
	c.Id = clientId
	c.Online = online
	_, err := models.ChangeClientOnline(&c)
	if err != nil {
		log.Printf("更新客户端[%d]在线状态失败: %v", clientId, err)
	}
}

// InitTask 初始化定时任务
func InitTask() {
	cron := beego.AppConfig.String("cron")
	tk := toolbox.NewTask("task", cron, task)
	toolbox.AddTask("task", tk)
}
