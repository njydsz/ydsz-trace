// Package client 包含客户端（logc agent）管理控制器。
//
// 支持两种注册路径：
//   - 传统模式 (source_type=file)：由管理员在控制台或 server 预注册；logc 启动后携带 HMAC 签名自注册
//   - 自动模式 (source_type=docker/k8s)：logc 启动时主动上报虚拟客户端；服务端根据 identity 去重
//
// 安全设计：
//   - 注册鉴权支持 HMAC-SHA1 签名（推荐）与明文 vkey（降级兼容）两种模式
//   - 传统客户端默认不允许自注册新建，须管理员预注册（防恶意注册）
//   - 客户端 IP 默认从 RemoteAddr 获取，仅在 YDSZ_TRUSTED_PROXY 设置时信任代理头
//   - 所有注册操作均记录审计日志
package client

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"ydsz-trace/logs/controllers/audit"
	models "ydsz-trace/logs/models"
	itemModels "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/config"

	"github.com/gin-gonic/gin"
)

// maxSignatureDrift 签名时间戳允许的最大偏差（秒）。
// 超过此偏差的注册请求视为重放攻击予以拒绝。
const maxSignatureDrift int64 = 300 // 5 分钟

// idReq 通用 id 请求体（用于删除/状态切换）。
type idReq struct {
	ID int64 `json:"id" binding:"required"`
}

// statusReq 状态切换请求体。
type statusReq struct {
	ID     int64 `json:"id" binding:"required"`
	Status int   `json:"status" binding:"required"`
}

// ClientRegisterReq 客户端注册请求体。
//
// 兼容两种模式：传统签名注册 / Docker-K8s 虚拟客户端。
//
// 安全增强（FR-1308）：
//   - timestamp + signature 为 HMAC-SHA1 签名验证字段，替代明文 vkey 传输
//   - 旧版 logc（未签名）使用 key + vkey 直接比对作为降级兼容
type ClientRegisterReq struct {
	// VKey 预共享密钥（明文降级兼容，新客户端应使用签名）
	VKey string `json:"key"`
	// Timestamp 签名时间戳（Unix 秒），用于防重放
	Timestamp int64 `json:"timestamp"`
	// Signature HMAC-SHA1 签名，签名字符串 = timestamp + "." + vkey
	Signature string `json:"signature"`
	// SourceType file | docker | k8s
	SourceType string `json:"source_type"`
	// Identity 虚拟客户端唯一标识（仅在 docker/k8s 模式有意义）
	Identity string `json:"identity"`
	// DisplayName 展示名称
	DisplayName string `json:"display_name"`
	// LogPath 日志可读路径（container:id 或 k8s://namespace/pod/container）
	LogPath string `json:"log_path"`
	// LocalLogcPort logc HTTP 端口
	LocalLogcPort string `json:"local_logc_port"`
	// Labels 扩展标签 JSON
	Labels map[string]string `json:"labels"`
	// Action remove = 注销
	Action string `json:"action"`
}

// nowStr 返回当前时间的格式化字符串。
func nowStr() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// Add 新增客户端。
func Add(c *gin.Context) {
	var client models.TClient
	req, err := c.GetRawData()
	if err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}
	err = json.Unmarshal(req, &client)
	if err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}
	client.Online = "0"
	client.CreatedBy = "admin"
	client.CreatedTime = nowStr()
	client.UpdatedBy = "admin"
	client.UpdatedTime = nowStr()
	id, err := models.AddClient(&client)
	if err != nil {
		log.Printf("新增客户端失败: %v", err)
		api.Fail(c, api.CodeServerError, "新增客户端失败")
		return
	}
	log.Printf("新增客户端: ID=%d", id)
	api.Success(c, "客户端新增成功", gin.H{"id": id})
}

// Delete 按 id 删除客户端。
func Delete(c *gin.Context) {
	var req idReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: id 不能为空")
		return
	}
	models.DeleteClient(req.ID)
	api.Success(c, "删除客户端成功", nil)
}

// Query 按 id 查询单个客户端详情。
func Query(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Query("id"), 10, 64)
	client := models.ReadClient(id)
	api.Success(c, "查询客户端成功", client)
}

// Update 更新客户端全量字段。
func Update(c *gin.Context) {
	var client models.TClient
	req, err := c.GetRawData()
	if err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}
	err = json.Unmarshal(req, &client)
	if err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}
	client.UpdatedTime = nowStr()
	models.UpdateClient(&client)
	api.Success(c, "更新客户端成功", nil)
}

