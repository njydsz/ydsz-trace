package item

import (
	"encoding/json"
	"log"
	"time"

	models "ydsz-trace/logs/models"

	"github.com/astaxie/beego"
)

// ItemController 项目控制器
type ItemController struct {
	beego.Controller
}

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

// Add 新增项目日志
func (this *ItemController) Add() {
	var item models.TItem
	req := this.Ctx.Input.RequestBody
	err := json.Unmarshal(req, &item)
	if err != nil {
		data := ItemResp{"400", "请求参数错误", models.TItem{}}
		this.Data["json"] = &data
		this.ServeJSON()
		return
	}
	item.CreatedBy = "admin"
	item.CreatedTime = time.Now()
	item.UpdatedBy = "admin"
	item.UpdatedTime = time.Now()
	id, err := models.AddItem(&item)
	log.Printf("ID: %d, ERR: %v\n", id, err)
	data := ItemResp{"200", "项目日志新增成功", models.TItem{}}
	this.Data["json"] = &data
	this.ServeJSON()
}

// Delete 删除项目日志
func (this *ItemController) Delete() {
	id, _ := this.GetInt64("id")
	models.DeleteItem(id)
	data := ItemResp{"200", "删除项目日志成功", models.TItem{}}
	this.Data["json"] = &data
	this.ServeJSON()
}

// Query 根据Id查询项目日志
func (this *ItemController) Query() {
	id, _ := this.GetInt64("id")
	item := models.ReadItem(id)
	data := ItemResp{"200", "查询项目日志成功", item}
	this.Data["json"] = &data
	this.ServeJSON()
}

// Update 更新项目日志
func (this *ItemController) Update() {
	var item models.TItem
	req := this.Ctx.Input.RequestBody
	err := json.Unmarshal(req, &item)
	if err != nil {
		data := ItemResp{"400", "请求参数错误", models.TItem{}}
		this.Data["json"] = &data
		this.ServeJSON()
		return
	}
	models.UpdateItem(&item)
	data := ItemResp{"200", "更新项目日志成功", models.TItem{}}
	this.Data["json"] = &data
	this.ServeJSON()
}

// ChangeItemStatus 切换项目状态
func (this *ItemController) ChangeItemStatus() {
	id, _ := this.GetInt64("id")
	models.ChangeItemStatus(id)
	data := ItemResp{"200", "更新项目日志成功", models.TItem{}}
	this.Data["json"] = &data
	this.ServeJSON()
}

// QueryAll 查询所有项目日志
func (this *ItemController) QueryAll() {
	items, _ := models.QueryAllItem()
	this.Data["json"] = &items
	this.ServeJSON()
}

// QueryPage 分页查询项目日志
func (this *ItemController) QueryPage() {
	pageNum, _ := this.GetInt("page")
	pageSize, _ := this.GetInt("limit")
	page := models.QueryPageItem(pageNum, pageSize)
	data := PageResp{"200", "分页查询项目日志成功", page}
	this.Data["json"] = &data
	this.ServeJSON()
}
