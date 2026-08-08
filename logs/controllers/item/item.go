package item

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
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

// ItemResp 项目响应
type ItemResp struct {
	Code string       `json:"code"`
	Msg  string       `json:"msg"`
	Data models.TItem `json:"data"`
}

// nowStr 返回当前时间的格式化字符串（SQLite 存储格式）
func nowStr() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// Add 新增项目日志
func Add(c *gin.Context) {
	var item models.TItem
	req, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusOK, ItemResp{"400", "请求参数错误", models.TItem{}})
		return
	}
	err = json.Unmarshal(req, &item)
	if err != nil {
		c.JSON(http.StatusOK, ItemResp{"400", "请求参数错误", models.TItem{}})
		return
	}
	item.CreatedBy = "admin"
	item.CreatedTime = nowStr()
	item.UpdatedBy = "admin"
	item.UpdatedTime = nowStr()
	id, err := models.AddItem(&item)
	log.Printf("ID: %d, ERR: %v\n", id, err)
	c.JSON(http.StatusOK, ItemResp{"200", "项目日志新增成功", models.TItem{}})
}

// Delete 删除项目日志
func Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Query("id"), 10, 64)
	models.DeleteItem(id)
	c.JSON(http.StatusOK, ItemResp{"200", "删除项目日志成功", models.TItem{}})
}

// Query 根据Id查询项目日志
func Query(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Query("id"), 10, 64)
	item := models.ReadItem(id)
	c.JSON(http.StatusOK, ItemResp{"200", "查询项目日志成功", item})
}

// Update 更新项目日志
func Update(c *gin.Context) {
	var item models.TItem
	req, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusOK, ItemResp{"400", "请求参数错误", models.TItem{}})
		return
	}
	err = json.Unmarshal(req, &item)
	if err != nil {
		c.JSON(http.StatusOK, ItemResp{"400", "请求参数错误", models.TItem{}})
		return
	}
	item.UpdatedTime = nowStr()
	models.UpdateItem(&item)
	c.JSON(http.StatusOK, ItemResp{"200", "更新项目日志成功", models.TItem{}})
}

// ChangeItemStatus 切换项目状态
func ChangeItemStatus(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Query("id"), 10, 64)
	models.ChangeItemStatus(id, nowStr())
	c.JSON(http.StatusOK, ItemResp{"200", "更新项目日志成功", models.TItem{}})
}

// QueryAll 查询所有项目日志
func QueryAll(c *gin.Context) {
	items, _ := models.QueryAllItem()
	c.JSON(http.StatusOK, items)
}

// QueryPage 分页查询项目日志
func QueryPage(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	page := models.QueryPageItem(pageNum, pageSize)
	c.JSON(http.StatusOK, PageResp{"200", "分页查询项目日志成功", page})
}
