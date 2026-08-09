// Package client 包含客户端（logc agent）管理控制器。
//
// 支持两种注册路径：
//   - 传统模式 (source_type=file)：由管理员在前端页面手动创建 client；logc 启动后端通过 IP+port+vkey 自注册
//   - 自动模式 (source_type=docker/k8s)：logc 在启动时主动上报虚拟客户端；服务端根据 identity 去重
package client

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	models "ydsz-trace/logs/models"
	itemModels "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

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
// 兼容两种模式：传统只传 key；虚拟客户端传 source_type + identity + display_name + log_path。
type ClientRegisterReq struct {
	// VKey 预共享密钥
	VKey string `json:"key"`
	// SourceType file | docker | k8s
	SourceType string `json:"source_type"`
	// Identity 虚拟客户端唯一标识（仅在 docker/k8s 模式有意义）
	Identity string `json:"identity"`
	// DisplayName 展示名
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
	log.Printf("ID: %d, ERR: %v\n", id, err)
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

// Register 接收 logc 自注册请求。
//
// 两种模式：
//  1. 传统模式 (source_type 为空或 file)：按 ip+port+vkey 匹配；通过后置 online=1
//  2. 虚拟客户端 (source_type=docker|k8s)：按 virtual_parent+identity 去重，
//     并自动创建关联的 item（日志项）记录
func Register(c *gin.Context) {
	addr := c.Request.RemoteAddr
	s := strings.Split(addr, ":")
	ip := s[0]
	port := "2020"

	var reg ClientRegisterReq
	reqBody, err := c.GetRawData()
	if err == nil {
		err = json.Unmarshal(reqBody, &reg)
		if err == nil {
			switch reg.SourceType {
			case "docker", "k8s":
				handleVirtualRegister(c, ip, port, &reg)
				return
			default:
				handleTraditionalRegister(c, ip, port, reg.VKey, reg.LocalLogcPort)
				return
			}
		}
	}
	api.Success(c, "客户端上线成功", nil)
}

// handleTraditionalRegister 传统 IP+Port+vKey 注册。
// localLogcPort 不为空时用其替代默认 2020。
func handleTraditionalRegister(c *gin.Context, ip, port, vKey, localLogcPort string) {
	if localLogcPort != "" {
		port = localLogcPort
	}
	client := models.CheckClient(ip, port, vKey)
	if client.Id != 0 {
		cl := models.TClient{Id: client.Id, Online: "1", UpdatedTime: nowStr()}
		models.ChangeClientOnline(&cl)
		log.Printf("[register] 传统客户端上线 ip=%s port=%s id=%d", ip, port, client.Id)
	} else {
		// 未找到匹配：可能是首次注册，新增一条记录
		nowS := nowStr()
		newClient := &models.TClient{
			Ip: ip, Port: port, Vkey: vKey, SourceType: "file",
			Status: "1", Online: "1", Zip: "1",
			CreatedBy: "admin", UpdatedBy: "admin", CreatedTime: nowS, UpdatedTime: nowS,
		}
		id, err := models.AddClient(newClient)
		if err != nil {
			log.Printf("[register] 传统客户端自注册失败 ip=%s: %v", ip, err)
		} else {
			log.Printf("[register] 传统客户端自注册成功 ip=%s id=%d", ip, id)
		}
	}
	api.Success(c, "客户端上线成功", nil)
}

// handleVirtualRegister Docker/K8s 模式的虚拟客户端注册/注销。
//
// reg.Action = remove：注销指定 identity；
// reg.Action 非 remove：新增或更新 identity。
func handleVirtualRegister(c *gin.Context, ip, port string, reg *Register) {
	port2 := port
	if reg.LocalLogcPort != "" {
		port2 = reg.LocalLogcPort
	}
	virtualParent := ip + ":" + port2

	if reg.Action == "remove" {
		// 注销
		models.DeleteVirtualClient(virtualParent, reg.Identity)
		log.Printf("[register] 虚拟客户端注销 parent=%s identity=%s", virtualParent, reg.Identity)
		api.Success(c, "虚拟客户端注销成功", nil)
		return
	}

	if reg.Identity == "" {
		// 不允许空 identity
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
