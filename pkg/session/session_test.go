package session

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newCtx 构造一个经过会话中间件的 gin 测试 context
func newCtx(t *testing.T) (*gin.Context, *Manager) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	m := NewManager()
	m.Middleware()(c)
	return c, m
}

func TestMiddleware_CreatesSession(t *testing.T) {
	c, _ := newCtx(t)
	s := Get(c)
	if s == nil {
		t.Fatalf("中间件应创建会话")
	}
	if cookie := c.Writer.Header().Get("Set-Cookie"); cookie == "" {
		t.Fatalf("应为新会话设置 cookie")
	}
}

func TestSetGetDelete(t *testing.T) {
	c, _ := newCtx(t)
	Set(c, "user", "alice")
	if GetString(c, "user") != "alice" {
		t.Fatalf("Set/Get 失败: got %q", GetString(c, "user"))
	}
	Delete(c, "user")
	if GetString(c, "user") != "" {
		t.Fatalf("Delete 后应为空")
	}
}

func TestSession_PersistsAcrossRequests(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	m := NewManager()
	m.Middleware()(c)
	Set(c, "uid", "42")
	cookie := c.Writer.Header().Get("Set-Cookie")

	// 第二个请求携带 cookie
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	req2, _ := http.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Cookie", cookie)
	c2.Request = req2
	m.Middleware()(c2)

	if GetString(c2, "uid") != "42" {
		t.Fatalf("会话未跨请求保持: got %q", GetString(c2, "uid"))
	}
}

func TestDestroy(t *testing.T) {
	c, m := newCtx(t)
	Set(c, "user", "bob")
	cookie := c.Writer.Header().Get("Set-Cookie")

	// 还原 cookie 到请求，再销毁
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Cookie", cookie)
	c.Request = req
	Destroy(c)

	// 销毁后再用同 token 访问应得到新会话（旧数据消失）
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	req2, _ := http.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Cookie", cookie)
	c2.Request = req2
	m.Middleware()(c2)
	if GetString(c2, "user") != "" {
		t.Fatalf("Destroy 后会话数据应清空: got %q", GetString(c2, "user"))
	}
}
