package test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ydsz-trace/logs/routers"
	"ydsz-trace/pkg/config"

	"github.com/gin-gonic/gin"
)

// newTestRouter 构建测试用路由（使用默认配置，不连接数据库）
func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := config.NewDefault()
	return routers.SetupRouter(cfg)
}

// TestHealth 健康检查端点测试
func TestHealth(t *testing.T) {
	router := newTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Health endpoint returned %d, want 200", w.Code)
	}
}

// TestLoginInvalid 未登录访问受保护接口应返回401
func TestLoginRequired(t *testing.T) {
	router := newTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/client/queryAll", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Protected endpoint returned %d, want 401", w.Code)
	}
}
