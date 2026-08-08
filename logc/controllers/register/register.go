package register

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/util"

	"github.com/gin-gonic/gin"
)

// Resp 通用 JSON 响应结构。
type Resp struct {
	// Code 业务状态码，"200" 表示成功
	Code string `json:"code"`
	// Msg 提示信息
	Msg string `json:"msg"`
}

// Register 手动触发一次本地客户端注册（HTTP 接口）。
//
// 读取 cfg 中的 logs 地址与 vKey，调用 RegisterLocalIp 向 logs 注册。
func Register(c *gin.Context) {
	cfg := c.MustGet("cfg").(*config.Config)
	server := config.EnvOrConfig("YDSZ_LOG_SERVER", cfg.String("logs"), "127.0.0.1:2021")
	vKey := GetVKey(cfg)
	RegisterLocalIp(server, vKey)
	c.JSON(http.StatusOK, Resp{"200", "注册请求已发送"})
}

// CheckOnline 返回客户端在线状态（供 logs 定时探测任务调用）。
func CheckOnline(c *gin.Context) {
	c.JSON(http.StatusOK, Resp{"200", "客户端在线"})
}

// GetVKey 按优先级获取客户端密钥：环境变量 YDSZ_CLIENT_KEY > 配置文件 > "123456"。
func GetVKey(cfg *config.Config) string {
	if v := os.Getenv("YDSZ_CLIENT_KEY"); v != "" {
		return v
	}
	return cfg.StringOr("key", "123456")
}

// RegisterLocalIp 向 logs 服务发起一次自注册请求。
//
// 请求体：{"key": "<vKey>"}，endpoint：POST http://server/client/register。
// 调用方需自行处理错误（此处仅打日志）。
func RegisterLocalIp(server string, vKey string) {
	body, err := json.Marshal(map[string]interface{}{"key": vKey})
	if err != nil {
		log.Printf("序列化注册参数失败: %v", err)
		return
	}

	url := "http://" + server + "/client/register"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("构造注册请求失败: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := util.NewClientWithTimeout(10 * time.Second)
	resp, err := client.Do(req)
	log.Printf("logc register url=%v param=%v errMsg=%v\n", url, vKey, err)
	if err != nil {
		log.Printf("Local client registered error.")
		return
	}
	defer resp.Body.Close()
	log.Printf("Local client registered successfully. status=%d", resp.StatusCode)
}
