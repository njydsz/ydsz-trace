package item

import (
	"net/http"
	"strconv"
	"time"

	models "ydsz-trace/logs/models"

	"github.com/gin-gonic/gin"
)

// queryInt64 从 query 参数解析 int64
func queryInt64(c *gin.Context, key string) (int64, error) {
	return strconv.ParseInt(c.Query(key), 10, 64)
}

// Add 新增项目日志
func Add(c *gin.Context) {
	var item models.TItem
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "400", "msg": "请求参数错误", "data": nil})
		return
	}
	item.CreatedBy = "admin"
	item.CreatedTime = time.Now()
	item.UpdatedBy = "admin"
	item.UpdatedTime = time.Now()
	if _, err := models.AddItem(&item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "项目日志新增失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "项目日志新增成功", "data": nil})
}

// Delete 删除项目日志
func Delete(c *gin.Context) {
	itemId, err := queryInt64(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "400", "msg": "参数错误", "data": nil})
		return
	}
	if _, err := models.DeleteItem(itemId); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "删除项目日志失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "删除项目日志成功", "data": nil})
}

// Query 根据Id查询项目日志
func Query(c *gin.Context) {
	itemId, err := queryInt64(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "400", "msg": "参数错误", "data": nil})
		return
	}
	item, err := models.ReadItem(itemId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "404", "msg": "项目不存在", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "查询项目日志成功", "data": item})
}

// Update 更新项目日志
func Update(c *gin.Context) {
	var item models.TItem
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "400", "msg": "请求参数错误", "data": nil})
		return
	}
	item.UpdatedTime = time.Now()
	if _, err := models.UpdateItem(&item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "更新项目日志失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "更新项目日志成功", "data": nil})
}

// ChangeItemStatus 切换项目状态
func ChangeItemStatus(c *gin.Context) {
	itemId, err := queryInt64(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "400", "msg": "参数错误", "data": nil})
		return
	}
	if _, err := models.ChangeItemStatus(itemId); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "更新项目日志失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "更新项目日志成功", "data": nil})
}

// QueryAll 查询所有项目日志
func QueryAll(c *gin.Context) {
	items, err := models.QueryAllItem()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "查询项目日志失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "查询项目日志成功", "data": items})
}

// QueryPage 分页查询项目日志
func QueryPage(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("limit"))
	page, err := models.QueryPageItem(pageNum, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "500", "msg": "分页查询项目日志失败", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "200", "msg": "分页查询项目日志成功", "data": page})
}
