// Package register 处理 logc 向 logs 服务端的注册逻辑。
//
// 两种注册模式：
//   - 单实例注册（传统）：一个 logc 注册为一个客户端，logs 通过 IP+Port 调用
//   - 虚拟客户端注册（Docker/K8s）：logc 自动发现容器/Pod，为每个发现的客户端
//     向 logs 注册独立的"虚拟客户端"记录；queries 通过 address+VirtualParent 路由到该 logc
package register

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	Code string `json:"code"`
	Msg  string `json:"msg"`
}

// Register 手动触发一次本地客户端注册（HTTP 接口）。
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

// GetVKey 按优先级获取客户端密钥。
func GetVKey(cfg *config.Config) string {
	if v := os.Getenv("YDSZ_CLIENT_KEY"); v != "" {
		return v
	}
	return cfg.StringOr("key", "123456")
}

// RegisterLocalIp 向 logs 服务发起一次自注册请求（传统模式）。
func RegisterLocalIp(server string, vKey string) {
	body, err := json.Marshal(map[string]interface{}{
		"key":         vKey,
		"source_type": "file",
	})
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
		log.Printf("Local client registration error.")
		return
	}
	defer resp.Body.Close()
	log.Printf("Local client registered successfully. status=%d", resp.StatusCode)
}

// VirtualRegisterRequest 虚拟客户端注册请求（Docker / K8s 模式）。
// logs 服务端根据 SourceType+Identity 创建独立的 client 记录和关联 item。
type VirtualRegisterRequest struct {
	Server        string            `json:"server"`
	VKey          string            `json:"vkey"`
	SourceType    string            `json:"source_type"`    // docker / k8s
	Identity      string            `json:"identity"`       // 稳定标识（容器ID/podUID+container）
	DisplayName   string            `json:"display_name"`   // 展示名
	LogPath       string            `json:"log_path"`       // 可读路径（container:id 或 k8s://...）
	LocalLogcPort string            `json:"local_logc_port"`
	Labels        map[string]string `json:"labels"` // 扩展标签
}

// VirtualClient 向 logs 注册一个虚拟客户端。
func VirtualClient(req *VirtualRegisterRequest) error {
	if req.Server == "" || req.Identity == "" {
		return fmt.Errorf("server 和 identity 不能为空")
	}

	body := map[string]interface{}{
		"key":             req.VKey,
		"source_type":     req.SourceType,
		"identity":        req.Identity,
		"display_name":    req.DisplayName,
		"log_path":        req.LogPath,
		"local_logc_port": req.LocalLogcPort,
		"labels":          req.Labels,
	}
	return sendRegister(req.Server, body)
}

// RemoveVirtualClient 通知 logs 注销指定标识的虚拟客户端。
func RemoveVirtualClient(server, vKey, identity string) error {
	body := map[string]interface{}{
		"key":      vKey,
		"identity": identity,
		"action":   "remove",
	}
	return sendRegister(server, body)
}

// sendRegister 向 logs 服务端发送注册请求。
func sendRegister(server string, body map[string]interface{}) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	url := "http://" + server + "/client/register"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := util.NewClientWithTimeout(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("register 返回 %d", resp.StatusCode)
	}
	log.Printf("[register] %s 注册/注销成功: %s", body["source_type"], body["identity"])
	return nil
}

