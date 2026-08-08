package register

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"ydsz-trace/pkg/config"

	"github.com/gin-gonic/gin"
)

// Resp 通用响应
type Resp struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
}

// Register 通过配置文件注册本地客户端
func Register(c *gin.Context) {
	cfg := c.MustGet("cfg").(*config.Config)
	server := config.EnvOrConfig("YDSZ_LOG_SERVER", cfg.String("logs"), "127.0.0.1:2021")
	vKey := GetVKey(cfg)
	RegisterLocalIp(server, vKey)
	c.JSON(http.StatusOK, Resp{"200", "注册请求已发送"})
}

// CheckOnline 在线检测接口，供 logs 服务端探测
func CheckOnline(c *gin.Context) {
	c.JSON(http.StatusOK, Resp{"200", "客户端在线"})
}

// GetVKey 优先从环境变量获取密钥，降级到配置文件
func GetVKey(cfg *config.Config) string {
	if v := os.Getenv("YDSZ_CLIENT_KEY"); v != "" {
		return v
	}
	return cfg.StringOr("key", "123456")
}

// RegisterLocalIp 启动时自动注册客户端到服务端
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

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	log.Printf("logc register url=%v param=%v errMsg=%v\n", url, vKey, err)
	if err != nil {
		log.Printf("Local client registered error.")
		return
	}
	defer resp.Body.Close()
	log.Printf("Local client registered successfully. status=%d", resp.StatusCode)
}