// getRegisterKey 获取注册鉴权密钥。
// 优先级：环境变量 YDSZ_REGISTER_KEY > 配置文件 register_key > 默认值。
//
// 【安全】默认值 "ydsz-trace-register-2026" 仅适用于开发环境，
// 生产环境务必通过 YDSZ_REGISTER_KEY 注入强密钥（≥16 字符）。
func getRegisterKey(c *gin.Context) string {
	if v := os.Getenv("YDSZ_REGISTER_KEY"); v != "" {
		return v
	}
	cfg, exists := c.Get("cfg")
	if exists {
		if conf, ok := cfg.(*config.Config); ok {
			if v := conf.StringOr("register_key", ""); v != "" {
				return v
			}
		}
	}
	log.Printf("[security] 警告：使用默认注册密钥，生产环境请通过 YDSZ_REGISTER_KEY 环境变量配置强密钥")
	return "ydsz-trace-register-2026"
}

// isSignatureEnabled 判断是否启用 HMAC 签名验证。
// 当注册密钥非默认值且长度 ≥ 16 时启用，使用默认降级密钥时走明文兼容。
func isSignatureEnabled(registerKey string) bool {
	return len(registerKey) >= 16 && registerKey != "ydsz-trace-register-2026"
}

// verifyRegister 验证注册请求的鉴权信息。
//
// 验证策略：
//   - 签名验证启用时（isSignatureEnabled），优先校验 HMAC-SHA1 签名
//   - 旧版客户端（无 signature）降级为明文 vkey 常量时间比对
//   - 时间戳偏差超过 maxSignatureDrift 时拒绝（防重放攻击）
//
// 符合编码规范 §9.1（认证安全）和 PRD FR-1308（Agent 注册鉴权）。
func verifyRegister(secret string, reg *ClientRegisterReq) error {
	if isSignatureEnabled(secret) && reg.Signature != "" {
		return verifyHMACSignature(secret, reg.VKey, reg.Timestamp, reg.Signature)
	}
	// 降级兼容：明文 vkey 比对（常量时间比较防时序攻击）
	if subtle.ConstantTimeCompare([]byte(reg.VKey), []byte(secret)) != 1 {
		return errors.New("明文密钥不匹配")
	}
	return nil
}

// verifyHMACSignature 验证客户端提交的 HMAC-SHA1 签名。
//
// 签名算法：HMAC-SHA1(secret, timestamp_decimal + "." + vkey)
// 与 logc 侧 RegisterLocalIp 中的 computeHMAC 保持一致。
//
// 安全检查：
//   - 时间戳漂移检查（防重放）
//   - hmac.Equal 常量时间比较（防时序攻击）
func verifyHMACSignature(secret, vkey string, timestamp int64, signature string) error {
	drift := time.Now().Unix() - timestamp
	if drift < -maxSignatureDrift || drift > maxSignatureDrift {
		return fmt.Errorf("签名时间戳超出允许偏差: drift=%ds, max=%ds", drift, maxSignatureDrift)
	}
	tsStr := strconv.FormatInt(timestamp, 10)
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(tsStr + "." + vkey))
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expectedSig), []byte(signature)) {
		return errors.New("签名验证失败")
	}
	return nil
}

// extractClientIP 安全地提取客户端真实 IP。
//
// 优先级：X-Real-IP（单跳代理） > X-Forwarded-For 首个非空 IP > RemoteAddr。
//
// 【安全】仅在环境变量 YDSZ_TRUSTED_PROXY 设置时信任 X-Forwarded-For/X-Real-IP 头，
// 否则直接使用 RemoteAddr，防止恶意客户端伪造代理头进行 IP 欺骗。
// 符合编码规范 §9.2（输入校验）和 PRD FR-1308。
func extractClientIP(c *gin.Context) string {
	if os.Getenv("YDSZ_TRUSTED_PROXY") != "" {
		if realIP := c.GetHeader("X-Real-IP"); realIP != "" {
			return realIP
		}
		if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
			if idx := strings.Index(xff, ","); idx > 0 {
				return strings.TrimSpace(xff[:idx])
			}
			return strings.TrimSpace(xff)
		}
	}
	addr := c.Request.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx > 0 {
		return addr[:idx]
	}
	return addr
}

// shouldAutoCreateClient 判断是否允许自注册时自动创建客户端。
// 默认返回 false（强制预注册）。仅当环境变量 YDSZ_AUTO_CREATE_CLIENT=true 时开启。
// 符合 PRD §1.2 的注册安全要求（防止恶意节点自注册）。
func shouldAutoCreateClient() bool {
	return os.Getenv("YDSZ_AUTO_CREATE_CLIENT") == "true"
}

