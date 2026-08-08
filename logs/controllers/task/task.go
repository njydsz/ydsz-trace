package task

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/config"

	"github.com/robfig/cron/v3"
)

// Resp 探测响应
type Resp struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
}

// checkClient 探测单个客户端在线状态
func checkClient(client models.TClient) {
	logcServer := client.Ip + ":" + client.Port
	httpClient := &http.Client{Timeout: 10 * time.Second}

	resp, err := httpClient.Get("http://" + logcServer + "/checkOn")
	if err != nil {
		log.Printf("check logc Online status err: %v", err)
		updateOnline(client.Id, "0")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("check logc Online status http code: %d", resp.StatusCode)
		updateOnline(client.Id, "0")
		return
	}

	var r Resp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		log.Printf("解析探测响应失败: %v", err)
		updateOnline(client.Id, "0")
		return
	}

	if "200" == r.Code {
		log.Printf("check logc Online status resp code: %v", r.Code)
		updateOnline(client.Id, "1")
	} else {
		log.Printf("check logc Online status resp code: %v", r.Code)
		updateOnline(client.Id, "0")
	}
}

// updateOnline 更新客户端在线状态
func updateOnline(clientId int64, online string) {
	c := models.TClient{Id: clientId, Online: online}
	if _, err := models.ChangeClientOnline(&c); err != nil {
		log.Printf("更新客户端[%d]在线状态失败: %v", clientId, err)
	}
}

// monitorTask 定时检测所有客户端在线状态
func monitorTask(cfg *config.Config) {
	log.Printf("定时检测客户端是否在线 Time: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	clients, err := models.QueryAllClient()
	if err != nil {
		log.Printf("orm query all client err: %v", err)
		return
	}

	for _, client := range clients {
		checkClient(client)
	}
}

// StartMonitor 启动定时任务（cron 表达式，如 "0 0/5 * * * *"）
func StartMonitor(cfg *config.Config) *cron.Cron {
	c := cron.New()
	cronExpr := cfg.StringOr("cron", "0 0/5 * * * *")

	// 兼容 beego 6 段 cron 表达式，去掉秒字段
	expr := cronExpr
	if len(expr) > 0 {
		// robfig/cron 标准 5 段，若为 6 段则裁剪
		fields := splitFields(expr)
		if len(fields) == 6 {
			expr = joinFields(fields[1:])
		}
	}

	if _, err := c.AddFunc(expr, func() {
		monitorTask(cfg)
	}); err != nil {
		log.Printf("注册定时任务失败: %v，将使用默认5分钟间隔", err)
		if _, err2 := c.AddFunc("0 */5 * * * *", func() {
			monitorTask(cfg)
		}); err2 != nil {
			log.Printf("注册默认定时任务失败: %v", err2)
		}
	}

	c.Start()
	log.Printf("定时任务已启动: %s\n", expr)
	return c
}

// splitFields 按空格切分 cron 表达式
func splitFields(expr string) []string {
	var fields []string
	var current strings.Builder
	for _, r := range expr {
		if r == ' ' || r == '\t' {
			if current.Len() > 0 {
				fields = append(fields, current.String())
				current.Reset()
			}
		} else {
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		fields = append(fields, current.String())
	}
	return fields
}

// joinFields 用空格拼接字段
func joinFields(fields []string) string {
	return strings.Join(fields, " ")
}
