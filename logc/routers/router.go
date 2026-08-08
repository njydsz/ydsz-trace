package routers

import (
	"os"
	"strings"

	"ydsz-trace/logc/controllers"
	"ydsz-trace/logc/controllers/file"
	"ydsz-trace/logc/controllers/register"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/plugins/cors"
)

func init() {
	// CORS 白名单：从环境变量 YDSZ_CORS_ORIGINS 读取，逗号分隔
	// 默认为本地开发地址
	corsOrigins := getCORSOrigins()

	beego.InsertFilter("*", beego.BeforeRouter, cors.Allow(&cors.Options{
		AllowOrigins:     corsOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Authorization", "Content-Type", "X-Token", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
	}))
	beego.Router("/", &controllers.MainController{})
	beego.Router("/health", &controllers.MainController{}, "*:Health")
	beego.Router("/ready", &controllers.MainController{}, "*:Ready")
	beego.Router("/file/query", &file.FileController{}, "*:Query")
	beego.Router("/register", &register.RegisterController{}, "*:Register")
	beego.Router("/checkOn", &register.RegisterController{}, "*:CheckOnline")
}

// getCORSOrigins 从环境变量读取 CORS 白名单，逗号分隔
func getCORSOrigins() []string {
	origins := os.Getenv("YDSZ_CORS_ORIGINS")
	if origins == "" {
		// 默认：仅允许本地开发
		return []string{"http://localhost:*", "http://127.0.0.1:*"}
	}
	return strings.Split(origins, ",")
}
