package client

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	models "ydsz-trace/logs/models"

	"github.com/gin-gonic/gin"
)

// Register 客户端注册请求
type Register struct {
	VKey string `json:"key"`
}

// queryInt64 从 query 参数解析 int64
func queryInt64(c *gin.Context, key string) (int64, error) {
	return strconv.ParseInt(c.Query(key), 10, 64)
}

// Add 新增客户端
func Add(c *gin.Context) {
	var client models.TClient
	if err := c.ShouldBindJSON(&client); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "400", "msg": "请求参数错误", "data": nil})
		return
	}
	client.Online = "0"
	client.CreatedBy = "admin"
	client.CreatedTime = time.Now()
	client.UpdatedBy = "admin"
	client.UpdatedTime = time.Now()
	if _, err := models.AddClient(&client); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "客户端新增失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "客户端新增成功", "data": nil})
}

// Delete 删除客户端
func Delete(c *gin.Context) {
	clientId, err := queryInt64(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "400", "msg": "参数错误", "data": nil})
		return
	}
	if _, err := models.DeleteClient(clientId); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "删除客户端失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "删除客户端成功", "data": nil})
}

// Query 根据Id查询客户端
func Query(c *gin.Context) {
	clientId, err := queryInt64(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "400", "msg": "参数错误", "data": nil})
		return
	}
	client, err := models.ReadClient(clientId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "404", "msg": "客户端不存在", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "查询客户端成功", "data": client})
}

// Update 更新客户端
func Update(c *gin.Context) {
	var client models.TClient
	if err := c.ShouldBindJSON(&client); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "400", "msg": "请求参数错误", "data": nil})
		return
	}
	client.UpdatedTime = time.Now()
	if _, err := models.UpdateClient(&client); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "更新客户端失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "更新客户端成功", "data": nil})
}

// Register 客户端上线注册（供 logc 代理调用）
func Register(c *gin.Context) {
	// 获取请求的IP
	addr := c.Request.RemoteAddr
	s := strings.Split(addr, ":")
	ip := s[0]
	port := "2020"

	var register Register
	if err := c.ShouldBindJSON(&register); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "400", "msg": "请求参数错误", "data": nil})
		return
	}

	// 根据ip、port、vkey查询客户端的有效性
	client, err := models.CheckClient(ip, port, register.VKey)
	if err != nil || client.Id == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "401", "msg": "客户端校验失败", "data": nil})
		return
	}

	cc := models.TClient{Id: client.Id, Online: "1"}
	_, _ = models.ChangeClientOnline(&cc)
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "客户端上线成功", "data": nil})
}

// ChangeClientStatus 切换客户端状态
func ChangeClientStatus(c *gin.Context) {
	clientId, err := queryInt64(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "400", "msg": "参数错误", "data": nil})
		return
	}
	if _, err := models.ChangeClientStatus(clientId); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "更新客户端失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "更新客户端成功", "data": nil})
}

// QueryAll 查询所有客户端
func QueryAll(c *gin.Context) {
	clients, err := models.QueryAllClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "查询客户端失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "查询客户端成功", "data": clients})
}

// QueryPage 分页查询客户端
func QueryPage(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("limit"))
	page, err := models.QueryPageClient(pageNum, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "分页查询客户端失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "分页查询客户端成功", "data": page})
}
