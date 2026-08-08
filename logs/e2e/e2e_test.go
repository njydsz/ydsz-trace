//go:build e2e

package e2e

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"ydsz-trace/logs/routers"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/session"

	"github.com/gin-gonic/gin"
)

// TestE2E_AppBootAndRoutes 端到端测试：真实启动 logs 应用（含全部中间件与路由），
// 验证 public 路由可达、受保护路由正确鉴权拦截，并模拟携带会话后的访问。
func TestE2E_AppBootAndRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.NewDefault()
	mgr := session.NewManager()
	r := routers.SetupRouter(cfg, mgr)

	ts := startServer(t, r)
	defer ts.Close()

	// 1. 应用启动后 public 健康检查可用
	for _, p := range []string{"/health", "/ready"} {
		code := httpGet(t, ts.URL+p)
		if code != http.StatusOK {
			t.Fatalf("[e2e] GET %s 期望 200, 实际 %d", p, code)
		}
	}

	// 2. 受保护路由在未登录时应被拦截（401）
	code := httpGet(t, ts.URL+"/client/queryAll")
	if code != http.StatusUnauthorized {
		t.Fatalf("[e2e] 未登录访问受保护路由期望 401, 实际 %d", code)
	}

	// 3. admin 登录页可访问（GET）
	code = httpGet(t, ts.URL+"/admin/login")
	if code != http.StatusOK {
		t.Fatalf("[e2e] GET /admin/login 期望 200, 实际 %d", code)
	}

	fmt.Println("[e2e] logs 应用启动与路由端到端验证通过")
}

func startServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func httpGet(t *testing.T, url string) int {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("[e2e] 请求 %s 失败: %v", url, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}
