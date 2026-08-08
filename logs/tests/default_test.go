// Package test 提供 logs 服务端基础路由测试。
package test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ydsz-trace/logs/routers"
	"ydsz-trace/pkg/config"
	"ydsz-trace/pkg/session"
)

// TestMainRoute 根路径测试
func TestMainRoute(t *testing.T) {
	cfg := config.NewDefault()
	sessionMgr := session.NewManager()
	r := routers.SetupRouter(cfg, sessionMgr)

	// GET /health 应返回 200
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /health 期望 200，实际 %d", w.Code)
	}

	// GET /ready 应返回 200
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/ready", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /ready 期望 200，实际 %d", w.Code)
	}

	// 未登录访问 /client/queryAll 应返回 401
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/client/queryAll", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("未登录访问 /client/queryAll 期望 401，实际 %d", w.Code)
	}
}
