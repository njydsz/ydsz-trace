package test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ydsz-trace/logc/routers"
	"ydsz-trace/pkg/config"
)

// TestMainRoute 根路径测试
func TestMainRoute(t *testing.T) {
	cfg := config.NewDefault()
	r := routers.SetupRouter(cfg)

	// GET / 应返回 200
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET / 期望 200，实际 %d", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Error("GET / 响应体不应为空")
	}

	// GET /health 应返回 200
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/health", nil)
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
}
