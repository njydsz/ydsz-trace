// Package register 处理 logc 向 logs 服务端的注册逻辑。
//
// 两种注册模式：
//   - 单实例注册（传统）：一个 logc 注册为一个客户端，logs 通过 IP+Port 调用
//   - 虚拟客户端注册（Docker/K8s）：logc 自动发现容器/Pod，为每个发现的客户端
//     向 logs 注册独立的"虚拟客户端"记录；queries 通过 address+VirtualParent 路由到该 logc
package register

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/util"

	"github.com/gin-gonic/gin"
)

// Resp 通用 JSON 响应结构。
// Code 为数字类型（0 = 成功），与 pkg/api.Response 保持一致。
type Resp struct {
	Code int    `json:"code"`
	Msg  string `json:"message"`
}

// errRegisterFailed 注册请求失败时的哨兵错误。
var errRegisterFailed = errors.New("register request failed")

// Register 手动触发一次本地客户端注册（HTTP 接口）。
func Register(c *gin.Context) {
	cfg := c.MustGet("cfg").(*config.Config)
	server := config.EnvOrConfig("YDSZ_LOG_SERVER", cfg.String("logs"), "127.0.0.1:2021")
	vKey := GetVKey(cfg)
	regKey := getRegisterKey(cfg)
	if _, err := RegisterLocalIp(server, vKey, regKey); err != nil {
		c.JSON(http.StatusOK, Resp{500, fmt.Sprintf("注册失败: %v", err)})
		return
	}
	c.JSON(http.StatusOK, Resp{200, "注册请求已发送"})
}

// CheckOnline 返回客户端在线状态（供 logs 定时探测任务调用）。
func CheckOnline(c *gin.Context) {
	c.JSON(http.StatusOK, Resp{200, "客户端在线"})
}

// GetVKey 按优先级获取客户端密钥。
func GetVKey(cfg *config.Config) string {
	if v := os.Getenv("YDSZ_CLIENT_KEY"); v != "" {
		return v
	}
	return cfg.StringOr("key", "123456")
}

// getRegisterKey 获取注册鉴权密钥，优先级：环境变量 YDSZ_REGISTER_KEY > 配置文件 register_key > 默认值。
//
// 【安全】默认值 "ydsz-trace-register-2026" 仅适用于开发环境。
// 生产环境必须通过 YDSZ_REGISTER_KEY 环境变量注入强密钥，
// 否则启动时将打印安全警告日志。
func getRegisterKey(cfg *config.Config) string {
	if v := os.Getenv("YDSZ_REGISTER_KEY"); v != "" {
		return v
	}
	if cfg != nil {
		if v := cfg.StringOr("register_key", ""); v != "" {
			return v
		}
	}
	log.Printf("[security] 警告：使用默认注册密钥，生产环境请通过 YDSZ_REGISTER_KEY 环境变量配置强密钥")
	return "ydsz-trace-register-2026"
}

// computeHMAC 计算时间戳签名字符串。
// 签名算法：HMAC-SHA1(secret, timestamp_hex + "." + vkey)。
// frp 认证协议兼容格式：HMAC-SHA1(secret, timestamp + client_id)。
func computeHMAC(secret, vkey string, timestamp int64) string {
	tsStr := strconv.FormatInt(timestamp, 10)
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(tsStr + "." + vkey))
	return hex.EncodeToString(mac.Sum(nil))
}

// RegisterLocalIp 向 logs 服务端发起一次自注册请求（传统模式）。
//
// 参数：
//   - server  logs 服务端地址（ip:port）
//   - vKey    客户端预共享密钥（用于 HMAC 签名中的 vkey 字段）
//   - regKey  注册鉴权密钥（用于 HMAC 签名的 secret 字段，与服务端 getRegisterKey 一致）
//
// 返回值：
//   - 成功时返回 nil
//   - 网络错误、非 200 响应、服务端业务码非 "200" 时返回 error
//
// 签名机制（FR-1308）：
// 注册请求在 body 中携带 timestamp 与 signature，服务端使用 HMAC-SHA1 验证，
// 替代原先明文传输 vkey 的方式，防止重放攻击与密钥泄漏。
// 签名算法：HMAC-SHA1(secret=regKey, timestamp_decimal + "." + vKey)
// 与 logs 侧 verifyHMACSignature 保持一致。
func RegisterLocalIp(server string, vKey string, regKey string) (time.Duration, error) {
	if server == "" {
		return 0, fmt.Errorf("server 地址为空")
	}
	if regKey == "" {
		regKey = getRegisterKey(nil)
	}

	timestamp := time.Now().Unix()
	signature := computeHMAC(regKey, vKey, timestamp)

	body, err := json.Marshal(map[string]interface{}{
		"key":         vKey,
		"timestamp":   timestamp,
		"signature":   signature,
		"source_type": "file",
	})
	if err != nil {
		return 0, fmt.Errorf("marshal register body: %w", err)
	}

	url := "http://" + server + "/client/register"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("new register request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := util.NewClientWithTimeout(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("send register request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%w: HTTP %d", errRegisterFailed, resp.StatusCode)
	}

	var result Resp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode register response: %w", err)
	}

	log.Printf("[register] 传统注册完成 server=%s code=%d msg=%s", server, result.Code, result.Msg)

	if result.Code != 0 {
		return 0, fmt.Errorf("%w: code=%d msg=%s", errRegisterFailed, result.Code, result.Msg)
	}

	return 0, nil
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
// key 字段使用注册鉴权密钥（与 logs 侧校验保持一致）。
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
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("register 返回 %d", resp.StatusCode)
	}
	log.Printf("[register] %s 注册/注销成功: %s", body["source_type"], body["identity"])
	return nil
}
