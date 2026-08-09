// Package client 包含客户端（logc agent）管理控制器。
package client

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	models "ydsz-trace/logs/models"
	"ydsz-trace/pkg/api"

	"github.com/gin-gonic/gin"
)

// RegisterReq 客户端注册请求体。
type RegisterReq struct {
	// VKey 预共享密钥
	VKey string `json:"key"`
}

// nowStr 返回当前时间的格式化字符串（统一时间格式）。
func nowStr() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// Add 新增客户端：解析请求体并落库（online 初始化为 "0"）。
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
	id, _ := strconv.ParseInt(c.Query("id"), 10, 64)
	models.DeleteClient(id)
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

// Register 接收 logc 自注册请求：校验 ip + port + vkey，通过后置 online=1。
func Register(c *gin.Context) {
	// 获取请求的IP
	addr := c.Request.RemoteAddr
	s := strings.Split(addr, ":")
	ip := s[0]
	port := "2020"

	var register RegisterReq
	reqBody, err := c.GetRawData()
	if err == nil {
		err = json.Unmarshal(reqBody, &register)
		if err == nil {
			// 根据ip、port、vkey查询客户端的有效性
			client := models.CheckClient(ip, port, register.VKey)
			if client.Id != 0 {
				cl := models.TClient{}
				cl.Id = client.Id
				cl.Online = "1"
				cl.UpdatedTime = nowStr()
				models.ChangeClientOnline(&cl)
			}
		}
	}
	api.Success(c, "客户端上线成功", nil)
}

// ChangeClientStatus 切换客户端启用/禁用状态（0 ↔ 1）。
func ChangeClientStatus(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Query("id"), 10, 64)
	models.ChangeClientStatus(id, nowStr())
	api.Success(c, "更新客户端成功", nil)
}

// QueryAll 查询全部客户端列表。
func QueryAll(c *gin.Context) {
	clients, err := models.QueryAllClient()
	if err != nil {
		api.Fail(c, api.CodeServerError, "查询客户端列表失败")
		return
	}
	api.Success(c, "查询客户端列表成功", clients)
}

// QueryPage 分页查询客户端列表。
func QueryPage(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	page := models.QueryPageClient(pageNum, pageSize)
	api.Paginated(c, "分页查询客户端成功", page.List, page.TotalCount, pageNum, pageSize)
}
