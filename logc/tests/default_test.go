package test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ydsz-trace/logc/routers"
	"ydsz-trace/pkg/config"

	"github.com/gin-gonic/gin"
)

// newTestRouter 构建测试用路由
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

// TestRoot 根路径测试
func TestRoot(t *testing.T) {
	router := newTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Root endpoint returned %d, want 200", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Error("Root endpoint returned empty body")
	}
}
