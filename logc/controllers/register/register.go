package register

import (
	"log"
	"os"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/httplib"
)

// RegisterController 注册控制器
type RegisterController struct {
	beego.Controller
}

// Resp 通用响应
type Resp struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
}

// Register 通过配置文件注册本地客户端
func (this *RegisterController) Register() {
	server := beego.AppConfig.String("logs")
	vKey := getVKey()
	RegisterLocalIp(server, vKey)
}

// CheckOnline 在线检测接口，供 logs 服务端探测
func (this *RegisterController) CheckOnline() {
	data := Resp{"200", "客户端在线"}
	this.Data["json"] = &data
	this.ServeJSON()
}

// getVKey 优先从环境变量获取密钥，降级到配置文件
func getVKey() string {
	if v := os.Getenv("YDSZ_CLIENT_KEY"); v != "" {
		return v
	}
	return beego.AppConfig.String("key")
}

// RegisterLocalIp 启动时自动注册客户端到服务端
func RegisterLocalIp(server string, vKey string) {
	req := httplib.Post("http://" + server + "/client/register").Debug(true)
	req.JSONBody(map[string]interface{}{"key": vKey})
	_, err := req.String()
	log.Printf("logc register url=%v param=%v errMsg=%v\n", "http://"+server+"/client/register", vKey, err)
	if err != nil {
		log.Printf("Local client registered error.")
	} else {
		log.Printf("Local client registered successfully.")
	}
}