// Register 接收 logc 代理自注册请求。
//
// 安全特性（PRD FR-1308）：
//   - HMAC-SHA1 签名验证或降级明文 vkey 验证（二选一）
//   - IP 地址安全提取（防伪造）
//   - 传统客户端默认拒绝自动新建（需管理员预注册）
//
// @Summary 客户端注册
// @Description 接收 logc agent 的自注册请求，支持传统（HMAC 签名）与虚拟客户端（Docker/K8s identity）两种模式。注册鉴通过后上线或返回预注册提示。
// @Tags Client
// @Accept json
// @Produce json
// @Param body body ClientRegisterReq true "注册请求：携带 timestamp + signature（推荐）或 key（降级）"
// @Success 200 {object} api.Response "注册成功"
// @Failure 400 {object} api.Response "参数错误或缺少 identity"
// @Failure 401 {object} api.Response "注册鉴权失败"
// @Failure 500 {object} api.Response "服务端错误"
// @Router /client/register [post]
func Register(c *gin.Context) {
	ip := extractClientIP(c)
	port := "2020"

	var reg ClientRegisterReq
	reqBody, err := c.GetRawData()
	if err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}
	if err := json.Unmarshal(reqBody, &reg); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误")
		return
	}

	// 注册鉴权：HMAC 签名 vkey + timestamp）或降级明文 vkey 比对
	expectedKey := getRegisterKey(c)
	if err := verifyRegister(expectedKey, &reg); err != nil {
		log.Printf("[security] 注册鉴权失败 ip=%s: %v", ip, err)
		audit.RecordAudit(c, "register", "/client/register", map[string]interface{}{
			"ip":    ip,
			"error": err.Error(),
		}, "fail")
		api.Fail(c, api.CodeUnauthorized, "注册鉴权失败")
		return
	}

	switch reg.SourceType {
	case "docker", "k8s":
		handleVirtualRegister(c, ip, port, &reg)
		return
	default:
		handleTraditionalRegister(c, ip, port, reg.VKey, reg.LocalLogcPort)
		return
	}
}

// handleTraditionalRegister 传统 IP+Port+vKey 注册。
// localLogcPort 不为空时用其替代默认 2020。
//
// 安全策略变更（P0-2）：默认不允许自注册未预创建的客户端。
// 仅在 YDSZ_AUTO_CREATE_CLIENT=true 时保留原有"未注册即新建"行为。
// 符合 PRD §1.2 注册安全要求与 FR-1308。
func handleTraditionalRegister(c *gin.Context, ip, port, vKey, localLogcPort string) {
	if localLogcPort != "" {
		port = localLogcPort
	}
	client := models.CheckClient(ip, port, vKey)
	if client.Id != 0 {
		cl := models.TClient{Id: client.Id, Online: "1", UpdatedTime: nowStr()}
		if _, err := models.ChangeClientOnline(&cl); err != nil {
			log.Printf("[register] 上线状态更新失败 client_id=%d: %v", client.Id, err)
			api.Fail(c, api.CodeServerError, "客户端上线失败")
			return
		}
		log.Printf("[register] 传统客户端上线 ip=%s port=%s id=%d", ip, port, client.Id)
	} else if shouldAutoCreateClient() {
		// 兼容性开关开启时允许自建（仅限开发/测试场景）
		nowS := nowStr()
		newClient := &models.TClient{
			Ip: ip, Port: port, Vkey: vKey, SourceType: "file",
			Status: "1", Online: "1", Zip: "1",
			CreatedBy: "admin", UpdatedBy: "admin", CreatedTime: nowS, UpdatedTime: nowS,
		}
		id, err := models.AddClient(newClient)
		if err != nil {
			log.Printf("[register] 传统客户端自注册失败 ip=%s: %v", ip, err)
			api.Fail(c, api.CodeServerError, "客户端注册失败")
			return
		}
		log.Printf("[register] 传统客户端自注册成功 ip=%s id=%d", ip, id)
	} else {
		// 默认路径：拒绝未预注册的客户端
		log.Printf("[security] 拒绝未预注册客户端: ip=%s port=%s", ip, port)
		audit.RecordAudit(c, "register", "/client/register", map[string]interface{}{
			"ip":   ip,
			"port": port,
		}, "rejected")
		api.Fail(c, api.CodeForbidden, "客户端未预注册，请管理员先在控制台添加该节点")
		return
	}
	// 审计埋点：客户端注册
	audit.RecordAudit(c, "register", "/client/register", map[string]interface{}{
		"ip":         ip,
		"port":       port,
		"sourceType": "file",
	}, "success")

	api.Success(c, "客户端上线成功", nil)
}

