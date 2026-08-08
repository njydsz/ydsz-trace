package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ydsz-trace/pkg/api"
	"ydsz-trace/pkg/auth"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/session"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestFullFlow 集成测试：config 加载 → 生成 JWT → 会话存储 → API 响应
func TestFullFlow(t *testing.T) {
	// 1. 配置加载
	cfg := config.NewDefault()
	_ = cfg

	// 2. 生成并校验 JWT
	j := auth.NewJWT("integration-secret", 0)
	token, err := j.GenerateToken("alice", "admin")
	if err != nil {
		t.Fatalf("生成 token 失败: %v", err)
	}
	claims, err := j.ValidateToken(token)
	if err != nil {
		t.Fatalf("校验 token 失败: %v", err)
	}
	if claims.Username != "alice" {
		t.Fatalf("用户名不匹配: %q", claims.Username)
	}

	// 3. 会话管理
	mgr := session.NewManager()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	mgr.Middleware()(c)
	session.Set(c, "username", claims.Username)
	if session.GetString(c, "username") != "alice" {
		t.Fatalf("会话写入/读取失败")
	}

	// 4. API 响应封装
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	api.Success(c2, "ok", gin.H{"user": claims.Username})
	if w2.Code != http.StatusOK {
		t.Fatalf("API 响应状态码错误: %d", w2.Code)
	}
}
