// Package task 包含 logs 后端的定时任务定义。
//
// 当前任务：
//	- checkOnlineTask（默认每 5 分钟）：轮询所有客户端的 /checkOn，更新 online 字段。
package task

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/metrics"
	"ydsz-trace/pkg/util"

	"github.com/robfig/cron/v3"
)

// checkOnlineTask 定时任务：轮询所有客户端 /checkOn，更新在线状态。
//
// 重试策略：单个节点探测失败不阻塞整体流程，仅标记该节点离线。
// 使用 prometheus gauge 同步上报 total/online 到 /metrics 端点。
func checkOnlineTask() {
	log.Printf("[task] 定时检测客户端在线状态 time=%s", time.Now().Format("2006-01-02 15:04:05"))

	clients, err := models.QueryAllClient()
	if err != nil {
		log.Printf("[task] 查询客户端列表失败: %v", err)
		return
	}

	total := int64(len(clients))
	online := int64(0)

	httpClient := util.NewClientWithTimeout(10 * time.Second)
	for _, client := range clients {
		addr := fmt.Sprintf("http://%s:%s/checkOn", client.Ip, client.Port)
		resp, err := httpClient.Get(addr)
		if err != nil {
			log.Printf("[task] 探测客户端 %s:%s 失败: %v", client.Ip, client.Port, err)
			updateOnline(client.Id, "0")
			continue
		}

		var result api.Response
		dErr := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if dErr != nil {
			log.Printf("[task] 解析客户端 %s 响应失败: %v", client.Ip, dErr)
			updateOnline(client.Id, "0")
			continue
		}

		// 探测成功：HTTP 可达且响应为合法 JSON 即视为在线
		updateOnline(client.Id, "1")
		online++
	}

	// 更新 prometheus gauge
	metrics.Global().UpdateClientStats(total, online)
	log.Printf("[task] 客户端统计更新完成: total=%d, online=%d", total, online)
}

// updateOnline 写入客户端在线状态（1=在线，0=离线）。
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

// InitTask 基于 robfig/cron 创建定时任务，返回 Cron 实例。
//
// 通过配置 cron 表达式控制调度（默认 "0 0/5 * * * *"，即每 5 分钟）。
func InitTask(cfg *config.Config) *cron.Cron {
	c := cron.New(cron.WithSeconds())

	cronExpr := cfg.StringOr("cron", "0 0/5 * * * *")
	if _, err := c.AddFunc(cronExpr, checkOnlineTask); err != nil {
		log.Printf("注册定时任务失败: %v", err)
	}

	return c
}
