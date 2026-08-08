package test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"ydsz-trace/logs/routers"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/session"
)

// setupTestServer 构造一个集成测试用的 HTTP server
func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := config.NewDefault()

	mgr := session.NewManager()
	r := routers.SetupRouter(cfg, mgr)
	return httptest.NewServer(r)
}

func TestIntegration_PublicEndpoints(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	cases := []struct {
		path       string
		wantStatus int
	}{
		{"/health", http.StatusOK},
		{"/ready", http.StatusOK},
	}
	for _, c := range cases {
		resp, err := http.Get(srv.URL + c.path)
		if err != nil {
			t.Fatalf("请求 %s 失败: %v", c.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != c.wantStatus {
			t.Fatalf("GET %s 状态码错误: got %d want %d", c.path, resp.StatusCode, c.wantStatus)
		}
	}
}

func TestIntegration_ProtectedRequiresAuth(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	// 未登录访问受保护路由应返回 401
	resp, err := http.Get(srv.URL + "/client/queryAll")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未登录应返回 401，实际 %d", resp.StatusCode)
	}
}

func TestIntegration_LoginFlow(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	// GET 登录页应可访问
	resp, err := http.Get(srv.URL + "/admin/login")
	if err != nil {
		t.Fatalf("登录请求失败: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/admin/login 状态码错误: got %d", resp.StatusCode)
	}
}

func TestIntegration_ConfigLoadsInRouter(t *testing.T) {
	// 验证配置文件可被加载并注入路由（集成：config + routers）
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "app.conf")
	if err := os.WriteFile(cfgPath, []byte("runmode = test\nhost = 127.0.0.1\nport = 8080\n"), 0644); err != nil {
		t.Fatalf("写配置失败: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load 配置失败: %v", err)
	}
	mgr := session.NewManager()
	r := routers.SetupRouter(cfg, mgr)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("路由注入配置后 /health 失败: %d", w.Code)
	}
}
