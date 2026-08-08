package routers

import (
	"os"
	"strings"

	"ydsz-trace/logs/controllers/admin"
	"ydsz-trace/logs/controllers/client"
	"ydsz-trace/logs/controllers/item"
	"ydsz-trace/logs/controllers/logs"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/context"
	"github.com/astaxie/beego/plugins/cors"
)

func init() {
	// CORS 白名单：从环境变量 YDSZ_CORS_ORIGINS 读取，逗号分隔
	corsOrigins := getCORSOrigins()

	beego.InsertFilter("*", beego.BeforeRouter, cors.Allow(&cors.Options{
		AllowOrigins:     corsOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type", "X-Token", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
	}))

	// 鉴权中间件：除登录和退出接口外，其他 API 需要登录
	beego.InsertFilter("/client/*", beego.BeforeExec, filterAuth)
	beego.InsertFilter("/item/*", beego.BeforeExec, filterAuth)
	beego.InsertFilter("/logs/*", beego.BeforeExec, filterAuth)
	beego.InsertFilter("/admin/console", beego.BeforeExec, filterAuth)

	beego.Router("/", &admin.UserController{}, "*:Login")
	beego.Router("/health", &admin.UserController{}, "*:Health")
	beego.Router("/ready", &admin.UserController{}, "*:Ready")
	beego.Router("/admin/test", &admin.UserController{}, "*:Test")
	beego.Router("/admin/console", &admin.UserController{}, "*:Console")
	beego.Router("/admin/login", &admin.UserController{}, "*:Login")
	beego.Router("/admin/exit", &admin.UserController{}, "*:Exit")

	beego.Router("/client/register", &client.ClientController{}, "*:Register")
	beego.Router("/client/add", &client.ClientController{}, "*:Add")
	beego.Router("/client/delete", &client.ClientController{}, "*:Delete")
	beego.Router("/client/update", &client.ClientController{}, "*:Update")
	beego.Router("/client/changeStatus", &client.ClientController{}, "*:ChangeClientStatus")
	beego.Router("/client/query", &client.ClientController{}, "*:Query")
	beego.Router("/client/queryAll", &client.ClientController{}, "*:QueryAll")
	beego.Router("/client/queryPage", &client.ClientController{}, "*:QueryPage")

	beego.Router("/item/add", &item.ItemController{}, "*:Add")
	beego.Router("/item/delete", &item.ItemController{}, "*:Delete")
	beego.Router("/item/update", &item.ItemController{}, "*:Update")
	beego.Router("/item/changeStatus", &item.ItemController{}, "*:ChangeItemStatus")
	beego.Router("/item/query", &item.ItemController{}, "*:Query")
	beego.Router("/item/queryAll", &item.ItemController{}, "*:QueryAll")
	beego.Router("/item/queryPage", &item.ItemController{}, "*:QueryPage")

	beego.Router("/logs/query", &logs.LogsController{}, "*:Query")
	beego.Router("/logs/queryClients", &logs.LogsController{}, "*:QueryClient")
	beego.Router("/logs/queryItems", &logs.LogsController{}, "*:QueryItem")
}

// getCORSOrigins 从环境变量读取 CORS 白名单
func getCORSOrigins() []string {
	origins := os.Getenv("YDSZ_CORS_ORIGINS")
	if origins == "" {
		return []string{"http://localhost:*", "http://127.0.0.1:*"}
	}
	return strings.Split(origins, ",")
}

// filterAuth 鉴权中间件：检查用户是否已登录
var filterAuth = func(ctx *context.Context) {
	username := ctx.Input.Session("username")
	if username == nil {
		ctx.Output.SetStatus(401)
		ctx.Output.JSON(map[string]interface{}{
			"code": "401",
			"msg":  "未登录或登录已过期",
			"data": nil,
		}, true, false)
		return
	}
}
