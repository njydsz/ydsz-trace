package admin

import (
	"encoding/json"
	"os"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/logs"
)

// UserController 用户控制器
type UserController struct {
	beego.Controller
}

// UserResp 用户响应体
type UserResp struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data User   `json:"data"`
}

// User 用户信息
type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// getAdminUser 从环境变量获取管理员用户名，降级到配置文件
func getAdminUser() string {
	if v := os.Getenv("YDSZ_ADMIN_USER"); v != "" {
		return v
	}
	return beego.AppConfig.String("username")
}

// getAdminPassword 从环境变量获取管理员密码，降级到配置文件
func getAdminPassword() string {
	if v := os.Getenv("YDSZ_ADMIN_PASSWORD"); v != "" {
		return v
	}
	return beego.AppConfig.String("password")
}

// Test 测试路由
func (this *UserController) Test() {
	l := logs.GetLogger()
	l.Println("this is a message of http")
	logs.GetLogger("ORM").Println("this is a message of orm")
	logs.Debug("my book is bought in the year of ", 2016)
	logs.Info("this %s cat is %v years old", "yellow", 3)
	logs.Warn("json is a type of kv like", map[string]int{"key": 2016})
	logs.Error(1024, "is a very", "good game")
	logs.Critical("oh,crash")
	this.Ctx.WriteString("这是正则路由 user/test")
}

// Console 控制台
func (this *UserController) Console() {
	this.TplName = "console.html"
}

// Login 用户登陆接口
func (this *UserController) Login() {
	// 先获取 session，判断用户是否已经登录
	userName := this.GetSession("username")
	if userName != nil {
		if unameStr, ok := userName.(string); ok && unameStr != "" {
			data := UserResp{"200", "用户已经登陆", User{unameStr, ""}}
			this.Data["json"] = &data
			this.ServeJSON()
			return
		}
	}

	// 用户没有登录，获取请求参数
	var user User
	data := this.Ctx.Input.RequestBody
	err := json.Unmarshal(data, &user)
	if err != nil {
		data := UserResp{"400", "请求参数错误", User{}}
		this.Data["json"] = &data
		this.ServeJSON()
		return
	}

	// 优先从环境变量获取，其次从配置文件获取
	uname := getAdminUser()
	upwd := getAdminPassword()

	// 判断用户名、密码是否正确
	if uname == user.Username && upwd == user.Password {
		// 修复：使用用户实际输入的 user.Username，而非从 Session 取出的旧值
		this.SetSession("username", user.Username)
		data := UserResp{"200", "登陆成功", User{uname, ""}}
		this.Data["json"] = &data
		this.ServeJSON()
	} else {
		data := UserResp{"401", "用户名或密码错误", User{}}
		this.Data["json"] = &data
		this.ServeJSON()
	}
}

// Exit 退出登陆
func (this *UserController) Exit() {
	this.DelSession("username")
	this.DelSession("password")
	data := UserResp{"200", "退出成功", User{}}
	this.Data["json"] = &data
	this.ServeJSON()
}
