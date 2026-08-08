package client

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	models "ydsz-trace/logs/models"

	"github.com/astaxie/beego"
)

// ClientController 客户端控制器
type ClientController struct {
	beego.Controller
}

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
func (this *ClientController) Add() {
	var client models.TClient
	req := this.Ctx.Input.RequestBody
	err := json.Unmarshal(req, &client)
	if err != nil {
		data := ClientResp{"400", "请求参数错误", models.TClient{}}
		this.Data["json"] = &data
		this.ServeJSON()
		return
	}
	client.Online = "0"
	client.CreatedBy = "admin"
	client.CreatedTime = time.Now()
	client.UpdatedBy = "admin"
	client.UpdatedTime = time.Now()
	id, err := models.AddClient(&client)
	log.Printf("ID: %d, ERR: %v\n", id, err)
	data := ClientResp{"200", "客户端新增成功", models.TClient{}}
	this.Data["json"] = &data
	this.ServeJSON()
}

// Delete 删除客户端
func (this *ClientController) Delete() {
	id, _ := this.GetInt64("id")
	models.DeleteClient(id)
	data := ClientResp{"200", "删除客户端成功", models.TClient{}}
	this.Data["json"] = &data
	this.ServeJSON()
}

// Query 根据Id查询客户端
func (this *ClientController) Query() {
	id, _ := this.GetInt64("id")
	client := models.ReadClient(id)
	data := ClientResp{"200", "查询客户端成功", client}
	this.Data["json"] = &data
	this.ServeJSON()
}

// Update 更新客户端
func (this *ClientController) Update() {
	var client models.TClient
	req := this.Ctx.Input.RequestBody
	err := json.Unmarshal(req, &client)
	if err != nil {
		data := ClientResp{"400", "请求参数错误", models.TClient{}}
		this.Data["json"] = &data
		this.ServeJSON()
		return
	}
	models.UpdateClient(&client)
	data := ClientResp{"200", "更新客户端成功", models.TClient{}}
	this.Data["json"] = &data
	this.ServeJSON()
}

// Register 客户端上线注册（供 logc 代理调用）
func (this *ClientController) Register() {
	// 获取请求的IP
	req := this.Ctx.Request
	addr := req.RemoteAddr
	s := strings.Split(addr, ":")
	ip := s[0]
	port := "2020"

	var register Register
	reqBody := this.Ctx.Input.RequestBody
	err := json.Unmarshal(reqBody, &register)
	if err == nil {
		// 根据ip、port、vkey查询客户端的有效性
		client := models.CheckClient(ip, port, register.VKey)
		if client.Id != 0 {
			c := models.TClient{}
			c.Id = client.Id
			c.Online = "1"
			models.ChangeClientOnline(&c)
		}
	}
	data := ClientResp{"200", "客户端上线成功", models.TClient{}}
	this.Data["json"] = &data
	this.ServeJSON()
}

// ChangeClientStatus 切换客户端状态
func (this *ClientController) ChangeClientStatus() {
	id, _ := this.GetInt64("id")
	models.ChangeClientStatus(id)
	data := ClientResp{"200", "更新客户端成功", models.TClient{}}
	this.Data["json"] = &data
	this.ServeJSON()
}

// QueryAll 查询所有客户端
func (this *ClientController) QueryAll() {
	clients, _ := models.QueryAllClient()
	this.Data["json"] = &clients
	this.ServeJSON()
}

// QueryPage 分页查询客户端
func (this *ClientController) QueryPage() {
	pageNum, _ := this.GetInt("page")
	pageSize, _ := this.GetInt("limit")
	page := models.QueryPageClient(pageNum, pageSize)
	data := PageResp{"200", "分页查询客户端成功", page}
	this.Data["json"] = &data
	this.ServeJSON()
}
