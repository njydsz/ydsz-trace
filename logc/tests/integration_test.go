// Package test 提供 logc 微服务的集成测试，验证路由可用性。
package test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ydsz-trace/logc/routers"
	"ydsz-trace/pkg/config"
)

// setupLogcServer 构造一个用于集成测试的 logc HTTP 测试服务器。
func setupLogcServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := config.NewDefault()
	r := routers.SetupRouter(cfg)
	return httptest.NewServer(r)
}

func TestIntegration_HealthReady(t *testing.T) {
	srv := setupLogcServer(t)
	defer srv.Close()

	for _, path := range []string{"/health", "/ready"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("请求 %s 失败: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s 状态码错误: got %d want %d", path, resp.StatusCode, http.StatusOK)
		}
	}
}

func TestIntegration_Root(t *testing.T) {
	srv := setupLogcServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("请求 / 失败: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / 状态码错误: got %d", resp.StatusCode)
	}
}

func TestIntegration_RegisterEndpoint(t *testing.T) {
	srv := setupLogcServer(t)
	defer srv.Close()

	// 注册接口应可访问（具体业务校验由 handler 决定，此处仅验证路由可用）
	resp, err := http.Post(srv.URL+"/register", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /register 失败: %v", err)
	}
	resp.Body.Close()
	// 期望非 404（路由已注册）
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("/register 路由未注册")
	}
}
