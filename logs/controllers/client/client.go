package client

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	models "ydsz-trace/logs/models"

	"github.com/gin-gonic/gin"
)

// PageResp 分页响应
type PageResp struct {
	Code string      `json:"code"`
	Msg  string      `json:"msg"`
	Data models.Page `json:"data"`
}

// ClientResp 客户端响应
type ClientResp struct {
	Code string         `json:"code"`
	Msg  string         `json:"msg"`
	Data models.TClient `json:"data"`
}

// Register 客户端注册请求
type Register struct {
	VKey string `json:"key"`
}

// Add 新增客户端
func Add(c *gin.Context) {
	var client models.TClient
	req, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusOK, ClientResp{"400", "请求参数错误", models.TClient{}})
		return
	}
	err = json.Unmarshal(req, &client)
	if err != nil {
		c.JSON(http.StatusOK, ClientResp{"400", "请求参数错误", models.TClient{}})
		return
	}
	client.Online = "0"
	client.CreatedBy = "admin"
	client.CreatedTime = time.Now()
	client.UpdatedBy = "admin"
	client.UpdatedTime = time.Now()
	id, err := models.AddClient(&client)
	log.Printf("ID: %d, ERR: %v\n", id, err)
	c.JSON(http.StatusOK, ClientResp{"200", "客户端新增成功", models.TClient{}})
}

// Delete 删除客户端
func Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Query("id"), 10, 64)
	models.DeleteClient(id)
	c.JSON(http.StatusOK, ClientResp{"200", "删除客户端成功", models.TClient{}})
}

// Query 根据Id查询客户端
func Query(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Query("id"), 10, 64)
	client := models.ReadClient(id)
	c.JSON(http.StatusOK, ClientResp{"200", "查询客户端成功", client})
}

// Update 更新客户端
func Update(c *gin.Context) {
	var client models.TClient
	req, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusOK, ClientResp{"400", "请求参数错误", models.TClient{}})
		return
	}
	err = json.Unmarshal(req, &client)
	if err != nil {
		c.JSON(http.StatusOK, ClientResp{"400", "请求参数错误", models.TClient{}})
		return
	}
	models.UpdateClient(&client)
	c.JSON(http.StatusOK, ClientResp{"200", "更新客户端成功", models.TClient{}})
}

// Register 客户端上线注册（供 logc 代理调用）
func Register(c *gin.Context) {
	// 获取请求的IP
	addr := c.Request.RemoteAddr
	s := strings.Split(addr, ":")
	ip := s[0]
	port := "2020"

	var register Register
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
				models.ChangeClientOnline(&cl)
			}
		}
	}
	c.JSON(http.StatusOK, ClientResp{"200", "客户端上线成功", models.TClient{}})
}

// ChangeClientStatus 切换客户端状态
func ChangeClientStatus(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Query("id"), 10, 64)
	models.ChangeClientStatus(id)
	c.JSON(http.StatusOK, ClientResp{"200", "更新客户端成功", models.TClient{}})
}

// QueryAll 查询所有客户端
func QueryAll(c *gin.Context) {
	clients, _ := models.QueryAllClient()
	c.JSON(http.StatusOK, clients)
}

// QueryPage 分页查询客户端
func QueryPage(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	page := models.QueryPageClient(pageNum, pageSize)
	c.JSON(http.StatusOK, PageResp{"200", "分页查询客户端成功", page})
}