// handleVirtualRegister Docker/K8s 模式的虚拟客户端注册/注销。
//
// reg.Action = remove：注销指定 identity；
// reg.Action 非 remove：新增或更新 identity。
func handleVirtualRegister(c *gin.Context, ip, port string, reg *ClientRegisterReq) {
	port2 := port
	if reg.LocalLogcPort != "" {
		port2 = reg.LocalLogcPort
	}
	virtualParent := ip + ":" + port2

	if reg.Action == "remove" {
		models.DeleteVirtualClient(virtualParent, reg.Identity)
		log.Printf("[register] 虚拟客户端注销 parent=%s identity=%s", virtualParent, reg.Identity)
		api.Success(c, "虚拟客户端注销成功", nil)
		return
	}

	if reg.Identity == "" {
		api.Fail(c, api.CodeBadRequest, "缺少 identity")
		return
	}

	labelsJSON := "{}"
	if len(reg.Labels) > 0 {
		if b, err := json.Marshal(reg.Labels); err == nil {
			labelsJSON = string(b)
		}
	}

	client := &models.TClient{
		Ip:            ip,
		Port:          port2,
		Vkey:          reg.VKey,
		SourceType:    reg.SourceType,
		Identity:      reg.Identity,
		VirtualParent: virtualParent,
		Info:          reg.DisplayName,
		Labels:        labelsJSON,
	}
	id, err := models.AddVirtualClient(client)
	if err != nil {
		log.Printf("[register] 虚拟客户端注册失败 identity=%s: %v", reg.Identity, err)
		api.Fail(c, api.CodeServerError, "虚拟客户端注册失败")
		return
	}
	log.Printf("[register] 虚拟客户端注册成功 source=%s identity=%s id=%d",
		reg.SourceType, reg.Identity, id)

	// 自动创建关联 item（日志项），方便后续查询
	if reg.LogPath != "" {
		autoItemName := firstNonEmpty(reg.DisplayName, reg.Identity)
		item := &models.TItem{
			ClientId:    id,
			ItemName:    autoItemName,
			LogPath:     reg.LogPath,
			Status:      "1",
			CreatedBy:   "admin",
			CreatedTime: nowStr(),
			UpdatedBy:   "admin",
			UpdatedTime: nowStr(),
		}
		itemId, err := itemModels.AddItem(item)
		if err != nil {
			log.Printf("[register] 自动创建 item 失败: %v", err)
		} else {
			log.Printf("[register] 自动创建 item id=%d", itemId)
		}
	}

	// 审计埋点：虚拟客户端注册
	audit.RecordAudit(c, "register", "/client/register", map[string]interface{}{
		"ip":            ip,
		"port":          port2,
		"sourceType":    reg.SourceType,
		"identity":      reg.Identity,
		"virtualParent": virtualParent,
	}, "success")

	api.Success(c, "虚拟客户端注册成功", gin.H{"id": id})
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return "unknown"
}

// ChangeClientStatus 切换客户端启用/禁用状态。
func ChangeClientStatus(c *gin.Context) {
	var req statusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		api.Fail(c, api.CodeBadRequest, "请求参数错误: id 和 status 不能为空")
		return
	}
	models.ChangeClientStatusByIDAndStatus(req.ID, req.Status, nowStr())
	api.Success(c, "更新客户端成功", nil)
}

// QueryAll 查询全部客户端列表（传统 + 虚拟）。
func QueryAll(c *gin.Context) {
	clients, err := models.QueryAllClient()
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询客户端列表失败")
		return
	}
	api.Success(c, "查询客户端成功", clients)
}

// QueryAllTraditional 仅查询传统客户端（兼容旧接口）。
func QueryAllTraditional(c *gin.Context) {
	clients, err := models.QueryAllTraditionalClient()
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询客户端列表失败")
		return
	}
	api.Success(c, "查询客户端成功", clients)
}

// QueryVirtualByParent 按 virtual_parent 查询其下全部虚拟客户端。
func QueryVirtualByParent(c *gin.Context) {
	parent := c.Query("parent")
	clients, err := models.QueryVirtualClientByParent(parent)
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询虚拟客户端失败")
		return
	}
	api.Success(c, "查询虚拟客户端成功", clients)
}

// QueryPage 分页查询客户端。
func QueryPage(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	page := models.QueryPageClient(pageNum, pageSize)
	api.Paginated(c, "分页查询客户端成功", page.List, page.TotalCount, pageNum, pageSize)
}
